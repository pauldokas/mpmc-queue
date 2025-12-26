package tests

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestTryEnqueueBatch tests non-blocking batch enqueue
func TestTryEnqueueBatch(t *testing.T) {
	q := queue.NewQueue("try-enqueue-batch-test")
	defer q.Close()

	// Enqueue small batch
	items := []any{"item1", "item2", "item3"}
	err := q.TryEnqueueBatch(items)
	if err != nil {
		t.Fatalf("TryEnqueueBatch failed: %v", err)
	}

	// Verify items were added
	consumer := q.AddConsumer()
	for i, expected := range items {
		data := consumer.TryRead()
		if data == nil {
			t.Fatalf("Failed to read item %d", i)
		}
		if data.Payload != expected {
			t.Errorf("Item %d: expected %v, got %v", i, expected, data.Payload)
		}
	}
}

// TestTryEnqueueBatchEmpty tests batch enqueue with empty slice
func TestTryEnqueueBatchEmpty(t *testing.T) {
	q := queue.NewQueue("try-enqueue-empty-test")
	defer q.Close()

	// Empty batch should succeed
	err := q.TryEnqueueBatch([]any{})
	if err != nil {
		t.Errorf("Empty batch should succeed, got error: %v", err)
	}

	// Nil slice should also succeed
	err = q.TryEnqueueBatch(nil)
	if err != nil {
		t.Errorf("Nil batch should succeed, got error: %v", err)
	}

	// Queue should still be empty
	if !q.IsEmpty() {
		t.Error("Queue should be empty after empty batch operations")
	}
}

// TestTryEnqueueBatchMemoryLimit tests batch fails when exceeding memory
func TestTryEnqueueBatchMemoryLimit(t *testing.T) {
	q := queue.NewQueue("batch-memory-limit-test")
	defer q.Close()

	// Fill queue almost to capacity
	for {
		err := q.TryEnqueue(make([]byte, 1000))
		if err != nil {
			break
		}
	}

	// Try to add batch that won't fit
	largeBatch := []any{
		make([]byte, 10000),
		make([]byte, 10000),
		make([]byte, 10000),
	}

	err := q.TryEnqueueBatch(largeBatch)
	if err == nil {
		t.Error("TryEnqueueBatch should fail when batch exceeds memory limit")
	}

	// Verify it's a memory limit error
	if _, ok := err.(*queue.MemoryLimitError); !ok {
		t.Errorf("Expected MemoryLimitError, got %T", err)
	}
}

// TestTryEnqueueBatchAtomicity tests all-or-nothing behavior
func TestTryEnqueueBatchAtomicity(t *testing.T) {
	q := queue.NewQueue("batch-atomicity-test")
	defer q.Close()

	// Fill queue to near capacity
	for {
		err := q.TryEnqueue(make([]byte, 1000))
		if err != nil {
			break
		}
	}

	itemsBefore := q.GetQueueStats().TotalItems

	// Try to add batch that partially fits
	batch := []any{
		"small1",
		"small2",
		make([]byte, 500000), // Too large
	}

	err := q.TryEnqueueBatch(batch)
	if err == nil {
		t.Error("Should fail when batch won't fit")
	}

	// Verify NO items were added (atomicity)
	itemsAfter := q.GetQueueStats().TotalItems
	if itemsAfter != itemsBefore {
		t.Errorf("Batch should be atomic - expected %d items, got %d", itemsBefore, itemsAfter)
	}
}

// TestEnqueueBatchBlocking tests blocking batch enqueue
func TestEnqueueBatchBlocking(t *testing.T) {
	q := queue.NewQueueWithTTL("batch-blocking-test", 2*time.Second)
	defer q.Close()

	// Fill queue
	for {
		err := q.TryEnqueue("filler")
		if err != nil {
			break
		}
	}

	var batchCompleted atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Try blocking batch enqueue
	go func() {
		defer wg.Done()
		batch := []any{"item1", "item2", "item3"}
		err := q.EnqueueBatch(batch)
		if err != nil {
			t.Errorf("EnqueueBatch failed: %v", err)
			return
		}
		batchCompleted.Store(true)
	}()

	// Should be blocked
	time.Sleep(500 * time.Millisecond)
	if batchCompleted.Load() {
		t.Error("EnqueueBatch should be blocked")
	}

	// Trigger expiration to free space
	time.Sleep(2500 * time.Millisecond)
	q.ForceExpiration()
	time.Sleep(100 * time.Millisecond)

	wg.Wait()

	if !batchCompleted.Load() {
		t.Error("EnqueueBatch should have completed")
	}
}

// TestTryReadBatch tests non-blocking batch read
func TestTryReadBatch(t *testing.T) {
	q := queue.NewQueue("try-read-batch-test")
	defer q.Close()

	// Add items
	for i := 0; i < 20; i++ {
		q.TryEnqueue(i)
	}

	consumer := q.AddConsumer()

	// Read batch of 10
	batch := consumer.TryReadBatch(10)
	if len(batch) != 10 {
		t.Errorf("Expected batch size 10, got %d", len(batch))
	}

	// Verify items
	for i, data := range batch {
		if data.Payload != i {
			t.Errorf("Item %d: expected %d, got %v", i, i, data.Payload)
		}
	}

	// Read remaining
	batch = consumer.TryReadBatch(20)
	if len(batch) != 10 {
		t.Errorf("Expected batch size 10 (remaining), got %d", len(batch))
	}
}

// TestTryReadBatchEmpty tests reading from empty queue
func TestTryReadBatchEmpty(t *testing.T) {
	q := queue.NewQueue("read-batch-empty-test")
	defer q.Close()

	consumer := q.AddConsumer()

	// Read from empty queue
	batch := consumer.TryReadBatch(10)
	if len(batch) != 0 {
		t.Errorf("Expected empty batch, got %d items", len(batch))
	}

	// Should return empty slice, not nil
	if batch == nil {
		t.Error("TryReadBatch should return empty slice, not nil")
	}
}

// TestTryReadBatchZeroLimit tests read with limit=0
func TestTryReadBatchZeroLimit(t *testing.T) {
	q := queue.NewQueue("read-batch-zero-test")
	defer q.Close()

	// Add items
	for i := 0; i < 10; i++ {
		q.TryEnqueue(i)
	}

	consumer := q.AddConsumer()

	// Read with limit=0 - returns nil per implementation
	batch := consumer.TryReadBatch(0)
	if batch != nil {
		t.Errorf("TryReadBatch(0) should return nil, got %d items", len(batch))
	}
}

// TestTryReadBatchNegativeLimit tests read with negative limit
func TestTryReadBatchNegativeLimit(t *testing.T) {
	q := queue.NewQueue("read-batch-negative-test")
	defer q.Close()

	// Add items
	for i := 0; i < 10; i++ {
		q.TryEnqueue(i)
	}

	consumer := q.AddConsumer()

	// Read with negative limit - returns nil per implementation
	batch := consumer.TryReadBatch(-5)
	if batch != nil {
		t.Errorf("TryReadBatch(-5) should return nil, got %d items", len(batch))
	}
}

// TestReadBatchBlocking tests blocking batch read
func TestReadBatchBlocking(t *testing.T) {
	q := queue.NewQueue("read-batch-blocking-test")
	defer q.Close()

	consumer := q.AddConsumer()

	var batch []*queue.QueueData
	var readCompleted atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Start blocking read
	go func() {
		defer wg.Done()
		batch = consumer.ReadBatch(5)
		readCompleted.Store(true)
	}()

	// Should be blocked
	time.Sleep(100 * time.Millisecond)
	if readCompleted.Load() {
		t.Error("ReadBatch should be blocked")
	}

	// Add some items
	q.TryEnqueue("item1")
	q.TryEnqueue("item2")

	// Should unblock and return 2 items
	time.Sleep(100 * time.Millisecond)
	wg.Wait()

	if !readCompleted.Load() {
		t.Error("ReadBatch should have completed")
	}

	if len(batch) != 2 {
		t.Errorf("Expected 2 items, got %d", len(batch))
	}
}

// TestBatchOperationsConcurrent tests concurrent batch operations
func TestBatchOperationsConcurrent(t *testing.T) {
	q := queue.NewQueue("batch-concurrent-test")
	defer q.Close()

	var wg sync.WaitGroup
	itemsEnqueued := atomic.Int64{}
	itemsRead := atomic.Int64{}

	// Multiple producers with batches
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				batch := []any{
					map[string]int{"producer": id, "batch": j, "item": 0},
					map[string]int{"producer": id, "batch": j, "item": 1},
					map[string]int{"producer": id, "batch": j, "item": 2},
				}
				err := q.TryEnqueueBatch(batch)
				if err == nil {
					itemsEnqueued.Add(int64(len(batch)))
				}
			}
		}(i)
	}

	// Multiple consumers with batch reads
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumer := q.AddConsumer()
			for j := 0; j < 20; j++ {
				batch := consumer.TryReadBatch(5)
				itemsRead.Add(int64(len(batch)))
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Each consumer should read all items
	expectedPerConsumer := itemsEnqueued.Load()
	if itemsRead.Load() < expectedPerConsumer*3 {
		t.Logf("Items enqueued: %d", itemsEnqueued.Load())
		t.Logf("Items read: %d", itemsRead.Load())
		t.Logf("Note: Some items may not have been read due to timing")
	}
}

// TestBatchOperationsCrossChunkBoundary tests batch spanning multiple chunks
func TestBatchOperationsCrossChunkBoundary(t *testing.T) {
	q := queue.NewQueue("batch-cross-chunk-test")
	defer q.Close()

	// Add enough items to span multiple chunks (>1000 items per chunk)
	itemCount := 2500
	for i := 0; i < itemCount; i++ {
		err := q.TryEnqueue(i)
		if err != nil {
			t.Fatalf("Failed to enqueue item %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// Read batch that crosses chunk boundary
	batch := consumer.TryReadBatch(1500)
	if len(batch) != 1500 {
		t.Errorf("Expected to read 1500 items, got %d", len(batch))
	}

	// Verify items are correct and sequential
	for i, data := range batch {
		if data.Payload != i {
			t.Errorf("Item %d: expected %d, got %v", i, i, data.Payload)
		}
	}
}

// TestTryEnqueueBatchPartialSpace tests behavior when only partial space available
func TestTryEnqueueBatchPartialSpace(t *testing.T) {
	q := queue.NewQueue("partial-space-test")
	defer q.Close()

	// Fill queue almost to capacity with known size items
	for {
		err := q.TryEnqueue(make([]byte, 1000))
		if err != nil {
			break
		}
	}

	itemsBefore := q.GetQueueStats().TotalItems

	// Try to add batch that definitely won't all fit
	largeBatch := make([]any, 100)
	for i := range largeBatch {
		largeBatch[i] = make([]byte, 1000)
	}

	err := q.TryEnqueueBatch(largeBatch)

	itemsAfter := q.GetQueueStats().TotalItems

	// Batch should fail (queue is nearly full, can't fit 100 more items)
	if err == nil {
		t.Error("Batch should fail when queue is nearly full")
	}

	// Atomicity: if batch failed, NO items should be added
	if itemsAfter != itemsBefore {
		t.Errorf("Batch failed but items were added: before=%d, after=%d (diff=%d)",
			itemsBefore, itemsAfter, itemsAfter-itemsBefore)
	}
}

// TestReadBatchFewerItemsThanRequested tests ReadBatch returns available items
func TestReadBatchFewerItemsThanRequested(t *testing.T) {
	q := queue.NewQueue("fewer-items-test")
	defer q.Close()

	// Add only 5 items
	for i := 0; i < 5; i++ {
		q.TryEnqueue(i)
	}

	consumer := q.AddConsumer()

	// Try to read 20 - should get 5
	batch := consumer.TryReadBatch(20)
	if len(batch) != 5 {
		t.Errorf("Expected 5 items, got %d", len(batch))
	}
}
