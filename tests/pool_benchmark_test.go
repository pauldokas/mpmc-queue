package tests

import (
	"mpmc-queue/queue"
	"testing"
)

type benchSizeable struct{}

func (b benchSizeable) Size() int { return 10 }

func BenchmarkPooling_Churn(b *testing.B) {
	memTracker := queue.NewMemoryTracker(queue.MaxQueueMemory)
	cl := queue.NewChunkedList(memTracker)

	// Use Sizeable payload to minimize allocs
	staticData := queue.NewQueueData(benchSizeable{}, "bench")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Fill 5 chunks (5000 items)
		for j := 0; j < 5000; j++ {
			_ = cl.Enqueue(staticData)
		}

		// Clear (returns chunks to pool)
		cl.Clear()
	}
}
