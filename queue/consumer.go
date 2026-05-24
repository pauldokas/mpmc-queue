package queue

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// DequeueRecord represents a single dequeue event for a consumer
type DequeueRecord struct {
	DataID    uint64    `json:"data_id"`
	Timestamp time.Time `json:"timestamp"`
}

// Consumer represents a queue consumer with independent position tracking
type Consumer struct {
	id             string
	chunkElement   *list.Element // Current chunk position
	indexInChunk   int           // Position within current chunk
	notificationCh chan int      // Notification of expired items

	_     [64]byte // Prevent false sharing
	mutex sync.Mutex
	_     [64]byte // Prevent false sharing

	totalItemsRead atomic.Int64 // Total items this consumer has read
	_              [64]byte // Prevent false sharing

	queue          *Queue          // Reference to parent queue
	lastReadTime   time.Time       // For tracking consumer activity
	dequeueHistory []DequeueRecord // Track dequeue events locally
	maxHistory     int             // Maximum history records to keep
	closed         atomic.Bool     // Tracks if consumer is closed
	group          *ConsumerGroup  // Reference to parent group (nil if independent)
}

// NewConsumer creates a new consumer
func NewConsumer(queue *Queue) *Consumer {
	maxHistory := DefaultMaxConsumerHistory
	if queue != nil {
		maxHistory = queue.maxConsumerHistory
	}

	return &Consumer{
		id:             uuid.New().String(),
		chunkElement:   nil, // Will be set when first item is read
		indexInChunk:   0,
		notificationCh: make(chan int, 100), // Buffered channel for notifications
		queue:          queue,
		lastReadTime:   time.Now(),
		dequeueHistory: make([]DequeueRecord, 0, 100), // Pre-allocate some capacity
		maxHistory:     maxHistory,
		group:          nil,
	}
}

func (c *Consumer) addToHistoryUnsafe(dataID uint64) {
	c.dequeueHistory = append(c.dequeueHistory, DequeueRecord{
		DataID:    dataID,
		Timestamp: time.Now(),
	})

	if len(c.dequeueHistory) > c.maxHistory {
		excess := len(c.dequeueHistory) - c.maxHistory
		c.dequeueHistory = c.dequeueHistory[excess:]
	}
}

// GetID returns the consumer's unique identifier
func (c *Consumer) GetID() string {
	return c.id
}

// GetNotificationChannel returns the channel for expired item notifications
func (c *Consumer) GetNotificationChannel() <-chan int {
	if c.group != nil {
		return c.group.notificationCh
	}
	return c.notificationCh
}

// GetPosition returns the consumer's current position
func (c *Consumer) GetPosition() (*list.Element, int) {
	if c.group != nil {
		return c.group.getPosition()
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.chunkElement, c.indexInChunk
}

// getPositionUnsafe returns position without locking (caller must hold consumer lock)
func (c *Consumer) getPositionUnsafe() (*list.Element, int) {
	if c.group != nil {
		// Even if unsafe requested, group access must be safe because group lock is distinct
		return c.group.getPosition()
	}
	return c.chunkElement, c.indexInChunk
}

// SetPosition sets the consumer's position (used for initialization)
func (c *Consumer) SetPosition(element *list.Element, index int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.chunkElement = element
	c.indexInChunk = index
}

// TryRead attempts to read the next available data item for this consumer without blocking
// Returns nil if no data is available
func (c *Consumer) TryRead() *QueueData {
	if c.closed.Load() {
		return nil
	}

	if c.group != nil {
		data := c.group.TryRead()
		if data != nil {
			c.mutex.Lock()
			c.addToHistoryUnsafe(data.ID)
			c.lastReadTime = time.Now()
			c.totalItemsRead.Add(1)
			c.mutex.Unlock()
		}
		return data
	}

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

		chunk := currentElement.Value.(*ChunkNode)
		
		if chunk.pooled.Load() {
			c.mutex.Lock()
			if c.chunkElement == currentElement {
				c.chunkElement = nil
				c.indexInChunk = 0
			}
			c.mutex.Unlock()
			return nil
		}

		chunkSize := chunk.GetSize()

		if currentIndex < chunkSize {
			c.queue.mutex.RLock()
			data := chunk.Get(currentIndex)
			var dataCopy QueueData
			if data != nil {
				dataCopy = *data
			}
			c.queue.mutex.RUnlock()

			if data != nil {
				c.mutex.Lock()
				if c.chunkElement == currentElement && c.indexInChunk == currentIndex {
					c.addToHistoryUnsafe(dataCopy.ID)
					c.indexInChunk++
					c.lastReadTime = time.Now()
					c.totalItemsRead.Add(1)
					c.mutex.Unlock()

					return &dataCopy
				}
				c.mutex.Unlock()
				continue
			}
			
			head := int(atomic.LoadInt32(&chunk.head))
			if currentIndex < head {
				c.mutex.Lock()
				if c.chunkElement == currentElement && c.indexInChunk == currentIndex {
					c.indexInChunk = head
				}
				c.mutex.Unlock()
				continue
			}
			
			return nil
		} else {
			nextElement := chunk.NextElement.Load()

			c.mutex.Lock()
			// Only update if position hasn't changed
			if c.chunkElement == currentElement && c.indexInChunk == currentIndex {
				if nextElement != nil {
					c.chunkElement = nextElement
					c.indexInChunk = 0
				}
			}
			c.mutex.Unlock()

			if nextElement == nil {
				return nil
			}
		}
	}
}

// Read reads the next available data item for this consumer, blocking if no data is available
// Blocks until data becomes available or the queue is closed
func (c *Consumer) Read() *QueueData {
	for {
		data := c.TryRead()
		if data != nil {
			return data
		}

		// No data available, wait for notification
		select {
		case <-c.queue.enqueueNotify:
			// New data might be available, retry
			continue
		case <-c.queue.stopChan:
			// Queue is closing, return nil
			return nil
		}
	}
}

// ReadWithContext reads the next available data item for this consumer, blocking if no data is available
// Blocks until data becomes available, the queue is closed, or the context is cancelled
func (c *Consumer) ReadWithContext(ctx context.Context) (*QueueData, error) {
	for {
		// Check context first
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		data := c.TryRead()
		if data != nil {
			return data, nil
		}

		// No data available, wait for notification
		select {
		case <-c.queue.enqueueNotify:
			// New data might be available, retry
			continue
		case <-c.queue.stopChan:
			// Queue is closing, return nil
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// TryReadBatch attempts to read multiple items up to the specified limit without blocking
// Returns immediately with whatever items are available (may be less than limit)
func (c *Consumer) TryReadBatch(limit int) []*QueueData {
	if limit <= 0 {
		return nil
	}

	cap := limit
	if cap > 1024 {
		cap = 1024
	}
	batch := make([]*QueueData, 0, cap)

	for len(batch) < limit {
		data := c.TryRead()
		if data == nil {
			break // No more data available
		}
		batch = append(batch, data)
	}

	return batch
}

// ReadBatch reads multiple items up to the specified limit, blocking until at least one item is available
// Returns a batch of items (may be less than limit if queue has fewer items)
func (c *Consumer) ReadBatch(limit int) []*QueueData {
	if limit <= 0 {
		return nil
	}

	cap := limit
	if cap > 1024 {
		cap = 1024
	}
	batch := make([]*QueueData, 0, cap)

	// Block until at least one item is available
	firstItem := c.Read()
	if firstItem == nil {
		return batch // Queue closed
	}
	batch = append(batch, firstItem)

	// Try to read more items without blocking
	for len(batch) < limit {
		data := c.TryRead()
		if data == nil {
			break // No more data immediately available
		}
		batch = append(batch, data)
	}

	return batch
}

// ReadBatchWithContext reads multiple items up to the specified limit, blocking until at least one item is available
// Blocks until data becomes available, the queue is closed, or the context is cancelled
func (c *Consumer) ReadBatchWithContext(ctx context.Context, limit int) ([]*QueueData, error) {
	if limit <= 0 {
		return nil, nil
	}

	cap := limit
	if cap > 1024 {
		cap = 1024
	}
	batch := make([]*QueueData, 0, cap)

	// Block until at least one item is available or context cancelled
	firstItem, err := c.ReadWithContext(ctx)
	if err != nil {
		return batch, err
	}
	if firstItem == nil {
		return batch, nil // Queue closed
	}
	batch = append(batch, firstItem)

	// Try to read more items without blocking
	for len(batch) < limit {
		// Check context between reads just in case
		select {
		case <-ctx.Done():
			// Even if context is done, we already have some data, so we can return it
			// However, convention usually suggests returning error if context is cancelled.
			// But here we've successfully read at least one item.
			// Let's return what we have so far and the error.
			return batch, ctx.Err()
		default:
		}

		data := c.TryRead()
		if data == nil {
			break // No more data immediately available
		}
		batch = append(batch, data)
	}

	return batch, nil
}

// HasMoreData checks if there's more data available for this consumer
func (c *Consumer) HasMoreData() bool {
	if c.closed.Load() {
		return false
	}

	if c.group != nil {
		return c.group.HasMoreData()
	}

	c.mutex.Lock()
	chunkElement := c.chunkElement
	indexInChunk := c.indexInChunk
	c.mutex.Unlock()

	if chunkElement == nil {
		return !c.queue.data.IsEmpty()
	}

	// Check current chunk
	chunk := chunkElement.Value.(*ChunkNode)
	if indexInChunk < chunk.GetSize() {
		return true
	}

	return chunk.NextElement.Load() != nil
}

// GetUnreadCount returns the number of unread items for this consumer
func (c *Consumer) GetUnreadCount() int64 {
	if c.closed.Load() {
		return 0
	}

	if c.group != nil {
		return c.group.GetUnreadCount()
	}

	c.queue.mutex.RLock()
	defer c.queue.mutex.RUnlock()

	c.mutex.Lock()
	chunkElement := c.chunkElement
	indexInChunk := c.indexInChunk
	c.mutex.Unlock()

	if chunkElement == nil {
		return c.queue.data.GetTotalItems()
	}

	return c.queue.data.CountItemsFrom(chunkElement, indexInChunk)
}

// GetStats returns consumer statistics
func (c *Consumer) GetStats() ConsumerStats {
	if c.group != nil {
		c.mutex.Lock()
		id := c.id
		totalItemsRead := c.totalItemsRead.Load()
		lastReadTime := c.lastReadTime
		c.mutex.Unlock()

		return ConsumerStats{
			ID:             id,
			TotalItemsRead: totalItemsRead,
			UnreadItems:    c.group.GetUnreadCount(),
			LastReadTime:   lastReadTime,
		}
	}

	c.queue.mutex.RLock()
	defer c.queue.mutex.RUnlock()

	c.mutex.Lock()
	id := c.id
	totalItemsRead := c.totalItemsRead.Load()
	lastReadTime := c.lastReadTime
	chunkElement := c.chunkElement
	indexInChunk := c.indexInChunk
	c.mutex.Unlock()

	// Calculate unread count
	var unreadCount int64
	if chunkElement == nil {
		unreadCount = c.queue.data.GetTotalItems()
	} else {
		unreadCount = c.queue.data.CountItemsFrom(chunkElement, indexInChunk)
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
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed.Load() {
		return
	}

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
	if c.group != nil {
		return // Group handles position update
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.chunkElement == nil || expiredCount == 0 {
		return
	}

	for _, info := range removalInfo {
		if info.Element == c.chunkElement {
			if c.indexInChunk < info.NewHead {
				c.indexInChunk = info.NewHead
			}
			break
		}
	}

	if newFirstElement != nil {
		if c.chunkElement != nil {
			chunk := c.chunkElement.Value.(*ChunkNode)
			if chunk.pooled.Load() {
				c.chunkElement = newFirstElement
				c.indexInChunk = 0
			}
		}
	} else {
		c.chunkElement = nil
		c.indexInChunk = 0
	}
}

// Close closes the consumer and cleans up resources
// Safe to call multiple times (idempotent)
func (c *Consumer) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Only close once
	if !c.closed.CompareAndSwap(false, true) {
		return // Already closed
	}
	close(c.notificationCh)
}

// TryReadWhere attempts to read the next data item that matches the predicate without blocking
// Returns nil if no matching data is available
// The predicate function should return true for items that should be returned
// Note: This advances the consumer position as it searches, consuming non-matching items
func (c *Consumer) TryReadWhere(predicate func(*QueueData) bool) (*QueueData, error) {
	if c.group != nil {
		// Filtering is restricted for ConsumerGroups to prevent destructive consumption
		// of non-matching items from the shared cursor.
		return nil, &FilterNotSupportedError{}
	}

	if predicate == nil {
		return nil, nil
	}

	// Keep reading until we find a match or run out of data
	for {
		data := c.TryRead()
		if data == nil {
			// No more data available
			return nil, nil
		}

		// Check if data matches predicate
		if predicate(data) {
			return data, nil
		}

		// Continue to next item
	}
}

// ReadWhere reads the next data item that matches the predicate, blocking until a match is found
// Blocks until matching data becomes available or the queue is closed
// The predicate function should return true for items that should be returned
func (c *Consumer) ReadWhere(predicate func(*QueueData) bool) (*QueueData, error) {
	if predicate == nil {
		return nil, nil
	}

	if c.group != nil {
		// Filtering is restricted for ConsumerGroups
		return nil, &FilterNotSupportedError{}
	}

	for {
		data, err := c.TryReadWhere(predicate)
		if err != nil {
			return nil, err
		}
		if data != nil {
			return data, nil
		}

		// No matching data available, wait for notification
		select {
		case <-c.queue.enqueueNotify:
			// New data might be available, retry
			continue
		case <-c.queue.stopChan:
			// Queue is closing, return nil
			return nil, nil
		}
	}
}

// ReadWhereWithContext reads the next data item that matches the predicate, blocking until a match is found
// Blocks until matching data becomes available, the queue is closed, or the context is cancelled
// The predicate function should return true for items that should be returned
func (c *Consumer) ReadWhereWithContext(ctx context.Context, predicate func(*QueueData) bool) (*QueueData, error) {
	if predicate == nil {
		return nil, nil
	}

	if c.group != nil {
		// Filtering is restricted for ConsumerGroups
		return nil, &FilterNotSupportedError{}
	}

	for {
		// Check for context cancellation before checking for data
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// Context not cancelled, continue
		}

		data, err := c.TryReadWhere(predicate)
		if err != nil {
			return nil, err
		}
		if data != nil {
			return data, nil
		}

		// No matching data available, wait for notification
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.queue.enqueueNotify:
			// New data might be available, retry
			continue
		case <-c.queue.stopChan:
			// Queue is closing, return nil
			return nil, nil
		}
	}
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
	consumers       map[string]*Consumer
	groups          map[string]*ConsumerGroup
	activeConsumers atomic.Value // Stores []*Consumer
	mutex           sync.RWMutex
	queue           *Queue
}

// NewConsumerManager creates a new consumer manager
func NewConsumerManager(queue *Queue) *ConsumerManager {
	cm := &ConsumerManager{
		consumers: make(map[string]*Consumer),
		groups:    make(map[string]*ConsumerGroup),
		queue:     queue,
	}
	cm.activeConsumers.Store(make([]*Consumer, 0))
	return cm
}

// AddConsumerToGroup adds a new consumer to a specific group
func (cm *ConsumerManager) AddConsumerToGroup(group *ConsumerGroup) *Consumer {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	consumer := NewConsumer(cm.queue)
	consumer.group = group

	// Position is managed by the group, so we don't need to set it here

	cm.consumers[consumer.GetID()] = consumer
	cm.updateActiveConsumers()

	return consumer
}

// AddConsumer adds a new consumer to the queue
// New consumers start reading from the beginning of the queue
func (cm *ConsumerManager) AddConsumer() *Consumer {
	// Acquire Queue.mutex BEFORE ConsumerManager.mutex to prevent deadlocks
	cm.queue.mutex.RLock()
	defer cm.queue.mutex.RUnlock()

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	consumer := NewConsumer(cm.queue)

	// Initialize consumer to read from the beginning
	firstElement := cm.queue.data.GetFirstElement()

	if firstElement != nil {
		consumer.SetPosition(firstElement, 0)
	}

	cm.consumers[consumer.GetID()] = consumer
	cm.updateActiveConsumers()

	return consumer
}

// AddGroup adds a new consumer group or returns existing one
func (cm *ConsumerManager) AddGroup(name string) *ConsumerGroup {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if group, exists := cm.groups[name]; exists {
		return group
	}

	group := NewConsumerGroup(name, cm.queue)
	cm.groups[name] = group
	return group
}

// RemoveGroup removes a consumer group and all its associated consumers
func (cm *ConsumerManager) RemoveGroup(name string) bool {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	group, exists := cm.groups[name]
	if !exists {
		return false
	}

	for id, c := range cm.consumers {
		if c.group == group {
			c.Close()
			delete(cm.consumers, id)
		}
	}

	group.Close()
	delete(cm.groups, name)
	cm.updateActiveConsumers()

	return true
}

func (cm *ConsumerManager) CleanInactive(timeout time.Duration) int {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	now := time.Now()
	removed := 0

	for id, c := range cm.consumers {
		c.mutex.Lock()
		lastRead := c.lastReadTime
		c.mutex.Unlock()

		if now.Sub(lastRead) > timeout {
			c.Close()
			delete(cm.consumers, id)
			removed++
		}
	}

	for name, g := range cm.groups {
		hasConsumers := false
		for _, c := range cm.consumers {
			if c.group == g {
				hasConsumers = true
				break
			}
		}
		if !hasConsumers {
			g.Close()
			delete(cm.groups, name)
		}
	}

	if removed > 0 {
		cm.updateActiveConsumers()
	}

	return removed
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
	cm.updateActiveConsumers()

	return true
}

// updateActiveConsumers updates the atomic snapshot of consumers
// Must be called with lock held
func (cm *ConsumerManager) updateActiveConsumers() {
	consumers := make([]*Consumer, 0, len(cm.consumers))
	for _, consumer := range cm.consumers {
		consumers = append(consumers, consumer)
	}
	cm.activeConsumers.Store(consumers)
}

// GetConsumer returns a consumer by ID
func (cm *ConsumerManager) GetConsumer(consumerID string) *Consumer {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	return cm.consumers[consumerID]
}

// GetAllConsumers returns all active consumers
func (cm *ConsumerManager) GetAllConsumers() []*Consumer {
	// Lock-free read via atomic value
	return cm.activeConsumers.Load().([]*Consumer)
}

// NotifyAllConsumersOfExpiration notifies all consumers about expired items
func calculateExpiredCount(chunkElement *list.Element, indexInChunk int, removalInfo []ChunkRemovalInfo) int {
	if chunkElement == nil {
		total := 0
		for _, info := range removalInfo {
			total += info.RemovedCount
		}
		return total
	}

	expiredCount := 0
	foundConsumerChunk := false

	for _, info := range removalInfo {
		if foundConsumerChunk {
			expiredCount += info.RemovedCount
		} else if info.Element == chunkElement {
			foundConsumerChunk = true
			if indexInChunk < info.NewHead {
				if indexInChunk <= info.OldHead {
					expiredCount += info.RemovedCount
				} else {
					expiredCount += info.NewHead - indexInChunk
				}
			}
		}
	}
	return expiredCount
}

func (cm *ConsumerManager) NotifyAllConsumersOfExpiration(newFirstElement *list.Element, removalInfo []ChunkRemovalInfo) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	// Calculate total expired items for position adjustment
	totalExpired := 0
	for _, info := range removalInfo {
		totalExpired += info.RemovedCount
	}

	for _, consumer := range cm.consumers {
		if consumer.group != nil {
			continue
		}

		consumer.mutex.Lock()
		chunkElement, indexInChunk := consumer.getPositionUnsafe()
		consumer.mutex.Unlock()

		expiredCount := calculateExpiredCount(chunkElement, indexInChunk, removalInfo)

		// Notify consumers about their unread expired items
		if expiredCount > 0 {
			consumer.NotifyExpiredItems(expiredCount)
		}

		// ALWAYS update position if any items expired (even if consumer already read past them)
		// This is necessary because chunk compaction affects all consumer positions
		if totalExpired > 0 {
			consumer.UpdatePositionAfterExpiration(totalExpired, newFirstElement, removalInfo)
		}
	}

	if totalExpired > 0 || len(removalInfo) > 0 {
		for _, group := range cm.groups {
			group.mutex.Lock()
			chunkElement := group.chunkElement
			indexInChunk := group.indexInChunk
			group.mutex.Unlock()

			expiredCount := calculateExpiredCount(chunkElement, indexInChunk, removalInfo)
			if expiredCount > 0 {
				group.NotifyExpiredItems(expiredCount)
			}

			if totalExpired > 0 {
				group.UpdatePositionAfterExpiration(totalExpired, newFirstElement, removalInfo)
			}
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
	consumers := cm.GetAllConsumers()

	stats := make([]ConsumerStats, 0, len(consumers))
	for _, consumer := range consumers {
		stats = append(stats, consumer.GetStats())
	}

	return stats
}
