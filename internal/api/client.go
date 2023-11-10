package api

import (
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
	StartSyncing(t *testing.T) (stopSyncing func())
	// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
	// provide a bogus room ID.
	IsRoomEncrypted(t *testing.T, roomID string) (bool, error)
	// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
	// room. Returns the event ID of the sent event.
	SendMessage(t *testing.T, roomID, text string)
	// Wait until an event with the given body is seen. Not all impls expose event IDs
	// hence needing to use body as a proxy.
	WaitUntilEventInRoom(t *testing.T, roomID string, checker func(e Event) bool) Waiter
	// Backpaginate in this room by `count` events.
	MustBackpaginate(t *testing.T, roomID string, count int)
	Type() ClientType
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
