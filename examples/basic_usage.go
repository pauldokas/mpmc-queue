package main

import (
	"fmt"
	"log"
	"time"

	"mpmc-queue/queue"
)

func main() {
	// Create a new queue
	q := queue.NewQueue("example-queue")
	defer q.Close()

	fmt.Println("=== Basic Queue Usage Example ===")

	// Basic enqueue/dequeue example
	fmt.Println("\n1. Basic Enqueue/Dequeue:")

	// Enqueue some data
	messages := []string{"Hello", "World", "from", "Queue"}
	for i, msg := range messages {
		err := q.TryEnqueue(msg)
		if err != nil {
			log.Fatalf("Failed to enqueue message %d: %v", i, err)
		}
		fmt.Printf("   Enqueued: %s\n", msg)
	}

	// Create a consumer
	consumer := q.AddConsumer()
	fmt.Printf("   Created consumer: %s\n", consumer.GetID())

	// Read messages
	fmt.Println("   Reading messages:")
	for {
		data := consumer.TryRead()
		if data == nil {
			break
		}
		fmt.Printf("   Dequeued: %s (ID: %s)\n", data.Payload, data.ID)
	}

	// Multiple consumers example
	fmt.Println("\n2. Multiple Consumers:")

	// Add more data
	for i := 1; i <= 5; i++ {
		q.TryEnqueue(fmt.Sprintf("Item %d", i))
	}

	// Create multiple consumers
	consumer1 := q.AddConsumer()
	consumer2 := q.AddConsumer()

	fmt.Printf("   Consumer 1 ID: %s\n", consumer1.GetID())
	fmt.Printf("   Consumer 2 ID: %s\n", consumer2.GetID())

	// Both consumers read independently
	fmt.Println("   Consumer 1 reads:")
	for {
		data := consumer1.TryRead()
		if data == nil {
			break
		}
		fmt.Printf("     %s\n", data.Payload)
	}

	fmt.Println("   Consumer 2 reads:")
	for {
		data := consumer2.TryRead()
		if data == nil {
			break
		}
		fmt.Printf("     %s\n", data.Payload)
	}

	// Batch operations
	fmt.Println("\n3. Batch Operations:")

	// Enqueue batch
	batchData := []any{"Batch1", "Batch2", "Batch3", "Batch4"}
	err := q.TryEnqueueBatch(batchData)
	if err != nil {
		log.Fatalf("Failed to enqueue batch: %v", err)
	}
	fmt.Printf("   Enqueued batch of %d items\n", len(batchData))

	// Read batch
	consumer3 := q.AddConsumer()
	batch := consumer3.TryReadBatch(2)
	fmt.Printf("   Read batch of %d items: %v\n", len(batch), batch)

	// Queue statistics
	fmt.Println("\n4. Queue Statistics:")
	stats := q.GetQueueStats()
	fmt.Printf("   Queue Name: %s\n", stats.Name)
	fmt.Printf("   Total Items: %d\n", stats.TotalItems)
	fmt.Printf("   Memory Usage: %d bytes (%.2f%%)\n", stats.MemoryUsage, stats.MemoryPercent)
	fmt.Printf("   Consumer Count: %d\n", stats.ConsumerCount)
	fmt.Printf("   TTL: %v\n", stats.TTL)

	// Consumer statistics
	fmt.Println("\n5. Consumer Statistics:")
	consumerStats := q.GetConsumerStats()
	for _, cs := range consumerStats {
		fmt.Printf("   Consumer %s: read %d items, %d unread\n",
			cs.ID, cs.TotalItemsRead, cs.UnreadItems)
	}

	// Event history example
	fmt.Println("\n6. Event History:")
	q.TryEnqueue("Tracked Item")
	trackedConsumer := q.AddConsumer()
	// Use TryRead to avoid blocking if the item was somehow already consumed or expired (unlikely here but good practice in examples)
	data := trackedConsumer.TryRead()
	if data != nil {
		fmt.Printf("   Item ID: %s\n", data.ID)
		fmt.Printf("   Enqueue Event:\n")
		event := data.GetEnqueueEvent()
		fmt.Printf("     - %s at %s in queue %s\n",
			event.EventType, event.Timestamp.Format(time.RFC3339), event.QueueName)

		// Show dequeue history from consumer
		fmt.Printf("   Dequeue History:\n")
		history := trackedConsumer.GetDequeueHistory()
		for i, record := range history {
			fmt.Printf("     %d. Dequeued item %s at %s\n",
				i+1, record.DataID, record.Timestamp.Format(time.RFC3339))
		}
	}

	fmt.Println("\n=== Example Complete ===")
}
