package tests

import (
	"sync"
	"testing"

	"mpmc-queue/queue"
)

// TestConcurrentDequeueNoRace tests that multiple consumers can safely read the same QueueData
// This is the exact scenario that was causing SIGSEGV before the fix
func TestConcurrentDequeueNoRace(t *testing.T) {
	q := queue.NewQueue("race-test")
	defer q.Close()

	// Enqueue a single item that will be read by many consumers concurrently
	testPayload := "shared data item"
	if err := q.Enqueue(testPayload); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Create many consumers that will all read the same item concurrently
	const numConsumers = 100
	var wg sync.WaitGroup
	wg.Add(numConsumers)

	// Track successful reads
	successCount := 0
	var countMutex sync.Mutex

	for i := 0; i < numConsumers; i++ {
		go func(consumerID int) {
			defer wg.Done()

			consumer := q.AddConsumer()
			data := consumer.Read()

			if data != nil {
				// Access all fields of QueueData - this would race if not properly synchronized
				_ = data.ID
				_ = data.Payload
				_ = data.Created
				_ = data.EnqueueEvent.EventType
				_ = data.EnqueueEvent.QueueName
				_ = data.EnqueueEvent.Timestamp

				// Verify the data
				if data.Payload != testPayload {
					t.Errorf("Consumer %d: expected payload %v, got %v", consumerID, testPayload, data.Payload)
				}

				// Check dequeue history
				history := consumer.GetDequeueHistory()
				if len(history) != 1 {
					t.Errorf("Consumer %d: expected 1 dequeue record, got %d", consumerID, len(history))
				}

				countMutex.Lock()
				successCount++
				countMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	if successCount != numConsumers {
		t.Errorf("Expected %d successful reads, got %d", numConsumers, successCount)
	}
}

// TestMassiveConcurrentDequeue tests extreme concurrent access to shared QueueData
func TestMassiveConcurrentDequeue(t *testing.T) {
	q := queue.NewQueue("massive-race-test")
	defer q.Close()

	// Enqueue multiple items
	const numItems = 10
	for i := 0; i < numItems; i++ {
		if err := q.Enqueue(i); err != nil {
			t.Fatalf("Failed to enqueue item %d: %v", i, err)
		}
	}

	// Create many consumers reading concurrently
	const numConsumers = 200
	var wg sync.WaitGroup
	wg.Add(numConsumers)

	for i := 0; i < numConsumers; i++ {
		go func(consumerID int) {
			defer wg.Done()

			consumer := q.AddConsumer()

			// Each consumer reads all items
			for j := 0; j < numItems; j++ {
				data := consumer.Read()
				if data != nil {
					// Access all fields - would race if QueueData was being modified
					_ = data.ID
					_ = data.Payload
					_ = data.Created
					_ = data.EnqueueEvent
				}
			}

			// Verify dequeue history
			history := consumer.GetDequeueHistory()
			if len(history) != numItems {
				t.Errorf("Consumer %d: expected %d dequeue records, got %d", consumerID, numItems, len(history))
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentProducersAndConsumersNoRace tests simultaneous production and consumption
func TestConcurrentProducersAndConsumersNoRace(t *testing.T) {
	q := queue.NewQueue("producer-consumer-race-test")
	defer q.Close()

	const numProducers = 10
	const numConsumers = 10
	const itemsPerProducer = 50

	var wg sync.WaitGroup

	// Start producers
	wg.Add(numProducers)
	for i := 0; i < numProducers; i++ {
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				payload := map[string]any{
					"producer": producerID,
					"item":     j,
				}
				q.Enqueue(payload)
			}
		}(i)
	}

	// Start consumers immediately (reading while producers are still writing)
	wg.Add(numConsumers)
	for i := 0; i < numConsumers; i++ {
		go func(consumerID int) {
			defer wg.Done()
			consumer := q.AddConsumer()

			// Read until we've consumed a reasonable amount
			readCount := 0
			for readCount < itemsPerProducer {
				data := consumer.Read()
				if data != nil {
					// Access all fields
					_ = data.ID
					_ = data.Payload
					_ = data.Created
					_ = data.EnqueueEvent
					readCount++
				}
			}

			// Verify dequeue history matches read count
			history := consumer.GetDequeueHistory()
			if len(history) != readCount {
				t.Errorf("Consumer %d: read count %d doesn't match history length %d",
					consumerID, readCount, len(history))
			}
		}(i)
	}

	wg.Wait()
}

// TestQueueDataImmutability verifies that QueueData cannot be modified after creation
func TestQueueDataImmutability(t *testing.T) {
	q := queue.NewQueue("immutable-test")
	defer q.Close()

	testPayload := "immutable payload"
	if err := q.Enqueue(testPayload); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Create multiple consumers
	consumer1 := q.AddConsumer()
	consumer2 := q.AddConsumer()

	data1 := consumer1.Read()
	data2 := consumer2.Read()

	if data1 == nil || data2 == nil {
		t.Fatal("Expected data from both consumers")
	}

	// Verify both consumers got the exact same QueueData instance (pointer equality)
	if data1 != data2 {
		t.Error("Expected both consumers to read the same QueueData instance")
	}

	// Verify enqueue event is immutable
	if data1.EnqueueEvent.EventType != "enqueue" {
		t.Errorf("Expected enqueue event type, got %s", data1.EnqueueEvent.EventType)
	}

	// Verify each consumer has independent dequeue history
	history1 := consumer1.GetDequeueHistory()
	history2 := consumer2.GetDequeueHistory()

	if len(history1) != 1 || len(history2) != 1 {
		t.Errorf("Expected 1 dequeue record per consumer, got %d and %d", len(history1), len(history2))
	}

	// Verify dequeue records are for the same data but tracked separately
	if history1[0].DataID != history2[0].DataID {
		t.Error("Both consumers should have dequeued the same data ID")
	}

	// Verify histories are independent (not the same slice)
	if &history1[0] == &history2[0] {
		t.Error("Dequeue histories should be independent for each consumer")
	}
}
