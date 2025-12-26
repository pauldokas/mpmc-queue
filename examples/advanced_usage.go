package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"mpmc-queue/queue"
)

func main() {
	fmt.Println("=== Advanced Queue Usage Example ===")

	// Example 1: TTL and Expiration
	fmt.Println("\n1. TTL and Expiration Example:")
	
	// Create queue with 2-second TTL
	q := queue.NewQueueWithTTL("expiration-queue", 2*time.Second)
	defer q.Close()

	// Add some items
	for i := 1; i <= 5; i++ {
		q.TryEnqueue(fmt.Sprintf("Item %d", i))
		fmt.Printf("   Enqueued: Item %d\n", i)
	}

	// Create consumer but don't read immediately
	consumer := q.AddConsumer()
	
	fmt.Printf("   Queue has %d items\n", q.GetQueueStats().TotalItems)
	fmt.Println("   Waiting for items to expire...")
	
	// Wait for items to expire
	time.Sleep(3 * time.Second)
	
	// Force expiration check
	expired := q.ForceExpiration()
	fmt.Printf("   Expired %d items\n", expired)
	
	// Check for expiration notification
	select {
	case expiredCount := <-consumer.GetNotificationChannel():
		fmt.Printf("   Consumer received notification: %d items expired\n", expiredCount)
	default:
		fmt.Println("   No expiration notification received")
	}

	fmt.Printf("   Queue now has %d items\n", q.GetQueueStats().TotalItems)

	// Example 2: Concurrent Producers and Consumers
	fmt.Println("\n2. Concurrent Producers and Consumers:")

	q2 := queue.NewQueue("concurrent-queue")
	defer q2.Close()

	const numProducers = 3
	const numConsumers = 2
	const itemsPerProducer = 10

	var wg sync.WaitGroup

	// Start producers
	fmt.Printf("   Starting %d producers...\n", numProducers)
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				item := fmt.Sprintf("Producer%d-Item%d", producerID, j)
				if err := q2.Enqueue(item); err != nil {
					log.Printf("Producer %d error: %v", producerID, err)
				} else {
					fmt.Printf("   Producer %d: enqueued %s\n", producerID, item)
				}
				time.Sleep(10 * time.Millisecond) // Small delay
			}
		}(i)
	}

	// Start consumers
	fmt.Printf("   Starting %d consumers...\n", numConsumers)
	consumedCount := make([]int, numConsumers)
	
	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func(consumerID int) {
			defer wg.Done()
			consumer := q2.AddConsumer()
			fmt.Printf("   Consumer %d ID: %s\n", consumerID, consumer.GetID())
			
			for {
				data := consumer.TryRead()
				if data == nil {
					time.Sleep(50 * time.Millisecond)
					// Check if we should stop (no more producers)
					if q2.GetQueueStats().TotalItems == 0 {
						break
					}
					continue
				}
				consumedCount[consumerID]++
				fmt.Printf("   Consumer %d: read %s\n", consumerID, data.Payload)
			}
		}(i)
	}

	// Wait for producers to finish
	time.Sleep(500 * time.Millisecond)
	wg.Wait()

	// Final statistics
	fmt.Printf("   Final queue stats: %d items remaining\n", q2.GetQueueStats().TotalItems)
	for i, count := range consumedCount {
		fmt.Printf("   Consumer %d consumed: %d items\n", i, count)
	}

	// Example 3: Memory Management
	fmt.Println("\n3. Memory Management Example:")

	q3 := queue.NewQueue("memory-queue")
	defer q3.Close()

	// Create large payloads to test memory limits
	largePayload := make([]byte, 50*1024) // 50KB payload
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}

	fmt.Printf("   Payload size: %d bytes\n", len(largePayload))
	
	// Enqueue until memory limit is reached
	count := 0
	for {
		err := q3.Enqueue(largePayload)
		if err != nil {
			fmt.Printf("   Memory limit reached after %d items\n", count)
			fmt.Printf("   Error: %v\n", err)
			break
		}
		count++
		
		if count%5 == 0 {
			stats := q3.GetQueueStats()
			fmt.Printf("   Enqueued %d items, memory usage: %d bytes (%.1f%%)\n",
				count, stats.MemoryUsage, stats.MemoryPercent)
		}
	}

	// Example 4: Dynamic TTL Changes
	fmt.Println("\n4. Dynamic TTL Changes:")

	q4 := queue.NewQueueWithTTL("dynamic-ttl-queue", 5*time.Second)
	defer q4.Close()

	// Add items with long TTL
	for i := 1; i <= 3; i++ {
		q4.Enqueue(fmt.Sprintf("Long TTL Item %d", i))
	}
	
	fmt.Printf("   Added items with TTL: %v\n", q4.GetTTL())
	fmt.Printf("   Queue has %d items\n", q4.GetQueueStats().TotalItems)
	
	// Change TTL to very short
	q4.SetTTL(100 * time.Millisecond)
	fmt.Printf("   Changed TTL to: %v\n", q4.GetTTL())
	
	// Wait and check expiration
	time.Sleep(200 * time.Millisecond)
	expired = q4.ForceExpiration()
	fmt.Printf("   Expired %d items with new TTL\n", expired)
	fmt.Printf("   Queue now has %d items\n", q4.GetQueueStats().TotalItems)

	// Example 5: Consumer Position Recovery
	fmt.Println("\n5. Consumer Position Recovery:")

	q5 := queue.NewQueue("recovery-queue")
	defer q5.Close()

	// Add items
	for i := 1; i <= 10; i++ {
		q5.Enqueue(fmt.Sprintf("Recovery Item %d", i))
	}

	// Create consumer and read some items
	consumer1 := q5.AddConsumer()
	fmt.Printf("   Consumer created, reading 5 items...\n")
	for i := 0; i < 5; i++ {
		data := consumer1.Read()
		if data != nil {
			fmt.Printf("   Read: %s\n", data.Payload)
		}
	}

	// Create new consumer (should read from beginning)
	consumer2 := q5.AddConsumer()
	fmt.Printf("   New consumer created, reading first 3 items...\n")
	for i := 0; i < 3; i++ {
		data := consumer2.Read()
		if data != nil {
			fmt.Printf("   New consumer read: %s\n", data.Payload)
		}
	}

	// Show consumer statistics
	fmt.Println("   Consumer statistics:")
	stats := q5.GetConsumerStats()
	for _, cs := range stats {
		fmt.Printf("   Consumer %s: read %d, unread %d\n", 
			cs.ID, cs.TotalItemsRead, cs.UnreadItems)
	}

	fmt.Println("\n=== Advanced Example Complete ===")
}