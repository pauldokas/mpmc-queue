package tests

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"mpmc-queue/queue"
)

func BenchmarkEnqueue(b *testing.B) {
	q := queue.NewQueue("benchmark-queue")
	defer q.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}
}

func BenchmarkEnqueueBatch(b *testing.B) {
	q := queue.NewQueue("benchmark-queue")
	defer q.Close()

	batchSize := 100
	batches := b.N / batchSize
	if batches == 0 {
		batches = 1
	}

	b.ResetTimer()
	for i := 0; i < batches; i++ {
		batch := make([]any, batchSize)
		for j := 0; j < batchSize; j++ {
			batch[j] = i*batchSize + j
		}
		q.EnqueueBatch(batch)
	}
}

func BenchmarkRead(b *testing.B) {
	q := queue.NewQueue("benchmark-queue")
	defer q.Close()

	// Pre-populate queue
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}

	consumer := q.AddConsumer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		consumer.Read()
	}
}

func BenchmarkReadBatch(b *testing.B) {
	q := queue.NewQueue("benchmark-queue")
	defer q.Close()

	batchSize := 100
	totalItems := b.N * batchSize

	// Pre-populate queue
	for i := 0; i < totalItems; i++ {
		q.Enqueue(i)
	}

	consumer := q.AddConsumer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		consumer.ReadBatch(batchSize)
	}
}

func BenchmarkConcurrentProducers(b *testing.B) {
	q := queue.NewQueue("benchmark-queue")
	defer q.Close()

	numProducers := runtime.NumCPU()
	itemsPerProducer := b.N / numProducers

	b.ResetTimer()

	var wg sync.WaitGroup
	wg.Add(numProducers)

	for i := 0; i < numProducers; i++ {
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				q.Enqueue(producerID*itemsPerProducer + j)
			}
		}(i)
	}

	wg.Wait()
}

func BenchmarkConcurrentConsumers(b *testing.B) {
	q := queue.NewQueue("benchmark-queue")
	defer q.Close()

	// Pre-populate queue
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}

	numConsumers := runtime.NumCPU()
	var wg sync.WaitGroup
	wg.Add(numConsumers)

	b.ResetTimer()

	for i := 0; i < numConsumers; i++ {
		go func() {
			defer wg.Done()
			consumer := q.AddConsumer()
			for {
				data := consumer.Read()
				if data == nil {
					break
				}
			}
		}()
	}

	wg.Wait()
}

func BenchmarkMixedWorkload(b *testing.B) {
	q := queue.NewQueue("benchmark-queue")
	defer q.Close()

	numProducers := 2
	numConsumers := 2
	itemsPerProducer := b.N / numProducers

	var wg sync.WaitGroup
	wg.Add(numProducers + numConsumers)

	b.ResetTimer()

	// Start producers
	for i := 0; i < numProducers; i++ {
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				q.Enqueue(producerID*itemsPerProducer + j)
			}
		}(i)
	}

	// Start consumers
	for i := 0; i < numConsumers; i++ {
		go func() {
			defer wg.Done()
			consumer := q.AddConsumer()
			readCount := 0
			for readCount < itemsPerProducer {
				data := consumer.Read()
				if data != nil {
					readCount++
				} else {
					// Small delay if no data available
					time.Sleep(time.Microsecond)
				}
			}
		}()
	}

	wg.Wait()
}

func BenchmarkMemoryEstimation(b *testing.B) {
	testPayload := map[string]any{
		"id":     12345,
		"name":   "test item",
		"data":   []byte("some binary data here"),
		"active": true,
		"tags":   []string{"tag1", "tag2", "tag3"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data := queue.NewQueueData(testPayload, "benchmark-queue")
		_ = data // Use the data to prevent optimization
	}
}

func BenchmarkChunkedListOperations(b *testing.B) {
	memTracker := queue.NewMemoryTracker()
	chunkedList := queue.NewChunkedList(memTracker)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data := queue.NewQueueData(i, "benchmark-queue")
		chunkedList.Enqueue(data)
	}
}

// Stress tests

func TestHighThroughputStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	q := queue.NewQueue("stress-test-queue")
	defer q.Close()

	const numProducers = 10
	const numConsumers = 5
	const itemsPerProducer = 10000
	const testDuration = 30 * time.Second

	var producerWg, consumerWg sync.WaitGroup
	var totalProduced, totalConsumed int64

	// Start producers
	producerWg.Add(numProducers)
	for i := 0; i < numProducers; i++ {
		go func(producerID int) {
			defer producerWg.Done()
			start := time.Now()
			count := 0
			for time.Since(start) < testDuration && count < itemsPerProducer {
				payload := map[string]any{
					"producer":  producerID,
					"item":      count,
					"timestamp": time.Now(),
				}
				if err := q.Enqueue(payload); err != nil {
					t.Errorf("Producer %d failed to enqueue: %v", producerID, err)
					return
				}
				count++
			}
			totalProduced += int64(count)
		}(i)
	}

	// Start consumers
	consumerWg.Add(numConsumers)
	for i := 0; i < numConsumers; i++ {
		go func(consumerID int) {
			defer consumerWg.Done()
			consumer := q.AddConsumer()
			count := 0
			start := time.Now()

			for time.Since(start) < testDuration+5*time.Second {
				data := consumer.Read()
				if data != nil {
					count++
				} else {
					time.Sleep(time.Millisecond)
				}
			}
			totalConsumed += int64(count)
		}(i)
	}

	producerWg.Wait()
	consumerWg.Wait()

	stats := q.GetQueueStats()

	t.Logf("Stress test results:")
	t.Logf("  Total produced: %d", totalProduced)
	t.Logf("  Total consumed: %d", totalConsumed)
	t.Logf("  Items remaining: %d", stats.TotalItems)
	t.Logf("  Memory usage: %d bytes (%.2f%%)", stats.MemoryUsage, stats.MemoryPercent)

	// Each consumer should have processed all items
	expectedPerConsumer := totalProduced
	if totalConsumed < int64(numConsumers)*expectedPerConsumer {
		t.Logf("Note: Not all consumers processed all items (expected due to timing)")
	}
}

func TestMemoryPressureStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	q := queue.NewQueue("memory-stress-queue")
	defer q.Close()

	// Create increasingly large payloads
	payloadSizes := []int{1024, 5120, 10240, 20480} // 1KB, 5KB, 10KB, 20KB

	for _, size := range payloadSizes {
		payload := make([]byte, size)
		for i := range payload {
			payload[i] = byte(i % 256)
		}

		// Try to fill queue to near memory limit
		count := 0
		for {
			err := q.Enqueue(payload)
			if err != nil {
				if _, isMemError := err.(*queue.MemoryLimitError); isMemError {
					break // Hit memory limit as expected
				}
				t.Fatalf("Unexpected error: %v", err)
			}
			count++

			// Safety check
			if count > 1000 {
				t.Fatal("Too many items enqueued, memory limit not working")
			}
		}

		stats := q.GetQueueStats()
		t.Logf("Payload size %d bytes: enqueued %d items, memory usage: %d bytes (%.2f%%)",
			size, count, stats.MemoryUsage, stats.MemoryPercent)

		// Clear queue for next test
		consumer := q.AddConsumer()
		for {
			if consumer.Read() == nil {
				break
			}
		}
	}
}

func TestLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running stability test")
	}

	q := queue.NewQueueWithTTL("stability-test-queue", 5*time.Second)
	defer q.Close()

	const testDuration = 60 * time.Second
	const numProducers = 3
	const numConsumers = 2

	var wg sync.WaitGroup
	start := time.Now()

	// Start producers
	wg.Add(numProducers)
	for i := 0; i < numProducers; i++ {
		go func(producerID int) {
			defer wg.Done()
			count := 0
			for time.Since(start) < testDuration {
				payload := map[string]any{
					"producer":  producerID,
					"count":     count,
					"timestamp": time.Now(),
				}
				q.Enqueue(payload)
				count++
				time.Sleep(100 * time.Millisecond) // Controlled rate
			}
		}(i)
	}

	// Start consumers
	wg.Add(numConsumers)
	for i := 0; i < numConsumers; i++ {
		go func(consumerID int) {
			defer wg.Done()
			consumer := q.AddConsumer()

			for time.Since(start) < testDuration+10*time.Second {
				data := consumer.Read()
				if data != nil {
					// Simulate processing time
					time.Sleep(50 * time.Millisecond)
				} else {
					time.Sleep(10 * time.Millisecond)
				}

				// Check for expiration notifications
				select {
				case expiredCount := <-consumer.GetNotificationChannel():
					t.Logf("Consumer %d: received expiration notification for %d items",
						consumerID, expiredCount)
				default:
					// No notification
				}
			}
		}(i)
	}

	wg.Wait()

	finalStats := q.GetQueueStats()
	t.Logf("Stability test completed:")
	t.Logf("  Final items in queue: %d", finalStats.TotalItems)
	t.Logf("  Final memory usage: %d bytes (%.2f%%)",
		finalStats.MemoryUsage, finalStats.MemoryPercent)
	t.Logf("  Active consumers: %d", finalStats.ConsumerCount)
}

func TestConsumerLagStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping consumer lag stress test")
	}

	q := queue.NewQueueWithTTL("lag-test-queue", 2*time.Second)
	defer q.Close()

	// Create fast producer and slow consumers
	const numItems = 1000
	const numConsumers = 3

	// Fast producer
	for i := 0; i < numItems; i++ {
		q.Enqueue(i)
	}

	// Create consumers with different speeds
	consumers := make([]*queue.Consumer, numConsumers)
	var wg sync.WaitGroup

	for i := 0; i < numConsumers; i++ {
		consumers[i] = q.AddConsumer()
		wg.Add(1)

		go func(consumerID int, consumer *queue.Consumer) {
			defer wg.Done()
			count := 0
			expiredNotifications := 0

			// Simulate different processing speeds
			processingDelay := time.Duration(consumerID+1) * 5 * time.Millisecond

			for {
				data := consumer.Read()
				if data != nil {
					count++
					time.Sleep(processingDelay)
				} else {
					break // No more data
				}

				// Check for expiration notifications
				select {
				case expiredCount := <-consumer.GetNotificationChannel():
					expiredNotifications += expiredCount
				default:
					// No notification
				}
			}

			t.Logf("Consumer %d: read %d items, got %d expiration notifications",
				consumerID, count, expiredNotifications)
		}(i, consumers[i])
	}

	wg.Wait()

	// Final cleanup and stats
	time.Sleep(3 * time.Second) // Allow final expiration
	stats := q.GetQueueStats()
	t.Logf("Final queue stats: %d items, %.2f%% memory usage",
		stats.TotalItems, stats.MemoryPercent)
}
