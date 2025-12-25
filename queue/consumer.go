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
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Initialize position if this is the first read
	if c.chunkElement == nil {
		c.queue.mutex.RLock()
		firstElement := c.queue.data.GetFirstElement()
		c.queue.mutex.RUnlock()

		if firstElement == nil {
			return nil // No data available
		}

		c.chunkElement = firstElement
		c.indexInChunk = 0
	}

	// Try to read from current position
	for c.chunkElement != nil {
		// Access chunk safely with read lock
		c.queue.mutex.RLock()
		if c.chunkElement == nil {
			c.queue.mutex.RUnlock()
			return nil
		}
		chunk := c.chunkElement.Value.(*ChunkNode)
		chunkSize := chunk.GetSize()
		c.queue.mutex.RUnlock()

		if c.indexInChunk < chunkSize {
			data := chunk.Get(c.indexInChunk)
			if data != nil {
				// Record dequeue event locally (no modification to shared data)
				c.dequeueHistory = append(c.dequeueHistory, DequeueRecord{
					DataID:    data.ID,
					Timestamp: time.Now(),
				})

				// Move to next position
				c.indexInChunk++
				c.lastReadTime = time.Now()
				c.totalItemsRead++

				return data
			}
			// Skip nil items
			c.indexInChunk++
		} else {
			// Move to next chunk - need lock to safely navigate list
			c.queue.mutex.RLock()
			if c.chunkElement != nil {
				c.chunkElement = c.chunkElement.Next()
			}
			c.queue.mutex.RUnlock()
			c.indexInChunk = 0
		}
	}

	return nil // No more data available
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
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.chunkElement == nil {
		c.queue.mutex.RLock()
		hasData := !c.queue.data.IsEmpty()
		c.queue.mutex.RUnlock()
		return hasData
	}

	// Need to hold queue lock to safely access list structure
	c.queue.mutex.RLock()
	defer c.queue.mutex.RUnlock()

	if c.chunkElement == nil {
		return false
	}

	// Check current chunk
	chunk := c.chunkElement.Value.(*ChunkNode)
	if c.indexInChunk < chunk.GetSize() {
		return true
	}

	// Check if there are more chunks
	return c.chunkElement.Next() != nil
}

// GetUnreadCount returns the number of unread items for this consumer
func (c *Consumer) GetUnreadCount() int64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.chunkElement == nil {
		c.queue.mutex.RLock()
		totalItems := c.queue.data.GetTotalItems()
		c.queue.mutex.RUnlock()
		return totalItems
	}

	c.queue.mutex.RLock()
	unreadCount := c.queue.data.CountItemsFrom(c.chunkElement, c.indexInChunk)
	c.queue.mutex.RUnlock()

	return unreadCount
}

// GetStats returns consumer statistics
func (c *Consumer) GetStats() ConsumerStats {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Calculate unread count without additional locking
	var unreadCount int64
	if c.chunkElement == nil {
		c.queue.mutex.RLock()
		unreadCount = c.queue.data.GetTotalItems()
		c.queue.mutex.RUnlock()
	} else {
		c.queue.mutex.RLock()
		unreadCount = c.queue.data.CountItemsFrom(c.chunkElement, c.indexInChunk)
		c.queue.mutex.RUnlock()
	}

	return ConsumerStats{
		ID:             c.id,
		TotalItemsRead: c.totalItemsRead,
		UnreadItems:    unreadCount,
		LastReadTime:   c.lastReadTime,
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
func (c *Consumer) UpdatePositionAfterExpiration(expiredCount int, newFirstElement *list.Element) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.chunkElement == nil || expiredCount == 0 {
		return
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
func (cm *ConsumerManager) NotifyAllConsumersOfExpiration(expiredCounts map[string]int, newFirstElement *list.Element) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	for consumerID, consumer := range cm.consumers {
		if expiredCount, hasExpired := expiredCounts[consumerID]; hasExpired && expiredCount > 0 {
			consumer.NotifyExpiredItems(expiredCount)
			consumer.UpdatePositionAfterExpiration(expiredCount, newFirstElement)
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
