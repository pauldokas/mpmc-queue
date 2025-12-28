package tests

import (
	"mpmc-queue/queue"
	"testing"
)

type complexStruct struct {
	ID   int
	Name string
	Tags []string
	Meta map[string]int
}

type SizerStruct struct {
	Data [100]byte
}

func (s SizerStruct) Size() int64 {
	return 100
}

func BenchmarkMemoryEstimation(b *testing.B) {
	mt := queue.NewMemoryTracker(1024 * 1024)

	// Test cases
	intVal := 42
	stringVal := "hello world"
	byteSliceVal := []byte("hello world")
	structVal := complexStruct{
		ID:   1,
		Name: "test",
		Tags: []string{"a", "b", "c"},
		Meta: map[string]int{"x": 1, "y": 2},
	}
	sizerVal := SizerStruct{}

	b.Run("Int", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mt.EstimateQueueDataSize(&queue.QueueData{Payload: intVal})
		}
	})

	b.Run("String", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mt.EstimateQueueDataSize(&queue.QueueData{Payload: stringVal})
		}
	})

	b.Run("ByteSlice", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mt.EstimateQueueDataSize(&queue.QueueData{Payload: byteSliceVal})
		}
	})

	b.Run("Struct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mt.EstimateQueueDataSize(&queue.QueueData{Payload: structVal})
		}
	})

	b.Run("Sizer", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mt.EstimateQueueDataSize(&queue.QueueData{Payload: sizerVal})
		}
	})
}
