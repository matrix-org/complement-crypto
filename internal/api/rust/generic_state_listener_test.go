package rust

import (
	"testing"
	"time"

	"github.com/matrix-org/complement/must"
)

func receiveFromChannel(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case val := <-ch:
		return val
	case <-time.After(time.Second):
		t.Fatalf("failed to receive from channel")
	}
	return ""
}

func TestGenericStateListener(t *testing.T) {
	l := newGenericStateListener[string]()
	go l.OnUpdate("foo")
	must.Equal(t, receiveFromChannel(t, l.ch), "foo", "OnUpdate")
	go l.OnUpdate("bar")
	must.Equal(t, receiveFromChannel(t, l.ch), "bar", "OnUpdate")

	// can close and then no more updates get sent
	l.Close()
	l.OnUpdate("baz")                                        // this should not block due to not sending on the channel
	must.Equal(t, receiveFromChannel(t, l.ch), "", "Closed") // recv on a closed channel is the zero value

	// can close repeatedly without panicking
	l.Close()
	l.Close()
}
