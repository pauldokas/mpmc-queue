package tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"mpmc-queue/queue"
)

func TestEnqueueWithContext(t *testing.T) {
	q := queue.NewQueueWithTTL("test-context-enqueue", 10*time.Minute)
	defer q.Close()

	// Fill the queue to force blocking
	// We'll use a large payload to fill it quickly. Max memory is 1MB.
	largePayload := make([]byte, 100*1024) // 100KB
	for i := 0; i < 15; i++ {              // Should fill > 1MB
		_ = q.TryEnqueue(largePayload)
	}

	// Verify queue is full (TryEnqueue should fail)
	if err := q.TryEnqueue(largePayload); err == nil {
		t.Fatal("Queue should be full, but TryEnqueue succeeded")
	}

	// 1. Test immediate cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Use largePayload to ensure it doesn't fit
	err := q.EnqueueWithContext(ctx, largePayload)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}

	// 2. Test cancellation during wait
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = q.EnqueueWithContext(ctx, largePayload)
	duration := time.Since(start)

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}

	if duration < 100*time.Millisecond {
		t.Errorf("Returned too early: %v", duration)
	}
}

func TestReadWithContext(t *testing.T) {
	q := queue.NewQueue("test-context-read")
	defer q.Close()
	consumer := q.AddConsumer()

	// 1. Test immediate cancellation on empty queue
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	data, err := consumer.ReadWithContext(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
	if data != nil {
		t.Errorf("Expected nil data, got %v", data)
	}

	// 2. Test cancellation during wait
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	data, err = consumer.ReadWithContext(ctx)
	duration := time.Since(start)

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
	if data != nil {
		t.Errorf("Expected nil data, got %v", data)
	}
	if duration < 100*time.Millisecond {
		t.Errorf("Returned too early: %v", duration)
	}

	// 3. Test success path
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = q.TryEnqueue("success")
	}()

	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	data, err = consumer.ReadWithContext(ctx)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if data == nil || data.Payload != "success" {
		t.Errorf("Expected 'success', got %v", data)
	}
}

func TestEnqueueBatchWithContext(t *testing.T) {
	q := queue.NewQueue("test-context-enqueue-batch")
	defer q.Close()

	// Fill queue
	largePayload := make([]byte, 100*1024)
	for i := 0; i < 15; i++ {
		_ = q.TryEnqueue(largePayload)
	}

	// Test cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Use large payloads that won't fit
	payloads := []any{largePayload, largePayload}
	err := q.EnqueueBatchWithContext(ctx, payloads)

	if err != context.DeadlineExceeded {

		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}

func TestReadBatchWithContext(t *testing.T) {
	q := queue.NewQueue("test-context-read-batch")
	defer q.Close()
	consumer := q.AddConsumer()

	// 1. Test timeout on empty queue
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	batch, err := consumer.ReadBatchWithContext(ctx, 5)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("Expected empty batch, got %d items", len(batch))
	}

	// 2. Test partial read before timeout
	// Since ReadBatchWithContext doesn't block for full batch (only first item),
	// we need to ensure it gets at least one item
	if err := q.TryEnqueue("item1"); err != nil {
		t.Fatalf("Failed to enqueue item1: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	batch, err = consumer.ReadBatchWithContext(ctx, 5)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if len(batch) != 1 {
		t.Errorf("Expected 1 item, got %d", len(batch))
	}
}

func TestContextCancellationConcurrent(t *testing.T) {
	q := queue.NewQueue("test-concurrent-context")
	defer q.Close()
	consumer := q.AddConsumer()

	var wg sync.WaitGroup
	wg.Add(10)

	// Launch multiple readers
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			_, _ = consumer.ReadWithContext(ctx)
		}()
	}

	wg.Wait()
	// Just verify no panic/deadlock
}
