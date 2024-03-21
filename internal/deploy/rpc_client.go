package deploy

import (
	"fmt"
	"net/rpc"
	"time"

	"github.com/matrix-org/complement-crypto/internal/api"
	"github.com/matrix-org/complement/ct"
)

// RPCLanguageBindings implements api.LanguageBindings and instead issues RPC calls to a remote server.
// All fields must be serialisable with encoding/gob.
type RPCLanguageBindings struct {
	client     *rpc.Client
	clientType api.ClientTypeLang
}

func NewRPCLanguageBindings(address string, clientType api.ClientTypeLang) (*RPCLanguageBindings, error) {
	client, err := rpc.DialHTTP("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("NewRPCLanguageBindings: DialHTTP: %v", err)
	}
	return &RPCLanguageBindings{
		client:     client,
		clientType: clientType,
	}, nil
}

func (r *RPCLanguageBindings) PreTestRun() {
	var void int
	err := r.client.Call("RPCServer.PreTestRun", r.clientType, &void)
	if err != nil {
		panic("RPCLanguageBindings.PreTestRun: " + err.Error())
	}
}
func (r *RPCLanguageBindings) PostTestRun() {
	var void int
	err := r.client.Call("RPCServer.PostTestRun", r.clientType, &void)
	if err != nil {
		panic("RPCLanguageBindings.PostTestRun: " + err.Error())
	}
}
func (r *RPCLanguageBindings) MustCreateClient(t ct.TestLike, cfg api.ClientCreationOpts) api.Client {
	var void int
	r.client.Call("RPCServer.MustCreateClient", RPCClientCreationOpts{
		ClientCreationOpts: cfg,
		Lang:               r.clientType,
	}, &void)

	return &RPCClient{
		client: r.client,
		lang:   r.clientType,
	}
}

// RPCClient implements api.Client by making RPC calls to an RPC server, which actually has a concrete api.Client
type RPCClient struct {
	client *rpc.Client
	lang   api.ClientTypeLang
}

// Close is called to clean up resources.
// Specifically, we need to shut off existing browsers and any FFI bindings.
// If we get callbacks/events after this point, tests may panic if the callbacks
// log messages.
func (c *RPCClient) Close(t ct.TestLike) {
	var void int
	err := c.client.Call("RPCServer.Close", t.Name(), &void)
	if err != nil {
		t.Fatalf("RPCClient.Close: %s", err)
	}
}

// Remove any persistent storage, if it was enabled.
func (c *RPCClient) DeletePersistentStorage(t ct.TestLike) {
	var void int
	err := c.client.Call("RPCServer.DeletePersistentStorage", t.Name(), &void)
	if err != nil {
		t.Fatalf("RPCClient.DeletePersistentStorage: %s", err)
	}
}
func (c *RPCClient) Login(t ct.TestLike, opts api.ClientCreationOpts) error {
	var void int
	return c.client.Call("RPCServer.Login", RPCClientCreationOpts{
		ClientCreationOpts: opts,
		Lang:               c.lang,
	}, &void)
}

// MustStartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
// MUST BLOCK until the initial sync is complete.
// Fails the test if there was a problem syncing.
func (c *RPCClient) MustStartSyncing(t ct.TestLike) (stopSyncing func()) {
	var void int
	err := c.client.Call("RPCServer.MustStartSyncing", t.Name(), &void)
	if err != nil {
		t.Fatalf("RPCClient.MustStartSyncing: %s", err)
	}
	return func() {
		err := c.client.Call("RPCServer.StopSyncing", t.Name(), &void)
		if err != nil {
			t.Fatalf("RPCClient.StopSyncing: %s", err)
		}
	}
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
// MUST BLOCK until the initial sync is complete.
// Returns an error if there was a problem syncing.
func (c *RPCClient) StartSyncing(t ct.TestLike) (stopSyncing func(), err error) {
	var void int
	err = c.client.Call("RPCServer.StartSyncing", t.Name(), &void)
	if err != nil {
		return
	}
	return func() {
		err := c.client.Call("RPCServer.StopSyncing", t.Name(), &void)
		if err != nil {
			t.Logf("RPCClient.StopSyncing: %s", err)
		}
	}, nil
}

// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
// provide a bogus room ID.
func (c *RPCClient) IsRoomEncrypted(t ct.TestLike, roomID string) (bool, error) {
	var isEncrypted bool
	err := c.client.Call("RPCServer.IsRoomEncrypted", roomID, &isEncrypted)
	return isEncrypted, err
}

// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
// room. Returns the event ID of the sent event, so MUST BLOCK until the event has been sent.
func (c *RPCClient) SendMessage(t ct.TestLike, roomID, text string) (eventID string) {
	err := c.client.Call("RPCServer.SendMessage", RPCSendMessage{
		TestName: t.Name(),
		RoomID:   roomID,
		Text:     text,
	}, &eventID)
	if err != nil {
		t.Fatalf("RPCClient.SendMessage: %s", err)
	}
	return
}

// TrySendMessage tries to send the message, but can fail.
func (c *RPCClient) TrySendMessage(t ct.TestLike, roomID, text string) (eventID string, err error) {
	err = c.client.Call("RPCServer.TrySendMessage", RPCSendMessage{
		TestName: t.Name(),
		RoomID:   roomID,
		Text:     text,
	}, &eventID)
	return
}

// Wait until an event is seen in the given room. The checker functions can be custom or you can use
// a pre-defined one like api.CheckEventHasMembership, api.CheckEventHasBody, or api.CheckEventHasEventID.
func (c *RPCClient) WaitUntilEventInRoom(t ct.TestLike, roomID string, checker func(e api.Event) bool) api.Waiter {
	var waiterID int
	err := c.client.Call("RPCServer.WaitUntilEventInRoom", RPCWaitUntilEvent{
		TestName: t.Name(),
		RoomID:   roomID,
		// TODO WantEvent
	}, &waiterID)
	if err != nil {
		t.Fatalf("RPCClient.WaitUntilEventInRoom: %s", err)
	}
	return &RPCWaiter{
		client:   c.client,
		waiterID: waiterID,
	}
}

// Backpaginate in this room by `count` events.
func (c *RPCClient) MustBackpaginate(t ct.TestLike, roomID string, count int) {
	var void int
	err := c.client.Call("RPCServer.MustBackpaginate", RPCBackpaginate{
		TestName: t.Name(),
		RoomID:   roomID,
		Count:    count,
	}, &void)
	if err != nil {
		t.Fatalf("RPCClient.MustBackpaginate: %s", err)
	}
}

// MustGetEvent will return the client's view of this event, or fail the test if the event cannot be found.
func (c *RPCClient) MustGetEvent(t ct.TestLike, roomID, eventID string) api.Event {
	var ev api.Event
	err := c.client.Call("RPCServer.MustGetEvent", RPCGetEvent{
		TestName: t.Name(),
		RoomID:   roomID,
		EventID:  eventID,
	}, &ev)
	if err != nil {
		t.Fatalf("RPCClient.MustGetEvent: %s", err)
	}
	return ev
}

// MustBackupKeys will backup E2EE keys, else fail the test.
func (c *RPCClient) MustBackupKeys(t ct.TestLike) (recoveryKey string) {
	err := c.client.Call("RPCServer.MustBackupKeys", 0, &recoveryKey)
	if err != nil {
		t.Fatalf("RPCClient.MustBackupKeys: %v", err)
	}
	return
}

// MustLoadBackup will recover E2EE keys from the latest backup, else fail the test.
func (c *RPCClient) MustLoadBackup(t ct.TestLike, recoveryKey string) {
	var void int
	err := c.client.Call("RPCServer.MustLoadBackup", recoveryKey, &void)
	if err != nil {
		t.Fatalf("RPCClient.MustLoadBackup: %v", err)
	}
}

// LoadBackup will recover E2EE keys from the latest backup, else return an error.
func (c *RPCClient) LoadBackup(t ct.TestLike, recoveryKey string) error {
	var void int
	return c.client.Call("RPCServer.LoadBackup", recoveryKey, &void)
}

// Log something to stdout and the underlying client log file
func (c *RPCClient) Logf(t ct.TestLike, format string, args ...interface{}) {
	str := fmt.Sprintf(format, args...)
	str = t.Name() + ": " + str
	var void int
	err := c.client.Call("RPCServer.Logf", str, &void)
	if err != nil {
		t.Fatalf("RPCClient.Logf: %s", err)
	}
}

func (c *RPCClient) UserID() string {
	var userID string
	c.client.Call("RPCServer.UserID", 0, &userID)
	return userID
}
func (c *RPCClient) Type() api.ClientTypeLang {
	var lang api.ClientTypeLang
	c.client.Call("RPCServer.Type", 0, &lang)
	return lang
}
func (c *RPCClient) Opts() api.ClientCreationOpts {
	var opts api.ClientCreationOpts
	c.client.Call("RPCServer.Opts", 0, &opts)
	return opts
}

type RPCWaiter struct {
	waiterID int
	client   *rpc.Client
}

func (w *RPCWaiter) Wait(t ct.TestLike, s time.Duration) {
	var void int
	err := w.client.Call("RPCServer.Wait", RPCWait{
		TestName: t.Name(),
		WaiterID: w.waiterID,
		Timeout:  s,
	}, &void)
	if err != nil {
		t.Fatalf("RPCWaiter.Wait: %v", err)
	}
}
