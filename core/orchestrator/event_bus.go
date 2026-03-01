package orchestrator

import "sync"

type Bus struct {
	mu   sync.RWMutex
	next int
	subs map[int]chan Event
}

func NewBus() *Bus {
	return &Bus{subs: map[int]chan Event{}}
}

func (b *Bus) Subscribe(buffer int) (int, <-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan Event, buffer)
	b.subs[id] = ch
	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(c)
		}
	}
	return id, ch, unsub
}

func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- event:
		default:
		}
	}
}
