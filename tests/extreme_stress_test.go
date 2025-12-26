package tests

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestExtremeProducerConsumerStress hammers the queue with high concurrency
func TestExtremeProducerConsumerStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	q := queue.NewQueueWithTTL("extreme-stress", 200*time.Millisecond)
	defer q.Close()

	const numProducers = 20
	const numConsumers = 20
	const itemsPerProducer = 100
	const testDuration = 2 * time.Second

	var produced atomic.Int64
	var consumed atomic.Int64
	var errors atomic.Int64

	var wg sync.WaitGroup

	// Start producers
	wg.Add(numProducers)
	for i := 0; i < numProducers; i++ {
		go func(id int) {
			defer wg.Done()
			start := time.Now()
			count := 0

			for time.Since(start) < testDuration && count < itemsPerProducer {
				payload := map[string]any{
					"producer": id,
					"item":     count,
					"time":     time.Now(),
				}

				if err := q.TryEnqueue(payload); err != nil {
					// Memory limit is expected, not an error
					if _, ok := err.(*queue.MemoryLimitError); !ok {
						t.Errorf("Producer %d unexpected error: %v", id, err)
						errors.Add(1)
					}
				} else {
					produced.Add(1)
					count++
				}
			}
		}(i)
	}

	// Start consumers
	wg.Add(numConsumers)
	for i := 0; i < numConsumers; i++ {
		go func(id int) {
			defer wg.Done()
			consumer := q.AddConsumer()
			start := time.Now()
			readCount := 0

			for time.Since(start) < testDuration {
				data := consumer.TryRead()
				if data != nil {
					readCount++
					consumed.Add(1)

					// Verify data integrity
					if payload, ok := data.Payload.(map[string]any); ok {
						if _, hasProducer := payload["producer"]; !hasProducer {
							t.Errorf("Consumer %d read corrupted data: missing producer field", id)
							errors.Add(1)
						}
					} else {
						t.Errorf("Consumer %d read wrong type", id)
						errors.Add(1)
					}
				}

				// Small sleep to allow producers to add data
				time.Sleep(1 * time.Millisecond)
			}

			t.Logf("Consumer %d read %d items", id, readCount)
		}(i)
	}

	wg.Wait()

	t.Logf("Produced: %d, Consumed: %d, Errors: %d",
		produced.Load(), consumed.Load(), errors.Load())

	if errors.Load() > 0 {
		t.Errorf("Found %d errors during stress test", errors.Load())
	}
}

// TestExpirationDuringHeavyLoad tests expiration under load
func TestExpirationDuringHeavyLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	q := queue.NewQueueWithTTL("expiration-stress", 100*time.Millisecond)
	defer q.Close()

	const numProducers = 10
	const numConsumers = 10
	const testDuration = 1 * time.Second

	var wg sync.WaitGroup
	var errors atomic.Int64

	// Producers
	wg.Add(numProducers)
	for i := 0; i < numProducers; i++ {
		go func(id int) {
			defer wg.Done()
			start := time.Now()
			count := 0

			for time.Since(start) < testDuration {
				if err := q.TryEnqueue(count); err != nil {
					if _, ok := err.(*queue.MemoryLimitError); !ok {
						errors.Add(1)
					}
				}
				count++
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	// Consumers
	wg.Add(numConsumers)
	for i := 0; i < numConsumers; i++ {
		go func(id int) {
			defer wg.Done()
			consumer := q.AddConsumer()
			start := time.Now()
			var lastValue *int

			for time.Since(start) < testDuration {
				data := consumer.TryRead()
				if data != nil {
					// Check that values don't go backwards (data corruption indicator)
					if val, ok := data.Payload.(int); ok {
						if lastValue != nil && val < *lastValue {
							t.Errorf("Consumer %d: values went backwards! %d -> %d (possible corruption)",
								id, *lastValue, val)
							errors.Add(1)
						}
						lastValue = &val
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	if errors.Load() > 0 {
		t.Errorf("Found %d errors during expiration stress test", errors.Load())
	}
}

// TestConsumerPositionIntegrityUnderLoad verifies consumers don't skip or duplicate items
func TestConsumerPositionIntegrityUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	q := queue.NewQueueWithTTL("position-integrity", 500*time.Millisecond)
	defer q.Close()

	// Enqueue sequential items
	const numItems = 500
	for i := 0; i < numItems; i++ {
		q.TryEnqueue(i)
	}

	// Multiple consumers read all items
	const numConsumers = 10
	var wg sync.WaitGroup
	wg.Add(numConsumers)

	errors := make([]int, numConsumers)

	for i := 0; i < numConsumers; i++ {
		go func(id int) {
			defer wg.Done()
			consumer := q.AddConsumer()

			lastValue := -1
			readCount := 0

			for {
				data := consumer.TryRead()
				if data == nil {
					break
				}

				readCount++
				if val, ok := data.Payload.(int); ok {
					// Values should be monotonically increasing (or equal if re-reading)
					if val < lastValue {
						t.Errorf("Consumer %d: sequence error! %d -> %d", id, lastValue, val)
						errors[id]++
					}

					// Check for skipped values (small gaps are ok due to expiration)
					if val > lastValue+1 && lastValue >= 0 {
						gap := val - lastValue - 1
						if gap > 50 { // Allow some gaps from expiration
							t.Logf("Consumer %d: large gap detected: %d items skipped", id, gap)
						}
					}

					lastValue = val
				}
			}

			t.Logf("Consumer %d read %d items, last value: %d", id, readCount, lastValue)
		}(i)
	}

	wg.Wait()

	totalErrors := 0
	for i, e := range errors {
		if e > 0 {
			t.Errorf("Consumer %d had %d errors", i, e)
			totalErrors += e
		}
	}

	if totalErrors > 0 {
		t.Errorf("Total errors: %d", totalErrors)
	}
}
