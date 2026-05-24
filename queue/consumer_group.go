package queue

import (
	"container/list"
	"sync"
)

// ConsumerGroup represents a group of consumers that share the work of processing the queue.
// Items are distributed among the consumers in the group.
type ConsumerGroup struct {
	name           string
	queue          *Queue
	chunkElement   *list.Element // Current shared position
	indexInChunk   int           // Shared position within current chunk
	notificationCh chan int      // Notification of expired items for the group
	mutex          sync.Mutex
	consumers      []*Consumer // Track consumers in this group
}

// NewConsumerGroup creates a new consumer group
func NewConsumerGroup(name string, queue *Queue) *ConsumerGroup {
	return &ConsumerGroup{
		name:           name,
		queue:          queue,
		notificationCh: make(chan int, 100),
		consumers:      make([]*Consumer, 0),
	}
}

// GetName returns the group name
func (cg *ConsumerGroup) GetName() string {
	return cg.name
}

// NotifyExpiredItems notifies the group about expired items
func (cg *ConsumerGroup) NotifyExpiredItems(count int) {
	cg.mutex.Lock()
	defer cg.mutex.Unlock()

	select {
	case cg.notificationCh <- count:
		// Notification sent successfully
	default:
		// Channel full, drop notification
	}
}

// AddConsumer adds a new consumer to this group
func (cg *ConsumerGroup) AddConsumer() *Consumer {
	// Register with the queue's manager, but mark as part of this group
	consumer := cg.queue.consumers.AddConsumerToGroup(cg)

	cg.mutex.Lock()
	cg.consumers = append(cg.consumers, consumer)
	cg.mutex.Unlock()

	return consumer
}

// getPosition returns the group's current position safely
func (cg *ConsumerGroup) getPosition() (*list.Element, int) {
	cg.mutex.Lock()
	defer cg.mutex.Unlock()
	return cg.chunkElement, cg.indexInChunk
}

// TryRead attempts to read the next item for the group
// This is called by consumers belonging to the group
func (cg *ConsumerGroup) TryRead() *QueueData {
	// Logic is very similar to Consumer.TryRead but acts on group state

	// Initialize position if this is the first read
	cg.mutex.Lock()
	needsInit := cg.chunkElement == nil
	cg.mutex.Unlock()

	if needsInit {
		cg.queue.mutex.RLock()
		firstElement := cg.queue.data.GetFirstElement()
		cg.queue.mutex.RUnlock()

		if firstElement == nil {
			return nil // No data available
		}

		cg.mutex.Lock()
		// Double check inside lock
		if cg.chunkElement == nil {
			cg.chunkElement = firstElement
			cg.indexInChunk = 0
		}
		cg.mutex.Unlock()
	}

	for {
		cg.queue.mutex.RLock()

		cg.mutex.Lock()
		currentElement := cg.chunkElement
		currentIndex := cg.indexInChunk
		cg.mutex.Unlock()

		if currentElement == nil {
			cg.queue.mutex.RUnlock()
			return nil
		}

		// Access chunk safely with read lock
		chunk := currentElement.Value.(*ChunkNode)
		chunkSize := chunk.GetSize()

		if currentIndex < chunkSize {
			// Read data while holding queue lock
			data := chunk.Get(currentIndex)
			cg.queue.mutex.RUnlock()

			if data != nil {
				// Record dequeue and update position atomically
				cg.mutex.Lock()
				if cg.chunkElement == currentElement && cg.indexInChunk == currentIndex {
					cg.indexInChunk++
					cg.mutex.Unlock()
					return data
				}
				cg.mutex.Unlock()
				// Position changed, retry
				continue
			}

			// Skip nil items
			cg.mutex.Lock()
			if cg.chunkElement == currentElement && cg.indexInChunk == currentIndex {
				cg.indexInChunk++
			}
			cg.mutex.Unlock()
		} else {
			// Move to next chunk
			nextElement := currentElement.Next()
			cg.queue.mutex.RUnlock()

			cg.mutex.Lock()
			if cg.chunkElement == currentElement && cg.indexInChunk == currentIndex {
				if nextElement != nil {
					cg.chunkElement = nextElement
					cg.indexInChunk = 0
				}
			}
			cg.mutex.Unlock()

			if nextElement == nil {
				return nil
			}
		}
	}
}

// UpdatePositionAfterExpiration updates the group's position after items are expired
func (cg *ConsumerGroup) UpdatePositionAfterExpiration(expiredCount int, newFirstElement *list.Element, removalInfo []ChunkRemovalInfo) {
	cg.mutex.Lock()
	defer cg.mutex.Unlock()

	if cg.chunkElement == nil || expiredCount == 0 {
		return
	}

	for _, info := range removalInfo {
		if info.Element == cg.chunkElement {
			cg.indexInChunk -= info.RemovedCount
			if cg.indexInChunk < 0 {
				cg.indexInChunk = 0
			}
			break
		}
	}

	if newFirstElement != nil {
		currentElement := cg.chunkElement
		stillValid := false
		for element := newFirstElement; element != nil; element = element.Next() {
			if element == currentElement {
				stillValid = true
				break
			}
		}

		if !stillValid {
			cg.chunkElement = newFirstElement
			cg.indexInChunk = 0
		}
	} else {
		cg.chunkElement = nil
		cg.indexInChunk = 0
	}
}

// TryReadWhere is not supported for ConsumerGroups as it permanently consumes non-matching items
// from the shared cursor, robbing other consumers of data.
func (cg *ConsumerGroup) TryReadWhere(predicate func(*QueueData) bool) *QueueData {
	// Restricting filtering logic on consumer groups to prevent data loss for other consumers.
	return nil
}

// HasMoreData checks if the group has more data
func (cg *ConsumerGroup) HasMoreData() bool {
	cg.queue.mutex.RLock()
	defer cg.queue.mutex.RUnlock()

	cg.mutex.Lock()
	chunkElement := cg.chunkElement
	indexInChunk := cg.indexInChunk
	cg.mutex.Unlock()

	if chunkElement == nil {
		return !cg.queue.data.IsEmpty()
	}

	// Check current chunk
	chunk := chunkElement.Value.(*ChunkNode)
	if indexInChunk < chunk.GetSize() {
		return true
	}

	// Check if there are more chunks
	return chunkElement.Next() != nil
}

// GetUnreadCount returns the number of unread items for the group
func (cg *ConsumerGroup) GetUnreadCount() int64 {
	cg.queue.mutex.RLock()
	defer cg.queue.mutex.RUnlock()

	cg.mutex.Lock()
	chunkElement := cg.chunkElement
	indexInChunk := cg.indexInChunk
	cg.mutex.Unlock()

	if chunkElement == nil {
		return cg.queue.data.GetTotalItems()
	}

	return cg.queue.data.CountItemsFrom(chunkElement, indexInChunk)
}
