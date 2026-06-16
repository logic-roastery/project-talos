package deploy

import (
	"sync"

	"github.com/logic-roastery/project-talos/internal/domain"
)

const deployEventSubscriberBuffer = 256

type EventBroadcaster struct {
	mu          sync.Mutex
	nextID      int64
	subscribers map[int64]map[int64]chan *domain.DeployEvent
}

func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		subscribers: make(map[int64]map[int64]chan *domain.DeployEvent),
	}
}

func (b *EventBroadcaster) Subscribe(deployID int64) (<-chan *domain.DeployEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	subID := b.nextID
	ch := make(chan *domain.DeployEvent, deployEventSubscriberBuffer)
	if _, ok := b.subscribers[deployID]; !ok {
		b.subscribers[deployID] = make(map[int64]chan *domain.DeployEvent)
	}
	b.subscribers[deployID][subID] = ch

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()

			deploySubs, ok := b.subscribers[deployID]
			if !ok {
				return
			}
			current, ok := deploySubs[subID]
			if !ok {
				return
			}
			delete(deploySubs, subID)
			if len(deploySubs) == 0 {
				delete(b.subscribers, deployID)
			}
			close(current)
		})
	}

	return ch, unsubscribe
}

func (b *EventBroadcaster) Publish(event *domain.DeployEvent) {
	if event == nil {
		return
	}

	b.mu.Lock()
	deploySubs := b.subscribers[event.DeployID]
	if len(deploySubs) == 0 {
		b.mu.Unlock()
		return
	}

	type target struct {
		id int64
		ch chan *domain.DeployEvent
	}

	targets := make([]target, 0, len(deploySubs))
	for id, ch := range deploySubs {
		targets = append(targets, target{id: id, ch: ch})
	}
	b.mu.Unlock()

	var slow []int64
	for _, target := range targets {
		copy := *event
		select {
		case target.ch <- &copy:
		default:
			slow = append(slow, target.id)
		}
	}

	if len(slow) == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	deploySubs = b.subscribers[event.DeployID]
	for _, id := range slow {
		ch, ok := deploySubs[id]
		if !ok {
			continue
		}
		delete(deploySubs, id)
		close(ch)
	}
	if len(deploySubs) == 0 {
		delete(b.subscribers, event.DeployID)
	}
}

func (b *EventBroadcaster) SubscriberCount(deployID int64) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.subscribers[deployID])
}
