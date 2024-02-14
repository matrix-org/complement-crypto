package rust

import (
	"sync"
	"sync/atomic"
)

type RoomsListener struct {
	listeners  map[int32]func(roomID string) (cancel bool)
	listenerID atomic.Int32
	mu         *sync.RWMutex
}

func NewRoomsListener() *RoomsListener {
	return &RoomsListener{
		listeners: make(map[int32]func(roomID string) (cancel bool)),
		mu:        &sync.RWMutex{},
	}
}

// AddListener registers the given callback, which will be invoked for every call to BroadcastUpdateForRoom.
func (l *RoomsListener) AddListener(callback func(broadcastRoomID string) (cancel bool)) (cancel func()) {
	id := l.listenerID.Add(1)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.listeners[id] = callback
	return func() {
		l.removeListener(id)
	}
}

func (l *RoomsListener) removeListener(id int32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.listeners, id)
}

// BroadcastUpdateForRoom informs all attached listeners that something has happened in relation
// to this room ID. This could be a new event, or the room appearing in all_rooms, or something else entirely.
// It is up to the listener to decide what to do upon receipt of the poke.
func (l *RoomsListener) BroadcastUpdateForRoom(roomID string) {
	// take a snapshot of callbacks so callbacks can unregister themselves without causing deadlocks due to
	// .Lock within .RLock.
	callbacks := map[int32]func(roomID string) (cancel bool){}
	l.mu.RLock()
	for id, cb := range l.listeners {
		callbacks[id] = cb
	}
	l.mu.RUnlock()
	for id, cb := range callbacks {
		if cb(roomID) {
			l.removeListener(id)
		}
	}
}
