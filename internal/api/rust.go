package api

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

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
	attachedListener bool
	room             *matrix_sdk_ffi.Room
	timeline         []*Event
}

type RustClient struct {
	FFIClient  *matrix_sdk_ffi.Client
	rooms      map[string]*RustRoomInfo
	listeners  map[int32]func(roomID string)
	listenerID atomic.Int32
	userID     string
}

func NewRustClient(t *testing.T, opts ClientCreationOpts, ssURL string) (Client, error) {
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
	}
	c.Logf(t, "NewRustClient[%s] created client", opts.UserID)
	return &LoggedClient{Client: c}, nil
}

func (c *RustClient) Close(t *testing.T) {
	t.Helper()
	c.FFIClient.Destroy()
}

func (c *RustClient) MustGetEvent(t *testing.T, roomID, eventID string) Event {
	return Event{}
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
func (c *RustClient) StartSyncing(t *testing.T) (stopSyncing func()) {
	t.Helper()
	syncService, err := c.FFIClient.SyncService().Finish()
	must.NotError(t, fmt.Sprintf("[%s]failed to make sync service", c.userID), err)
	ch := make(chan matrix_sdk_ffi.RoomListLoadingState, 10)
	roomList, err := syncService.RoomListService().AllRooms()
	must.NotError(t, "failed to call SyncService.RoomListService.AllRooms", err)
	result, err := roomList.LoadingState(&roomListLoadingStateListener{
		ch: ch,
	})
	must.NotError(t, "failed to call RoomList.LoadingState", err)
	go syncService.Start()

	isSyncing := false

	for !isSyncing {
		select {
		case <-time.After(5 * time.Second):
			fatalf(t, "[%s](rust) timed out after 5s StartSyncing", c.userID)
		case state := <-ch:
			fmt.Println(state)
			switch state.(type) {
			case matrix_sdk_ffi.RoomListLoadingStateLoaded:
				isSyncing = true
			case matrix_sdk_ffi.RoomListLoadingStateNotLoaded:
				isSyncing = false
			}
		}
	}

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
	r := c.findRoom(roomID)
	if r == nil {
		rooms := c.FFIClient.Rooms()
		return false, fmt.Errorf("failed to find room %s, got %d rooms", roomID, len(rooms))
	}
	return r.IsEncrypted()
}

func (c *RustClient) WaitUntilEventInRoom(t *testing.T, roomID string, checker func(Event) bool) Waiter {
	t.Helper()
	c.ensureListening(t, roomID)
	return &timelineWaiter{
		roomID:  roomID,
		checker: checker,
		client:  c,
	}
}

func (c *RustClient) Type() ClientType {
	return ClientTypeRust
}

// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
// room. Returns the event ID of the sent event.
func (c *RustClient) SendMessage(t *testing.T, roomID, text string) (eventID string) {
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
	r.Send(matrix_sdk_ffi.MessageEventContentFromHtml(text, text))
	select {
	case <-time.After(5 * time.Second):
		fatalf(t, "SendMessage: timed out after 5s")
	case <-ch:
		return
	}
	return
}

func (c *RustClient) MustBackpaginate(t *testing.T, roomID string, count int) {
	t.Helper()
	r := c.findRoom(roomID)
	must.NotEqual(t, r, nil, "unknown room")
	must.NotError(t, "failed to backpaginate", r.PaginateBackwards(matrix_sdk_ffi.PaginationOptionsSingleRequest{
		EventLimit: uint16(count),
	}))
}

func (c *RustClient) UserID() string {
	return c.userID
}

func (c *RustClient) findRoom(roomID string) *matrix_sdk_ffi.Room {
	rooms := c.FFIClient.Rooms()
	for i, r := range rooms {
		rid := r.Id()
		// ensure we only store rooms once
		_, exists := c.rooms[rid]
		if !exists {
			c.rooms[rid] = &RustRoomInfo{
				room: rooms[i],
			}
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
	r := c.findRoom(roomID)
	must.NotEqual(t, r, nil, fmt.Sprintf("room %s does not exist", roomID))

	info := c.rooms[roomID]
	if info.attachedListener {
		return r
	}

	t.Logf("[%s]AddTimelineListener[%s]", c.userID, roomID)
	// we need a timeline listener before we can send messages
	r.AddTimelineListener(&timelineListener{fn: func(diff []*matrix_sdk_ffi.TimelineDiff) {
		timeline := c.rooms[roomID].timeline
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
			case matrix_sdk_ffi.TimelineChangeAppend:
				appendItems := d.Append()
				if appendItems == nil {
					continue
				}
				for _, item := range *appendItems {
					ev := timelineItemToEvent(item)
					timeline = append(timeline, ev)
					fmt.Printf("[%s]_______ APPEND %+v\n", c.userID, ev)
				}
			case matrix_sdk_ffi.TimelineChangePushBack: // append but 1 element
				pbData := d.PushBack()
				if pbData == nil {
					continue
				}
				ev := timelineItemToEvent(*pbData)
				timeline = append(timeline, ev)
				fmt.Printf("[%s]_______ PUSH BACK %+v\n", c.userID, ev)
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
			default:
				t.Logf("Unhandled TimelineDiff change %v", d.Change())
			}
		}
		c.rooms[roomID].timeline = timeline
		for _, l := range c.listeners {
			l(roomID)
		}
	}})
	info.attachedListener = true
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
	checker func(e Event) bool
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

	updates := make(chan bool, 10)
	cancel := w.client.listenForUpdates(func(roomID string) {
		if w.roomID != roomID {
			return
		}
		updates <- true
	})
	defer cancel()

	// either no timeline or doesn't exist yet, start blocking
	start := time.Now()
	for {
		timeLeft := s - time.Since(start)
		if timeLeft <= 0 {
			fatalf(t, "%s: Wait[%s]: timed out", w.client.userID, w.roomID)
		}
		select {
		case <-time.After(timeLeft):
			fatalf(t, "%s: Wait[%s]: timed out", w.client.userID, w.roomID)
		case <-updates:
			if checkForEvent() {
				return
			}
		}
	}
}

type timelineListener struct {
	fn func(diff []*matrix_sdk_ffi.TimelineDiff)
}

func (l *timelineListener) OnUpdate(diff []*matrix_sdk_ffi.TimelineDiff) {
	l.fn(diff)
}

func timelineItemToEvent(item *matrix_sdk_ffi.TimelineItem) *Event {
	ev := item.AsEvent()
	if ev == nil { // e.g day divider
		return nil
	}
	evv := *ev
	if evv == nil {
		return nil
	}
	eventID := ""
	if evv.EventId() != nil {
		eventID = *evv.EventId()
	}
	complementEvent := Event{
		ID:     eventID,
		Sender: evv.Sender(),
	}
	switch k := evv.Content().Kind().(type) {
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
	}

	content := evv.Content()
	if content != nil {
		msg := content.AsMessage()
		if msg != nil {
			msgg := *msg
			complementEvent.Text = msgg.Body()
		}
	}
	return &complementEvent
}

type roomListLoadingStateListener struct {
	ch chan matrix_sdk_ffi.RoomListLoadingState
}

func (s *roomListLoadingStateListener) OnUpdate(state matrix_sdk_ffi.RoomListLoadingState) {
	s.ch <- state
}
