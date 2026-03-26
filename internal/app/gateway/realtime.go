package gateway

import "sync"

type syncNotifier struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

func newSyncNotifier() *syncNotifier {
	return &syncNotifier{
		subscribers: make(map[chan struct{}]struct{}),
	}
}

func (n *syncNotifier) subscribe() (<-chan struct{}, func()) {
	if n == nil {
		return nil, func() {}
	}

	ch := make(chan struct{}, 1)

	n.mu.Lock()
	n.subscribers[ch] = struct{}{}
	n.mu.Unlock()

	return ch, func() {
		n.mu.Lock()
		delete(n.subscribers, ch)
		n.mu.Unlock()
		close(ch)
	}
}

func (n *syncNotifier) notify() {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	for subscriber := range n.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

func (a *api) notifySyncSubscribers() {
	if a == nil {
		return
	}

	a.syncNotifier.notify()
}

func (a *api) subscribeSyncNotifications() (<-chan struct{}, func()) {
	if a == nil {
		return nil, func() {}
	}

	return a.syncNotifier.subscribe()
}
