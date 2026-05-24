package tests

import (
	"sync"
	"testing"
	"mpmc-queue/queue"
)

func TestQueue_ConcurrentMapIterationPanic(t *testing.T) {
	t.Parallel()

	config := queue.QueueConfig{
		MaxMemory: 1000000,
	}
	q := queue.NewQueueWithConfig("map-test", config)
	defer q.Close()

	payload := make(map[int]int)
	
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			payload[i] = i
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = q.TryEnqueue(payload)
		}
	}()

	wg.Wait()
}
