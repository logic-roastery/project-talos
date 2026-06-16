package deploy

import (
	"testing"
	"time"

	"github.com/logic-roastery/project-talos/internal/domain"
)

func TestEventBroadcasterCleansUpOnUnsubscribe(t *testing.T) {
	t.Parallel()

	broadcaster := NewEventBroadcaster()
	_, unsubscribe := broadcaster.Subscribe(42)
	if count := broadcaster.SubscriberCount(42); count != 1 {
		t.Fatalf("SubscriberCount() = %d, want 1", count)
	}

	unsubscribe()
	if count := broadcaster.SubscriberCount(42); count != 0 {
		t.Fatalf("SubscriberCount() = %d, want 0", count)
	}
}

func TestEventBroadcasterDoesNotBlockSlowSubscribers(t *testing.T) {
	t.Parallel()

	broadcaster := NewEventBroadcaster()
	ch, _ := broadcaster.Subscribe(7)

	for i := 0; i < deployEventSubscriberBuffer; i++ {
		broadcaster.Publish(&domain.DeployEvent{DeployID: 7, Message: "fill"})
	}

	done := make(chan struct{})
	go func() {
		broadcaster.Publish(&domain.DeployEvent{DeployID: 7, Message: "overflow"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish() blocked on a slow subscriber")
	}

	select {
	case <-ch:
	default:
	}

	if count := broadcaster.SubscriberCount(7); count != 0 {
		t.Fatalf("slow subscriber should be dropped, got %d subscribers", count)
	}
}
