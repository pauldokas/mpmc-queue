package queue

import (
	"container/list"
	"fmt"
	"sync/atomic"
	"time"
)

// ChunkedList wraps container/list to provide chunked storage of QueueData
type ChunkedList struct {
	list          *list.List
	memoryTracker *MemoryTracker
	totalItems    atomic.Int64
}

// NewChunkedList creates a new chunked list
func NewChunkedList(memoryTracker *MemoryTracker) *ChunkedList {
	return &ChunkedList{
		list:          list.New(),
		memoryTracker: memoryTracker,
	}
}

// Enqueue adds data to the end of the list
func (cl *ChunkedList) Enqueue(data *QueueData) error {
	if !cl.memoryTracker.CanAddData(data) {
		return &MemoryLimitError{
			Current: cl.memoryTracker.GetMemoryUsage(),
			Max:     cl.memoryTracker.GetMaxMemory(),
			Needed:  cl.memoryTracker.EstimateQueueDataSize(data),
		}
	}

	// Get or create the last chunk
	var lastChunk *ChunkNode
	var lastElement *list.Element

	if cl.list.Len() == 0 {
		// Create first chunk
		lastChunk = NewChunkNode()
		lastElement = cl.list.PushBack(lastChunk)
		cl.memoryTracker.AddChunk()
	} else {
		lastElement = cl.list.Back()
		lastChunk = lastElement.Value.(*ChunkNode)

		if lastChunk.IsFull() {
			// Create new chunk
			lastChunk = NewChunkNode()
			lastElement = cl.list.PushBack(lastChunk)
			cl.memoryTracker.AddChunk()
		}
	}

	// Add data to the chunk
	if lastChunk.Add(data) {
		cl.memoryTracker.AddData(data)
		cl.totalItems.Add(1)
		return nil
	}

	return &QueueError{Message: "Failed to add data to chunk"}
}

// GetTotalItems returns the total number of items in all chunks
func (cl *ChunkedList) GetTotalItems() int64 {
	return cl.totalItems.Load()
}

// GetFirstElement returns the first element in the list
func (cl *ChunkedList) GetFirstElement() *list.Element {
	return cl.list.Front()
}

// GetLastElement returns the last element in the list
func (cl *ChunkedList) GetLastElement() *list.Element {
	return cl.list.Back()
}

// GetChunk returns the chunk from a list element
func (cl *ChunkedList) GetChunk(element *list.Element) *ChunkNode {
	if element == nil {
		return nil
	}
	return element.Value.(*ChunkNode)
}

// ChunkRemovalInfo tracks how many items were removed from each chunk
type ChunkRemovalInfo struct {
	Element      *list.Element
	RemovedCount int
}

// RemoveExpiredData removes expired data from all chunks
// Returns the total number of items removed and per-chunk removal info
func (cl *ChunkedList) RemoveExpiredData(ttl time.Duration) (int, []ChunkRemovalInfo) {
	totalRemoved := 0
	removalInfo := make([]ChunkRemovalInfo, 0)

	// Start from the front (oldest data)
	element := cl.list.Front()
	for element != nil {
		chunk := element.Value.(*ChunkNode)
		removedFromChunk, removedItems := chunk.RemoveExpired(ttl)

		if removedFromChunk > 0 {
			// Update memory tracking properly
			for _, data := range removedItems {
				cl.memoryTracker.RemoveData(data)
				cl.totalItems.Add(-1)
			}
			totalRemoved += removedFromChunk

			// Track removal info for this chunk (before it might be removed)
			removalInfo = append(removalInfo, ChunkRemovalInfo{
				Element:      element,
				RemovedCount: removedFromChunk,
			})
		}

		nextElement := element.Next()

		// Remove empty chunks
		if chunk.IsEmpty() {
			cl.list.Remove(element)
			cl.memoryTracker.RemoveChunk()
		}

		element = nextElement

		// If this chunk still has non-expired items, we can stop
		// (since items are ordered by creation time)
		if !chunk.IsEmpty() {
			earliestExpiry := chunk.GetEarliestExpiry()
			if earliestExpiry != nil && time.Since(*earliestExpiry) <= ttl {
				break
			}
		}
	}

	return totalRemoved, removalInfo
}

// IsEmpty returns true if the list has no items
func (cl *ChunkedList) IsEmpty() bool {
	return cl.totalItems.Load() == 0
}

// Clear removes all items from the list
func (cl *ChunkedList) Clear() {
	for cl.list.Len() > 0 {
		element := cl.list.Front()
		cl.list.Remove(element)
		cl.memoryTracker.RemoveChunk()
	}
	cl.totalItems.Store(0)
}

// GetMemoryUsage returns current memory usage
func (cl *ChunkedList) GetMemoryUsage() int64 {
	return cl.memoryTracker.GetMemoryUsage()
}

// IterateFrom allows iteration starting from a specific position
func (cl *ChunkedList) IterateFrom(element *list.Element, indexInChunk int, callback func(*QueueData, *list.Element, int) bool) {
	currentElement := element
	currentIndex := indexInChunk

	for currentElement != nil {
		chunk := currentElement.Value.(*ChunkNode)
		chunkSize := chunk.GetSize()

		for i := currentIndex; i < chunkSize; i++ {
			data := chunk.Get(i)
			if data != nil {
				if !callback(data, currentElement, i) {
					return
				}
			}
		}

		currentElement = currentElement.Next()
		currentIndex = 0 // Reset index for subsequent chunks
	}
}

// CountItemsFrom counts items from a specific position to the end
func (cl *ChunkedList) CountItemsFrom(element *list.Element, indexInChunk int) int64 {
	var count int64 = 0

	cl.IterateFrom(element, indexInChunk, func(data *QueueData, element *list.Element, index int) bool {
		count++
		return true
	})

	return count
}

// MemoryLimitError represents a memory limit exceeded error
type MemoryLimitError struct {
	Current int64
	Max     int64
	Needed  int64
}

func (e *MemoryLimitError) Error() string {
	return fmt.Sprintf("memory limit exceeded: current=%d, max=%d, needed=%d", e.Current, e.Max, e.Needed)
}

// QueueError represents a general queue error
type QueueError struct {
	Message string
}

func (e *QueueError) Error() string {
	return e.Message
}
