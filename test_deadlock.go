package main

import (
	"fmt"
	"sync"
	"time"
	"mpmc-queue/queue"
	"runtime"
)

func main() {
	runtime.GOMAXPROCS(4)
	q := queue.NewQueue("deadlock-test")

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < 50000; i++ {
			q.GetConsumerStats()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 50000; i++ {
			c := q.AddConsumer()
			q.RemoveConsumer(c.GetID())
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 50000; i++ {
			q.TryEnqueue("test")
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("No deadlock detected")
	case <-time.After(5 * time.Second):
		fmt.Println("DEADLOCK DETECTED!")
	}
}
