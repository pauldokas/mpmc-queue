package queue

import (
	"reflect"
	"sync"
	"sync/atomic"
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

// Sizeable allows structs to report their own size, bypassing reflection.
type Sizeable interface {
	Size() int
}

// MemoryTracker tracks memory usage for the queue
type MemoryTracker struct {
	totalMemory atomic.Int64
	maxMemory   int64
	sizeCache   sync.Map
}

// NewMemoryTracker creates a new memory tracker
func NewMemoryTracker(maxMemory int64) *MemoryTracker {
	if maxMemory <= 0 {
		maxMemory = MaxQueueMemory
	}
	mt := &MemoryTracker{
		maxMemory: maxMemory,
	}
	mt.totalMemory.Store(0)
	return mt
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

// CanAddData checks if adding the data would exceed memory limit
func (mt *MemoryTracker) CanAddData(data *QueueData) bool {
	return mt.totalMemory.Load()+data.GetSize() <= mt.maxMemory
}

// AddData adds memory usage for the data
func (mt *MemoryTracker) AddData(data *QueueData) {
	mt.totalMemory.Add(data.GetSize())
}

// RemoveData removes memory usage for the data
func (mt *MemoryTracker) RemoveData(data *QueueData) {
	newVal := mt.totalMemory.Add(-data.GetSize())
	if newVal < 0 {
		mt.totalMemory.Store(0)
	}
}

// estimatePayloadSize estimates the size of an arbitrary payload
func (mt *MemoryTracker) estimatePayloadSize(payload any) int64 {
	if payload == nil {
		return 0
	}

	// Fast path for common types to avoid reflection overhead
	switch v := payload.(type) {
	case string:
		return int64(len(v))
	case []byte:
		return int64(len(v))
	case int:
		return int64(unsafe.Sizeof(v))
	case int64:
		return int64(unsafe.Sizeof(v))
	case int32:
		return int64(unsafe.Sizeof(v))
	case int16:
		return int64(unsafe.Sizeof(v))
	case int8:
		return 1
	case uint:
		return int64(unsafe.Sizeof(v))
	case uint64:
		return int64(unsafe.Sizeof(v))
	case uint32:
		return int64(unsafe.Sizeof(v))
	case uint16:
		return int64(unsafe.Sizeof(v))
	case uint8:
		return 1
	case bool:
		return 1
	case float64:
		return int64(unsafe.Sizeof(v))
	case float32:
		return int64(unsafe.Sizeof(v))
	}

	// Check for Size() method (Sizeable interface)
	if s, ok := payload.(Sizeable); ok {
		size := s.Size()
		if size < 0 {
			return 0
		}
		return int64(size)
	}

	v := reflect.ValueOf(payload)
	size, _ := mt.estimateValueSize(v, make(map[uintptr]bool))
	return size
}

// estimateValueSize recursively estimates the size of a reflect.Value
// Returns size and whether the type has a fixed size
func (mt *MemoryTracker) estimateValueSize(v reflect.Value, visited map[uintptr]bool) (int64, bool) {
	if !v.IsValid() {
		return 0, true
	}

	// Cycle detection for pointers, maps, and slices
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice:
		if !v.IsNil() {
			ptr := v.Pointer()
			if visited[ptr] {
				// Already visited, don't count again to prevent stack overflow on cycles
				return 0, false
			}
			visited[ptr] = true
		}
	}

	t := v.Type()

	// Check cache
	cachedVal, ok := mt.sizeCache.Load(t)
	if ok {
		cached := cachedVal.(int64)
		if cached >= 0 {
			return cached, true
		}
		// If cached is -1, it's variable size, so we must recalculate
		// We fall through to the switch but know it's not fixed
	}

	var size int64
	var isFixed = true

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
			elemSize, elemFixed := mt.estimateValueSize(v.Index(i), visited)
			size += elemSize
			if !elemFixed {
				isFixed = false
			}
		}
	case reflect.Slice:
		isFixed = false
		size = 0
		for i := 0; i < v.Len(); i++ {
			elemSize, _ := mt.estimateValueSize(v.Index(i), visited)
			size += elemSize
		}
	case reflect.Map:
		isFixed = false
		size = 0
		// We cannot safely iterate over a map using reflection (v.MapKeys())
		// because if the user mutates the map concurrently, Go will throw a fatal unrecoverable error.
		// If users need exact memory tracking for maps, they should wrap them in a type that implements Sizeable.
	case reflect.Struct:
		size = 0
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanInterface() {
				fSize, fFixed := mt.estimateValueSize(v.Field(i), visited)
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
			elemSize, _ := mt.estimateValueSize(v.Elem(), visited)
			size = int64(unsafe.Sizeof(uintptr(0))) + elemSize
		}
	default:
		// For unknown types, use the type's size
		size = int64(t.Size())
	}

	// Update cache if needed
	if !ok {
		if isFixed {
			mt.sizeCache.Store(t, size)
		} else {
			mt.sizeCache.Store(t, int64(-1)) // Mark as variable
		}
	}

	return size, isFixed
}

// AddChunk adds memory usage for a chunk
func (mt *MemoryTracker) AddChunk() {
	mt.totalMemory.Add(ChunkNodeSize)
}

// RemoveChunk removes memory usage for a chunk
func (mt *MemoryTracker) RemoveChunk() {
	newVal := mt.totalMemory.Add(-ChunkNodeSize)
	if newVal < 0 {
		mt.totalMemory.Store(0)
	}
}

// GetMemoryUsage returns current memory usage
func (mt *MemoryTracker) GetMemoryUsage() int64 {
	return mt.totalMemory.Load()
}

// GetMemoryUsagePercent returns memory usage as a percentage
func (mt *MemoryTracker) GetMemoryUsagePercent() float64 {
	return float64(mt.totalMemory.Load()) / float64(mt.maxMemory) * 100
}

// IsNearLimit checks if memory usage is near the limit (>90%)
func (mt *MemoryTracker) IsNearLimit() bool {
	return mt.GetMemoryUsagePercent() > 90.0
}
