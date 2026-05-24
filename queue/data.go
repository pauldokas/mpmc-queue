package queue

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var dataIDSequence atomic.Uint64

// QueueEvent represents an event in the queue's history
type QueueEvent struct {
	Timestamp time.Time `json:"timestamp"`
	QueueName string    `json:"queue_name"`
	EventType string    `json:"event_type"` // "enqueue" or "dequeue"
}

// QueueData represents a single item in the queue
// QueueData is immutable after creation for thread-safety
type QueueData struct {
	ID           string     `json:"id"`            // UUID or Sequence
	Payload      any        `json:"payload"`       // Arbitrary data
	EnqueueEvent QueueEvent `json:"enqueue_event"` // Single enqueue event (immutable)
	Created      time.Time  `json:"created"`       // For expiration tracking
	size         int64
}

var queueDataPool = sync.Pool{
	New: func() any {
		return &QueueData{}
	},
}

// NewQueueData creates a new QueueData instance with enqueue event
func NewQueueData(payload any, queueName string) *QueueData {
	now := time.Now()
	id := dataIDSequence.Add(1)
	
	qd := queueDataPool.Get().(*QueueData)
	qd.ID = strconv.FormatUint(id, 10)
	qd.Payload = payload
	qd.EnqueueEvent.Timestamp = now
	qd.EnqueueEvent.QueueName = queueName
	qd.EnqueueEvent.EventType = "enqueue"
	qd.Created = now
	qd.size = 0
	
	return qd
}

// RecycleQueueData puts the QueueData back into the pool
func RecycleQueueData(qd *QueueData) {
	if qd == nil {
		return
	}
	qd.Payload = nil // Prevent memory leaks
	qd.EnqueueEvent.QueueName = ""
	qd.ID = ""
	queueDataPool.Put(qd)
}

// SetSize sets the pre-calculated size of the data
func (qd *QueueData) SetSize(size int64) {
	qd.size = size
}

// GetSize returns the pre-calculated size of the data
func (qd *QueueData) GetSize() int64 {
	return qd.size
}

// GetEnqueueEvent returns the enqueue event for this data
func (qd *QueueData) GetEnqueueEvent() QueueEvent {
	return qd.EnqueueEvent
}

// IsExpired checks if the data has exceeded the TTL
func (qd *QueueData) IsExpired(ttl time.Duration) bool {
	return time.Since(qd.Created) > ttl
}

// ChunkNode represents a node in the chunked list containing up to 1000 data items
type ChunkNode struct {
	Data [1000]*QueueData `json:"data"`

	_    [64]byte // Prevent false sharing
	size int32    // Current number of items in this chunk (use atomic operations)
	_    [64]byte // Prevent false sharing
}

// NewChunkNode creates a new empty chunk node
func NewChunkNode() *ChunkNode {
	return GetChunkNode()
}

// GetSize returns the current size using atomic load
func (cn *ChunkNode) GetSize() int {
	return int(atomic.LoadInt32(&cn.size))
}

// setSize sets the size using atomic store (private method)
func (cn *ChunkNode) setSize(newSize int) {
	atomic.StoreInt32(&cn.size, int32(newSize))
}

// incrementSize atomically increments the size and returns the new value
func (cn *ChunkNode) incrementSize() int {
	return int(atomic.AddInt32(&cn.size, 1))
}

// Add adds data to the chunk if there's space
func (cn *ChunkNode) Add(data *QueueData) bool {
	currentSize := cn.GetSize()
	if currentSize >= 1000 {
		return false
	}
	cn.Data[currentSize] = data
	cn.incrementSize()
	return true
}

// Get retrieves data at the specified index
func (cn *ChunkNode) Get(index int) *QueueData {
	size := cn.GetSize()
	if index < 0 || index >= size {
		return nil
	}
	return cn.Data[index]
}

// IsFull returns true if the chunk is at capacity
func (cn *ChunkNode) IsFull() bool {
	return cn.GetSize() >= 1000
}

// IsEmpty returns true if the chunk has no data
func (cn *ChunkNode) IsEmpty() bool {
	return cn.GetSize() == 0
}

// RemoveExpired removes expired items from the beginning of the chunk
// Returns the number of items removed and the removed items for memory tracking
func (cn *ChunkNode) RemoveExpired(ttl time.Duration) (int, []*QueueData) {
	size := cn.GetSize()
	removed := 0
	removedItems := make([]*QueueData, 0)

	for i := 0; i < size; i++ {
		if cn.Data[i] != nil && cn.Data[i].IsExpired(ttl) {
			removedItems = append(removedItems, cn.Data[i])
			cn.Data[i] = nil
			removed++
		} else {
			break // Items are ordered by creation time
		}
	}

	// Compact the array if items were removed
	if removed > 0 {
		for i := removed; i < size; i++ {
			cn.Data[i-removed] = cn.Data[i]
			cn.Data[i] = nil
		}
		cn.setSize(size - removed)
	}

	return removed, removedItems
}

// GetEarliestExpiry returns the creation time of the earliest item in the chunk
func (cn *ChunkNode) GetEarliestExpiry() *time.Time {
	size := cn.GetSize()
	if size == 0 {
		return nil
	}

	for i := 0; i < size; i++ {
		if cn.Data[i] != nil {
			return &cn.Data[i].Created
		}
	}

	return nil
}
