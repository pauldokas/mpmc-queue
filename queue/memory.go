package queue

import (
	"math"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	MaxQueueMemory = 1024 * 1024

	BaseQueueDataSize = int64(unsafe.Sizeof(QueueData{}))

	BaseQueueEventSize = int64(unsafe.Sizeof(QueueEvent{}))

	ChunkNodeSize = int64(unsafe.Sizeof(ChunkNode{}))
)

type Sizeable interface {
	Size() int
}

type MemoryTracker struct {
	totalMemory atomic.Int64
	maxMemory   int64
	sizeCache   sync.Map
}

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

func (mt *MemoryTracker) GetMaxMemory() int64 {
	return mt.maxMemory
}

func (mt *MemoryTracker) EstimateQueueDataSize(data *QueueData) int64 {
	if data == nil {
		return 0
	}

	size := BaseQueueDataSize

	size = safeAdd(size, mt.estimatePayloadSize(data.Payload))

	size = safeAdd(size, BaseQueueEventSize)
	size = safeAdd(size, int64(len(data.EnqueueEvent.QueueName)))
	size = safeAdd(size, int64(len(data.EnqueueEvent.EventType)))

	return size
}

func safeAdd(a, b int64) int64 {
	if b <= 0 {
		return a
	}
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

func (mt *MemoryTracker) CanAddData(data *QueueData) bool {
	size := data.GetSize()
	if size < 0 {
		return false
	}
	return mt.totalMemory.Load() <= mt.maxMemory-size
}

func (mt *MemoryTracker) AddData(data *QueueData) {
	size := data.GetSize()
	if size > 0 {
		mt.totalMemory.Add(size)
	}
}

func (mt *MemoryTracker) RemoveData(data *QueueData) {
	size := data.GetSize()
	if size <= 0 {
		return
	}
	for {
		current := mt.totalMemory.Load()
		newVal := current - size
		if newVal < 0 {
			newVal = 0
		}
		if mt.totalMemory.CompareAndSwap(current, newVal) {
			break
		}
	}
}

func (mt *MemoryTracker) estimatePayloadSize(payload any) (size int64) {
	defer func() {
		if r := recover(); r != nil {
			size = 1 << 40
		}
	}()

	if payload == nil {
		return 0
	}

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

	if s, ok := payload.(Sizeable); ok {
		sSize := s.Size()
		if sSize < 0 {
			return 0
		}
		return int64(sSize)
	}

	v := reflect.ValueOf(payload)
	size, _ = mt.estimateValueSize(v, nil, 0)
	return size
}

func (mt *MemoryTracker) estimateValueSize(v reflect.Value, visited map[uintptr]bool, depth int) (size int64, isFixed bool) {
	isFixed = true
	if depth > 1000 {
		return 0, false
	}

	if len(visited) > 10000 {
		return 1 << 40, false
	}

	if !v.IsValid() {
		return 0, true
	}

	defer func() {
		if r := recover(); r != nil {
			size = 1 << 40
			isFixed = false
		}
	}()

	if (v.Kind() == reflect.Struct || v.Kind() == reflect.Array) && !v.CanAddr() {
		if v.Type().Size() > uintptr(mt.maxMemory) {
			return 1 << 40, false
		}
		vp := reflect.New(v.Type())
		vp.Elem().Set(v)
		v = vp.Elem()
	}

	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice:
		if !v.IsNil() {
			ptr := v.Pointer()
			if visited != nil && visited[ptr] {
				return 0, false
			}
			if visited == nil {
				visited = make(map[uintptr]bool)
			}
			visited[ptr] = true
		}
	}

	t := v.Type()

	cachedVal, ok := mt.sizeCache.Load(t)
	if ok {
		cached := cachedVal.(int64)
		if cached >= 0 {
			return cached, true
		}
	}

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
		for i := 0; i < v.Len(); i++ {
			elemSize, elemFixed := mt.estimateValueSize(v.Index(i), visited, depth+1)
			size = safeAdd(size, elemSize)
			if !elemFixed {
				isFixed = false
			}
		}
	case reflect.Slice:
		isFixed = false
		size = int64(t.Size())
		n := v.Len()
		if n > 0 {
			elemKind := t.Elem().Kind()
			if elemKind >= reflect.Bool && elemKind <= reflect.Float64 {
				size = safeAdd(size, int64(n)*int64(t.Elem().Size()))
			} else {
				for i := 0; i < n; i++ {
					elemSize, _ := mt.estimateValueSize(v.Index(i), visited, depth+1)
					size = safeAdd(size, elemSize)
				}
			}
		}
	case reflect.Map:
		isFixed = false
		size = 1 << 40
	case reflect.Struct:
		size = 0
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanInterface() && f.CanAddr() {
				f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
			}

			if f.CanInterface() {
				fSize, fFixed := mt.estimateValueSize(f, visited, depth+1)
				size = safeAdd(size, fSize)
				if !fFixed {
					isFixed = false
				}
			} else {
				size = safeAdd(size, int64(f.Type().Size()))
			}
		}
	case reflect.Ptr, reflect.Interface:
		isFixed = false
		size = int64(t.Size())
		if !v.IsNil() {
			elemSize, _ := mt.estimateValueSize(v.Elem(), visited, depth+1)
			size = safeAdd(size, elemSize)
		}
	default:
		size = int64(t.Size())
	}

	if !ok {
		if isFixed {
			mt.sizeCache.Store(t, size)
		} else {
			mt.sizeCache.Store(t, int64(-1))
		}
	}

	return size, isFixed
}

func (mt *MemoryTracker) AddChunk() {
	mt.totalMemory.Add(ChunkNodeSize)
}

func (mt *MemoryTracker) RemoveChunk() {
	for {
		current := mt.totalMemory.Load()
		newVal := current - ChunkNodeSize
		if newVal < 0 {
			newVal = 0
		}
		if mt.totalMemory.CompareAndSwap(current, newVal) {
			break
		}
	}
}

func (mt *MemoryTracker) GetMemoryUsage() int64 {
	return mt.totalMemory.Load()
}

func (mt *MemoryTracker) GetMemoryUsagePercent() float64 {
	return float64(mt.totalMemory.Load()) / float64(mt.maxMemory) * 100
}

func (mt *MemoryTracker) IsNearLimit() bool {
	return mt.GetMemoryUsagePercent() > 90.0
}
