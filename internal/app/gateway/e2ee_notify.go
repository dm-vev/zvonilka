package gateway

import (
	"sync"

	domaine2ee "github.com/dm-vev/zvonilka/internal/domain/e2ee"
)

type e2eeSignal struct {
	update *domaine2ee.Update
}

type e2eeNotifier struct {
	mu          sync.Mutex
	subscribers map[chan e2eeSignal]struct{}
}

func newE2EENotifier() *e2eeNotifier {
	return &e2eeNotifier{subscribers: make(map[chan e2eeSignal]struct{})}
}

func (n *e2eeNotifier) subscribe() (<-chan e2eeSignal, func()) {
	ch := make(chan e2eeSignal, 32)

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

func (n *e2eeNotifier) publish(updates ...domaine2ee.Update) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	for _, update := range updates {
		if update.ID == "" {
			continue
		}
		cloned := cloneE2EEUpdate(update)
		for subscriber := range n.subscribers {
			select {
			case subscriber <- e2eeSignal{update: &cloned}:
			default:
			}
		}
	}
}

func (a *api) publishE2EEUpdates(updates ...domaine2ee.Update) {
	if a == nil || a.e2eeNotifier == nil {
		return
	}
	a.e2eeNotifier.publish(updates...)
}

func (a *api) subscribeE2EEUpdates() (<-chan e2eeSignal, func()) {
	if a == nil || a.e2eeNotifier == nil {
		return nil, func() {}
	}
	return a.e2eeNotifier.subscribe()
}

func cloneE2EEUpdate(value domaine2ee.Update) domaine2ee.Update {
	cloned := value
	cloned.Metadata = cloneStringMap(value.Metadata)
	return cloned
}
