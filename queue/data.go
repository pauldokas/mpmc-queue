package queue

import (
	"time"

	"github.com/google/uuid"
)

// QueueEvent represents an event in the queue's history
type QueueEvent struct {
	Timestamp time.Time `json:"timestamp"`
	QueueName string    `json:"queue_name"`
	EventType string    `json:"event_type"` // "enqueue" or "dequeue"
}

// QueueData represents a single item in the queue
// QueueData is immutable after creation for thread-safety
type QueueData struct {
	ID           string     `json:"id"`            // UUID
	Payload      any        `json:"payload"`       // Arbitrary data
	EnqueueEvent QueueEvent `json:"enqueue_event"` // Single enqueue event (immutable)
	Created      time.Time  `json:"created"`       // For expiration tracking
}

// NewQueueData creates a new QueueData instance with enqueue event
func NewQueueData(payload any, queueName string) *QueueData {
	now := time.Now()
	return &QueueData{
		ID:      uuid.New().String(),
		Payload: payload,
		EnqueueEvent: QueueEvent{
			Timestamp: now,
			QueueName: queueName,
			EventType: "enqueue",
		},
		Created: now,
	}
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
	Size int              `json:"size"` // Current number of items in this chunk
}

// NewChunkNode creates a new empty chunk node
func NewChunkNode() *ChunkNode {
	return &ChunkNode{
		Data: [1000]*QueueData{},
		Size: 0,
	}
}

// Add adds data to the chunk if there's space
func (cn *ChunkNode) Add(data *QueueData) bool {
	if cn.Size >= 1000 {
		return false
	}
	cn.Data[cn.Size] = data
	cn.Size++
	return true
}

// Get retrieves data at the specified index
func (cn *ChunkNode) Get(index int) *QueueData {
	if index < 0 || index >= cn.Size {
		return nil
	}
	return cn.Data[index]
}

// IsFull returns true if the chunk is at capacity
func (cn *ChunkNode) IsFull() bool {
	return cn.Size >= 1000
}

// IsEmpty returns true if the chunk has no data
func (cn *ChunkNode) IsEmpty() bool {
	return cn.Size == 0
}

// RemoveExpired removes expired items from the beginning of the chunk
// Returns the number of items removed
func (cn *ChunkNode) RemoveExpired(ttl time.Duration) int {
	removed := 0
	for i := 0; i < cn.Size; i++ {
		if cn.Data[i] != nil && cn.Data[i].IsExpired(ttl) {
			cn.Data[i] = nil
			removed++
		} else {
			break // Items are ordered by creation time
		}
	}

	// Compact the array if items were removed
	if removed > 0 {
		for i := removed; i < cn.Size; i++ {
			cn.Data[i-removed] = cn.Data[i]
			cn.Data[i] = nil
		}
		cn.Size -= removed
	}

	return removed
}

// GetEarliestExpiry returns the creation time of the earliest item in the chunk
func (cn *ChunkNode) GetEarliestExpiry() *time.Time {
	if cn.Size == 0 {
		return nil
	}

	for i := 0; i < cn.Size; i++ {
		if cn.Data[i] != nil {
			return &cn.Data[i].Created
		}
	}

	return nil
}
