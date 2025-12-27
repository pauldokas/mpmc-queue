package queue

import (
	"reflect"
	"sync"
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
	sizeCache   map[reflect.Type]int64
	cacheMutex  sync.RWMutex
}

// NewMemoryTracker creates a new memory tracker
func NewMemoryTracker(maxMemory int64) *MemoryTracker {
	if maxMemory <= 0 {
		maxMemory = MaxQueueMemory
	}
	return &MemoryTracker{
		totalMemory: 0,
		maxMemory:   maxMemory,
		sizeCache:   make(map[reflect.Type]int64),
	}
}

// GetMaxMemory returns the maximum allowed memory
func (mt *MemoryTracker) GetMaxMemory() int64 {
	return mt.maxMemory
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
func (mt *MemoryTracker) estimatePayloadSize(payload any) int64 {
	if payload == nil {
		return 0
	}

	v := reflect.ValueOf(payload)
	size, _ := mt.estimateValueSize(v)
	return size
}

// estimateValueSize recursively estimates the size of a reflect.Value
// Returns size and whether the type has a fixed size
func (mt *MemoryTracker) estimateValueSize(v reflect.Value) (int64, bool) {
	if !v.IsValid() {
		return 0, true
	}

	t := v.Type()

	// Check cache
	mt.cacheMutex.RLock()
	cached, ok := mt.sizeCache[t]
	mt.cacheMutex.RUnlock()

	if ok {
		if cached >= 0 {
			return cached, true
		}
		// If cached is -1, it's variable size, so we must recalculate
		// We fall through to the switch but know it's not fixed
	}

	var size int64
	var isFixed bool = true

	switch v.Kind() {
	case reflect.Bool:
		size = 1
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		size = int64(t.Size())
	case reflect.String:
		size = int64(len(v.String()))
		isFixed = false
	case reflect.Array:
		size = 0
		// Arrays have fixed length, but elements might be variable (e.g. [10]string)
		for i := 0; i < v.Len(); i++ {
			elemSize, elemFixed := mt.estimateValueSize(v.Index(i))
			size += elemSize
			if !elemFixed {
				isFixed = false
			}
		}
	case reflect.Slice:
		isFixed = false
		size = 0
		for i := 0; i < v.Len(); i++ {
			elemSize, _ := mt.estimateValueSize(v.Index(i))
			size += elemSize
		}
	case reflect.Map:
		isFixed = false
		size = 0
		for _, key := range v.MapKeys() {
			kSize, _ := mt.estimateValueSize(key)
			vSize, _ := mt.estimateValueSize(v.MapIndex(key))
			size += kSize + vSize
		}
	case reflect.Struct:
		size = 0
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanInterface() {
				fSize, fFixed := mt.estimateValueSize(v.Field(i))
				size += fSize
				if !fFixed {
					isFixed = false
				}
			}
		}
	case reflect.Ptr, reflect.Interface:
		isFixed = false
		if v.IsNil() {
			size = int64(unsafe.Sizeof(uintptr(0)))
		} else {
			elemSize, _ := mt.estimateValueSize(v.Elem())
			size = int64(unsafe.Sizeof(uintptr(0))) + elemSize
		}
	default:
		// For unknown types, use the type's size
		size = int64(t.Size())
	}

	// Update cache if needed
	if !ok {
		mt.cacheMutex.Lock()
		if isFixed {
			mt.sizeCache[t] = size
		} else {
			mt.sizeCache[t] = -1 // Mark as variable
		}
		mt.cacheMutex.Unlock()
	}

	return size, isFixed
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
