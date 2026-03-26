package gateway

import (
	"sync"

	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
)

type syncSignal struct {
	event *domainconversation.EventEnvelope
}

type syncNotifier struct {
	mu          sync.Mutex
	subscribers map[chan syncSignal]struct{}
}

func newSyncNotifier() *syncNotifier {
	return &syncNotifier{
		subscribers: make(map[chan syncSignal]struct{}),
	}
}

func (n *syncNotifier) subscribe() (<-chan syncSignal, func()) {
	if n == nil {
		return nil, func() {}
	}

	ch := make(chan syncSignal, 32)

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
	n.broadcast(syncSignal{})
}

func (n *syncNotifier) publish(event domainconversation.EventEnvelope) {
	if n == nil || event.EventID == "" {
		return
	}

	cloned := cloneSyncEvent(event)
	n.broadcast(syncSignal{event: &cloned})
}

func (n *syncNotifier) broadcast(signal syncSignal) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	for subscriber := range n.subscribers {
		select {
		case subscriber <- signal:
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

func (a *api) publishSyncEvent(event domainconversation.EventEnvelope) {
	if a == nil || a.syncNotifier == nil {
		return
	}

	a.syncNotifier.publish(event)
}

func (a *api) publishSyncEvents(events ...domainconversation.EventEnvelope) {
	if a == nil || a.syncNotifier == nil {
		return
	}

	for _, event := range events {
		if event.EventID == "" {
			continue
		}
		a.syncNotifier.publish(event)
	}
}

func (a *api) subscribeSyncNotifications() (<-chan syncSignal, func()) {
	if a == nil {
		return nil, func() {}
	}

	return a.syncNotifier.subscribe()
}

func cloneSyncEvent(event domainconversation.EventEnvelope) domainconversation.EventEnvelope {
	cloned := event
	cloned.Metadata = cloneStringMap(event.Metadata)
	cloned.Payload.Nonce = append([]byte(nil), event.Payload.Nonce...)
	cloned.Payload.Ciphertext = append([]byte(nil), event.Payload.Ciphertext...)
	cloned.Payload.AAD = append([]byte(nil), event.Payload.AAD...)
	cloned.Payload.Metadata = cloneStringMap(event.Payload.Metadata)
	return cloned
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
