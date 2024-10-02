package api

import (
	"fmt"
	"time"

	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/ct"
)

type ClientType struct {
	Lang ClientTypeLang // rust or js
	HS   string         // hs1 or hs2
}

// Client represents a generic crypto client.
// It is an abstraction to allow tests to interact with JS and FFI bindings in an agnostic way.
// Clients are not limited to this interface, and can test functionality specific to their client
// by type casting at runtime.
type Client interface {
	// Close is called to clean up resources.
	// Specifically, we need to shut off existing browsers and any FFI bindings.
	// If we get callbacks/events after this point, tests may panic if the callbacks
	// log messages.
	Close(t ct.TestLike)
	// ForceClose should uncleanly shut down the client e.g
	// sending SIGKILL. This is typically useful for tests which want to explicitly test
	// unclean shutdowns.
	ForceClose(t ct.TestLike)
	// Remove any persistent storage, if it was enabled.
	DeletePersistentStorage(t ct.TestLike)
	// Login the given user. This function MUST block until one-time keys and device keys have been
	// uploaded to the server. Failure to block will result in flakey tests as other users may not
	// encrypt for this Client due to not detecting keys for the Client.
	Login(t ct.TestLike, opts ClientCreationOpts) error
	// StartSyncing to begin syncing from sync v2 / sliding sync.
	// Tests should call stopSyncing() at the end of the test.
	// MUST BLOCK until the initial sync is complete.
	// Returns an error if there was a problem syncing.
	StartSyncing(t ct.TestLike) (stopSyncing func(), err error)
	// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
	// provide a bogus room ID.
	IsRoomEncrypted(t ct.TestLike, roomID string) (bool, error)
	// InviteUser attempts to invite the given user into the given room.
	InviteUser(t ct.TestLike, roomID, userID string) error
	// SendMessage sends the given text as an encrypted/unencrypted message in the room, depending
	// if the room is encrypted or not. Returns the event ID of the sent event, so MUST BLOCK until the event has been sent.
	// If the event cannot be sent, returns an error.
	SendMessage(t ct.TestLike, roomID, text string) (eventID string, err error)
	// Wait until an event is seen in the given room. The checker functions can be custom or you can use
	// a pre-defined one like api.CheckEventHasMembership, api.CheckEventHasBody, or api.CheckEventHasEventID.
	WaitUntilEventInRoom(t ct.TestLike, roomID string, checker func(e Event) bool) Waiter
	// Backpaginate in this room by `count` events. Returns an error if there was a problem backpaginating.
	// Getting to the beginning of the room is not an error condition.
	Backpaginate(t ct.TestLike, roomID string, count int) error
	// GetEvent will return the client's view of this event, or returns an error if the event cannot be found.
	GetEvent(t ct.TestLike, roomID, eventID string) (*Event, error)
	// BackupKeys will backup E2EE keys, else return an error.
	BackupKeys(t ct.TestLike) (recoveryKey string, err error)
	// LoadBackup will recover E2EE keys from the latest backup, else return an error.
	LoadBackup(t ct.TestLike, recoveryKey string) error
	// GetNotification gets push notification-like information for the given event. If there is a problem, an error is returned.
	// Clients should implement this AS IF they received a push notification.
	GetNotification(t ct.TestLike, roomID, eventID string) (*Notification, error)
	// ListenForVerificationRequests will listen for incoming verification requests.
	// See RequestOwnUserVerification for information on the stages.
	ListenForVerificationRequests(t ct.TestLike) chan VerificationStage
	// RequestOwnUserVerification tries to verify this device with another logged in device.
	//
	// Returns a stream of verification stages. Callers should listen on this stream
	// (with appropriate timeouts if no change has been seen) and then type switch to
	// determine what the current stage is. The type switched interface will contain only
	// the valid state transitions for that stage. E.g:
	//    for stage := range client.RequestOwnUserVerification(t) {
	//        switch stg := stage.(type) {
	//            case api.VerificationStageReady:
	//               // ...
	//        }
	//    }
	// The channel is closed when the verification process reaches a terminal state.
	RequestOwnUserVerification(t ct.TestLike) chan VerificationStage
	// Log something to stdout and the underlying client log file
	Logf(t ct.TestLike, format string, args ...interface{})
	// The user for this client
	UserID() string
	// The current access token for this client
	CurrentAccessToken(t ct.TestLike) string
	Type() ClientTypeLang
	Opts() ClientCreationOpts
}

// TestClient is a Client with extra helper functions added to make writing tests easier.
// Client implementations are not expected to implement these helper functions, and are
// instead composed together by the test rig itself.
type TestClient interface {
	Client
	// MustStartSyncing is StartSyncing but fails the test on error.
	MustStartSyncing(t ct.TestLike) (stopSyncing func())
	// MustLoadBackup is LoadBackup but fails the test on error.
	MustLoadBackup(t ct.TestLike, recoveryKey string)
	// MustSendMessage is SendMessage but fails the test on error.
	MustSendMessage(t ct.TestLike, roomID, text string) (eventID string)
	// MustGetEvent is GetEvent but fails the test on error.
	MustGetEvent(t ct.TestLike, roomID, eventID string) *Event
	// MustBackupKeys is BackupKeys but fails the test on error.
	MustBackupKeys(t ct.TestLike) (recoveryKey string)
	// MustBackpaginate is Backpaginate but fails the test on error.
	MustBackpaginate(t ct.TestLike, roomID string, count int)
}

// NewTestClient wraps a Client implementation with helper functions which tests can use.
func NewTestClient(c Client) TestClient {
	return &testClientImpl{
		Client: c,
	}
}

type testClientImpl struct {
	Client
}

func (c *testClientImpl) MustStartSyncing(t ct.TestLike) (stopSyncing func()) {
	t.Helper()
	stopSyncing, err := c.StartSyncing(t)
	if err != nil {
		ct.Fatalf(t, "MustStartSyncing: %s", err)
	}
	return stopSyncing
}

func (c *testClientImpl) MustLoadBackup(t ct.TestLike, recoveryKey string) {
	t.Helper()
	err := c.LoadBackup(t, recoveryKey)
	if err != nil {
		ct.Fatalf(t, "MustLoadBackup: %s", err)
	}
}

func (c *testClientImpl) MustBackupKeys(t ct.TestLike) (recoveryKey string) {
	t.Helper()
	recoveryKey, err := c.BackupKeys(t)
	if err != nil {
		ct.Fatalf(t, "MustBackupKeys: %s", err)
	}
	return recoveryKey
}

func (c *testClientImpl) MustBackpaginate(t ct.TestLike, roomID string, count int) {
	t.Helper()
	err := c.Backpaginate(t, roomID, count)
	if err != nil {
		ct.Fatalf(t, "MustBackpaginate: %s", err)
	}
}

func (c *testClientImpl) MustSendMessage(t ct.TestLike, roomID, text string) (eventID string) {
	t.Helper()
	eventID, err := c.SendMessage(t, roomID, text)
	if err != nil {
		ct.Fatalf(t, "MustSendMessage: %s", err)
	}
	return eventID
}

func (c *testClientImpl) MustGetEvent(t ct.TestLike, roomID, eventID string) *Event {
	t.Helper()
	ev, err := c.GetEvent(t, roomID, eventID)
	if err != nil {
		ct.Fatalf(t, "MustGetEvent: %s", err)
	}
	return ev
}

type LoggedClient struct {
	Client
}

func (c *LoggedClient) CurrentAccessToken(t ct.TestLike) string {
	t.Helper()
	token := c.Client.CurrentAccessToken(t)
	c.Logf(t, "%s CurrentAccessToken => %s", c.logPrefix(), token)
	return token
}

func (c *LoggedClient) Login(t ct.TestLike, opts ClientCreationOpts) error {
	t.Helper()
	c.Logf(t, "%s Login %+v", c.logPrefix(), opts)
	return c.Client.Login(t, opts)
}

func (c *LoggedClient) Close(t ct.TestLike) {
	t.Helper()
	c.Logf(t, "%s Close", c.logPrefix())
	c.Client.Close(t)
}

func (c *LoggedClient) ForceClose(t ct.TestLike) {
	t.Helper()
	c.Logf(t, "%s ForceClose", c.logPrefix())
	c.Client.ForceClose(t)
}

func (c *LoggedClient) GetEvent(t ct.TestLike, roomID, eventID string) (*Event, error) {
	t.Helper()
	c.Logf(t, "%s GetEvent(%s, %s)", c.logPrefix(), roomID, eventID)
	return c.Client.GetEvent(t, roomID, eventID)
}

func (c *LoggedClient) StartSyncing(t ct.TestLike) (stopSyncing func(), err error) {
	t.Helper()
	c.Logf(t, "%s StartSyncing starting to sync", c.logPrefix())
	stopSyncing, err = c.Client.StartSyncing(t)
	c.Logf(t, "%s StartSyncing now syncing", c.logPrefix())
	return
}

func (c *LoggedClient) IsRoomEncrypted(t ct.TestLike, roomID string) (bool, error) {
	t.Helper()
	c.Logf(t, "%s IsRoomEncrypted %s", c.logPrefix(), roomID)
	return c.Client.IsRoomEncrypted(t, roomID)
}

func (c *LoggedClient) SendMessage(t ct.TestLike, roomID, text string) (eventID string, err error) {
	t.Helper()
	c.Logf(t, "%s SendMessage %s => %s", c.logPrefix(), roomID, text)
	eventID, err = c.Client.SendMessage(t, roomID, text)
	c.Logf(t, "%s SendMessage %s => %s %s", c.logPrefix(), roomID, eventID, err)
	return
}

func (c *LoggedClient) WaitUntilEventInRoom(t ct.TestLike, roomID string, checker func(e Event) bool) Waiter {
	t.Helper()
	c.Logf(t, "%s WaitUntilEventInRoom %s", c.logPrefix(), roomID)
	return c.Client.WaitUntilEventInRoom(t, roomID, checker)
}

func (c *LoggedClient) Backpaginate(t ct.TestLike, roomID string, count int) error {
	t.Helper()
	c.Logf(t, "%s Backpaginate %d %s", c.logPrefix(), count, roomID)
	err := c.Client.Backpaginate(t, roomID, count)
	c.Logf(t, "%s Backpaginate %d %s => %s", c.logPrefix(), count, roomID, err)
	return err
}

func (c *LoggedClient) BackupKeys(t ct.TestLike) (recoveryKey string, err error) {
	t.Helper()
	c.Logf(t, "%s BackupKeys", c.logPrefix())
	recoveryKey, err = c.Client.BackupKeys(t)
	c.Logf(t, "%s BackupKeys => %s %s", c.logPrefix(), recoveryKey, err)
	return recoveryKey, err
}

func (c *LoggedClient) LoadBackup(t ct.TestLike, recoveryKey string) error {
	t.Helper()
	c.Logf(t, "%s LoadBackup key=%s", c.logPrefix(), recoveryKey)
	return c.Client.LoadBackup(t, recoveryKey)
}

func (c *LoggedClient) DeletePersistentStorage(t ct.TestLike) {
	t.Helper()
	c.Logf(t, "%s DeletePersistentStorage", c.logPrefix())
	c.Client.DeletePersistentStorage(t)
}

func (c *LoggedClient) logPrefix() string {
	return fmt.Sprintf("[%s](%s)", c.UserID(), c.Type())
}

type Notification struct {
	Event
	HasMentions *bool
}

// ClientCreationOpts are options to use when creating crypto clients.
//
// This contains a mixture of generic options which can be used across any client, and specific
// options which are only supported in some clients. These are clearly documented.
type ClientCreationOpts struct {
	// Required. The base URL of the homeserver.
	BaseURL string
	// Required. The user to login as.
	UserID string
	// Required. The password for this account.
	Password string

	// Required for rust clients. The URL of the sliding sync proxy.
	SlidingSyncURL string
	// Optional. Set this to login with this device ID.
	DeviceID string

	// A hint to the client implementation that persistent storage is required. Clients may ignore
	// this flag and always use persistence.
	PersistentStorage bool

	// A map containing any client-specific creation options, for use for client-specific tests.
	// Any options in this map MUST BE SERIALISABLE as they may be sent over RPC boundaries.
	ExtraOpts map[string]any

	// Rust only. If set with EnableCrossProcessRefreshLockProcessName=ProcessNameNSE, the client will be seeded
	// with a logged in session.
	AccessToken string
}

// GetExtraOption is a safe way to get an extra option from ExtraOpts, with a default value if the key does not exist.
func (o *ClientCreationOpts) GetExtraOption(key string, defaultValue any) any {
	if o.ExtraOpts == nil {
		return defaultValue
	}
	val, ok := o.ExtraOpts[key]
	if !ok {
		return defaultValue
	}
	return val
}

func NewClientCreationOpts(c *client.CSAPI) ClientCreationOpts {
	return ClientCreationOpts{
		BaseURL:  c.BaseURL,
		UserID:   c.UserID,
		Password: c.Password,
		DeviceID: c.DeviceID,
	}
}

// Combine the other opts into this set of opts.
func (o *ClientCreationOpts) Combine(other *ClientCreationOpts) {
	if other.AccessToken != "" {
		o.AccessToken = other.AccessToken
	}
	if other.BaseURL != "" {
		o.BaseURL = other.BaseURL
	}
	if other.DeviceID != "" {
		o.DeviceID = other.DeviceID
	}
	if other.ExtraOpts != nil {
		if o.ExtraOpts == nil {
			o.ExtraOpts = make(map[string]any)
		}
		for k, v := range other.ExtraOpts {
			o.ExtraOpts[k] = v
		}
	}
	if other.Password != "" {
		o.Password = other.Password
	}
	if other.PersistentStorage {
		o.PersistentStorage = true
	}
	if other.SlidingSyncURL != "" {
		o.SlidingSyncURL = other.SlidingSyncURL
	}
	if other.UserID != "" {
		o.UserID = other.UserID
	}
}

type Event struct {
	ID     string
	Text   string // FFI bindings don't expose the content object
	Sender string
	// FFI bindings don't expose state key
	Target string
	// FFI bindings don't expose type
	Membership      string
	FailedToDecrypt bool
}

type Waiter interface {
	// Wait for something to happen, up until the timeout s. If nothing happens,
	// fail the test with the formatted string provided.
	Waitf(t ct.TestLike, s time.Duration, format string, args ...any)
	// Wait for something to happen, up until the timeout s. If nothing happens,
	// return an error with the formatted string provided.
	TryWaitf(t ct.TestLike, s time.Duration, format string, args ...any) error
}

func CheckEventHasBody(body string) func(e Event) bool {
	return func(e Event) bool {
		return e.Text == body
	}
}

func CheckEventHasMembership(target, membership string) func(e Event) bool {
	return func(e Event) bool {
		return e.Membership == membership && e.Target == target
	}
}

func CheckEventHasEventID(eventID string) func(e Event) bool {
	return func(e Event) bool {
		return e.ID == eventID
	}
}
