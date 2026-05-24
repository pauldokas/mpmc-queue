package queue

import (
	"container/list"
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// DefaultTTL is the default time-to-live for queue items (10 minutes)
	DefaultTTL = 10 * time.Minute

	// DefaultExpirationCheckInterval is the default interval for checking expired items
	DefaultExpirationCheckInterval = 30 * time.Second

	// ExpirationCheckInterval is how often to check for expired items
	// Deprecated: Use QueueConfig.ExpirationCheckInterval instead
	ExpirationCheckInterval = DefaultExpirationCheckInterval

	// ChunkSize is the number of items per chunk
	ChunkSize = 1000

	// DefaultMaxConsumerHistory is the default maximum number of dequeue records to keep
	DefaultMaxConsumerHistory = 1000
)

// Queue represents a multi-producer, multi-consumer queue
type Queue struct {
	name          string
	data          *ChunkedList
	consumers     *ConsumerManager
	memoryTracker *MemoryTracker

	_     [64]byte // Prevent false sharing
	mutex sync.RWMutex
	_     [64]byte // Prevent false sharing

	closed                  atomic.Bool
	ttl                     time.Duration
	stopChan                chan struct{}
	wg                      sync.WaitGroup
	expirationEnabled       bool
	createdAt               time.Time
	enqueueNotify           chan struct{}
	dequeueNotify           chan struct{}
	maxConsumerHistory      int
	expirationCheckInterval time.Duration
	consumerEvictionTimeout time.Duration
}

// QueueConfig holds configuration for the queue
type QueueConfig struct {
	TTL                     time.Duration
	MaxMemory               int64
	MaxConsumerHistory      int
	ExpirationCheckInterval time.Duration
	ConsumerEvictionTimeout time.Duration
}

// NewQueue creates a new queue with the specified name and default TTL
func NewQueue(name string) *Queue {
	return NewQueueWithConfig(name, QueueConfig{
		TTL:                     DefaultTTL,
		MaxMemory:               MaxQueueMemory,
		MaxConsumerHistory:      DefaultMaxConsumerHistory,
		ExpirationCheckInterval: DefaultExpirationCheckInterval,
	})
}

// NewQueueWithTTL creates a new queue with a custom TTL
func NewQueueWithTTL(name string, ttl time.Duration) *Queue {
	return NewQueueWithConfig(name, QueueConfig{
		TTL:                     ttl,
		MaxMemory:               MaxQueueMemory,
		MaxConsumerHistory:      DefaultMaxConsumerHistory,
		ExpirationCheckInterval: DefaultExpirationCheckInterval,
	})
}

// NewQueueWithConfig creates a new queue with the specified configuration
func NewQueueWithConfig(name string, config QueueConfig) *Queue {
	memoryTracker := NewMemoryTracker(config.MaxMemory)

	// Use default if interval is not specified
	interval := config.ExpirationCheckInterval
	if interval <= 0 {
		interval = DefaultExpirationCheckInterval
	}
	
	if config.MaxConsumerHistory < 0 {
		config.MaxConsumerHistory = 0
	}

	expirationEnabled := config.TTL > 0

	queue := &Queue{
		name:                    name,
		data:                    NewChunkedList(memoryTracker),
		memoryTracker:           memoryTracker,
		ttl:                     config.TTL,
		stopChan:                make(chan struct{}),
		expirationEnabled:       expirationEnabled,
		createdAt:               time.Now(),
		enqueueNotify:           make(chan struct{}, 100),
		dequeueNotify:           make(chan struct{}, 100),
		maxConsumerHistory:      config.MaxConsumerHistory,
		expirationCheckInterval: interval,
		consumerEvictionTimeout: config.ConsumerEvictionTimeout,
	}

	queue.consumers = NewConsumerManager(queue)

	// Start expiration background task
	queue.wg.Add(1)
	go queue.expirationWorker()

	return queue
}

// GetName returns the queue's name
func (q *Queue) GetName() string {
	return q.name
}

// TryEnqueue attempts to add data to the queue without blocking
// Returns an error if the queue is full (memory limit exceeded)
func (q *Queue) TryEnqueue(payload any) error {
	if q.closed.Load() {
		return &QueueClosedError{Operation: "enqueue"}
	}

	data := NewQueueData(payload, q.name)
	data.SetSize(q.memoryTracker.EstimateQueueDataSize(data))

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.closed.Load() {
		return &QueueClosedError{Operation: "enqueue"}
	}

	err := q.data.Enqueue(data)
	if err == nil {
		q.notifyWaitingConsumers()
	}
	return err
}

// Enqueue adds data to the queue, blocking if the queue is full
// Blocks until space becomes available (via expiration or dequeue)
func (q *Queue) Enqueue(payload any) error {
	if q.closed.Load() {
		return &QueueClosedError{Operation: "enqueue"}
	}

	data := NewQueueData(payload, q.name)
	data.SetSize(q.memoryTracker.EstimateQueueDataSize(data))

	if data.GetSize() > q.memoryTracker.GetMaxMemory() {
		return &MemoryLimitError{
			Current: q.memoryTracker.GetMemoryUsage(),
			Max:     q.memoryTracker.GetMaxMemory(),
			Needed:  data.GetSize(),
		}
	}

	for {
		q.mutex.Lock()
		if q.closed.Load() {
			q.mutex.Unlock()
			return &QueueClosedError{Operation: "enqueue"}
		}

		err := q.data.Enqueue(data)
		if err == nil {
			q.mutex.Unlock()
			q.notifyWaitingConsumers()
			return nil
		}

		// Check if it's a memory limit error
		if memErr, ok := err.(*MemoryLimitError); ok {
			if memErr.Needed > memErr.Max {
				q.mutex.Unlock()
				return err
			}
		} else {
			q.mutex.Unlock()
			return err // Return non-memory errors immediately
		}
		q.mutex.Unlock()

		// Wait for space to become available
		select {
		case <-q.dequeueNotify:
			// Space might be available, retry
			continue
		case <-q.stopChan:
			// Queue is closing, return error
			return &QueueClosedError{Operation: "enqueue"}
		}
	}
}

// EnqueueWithContext adds a single item to the queue, blocking if the queue is full
// Blocks until space becomes available, the queue is closed, or the context is cancelled
func (q *Queue) EnqueueWithContext(ctx context.Context, payload any) error {
	if q.closed.Load() {
		return &QueueClosedError{Operation: "enqueue"}
	}

	data := NewQueueData(payload, q.name)
	data.SetSize(q.memoryTracker.EstimateQueueDataSize(data))

	if data.GetSize() > q.memoryTracker.GetMaxMemory() {
		return &MemoryLimitError{
			Current: q.memoryTracker.GetMemoryUsage(),
			Max:     q.memoryTracker.GetMaxMemory(),
			Needed:  data.GetSize(),
		}
	}

	for {
		// Check context first before locking
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		q.mutex.Lock()
		if q.closed.Load() {
			q.mutex.Unlock()
			return &QueueClosedError{Operation: "enqueue"}
		}

		err := q.data.Enqueue(data)
		if err == nil {
			q.mutex.Unlock()
			q.notifyWaitingConsumers()
			return nil
		}

		// Check if it's a memory limit error
		if memErr, ok := err.(*MemoryLimitError); ok {
			if memErr.Needed > memErr.Max {
				q.mutex.Unlock()
				return fmt.Errorf("enqueue failed: %w", err)
			}
		} else {
			q.mutex.Unlock()
			return fmt.Errorf("enqueue failed: %w", err) // Return non-memory errors with wrapping
		}
		q.mutex.Unlock()

		// Wait for space to become available
		select {
		case <-q.dequeueNotify:
			// Space might be available, retry
			continue
		case <-q.stopChan:
			// Queue is closing, return error
			return &QueueClosedError{Operation: "enqueue"}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// TryEnqueueBatch attempts to add multiple items to the queue without blocking
// Returns an error if any item would exceed the memory limit
// This is an atomic operation - either all items are added or none are
func (q *Queue) TryEnqueueBatch(payloads []any) error {
	if q.closed.Load() {
		return &QueueClosedError{Operation: "enqueue batch"}
	}

	if len(payloads) == 0 {
		return nil
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.closed.Load() {
		return &QueueClosedError{Operation: "enqueue batch"}
	}

	// Create all data items upfront and calculate total size
	dataItems := make([]*QueueData, len(payloads))
	var totalBatchSize int64

	for i, payload := range payloads {
		dataItems[i] = NewQueueData(payload, q.name)
		size := q.memoryTracker.EstimateQueueDataSize(dataItems[i])
		dataItems[i].SetSize(size)
		if math.MaxInt64 - totalBatchSize < size {
			return fmt.Errorf("integer overflow in batch memory calculation")
		}
		totalBatchSize += size
	}

	if totalBatchSize > q.memoryTracker.GetMaxMemory() - q.memoryTracker.GetMemoryUsage() {
		return &MemoryLimitError{
			Current: q.memoryTracker.GetMemoryUsage(),
			Max:     q.memoryTracker.GetMaxMemory(),
			Needed:  totalBatchSize,
		}
	}

	if err := q.data.EnqueueBatch(dataItems); err != nil {
		return err
	}

	q.notifyWaitingConsumers()

	return nil
}

// EnqueueBatch adds multiple items to the queue, blocking if the queue is full
// This is an atomic operation - either all items are added or it blocks until space is available
func (q *Queue) EnqueueBatch(payloads []any) error {
	if q.closed.Load() {
		return &QueueClosedError{Operation: "enqueue batch"}
	}

	if len(payloads) == 0 {
		return nil
	}

	// Create all data items upfront and calculate total size
	dataItems := make([]*QueueData, len(payloads))
	var totalBatchSize int64

	for i, payload := range payloads {
		dataItems[i] = NewQueueData(payload, q.name)
		size := q.memoryTracker.EstimateQueueDataSize(dataItems[i])
		dataItems[i].SetSize(size)
		if math.MaxInt64 - totalBatchSize < size {
			return fmt.Errorf("integer overflow in batch memory calculation")
		}
		totalBatchSize += size
	}

	for {
		q.mutex.Lock()
		if q.closed.Load() {
			q.mutex.Unlock()
			return &QueueClosedError{Operation: "enqueue batch"}
		}

		if totalBatchSize > q.memoryTracker.GetMaxMemory() {
			q.mutex.Unlock()
			return &MemoryLimitError{
				Current: q.memoryTracker.GetMemoryUsage(),
				Max:     q.memoryTracker.GetMaxMemory(),
				Needed:  totalBatchSize,
			}
		}

		if q.memoryTracker.GetMaxMemory() - q.memoryTracker.GetMemoryUsage() >= totalBatchSize {
			if err := q.data.EnqueueBatch(dataItems); err != nil {
				q.mutex.Unlock()
				return err
			}
			q.mutex.Unlock()
			q.notifyWaitingConsumers()
			return nil
		}

		q.mutex.Unlock()

		// Wait for space to become available
		select {
		case <-q.dequeueNotify:
			// Space might be available, retry
			continue
		case <-q.stopChan:
			return &QueueClosedError{Operation: "enqueue batch"}
		}
	}
}

// EnqueueBatchWithContext adds multiple items to the queue, blocking if the queue is full
// Blocks until space becomes available, the queue is closed, or the context is cancelled
func (q *Queue) EnqueueBatchWithContext(ctx context.Context, payloads []any) error {
	if q.closed.Load() {
		return &QueueClosedError{Operation: "enqueue batch"}
	}

	if len(payloads) == 0 {
		return nil
	}

	// Create all data items upfront and calculate total size
	dataItems := make([]*QueueData, len(payloads))
	var totalBatchSize int64

	for i, payload := range payloads {
		dataItems[i] = NewQueueData(payload, q.name)
		size := q.memoryTracker.EstimateQueueDataSize(dataItems[i])
		dataItems[i].SetSize(size)
		if math.MaxInt64 - totalBatchSize < size {
			return fmt.Errorf("integer overflow in batch memory calculation")
		}
		totalBatchSize += size
	}

	for {
		// Check context before trying to acquire lock
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		q.mutex.Lock()
		if q.closed.Load() {
			q.mutex.Unlock()
			return &QueueClosedError{Operation: "enqueue batch"}
		}

		if totalBatchSize > q.memoryTracker.GetMaxMemory() {
			q.mutex.Unlock()
			return &MemoryLimitError{
				Current: q.memoryTracker.GetMemoryUsage(),
				Max:     q.memoryTracker.GetMaxMemory(),
				Needed:  totalBatchSize,
			}
		}

		if q.memoryTracker.GetMaxMemory() - q.memoryTracker.GetMemoryUsage() >= totalBatchSize {
			if err := q.data.EnqueueBatch(dataItems); err != nil {
				q.mutex.Unlock()
				return err
			}
			q.mutex.Unlock()
			q.notifyWaitingConsumers()
			return nil
		}

		q.mutex.Unlock()

		// Wait for space to become available
		select {
		case <-q.dequeueNotify:
			// Space might be available, retry
			continue
		case <-q.stopChan:
			return &QueueClosedError{Operation: "enqueue batch"}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// AddConsumer adds a new consumer to the queue
func (q *Queue) AddConsumer() *Consumer {
	return q.consumers.AddConsumer()
}

// AddConsumerGroup adds a new consumer group to the queue
func (q *Queue) AddConsumerGroup(name string) *ConsumerGroup {
	return q.consumers.AddGroup(name)
}

// RemoveConsumer removes a consumer from the queue
func (q *Queue) RemoveConsumer(consumerID string) bool {
	return q.consumers.RemoveConsumer(consumerID)
}

// RemoveConsumerGroup removes a consumer group and all its consumers
func (q *Queue) RemoveConsumerGroup(name string) bool {
	return q.consumers.RemoveGroup(name)
}

// CleanInactiveConsumers removes independent consumers that haven't read for the given duration.
// Returns the number of evicted consumers.
func (q *Queue) CleanInactiveConsumers(timeout time.Duration) int {
	return q.consumers.CleanInactive(timeout)
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
	// No lock needed as totalItems is atomic and pointer is constant
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
// Safe to call multiple times (idempotent)
func (q *Queue) Close() {
	// Only close once
	if !q.closed.CompareAndSwap(false, true) {
		return // Already closed
	}

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

	ticker := time.NewTicker(q.expirationCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if q.expirationEnabled {
				q.cleanupExpiredItems()
			}
			if q.consumerEvictionTimeout > 0 {
				q.CleanInactiveConsumers(q.consumerEvictionTimeout)
			}
		case <-q.stopChan:
			return
		}
	}
}

// cleanupExpiredItems removes expired items and notifies consumers
func (q *Queue) cleanupExpiredItems() int {
	q.mutex.Lock()

	// Check if expiration is enabled
	if !q.expirationEnabled {
		q.mutex.Unlock()
		return 0
	}

	// Remove expired items
	expiredCount, removalInfo := q.data.RemoveExpiredData(q.ttl)
	
	var newFirstElement *list.Element
	if expiredCount > 0 {
		newFirstElement = q.data.GetFirstElement()

		// MUST hold q.mutex while notifying consumers to safely traverse the chunked list
		q.consumers.NotifyAllConsumersOfExpiration(newFirstElement, removalInfo)
	}
	
	q.mutex.Unlock()

	if expiredCount > 0 {
		// Notify ALL waiting producers that space is available
		// Fill the channel to wake up as many as possible
		for {
			select {
			case q.dequeueNotify <- struct{}{}:
				// Sent successfully
			default:
				// Channel full, stop sending
				return expiredCount
			}
		}
	}

	return expiredCount
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
