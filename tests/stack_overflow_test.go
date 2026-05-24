package tests

import (
	"testing"
	"mpmc-queue/queue"
)

func TestDeeplyNestedSliceStackOverflow(t *testing.T) {
	q := queue.NewQueueWithConfig("stack-overflow", queue.QueueConfig{
		MaxMemory: 1024 * 1024,
	})
	defer q.Close()

	// Create a deeply nested slice
	var payload any = []any{"hello"}
	for i := 0; i < 100000; i++ {
		payload = []any{payload}
	}

	// This should either fail safely or succeed, but NOT crash the process with Stack Overflow
	err := q.TryEnqueue(payload)
	if err == nil {
		t.Log("Successfully enqueued")
	} else {
		t.Logf("Failed to enqueue safely: %v", err)
	}
}
