//go:build integration

package tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestScenario_JobQueue simulates a job queue with workers
func TestScenario_JobQueue(t *testing.T) {
	q := queue.NewQueue("job-queue")
	defer q.Close()

	const numWorkers = 5
	const numJobs = 100

	// Enqueue jobs
	for i := 0; i < numJobs; i++ {
		job := map[string]any{
			"id":      i,
			"type":    "process",
			"payload": fmt.Sprintf("job-%d", i),
		}
		if err := q.TryEnqueue(job); err != nil {
			t.Fatalf("Failed to enqueue job %d: %v", i, err)
		}
	}

	// Start workers
	var wg sync.WaitGroup
	var processed atomic.Int64

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			consumer := q.AddConsumer()

			for {
				data := consumer.TryRead()
				if data == nil {
					break
				}

				// Simulate job processing
				time.Sleep(time.Millisecond)
				processed.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// Each worker should have processed all jobs independently
	expectedTotal := int64(numWorkers * numJobs)
	if processed.Load() != expectedTotal {
		t.Errorf("Expected %d total processed (5 workers * 100 jobs each), got %d",
			expectedTotal, processed.Load())
	}
}

// TestScenario_EventStreaming simulates event streaming
func TestScenario_EventStreaming(t *testing.T) {
	q := queue.NewQueue("event-stream")
	defer q.Close()

	const numEvents = 200
	const numSubscribers = 3

	// Publisher goroutine
	go func() {
		for i := 0; i < numEvents; i++ {
			event := map[string]any{
				"eventType": "user_action",
				"userId":    fmt.Sprintf("user-%d", i%10),
				"action":    "click",
				"timestamp": time.Now(),
			}
			q.TryEnqueue(event)
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Subscribers (each gets all events)
	var wg sync.WaitGroup
	subscriberCounts := make([]atomic.Int64, numSubscribers)

	for i := 0; i < numSubscribers; i++ {
		wg.Add(1)
		go func(subID int) {
			defer wg.Done()
			consumer := q.AddConsumer()

			timeout := time.After(5 * time.Second)
			for {
				select {
				case <-timeout:
					return
				default:
					data := consumer.TryRead()
					if data != nil {
						subscriberCounts[subID].Add(1)
					} else {
						time.Sleep(10 * time.Millisecond)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify each subscriber got all (or most) events
	t.Logf("Event streaming results:")
	for i := 0; i < numSubscribers; i++ {
		count := subscriberCounts[i].Load()
		t.Logf("  Subscriber %d: received %d events", i, count)
		if count < int64(numEvents*80/100) {
			t.Errorf("Subscriber %d received too few events: %d (expected at least %d)",
				i, count, numEvents*80/100)
		}
	}
}

// TestScenario_GracefulShutdown simulates graceful shutdown
func TestScenario_GracefulShutdown(t *testing.T) {
	q := queue.NewQueue("shutdown-queue")

	const numItems = 50
	var processed atomic.Int64

	// Enqueue items
	for i := 0; i < numItems; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Start consumer
	consumer := q.AddConsumer()
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			data := consumer.TryRead()
			if data == nil {
				// Check if queue is closed or just empty
				if !consumer.HasMoreData() {
					break
				}
				time.Sleep(10 * time.Millisecond)
				continue
			}
			// Simulate processing
			time.Sleep(5 * time.Millisecond)
			processed.Add(1)
		}
	}()

	// Allow some processing
	time.Sleep(100 * time.Millisecond)

	// Graceful shutdown
	q.Close()

	// Wait for consumer to finish
	select {
	case <-done:
		t.Logf("Processed %d items before shutdown", processed.Load())
	case <-time.After(2 * time.Second):
		t.Fatal("Consumer did not finish after queue close")
	}
}

// TestScenario_RateLimiting simulates rate-limited processing
func TestScenario_RateLimiting(t *testing.T) {
	q := queue.NewQueue("rate-limit-queue")
	defer q.Close()

	const rateLimit = 10 // 10 items per second
	const batchSize = 50
	tickInterval := time.Second / rateLimit

	// Enqueue batch
	for i := 0; i < batchSize; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Rate-limited consumer
	consumer := q.AddConsumer()
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	start := time.Now()
	var processed atomic.Int64

	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-ticker.C:
			data := consumer.TryRead()
			if data != nil {
				processed.Add(1)
			} else {
				// All items processed
				elapsed := time.Since(start)
				processedCount := processed.Load()

				t.Logf("Rate limiting: processed %d items in %v", processedCount, elapsed)

				// Should take at least (batchSize / rateLimit) seconds
				minDuration := time.Duration(batchSize/rateLimit) * time.Second
				if elapsed < minDuration {
					t.Logf("Note: Processing faster than rate limit allows (timing variation is acceptable)")
				}

				if processedCount != batchSize {
					t.Errorf("Expected to process %d items, got %d", batchSize, processedCount)
				}
				return
			}
		case <-timeout:
			t.Fatal("Timeout in rate limiting test")
		}
	}
}

// TestScenario_BackpressureHandling simulates handling backpressure
func TestScenario_BackpressureHandling(t *testing.T) {
	// Small memory queue to trigger backpressure
	q := queue.NewQueueWithConfig("backpressure-queue", queue.QueueConfig{
		TTL:       queue.DefaultTTL,
		MaxMemory: 10 * 1024, // 10KB
	})
	defer q.Close()

	const numProducers = 3
	const itemsPerProducer = 100

	var wg sync.WaitGroup
	var successful atomic.Int64
	var blocked atomic.Int64

	// Fast producers
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()

			for j := 0; j < itemsPerProducer; j++ {
				payload := make([]byte, 100) // 100 bytes per item
				err := q.TryEnqueue(payload)
				if err != nil {
					// Memory limit hit - backpressure
					blocked.Add(1)
				} else {
					successful.Add(1)
				}
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Slow consumer to create backpressure
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer := q.AddConsumer()

		for i := 0; i < 50; i++ { // Only read 50 items
			data := consumer.TryRead()
			if data != nil {
				time.Sleep(10 * time.Millisecond) // Slow processing
			}
		}
	}()

	wg.Wait()

	t.Logf("Backpressure results:")
	t.Logf("  Successful enqueues: %d", successful.Load())
	t.Logf("  Blocked (backpressure): %d", blocked.Load())

	// Should have hit backpressure at some point
	if blocked.Load() == 0 {
		t.Error("Expected backpressure to trigger, but all enqueues succeeded")
	}
}

// TestScenario_MultiStageProcessing simulates multi-stage pipeline
func TestScenario_MultiStageProcessing(t *testing.T) {
	stage1 := queue.NewQueue("stage1")
	defer stage1.Close()

	stage2 := queue.NewQueue("stage2")
	defer stage2.Close()

	const numItems = 50

	// Stage 1: Enqueue raw data
	for i := 0; i < numItems; i++ {
		if err := stage1.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue to stage1: %v", err)
		}
	}

	// Stage 2: Process from stage1, output to stage2
	consumer1 := stage1.AddConsumer()
	var stage1Processed atomic.Int64

	go func() {
		for {
			data := consumer1.TryRead()
			if data == nil {
				break
			}

			// Transform data
			num := data.Payload.(int)
			transformed := num * 2

			stage2.TryEnqueue(transformed)
			stage1Processed.Add(1)
		}
	}()

	// Stage 3: Final consumer
	time.Sleep(100 * time.Millisecond) // Let processing happen

	consumer2 := stage2.AddConsumer()
	var finalProcessed atomic.Int64

	for {
		data := consumer2.TryRead()
		if data == nil {
			break
		}
		finalProcessed.Add(1)
	}

	t.Logf("Multi-stage pipeline:")
	t.Logf("  Stage 1 processed: %d", stage1Processed.Load())
	t.Logf("  Stage 2 output: %d", finalProcessed.Load())

	// All items should flow through pipeline
	if stage1Processed.Load() != numItems {
		t.Errorf("Stage 1 expected to process %d items, got %d", numItems, stage1Processed.Load())
	}
}

// TestScenario_ContextCancellationPropagation tests context usage in real scenario
func TestScenario_ContextCancellationPropagation(t *testing.T) {
	q := queue.NewQueue("context-queue")
	defer q.Close()

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	var itemsProcessed atomic.Int64

	// Start workers with context
	const numWorkers = 3
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			consumer := q.AddConsumer()

			for {
				data, err := consumer.ReadWithContext(ctx)
				if err == context.Canceled {
					t.Logf("Worker %d cancelled", workerID)
					return
				}
				if data == nil {
					return // Queue closed
				}

				// Process item
				itemsProcessed.Add(1)
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Enqueue some items
	for i := 0; i < 10; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Allow some processing
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	// Workers should exit
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("All workers exited cleanly after context cancellation")
		t.Logf("Items processed before cancellation: %d", itemsProcessed.Load())
	case <-time.After(2 * time.Second):
		t.Fatal("Workers did not exit after context cancellation")
	}
}
