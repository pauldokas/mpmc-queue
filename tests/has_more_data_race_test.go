package tests

import (
	"sync"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestHasMoreDataRace tests that HasMoreData() is safe to call concurrently with Enqueue()
func TestHasMoreDataRace(t *testing.T) {
	q := queue.NewQueue("has-more-race-test")
	defer q.Close()

	// Add initial data
	for i := 0; i < 5; i++ {
		q.Enqueue(i)
	}

	consumer := q.AddConsumer()

	// Start a goroutine that continuously calls HasMoreData()
	var wg sync.WaitGroup
	wg.Add(2)

	// Consumer goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			// This should not race with Enqueue()
			hasMore := consumer.HasMoreData()
			_ = hasMore
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Producer goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			q.Enqueue(i)
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
}

// TestHasMoreDataWithReading tests HasMoreData() while actively reading
func TestHasMoreDataWithReading(t *testing.T) {
	q := queue.NewQueue("has-more-reading-test")
	defer q.Close()

	// Add data
	for i := 0; i < 100; i++ {
		q.Enqueue(i)
	}

	consumer := q.AddConsumer()

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine that checks HasMoreData
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			consumer.HasMoreData()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Goroutine that reads data
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			consumer.Read()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
}

// TestMultipleConsumersHasMoreData tests HasMoreData() with multiple consumers
func TestMultipleConsumersHasMoreData(t *testing.T) {
	q := queue.NewQueue("multi-consumer-has-more-test")
	defer q.Close()

	// Add initial data
	for i := 0; i < 50; i++ {
		q.Enqueue(i)
	}

	const numConsumers = 10
	var wg sync.WaitGroup
	wg.Add(numConsumers + 1)

	// Create multiple consumers checking HasMoreData
	for i := 0; i < numConsumers; i++ {
		go func(consumerID int) {
			defer wg.Done()
			consumer := q.AddConsumer()

			for j := 0; j < 20; j++ {
				hasMore := consumer.HasMoreData()
				if hasMore {
					consumer.Read()
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Producer adding more data
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			q.Enqueue(i + 1000)
			time.Sleep(2 * time.Millisecond)
		}
	}()

	wg.Wait()
}
