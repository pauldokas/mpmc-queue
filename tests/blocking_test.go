package tests

import (
	"sync"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestBlockingEnqueue tests that Enqueue blocks when queue is full and unblocks when space is available
func TestBlockingEnqueue(t *testing.T) {
	// Use short TTL so items expire quickly
	q := queue.NewQueueWithTTL("blocking-enqueue-test", 2*time.Second)
	defer q.Close()

	// Fill the queue to capacity
	for {
		err := q.TryEnqueue("filler")
		if err != nil {
			// Queue is full
			break
		}
	}

	// Try to enqueue one more item - this should block
	var enqueueCompleted bool
	var mu sync.Mutex

	go func() {
		// This should block until items expire
		err := q.Enqueue("blocked-item")
		if err != nil {
			t.Errorf("Enqueue failed: %v", err)
			return
		}
		mu.Lock()
		enqueueCompleted = true
		mu.Unlock()
	}()

	// Check that enqueue is blocked
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	if enqueueCompleted {
		t.Error("Enqueue should still be blocked")
	}
	mu.Unlock()

	// Wait for items to expire (TTL is 2 seconds) and force expiration
	time.Sleep(3 * time.Second)
	q.ForceExpiration()                // Manually trigger expiration to free space
	time.Sleep(100 * time.Millisecond) // Give enqueue goroutine time to complete

	// Check that enqueue completed
	mu.Lock()
	completed := enqueueCompleted
	mu.Unlock()

	if !completed {
		t.Error("Enqueue should have completed after expiration freed space")
	}
}

// TestBlockingRead tests that Read blocks when no data is available and unblocks when data is enqueued
func TestBlockingRead(t *testing.T) {
	q := queue.NewQueue("blocking-read-test")
	defer q.Close()

	consumer := q.AddConsumer()

	var readCompleted bool
	var readData *queue.QueueData
	var mu sync.Mutex

	// Start a goroutine that tries to read (should block)
	go func() {
		data := consumer.Read()
		mu.Lock()
		readData = data
		readCompleted = true
		mu.Unlock()
	}()

	// Check that read is blocked
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	if readCompleted {
		t.Error("Read should still be blocked")
	}
	mu.Unlock()

	// Enqueue data
	expectedPayload := "test-data"
	if err := q.TryEnqueue(expectedPayload); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Wait for read to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	completed := readCompleted
	data := readData
	mu.Unlock()

	if !completed {
		t.Error("Read should have completed after data was enqueued")
	}

	if data == nil {
		t.Fatal("Read returned nil data")
	}

	if data.Payload != expectedPayload {
		t.Errorf("Expected payload %v, got %v", expectedPayload, data.Payload)
	}
}

// TestTryEnqueueNonBlocking tests that TryEnqueue returns immediately when queue is full
func TestTryEnqueueNonBlocking(t *testing.T) {
	q := queue.NewQueue("try-enqueue-test")
	defer q.Close()

	// Fill the queue to capacity
	for {
		err := q.TryEnqueue("filler")
		if err != nil {
			// Queue is full
			break
		}
	}

	// Try to enqueue one more item - should return error immediately
	start := time.Now()
	err := q.TryEnqueue("should-fail")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("TryEnqueue should have returned an error when queue is full")
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("TryEnqueue took %v, should return immediately", elapsed)
	}
}

// TestTryReadNonBlocking tests that TryRead returns immediately when no data is available
func TestTryReadNonBlocking(t *testing.T) {
	q := queue.NewQueue("try-read-test")
	defer q.Close()

	consumer := q.AddConsumer()

	// Try to read from empty queue - should return nil immediately
	start := time.Now()
	data := consumer.TryRead()
	elapsed := time.Since(start)

	if data != nil {
		t.Error("TryRead should have returned nil when queue is empty")
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("TryRead took %v, should return immediately", elapsed)
	}
}

// TestBlockingReadBatch tests that ReadBatch blocks until at least one item is available
func TestBlockingReadBatch(t *testing.T) {
	q := queue.NewQueue("blocking-readbatch-test")
	defer q.Close()

	consumer := q.AddConsumer()

	var batchCompleted bool
	var batch []*queue.QueueData
	var mu sync.Mutex

	// Start a goroutine that tries to read batch (should block)
	go func() {
		result := consumer.ReadBatch(5)
		mu.Lock()
		batch = result
		batchCompleted = true
		mu.Unlock()
	}()

	// Check that read is blocked
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	if batchCompleted {
		t.Error("ReadBatch should still be blocked")
	}
	mu.Unlock()

	// Enqueue some data (less than requested batch size)
	for i := 0; i < 3; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Wait for read to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	completed := batchCompleted
	resultBatch := batch
	mu.Unlock()

	if !completed {
		t.Error("ReadBatch should have completed after data was enqueued")
	}

	if len(resultBatch) != 3 {
		t.Errorf("Expected batch size 3, got %d", len(resultBatch))
	}
}
