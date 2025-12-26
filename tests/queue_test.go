package tests

import (
	"sync"
	"testing"

	"mpmc-queue/queue"
)

func TestNewQueue(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	if q.GetName() != "test-queue" {
		t.Errorf("Expected queue name 'test-queue', got '%s'", q.GetName())
	}

	if !q.IsEmpty() {
		t.Error("New queue should be empty")
	}

	stats := q.GetQueueStats()
	if stats.TotalItems != 0 {
		t.Errorf("Expected 0 items, got %d", stats.TotalItems)
	}
}

func TestEnqueueDequeue(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Test enqueue
	testData := "test payload"
	err := q.TryEnqueue(testData)
	if err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	if q.IsEmpty() {
		t.Error("Queue should not be empty after enqueue")
	}

	// Test consumer
	consumer := q.AddConsumer()
	data := consumer.TryRead()

	if data == nil {
		t.Fatal("Expected data, got nil")
	}

	if data.Payload != testData {
		t.Errorf("Expected payload '%v', got '%v'", testData, data.Payload)
	}

	// Verify enqueue event
	if data.EnqueueEvent.EventType != "enqueue" {
		t.Errorf("Enqueue event should be 'enqueue', got '%s'", data.EnqueueEvent.EventType)
	}

	if data.EnqueueEvent.QueueName != "test-queue" {
		t.Errorf("Expected queue name 'test-queue', got '%s'", data.EnqueueEvent.QueueName)
	}

	// Verify dequeue was recorded in consumer history
	history := consumer.GetDequeueHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 dequeue record, got %d", len(history))
	}

	if len(history) > 0 && history[0].DataID != data.ID {
		t.Errorf("Expected dequeue record for data ID %s, got %s", data.ID, history[0].DataID)
	}
}

func TestMultipleConsumers(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Enqueue test data
	payloads := []any{"item1", "item2", "item3"}
	for _, payload := range payloads {
		if err := q.TryEnqueue(payload); err != nil {
			t.Fatalf("Failed to enqueue %v: %v", payload, err)
		}
	}

	// Create multiple consumers
	consumer1 := q.AddConsumer()
	consumer2 := q.AddConsumer()

	// Both consumers should read all items independently
	for i, expectedPayload := range payloads {
		data1 := consumer1.TryRead()
		data2 := consumer2.TryRead()

		if data1 == nil || data2 == nil {
			t.Fatalf("Consumer read failed at index %d", i)
		}

		if data1.Payload != expectedPayload || data2.Payload != expectedPayload {
			t.Errorf("Index %d: expected %v, got consumer1=%v, consumer2=%v",
				i, expectedPayload, data1.Payload, data2.Payload)
		}
	}

	// No more data should be available
	if consumer1.TryRead() != nil || consumer2.TryRead() != nil {
		t.Error("No more data should be available")
	}
}

func TestNewConsumerReadsAllData(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Enqueue data
	payloads := []any{"item1", "item2", "item3"}
	for _, payload := range payloads {
		q.TryEnqueue(payload)
	}

	// Create consumer and read some data
	consumer1 := q.AddConsumer()
	consumer1.TryRead() // Read first item

	// Add more data
	q.TryEnqueue("item4")

	// Create new consumer
	consumer2 := q.AddConsumer()

	// New consumer should read all data from beginning
	allData := []any{"item1", "item2", "item3", "item4"}
	for i, expectedPayload := range allData {
		data := consumer2.TryRead()
		if data == nil {
			t.Fatalf("Consumer2 failed to read item at index %d", i)
		}
		if data.Payload != expectedPayload {
			t.Errorf("Index %d: expected %v, got %v", i, expectedPayload, data.Payload)
		}
	}
}

func TestConcurrentProducers(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	const numProducers = 10
	const itemsPerProducer = 100
	var wg sync.WaitGroup

	// Start concurrent producers
	wg.Add(numProducers)
	for i := 0; i < numProducers; i++ {
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				payload := map[string]int{"producer": producerID, "item": j}
				if err := q.TryEnqueue(payload); err != nil {
					t.Errorf("Producer %d failed to enqueue item %d: %v", producerID, j, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify total items
	stats := q.GetQueueStats()
	expectedTotal := int64(numProducers * itemsPerProducer)
	if stats.TotalItems != expectedTotal {
		t.Errorf("Expected %d total items, got %d", expectedTotal, stats.TotalItems)
	}

	// Verify consumer can read all items
	consumer := q.AddConsumer()
	count := int64(0)
	for {
		data := consumer.TryRead()
		if data == nil {
			break
		}
		count++
	}

	if count != expectedTotal {
		t.Errorf("Consumer read %d items, expected %d", count, expectedTotal)
	}
}

func TestConcurrentConsumers(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	const numItems = 1000
	const numConsumers = 5

	// Enqueue items
	for i := 0; i < numItems; i++ {
		q.TryEnqueue(i)
	}

	// Create consumers and track what they read
	consumers := make([]*queue.Consumer, numConsumers)
	itemsCounts := make([]int, numConsumers)
	var wg sync.WaitGroup

	for i := 0; i < numConsumers; i++ {
		consumers[i] = q.AddConsumer()
		wg.Add(1)
		go func(consumerIndex int) {
			defer wg.Done()
			consumer := consumers[consumerIndex]
			for {
				data := consumer.TryRead()
				if data == nil {
					break
				}
				itemsCounts[consumerIndex]++
			}
		}(i)
	}

	wg.Wait()

	// Each consumer should have read all items
	for i, count := range itemsCounts {
		if count != numItems {
			t.Errorf("Consumer %d read %d items, expected %d", i, count, numItems)
		}
	}
}

func TestMemoryLimit(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Create large payloads to test memory limits
	largePayload := make([]byte, 100*1024) // 100KB payload
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}

	// Enqueue items until memory limit is reached
	enqueueCount := 0
	for {
		err := q.TryEnqueue(largePayload)
		if err != nil {
			// Should get memory limit error
			if _, isMemoryError := err.(*queue.MemoryLimitError); !isMemoryError {
				t.Errorf("Expected MemoryLimitError, got %T: %v", err, err)
			}
			break
		}
		enqueueCount++

		// Prevent infinite loop in case memory limit isn't working
		if enqueueCount > 20 {
			t.Error("Memory limit not enforced - too many items enqueued")
			break
		}
	}

	// Verify memory usage is near limit
	stats := q.GetQueueStats()
	if stats.MemoryPercent < 90 {
		t.Errorf("Memory usage should be near 100%%, got %.2f%%", stats.MemoryPercent)
	}

	t.Logf("Enqueued %d large items, memory usage: %d bytes (%.2f%%)",
		enqueueCount, stats.MemoryUsage, stats.MemoryPercent)
}

func TestEnqueueBatch(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Test batch enqueue
	payloads := []any{"batch1", "batch2", "batch3", "batch4", "batch5"}
	err := q.TryEnqueueBatch(payloads)
	if err != nil {
		t.Fatalf("Failed to enqueue batch: %v", err)
	}

	// Verify all items are in queue
	stats := q.GetQueueStats()
	if stats.TotalItems != int64(len(payloads)) {
		t.Errorf("Expected %d items, got %d", len(payloads), stats.TotalItems)
	}

	// Verify consumer can read all items in order
	consumer := q.AddConsumer()
	for i, expectedPayload := range payloads {
		data := consumer.TryRead()
		if data == nil {
			t.Fatalf("Failed to read item at index %d", i)
		}
		if data.Payload != expectedPayload {
			t.Errorf("Index %d: expected %v, got %v", i, expectedPayload, data.Payload)
		}
	}
}

func TestChunkedStorage(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Enqueue more than 1000 items to test multiple chunks
	const numItems = 2500
	for i := 0; i < numItems; i++ {
		err := q.TryEnqueue(i)
		if err != nil {
			t.Fatalf("Failed to enqueue item %d: %v", i, err)
		}
	}

	// Verify all items are stored
	stats := q.GetQueueStats()
	if stats.TotalItems != numItems {
		t.Errorf("Expected %d items, got %d", stats.TotalItems, numItems)
	}

	// Verify consumer can read all items in order
	consumer := q.AddConsumer()
	for i := 0; i < numItems; i++ {
		data := consumer.TryRead()
		if data == nil {
			t.Fatalf("Failed to read item at index %d", i)
		}
		if data.Payload.(int) != i {
			t.Errorf("Index %d: expected %d, got %v", i, i, data.Payload)
		}
	}

	// No more data should be available
	if consumer.TryRead() != nil {
		t.Error("No more data should be available")
	}
}

func TestConsumerStats(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Enqueue items
	for i := 0; i < 10; i++ {
		q.TryEnqueue(i)
	}

	consumer := q.AddConsumer()

	// Check initial stats
	stats := consumer.GetStats()
	if stats.TotalItemsRead != 0 {
		t.Errorf("Expected 0 items read initially, got %d", stats.TotalItemsRead)
	}
	if stats.UnreadItems != 10 {
		t.Errorf("Expected 10 unread items, got %d", stats.UnreadItems)
	}

	// Read some items
	for i := 0; i < 5; i++ {
		consumer.TryRead()
	}

	// Check updated stats
	stats = consumer.GetStats()
	if stats.TotalItemsRead != 5 {
		t.Errorf("Expected 5 items read, got %d", stats.TotalItemsRead)
	}
	if stats.UnreadItems != 5 {
		t.Errorf("Expected 5 unread items, got %d", stats.UnreadItems)
	}
}
