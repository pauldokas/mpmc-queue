package tests

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mpmc-queue/queue"
)

func TestMaxItems_NonBlocking(t *testing.T) {
	q := queue.NewQueueWithConfig("test-max-items", queue.QueueConfig{
		MaxItems: 3,
	})
	defer q.Close()

	// Fill queue
	for i := 0; i < 3; i++ {
		err := q.TryEnqueue(i)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// 4th item should fail with QueueFullError
	err := q.TryEnqueue(3)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*queue.QueueFullError); !ok {
		t.Fatalf("expected QueueFullError, got %T", err)
	}

	// TryEnqueueBatch should also fail
	err = q.TryEnqueueBatch([]any{4, 5})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*queue.QueueFullError); !ok {
		t.Fatalf("expected QueueFullError, got %T", err)
	}
}

func TestMaxItems_Blocking(t *testing.T) {
	q := queue.NewQueueWithConfig("test-max-items-blocking", queue.QueueConfig{
		MaxItems: 2,
		TTL:      50 * time.Millisecond,
		ExpirationCheckInterval: 25 * time.Millisecond,
	})
	defer q.Close()

	// Fill queue
	_ = q.Enqueue(1)
	_ = q.Enqueue(2)

	var wg sync.WaitGroup
	wg.Add(1)

	var enqueued atomic.Bool
	go func() {
		defer wg.Done()
		// This should block until an item expires and is removed
		_ = q.Enqueue(3)
		enqueued.Store(true)
	}()

	time.Sleep(20 * time.Millisecond) // Give goroutine time to block
	if enqueued.Load() {
		t.Fatal("expected Enqueue to block")
	}

	wg.Wait()
	if !enqueued.Load() {
		t.Fatal("expected Enqueue to succeed after space was freed via expiration")
	}
}

func TestMaxItems_EnqueueBatchBlocking(t *testing.T) {
	q := queue.NewQueueWithConfig("test-max-items-batch", queue.QueueConfig{
		MaxItems: 3,
		TTL:      50 * time.Millisecond,
		ExpirationCheckInterval: 25 * time.Millisecond,
	})
	defer q.Close()

	_ = q.Enqueue(1)

	var wg sync.WaitGroup
	wg.Add(1)
	
	var enqueued atomic.Bool
	go func() {
		defer wg.Done()
		// Queue has 1 item, max is 3. We try to batch 3 items, which would make 4. This should block.
		_ = q.EnqueueBatch([]any{2, 3, 4})
		enqueued.Store(true)
	}()

	time.Sleep(20 * time.Millisecond)
	if enqueued.Load() {
		t.Fatal("expected EnqueueBatch to block")
	}

	wg.Wait()
	if !enqueued.Load() {
		t.Fatal("expected EnqueueBatch to succeed after space freed via expiration")
	}
}

func TestMaxItems_BatchTooLarge(t *testing.T) {
	q := queue.NewQueueWithConfig("test-max-items-batch-large", queue.QueueConfig{
		MaxItems: 3,
	})
	defer q.Close()

	// Trying to enqueue a batch larger than MaxItems should fail immediately without blocking
	err := q.EnqueueBatch([]any{1, 2, 3, 4})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*queue.QueueFullError); !ok {
		t.Fatalf("expected QueueFullError, got %T", err)
	}
}

func TestDoneChannel(t *testing.T) {
	q := queue.NewQueue("test-done")

	doneChan := q.Done()

	select {
	case <-doneChan:
		t.Fatal("done channel closed prematurely")
	default:
	}

	q.Close()

	select {
	case <-doneChan:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("done channel was not closed after Close()")
	}
}

func TestReadyChannel(t *testing.T) {
	q := queue.NewQueue("test-ready")
	defer q.Close()

	c := q.AddConsumer()

	select {
	case <-c.Ready():
		t.Fatal("ready channel signaled prematurely")
	default:
	}

	_ = q.TryEnqueue("hello")

	select {
	case <-c.Ready():
		// Expected
		data := c.TryRead()
		if data == nil || data.Payload != "hello" {
			t.Fatal("failed to read expected data after ready signal")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ready channel was not signaled after enqueue")
	}
}

func TestNativeChannel(t *testing.T) {
	q := queue.NewQueue("test-native")
	defer q.Close()

	c := q.AddConsumer()
	ch := c.NativeChannel()

	_ = q.TryEnqueue("item1")
	_ = q.TryEnqueue("item2")

	items := []string{}
	items = append(items, (<-ch).Payload.(string))
	items = append(items, (<-ch).Payload.(string))

	// Close queue to stop NativeChannel
	q.Close()

	// Ensure channel is closed eventually
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed, got item")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel not closed in time")
	}

	if len(items) != 2 || items[0] != "item1" || items[1] != "item2" {
		t.Fatalf("unexpected items received from NativeChannel: %v", items)
	}
}

func TestNativeChannel_ContextCancellation(t *testing.T) {
	q := queue.NewQueue("test-native-ctx")
	defer q.Close()

	c := q.AddConsumer()
	ch := c.NativeChannel()
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create another goroutine that just reads a bit then cancels the context to simulate an early exit scenario
	go func() {
		_ = q.TryEnqueue("item1")
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	received := 0
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				t.Fatal("channel closed before context cancelled")
			}
			if data.Payload == "item1" {
				received++
			}
		case <-ctx.Done():
			// Wait a bit to verify channel doesn't close on its own, it only closes when queue closes.
			time.Sleep(50 * time.Millisecond)
			if received != 1 {
				t.Fatalf("expected 1 item, got %d", received)
			}
			return
		}
	}
}
