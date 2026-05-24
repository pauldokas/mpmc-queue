package tests

import (
	"math"
	"testing"
	"mpmc-queue/queue"
)

type MaliciousBatchItem struct{}

func (m MaliciousBatchItem) Size() int {
	return math.MaxInt64 - int(queue.BaseQueueDataSize) - 100 // Try to make it overflow
}

func TestBatchMemoryOverflow(t *testing.T) {
	q := queue.NewQueue("test-overflow")
	defer q.Close()

	// 1 MB max memory
	batch := []any{MaliciousBatchItem{}, MaliciousBatchItem{}}

	err := q.TryEnqueueBatch(batch)
	if err == nil {
		t.Fatalf("VULNERABILITY: Integer overflow allowed bypassing memory limit!")
	} else {
		t.Logf("Safe: %v", err)
	}
}
