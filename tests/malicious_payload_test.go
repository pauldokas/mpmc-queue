package tests

import (
	"testing"
	"mpmc-queue/queue"
)

type MaliciousPayload struct{}

func (m MaliciousPayload) Size() int {
	return -1000000
}

func TestQueue_MemoryLimitBypass_NegativeSizeable(t *testing.T) {
	t.Parallel()

	config := queue.QueueConfig{
		MaxMemory: 1000,
	}
	q := queue.NewQueueWithConfig("malicious-test", config)
	defer q.Close()

	err := q.TryEnqueue(MaliciousPayload{})
	if err != nil {
		t.Fatalf("Failed to enqueue malicious payload: %v", err)
	}

	memory := q.GetMemoryUsage()
	if memory < 0 {
		t.Fatalf("Queue allowed negative memory tracking: %v", memory)
	}

	if memory <= 0 {
		t.Fatalf("Expected memory to be > 0 for malicious payload, got %v", memory)
	}
}
