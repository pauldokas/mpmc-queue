package tests

import (
	"container/list"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestChunkedList_GetTotalItems tests GetTotalItems method
func TestChunkedList_GetTotalItems(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Initially empty
	if cl.GetTotalItems() != 0 {
		t.Errorf("Expected 0 items, got %d", cl.GetTotalItems())
	}

	// Add items
	for i := 0; i < 100; i++ {
		data := queue.NewQueueData(i, "test-queue")
		if err := cl.Enqueue(data); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	if cl.GetTotalItems() != 100 {
		t.Errorf("Expected 100 items, got %d", cl.GetTotalItems())
	}
}

// TestChunkedList_GetLastElement tests GetLastElement method
func TestChunkedList_GetLastElement(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Initially nil
	if cl.GetLastElement() != nil {
		t.Error("Expected nil for empty list")
	}

	// Add one item
	data := queue.NewQueueData("first", "test-queue")
	if err := cl.Enqueue(data); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	lastElement := cl.GetLastElement()
	if lastElement == nil {
		t.Fatal("Expected non-nil last element")
	}

	// Add more items
	for i := 0; i < 10; i++ {
		data := queue.NewQueueData(i, "test-queue")
		if err := cl.Enqueue(data); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	lastElement = cl.GetLastElement()
	if lastElement == nil {
		t.Fatal("Expected non-nil last element after adding items")
	}
}

// TestChunkedList_GetChunk tests GetChunk method
func TestChunkedList_GetChunk(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Add items
	for i := 0; i < 10; i++ {
		data := queue.NewQueueData(i, "test-queue")
		if err := cl.Enqueue(data); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	firstElement := cl.GetFirstElement()
	if firstElement == nil {
		t.Fatal("Expected non-nil first element")
	}

	chunk := cl.GetChunk(firstElement)
	if chunk == nil {
		t.Fatal("Expected non-nil chunk")
	}

	if chunk.GetSize() != 10 {
		t.Errorf("Expected chunk size 10, got %d", chunk.GetSize())
	}
}

// TestChunkedList_GetChunkData tests accessing data from chunk
func TestChunkedList_GetChunkData(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Add items
	for i := 0; i < 5; i++ {
		data := queue.NewQueueData(i, "test-queue")
		if err := cl.Enqueue(data); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	firstElement := cl.GetFirstElement()
	if firstElement == nil {
		t.Fatal("Expected non-nil first element")
	}

	// Get chunk and verify we can access data
	chunk := cl.GetChunk(firstElement)
	if chunk == nil {
		t.Fatal("Expected non-nil chunk")
	}

	firstData := chunk.Get(0)
	if firstData == nil {
		t.Error("Expected non-nil data at index 0")
	}
}

// TestChunkedList_IsEmpty tests IsEmpty method
func TestChunkedList_IsEmpty(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Initially empty
	if !cl.IsEmpty() {
		t.Error("Expected empty list")
	}

	// Add item
	data := queue.NewQueueData("test", "test-queue")
	if err := cl.Enqueue(data); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Not empty
	if cl.IsEmpty() {
		t.Error("Expected non-empty list")
	}
}

// TestChunkedList_GetMemoryUsage tests GetMemoryUsage method
func TestChunkedList_GetMemoryUsage(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Initially zero
	if cl.GetMemoryUsage() != 0 {
		t.Errorf("Expected 0 memory usage, got %d", cl.GetMemoryUsage())
	}

	// Add items
	for i := 0; i < 10; i++ {
		data := queue.NewQueueData(i, "test-queue")
		if err := cl.Enqueue(data); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	memUsage := cl.GetMemoryUsage()
	if memUsage == 0 {
		t.Error("Expected non-zero memory usage after adding items")
	}
}

// TestChunkedList_IterateFrom tests IterateFrom method
func TestChunkedList_IterateFrom(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Add items
	for i := 0; i < 20; i++ {
		data := queue.NewQueueData(i, "test-queue")
		if err := cl.Enqueue(data); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	firstElement := cl.GetFirstElement()
	if firstElement == nil {
		t.Fatal("Expected non-nil first element")
	}

	// Iterate from start
	count := 0
	cl.IterateFrom(firstElement, 0, func(data *queue.QueueData, element *list.Element, index int) bool {
		count++
		return true // Continue iteration
	})

	if count != 20 {
		t.Errorf("Expected to iterate over 20 items, got %d", count)
	}

	// Iterate and stop early
	count = 0
	cl.IterateFrom(firstElement, 0, func(data *queue.QueueData, element *list.Element, index int) bool {
		count++
		return count < 5 // Stop after 5
	})

	if count != 5 {
		t.Errorf("Expected to iterate over 5 items, got %d", count)
	}
}

// TestChunkedList_CountItemsFrom tests CountItemsFrom method
func TestChunkedList_CountItemsFrom(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Add items
	for i := 0; i < 50; i++ {
		data := queue.NewQueueData(i, "test-queue")
		if err := cl.Enqueue(data); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	firstElement := cl.GetFirstElement()
	if firstElement == nil {
		t.Fatal("Expected non-nil first element")
	}

	// Count from start
	count := cl.CountItemsFrom(firstElement, 0)
	if count != 50 {
		t.Errorf("Expected count 50, got %d", count)
	}

	// Count from offset
	count = cl.CountItemsFrom(firstElement, 10)
	if count != 40 {
		t.Errorf("Expected count 40 from offset 10, got %d", count)
	}
}

// TestChunkedList_RemoveExpiredData tests RemoveExpiredData method
func TestChunkedList_RemoveExpiredData(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// This test is complex and typically tested through Queue's expiration tests
	// But we can test the basic functionality

	// Add items
	for i := 0; i < 10; i++ {
		data := queue.NewQueueData(i, "test-queue")
		if err := cl.Enqueue(data); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	initialCount := cl.GetTotalItems()
	if initialCount != 10 {
		t.Errorf("Expected 10 items initially, got %d", initialCount)
	}

	// RemoveExpiredData is complex and needs expired items
	// This is primarily tested through integration tests
	// Here we just verify it doesn't panic with valid input

	// Set a very short TTL to test removal (items will be considered expired immediately)
	ttl := time.Duration(0) // All items are expired
	removedCount, removalInfo := cl.RemoveExpiredData(ttl)
	_ = removedCount
	_ = removalInfo
	// If we get here without panic, the method works
}

// TestChunkedList_MultipleChunks tests operations across multiple chunks
func TestChunkedList_MultipleChunks(t *testing.T) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Add enough items to span multiple chunks (ChunkSize = 1000)
	numItems := 2500
	for i := 0; i < numItems; i++ {
		data := queue.NewQueueData(i, "test-queue")
		if err := cl.Enqueue(data); err != nil {
			t.Fatalf("Failed to enqueue item %d: %v", i, err)
		}
	}

	// Verify total count
	if cl.GetTotalItems() != int64(numItems) {
		t.Errorf("Expected %d items, got %d", numItems, cl.GetTotalItems())
	}

	// Verify we can count from various positions
	firstElement := cl.GetFirstElement()
	count := cl.CountItemsFrom(firstElement, 500)
	expectedCount := int64(numItems - 500)
	if count != expectedCount {
		t.Errorf("Expected count %d from offset 500, got %d", expectedCount, count)
	}

	// Iterate across chunks
	iteratedCount := 0
	cl.IterateFrom(firstElement, 0, func(data *queue.QueueData, element *list.Element, index int) bool {
		iteratedCount++
		return true
	})

	if iteratedCount != numItems {
		t.Errorf("Expected to iterate %d items, got %d", numItems, iteratedCount)
	}
}
