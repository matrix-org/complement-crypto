package rust

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement-crypto/rust/matrix_sdk_ffi"
	"github.com/matrix-org/complement/must"
	"golang.org/x/exp/slices"
)

func init() {
	matrix_sdk_ffi.SetupTracing(matrix_sdk_ffi.TracingConfiguration{
		WriteToStdoutOrSystem: false,
		Filter:                "debug",
		WriteToFiles: &matrix_sdk_ffi.TracingFileConfiguration{
			Path:       ".",
			FilePrefix: "rust_sdk_logs",
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
	FFIClient  *matrix_sdk_ffi.Client
	listeners  map[int32]func(roomID string)
	listenerID atomic.Int32
	allRooms   *matrix_sdk_ffi.RoomList
	rooms      map[string]*RustRoomInfo
	roomsMu    *sync.RWMutex
	userID     string
}

func NewRustClient(t *testing.T, opts api.ClientCreationOpts, ssURL string) (api.Client, error) {
	t.Logf("NewRustClient[%s] creating...", opts.UserID)
	ab := matrix_sdk_ffi.NewClientBuilder().HomeserverUrl(opts.BaseURL).SlidingSyncProxy(&ssURL)
	client, err := ab.Build()
	if err != nil {
		return nil, fmt.Errorf("ClientBuilder.Build failed: %s", err)
	}
	var deviceID *string
	if opts.DeviceID != "" {
		deviceID = &opts.DeviceID
	}
	err = client.Login(opts.UserID, opts.Password, nil, deviceID)
	if err != nil {
		return nil, fmt.Errorf("Client.Login failed: %s", err)
	}
	c := &RustClient{
		userID:    opts.UserID,
		FFIClient: client,
		rooms:     make(map[string]*RustRoomInfo),
		listeners: make(map[int32]func(roomID string)),
		roomsMu:   &sync.RWMutex{},
	}
	c.Logf(t, "NewRustClient[%s] created client", opts.UserID)
	return &api.LoggedClient{Client: c}, nil
}

func (c *RustClient) Close(t *testing.T) {
	t.Helper()
	c.roomsMu.Lock()
	for _, rri := range c.rooms {
		if rri.stream != nil {
			// ensure we don't see AddTimelineListener callbacks as they can cause panics
			// if we t.Logf after t has passed/failed.
			rri.stream.Cancel()
		}
	}
	c.FFIClient.Destroy()
}

func (c *RustClient) MustGetEvent(t *testing.T, roomID, eventID string) api.Event {
	t.Helper()
	room := c.findRoom(t, roomID)
	timelineItem, err := room.Timeline().GetEventTimelineItemByEventId(eventID)
	if err != nil {
		api.Fatalf(t, "MustGetEvent(rust) %s (%s, %s): %s", c.userID, roomID, eventID, err)
	}
	ev := eventTimelineItemToEvent(timelineItem)
	if ev == nil {
		api.Fatalf(t, "MustGetEvent(rust) %s (%s, %s): found timeline item but failed to convert it to an Event", c.userID, roomID, eventID)
	}
	return *ev
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
func (c *RustClient) StartSyncing(t *testing.T) (stopSyncing func()) {
	t.Helper()
	syncService, err := c.FFIClient.SyncService().Finish()
	must.NotError(t, fmt.Sprintf("[%s]failed to make sync service", c.userID), err)
	roomList, err := syncService.RoomListService().AllRooms()
	must.NotError(t, "failed to call SyncService.RoomListService.AllRooms", err)
	genericListener := newGenericStateListener[matrix_sdk_ffi.RoomListLoadingState]()
	result, err := roomList.LoadingState(genericListener)
	must.NotError(t, "failed to call RoomList.LoadingState", err)
	go syncService.Start()
	c.allRooms = roomList

	isSyncing := false

	for !isSyncing {
		select {
		case <-time.After(5 * time.Second):
			api.Fatalf(t, "[%s](rust) timed out after 5s StartSyncing", c.userID)
		case state := <-genericListener.ch:
			fmt.Println(state)
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
		syncService.Stop()
	}
}

// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
// provide a bogus room ID.
func (c *RustClient) IsRoomEncrypted(t *testing.T, roomID string) (bool, error) {
	t.Helper()
	r := c.findRoom(t, roomID)
	if r == nil {
		rooms := c.FFIClient.Rooms()
		return false, fmt.Errorf("failed to find room %s, got %d rooms", roomID, len(rooms))
	}
	return r.IsEncrypted()
}

func (c *RustClient) MustBackupKeys(t *testing.T) (recoveryKey string) {
	t.Helper()
	genericListener := newGenericStateListener[matrix_sdk_ffi.EnableRecoveryProgress]()
	var listener matrix_sdk_ffi.EnableRecoveryProgressListener = genericListener
	recoveryKey, err := c.FFIClient.Encryption().EnableRecovery(true, listener)
	for s := range genericListener.ch {
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
	}
	must.NotError(t, "Encryption.EnableRecovery", err)
	return recoveryKey
}

func (c *RustClient) MustLoadBackup(t *testing.T, recoveryKey string) {
	t.Helper()
	must.NotError(t, "Recover", c.FFIClient.Encryption().Recover(recoveryKey))
}

func (c *RustClient) WaitUntilEventInRoom(t *testing.T, roomID string, checker func(api.Event) bool) api.Waiter {
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
func (c *RustClient) SendMessage(t *testing.T, roomID, text string) (eventID string) {
	t.Helper()
	eventID, err := c.TrySendMessage(t, roomID, text)
	if err != nil {
		api.Fatalf(t, err.Error())
	}
	return eventID
}

func (c *RustClient) TrySendMessage(t *testing.T, roomID, text string) (eventID string, err error) {
	t.Helper()
	ch := make(chan bool)
	// we need a timeline listener before we can send messages, AND that listener must be attached to the
	// same *Room you call .Send on :S
	r := c.ensureListening(t, roomID)
	cancel := c.listenForUpdates(func(roomID string) {
		info := c.rooms[roomID]
		if info == nil {
			return
		}
		for _, ev := range info.timeline {
			if ev == nil {
				continue
			}
			if ev.Text == text && ev.ID != "" {
				eventID = ev.ID
				close(ch)
			}
		}
	})
	defer cancel()
	r.Timeline().Send(matrix_sdk_ffi.MessageEventContentFromHtml(text, text))
	select {
	case <-time.After(11 * time.Second):
		err = fmt.Errorf("SendMessage(rust) %s: timed out after 11s", c.userID)
		return
	case <-ch:
		return
	}
}

func (c *RustClient) MustBackpaginate(t *testing.T, roomID string, count int) {
	t.Helper()
	r := c.findRoom(t, roomID)
	must.NotEqual(t, r, nil, "unknown room")
	must.NotError(t, "failed to backpaginate", r.Timeline().PaginateBackwards(matrix_sdk_ffi.PaginationOptionsSimpleRequest{
		EventLimit: uint16(count),
	}))
}

func (c *RustClient) UserID() string {
	return c.userID
}

func (c *RustClient) findRoomInMap(roomID string) *matrix_sdk_ffi.Room {
	c.roomsMu.RLock()
	defer c.roomsMu.RUnlock()
	// do we have a reference to it already?
	roomInfo := c.rooms[roomID]
	if roomInfo != nil {
		return roomInfo.room
	}
	return nil
}

// findRoom returns the room, waiting up to 5s for it to appear
func (c *RustClient) findRoom(t *testing.T, roomID string) *matrix_sdk_ffi.Room {
	t.Helper()
	room := c.findRoomInMap(roomID)
	if room != nil {
		return room
	}
	// try to find it in all_rooms
	if c.allRooms != nil {
		roomListItem, err := c.allRooms.Room(roomID)
		if err != nil {
			c.Logf(t, "allRooms.Room(%s) err: %s", roomID, err)
		} else if roomListItem != nil {
			room := roomListItem.FullRoom()
			c.roomsMu.Lock()
			c.rooms[roomID] = &RustRoomInfo{
				room: room,
			}
			c.roomsMu.Unlock()
			return room
		}
	}
	// try to find it from cache?
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
	return nil
}

func (c *RustClient) Logf(t *testing.T, format string, args ...interface{}) {
	t.Helper()
	matrix_sdk_ffi.LogEvent("rust.go", &zero, matrix_sdk_ffi.LogLevelInfo, t.Name(), fmt.Sprintf(format, args...))
	t.Logf(format, args...)
}

func (c *RustClient) ensureListening(t *testing.T, roomID string) *matrix_sdk_ffi.Room {
	t.Helper()
	r := c.findRoom(t, roomID)
	must.NotEqual(t, r, nil, fmt.Sprintf("room %s does not exist", roomID))

	info := c.rooms[roomID]
	if info.stream != nil {
		return r
	}

	c.Logf(t, "[%s]AddTimelineListener[%s]", c.userID, roomID)
	// we need a timeline listener before we can send messages
	result := r.Timeline().AddListener(&timelineListener{fn: func(diff []*matrix_sdk_ffi.TimelineDiff) {
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
					continue
				}
				timeline = slices.Insert(timeline, i, timelineItemToEvent(insertData.Item))
				fmt.Printf("[%s]_______ INSERT %+v\n", c.userID, timeline[i])
				newEvents = append(newEvents, timeline[i])
			case matrix_sdk_ffi.TimelineChangeAppend:
				appendItems := d.Append()
				if appendItems == nil {
					continue
				}
				for _, item := range *appendItems {
					ev := timelineItemToEvent(item)
					timeline = append(timeline, ev)
					fmt.Printf("[%s]_______ APPEND %+v\n", c.userID, ev)
					newEvents = append(newEvents, ev)
				}
			case matrix_sdk_ffi.TimelineChangePushBack: // append but 1 element
				pbData := d.PushBack()
				if pbData == nil {
					continue
				}
				ev := timelineItemToEvent(*pbData)
				timeline = append(timeline, ev)
				fmt.Printf("[%s]_______ PUSH BACK %+v\n", c.userID, ev)
				newEvents = append(newEvents, ev)
			case matrix_sdk_ffi.TimelineChangeSet:
				setData := d.Set()
				if setData == nil {
					continue
				}
				i := int(setData.Index)
				if i >= len(timeline) {
					t.Logf("TimelineListener[%s] SET %d out of bounds of events timeline of size %d", roomID, i, len(timeline))
					continue
				}
				timeline[i] = timelineItemToEvent(setData.Item)
				fmt.Printf("[%s]_______ SET %+v\n", c.userID, timeline[i])
				newEvents = append(newEvents, timeline[i])
			default:
				t.Logf("Unhandled TimelineDiff change %v", d.Change())
			}
		}
		c.rooms[roomID].timeline = timeline
		for _, l := range c.listeners {
			l(roomID)
		}
		for _, e := range newEvents {
			c.Logf(t, "TimelineDiff change: %+v", e)
		}
	}})
	events := make([]*api.Event, len(result.Items))
	for i := range result.Items {
		events[i] = timelineItemToEvent(result.Items[i])
	}
	c.rooms[roomID].stream = result.ItemsStream
	c.rooms[roomID].timeline = events
	c.Logf(t, "[%s]AddTimelineListener[%s] result.Items len=%d", c.userID, roomID, len(result.Items))
	if len(events) > 0 {
		for _, l := range c.listeners {
			l(roomID)
		}
	}
	return r
}

func (c *RustClient) listenForUpdates(callback func(roomID string)) (cancel func()) {
	id := c.listenerID.Add(1)
	c.listeners[id] = callback
	return func() {
		delete(c.listeners, id)
	}
}

type timelineWaiter struct {
	roomID  string
	checker func(e api.Event) bool
	client  *RustClient
}

func (w *timelineWaiter) Wait(t *testing.T, s time.Duration) {
	t.Helper()

	checkForEvent := func() bool {
		t.Helper()
		// check if it exists in the timeline already
		info := w.client.rooms[w.roomID]
		if info == nil {
			fmt.Printf("_____checkForEvent[%s] room does not exist\n", w.client.userID)
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
		fmt.Printf("_____checkForEvent[%s] checked %d timeline events and no match \n", w.client.userID, len(info.timeline))
		return false
	}

	if checkForEvent() {
		return
	}

	updates := make(chan bool, 3)
	cancel := w.client.listenForUpdates(func(roomID string) {
		if w.roomID != roomID {
			return
		}
		if !checkForEvent() {
			return
		}
		close(updates)
	})
	defer cancel()

	// either no timeline or doesn't exist yet, start blocking
	start := time.Now()
	for {
		timeLeft := s - time.Since(start)
		if timeLeft <= 0 {
			api.Fatalf(t, "%s (rust): Wait[%s]: timed out", w.client.userID, w.roomID)
		}
		select {
		case <-time.After(timeLeft):
			api.Fatalf(t, "%s (rust): Wait[%s]: timed out", w.client.userID, w.roomID)
		case <-updates:
			return
		}
	}
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

type genericStateListener[T any] struct {
	ch       chan T
	isClosed bool
}

func newGenericStateListener[T any]() *genericStateListener[T] {
	return &genericStateListener[T]{
		ch: make(chan T),
	}
}

func (l *genericStateListener[T]) Close() {
	l.isClosed = true
	close(l.ch)
}

func (l *genericStateListener[T]) OnUpdate(state T) {
	if l.isClosed {
		return
	}
	l.ch <- state
}
