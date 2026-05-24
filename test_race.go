package main

import (
	"fmt"
	"mpmc-queue/queue"
	"sync"
	"time"
)

func main() {
	q := queue.NewQueueWithConfig("race-test", queue.QueueConfig{
		TTL: 1 * time.Millisecond,
		ExpirationCheckInterval: 1 * time.Millisecond,
	})
	
	// Add consumers
	for i := 0; i < 5; i++ {
		q.AddConsumer()
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			q.TryEnqueue("test data")
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			q.ForceExpiration()
		}
	}()

	wg.Wait()
	fmt.Println("Done")
}
