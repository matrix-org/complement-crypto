package api

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/rust/matrix_sdk_ffi"
	"github.com/matrix-org/complement/must"
)

func init() {
	matrix_sdk_ffi.SetupTracing(matrix_sdk_ffi.TracingConfiguration{
		WriteToStdoutOrSystem: true,
		Filter:                "debug",
	})
}

type RustRoomInfo struct {
	attachedListener bool
	room             *matrix_sdk_ffi.Room
	timeline         []*Event
}

type RustClient struct {
	FFIClient   *matrix_sdk_ffi.Client
	rooms       map[string]*RustRoomInfo
	listeners   map[int32]func(roomID string)
	listenerID  atomic.Int32
	userID      string
	syncService *matrix_sdk_ffi.SyncService
}

func NewRustClient(t *testing.T, opts ClientCreationOpts, ssURL string) (Client, error) {
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
	return &RustClient{
		userID:    opts.UserID,
		FFIClient: client,
		rooms:     make(map[string]*RustRoomInfo),
		listeners: make(map[int32]func(roomID string)),
	}, nil
}

func (c *RustClient) Close(t *testing.T) {
	c.FFIClient.Destroy()
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
func (c *RustClient) StartSyncing(t *testing.T) (stopSyncing func()) {
	syncService, err := c.FFIClient.SyncService().FinishBlocking()
	must.NotError(t, fmt.Sprintf("[%s]failed to make sync service", c.userID), err)
	c.syncService = syncService
	t.Logf("%s: Starting sync service", c.userID)
	go syncService.StartBlocking()
	return func() {
		t.Logf("%s: Stopping sync service", c.userID)
		syncService.StopBlocking()
	}
}

// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
// provide a bogus room ID.
func (c *RustClient) IsRoomEncrypted(t *testing.T, roomID string) (bool, error) {
	r := c.findRoom(roomID)
	if r == nil {
		rooms := c.FFIClient.Rooms()
		return false, fmt.Errorf("failed to find room %s, got %d rooms", roomID, len(rooms))
	}
	return r.IsEncrypted()
}

func (c *RustClient) WaitUntilEventInRoom(t *testing.T, roomID, wantBody string) Waiter {
	c.ensureListening(t, roomID)
	return &timelineWaiter{
		roomID:   roomID,
		wantBody: wantBody,
		client:   c,
	}
}

func (c *RustClient) Type() ClientType {
	return ClientTypeRust
}

// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
// room. Returns the event ID of the sent event.
func (c *RustClient) SendMessage(t *testing.T, roomID, text string) {
	// we need a timeline listener before we can send messages, AND that listener must be attached to the
	// same *Room you call .Send on :S
	r := c.ensureListening(t, roomID)
	t.Logf("%s: SendMessage[%s]: '%s'", c.userID, roomID, text)
	r.Send(matrix_sdk_ffi.MessageEventContentFromHtml(text, text))
}

func (c *RustClient) MustBackpaginate(t *testing.T, roomID string, count int) {
	t.Helper()
	t.Logf("[%s] MustBackpaginate %d %s", c.userID, count, roomID)
	r := c.findRoom(roomID)
	must.NotEqual(t, r, nil, "unknown room")
	must.NotError(t, "failed to backpaginate", r.PaginateBackwards(matrix_sdk_ffi.PaginationOptionsSingleRequest{
		EventLimit: uint16(count),
	}))
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

func (c *RustClient) ensureListening(t *testing.T, roomID string) *matrix_sdk_ffi.Room {
	r := c.findRoom(roomID)
	must.NotEqual(t, r, nil, fmt.Sprintf("room %s does not exist", roomID))

	info := c.rooms[roomID]
	if info.attachedListener {
		return r
	}

	t.Logf("[%s]AddTimelineListenerBlocking[%s]", c.userID, roomID)
	// we need a timeline listener before we can send messages
	r.AddTimelineListenerBlocking(&timelineListener{fn: func(diff []*matrix_sdk_ffi.TimelineDiff) {
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
				if timeline[i] != nil {
					// shift the item in this position right and insert this item
					timeline = append(timeline, nil)
					copy(timeline[i+1:], timeline[i:])
					timeline[i] = timelineItemToEvent(insertData.Item)
				} else {
					timeline[i] = timelineItemToEvent(insertData.Item)
				}
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
			case matrix_sdk_ffi.TimelineChangePushBack:
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
	roomID   string
	wantBody string
	client   *RustClient
}

func (w *timelineWaiter) Wait(t *testing.T, s time.Duration) {
	t.Helper()

	checkForEvent := func() bool {
		// check if it exists in the timeline already
		info := w.client.rooms[w.roomID]
		if info == nil {
			return false
		}
		for _, ev := range info.timeline {
			if ev == nil {
				continue
			}
			if ev.Text == w.wantBody {
				t.Logf("%s: Wait[%s]: event exists in the timeline", w.client.userID, w.roomID)
				return true
			}
		}
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
			t.Fatalf("%s: Wait[%s]: timed out", w.client.userID, w.roomID)
		}
		select {
		case <-time.After(timeLeft):
			t.Fatalf("%s: Wait[%s]: timed out", w.client.userID, w.roomID)
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
