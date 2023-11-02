package api

import (
	"testing"
	"time"

	"github.com/matrix-org/complement/client"
)

// Client represents a generic crypto client.
// It is an abstraction to allow tests to interact with JS and FFI bindings in an agnostic way.
type Client interface {
	// Init is called prior to any test execution. Do any setup code here e.g run a browser.
	// Call close() when the test terminates to clean up resources.
	// TODO: will this be too slow if we spin up a browser for each test?
	Init(t *testing.T) (close func())
	// StartSyncing to begin syncing from sync v2 / sliding sync.
	// Tests should call stopSyncing() at the end of the test.
	StartSyncing(t *testing.T) (stopSyncing func())
	// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
	// provide a bogus room ID.
	IsRoomEncrypted(roomID string) (bool, error)
	// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
	// room. Returns the event ID of the sent event.
	SendMessage(t *testing.T, roomID, text string)
	// Wait until an event with the given body is seen. Not all impls expose event IDs
	// hence needing to use body as a proxy.
	WaitUntilEventInRoom(t *testing.T, roomID, wantBody string) Waiter
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
	// FFI bindings don't expose type
}

type Waiter interface {
	Wait(t *testing.T, s time.Duration)
}
