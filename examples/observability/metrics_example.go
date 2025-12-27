package main

import (
	"fmt"
	"time"

	"mpmc-queue/queue"
)

func main() {
	fmt.Println("=== Metrics and Observability Example ===\n")

	// Create queue
	q := queue.NewQueue("monitored-queue")
	defer q.Close()

	// Create metrics tracker
	metrics := queue.NewMetrics(q)

	// Simulate some operations
	fmt.Println("Simulating queue operations...")

	// Enqueue with metrics tracking
	for i := 0; i < 100; i++ {
		start := time.Now()
		err := q.TryEnqueue(i)
		duration := time.Since(start)
		metrics.RecordEnqueue(duration, err)
	}

	// Consume with metrics tracking
	consumer := q.AddConsumer()
	for i := 0; i < 50; i++ {
		start := time.Now()
		data := consumer.TryRead()
		duration := time.Since(start)
		if data != nil {
			metrics.RecordDequeue(duration)
		}
	}

	// Get metrics snapshot
	snapshot := metrics.GetSnapshot()

	fmt.Println("\n=== Metrics Snapshot ===")
	fmt.Printf("Queue Name: %s\n", snapshot.QueueName)
	fmt.Printf("Total Items: %d\n", snapshot.TotalItems)
	fmt.Printf("Memory Usage: %d bytes (%.2f%%)\n", snapshot.MemoryUsage, snapshot.MemoryPercent)
	fmt.Printf("Consumer Count: %d\n", snapshot.ConsumerCount)
	fmt.Printf("\nOperations:\n")
	fmt.Printf("  Total Enqueued: %d\n", snapshot.TotalEnqueued)
	fmt.Printf("  Total Dequeued: %d\n", snapshot.TotalDequeued)
	fmt.Printf("  Total Expired: %d\n", snapshot.TotalExpired)
	fmt.Printf("  Enqueue Errors: %d\n", snapshot.EnqueueErrors)
	fmt.Printf("  Memory Limit Hits: %d\n", snapshot.MemoryLimitHits)
	fmt.Printf("\nLatency:\n")
	fmt.Printf("  Avg Enqueue: %v\n", snapshot.AvgEnqueueLatency)
	fmt.Printf("  P95 Enqueue: %v\n", snapshot.P95EnqueueLatency)
	fmt.Printf("  Avg Dequeue: %v\n", snapshot.AvgDequeueLatency)
	fmt.Printf("  P95 Dequeue: %v\n", snapshot.P95DequeueLatency)

	// Prometheus format
	fmt.Println("\n=== Prometheus Metrics ===")
	fmt.Print(metrics.GetPrometheusMetrics())

	fmt.Println("\n=== Monitoring Queue Statistics ===")

	// Show consumer stats
	consumerStats := q.GetConsumerStats()
	for _, cs := range consumerStats {
		fmt.Printf("\nConsumer %s:\n", cs.ID[:8])
		fmt.Printf("  Items Read: %d\n", cs.TotalItemsRead)
		fmt.Printf("  Unread Items: %d\n", cs.UnreadItems)
		fmt.Printf("  Last Read: %v ago\n", time.Since(cs.LastReadTime).Round(time.Millisecond))
	}
}
