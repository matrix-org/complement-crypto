package api

import (
	"testing"
)

type JSClient struct{}

func NewJSClient(opts ClientCreationOpts) Client {
	return nil
}

// Init is called prior to any test execution. Do any setup code here e.g run a browser.
// Call close() when the test terminates to clean up resources.
// TODO: will this be too slow if we spin up a browser for each test?
func (c *JSClient) Init(t *testing.T) (close func()) {
	return
}

// StartSyncing to begin syncing from sync v2 / sliding sync.
// Tests should call stopSyncing() at the end of the test.
func (c *JSClient) StartSyncing(t *testing.T) (stopSyncing func()) {
	return
}

// IsRoomEncrypted returns true if the room is encrypted. May return an error e.g if you
// provide a bogus room ID.
func (c *JSClient) IsRoomEncrypted(roomID string) (bool, error) {
	return false, nil
}

// SendMessage sends the given text as an m.room.message with msgtype:m.text into the given
// room. Returns the event ID of the sent event.
func (c *JSClient) SendMessage(t *testing.T, roomID, text string) {
	return
}

func (c *JSClient) WaitUntilEventInRoom(t *testing.T, roomID, wantBody string) Waiter {
	return nil
}
