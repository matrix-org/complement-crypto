package rust

// This is a recurring pattern in the ffi bindings, where a param is an interface
// which has a single OnUpdate(T) method. Rather than having many specific impls,
// just have a generic one which can be listened to via a channel. Using a channel
// means we can time out more easily using select {}.
type genericStateListener[T any] struct {
	ch       chan T
	isClosed bool
}

func newGenericStateListener[T any]() *genericStateListener[T] {
	return &genericStateListener[T]{
		ch: make(chan T),
	}
}

func (l *genericStateListener[T]) Close() {
	l.isClosed = true
	close(l.ch)
}

func (l *genericStateListener[T]) OnUpdate(state T) {
	if l.isClosed {
		return
	}
	l.ch <- state
}
