package api

import (
	"fmt"
	"time"

	"github.com/matrix-org/complement/client"
)

type ClientTypeLang string

var (
	ClientTypeRust ClientTypeLang = "rust"
	ClientTypeJS   ClientTypeLang = "js"
)

type ClientType struct {
	Lang ClientTypeLang // rust or js
	HS   string         // hs1 or hs2
}

// Client represents a generic crypto client.
// It is an abstraction to allow tests to interact with JS and FFI bindings in an agnostic way.
type Client interface {
	// Close is called to clean up resources.
	// Specifically, we need to shut off existing browsers and any FFI bindings.
	// If we get callbacks/events after this point, tests may panic if the callbacks
	// log messages.
	Close(t Test)
	// Remove any persistent storage, if it was enabled.
	DeletePersistentStorage(t Test)
	Login(t Test, opts ClientCreationOpts) error
	// StartSyncing to begin syncing from sync v2 / sliding sync.
	// Tests should call stopSyncing() at the end of the test.
	// MUST BLOCK until the initial sync is complete.
	StartSyncing(t Test) (stopSyncing func())
	// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
	// provide a bogus room ID.
	IsRoomEncrypted(t Test, roomID string) (bool, error)
	// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
	// room. Returns the event ID of the sent event, so MUST BLOCK until the event has been sent.
	SendMessage(t Test, roomID, text string) (eventID string)
	// TrySendMessage tries to send the message, but can fail.
	TrySendMessage(t Test, roomID, text string) (eventID string, err error)
	// Wait until an event with the given body is seen. Not all impls expose event IDs
	// hence needing to use body as a proxy.
	WaitUntilEventInRoom(t Test, roomID string, checker func(e Event) bool) Waiter
	// Backpaginate in this room by `count` events.
	MustBackpaginate(t Test, roomID string, count int)
	// MustGetEvent will return the client's view of this event, or fail the test if the event cannot be found.
	MustGetEvent(t Test, roomID, eventID string) Event
	// MustBackupKeys will backup E2EE keys, else fail the test.
	MustBackupKeys(t Test) (recoveryKey string)
	// MustLoadBackup will recover E2EE keys from the latest backup, else fail the test.
	MustLoadBackup(t Test, recoveryKey string)
	// Log something to stdout and the underlying client log file
	Logf(t Test, format string, args ...interface{})
	// The user for this client
	UserID() string
	Type() ClientTypeLang
}

type LoggedClient struct {
	Client
}

func (c *LoggedClient) Login(t Test, opts ClientCreationOpts) error {
	t.Helper()
	c.Logf(t, "%s Login %+v", c.logPrefix(), opts)
	return c.Client.Login(t, opts)
}

func (c *LoggedClient) Close(t Test) {
	t.Helper()
	c.Logf(t, "%s Close", c.logPrefix())
	c.Client.Close(t)
}

func (c *LoggedClient) StartSyncing(t Test) (stopSyncing func()) {
	t.Helper()
	c.Logf(t, "%s StartSyncing starting to sync", c.logPrefix())
	stopSyncing = c.Client.StartSyncing(t)
	c.Logf(t, "%s StartSyncing now syncing", c.logPrefix())
	return
}

func (c *LoggedClient) IsRoomEncrypted(t Test, roomID string) (bool, error) {
	t.Helper()
	c.Logf(t, "%s IsRoomEncrypted %s", c.logPrefix(), roomID)
	return c.Client.IsRoomEncrypted(t, roomID)
}

func (c *LoggedClient) TrySendMessage(t Test, roomID, text string) (eventID string, err error) {
	t.Helper()
	c.Logf(t, "%s TrySendMessage %s => %s", c.logPrefix(), roomID, text)
	eventID, err = c.Client.TrySendMessage(t, roomID, text)
	c.Logf(t, "%s TrySendMessage %s => %s", c.logPrefix(), roomID, eventID)
	return
}

func (c *LoggedClient) SendMessage(t Test, roomID, text string) (eventID string) {
	t.Helper()
	c.Logf(t, "%s SendMessage %s => %s", c.logPrefix(), roomID, text)
	eventID = c.Client.SendMessage(t, roomID, text)
	c.Logf(t, "%s SendMessage %s => %s", c.logPrefix(), roomID, eventID)
	return
}

func (c *LoggedClient) WaitUntilEventInRoom(t Test, roomID string, checker func(e Event) bool) Waiter {
	t.Helper()
	c.Logf(t, "%s WaitUntilEventInRoom %s", c.logPrefix(), roomID)
	return c.Client.WaitUntilEventInRoom(t, roomID, checker)
}

func (c *LoggedClient) MustBackpaginate(t Test, roomID string, count int) {
	t.Helper()
	c.Logf(t, "%s MustBackpaginate %d %s", c.logPrefix(), count, roomID)
	c.Client.MustBackpaginate(t, roomID, count)
}

func (c *LoggedClient) MustBackupKeys(t Test) (recoveryKey string) {
	t.Helper()
	c.Logf(t, "%s MustBackupKeys", c.logPrefix())
	recoveryKey = c.Client.MustBackupKeys(t)
	c.Logf(t, "%s MustBackupKeys => %s", c.logPrefix(), recoveryKey)
	return recoveryKey
}

func (c *LoggedClient) MustLoadBackup(t Test, recoveryKey string) {
	t.Helper()
	c.Logf(t, "%s MustLoadBackup key=%s", c.logPrefix(), recoveryKey)
	c.Client.MustLoadBackup(t, recoveryKey)
}

func (c *LoggedClient) DeletePersistentStorage(t Test) {
	t.Helper()
	c.Logf(t, "%s DeletePersistentStorage", c.logPrefix())
	c.Client.DeletePersistentStorage(t)
}

func (c *LoggedClient) logPrefix() string {
	return fmt.Sprintf("[%s](%s)", c.UserID(), c.Type())
}

// ClientCreationOpts are generic opts to use when creating crypto clients.
// Because this is generic, some features possible in some clients are unsupported here, notably
// you cannot provide an existing access_token to the FFI binding layer, hence you don't see
// an AccessToken field here.
type ClientCreationOpts struct {
	// Required. The base URL of the homeserver.
	BaseURL string
	// Required. The user to login as.
	UserID string
	// Required. The password for this account.
	Password string

	// Optional. If true, persistent storage will be used for the same user|device ID.
	PersistentStorage bool

	// Optional. Set this to login with this device ID.
	DeviceID string
}

func FromComplementClient(c *client.CSAPI, password string) ClientCreationOpts {
	return ClientCreationOpts{
		BaseURL:  c.BaseURL,
		UserID:   c.UserID,
		Password: password,
		DeviceID: c.DeviceID,
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
	Wait(t Test, s time.Duration)
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

const ansiRedForeground = "\x1b[31m"
const ansiResetForeground = "\x1b[39m"

// Errorf is a wrapper around t.Errorf which prints the failing error message in red.
func Errorf(t Test, format string, args ...any) {
	t.Helper()
	format = ansiRedForeground + format + ansiResetForeground
	t.Errorf(format, args...)
}

// Fatalf is a wrapper around t.Fatalf which prints the failing error message in red.
func Fatalf(t Test, format string, args ...any) {
	t.Helper()
	format = ansiRedForeground + format + ansiResetForeground
	t.Fatalf(format, args...)
}

type Test interface {
	Logf(f string, args ...any)
	Errorf(f string, args ...any)
	Fatalf(f string, args ...any)
	Helper()
	Name() string
}

// TODO move to must package when it accepts an interface

// NotError will ensure `err` is nil else terminate the test with `msg`.
func MustNotError(t Test, msg string, err error) {
	t.Helper()
	if err != nil {
		Fatalf(t, "must.NotError: %s -> %s", msg, err)
	}
}

// NotEqual ensures that got!=want else logs an error.
// The 'msg' is displayed with the error to provide extra context.
func MustNotEqual[V comparable](t Test, got, want V, msg string) {
	t.Helper()
	if got == want {
		Errorf(t, "NotEqual %s: got '%v', want '%v'", msg, got, want)
	}
}
