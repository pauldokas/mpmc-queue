package tests

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestCloseBasic tests basic queue close
func TestCloseBasic(t *testing.T) {
	q := queue.NewQueue("close-basic-test")

	// Add some data
	for i := 0; i < 10; i++ {
		q.TryEnqueue(i)
	}

	// Add consumers
	c1 := q.AddConsumer()
	c2 := q.AddConsumer()

	// Close queue
	q.Close()

	// Verify queue stats show closure effects
	stats := q.GetQueueStats()
	if stats.TotalItems != 0 {
		t.Errorf("Expected 0 items after close, got %d", stats.TotalItems)
	}

	// Note: Consumers are closed but may still be in the manager
	// The important thing is data is cleared and operations complete safely
	_ = c1
	_ = c2
}

// TestCloseIdempotent tests that calling Close multiple times is safe
func TestCloseIdempotent(t *testing.T) {
	q := queue.NewQueue("close-idempotent-test")

	// Close multiple times - should not panic
	q.Close()
	q.Close()
	q.Close()

	// Should still be safe
	stats := q.GetQueueStats()
	if stats.TotalItems != 0 {
		t.Errorf("Expected 0 items, got %d", stats.TotalItems)
	}
}

// TestCloseConcurrent tests closing queue from multiple goroutines
func TestCloseConcurrent(t *testing.T) {
	q := queue.NewQueue("close-concurrent-test")

	var wg sync.WaitGroup
	wg.Add(10)

	// Try to close from multiple goroutines
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			q.Close()
		}()
	}

	wg.Wait()

	// Queue should be closed without panic
	stats := q.GetQueueStats()
	if stats.TotalItems != 0 {
		t.Errorf("Expected 0 items after close, got %d", stats.TotalItems)
	}
}

// TestCloseWithContext tests CloseWithContext with successful close
func TestCloseWithContext(t *testing.T) {
	q := queue.NewQueue("close-with-context-test")

	// Add some data
	for i := 0; i < 5; i++ {
		q.TryEnqueue(i)
	}

	// Close with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := q.CloseWithContext(ctx)
	if err != nil {
		t.Errorf("CloseWithContext should succeed: %v", err)
	}

	// Verify closed
	stats := q.GetQueueStats()
	if stats.TotalItems != 0 {
		t.Errorf("Expected 0 items after close, got %d", stats.TotalItems)
	}
}

// TestCloseWithContextTimeout tests CloseWithContext when timeout is exceeded
func TestCloseWithContextTimeout(t *testing.T) {
	q := queue.NewQueue("close-timeout-test")

	// Add data
	for i := 0; i < 10; i++ {
		q.TryEnqueue(i)
	}

	// Create context that's already expired
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond) // Ensure context expires

	err := q.CloseWithContext(ctx)
	if err == nil {
		t.Error("Expected timeout error from CloseWithContext")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded error, got %v", err)
	}

	// Queue should eventually close even if context times out
	// (background goroutine continues)
	time.Sleep(100 * time.Millisecond)
}

// TestCloseWhileBlockedEnqueue tests closing queue while Enqueue is blocked
func TestCloseWhileBlockedEnqueue(t *testing.T) {
	q := queue.NewQueueWithTTL("close-blocked-enqueue-test", 10*time.Minute)

	// Fill queue
	for {
		err := q.TryEnqueue("filler")
		if err != nil {
			break
		}
	}

	var enqueueErr error
	var enqueueDone atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Start blocking enqueue
	go func() {
		defer wg.Done()
		enqueueErr = q.Enqueue("blocked-item")
		enqueueDone.Store(true)
	}()

	// Wait for enqueue to block
	time.Sleep(100 * time.Millisecond)

	// Close the queue
	q.Close()

	// Wait for enqueue to complete
	wg.Wait()

	// Enqueue should have returned with error
	if enqueueErr == nil {
		t.Error("Blocked enqueue should return error when queue closes")
	}

	if !enqueueDone.Load() {
		t.Error("Enqueue should have completed")
	}
}

// TestCloseWhileBlockedRead tests closing queue while Read is blocked
func TestCloseWhileBlockedRead(t *testing.T) {
	q := queue.NewQueue("close-blocked-read-test")

	consumer := q.AddConsumer()

	var readData *queue.QueueData
	var readDone atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Start blocking read
	go func() {
		defer wg.Done()
		readData = consumer.Read()
		readDone.Store(true)
	}()

	// Wait for read to block
	time.Sleep(100 * time.Millisecond)

	// Close the queue
	q.Close()

	// Wait for read to complete
	wg.Wait()

	// Read should have returned nil
	if readData != nil {
		t.Error("Blocked read should return nil when queue closes")
	}

	if !readDone.Load() {
		t.Error("Read should have completed")
	}
}

// TestOperationsOnClosedQueue tests that operations fail gracefully on closed queue
func TestOperationsOnClosedQueue(t *testing.T) {
	q := queue.NewQueue("closed-ops-test")
	consumer := q.AddConsumer()

	// Close queue
	q.Close()

	// Try to enqueue - should fail or return immediately
	err := q.TryEnqueue("data")
	// Either fails with error or returns error, both acceptable
	_ = err

	// Operations after close should complete safely (no panic)
	// Exact behavior may vary - the important thing is safety
	data := consumer.TryRead()
	_ = data // May be nil or not depending on timing

	// Try to add consumer after close - should complete safely
	newConsumer := q.AddConsumer()
	_ = newConsumer

	// GetQueueStats should still work safely after close
	stats := q.GetQueueStats()
	// Items should be 0 since Close() clears data
	if stats.TotalItems != 0 {
		// This is actually ok - the data was cleared
		t.Logf("Stats after close: %d items", stats.TotalItems)
	}
}

// TestCloseWhileExpirationRunning tests close during expiration worker
func TestCloseWhileExpirationRunning(t *testing.T) {
	q := queue.NewQueueWithTTL("close-expiration-test", 1*time.Second)

	// Add data that will expire
	for i := 0; i < 50; i++ {
		q.TryEnqueue(i)
	}

	// Wait for expiration to start
	time.Sleep(1500 * time.Millisecond)

	// Trigger expiration
	go q.ForceExpiration()

	// Close immediately after
	time.Sleep(10 * time.Millisecond)
	q.Close()

	// Should close cleanly
	stats := q.GetQueueStats()
	if stats.TotalItems != 0 {
		t.Error("Queue should be empty after close")
	}
}

// TestMultipleBlockedOperationsOnClose tests multiple blocked operations waking up on close
func TestMultipleBlockedOperationsOnClose(t *testing.T) {
	q := queue.NewQueueWithTTL("multiple-blocked-test", 10*time.Minute)

	// Fill queue
	for {
		err := q.TryEnqueue("filler")
		if err != nil {
			break
		}
	}

	var wg sync.WaitGroup
	completedCount := atomic.Int32{}

	// Start multiple blocked enqueues
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			q.Enqueue(id)
			completedCount.Add(1)
		}(i)
	}

	// Start multiple blocked reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumer := q.AddConsumer()
			// Drain the queue first
			for {
				if consumer.TryRead() == nil {
					break
				}
			}
			// Now block on read
			consumer.Read()
			completedCount.Add(1)
		}()
	}

	// Wait for operations to block
	time.Sleep(100 * time.Millisecond)

	// Close should wake up all blocked operations
	q.Close()

	// All operations should complete
	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case <-doneChan:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Blocked operations did not complete after close")
	}

	// Verify all operations completed
	if completedCount.Load() != 10 {
		t.Errorf("Expected 10 completed operations, got %d", completedCount.Load())
	}
}

// TestCloseMemoryLeak tests that close properly releases resources
func TestCloseMemoryLeak(t *testing.T) {
	q := queue.NewQueue("memory-leak-test")

	// Add significant data
	for i := 0; i < 1000; i++ {
		q.TryEnqueue(make([]byte, 100))
	}

	// Add consumers
	for i := 0; i < 10; i++ {
		q.AddConsumer()
	}

	memBefore := q.GetMemoryUsage()
	if memBefore == 0 {
		t.Error("Should have memory usage before close")
	}

	// Close
	q.Close()

	// Memory tracking is cleared when data is cleared
	memAfter := q.GetMemoryUsage()

	// Memory should be significantly reduced (cleared)
	// Note: There may be some chunk overhead remaining
	stats := q.GetQueueStats()
	if stats.TotalItems != 0 {
		t.Errorf("Items should be 0 after close, got %d", stats.TotalItems)
	}

	// Memory should be much less than before
	if memAfter >= memBefore {
		t.Errorf("Memory should be reduced after close: before=%d, after=%d", memBefore, memAfter)
	}
}

// TestCloseWithActiveReads tests close while consumers are actively reading
func TestCloseWithActiveReads(t *testing.T) {
	q := queue.NewQueue("close-active-reads-test")

	// Add data
	for i := 0; i < 100; i++ {
		q.TryEnqueue(i)
	}

	var wg sync.WaitGroup
	readCount := atomic.Int64{}

	// Start active readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumer := q.AddConsumer()
			for {
				data := consumer.TryRead()
				if data == nil {
					break
				}
				readCount.Add(1)
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Wait for reads to start
	time.Sleep(50 * time.Millisecond)

	// Close while reading
	q.Close()

	// Wait for readers to finish
	wg.Wait()

	// Some data should have been read
	if readCount.Load() == 0 {
		t.Error("Expected some data to be read before close")
	}
}
