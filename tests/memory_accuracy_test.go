package tests

import (
	"testing"

	"mpmc-queue/queue"
)

func TestMemoryAccuracy(t *testing.T) {
	q := queue.NewQueue("memory-accuracy-test")
	defer q.Close()

	// Initial memory usage should be ChunkNodeSize (one chunk created by default)
	// plus any other overhead.
	initialMem := q.GetMemoryUsage()

	// Create a known payload: 1KB byte slice
	payload := make([]byte, 1024)

	// Enqueue the payload
	if err := q.TryEnqueue(payload); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Get memory usage after enqueue
	afterMem := q.GetMemoryUsage()

	// Calculate the increase
	increase := afterMem - initialMem

	expectedIncrease := queue.BaseQueueDataSize + 1024 + 36 + int64(len("memory-accuracy-test")) + 7 + queue.BaseQueueEventSize + queue.ChunkNodeSize

	if increase != expectedIncrease {

		t.Errorf("Expected memory increase of %d, got %d", expectedIncrease, increase)
		t.Logf("BaseQueueDataSize: %d", queue.BaseQueueDataSize)
		t.Logf("Payload: 1024")
		t.Logf("UUID: 36")
		t.Logf("QueueName: %d", len("memory-accuracy-test"))
		t.Logf("EventType: 7")
		t.Logf("BaseQueueEventSize: %d", queue.BaseQueueEventSize)
	}
}
