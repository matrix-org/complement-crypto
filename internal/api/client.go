package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/matrix-org/complement/client"
)

type ClientType string

var (
	ClientTypeRust ClientType = "rust"
	ClientTypeJS   ClientType = "js"
)

// Client represents a generic crypto client.
// It is an abstraction to allow tests to interact with JS and FFI bindings in an agnostic way.
type Client interface {
	// Close is called to clean up resources.
	// Specifically, we need to shut off existing browsers and any FFI bindings.
	// If we get callbacks/events after this point, tests may panic if the callbacks
	// log messages.
	Close(t *testing.T)
	// StartSyncing to begin syncing from sync v2 / sliding sync.
	// Tests should call stopSyncing() at the end of the test.
	// MUST BLOCK until the initial sync is complete.
	StartSyncing(t *testing.T) (stopSyncing func())
	// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
	// provide a bogus room ID.
	IsRoomEncrypted(t *testing.T, roomID string) (bool, error)
	// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
	// room. Returns the event ID of the sent event, so MUST BLOCK until the event has been sent.
	SendMessage(t *testing.T, roomID, text string) (eventID string)
	// Wait until an event with the given body is seen. Not all impls expose event IDs
	// hence needing to use body as a proxy.
	WaitUntilEventInRoom(t *testing.T, roomID string, checker func(e Event) bool) Waiter
	// Backpaginate in this room by `count` events.
	MustBackpaginate(t *testing.T, roomID string, count int)
	// Log something to stdout and the underlying client log file
	Logf(t *testing.T, format string, args ...interface{})
	// The user for this client
	UserID() string
	Type() ClientType
}

type LoggedClient struct {
	Client
}

func (c *LoggedClient) Close(t *testing.T) {
	t.Helper()
	c.Logf(t, "%s Close", c.logPrefix())
	c.Client.Close(t)
}

func (c *LoggedClient) StartSyncing(t *testing.T) (stopSyncing func()) {
	t.Helper()
	c.Logf(t, "%s StartSyncing starting to sync", c.logPrefix())
	stopSyncing = c.Client.StartSyncing(t)
	c.Logf(t, "%s StartSyncing now syncing", c.logPrefix())
	return
}

func (c *LoggedClient) IsRoomEncrypted(t *testing.T, roomID string) (bool, error) {
	t.Helper()
	c.Logf(t, "%s IsRoomEncrypted %s", c.logPrefix(), roomID)
	return c.Client.IsRoomEncrypted(t, roomID)
}

func (c *LoggedClient) SendMessage(t *testing.T, roomID, text string) (eventID string) {
	t.Helper()
	c.Logf(t, "%s SendMessage %s => %s", c.logPrefix(), roomID, text)
	eventID = c.Client.SendMessage(t, roomID, text)
	c.Logf(t, "%s SendMessage %s => %s", c.logPrefix(), roomID, eventID)
	return
}

func (c *LoggedClient) WaitUntilEventInRoom(t *testing.T, roomID string, checker func(e Event) bool) Waiter {
	t.Helper()
	c.Logf(t, "%s WaitUntilEventInRoom %s", c.logPrefix(), roomID)
	return c.Client.WaitUntilEventInRoom(t, roomID, checker)
}

func (c *LoggedClient) MustBackpaginate(t *testing.T, roomID string, count int) {
	t.Helper()
	c.Logf(t, "%s MustBackpaginate %d %s", c.logPrefix(), count, roomID)
	c.Client.MustBackpaginate(t, roomID, count)
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
	Membership string
}

type Waiter interface {
	Wait(t *testing.T, s time.Duration)
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
