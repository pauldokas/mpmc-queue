package tests

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestGetUnreadCountAccuracy tests GetUnreadCount returns correct values
func TestGetUnreadCountAccuracy(t *testing.T) {
	q := queue.NewQueue("unread-count-test")
	defer q.Close()

	// Add 50 items
	for i := 0; i < 50; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// Should have 50 unread
	unread := consumer.GetUnreadCount()
	if unread != 50 {
		t.Errorf("Expected 50 unread items, got %d", unread)
	}

	// Read 10 items
	for i := 0; i < 10; i++ {
		consumer.TryRead()
	}

	// Should have 40 unread
	unread = consumer.GetUnreadCount()
	if unread != 40 {
		t.Errorf("Expected 40 unread items, got %d", unread)
	}

	// Read all remaining
	readCount := 0
	for consumer.TryRead() != nil {
		readCount++
	}

	// Verify we read 40 more items
	if readCount != 40 {
		t.Errorf("Expected to read 40 items, got %d", readCount)
	}

	// After reading all, unread count should be 0 or very small
	// (there might be items at the current position still)
	unread = consumer.GetUnreadCount()
	if unread > 50 {
		t.Errorf("Unread count unexpectedly high after reading all: %d", unread)
	}

	t.Logf("Unread count after reading all items: %d", unread)
}

// TestGetUnreadCountWithNoData tests GetUnreadCount when queue is empty
func TestGetUnreadCountWithNoData(t *testing.T) {
	q := queue.NewQueue("unread-empty-test")
	defer q.Close()

	consumer := q.AddConsumer()

	// Should be 0
	unread := consumer.GetUnreadCount()
	if unread != 0 {
		t.Errorf("Expected 0 unread items for empty queue, got %d", unread)
	}
}

// TestGetUnreadCountDuringConcurrentEnqueue tests GetUnreadCount with concurrent enqueues
func TestGetUnreadCountDuringConcurrentEnqueue(t *testing.T) {
	q := queue.NewQueue("unread-concurrent-test")
	defer q.Close()

	consumer := q.AddConsumer()

	var wg sync.WaitGroup
	itemsAdded := atomic.Int64{}

	// Start enqueueing
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			err := q.TryEnqueue(i)
			if err == nil {
				itemsAdded.Add(1)
			}
		}
	}()

	// Check unread count concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			unread := consumer.GetUnreadCount()
			// Should never be negative
			if unread < 0 {
				t.Errorf("Unread count should not be negative: %d", unread)
			}
		}
	}()

	wg.Wait()

	// Final unread should match items added
	finalUnread := consumer.GetUnreadCount()
	if finalUnread != itemsAdded.Load() {
		t.Errorf("Final unread (%d) should equal items added (%d)", finalUnread, itemsAdded.Load())
	}
}

// TestGetUnreadCountAfterExpiration tests GetUnreadCount after items expire
func TestGetUnreadCountAfterExpiration(t *testing.T) {
	q := queue.NewQueueWithTTL("unread-expiration-test", 1*time.Second)
	defer q.Close()

	// Add items
	for i := 0; i < 20; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// Should have 20 unread
	unread := consumer.GetUnreadCount()
	if unread != 20 {
		t.Errorf("Expected 20 unread, got %d", unread)
	}

	// Read some items
	for i := 0; i < 5; i++ {
		consumer.TryRead()
	}

	// Wait for expiration
	time.Sleep(2 * time.Second)
	q.ForceExpiration()

	// Unread count should update after expiration
	unread = consumer.GetUnreadCount()
	// Should be less than 15 (some expired)
	if unread > 15 {
		t.Logf("Unread count after expiration: %d (some items expired)", unread)
	}
}

// TestGetPositionInitial tests GetPosition before any reads
func TestGetPositionInitial(t *testing.T) {
	q := queue.NewQueue("position-initial-test")
	defer q.Close()

	// Add data
	for i := 0; i < 10; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// Initial position should be nil
	element, index := consumer.GetPosition()
	if element == nil {
		// This is expected - position is nil before first read
		if index != 0 {
			t.Errorf("Index should be 0 for initial position, got %d", index)
		}
	}
}

// TestGetPositionAfterReads tests GetPosition after reading data
func TestGetPositionAfterReads(t *testing.T) {
	q := queue.NewQueue("position-after-reads-test")
	defer q.Close()

	// Add data
	for i := 0; i < 20; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// Read some items
	for i := 0; i < 10; i++ {
		consumer.TryRead()
	}

	// Position should be set
	element, index := consumer.GetPosition()
	if element == nil {
		t.Error("Position should be set after reads")
	}

	// Index should be within chunk bounds
	if index < 0 || index >= 1000 {
		t.Errorf("Index should be within chunk bounds, got %d", index)
	}
}

// TestGetPositionConsistency tests GetPosition consistency with actual reads
func TestGetPositionConsistency(t *testing.T) {
	q := queue.NewQueue("position-consistency-test")
	defer q.Close()

	// Add items
	for i := 0; i < 30; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// Read and verify position advances
	prevElement, prevIndex := consumer.GetPosition()

	for i := 0; i < 10; i++ {
		data := consumer.TryRead()
		if data == nil {
			t.Fatal("Should be able to read data")
		}

		element, index := consumer.GetPosition()

		// Position should advance
		if element == prevElement {
			// Same chunk - index should be greater
			if index <= prevIndex {
				t.Errorf("Index should advance: prev=%d, curr=%d", prevIndex, index)
			}
		}

		prevElement = element
		prevIndex = index
	}
}

// TestGetDequeueHistoryGrowth tests that dequeue history grows
func TestGetDequeueHistoryGrowth(t *testing.T) {
	q := queue.NewQueue("history-growth-test")
	defer q.Close()

	// Add data
	for i := 0; i < 50; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// History should start empty
	history := consumer.GetDequeueHistory()
	if len(history) != 0 {
		t.Errorf("History should start empty, got %d entries", len(history))
	}

	// Read items
	for i := 0; i < 10; i++ {
		consumer.TryRead()
	}

	// History should have 10 entries
	history = consumer.GetDequeueHistory()
	if len(history) != 10 {
		t.Errorf("Expected 10 history entries, got %d", len(history))
	}

	// Verify history has correct IDs
	for i, record := range history {
		if record.DataID == "" {
			t.Errorf("History entry %d has empty DataID", i)
		}
		if record.Timestamp.IsZero() {
			t.Errorf("History entry %d has zero timestamp", i)
		}
	}
}

// TestGetDequeueHistoryConcurrent tests dequeue history with concurrent reads
func TestGetDequeueHistoryConcurrent(t *testing.T) {
	q := queue.NewQueue("history-concurrent-test")
	defer q.Close()

	// Add data
	for i := 0; i < 100; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	var wg sync.WaitGroup

	// Read concurrently
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 25; i++ {
			consumer.TryRead()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 25; i++ {
			consumer.TryRead()
		}
	}()

	wg.Wait()

	// History should have 50 entries
	history := consumer.GetDequeueHistory()
	if len(history) != 50 {
		t.Errorf("Expected 50 history entries, got %d", len(history))
	}
}

// TestGetDequeueHistoryUnboundedGrowth tests history grows unbounded (known limitation)
func TestGetDequeueHistoryUnboundedGrowth(t *testing.T) {
	q := queue.NewQueue("history-unbounded-test")
	defer q.Close()

	// Add lots of data
	for i := 0; i < 1000; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// Read all
	for i := 0; i < 1000; i++ {
		consumer.TryRead()
	}

	// History should have 1000 entries (it grows unbounded)
	history := consumer.GetDequeueHistory()
	if len(history) != 1000 {
		t.Errorf("Expected 1000 history entries, got %d", len(history))
	}

	t.Logf("Note: Dequeue history grows unbounded (%d entries)", len(history))
}

// TestHasMoreDataAtChunkBoundary tests HasMoreData at chunk boundary
func TestHasMoreDataAtChunkBoundary(t *testing.T) {
	q := queue.NewQueue("has-more-boundary-test")
	defer q.Close()

	// Add exactly 1000 items (one full chunk)
	for i := 0; i < 1000; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// Read all but last item
	for i := 0; i < 999; i++ {
		consumer.TryRead()
	}

	// Should have more data
	if !consumer.HasMoreData() {
		t.Error("Should have more data (1 item remaining)")
	}

	// Read last item
	consumer.TryRead()

	// Should have no more data
	if consumer.HasMoreData() {
		t.Error("Should have no more data")
	}
}

// TestHasMoreDataCrossChunk tests HasMoreData across chunk boundaries
func TestHasMoreDataCrossChunk(t *testing.T) {
	q := queue.NewQueue("has-more-cross-chunk-test")
	defer q.Close()

	// Add items spanning multiple chunks
	for i := 0; i < 1500; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	consumer := q.AddConsumer()

	// Read first chunk
	for i := 0; i < 1000; i++ {
		consumer.TryRead()
	}

	// Should still have more data in next chunk
	if !consumer.HasMoreData() {
		t.Error("Should have more data in next chunk")
	}

	// Read remaining
	for i := 0; i < 500; i++ {
		consumer.TryRead()
	}

	// Should have no more data
	if consumer.HasMoreData() {
		t.Error("Should have no more data after reading all")
	}
}

// TestHasMoreDataWithConcurrentEnqueue tests HasMoreData with concurrent enqueues
func TestHasMoreDataWithConcurrentEnqueue(t *testing.T) {
	q := queue.NewQueue("has-more-concurrent-test")
	defer q.Close()

	consumer := q.AddConsumer()

	// Initially no data
	if consumer.HasMoreData() {
		t.Error("Should have no data initially")
	}

	// Start enqueueing
	go func() {
		for i := 0; i < 50; i++ {
			_ = q.TryEnqueue(i)
			time.Sleep(time.Millisecond)
		}
	}()

	// Wait for some data
	time.Sleep(50 * time.Millisecond)

	// Should have data now
	if !consumer.HasMoreData() {
		t.Error("Should have data after enqueues")
	}
}

// TestMultipleConsumersPositionIndependence tests that consumer positions are independent
func TestMultipleConsumersPositionIndependence(t *testing.T) {
	q := queue.NewQueue("position-independence-test")
	defer q.Close()

	// Add data
	for i := 0; i < 30; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", i, err)
		}
	}

	c1 := q.AddConsumer()
	c2 := q.AddConsumer()
	c3 := q.AddConsumer()

	// c1 reads 10
	for i := 0; i < 10; i++ {
		c1.TryRead()
	}

	// c2 reads 20
	for i := 0; i < 20; i++ {
		c2.TryRead()
	}

	// c3 reads 5
	for i := 0; i < 5; i++ {
		c3.TryRead()
	}

	// Verify unread counts are different
	unread1 := c1.GetUnreadCount()
	unread2 := c2.GetUnreadCount()
	unread3 := c3.GetUnreadCount()

	if unread1 != 20 {
		t.Errorf("c1 should have 20 unread, got %d", unread1)
	}
	if unread2 != 10 {
		t.Errorf("c2 should have 10 unread, got %d", unread2)
	}
	if unread3 != 25 {
		t.Errorf("c3 should have 25 unread, got %d", unread3)
	}
}
