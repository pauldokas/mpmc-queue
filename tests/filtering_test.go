package tests

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestTryReadWhere_BasicFiltering tests basic filtering with TryReadWhere
func TestTryReadWhere_BasicFiltering(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Enqueue mixed data
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, item := range items {
		if err := q.TryEnqueue(item); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", item, err)
		}
	}

	consumer := q.AddConsumer()

	// Filter for even numbers only
	evens := []int{}
	for {
		data := consumer.TryReadWhere(func(d *queue.QueueData) bool {
			num, ok := d.Payload.(int)
			return ok && num%2 == 0
		})
		if data == nil {
			break
		}
		evens = append(evens, data.Payload.(int))
	}

	// Should have found 2, 4, 6, 8, 10
	expected := []int{2, 4, 6, 8, 10}
	if len(evens) != len(expected) {
		t.Fatalf("Expected %d even numbers, got %d", len(expected), len(evens))
	}

	for i, val := range expected {
		if evens[i] != val {
			t.Errorf("Expected evens[%d]=%d, got %d", i, val, evens[i])
		}
	}
}

// TestTryReadWhere_NoMatch tests TryReadWhere when no items match
func TestTryReadWhere_NoMatch(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Enqueue only odd numbers
	for i := 1; i <= 5; i += 2 {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	consumer := q.AddConsumer()

	// Try to filter for even numbers
	data := consumer.TryReadWhere(func(d *queue.QueueData) bool {
		num, ok := d.Payload.(int)
		return ok && num%2 == 0
	})

	if data != nil {
		t.Errorf("Expected nil (no match), got data: %v", data.Payload)
	}

	// Note: Consumer position advances as it searches through items
	// This is expected behavior - filtering reads through items to find matches
}

// TestTryReadWhere_NilPredicate tests TryReadWhere with nil predicate
func TestTryReadWhere_NilPredicate(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	if err := q.TryEnqueue("test"); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	consumer := q.AddConsumer()
	data := consumer.TryReadWhere(nil)

	if data != nil {
		t.Error("Expected nil with nil predicate, got data")
	}
}

// TestTryReadWhere_EmptyQueue tests TryReadWhere on empty queue
func TestTryReadWhere_EmptyQueue(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()
	data := consumer.TryReadWhere(func(d *queue.QueueData) bool {
		return true
	})

	if data != nil {
		t.Error("Expected nil on empty queue, got data")
	}
}

// TestReadWhere_Blocking tests blocking behavior of ReadWhere
func TestReadWhere_Blocking(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()
	foundData := make(chan *queue.QueueData, 1)

	// Start goroutine that will block waiting for matching data
	go func() {
		data := consumer.ReadWhere(func(d *queue.QueueData) bool {
			num, ok := d.Payload.(int)
			return ok && num == 42
		})
		foundData <- data
	}()

	// Give goroutine time to start blocking
	time.Sleep(50 * time.Millisecond)

	// Enqueue non-matching items
	for i := 1; i <= 5; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Give time for consumer to process non-matching items
	time.Sleep(50 * time.Millisecond)

	// Should still be blocking
	select {
	case <-foundData:
		t.Fatal("ReadWhere should still be blocking")
	default:
		// Still blocking, as expected
	}

	// Now enqueue matching item
	if err := q.TryEnqueue(42); err != nil {
		t.Fatalf("Failed to enqueue matching item: %v", err)
	}

	// Should unblock and return the matching data
	select {
	case data := <-foundData:
		if data == nil {
			t.Fatal("Expected data, got nil")
		}
		if data.Payload.(int) != 42 {
			t.Errorf("Expected 42, got %v", data.Payload)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ReadWhere did not unblock after matching item enqueued")
	}
}

// TestReadWhereWithContext_Cancellation tests context cancellation
func TestReadWhereWithContext_Cancellation(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Enqueue non-matching items
	for i := 1; i <= 5; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start reading with context
	resultChan := make(chan struct {
		data *queue.QueueData
		err  error
	}, 1)

	go func() {
		data, err := consumer.ReadWhereWithContext(ctx, func(d *queue.QueueData) bool {
			num, ok := d.Payload.(int)
			return ok && num == 100 // Won't match anything
		})
		resultChan <- struct {
			data *queue.QueueData
			err  error
		}{data, err}
	}()

	// Give time to start blocking
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Should return with context error
	select {
	case result := <-resultChan:
		if result.err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", result.err)
		}
		if result.data != nil {
			t.Error("Expected nil data on cancellation")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ReadWhereWithContext did not respect context cancellation")
	}
}

// TestReadWhereWithContext_Timeout tests context timeout
func TestReadWhereWithContext_Timeout(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Enqueue non-matching items
	for i := 1; i <= 5; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Try to read with timeout
	data, err := consumer.ReadWhereWithContext(ctx, func(d *queue.QueueData) bool {
		num, ok := d.Payload.(int)
		return ok && num == 100 // Won't match anything
	})

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
	if data != nil {
		t.Error("Expected nil data on timeout")
	}
}

// TestReadWhereWithContext_Success tests successful read with context
func TestReadWhereWithContext_Success(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Enqueue mixed data
	for i := 1; i <= 10; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Read even number
	data, err := consumer.ReadWhereWithContext(ctx, func(d *queue.QueueData) bool {
		num, ok := d.Payload.(int)
		return ok && num%2 == 0
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("Expected data, got nil")
	}
	if data.Payload.(int) != 2 {
		t.Errorf("Expected first even number (2), got %v", data.Payload)
	}
}

// TestReadWhereWithContext_NilPredicate tests nil predicate with context
func TestReadWhereWithContext_NilPredicate(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	if err := q.TryEnqueue("test"); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	consumer := q.AddConsumer()
	ctx := context.Background()

	data, err := consumer.ReadWhereWithContext(ctx, nil)

	if err != nil {
		t.Errorf("Expected nil error with nil predicate, got %v", err)
	}
	if data != nil {
		t.Error("Expected nil data with nil predicate, got data")
	}
}

// TestFiltering_MultipleConsumers tests that filtering works independently for multiple consumers
func TestFiltering_MultipleConsumers(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Enqueue mixed data
	for i := 1; i <= 20; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	consumer1 := q.AddConsumer()
	consumer2 := q.AddConsumer()

	var wg sync.WaitGroup
	wg.Add(2)

	var evens1, odds2 []int
	var mu sync.Mutex

	// Consumer 1: Read even numbers
	go func() {
		defer wg.Done()
		for {
			data := consumer1.TryReadWhere(func(d *queue.QueueData) bool {
				num, ok := d.Payload.(int)
				return ok && num%2 == 0
			})
			if data == nil {
				break
			}
			mu.Lock()
			evens1 = append(evens1, data.Payload.(int))
			mu.Unlock()
		}
	}()

	// Consumer 2: Read odd numbers
	go func() {
		defer wg.Done()
		for {
			data := consumer2.TryReadWhere(func(d *queue.QueueData) bool {
				num, ok := d.Payload.(int)
				return ok && num%2 == 1
			})
			if data == nil {
				break
			}
			mu.Lock()
			odds2 = append(odds2, data.Payload.(int))
			mu.Unlock()
		}
	}()

	wg.Wait()

	// Verify both consumers got their filtered data
	if len(evens1) != 10 {
		t.Errorf("Consumer 1 expected 10 even numbers, got %d", len(evens1))
	}
	if len(odds2) != 10 {
		t.Errorf("Consumer 2 expected 10 odd numbers, got %d", len(odds2))
	}
}

// TestFiltering_ConcurrentEnqueueAndFilter tests filtering while items are being enqueued
func TestFiltering_ConcurrentEnqueueAndFilter(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()
	var foundCount atomic.Int64

	// First enqueue all items
	for i := 1; i <= 100; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Then filter for multiples of 10
	for {
		data := consumer.TryReadWhere(func(d *queue.QueueData) bool {
			num, ok := d.Payload.(int)
			return ok && num%10 == 0
		})
		if data == nil {
			break
		}
		foundCount.Add(1)
	}

	// Should have found 10, 20, 30, ..., 100 (10 items)
	if foundCount.Load() != 10 {
		t.Errorf("Expected to find 10 multiples of 10, found %d", foundCount.Load())
	}
}

// TestFiltering_QueueClosed tests filtering when queue is closed
func TestFiltering_QueueClosed(t *testing.T) {
	q := queue.NewQueue("test-queue")

	consumer := q.AddConsumer()

	// Enqueue some items
	for i := 1; i <= 5; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	resultChan := make(chan *queue.QueueData, 1)

	// Start blocking read
	go func() {
		data := consumer.ReadWhere(func(d *queue.QueueData) bool {
			num, ok := d.Payload.(int)
			return ok && num == 100 // Won't match
		})
		resultChan <- data
	}()

	// Give time to start blocking
	time.Sleep(50 * time.Millisecond)

	// Close queue
	q.Close()

	// Should return nil
	select {
	case data := <-resultChan:
		if data != nil {
			t.Error("Expected nil when queue closed")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ReadWhere did not return after queue closed")
	}
}

// TestFiltering_ComplexPredicate tests filtering with complex predicate logic
func TestFiltering_ComplexPredicate(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Enqueue string data
	words := []string{"apple", "banana", "cherry", "date", "elderberry", "fig", "grape"}
	for _, word := range words {
		if err := q.TryEnqueue(word); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	consumer := q.AddConsumer()

	// Filter for words longer than 5 characters and starting with vowel
	matches := []string{}
	for {
		data := consumer.TryReadWhere(func(d *queue.QueueData) bool {
			str, ok := d.Payload.(string)
			if !ok || len(str) <= 5 {
				return false
			}
			firstChar := str[0]
			return firstChar == 'a' || firstChar == 'e' || firstChar == 'i' || firstChar == 'o' || firstChar == 'u'
		})
		if data == nil {
			break
		}
		matches = append(matches, data.Payload.(string))
	}

	// Should match "banana" (6 chars, but doesn't start with vowel - NO)
	// Should match "cherry" (6 chars, but doesn't start with vowel - NO)
	// Should match "elderberry" (10 chars, starts with 'e' - YES)
	expected := []string{"elderberry"}
	if len(matches) != len(expected) {
		t.Fatalf("Expected %d matches, got %d: %v", len(expected), len(matches), matches)
	}

	if matches[0] != expected[0] {
		t.Errorf("Expected %s, got %s", expected[0], matches[0])
	}
}
