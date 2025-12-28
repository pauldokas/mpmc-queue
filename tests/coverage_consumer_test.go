package tests

import (
	"context"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestConsumer_GetNotificationChannel tests GetNotificationChannel method
func TestConsumer_GetNotificationChannel(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()
	ch := consumer.GetNotificationChannel()

	if ch == nil {
		t.Error("Expected non-nil notification channel")
	}

	// Verify we can access the channel (type check)
	// Don't actually read from it as it would block
	var _ = ch // Type assertion: it's a receive-only channel
}

// TestConsumer_GetPosition tests GetPosition method
func TestConsumer_GetPosition(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Initial position (nil for new consumer)
	element, index := consumer.GetPosition()
	if element != nil {
		t.Error("Expected nil element for new consumer")
	}
	if index != 0 {
		t.Errorf("Expected index 0, got %d", index)
	}

	// Add item and read
	if err := q.TryEnqueue("test"); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	consumer.TryRead()

	// Position should have advanced
	element, _ = consumer.GetPosition()
	if element == nil {
		t.Error("Expected non-nil element after read")
	}
}

// TestConsumer_ReadWithContext tests ReadWithContext method
func TestConsumer_ReadWithContext(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Test timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	data, err := consumer.ReadWithContext(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
	if data != nil {
		t.Error("Expected nil data on timeout")
	}

	// Test successful read
	if err := q.TryEnqueue("test-data"); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel2()

	data, err = consumer.ReadWithContext(ctx2)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if data == nil {
		t.Error("Expected data, got nil")
	}
}

// TestConsumer_TryReadBatch tests TryReadBatch method
func TestConsumer_TryReadBatch(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Test empty queue
	consumer := q.AddConsumer()
	batch := consumer.TryReadBatch(5)
	if len(batch) != 0 {
		t.Errorf("Expected empty batch from empty queue, got %d items", len(batch))
	}

	// Enqueue items
	for i := 0; i < 10; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Read batch of 5
	batch = consumer.TryReadBatch(5)
	if len(batch) == 0 {
		t.Fatal("Expected non-empty batch")
	}
	if len(batch) > 5 {
		t.Errorf("Expected at most 5 items, got %d", len(batch))
	}

	// Read batch of 20 (more than available)
	batch = consumer.TryReadBatch(20)
	if len(batch) == 0 {
		t.Error("Expected some items in batch")
	}

	// Method works correctly - it returns available items up to limit
}

// TestConsumer_ReadBatch tests ReadBatch method (blocking)
func TestConsumer_ReadBatch(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Start goroutine to read batch (will block)
	resultChan := make(chan []*queue.QueueData, 1)
	go func() {
		batch := consumer.ReadBatch(5)
		resultChan <- batch
	}()

	// Give time to block
	time.Sleep(50 * time.Millisecond)

	// Enqueue items
	for i := 0; i < 3; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Should unblock with 3 items (less than requested)
	select {
	case batch := <-resultChan:
		if len(batch) < 1 {
			t.Error("Expected at least 1 item in batch")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ReadBatch did not unblock")
	}
}

// TestConsumer_ReadBatchWithContext tests ReadBatchWithContext method
func TestConsumer_ReadBatchWithContext(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Test timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	batch, err := consumer.ReadBatchWithContext(ctx, 5)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
	if len(batch) != 0 {
		t.Error("Expected empty batch on timeout")
	}

	// Test successful read
	for i := 0; i < 5; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel2()

	batch, err = consumer.ReadBatchWithContext(ctx2, 5)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(batch) != 5 {
		t.Errorf("Expected batch size 5, got %d", len(batch))
	}
}

// TestConsumer_HasMoreData tests HasMoreData method
func TestConsumer_HasMoreData(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Initially no data
	if consumer.HasMoreData() {
		t.Error("Expected false for empty queue")
	}

	// Add data
	if err := q.TryEnqueue("test"); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Should have data
	if !consumer.HasMoreData() {
		t.Error("Expected true after enqueue")
	}

	// Read data
	consumer.TryRead()

	// No more data
	if consumer.HasMoreData() {
		t.Error("Expected false after reading all data")
	}
}

// TestConsumer_GetUnreadCount tests GetUnreadCount method
func TestConsumer_GetUnreadCount(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Initially zero
	if consumer.GetUnreadCount() != 0 {
		t.Errorf("Expected 0 unread, got %d", consumer.GetUnreadCount())
	}

	// Add items
	for i := 0; i < 10; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Should be 10
	if consumer.GetUnreadCount() != 10 {
		t.Errorf("Expected 10 unread, got %d", consumer.GetUnreadCount())
	}

	// Read 3 items
	for i := 0; i < 3; i++ {
		consumer.TryRead()
	}

	// Should be 7
	if consumer.GetUnreadCount() != 7 {
		t.Errorf("Expected 7 unread, got %d", consumer.GetUnreadCount())
	}
}

// TestConsumer_GetStats tests GetStats method
func TestConsumer_GetStats(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Get initial stats
	stats := consumer.GetStats()
	if stats.ID == "" {
		t.Error("Expected non-empty ID")
	}
	if stats.TotalItemsRead != 0 {
		t.Error("Expected 0 items read initially")
	}
	if stats.UnreadItems != 0 {
		t.Error("Expected 0 unread items initially")
	}

	// Add and read items
	for i := 0; i < 5; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	for i := 0; i < 3; i++ {
		consumer.TryRead()
	}

	// Get stats again
	stats = consumer.GetStats()
	if stats.TotalItemsRead != 3 {
		t.Errorf("Expected 3 items read, got %d", stats.TotalItemsRead)
	}
	if stats.UnreadItems != 2 {
		t.Errorf("Expected 2 unread items, got %d", stats.UnreadItems)
	}
}

// TestConsumer_GetDequeueHistory tests GetDequeueHistory method
func TestConsumer_GetDequeueHistory(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Initially empty
	history := consumer.GetDequeueHistory()
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d records", len(history))
	}

	// Add and read items
	for i := 0; i < 5; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
		data := consumer.TryRead()
		if data == nil {
			t.Fatal("Expected data")
		}
	}

	// Check history
	history = consumer.GetDequeueHistory()
	if len(history) != 5 {
		t.Errorf("Expected 5 history records, got %d", len(history))
	}

	// Verify history has data IDs
	for i, record := range history {
		if record.DataID == "" {
			t.Errorf("Record %d has empty DataID", i)
		}
		if record.Timestamp.IsZero() {
			t.Errorf("Record %d has zero timestamp", i)
		}
	}
}

// TestConsumer_ReadWhere_Uncovered tests ReadWhere for coverage
func TestConsumer_ReadWhere_Uncovered(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	consumer := q.AddConsumer()

	// Start blocking read in goroutine
	resultChan := make(chan *queue.QueueData, 1)
	go func() {
		data := consumer.ReadWhere(func(d *queue.QueueData) bool {
			num, ok := d.Payload.(int)
			return ok && num == 42
		})
		resultChan <- data
	}()

	// Give time to block
	time.Sleep(50 * time.Millisecond)

	// Enqueue non-matching items
	for i := 0; i < 5; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Enqueue matching item
	if err := q.TryEnqueue(42); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Should find it
	select {
	case data := <-resultChan:
		if data == nil {
			t.Fatal("Expected data")
		}
		if data.Payload.(int) != 42 {
			t.Errorf("Expected 42, got %v", data.Payload)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ReadWhere did not return")
	}
}
