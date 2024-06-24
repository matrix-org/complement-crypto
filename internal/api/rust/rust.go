package rust

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/internal/api/rust/matrix_sdk_ffi"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
	"golang.org/x/exp/slices"
)

func DeleteOldLogs(prefix string) {
	// delete old log files
	files, _ := os.ReadDir("./logs")
	for _, f := range files {
		if strings.HasPrefix(f.Name(), prefix) {
			os.Remove(filepath.Join("./logs", f.Name()))
		}
	}
}

func SetupLogs(prefix string) {
	// log new files
	matrix_sdk_ffi.SetupTracing(matrix_sdk_ffi.TracingConfiguration{
		WriteToStdoutOrSystem: false,
		Filter:                "debug,hyper=warn,log=warn,eyeball=warn,matrix_sdk_common=trace", //,matrix_sdk_ffi=trace,matrix_sdk=trace,matrix_sdk_crypto=trace,matrix_sdk_base=trace,matrix_sdk_ui=trace",
		WriteToFiles: &matrix_sdk_ffi.TracingFileConfiguration{
			Path:       "./logs",
			FilePrefix: prefix,
		},
	})
}

var zero uint32

type RustRoomInfo struct {
	stream   *matrix_sdk_ffi.TaskHandle
	room     *matrix_sdk_ffi.Room
	timeline []*api.Event
}

type RustClient struct {
	FFIClient             *matrix_sdk_ffi.Client
	roomsListener         *RoomsListener
	allRooms              *matrix_sdk_ffi.RoomList
	rooms                 map[string]*RustRoomInfo
	roomsMu               *sync.RWMutex
	userID                string
	persistentStoragePath string
	opts                  api.ClientCreationOpts

	// for NSE tests
	notifClient *matrix_sdk_ffi.NotificationClient
}

func NewRustClient(t ct.TestLike, opts api.ClientCreationOpts) (api.Client, error) {
	t.Logf("NewRustClient[%s][%s] creating...", opts.UserID, opts.DeviceID)
	matrix_sdk_ffi.LogEvent("rust.go", &zero, matrix_sdk_ffi.LogLevelInfo, t.Name(), fmt.Sprintf("NewRustClient[%s][%s] creating...", opts.UserID, opts.DeviceID))
	ab := matrix_sdk_ffi.NewClientBuilder().HomeserverUrl(opts.BaseURL).SlidingSyncProxy(&opts.SlidingSyncURL).AutoEnableCrossSigning(true)
	var clientSessionDelegate matrix_sdk_ffi.ClientSessionDelegate
	if opts.EnableCrossProcessRefreshLockProcessName != "" {
		t.Logf("enabling cross process refresh lock with proc name=%s", opts.EnableCrossProcessRefreshLockProcessName)
		clientSessionDelegate = NewMemoryClientSessionDelegate()
		ab = ab.EnableCrossProcessRefreshLock(opts.EnableCrossProcessRefreshLockProcessName, clientSessionDelegate)
	}
	var username string
	if opts.PersistentStorage {
		// @alice:hs1, FOOBAR => alice_hs1_FOOBAR
		username = strings.Replace(opts.UserID[1:], ":", "_", -1) + "_" + opts.DeviceID
		ab = ab.SessionPath("rust_storage/" + username).Username(username)
	}
	client, err := ab.Build()
	if err != nil {
		return nil, fmt.Errorf("ClientBuilder.Build failed: %s", err)
	}
	c := &RustClient{
		userID:        opts.UserID,
		FFIClient:     client,
		roomsListener: NewRoomsListener(),
		rooms:         make(map[string]*RustRoomInfo),
		roomsMu:       &sync.RWMutex{},
		opts:          opts,
	}
	if opts.PersistentStorage {
		c.persistentStoragePath = "./rust_storage/" + username
	}
	if opts.AccessToken != "" { // restore the session
		session := matrix_sdk_ffi.Session{
			AccessToken:      opts.AccessToken,
			UserId:           opts.UserID,
			DeviceId:         opts.DeviceID,
			HomeserverUrl:    opts.BaseURL,
			SlidingSyncProxy: &opts.SlidingSyncURL,
		}
		if err := client.RestoreSession(session); err != nil {
			return nil, fmt.Errorf("RestoreSession: %s", err)
		}
		if opts.EnableCrossProcessRefreshLockProcessName == api.ProcessNameNSE {
			clientSessionDelegate.SaveSessionInKeychain(session)
			t.Logf("configure NSE client with logged in user: %+v", session)
			// We purposefully don't SetDelegate as it appears to be unnecessary.
			notifClient, err := client.NotificationClient(matrix_sdk_ffi.NotificationProcessSetupMultipleProcesses{})
			if err != nil {
				return nil, fmt.Errorf("NotificationClient failed: %s", err)
			}
			c.notifClient = notifClient
		}
	}

	c.Logf(t, "NewRustClient[%s] created client storage=%v", opts.UserID, c.persistentStoragePath)
	return &api.LoggedClient{Client: c}, nil
}

func (c *RustClient) Opts() api.ClientCreationOpts {
	// add access token if we weren't made with it
	if c.opts.AccessToken == "" && c.FFIClient != nil {
		session, err := c.FFIClient.Session()
		if err == nil { // if we ain't logged in, we expect an error
			c.opts.AccessToken = session.AccessToken
		}
	}
	return c.opts
}

func (c *RustClient) GetNotification(t ct.TestLike, roomID, eventID string) (*api.Notification, error) {
	if c.notifClient == nil {
		t.Errorf("RustClient misconfigured. You can only call GetNotification if this is an NSE process. " +
			"Ensure opts.EnableCrossProcessRefreshLockProcessName and opts.AccessToken are set!")
		return nil, fmt.Errorf("misconfigured rust client")
	}
	notifItem, err := c.notifClient.GetNotification(roomID, eventID)
	if err != nil {
		return nil, fmt.Errorf("GetNotification: %s", err)
	}
	// TODO: handle NotificationEventInvite
	notifEvent := notifItem.Event.(matrix_sdk_ffi.NotificationEventTimeline)
	// TODO: handle notifications other than messages..
	evType, err := notifEvent.Event.EventType()
	if err != nil {
		return nil, fmt.Errorf("notifItem.Event.EventType => %s", err)
	}
	msgLike := evType.(matrix_sdk_ffi.TimelineEventTypeMessageLike)
	failedToDecrypt := true
	body := ""
	switch msg := msgLike.Content.(type) {
	case matrix_sdk_ffi.MessageLikeEventContentRoomEncrypted:
		// failedToDecrypt = true
	case matrix_sdk_ffi.MessageLikeEventContentRoomMessage:
		failedToDecrypt = false
		switch msgType := msg.MessageType.(type) {
		case matrix_sdk_ffi.MessageTypeText:
			body = msgType.Content.Body
		}

	}
	n := api.Notification{
		Event: api.Event{
			ID:              notifEvent.Event.EventId(),
			Sender:          notifEvent.Event.SenderId(),
			Text:            body,
			FailedToDecrypt: failedToDecrypt,
		},
		HasMentions: notifItem.HasMention,
	}
	return &n, nil
}

func (c *RustClient) Login(t ct.TestLike, opts api.ClientCreationOpts) error {
	var deviceID *string
	if opts.DeviceID != "" {
		deviceID = &opts.DeviceID
	}
	err := c.FFIClient.Login(opts.UserID, opts.Password, nil, deviceID)
	if err != nil {
		return fmt.Errorf("Client.Login failed: %s", err)
	}
	// let the client upload device keys and OTKs
	e := c.FFIClient.Encryption()
	e.WaitForE2eeInitializationTasks()
	e.Destroy()
	return nil
}

func (c *RustClient) CurrentAccessToken(t ct.TestLike) string {
	s, err := c.FFIClient.Session()
	if err != nil {
		ct.Fatalf(t, "error retrieving session: %s", err)
	}
	return s.AccessToken
}

func (c *RustClient) RequestOwnUserVerification(t ct.TestLike, listener api.VerificationListener) {
	svc, err := c.FFIClient.GetSessionVerificationController()
	if err != nil {
		ct.Fatalf(t, "GetSessionVerificationController: %s", err)
	}
	delegateImpl := &SessionVerificationControllerDelegate{
		t:          t,
		controller: svc,
		listener:   listener,
	}
	c.FFIClient.Encryption().VerificationStateListener(delegateImpl)

	var delegate matrix_sdk_ffi.SessionVerificationControllerDelegate = delegateImpl
	svc.SetDelegate(&delegate)
	if err = svc.RequestVerification(); err != nil {
		ct.Fatalf(t, "RequestVerification: %s", err)
	}
}

func (c *RustClient) DeletePersistentStorage(t ct.TestLike) {
	t.Helper()
	if c.persistentStoragePath != "" {
		err := os.RemoveAll(c.persistentStoragePath)
		if err != nil {
			ct.Fatalf(t, "DeletePersistentStorage: %s", err)
		}
	}
}
func (c *RustClient) ForceClose(t ct.TestLike) {
	t.Helper()
	t.Fatalf("Cannot force close a rust client, use an RPC client instead.")
}

func (c *RustClient) Close(t ct.TestLike) {
	t.Helper()
	c.roomsMu.Lock()
	for _, rri := range c.rooms {
		if rri.stream != nil {
			// ensure we don't see AddTimelineListener callbacks as they can cause panics
			// if we t.Logf after t has passed/failed.
			rri.stream.Cancel()
		}
	}
	c.roomsMu.Unlock()
	c.FFIClient.Destroy()
	c.FFIClient = nil
	if c.notifClient != nil {
		c.notifClient.Destroy()
	}
}

func (c *RustClient) MustGetEvent(t ct.TestLike, roomID, eventID string) api.Event {
	t.Helper()
	room := c.findRoom(t, roomID)
	timelineItem, err := mustGetTimeline(t, room).GetEventTimelineItemByEventId(eventID)
	if err != nil {
		ct.Fatalf(t, "MustGetEvent(rust) %s (%s, %s): %s", c.userID, roomID, eventID, err)
	}
	ev := eventTimelineItemToEvent(timelineItem)
	if ev == nil {
		ct.Fatalf(t, "MustGetEvent(rust) %s (%s, %s): found timeline item but failed to convert it to an Event", c.userID, roomID, eventID)
	}
	return *ev
}

func (c *RustClient) MustStartSyncing(t ct.TestLike) (stopSyncing func()) {
	t.Helper()
	stopSyncing, err := c.StartSyncing(t)
	must.NotError(t, "StartSyncing", err)
	return stopSyncing
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
func (c *RustClient) StartSyncing(t ct.TestLike) (stopSyncing func(), err error) {
	t.Helper()
	// It's critical that we destroy the sync_service_builder object before we return.
	// You might be tempted to chain this function call e.g FFIClient.SyncService().Finish()
	// but if you do that, the builder is never destroyed. If that happens, the builder will
	// eventually be destroyed by Go finialisers running, but at that point we may not have
	// a tokio runtime running anymore. This will then cause a panic with something to the effect of:
	//  > thread '<unnamed>' panicked at 'there is no reactor running, must be called from the context of a Tokio 1.x runtime'
	// where the stack trace doesn't hit any test code, but does start at a `free_` function.
	sb := c.FFIClient.SyncService()
	if c.opts.EnableCrossProcessRefreshLockProcessName != "" {
		sb2 := sb.WithCrossProcessLock(&c.opts.EnableCrossProcessRefreshLockProcessName)
		sb.Destroy()
		sb = sb2
	}
	defer sb.Destroy()
	syncService, err := sb.Finish()
	if err != nil {
		return nil, fmt.Errorf("[%s]failed to make sync service: %s", c.userID, err)
	}
	rls := syncService.RoomListService()
	roomList, err := rls.AllRooms()
	if err != nil {
		return nil, fmt.Errorf("[%s]failed to call SyncService.RoomListService.AllRooms: %s", c.userID, err)
	}
	must.NotEqual(t, roomList, nil, "AllRooms room list must not be nil")
	genericListener := newGenericStateListener[matrix_sdk_ffi.RoomListLoadingState]()
	result, err := roomList.LoadingState(genericListener)
	if err != nil {
		return nil, fmt.Errorf("[%s]failed to call RoomList.LoadingState: %s", c.userID, err)
	}
	go syncService.Start()
	c.allRooms = roomList
	// track new rooms when they are made
	allRoomsListener := newGenericStateListener[[]matrix_sdk_ffi.RoomListEntriesUpdate]()
	go func() {
		var allRoomIds DynamicSlice[matrix_sdk_ffi.RoomListEntry]
		for !allRoomsListener.isClosed.Load() {
			updates := <-allRoomsListener.ch
			var newEntries []matrix_sdk_ffi.RoomListEntry
			for _, update := range updates {
				switch x := update.(type) {
				case matrix_sdk_ffi.RoomListEntriesUpdateAppend:
					allRoomIds.Append(x.Values...)
					newEntries = append(newEntries, x.Values...)
				case matrix_sdk_ffi.RoomListEntriesUpdateInsert:
					allRoomIds.Insert(int(x.Index), x.Value)
					newEntries = append(newEntries, x.Value)
				case matrix_sdk_ffi.RoomListEntriesUpdatePushBack:
					allRoomIds.PushBack(x.Value)
					newEntries = append(newEntries, x.Value)
				case matrix_sdk_ffi.RoomListEntriesUpdatePushFront:
					allRoomIds.PushFront(x.Value)
					newEntries = append(newEntries, x.Value)
				case matrix_sdk_ffi.RoomListEntriesUpdateSet:
					allRoomIds.Set(int(x.Index), x.Value)
					newEntries = append(newEntries, x.Value)
				case matrix_sdk_ffi.RoomListEntriesUpdateClear:
					allRoomIds.Clear()
				case matrix_sdk_ffi.RoomListEntriesUpdatePopBack:
					allRoomIds.PopBack()
				case matrix_sdk_ffi.RoomListEntriesUpdatePopFront:
					allRoomIds.PopFront()
				case matrix_sdk_ffi.RoomListEntriesUpdateRemove:
					allRoomIds.Remove(int(x.Index))
				case matrix_sdk_ffi.RoomListEntriesUpdateReset:
					allRoomIds.Reset(x.Values)
					newEntries = append(newEntries, x.Values...)
				case matrix_sdk_ffi.RoomListEntriesUpdateTruncate:
					allRoomIds.Truncate(int(x.Length))
				default:
					c.Logf(t, "unhandled all rooms update: %+v", update)
				}
			}
			// inform anything waiting on this room that it exists
			for _, entry := range newEntries {
				switch x := entry.(type) {
				case matrix_sdk_ffi.RoomListEntryFilled:
					c.roomsListener.BroadcastUpdateForRoom(x.RoomId)
				}
			}
		}
	}()
	c.allRooms.Entries(allRoomsListener)

	isSyncing := false

	for !isSyncing {
		select {
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("[%s](rust) timed out after 5s StartSyncing", c.userID)
		case state := <-genericListener.ch:
			switch state.(type) {
			case matrix_sdk_ffi.RoomListLoadingStateLoaded:
				isSyncing = true
			case matrix_sdk_ffi.RoomListLoadingStateNotLoaded:
				isSyncing = false
			}
		}
	}
	genericListener.Close()

	result.StateStream.Cancel()

	return func() {
		t.Logf("%s: Stopping sync service", c.userID)
		// we need to destroy all of these as they have been allocated Rust side.
		// If we don't, then the Go GC will eventually call Destroy for us, but
		// by this point there will be no tokio runtime running, which will then
		// cause a panic (as cleanup code triggered by Destroy calls async functions)
		roomList.Destroy()
		rls.Destroy()
		syncService.Stop()
		syncService.Destroy()
	}, nil
}

// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
// provide a bogus room ID.
func (c *RustClient) IsRoomEncrypted(t ct.TestLike, roomID string) (bool, error) {
	t.Helper()
	r := c.findRoom(t, roomID)
	if r == nil {
		rooms := c.FFIClient.Rooms()
		return false, fmt.Errorf("failed to find room %s, got %d rooms", roomID, len(rooms))
	}
	return r.IsEncrypted()
}

func (c *RustClient) MustBackupKeys(t ct.TestLike) (recoveryKey string) {
	t.Helper()
	genericListener := newGenericStateListener[matrix_sdk_ffi.EnableRecoveryProgress]()
	var listener matrix_sdk_ffi.EnableRecoveryProgressListener = genericListener
	e := c.FFIClient.Encryption()
	defer e.Destroy()
	recoveryKey, err := e.EnableRecovery(true, listener)
	must.NotError(t, "Encryption.EnableRecovery", err)
	for !genericListener.isClosed.Load() {
		select {
		case s := <-genericListener.ch:
			switch x := s.(type) {
			case matrix_sdk_ffi.EnableRecoveryProgressCreatingBackup:
				t.Logf("MustBackupKeys: state=CreatingBackup")
			case matrix_sdk_ffi.EnableRecoveryProgressBackingUp:
				t.Logf("MustBackupKeys: state=BackingUp %v/%v", x.BackedUpCount, x.TotalCount)
			case matrix_sdk_ffi.EnableRecoveryProgressCreatingRecoveryKey:
				t.Logf("MustBackupKeys: state=CreatingRecoveryKey")
			case matrix_sdk_ffi.EnableRecoveryProgressDone:
				t.Logf("MustBackupKeys: state=Done")
				genericListener.Close() // break the loop
			}
		case <-time.After(5 * time.Second):
			ct.Fatalf(t, "timed out enabling backup keys")
		}
	}
	return recoveryKey
}

func (c *RustClient) LoadBackup(t ct.TestLike, recoveryKey string) error {
	t.Helper()
	e := c.FFIClient.Encryption()
	defer e.Destroy()
	return e.Recover(recoveryKey)
}

func (c *RustClient) MustLoadBackup(t ct.TestLike, recoveryKey string) {
	t.Helper()
	c.LoadBackup(t, recoveryKey)
}

func (c *RustClient) WaitUntilEventInRoom(t ct.TestLike, roomID string, checker func(api.Event) bool) api.Waiter {
	t.Helper()
	c.ensureListening(t, roomID)
	return &timelineWaiter{
		roomID:  roomID,
		checker: checker,
		client:  c,
	}
}

func (c *RustClient) Type() api.ClientTypeLang {
	return api.ClientTypeRust
}

// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
// room. Returns the event ID of the sent event.
func (c *RustClient) SendMessage(t ct.TestLike, roomID, text string) (eventID string) {
	t.Helper()
	eventID, err := c.TrySendMessage(t, roomID, text)
	if err != nil {
		ct.Fatalf(t, err.Error())
	}
	return eventID
}

func (c *RustClient) TrySendMessage(t ct.TestLike, roomID, text string) (eventID string, err error) {
	t.Helper()
	var isChannelClosed atomic.Bool
	ch := make(chan bool)
	// we need a timeline listener before we can send messages, AND that listener must be attached to the
	// same *Room you call .Send on :S
	c.ensureListening(t, roomID)
	r := c.findRoom(t, roomID)
	cancel := c.roomsListener.AddListener(func(broadcastRoomID string) bool {
		if roomID != broadcastRoomID {
			return false
		}
		info := c.rooms[roomID]
		if info == nil {
			return false
		}
		for _, ev := range info.timeline {
			if ev == nil {
				continue
			}
			if ev.Text == text && ev.Sender == c.userID && ev.ID != "" {
				// if we haven't seen this event yet, assign the return arg and signal that
				// the function should unblock. It's important to only close the channel once
				// else this will panic on the 2nd call.
				if eventID == "" {
					eventID = ev.ID
					if isChannelClosed.CompareAndSwap(false, true) {
						close(ch)
					}
				}
			}
		}
		return false
	})
	defer cancel()
	if r == nil {
		err = fmt.Errorf("SendMessage(rust) %s: failed to find room %s", c.userID, roomID)
		return
	}
	timeline, err := r.Timeline()
	if err != nil {
		err = fmt.Errorf("SendMessage(rust) %s: %s", c.userID, err)
		return
	}
	timeline.Send(matrix_sdk_ffi.MessageEventContentFromHtml(text, text))
	select {
	case <-time.After(11 * time.Second):
		err = fmt.Errorf("SendMessage(rust) %s: timed out after 11s", c.userID)
		return
	case <-ch:
		return
	}
}

func (c *RustClient) MustBackpaginate(t ct.TestLike, roomID string, count int) {
	t.Helper()
	r := c.findRoom(t, roomID)
	must.NotEqual(t, r, nil, "unknown room")
	_, err := mustGetTimeline(t, r).PaginateBackwards(uint16(count))
	must.NotError(t, "failed to backpaginate", err)
}

func (c *RustClient) UserID() string {
	return c.userID
}

func (c *RustClient) findRoomInCache(roomID string) *matrix_sdk_ffi.Room {
	c.roomsMu.RLock()
	defer c.roomsMu.RUnlock()
	// do we have a reference to it already?
	roomInfo := c.rooms[roomID]
	if roomInfo != nil {
		return roomInfo.room
	}
	return nil
}

// findRoom tries to find the room in the FFI client. Has a cache of already found rooms to ensure
// the same pointer is always returned for the same room.
func (c *RustClient) findRoom(t ct.TestLike, roomID string) *matrix_sdk_ffi.Room {
	t.Helper()
	room := c.findRoomInCache(roomID)
	if room != nil {
		return room
	}
	// try to find it in all_rooms
	if c.allRooms != nil {
		roomListItem, err := c.allRooms.Room(roomID)
		if err != nil {
			c.Logf(t, "allRooms.Room(%s) err: %s", roomID, err)
		} else if roomListItem != nil {
			if !roomListItem.IsTimelineInitialized() {
				if err = roomListItem.InitTimeline(nil, nil); err != nil {
					c.Logf(t, "allRooms.InitTimeline(%s) err: %s", roomID, err)
				}
			}
			room, err := roomListItem.FullRoom()
			if err != nil {
				c.Logf(t, "allRooms.FullRoom(%s) err: %s", roomID, err)
			} else {
				c.roomsMu.Lock()
				c.rooms[roomID] = &RustRoomInfo{
					room: room,
				}
				c.roomsMu.Unlock()
				return room
			}
		}
	}
	// try to find it from FFI
	rooms := c.FFIClient.Rooms()
	for i, r := range rooms {
		rid := r.Id()
		// ensure we only store rooms once
		_, exists := c.rooms[rid]
		if !exists {
			c.roomsMu.Lock()
			c.rooms[rid] = &RustRoomInfo{
				room: rooms[i],
			}
			c.roomsMu.Unlock()
		}
		if r.Id() == roomID {
			return c.rooms[rid].room
		}
	}
	// we really don't know about this room yet
	return nil
}

func (c *RustClient) Logf(t ct.TestLike, format string, args ...interface{}) {
	t.Helper()
	c.logToFile(t, format, args...)
	t.Logf(format, args...)
}

func (c *RustClient) logToFile(t ct.TestLike, format string, args ...interface{}) {
	matrix_sdk_ffi.LogEvent("rust.go", &zero, matrix_sdk_ffi.LogLevelInfo, t.Name(), fmt.Sprintf(format, args...))
}

func (c *RustClient) ensureListening(t ct.TestLike, roomID string) {
	t.Helper()
	r := c.findRoom(t, roomID)
	if r == nil {
		// we allow the room to not exist yet. If this happens, wait until we see the room before continuing
		c.roomsListener.AddListener(func(broadcastRoomID string) bool {
			if broadcastRoomID != roomID {
				return false
			}
			if room := c.findRoom(t, roomID); room != nil {
				c.ensureListening(t, roomID) // this should work now
				return true
			}
			return false
		})
		return
	}
	must.NotEqual(t, r, nil, fmt.Sprintf("room %s does not exist", roomID))

	info := c.rooms[roomID]
	if info != nil && info.stream != nil {
		return
	}

	c.Logf(t, "[%s]AddTimelineListener[%s]", c.userID, roomID)
	// we need a timeline listener before we can send messages. Ensure we insert the initial
	// set of items prior to handling updates. If we don't wait, we risk the listener firing
	// _before_ we have set the initial entries in the timeline. This would cause a lost update
	// as setting the initial entries clears the timeline, which can then result in test flakes.
	waiter := helpers.NewWaiter()
	result := mustGetTimeline(t, r).AddListener(&timelineListener{fn: func(diff []*matrix_sdk_ffi.TimelineDiff) {
		waiter.Waitf(t, 5*time.Second, "timed out waiting for Timeline.AddListener to return")
		timeline := c.rooms[roomID].timeline
		var newEvents []*api.Event
		c.Logf(t, "[%s]AddTimelineListener[%s] TimelineDiff len=%d", c.userID, roomID, len(diff))
		for _, d := range diff {
			switch d.Change() {
			case matrix_sdk_ffi.TimelineChangeInsert:
				insertData := d.Insert()
				if insertData == nil {
					continue
				}
				i := int(insertData.Index)
				if i >= len(timeline) {
					t.Logf("TimelineListener[%s] INSERT %d out of bounds of events timeline of size %d", roomID, i, len(timeline))
					if i == len(timeline) {
						t.Logf("TimelineListener[%s] treating as append", roomID)
						timeline = append(timeline, timelineItemToEvent(insertData.Item))
						newEvents = append(newEvents, timeline[i])
					}
					continue
				}
				timeline = slices.Insert(timeline, i, timelineItemToEvent(insertData.Item))
				c.logToFile(t, "[%s]_______ INSERT %+v\n", c.userID, timeline[i])
				newEvents = append(newEvents, timeline[i])
			case matrix_sdk_ffi.TimelineChangeRemove:
				removeData := d.Remove()
				if removeData == nil {
					continue
				}
				i := int(*removeData)
				if i >= len(timeline) {
					t.Logf("TimelineListener[%s] REMOVE %d out of bounds of events timeline of size %d", roomID, i, len(timeline))
					continue
				}
				timeline = slices.Delete(timeline, i, i+1)
			case matrix_sdk_ffi.TimelineChangeAppend:
				appendItems := d.Append()
				if appendItems == nil {
					continue
				}
				for _, item := range *appendItems {
					ev := timelineItemToEvent(item)
					timeline = append(timeline, ev)
					c.logToFile(t, "[%s]_______ APPEND %+v\n", c.userID, ev)
					newEvents = append(newEvents, ev)
				}
			case matrix_sdk_ffi.TimelineChangeReset:
				resetItems := d.Reset()
				if resetItems == nil {
					continue
				}
				timeline = make([]*api.Event, len(*resetItems))
				for i, item := range *resetItems {
					ev := timelineItemToEvent(item)
					timeline[i] = ev
					c.logToFile(t, "[%s]_______ RESET %+v\n", c.userID, ev)
					newEvents = append(newEvents, ev)
				}
			case matrix_sdk_ffi.TimelineChangePushBack: // append but 1 element
				pbData := d.PushBack()
				if pbData == nil {
					continue
				}
				ev := timelineItemToEvent(*pbData)
				timeline = append(timeline, ev)
				c.logToFile(t, "[%s]_______ PUSH BACK %+v\n", c.userID, ev)
				newEvents = append(newEvents, ev)
			case matrix_sdk_ffi.TimelineChangeSet:
				setData := d.Set()
				if setData == nil {
					continue
				}
				ev := timelineItemToEvent(setData.Item)
				i := int(setData.Index)
				if i > len(timeline) { // allow appends, hence > not >=
					t.Logf("TimelineListener[%s] SET %d out of bounds of events timeline of size %d", roomID, i, len(timeline))
					continue
				} else if i < len(timeline) {
					timeline[i] = ev
				} else if i == len(timeline) {
					timeline = append(timeline, ev)
				}
				c.logToFile(t, "[%s]_______ SET %+v\n", c.userID, ev)
				newEvents = append(newEvents, ev)
			case matrix_sdk_ffi.TimelineChangePushFront:
				pushFrontData := d.PushFront()
				if pushFrontData == nil {
					continue
				}
				ev := timelineItemToEvent(*pushFrontData)
				timeline = slices.Insert(timeline, 0, ev)
				newEvents = append(newEvents, ev)
			default:
				t.Logf("Unhandled TimelineDiff change %v", d.Change())
			}
		}
		c.rooms[roomID].timeline = timeline
		c.roomsListener.BroadcastUpdateForRoom(roomID)
		for _, e := range newEvents {
			c.Logf(t, "[%s]TimelineDiff change: %+v", c.userID, e)
		}
	}})
	c.rooms[roomID].stream = result
	c.rooms[roomID].timeline = make([]*api.Event, 0)
	c.Logf(t, "[%s]AddTimelineListener[%s] set up", c.userID, roomID)
	waiter.Finish()
}

type timelineWaiter struct {
	roomID  string
	checker func(e api.Event) bool
	client  *RustClient
}

func (w *timelineWaiter) Waitf(t ct.TestLike, s time.Duration, format string, args ...any) {
	t.Helper()
	err := w.TryWaitf(t, s, format, args...)
	if err != nil {
		ct.Fatalf(t, err.Error())
	}
}

func (w *timelineWaiter) TryWaitf(t ct.TestLike, s time.Duration, format string, args ...any) error {
	t.Helper()

	checkForEvent := func() bool {
		t.Helper()
		// check if it exists in the timeline already
		info := w.client.rooms[w.roomID]
		if info == nil {
			w.client.logToFile(t, "_____checkForEvent[%s] room does not exist\n", w.client.userID)
			return false
		}
		for _, ev := range info.timeline {
			if ev == nil {
				continue
			}
			if w.checker(*ev) {
				t.Logf("%s: Wait[%s]: event exists in the timeline", w.client.userID, w.roomID)
				return true
			}
		}
		w.client.logToFile(t, "_____checkForEvent[%s] checked %d timeline events and no match \n", w.client.userID, len(info.timeline))
		return false
	}

	if checkForEvent() {
		return nil // event exists
	}

	updates := make(chan bool, 3)
	var isClosed atomic.Bool
	cancel := w.client.roomsListener.AddListener(func(roomID string) bool {
		if isClosed.Load() {
			return true
		}
		if w.roomID != roomID {
			return false
		}
		if !checkForEvent() {
			return false
		}
		if isClosed.CompareAndSwap(false, true) {
			close(updates)
		}
		return true
	})
	defer cancel()

	// check again in case it was added after the previous checkForEvent but before AddListener
	if checkForEvent() {
		return nil // event exists
	}

	msg := fmt.Sprintf(format, args...)
	// either no timeline or doesn't exist yet, start blocking
	start := time.Now()
	for {
		timeLeft := s - time.Since(start)
		if timeLeft <= 0 {
			return fmt.Errorf("%s (rust): Wait[%s]: timed out: %s", w.client.userID, w.roomID, msg)
		}
		select {
		case <-time.After(timeLeft):
			return fmt.Errorf("%s (rust): Wait[%s]: timed out %s", w.client.userID, w.roomID, msg)
		case <-updates:
			return nil // event exists
		}
	}
}

func mustGetTimeline(t ct.TestLike, room *matrix_sdk_ffi.Room) *matrix_sdk_ffi.Timeline {
	if room == nil {
		ct.Fatalf(t, "mustGetTimeline: room does not exist")
	}
	timeline, err := room.Timeline()
	must.NotError(t, "failed to get room timeline", err)
	return timeline
}

type timelineListener struct {
	fn func(diff []*matrix_sdk_ffi.TimelineDiff)
}

func (l *timelineListener) OnUpdate(diff []*matrix_sdk_ffi.TimelineDiff) {
	l.fn(diff)
}

func timelineItemToEvent(item *matrix_sdk_ffi.TimelineItem) *api.Event {
	ev := item.AsEvent()
	if ev == nil { // e.g day divider
		return nil
	}
	return eventTimelineItemToEvent(*ev)
}

func eventTimelineItemToEvent(item *matrix_sdk_ffi.EventTimelineItem) *api.Event {
	if item == nil {
		return nil
	}
	eventID := ""
	if item.EventId() != nil {
		eventID = *item.EventId()
	}
	complementEvent := api.Event{
		ID:     eventID,
		Sender: item.Sender(),
	}
	switch k := item.Content().Kind().(type) {
	case matrix_sdk_ffi.TimelineItemContentKindRoomMembership:
		complementEvent.Target = k.UserId
		change := *k.Change
		switch change {
		case matrix_sdk_ffi.MembershipChangeInvited:
			complementEvent.Membership = "invite"
		case matrix_sdk_ffi.MembershipChangeBanned:
			fallthrough
		case matrix_sdk_ffi.MembershipChangeKickedAndBanned:
			complementEvent.Membership = "ban"
		case matrix_sdk_ffi.MembershipChangeJoined:
			fallthrough
		case matrix_sdk_ffi.MembershipChangeInvitationAccepted:
			complementEvent.Membership = "join"
		case matrix_sdk_ffi.MembershipChangeLeft:
			fallthrough
		case matrix_sdk_ffi.MembershipChangeInvitationRevoked:
			fallthrough
		case matrix_sdk_ffi.MembershipChangeInvitationRejected:
			fallthrough
		case matrix_sdk_ffi.MembershipChangeKicked:
			fallthrough
		case matrix_sdk_ffi.MembershipChangeUnbanned:
			complementEvent.Membership = "leave"
		default:
			fmt.Printf("%s unhandled membership %d\n", k.UserId, change)
		}
	case matrix_sdk_ffi.TimelineItemContentKindUnableToDecrypt:
		complementEvent.FailedToDecrypt = true
	}

	content := item.Content()
	if content != nil {
		msg := content.AsMessage()
		if msg != nil {
			msgg := *msg
			complementEvent.Text = msgg.Body()
		}
	}
	return &complementEvent
}

// you call requestVerification(), then you wait for acceptedVerificationRequest and then you
// call startSasVerification
// you should then receivedVerificationData and approveVerification or declineVerification
type SessionVerificationControllerDelegate struct {
	t          ct.TestLike
	controller *matrix_sdk_ffi.SessionVerificationController
	listener   api.VerificationListener
}

func (s *SessionVerificationControllerDelegate) DidAcceptVerificationRequest() {
	s.t.Logf("SessionVerificationControllerDelegate.DidAcceptVerificationRequest")
	s.t.Logf("SessionVerificationControllerDelegate calling StartSasVerification")
	s.listener.DidAcceptVerificationRequest()
	if err := s.controller.StartSasVerification(); err != nil {
		ct.Fatalf(s.t, "StartSasVerification: %s", err)
	}
}

func (s *SessionVerificationControllerDelegate) DidStartSasVerification() {
	s.t.Logf("SessionVerificationControllerDelegate.DidStartSasVerification")
}

func (s *SessionVerificationControllerDelegate) DidReceiveVerificationData(data matrix_sdk_ffi.SessionVerificationData) {
	s.t.Logf("SessionVerificationControllerDelegate.DidReceiveVerificationData")
	vData := api.VerificationData{}
	switch d := data.(type) {
	case *matrix_sdk_ffi.SessionVerificationDataEmojis:
		var symbols []string
		for _, emoji := range d.Emojis {
			symbols = append(symbols, emoji.Symbol())
		}
		vData.Emojis = symbols
	case *matrix_sdk_ffi.SessionVerificationDataDecimals:
		ct.Fatalf(s.t, "DidReceiveVerificationData: decimals unsupported")
		vData.Decimals = d.Values
	}
	s.listener.DidReceiveVerificationData(vData)
}

func (s *SessionVerificationControllerDelegate) DidFail() {
	s.t.Logf("SessionVerificationControllerDelegate.DidFail")
	s.listener.DidFail()
}

func (s *SessionVerificationControllerDelegate) DidCancel() {
	s.t.Logf("SessionVerificationControllerDelegate.DidCancel")
	s.listener.DidCancel()
}

func (s *SessionVerificationControllerDelegate) DidFinish() {
	s.t.Logf("SessionVerificationControllerDelegate.DidFinish")
	s.listener.DidFinish()
}

func (s *SessionVerificationControllerDelegate) OnUpdate(status matrix_sdk_ffi.VerificationState) {
	s.t.Logf("SessionVerificationControllerDelegate.OnUpdate %v", status)
	var vState = api.VerificationStateUnknown
	switch status {
	case matrix_sdk_ffi.VerificationStateUnverified:
		vState = api.VerificationStateUnverified
	case matrix_sdk_ffi.VerificationStateVerified:
		vState = api.VerificationStateVerified
	}
	s.listener.OnVerificationStateChange(vState)
}
