package tests

import (
	"testing"
	"mpmc-queue/queue"
)

func TestMapBypassMemoryLimit(t *testing.T) {
	q := queue.NewQueueWithConfig("map-bypass", queue.QueueConfig{
		MaxMemory: 1024, // 1KB limit
	})
	defer q.Close()

	// Create a huge payload using maps
	hugeData := make([]byte, 10*1024*1024) // 10MB
	payload := map[string]any{
		"data": hugeData,
	}

	err := q.TryEnqueue(payload)
	if err == nil {
		t.Fatalf("VULNERABILITY: Queue allowed 10MB map payload despite 1KB limit!")
	} else {
		t.Logf("Safe: %v", err)
	}
}
