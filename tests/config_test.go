package tests

import (
	"testing"
	"time"

	"mpmc-queue/queue"
)

func TestConfigurableMemoryLimit(t *testing.T) {
	// Create queue with small memory limit
	config := queue.QueueConfig{
		TTL:       10 * time.Minute,
		MaxMemory: 1024, // 1KB
	}
	q := queue.NewQueueWithConfig("test-config", config)
	defer q.Close()

	if q.GetMemoryUsage() != 0 {
		t.Errorf("Expected 0 memory usage, got %d", q.GetMemoryUsage())
	}

	// Try to add large item
	largePayload := make([]byte, 2048) // 2KB
	err := q.TryEnqueue(largePayload)
	if err == nil {
		t.Error("Expected error for large payload, got nil")
	}

	if memErr, ok := err.(*queue.MemoryLimitError); ok {
		if memErr.Max != 1024 {
			t.Errorf("Expected max memory 1024, got %d", memErr.Max)
		}
	} else {
		t.Errorf("Expected MemoryLimitError, got %T: %v", err, err)
	}
}

func TestMemoryCaching(t *testing.T) {
	// We can't directly test internal cache, but we can verify performance or correctness
	q := queue.NewQueue("test-caching")
	defer q.Close()

	// 1. Primitive type (should be cached as fixed)
	if err := q.TryEnqueue(123); err != nil {
		t.Errorf("Failed to enqueue int: %v", err)
	}

	// 2. Struct with primitives (should be cached as fixed)
	type Point struct {
		X, Y int
	}
	if err := q.TryEnqueue(Point{1, 2}); err != nil {
		t.Errorf("Failed to enqueue struct: %v", err)
	}

	// 3. String (variable, not cached as fixed)
	if err := q.TryEnqueue("test string"); err != nil {
		t.Errorf("Failed to enqueue string: %v", err)
	}

	// 4. Struct with string (variable)
	type Person struct {
		Name string
		Age  int
	}
	if err := q.TryEnqueue(Person{"Alice", 30}); err != nil {
		t.Errorf("Failed to enqueue struct with string: %v", err)
	}
}

func TestBatchMemoryValidation(t *testing.T) {
	// Create queue with 20KB limit (enough for 2 items + chunk overhead, but not 3)
	// Chunk overhead is ~8KB. Item overhead is ~200B.
	// 2 items (4000B payload) = ~8.4KB items + 8KB chunk = ~16.4KB
	// 3 items (4000B payload) = ~12.6KB items + 8KB chunk = ~20.6KB
	config := queue.QueueConfig{
		TTL:       10 * time.Minute,
		MaxMemory: 20 * 1024,
	}
	q := queue.NewQueueWithConfig("test-batch-mem", config)
	defer q.Close()

	// Create payload that fits individually but batch fails
	payload := make([]byte, 4000)

	// Batch of 3
	batch := []any{payload, payload, payload}

	// Should fail atomically
	err := q.TryEnqueueBatch(batch)
	if err == nil {
		t.Error("Expected error for batch exceeding limit, got nil")
	}

	if !q.IsEmpty() {
		t.Error("Queue should be empty after failed batch")
	}

	// Verify logic with smaller batch that fits
	batchSmall := []any{payload, payload}
	if err := q.TryEnqueueBatch(batchSmall); err != nil {
		t.Errorf("Failed to enqueue valid batch: %v", err)
	}
}
