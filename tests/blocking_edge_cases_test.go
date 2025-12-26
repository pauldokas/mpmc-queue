package tests

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestMultipleProducersBlockedOnFull tests multiple producers blocked waiting for space
func TestMultipleProducersBlockedOnFull(t *testing.T) {
	q := queue.NewQueueWithTTL("multiple-blocked-producers-test", 2*time.Second)
	defer q.Close()

	// Fill queue
	for {
		err := q.TryEnqueue("filler")
		if err != nil {
			break
		}
	}

	completedCount := atomic.Int32{}
	var wg sync.WaitGroup

	// Start 5 producers that will block
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := q.Enqueue(id)
			if err == nil {
				completedCount.Add(1)
			}
		}(i)
	}

	// All should be blocked
	time.Sleep(200 * time.Millisecond)
	if completedCount.Load() > 0 {
		t.Error("Producers should be blocked")
	}

	// Trigger expiration to free space
	time.Sleep(2500 * time.Millisecond)
	expired := q.ForceExpiration()
	t.Logf("Expired %d items", expired)

	// Give producers time to wake up and complete
	time.Sleep(500 * time.Millisecond)

	// Use channel to wait with timeout
	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case <-doneChan:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Producers did not complete after expiration - possible notification issue")
	}

	// At least one should have completed
	if completedCount.Load() == 0 {
		t.Error("At least one producer should have completed")
	}

	t.Logf("Completed producers: %d/5", completedCount.Load())
}

// TestMultipleConsumersBlockedOnEmpty tests multiple consumers blocked waiting for data
func TestMultipleConsumersBlockedOnEmpty(t *testing.T) {
	q := queue.NewQueue("multiple-blocked-consumers-test")
	defer q.Close()

	completedCount := atomic.Int32{}
	var wg sync.WaitGroup

	// Start 5 consumers that will block
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumer := q.AddConsumer()
			data := consumer.Read()
			if data != nil {
				completedCount.Add(1)
			}
		}()
	}

	// All should be blocked
	time.Sleep(200 * time.Millisecond)
	if completedCount.Load() > 0 {
		t.Error("Consumers should be blocked")
	}

	// Add data
	for i := 0; i < 5; i++ {
		q.TryEnqueue(i)
		time.Sleep(50 * time.Millisecond)
	}

	wg.Wait()

	// All should have completed
	if completedCount.Load() != 5 {
		t.Errorf("Expected 5 completed consumers, got %d", completedCount.Load())
	}
}

// TestBlockingEnqueueInterruptedByClose tests blocked enqueue interrupted by close
func TestBlockingEnqueueInterruptedByClose(t *testing.T) {
	q := queue.NewQueueWithTTL("enqueue-interrupted-test", 10*time.Minute)

	// Fill queue
	for {
		err := q.TryEnqueue("filler")
		if err != nil {
			break
		}
	}

	var enqueueErr error
	var wg sync.WaitGroup
	wg.Add(1)

	// Start blocking enqueue
	go func() {
		defer wg.Done()
		enqueueErr = q.Enqueue("blocked")
	}()

	// Wait for block
	time.Sleep(100 * time.Millisecond)

	// Close queue
	q.Close()

	// Wait for completion
	wg.Wait()

	// Should have error
	if enqueueErr == nil {
		t.Error("Blocked enqueue should return error when interrupted by close")
	}

	if enqueueErr.Error() != "queue closed while waiting to enqueue" {
		t.Logf("Got error: %v", enqueueErr)
	}
}

// TestBlockingReadInterruptedByClose tests blocked read interrupted by close
func TestBlockingReadInterruptedByClose(t *testing.T) {
	q := queue.NewQueue("read-interrupted-test")

	consumer := q.AddConsumer()

	var readData *queue.QueueData
	var wg sync.WaitGroup
	wg.Add(1)

	// Start blocking read
	go func() {
		defer wg.Done()
		readData = consumer.Read()
	}()

	// Wait for block
	time.Sleep(100 * time.Millisecond)

	// Close queue
	q.Close()

	// Wait for completion
	wg.Wait()

	// Should return nil
	if readData != nil {
		t.Error("Blocked read should return nil when interrupted by close")
	}
}

// TestRaceBetweenBlockingAndClose tests race between blocking operation starting and close
func TestRaceBetweenBlockingAndClose(t *testing.T) {
	// This test verifies no panic or deadlock when close happens
	// at the same time as blocking operations
	for iteration := 0; iteration < 10; iteration++ {
		q := queue.NewQueue("race-blocking-close-test")

		var wg sync.WaitGroup

		// Start blocking read
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumer := q.AddConsumer()
			consumer.Read() // Will block
		}()

		// Close immediately
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond)
			q.Close()
		}()

		// Should complete without panic
		doneChan := make(chan struct{})
		go func() {
			wg.Wait()
			close(doneChan)
		}()

		select {
		case <-doneChan:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Race test timed out - possible deadlock")
		}
	}
}

// TestBlockingReadBatchInterruptedByClose tests blocked ReadBatch interrupted by close
func TestBlockingReadBatchInterruptedByClose(t *testing.T) {
	q := queue.NewQueue("readbatch-interrupted-test")

	consumer := q.AddConsumer()

	var batch []*queue.QueueData
	var wg sync.WaitGroup
	wg.Add(1)

	// Start blocking ReadBatch
	go func() {
		defer wg.Done()
		batch = consumer.ReadBatch(10)
	}()

	// Wait for block
	time.Sleep(100 * time.Millisecond)

	// Close queue
	q.Close()

	// Wait for completion
	wg.Wait()

	// Should return empty batch
	if len(batch) != 0 {
		t.Errorf("Blocked ReadBatch should return empty batch when interrupted, got %d items", len(batch))
	}
}

// TestBlockingEnqueueBatchInterruptedByClose tests blocked EnqueueBatch interrupted by close
func TestBlockingEnqueueBatchInterruptedByClose(t *testing.T) {
	q := queue.NewQueueWithTTL("enqueuebatch-interrupted-test", 10*time.Minute)

	// Fill queue
	for {
		err := q.TryEnqueue("filler")
		if err != nil {
			break
		}
	}

	var enqueueErr error
	var wg sync.WaitGroup
	wg.Add(1)

	// Start blocking EnqueueBatch
	go func() {
		defer wg.Done()
		batch := []any{"item1", "item2", "item3"}
		enqueueErr = q.EnqueueBatch(batch)
	}()

	// Wait for block
	time.Sleep(100 * time.Millisecond)

	// Close queue
	q.Close()

	// Wait for completion
	wg.Wait()

	// Should have error
	if enqueueErr == nil {
		t.Error("Blocked EnqueueBatch should return error when interrupted by close")
	}
}

// TestBlockingOperationsSpuriousWakeup tests handling of spurious wakeups
func TestBlockingOperationsSpuriousWakeup(t *testing.T) {
	q := queue.NewQueueWithTTL("spurious-wakeup-test", 5*time.Second)
	defer q.Close()

	// Fill queue
	for {
		err := q.TryEnqueue("filler")
		if err != nil {
			break
		}
	}

	var enqueueCompleted atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Start blocking enqueue
	go func() {
		defer wg.Done()
		err := q.Enqueue("test")
		if err != nil {
			return
		}
		enqueueCompleted.Store(true)
	}()

	// Wait for block
	time.Sleep(100 * time.Millisecond)

	// Trigger spurious wakeup by forcing expiration (but nothing expires yet)
	q.ForceExpiration()

	// Should still be blocked (TTL is 5 seconds, items not expired)
	time.Sleep(100 * time.Millisecond)
	if enqueueCompleted.Load() {
		t.Error("Enqueue should still be blocked after spurious wakeup")
	}

	// Now actually expire items
	time.Sleep(6 * time.Second)
	q.ForceExpiration()
	time.Sleep(100 * time.Millisecond)

	wg.Wait()

	if !enqueueCompleted.Load() {
		t.Error("Enqueue should complete after real expiration")
	}
}

// TestConcurrentBlockingAndNonBlocking tests mixing blocking and non-blocking operations
func TestConcurrentBlockingAndNonBlocking(t *testing.T) {
	q := queue.NewQueue("mixed-blocking-test")
	defer q.Close()

	var wg sync.WaitGroup
	successCount := atomic.Int32{}

	// Add data continuously with TryEnqueue
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			err := q.TryEnqueue(i)
			if err == nil {
				successCount.Add(1)
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Add data with blocking Enqueue
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 100; i < 150; i++ {
			err := q.Enqueue(i)
			if err == nil {
				successCount.Add(1)
			}
		}
	}()

	// Read with TryRead
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer := q.AddConsumer()
		for i := 0; i < 50; i++ {
			consumer.TryRead()
			time.Sleep(2 * time.Millisecond)
		}
	}()

	// Read with blocking Read
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer := q.AddConsumer()
		for i := 0; i < 30; i++ {
			data := consumer.Read()
			if data == nil {
				break
			}
		}
	}()

	wg.Wait()

	// Some operations should have succeeded
	if successCount.Load() == 0 {
		t.Error("Expected some successful enqueues")
	}

	t.Logf("Successful enqueues: %d", successCount.Load())
}
