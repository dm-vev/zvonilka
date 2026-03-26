package gateway

import (
	"sync"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

type callSignal struct {
	event *domaincall.Event
}

type callNotifier struct {
	mu          sync.Mutex
	subscribers map[chan callSignal]struct{}
}

func newCallNotifier() *callNotifier {
	return &callNotifier{subscribers: make(map[chan callSignal]struct{})}
}

func (n *callNotifier) subscribe() (<-chan callSignal, func()) {
	ch := make(chan callSignal, 32)

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

func (n *callNotifier) publish(events ...domaincall.Event) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	for _, event := range events {
		if event.EventID == "" {
			continue
		}
		cloned := cloneCallEvent(event)
		for subscriber := range n.subscribers {
			select {
			case subscriber <- callSignal{event: &cloned}:
			default:
			}
		}
	}
}

func (a *api) publishCallEvents(events ...domaincall.Event) {
	if a == nil || a.callNotifier == nil {
		return
	}
	a.callNotifier.publish(events...)
}

func (a *api) subscribeCallEvents() (<-chan callSignal, func()) {
	if a == nil || a.callNotifier == nil {
		return nil, func() {}
	}
	return a.callNotifier.subscribe()
}

func cloneCallEvent(event domaincall.Event) domaincall.Event {
	cloned := event
	cloned.Metadata = cloneStringMap(event.Metadata)
	cloned.Call = cloneCallModel(event.Call)
	return cloned
}

func cloneCallModel(value domaincall.Call) domaincall.Call {
	cloned := value
	cloned.Invites = append([]domaincall.Invite(nil), value.Invites...)
	cloned.Participants = append([]domaincall.Participant(nil), value.Participants...)
	return cloned
}
