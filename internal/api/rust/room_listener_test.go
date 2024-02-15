package rust

import (
	"testing"

	"github.com/matrix-org/complement/must"
)

func TestRoomListener(t *testing.T) {
	rl := NewRoomsListener()

	// basic functionality
	recv := make(chan string, 2)
	cancel := rl.AddListener(func(broadcastRoomID string) (cancel bool) {
		recv <- broadcastRoomID
		return false
	})
	rl.BroadcastUpdateForRoom("foo")
	must.Equal(t, <-recv, "foo", "basic usage")

	// multiple broadcasts
	rl.BroadcastUpdateForRoom("bar")
	rl.BroadcastUpdateForRoom("baz")
	must.Equal(t, <-recv, "bar", "multiple broadcasts")
	must.Equal(t, <-recv, "baz", "multiple broadcasts")

	// multiple listeners
	recv2 := make(chan string, 2)
	shouldCancel := false
	cancel2 := rl.AddListener(func(broadcastRoomID string) (cancel bool) {
		recv2 <- broadcastRoomID
		return shouldCancel
	})
	rl.BroadcastUpdateForRoom("ping")
	must.Equal(t, <-recv, "ping", "multiple listeners")
	must.Equal(t, <-recv2, "ping", "multiple listeners")

	// once cancelled, no more data
	cancel()
	rl.BroadcastUpdateForRoom("quuz")
	select {
	case <-recv:
		t.Fatalf("received room id after cancel()")
	default:
		// we expect to hit this
	}
	// but the 2nd listener gets it
	must.Equal(t, <-recv2, "quuz", "uncancelled listener")

	// returning true from the listener automatically cancels
	shouldCancel = true
	rl.BroadcastUpdateForRoom("final message")
	must.Equal(t, <-recv2, "final message", "cancel bool")
	rl.BroadcastUpdateForRoom("no one is listening")
	select {
	case <-recv2:
		t.Fatalf("received room id after returning true")
	default:
		// we expect to hit this
	}

	// calling the cancel() function in addition to returning true no-ops
	cancel2()
}
