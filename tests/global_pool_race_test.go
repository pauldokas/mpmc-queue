package tests

import (
	"sync"
	"testing"
	"time"

	"mpmc-queue/queue"
)

func TestGlobalPoolRace(t *testing.T) {
	q1 := queue.NewQueueWithTTL("q1", 10*time.Millisecond)
	defer q1.Close()

	q2 := queue.NewQueue("q2")
	defer q2.Close()

	for i := 0; i < 2000; i++ {
		_ = q1.TryEnqueue(i)
	}

	c1 := q1.AddConsumer()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			c1.TryRead()
			time.Sleep(time.Microsecond)
		}
	}()

	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		for i := 0; i < 10000; i++ {
			_ = q2.TryEnqueue(i)
		}
	}()

	wg.Wait()
}
