package tests

import (
	"fmt"
	"testing"

	"mpmc-queue/queue"
)

func TestConsumerHistoryLimit(t *testing.T) {
	// Create queue with small history limit
	limit := 10
	q := queue.NewQueueWithConfig("history-test", queue.QueueConfig{
		TTL:                queue.DefaultTTL,
		MaxMemory:          1024 * 1024,
		MaxConsumerHistory: limit,
	})
	defer q.Close()

	consumer := q.AddConsumer()

	// Enqueue more items than the limit
	numItems := 25
	for i := 0; i < numItems; i++ {
		err := q.TryEnqueue(fmt.Sprintf("item-%d", i))
		if err != nil {
			t.Fatalf("Failed to enqueue item %d: %v", i, err)
		}
	}

	// Read all items
	for i := 0; i < numItems; i++ {
		data := consumer.TryRead()
		if data == nil {
			t.Fatalf("Failed to read item %d", i)
		}
	}

	// Verify history size
	history := consumer.GetDequeueHistory()
	if len(history) != limit {
		t.Errorf("Expected history size %d, got %d", limit, len(history))
	}

	// Verify that history contains the MOST RECENT items
	// The last item should be "item-24"
	lastItem := history[len(history)-1]
	if lastItem.DataID == "" {
		t.Errorf("Last history item has empty DataID")
	}
}

func TestConsumerHistoryLimit_Rolling(t *testing.T) {
	limit := 5
	q := queue.NewQueueWithConfig("rolling-history-test", queue.QueueConfig{
		TTL:                queue.DefaultTTL,
		MaxMemory:          1024 * 1024,
		MaxConsumerHistory: limit,
	})
	defer q.Close()

	consumer := q.AddConsumer()

	// Enqueue and read items one by one
	for i := 0; i < 20; i++ {
		payload := fmt.Sprintf("data-%d", i)
		q.TryEnqueue(payload)
		data := consumer.TryRead()

		history := consumer.GetDequeueHistory()
		if len(history) > limit {
			t.Errorf("Iteration %d: History size %d exceeded limit %d", i, len(history), limit)
		}

		if i >= limit-1 {
			if len(history) != limit {
				t.Errorf("Iteration %d: Expected history size %d, got %d", i, limit, len(history))
			}
		} else {
			if len(history) != i+1 {
				t.Errorf("Iteration %d: Expected history size %d, got %d", i, i+1, len(history))
			}
		}

		// Verify the last item in history is the one we just read
		if history[len(history)-1].DataID != data.ID {
			t.Errorf("Iteration %d: Last history item ID mismatch", i)
		}
	}
}
