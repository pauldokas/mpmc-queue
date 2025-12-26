package queue

import (
	"container/list"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DequeueRecord represents a single dequeue event for a consumer
type DequeueRecord struct {
	DataID    string    `json:"data_id"`
	Timestamp time.Time `json:"timestamp"`
}

// Consumer represents a queue consumer with independent position tracking
type Consumer struct {
	id             string
	chunkElement   *list.Element // Current chunk position
	indexInChunk   int           // Position within current chunk
	notificationCh chan int      // Notification of expired items
	mutex          sync.Mutex
	queue          *Queue          // Reference to parent queue
	lastReadTime   time.Time       // For tracking consumer activity
	totalItemsRead int64           // Total items this consumer has read
	dequeueHistory []DequeueRecord // Track dequeue events locally
}

// NewConsumer creates a new consumer
func NewConsumer(queue *Queue) *Consumer {
	return &Consumer{
		id:             uuid.New().String(),
		chunkElement:   nil, // Will be set when first item is read
		indexInChunk:   0,
		notificationCh: make(chan int, 100), // Buffered channel for notifications
		queue:          queue,
		lastReadTime:   time.Now(),
		totalItemsRead: 0,
		dequeueHistory: make([]DequeueRecord, 0, 100), // Pre-allocate some capacity
	}
}

// GetID returns the consumer's unique identifier
func (c *Consumer) GetID() string {
	return c.id
}

// GetNotificationChannel returns the channel for expired item notifications
func (c *Consumer) GetNotificationChannel() <-chan int {
	return c.notificationCh
}

// GetPosition returns the consumer's current position
func (c *Consumer) GetPosition() (*list.Element, int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.chunkElement, c.indexInChunk
}

// getPositionUnsafe returns position without locking (caller must hold consumer lock)
func (c *Consumer) getPositionUnsafe() (*list.Element, int) {
	return c.chunkElement, c.indexInChunk
}

// SetPosition sets the consumer's position (used for initialization)
func (c *Consumer) SetPosition(element *list.Element, index int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.chunkElement = element
	c.indexInChunk = index
}

// Read reads the next available data item for this consumer
// Returns nil if no data is available
func (c *Consumer) Read() *QueueData {
	// Initialize position if this is the first read
	c.mutex.Lock()
	needsInit := c.chunkElement == nil
	c.mutex.Unlock()

	if needsInit {
		c.queue.mutex.RLock()
		firstElement := c.queue.data.GetFirstElement()
		c.queue.mutex.RUnlock()

		if firstElement == nil {
			return nil // No data available
		}

		c.mutex.Lock()
		c.chunkElement = firstElement
		c.indexInChunk = 0
		c.mutex.Unlock()
	}

	// Try to read from current position
	for {
		c.mutex.Lock()
		currentElement := c.chunkElement
		currentIndex := c.indexInChunk
		c.mutex.Unlock()

		if currentElement == nil {
			return nil
		}

		// Access chunk safely with read lock (no consumer lock held)
		// Keep queue lock held while reading data to prevent TOCTOU issues with expiration
		c.queue.mutex.RLock()
		if currentElement == nil {
			c.queue.mutex.RUnlock()
			return nil
		}
		chunk := currentElement.Value.(*ChunkNode)
		chunkSize := chunk.GetSize()

		if currentIndex < chunkSize {
			// Read data while holding queue lock to ensure consistency
			data := chunk.Get(currentIndex)
			c.queue.mutex.RUnlock()

			if data != nil {
				// Record dequeue and update position atomically
				c.mutex.Lock()
				// Double-check position hasn't changed (expiration could have updated it)
				if c.chunkElement == currentElement && c.indexInChunk == currentIndex {
					c.dequeueHistory = append(c.dequeueHistory, DequeueRecord{
						DataID:    data.ID,
						Timestamp: time.Now(),
					})
					c.indexInChunk++
					c.lastReadTime = time.Now()
					c.totalItemsRead++
					c.mutex.Unlock()
					return data
				}
				c.mutex.Unlock()
				// Position changed, retry from new position
				continue
			}
			// Skip nil items
			c.mutex.Lock()
			if c.chunkElement == currentElement && c.indexInChunk == currentIndex {
				c.indexInChunk++
			}
			c.mutex.Unlock()
		} else {
			// Move to next chunk - need lock to safely navigate list
			nextElement := currentElement.Next()
			c.queue.mutex.RUnlock()

			c.mutex.Lock()
			// Only update if position hasn't changed
			if c.chunkElement == currentElement && c.indexInChunk == currentIndex {
				c.chunkElement = nextElement
				c.indexInChunk = 0
			}
			c.mutex.Unlock()

			if nextElement == nil {
				return nil
			}
		}
	}
}

// ReadBatch reads multiple items up to the specified limit
func (c *Consumer) ReadBatch(limit int) []*QueueData {
	if limit <= 0 {
		return nil
	}

	batch := make([]*QueueData, 0, limit)

	for len(batch) < limit {
		data := c.Read()
		if data == nil {
			break // No more data available
		}
		batch = append(batch, data)
	}

	return batch
}

// HasMoreData checks if there's more data available for this consumer
func (c *Consumer) HasMoreData() bool {
	// Read position without holding lock to avoid lock ordering issues
	c.mutex.Lock()
	chunkElement := c.chunkElement
	indexInChunk := c.indexInChunk
	c.mutex.Unlock()

	if chunkElement == nil {
		c.queue.mutex.RLock()
		hasData := !c.queue.data.IsEmpty()
		c.queue.mutex.RUnlock()
		return hasData
	}

	// Need to hold queue lock to safely access list structure
	c.queue.mutex.RLock()
	defer c.queue.mutex.RUnlock()

	if chunkElement == nil {
		return false
	}

	// Check current chunk
	chunk := chunkElement.Value.(*ChunkNode)
	if indexInChunk < chunk.GetSize() {
		return true
	}

	// Check if there are more chunks
	return chunkElement.Next() != nil
}

// GetUnreadCount returns the number of unread items for this consumer
func (c *Consumer) GetUnreadCount() int64 {
	// Read position without holding lock to avoid lock ordering issues
	c.mutex.Lock()
	chunkElement := c.chunkElement
	indexInChunk := c.indexInChunk
	c.mutex.Unlock()

	if chunkElement == nil {
		c.queue.mutex.RLock()
		totalItems := c.queue.data.GetTotalItems()
		c.queue.mutex.RUnlock()
		return totalItems
	}

	c.queue.mutex.RLock()
	unreadCount := c.queue.data.CountItemsFrom(chunkElement, indexInChunk)
	c.queue.mutex.RUnlock()

	return unreadCount
}

// GetStats returns consumer statistics
func (c *Consumer) GetStats() ConsumerStats {
	// Read consumer state without holding lock to avoid lock ordering issues
	c.mutex.Lock()
	id := c.id
	totalItemsRead := c.totalItemsRead
	lastReadTime := c.lastReadTime
	chunkElement := c.chunkElement
	indexInChunk := c.indexInChunk
	c.mutex.Unlock()

	// Calculate unread count
	var unreadCount int64
	if chunkElement == nil {
		c.queue.mutex.RLock()
		unreadCount = c.queue.data.GetTotalItems()
		c.queue.mutex.RUnlock()
	} else {
		c.queue.mutex.RLock()
		unreadCount = c.queue.data.CountItemsFrom(chunkElement, indexInChunk)
		c.queue.mutex.RUnlock()
	}

	return ConsumerStats{
		ID:             id,
		TotalItemsRead: totalItemsRead,
		UnreadItems:    unreadCount,
		LastReadTime:   lastReadTime,
	}
}

// GetDequeueHistory returns a copy of the dequeue history for this consumer
func (c *Consumer) GetDequeueHistory() []DequeueRecord {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Return copy to prevent external modification
	history := make([]DequeueRecord, len(c.dequeueHistory))
	copy(history, c.dequeueHistory)
	return history
}

// NotifyExpiredItems notifies the consumer about expired items
func (c *Consumer) NotifyExpiredItems(count int) {
	select {
	case c.notificationCh <- count:
		// Notification sent successfully
	default:
		// Channel is full, could log this or handle differently
		// For now, we'll just drop the notification
	}
}

// UpdatePositionAfterExpiration updates the consumer's position after items are expired
// This is called by the queue when items are removed due to expiration
// NOTE: This must be called while holding queue.mutex to safely traverse the list
func (c *Consumer) UpdatePositionAfterExpiration(expiredCount int, newFirstElement *list.Element, removalInfo []ChunkRemovalInfo) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.chunkElement == nil || expiredCount == 0 {
		return
	}

	// Check if items were removed from the chunk we're currently reading
	for _, info := range removalInfo {
		if info.Element == c.chunkElement {
			// Items were removed from our current chunk, adjust our index
			// Since expired items are removed from the beginning and the chunk is compacted,
			// we need to shift our index back by the number of removed items
			c.indexInChunk -= info.RemovedCount

			// If our index is now negative, we're trying to read items that expired
			// Move to the beginning of the chunk (or next chunk if this one is empty)
			if c.indexInChunk < 0 {
				c.indexInChunk = 0
			}

			break
		}
	}

	// If the consumer is reading from expired chunks, update position
	if newFirstElement != nil {
		// Check if our current position is in an expired chunk
		currentElement := c.chunkElement
		stillValid := false

		// Walk through remaining chunks to see if our position is still valid
		// NOTE: We rely on the caller holding queue.mutex for safe list traversal
		for element := newFirstElement; element != nil; element = element.Next() {
			if element == currentElement {
				stillValid = true
				break
			}
		}

		if !stillValid {
			// Our position is in an expired chunk, move to the new first element
			c.chunkElement = newFirstElement
			c.indexInChunk = 0
		}
	}
}

// Close closes the consumer and cleans up resources
func (c *Consumer) Close() {
	close(c.notificationCh)
}

// ConsumerStats represents consumer statistics
type ConsumerStats struct {
	ID             string    `json:"id"`
	TotalItemsRead int64     `json:"total_items_read"`
	UnreadItems    int64     `json:"unread_items"`
	LastReadTime   time.Time `json:"last_read_time"`
}

// ConsumerManager manages multiple consumers for a queue
type ConsumerManager struct {
	consumers map[string]*Consumer
	mutex     sync.RWMutex
	queue     *Queue
}

// NewConsumerManager creates a new consumer manager
func NewConsumerManager(queue *Queue) *ConsumerManager {
	return &ConsumerManager{
		consumers: make(map[string]*Consumer),
		queue:     queue,
	}
}

// AddConsumer adds a new consumer to the queue
// New consumers start reading from the beginning of the queue
func (cm *ConsumerManager) AddConsumer() *Consumer {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	consumer := NewConsumer(cm.queue)

	// Initialize consumer to read from the beginning
	cm.queue.mutex.RLock()
	firstElement := cm.queue.data.GetFirstElement()
	cm.queue.mutex.RUnlock()

	if firstElement != nil {
		consumer.SetPosition(firstElement, 0)
	}

	cm.consumers[consumer.GetID()] = consumer

	return consumer
}

// RemoveConsumer removes a consumer from the queue
func (cm *ConsumerManager) RemoveConsumer(consumerID string) bool {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	consumer, exists := cm.consumers[consumerID]
	if !exists {
		return false
	}

	consumer.Close()
	delete(cm.consumers, consumerID)

	return true
}

// GetConsumer returns a consumer by ID
func (cm *ConsumerManager) GetConsumer(consumerID string) *Consumer {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	return cm.consumers[consumerID]
}

// GetAllConsumers returns all active consumers
func (cm *ConsumerManager) GetAllConsumers() []*Consumer {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	consumers := make([]*Consumer, 0, len(cm.consumers))
	for _, consumer := range cm.consumers {
		consumers = append(consumers, consumer)
	}

	return consumers
}

// NotifyAllConsumersOfExpiration notifies all consumers about expired items
func (cm *ConsumerManager) NotifyAllConsumersOfExpiration(expiredCounts map[string]int, newFirstElement *list.Element, removalInfo []ChunkRemovalInfo) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	// Calculate total expired items for position adjustment
	totalExpired := 0
	for _, info := range removalInfo {
		totalExpired += info.RemovedCount
	}

	for consumerID, consumer := range cm.consumers {
		// Notify consumers about their unread expired items
		if expiredCount, hasExpired := expiredCounts[consumerID]; hasExpired && expiredCount > 0 {
			consumer.NotifyExpiredItems(expiredCount)
		}

		// ALWAYS update position if any items expired (even if consumer already read past them)
		// This is necessary because chunk compaction affects all consumer positions
		if totalExpired > 0 {
			consumer.UpdatePositionAfterExpiration(totalExpired, newFirstElement, removalInfo)
		}
	}
}

// GetConsumerCount returns the number of active consumers
func (cm *ConsumerManager) GetConsumerCount() int {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	return len(cm.consumers)
}

// GetConsumerStats returns statistics for all consumers
func (cm *ConsumerManager) GetConsumerStats() []ConsumerStats {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	stats := make([]ConsumerStats, 0, len(cm.consumers))
	for _, consumer := range cm.consumers {
		stats = append(stats, consumer.GetStats())
	}

	return stats
}
