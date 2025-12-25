package queue

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// DefaultTTL is the default time-to-live for queue items (10 minutes)
	DefaultTTL = 10 * time.Minute

	// ExpirationCheckInterval is how often to check for expired items
	ExpirationCheckInterval = 30 * time.Second
)

// Queue represents a multi-producer, multi-consumer queue
type Queue struct {
	name              string
	data              *ChunkedList
	consumers         *ConsumerManager
	mutex             sync.RWMutex
	memoryTracker     *MemoryTracker
	ttl               time.Duration
	stopChan          chan struct{}
	wg                sync.WaitGroup
	expirationEnabled bool
	createdAt         time.Time
}

// NewQueue creates a new queue with the specified name
func NewQueue(name string) *Queue {
	memoryTracker := NewMemoryTracker()

	queue := &Queue{
		name:              name,
		data:              NewChunkedList(memoryTracker),
		memoryTracker:     memoryTracker,
		ttl:               DefaultTTL,
		stopChan:          make(chan struct{}),
		expirationEnabled: true,
		createdAt:         time.Now(),
	}

	queue.consumers = NewConsumerManager(queue)

	// Start expiration background task
	queue.wg.Add(1)
	go queue.expirationWorker()

	return queue
}

// NewQueueWithTTL creates a new queue with a custom TTL
func NewQueueWithTTL(name string, ttl time.Duration) *Queue {
	queue := NewQueue(name)
	queue.ttl = ttl
	return queue
}

// GetName returns the queue's name
func (q *Queue) GetName() string {
	return q.name
}

// Enqueue adds data to the queue
func (q *Queue) Enqueue(payload any) error {
	data := NewQueueData(payload, q.name)

	q.mutex.Lock()
	defer q.mutex.Unlock()

	return q.data.Enqueue(data)
}

// EnqueueBatch adds multiple items to the queue
func (q *Queue) EnqueueBatch(payloads []any) error {
	if len(payloads) == 0 {
		return nil
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	// Pre-validate that all items can be added
	dataItems := make([]*QueueData, len(payloads))
	for i, payload := range payloads {
		data := NewQueueData(payload, q.name)
		dataItems[i] = data

		if !q.memoryTracker.CanAddData(data) {
			return &MemoryLimitError{
				Current: q.memoryTracker.GetMemoryUsage(),
				Max:     MaxQueueMemory,
				Needed:  q.memoryTracker.EstimateQueueDataSize(data),
			}
		}
	}

	// Add all items
	for _, data := range dataItems {
		if err := q.data.Enqueue(data); err != nil {
			return err
		}
	}

	return nil
}

// AddConsumer adds a new consumer to the queue
func (q *Queue) AddConsumer() *Consumer {
	return q.consumers.AddConsumer()
}

// RemoveConsumer removes a consumer from the queue
func (q *Queue) RemoveConsumer(consumerID string) bool {
	return q.consumers.RemoveConsumer(consumerID)
}

// GetConsumer returns a consumer by ID
func (q *Queue) GetConsumer(consumerID string) *Consumer {
	return q.consumers.GetConsumer(consumerID)
}

// GetAllConsumers returns all active consumers
func (q *Queue) GetAllConsumers() []*Consumer {
	return q.consumers.GetAllConsumers()
}

// GetQueueStats returns queue statistics
func (q *Queue) GetQueueStats() QueueStats {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return QueueStats{
		Name:          q.name,
		TotalItems:    q.data.GetTotalItems(),
		MemoryUsage:   q.memoryTracker.GetMemoryUsage(),
		MemoryPercent: q.memoryTracker.GetMemoryUsagePercent(),
		ConsumerCount: q.consumers.GetConsumerCount(),
		CreatedAt:     q.createdAt,
		TTL:           q.ttl,
	}
}

// GetConsumerStats returns statistics for all consumers
func (q *Queue) GetConsumerStats() []ConsumerStats {
	return q.consumers.GetConsumerStats()
}

// IsEmpty returns true if the queue has no items
func (q *Queue) IsEmpty() bool {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.data.IsEmpty()
}

// GetMemoryUsage returns current memory usage
func (q *Queue) GetMemoryUsage() int64 {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.memoryTracker.GetMemoryUsage()
}

// SetTTL sets the time-to-live for queue items
func (q *Queue) SetTTL(ttl time.Duration) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.ttl = ttl
}

// GetTTL returns the current TTL setting
func (q *Queue) GetTTL() time.Duration {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.ttl
}

// EnableExpiration enables automatic expiration of items
func (q *Queue) EnableExpiration() {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.expirationEnabled = true
}

// DisableExpiration disables automatic expiration of items
func (q *Queue) DisableExpiration() {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.expirationEnabled = false
}

// ForceExpiration manually triggers expiration cleanup
func (q *Queue) ForceExpiration() int {
	return q.cleanupExpiredItems()
}

// Close closes the queue and cleans up resources
func (q *Queue) Close() {
	close(q.stopChan)
	q.wg.Wait()

	// Close all consumers
	for _, consumer := range q.consumers.GetAllConsumers() {
		consumer.Close()
	}

	// Clear queue data
	q.mutex.Lock()
	q.data.Clear()
	q.mutex.Unlock()
}

// CloseWithContext closes the queue with a context for timeout
func (q *Queue) CloseWithContext(ctx context.Context) error {
	done := make(chan struct{})

	go func() {
		q.Close()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// expirationWorker runs in the background to clean up expired items
func (q *Queue) expirationWorker() {
	defer q.wg.Done()

	ticker := time.NewTicker(ExpirationCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if q.expirationEnabled {
				q.cleanupExpiredItems()
			}
		case <-q.stopChan:
			return
		}
	}
}

// cleanupExpiredItems removes expired items and notifies consumers
func (q *Queue) cleanupExpiredItems() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// Check if expiration is enabled
	if !q.expirationEnabled {
		return 0
	}

	// Calculate expired counts for each consumer before cleanup
	expiredCounts := q.calculateExpiredCountsPerConsumer()

	// Remove expired items
	expiredCount, removalInfo := q.data.RemoveExpiredData(q.ttl)

	if expiredCount > 0 {
		// Notify consumers about expired items
		newFirstElement := q.data.GetFirstElement()
		q.consumers.NotifyAllConsumersOfExpiration(expiredCounts, newFirstElement, removalInfo)
	}

	return expiredCount
}

// calculateExpiredCountsPerConsumer calculates how many unread items will be expired for each consumer
func (q *Queue) calculateExpiredCountsPerConsumer() map[string]int {
	expiredCounts := make(map[string]int)

	consumers := q.consumers.GetAllConsumers()

	for _, consumer := range consumers {
		chunkElement, indexInChunk := consumer.GetPosition()

		if chunkElement == nil {
			// Consumer hasn't started reading, count all expired items
			expiredCount := q.countExpiredItemsFromBeginning()
			expiredCounts[consumer.GetID()] = expiredCount
		} else {
			// Count expired items from consumer's current position
			expiredCount := q.countExpiredItemsFromPosition(chunkElement, indexInChunk)
			expiredCounts[consumer.GetID()] = expiredCount
		}
	}

	return expiredCounts
}

// countExpiredItemsFromBeginning counts expired items from the beginning of the queue
func (q *Queue) countExpiredItemsFromBeginning() int {
	count := 0
	element := q.data.GetFirstElement()

	for element != nil {
		chunk := q.data.GetChunk(element)
		chunkSize := chunk.GetSize()
		for i := 0; i < chunkSize; i++ {
			data := chunk.Get(i)
			if data != nil && data.IsExpired(q.ttl) {
				count++
			} else {
				return count // Items are ordered by creation time
			}
		}
		element = element.Next()
	}

	return count
}

// countExpiredItemsFromPosition counts expired items from a specific position
func (q *Queue) countExpiredItemsFromPosition(startElement *list.Element, startIndex int) int {
	count := 0
	element := startElement
	index := startIndex

	for element != nil {
		chunk := q.data.GetChunk(element)
		chunkSize := chunk.GetSize()

		for i := index; i < chunkSize; i++ {
			data := chunk.Get(i)
			if data != nil && data.IsExpired(q.ttl) {
				count++
			} else {
				return count // Items are ordered by creation time
			}
		}

		element = element.Next()
		index = 0 // Reset index for subsequent chunks
	}

	return count
}

// QueueStats represents queue statistics
type QueueStats struct {
	Name          string        `json:"name"`
	TotalItems    int64         `json:"total_items"`
	MemoryUsage   int64         `json:"memory_usage"`
	MemoryPercent float64       `json:"memory_percent"`
	ConsumerCount int           `json:"consumer_count"`
	CreatedAt     time.Time     `json:"created_at"`
	TTL           time.Duration `json:"ttl"`
}

// String returns a string representation of the queue stats
func (qs QueueStats) String() string {
	return fmt.Sprintf("Queue[%s]: items=%d, memory=%d bytes (%.1f%%), consumers=%d, ttl=%v",
		qs.Name, qs.TotalItems, qs.MemoryUsage, qs.MemoryPercent, qs.ConsumerCount, qs.TTL)
}
