package queue

import (
	"reflect"
	"unsafe"
)

const (
	// MaxQueueMemory is the maximum memory allowed for a queue (1MB)
	MaxQueueMemory = 1024 * 1024 // 1MB

	// BaseQueueDataSize is the base size of QueueData struct without payload
	BaseQueueDataSize = int64(unsafe.Sizeof(QueueData{}))

	// BaseQueueEventSize is the base size of QueueEvent struct
	BaseQueueEventSize = int64(unsafe.Sizeof(QueueEvent{}))

	// ChunkNodeSize is the size of a ChunkNode struct
	ChunkNodeSize = int64(unsafe.Sizeof(ChunkNode{}))
)

// MemoryTracker tracks memory usage for the queue
type MemoryTracker struct {
	totalMemory int64
	maxMemory   int64
}

// NewMemoryTracker creates a new memory tracker
func NewMemoryTracker() *MemoryTracker {
	return &MemoryTracker{
		totalMemory: 0,
		maxMemory:   MaxQueueMemory,
	}
}

// EstimateQueueDataSize estimates the memory size of a QueueData item
// QueueData is now immutable, so this size is fixed after creation
func (mt *MemoryTracker) EstimateQueueDataSize(data *QueueData) int64 {
	if data == nil {
		return 0
	}

	size := BaseQueueDataSize

	// Add size of ID string
	size += int64(len(data.ID))

	// Add size of payload
	size += mt.estimatePayloadSize(data.Payload)

	// Add size of single enqueue event
	size += BaseQueueEventSize
	size += int64(len(data.EnqueueEvent.QueueName))
	size += int64(len(data.EnqueueEvent.EventType))

	return size
}

// estimatePayloadSize estimates the size of an arbitrary payload
func (mt *MemoryTracker) estimatePayloadSize(payload interface{}) int64 {
	if payload == nil {
		return 0
	}

	v := reflect.ValueOf(payload)
	return mt.estimateValueSize(v)
}

// estimateValueSize recursively estimates the size of a reflect.Value
func (mt *MemoryTracker) estimateValueSize(v reflect.Value) int64 {
	if !v.IsValid() {
		return 0
	}

	switch v.Kind() {
	case reflect.Bool:
		return 1
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int64(v.Type().Size())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(v.Type().Size())
	case reflect.Float32, reflect.Float64:
		return int64(v.Type().Size())
	case reflect.Complex64, reflect.Complex128:
		return int64(v.Type().Size())
	case reflect.String:
		return int64(len(v.String()))
	case reflect.Array, reflect.Slice:
		size := int64(0)
		for i := 0; i < v.Len(); i++ {
			size += mt.estimateValueSize(v.Index(i))
		}
		return size
	case reflect.Map:
		size := int64(0)
		for _, key := range v.MapKeys() {
			size += mt.estimateValueSize(key)
			size += mt.estimateValueSize(v.MapIndex(key))
		}
		return size
	case reflect.Struct:
		size := int64(0)
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanInterface() {
				size += mt.estimateValueSize(v.Field(i))
			}
		}
		return size
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return int64(unsafe.Sizeof(uintptr(0)))
		}
		return int64(unsafe.Sizeof(uintptr(0))) + mt.estimateValueSize(v.Elem())
	default:
		// For unknown types, use the type's size
		return int64(v.Type().Size())
	}
}

// CanAddData checks if adding the data would exceed memory limit
func (mt *MemoryTracker) CanAddData(data *QueueData) bool {
	estimatedSize := mt.EstimateQueueDataSize(data)
	return mt.totalMemory+estimatedSize <= mt.maxMemory
}

// AddData adds memory usage for the data
func (mt *MemoryTracker) AddData(data *QueueData) {
	size := mt.EstimateQueueDataSize(data)
	mt.totalMemory += size
}

// RemoveData removes memory usage for the data
func (mt *MemoryTracker) RemoveData(data *QueueData) {
	size := mt.EstimateQueueDataSize(data)
	mt.totalMemory -= size
	if mt.totalMemory < 0 {
		mt.totalMemory = 0
	}
}

// AddChunk adds memory usage for a chunk
func (mt *MemoryTracker) AddChunk() {
	mt.totalMemory += ChunkNodeSize
}

// RemoveChunk removes memory usage for a chunk
func (mt *MemoryTracker) RemoveChunk() {
	mt.totalMemory -= ChunkNodeSize
	if mt.totalMemory < 0 {
		mt.totalMemory = 0
	}
}

// GetMemoryUsage returns current memory usage
func (mt *MemoryTracker) GetMemoryUsage() int64 {
	return mt.totalMemory
}

// GetMemoryUsagePercent returns memory usage as a percentage
func (mt *MemoryTracker) GetMemoryUsagePercent() float64 {
	return float64(mt.totalMemory) / float64(mt.maxMemory) * 100
}

// IsNearLimit checks if memory usage is near the limit (>90%)
func (mt *MemoryTracker) IsNearLimit() bool {
	return mt.GetMemoryUsagePercent() > 90.0
}
