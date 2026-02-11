package tests

import (
	"mpmc-queue/queue"
	"testing"
)

// StandardStruct uses reflection for size estimation
type StandardStruct struct {
	ID    int
	Name  string
	Data  []byte
	Valid bool
}

// SizeableStruct implements queue.Sizeable for optimized size estimation
type SizeableStruct struct {
	ID    int
	Name  string
	Data  []byte
	Valid bool
}

func (s SizeableStruct) Size() int {
	// Approximate size: int(8) + string(len) + []byte(len) + bool(1)
	return 8 + len(s.Name) + len(s.Data) + 1
}

func BenchmarkEnqueue_Reflection(b *testing.B) {
	config := queue.QueueConfig{
		TTL:                queue.DefaultTTL,
		MaxMemory:          100 * 1024 * 1024, // 100MB to avoid filling up
		MaxConsumerHistory: queue.DefaultMaxConsumerHistory,
	}
	q := queue.NewQueueWithConfig("bench-reflection", config)
	defer q.Close()

	data := StandardStruct{
		ID:    1,
		Name:  "Test Name",
		Data:  make([]byte, 1024),
		Valid: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.TryEnqueue(data)
	}
}

func BenchmarkEnqueue_Sizeable(b *testing.B) {
	config := queue.QueueConfig{
		TTL:                queue.DefaultTTL,
		MaxMemory:          100 * 1024 * 1024, // 100MB to avoid filling up
		MaxConsumerHistory: queue.DefaultMaxConsumerHistory,
	}
	q := queue.NewQueueWithConfig("bench-sizeable", config)
	defer q.Close()

	data := SizeableStruct{
		ID:    1,
		Name:  "Test Name",
		Data:  make([]byte, 1024),
		Valid: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.TryEnqueue(data)
	}
}
