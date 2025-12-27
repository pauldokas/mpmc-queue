package queue_test

import (
	"context"
	"fmt"
	"time"

	"mpmc-queue/queue"
)

func ExampleNewQueue() {
	// Create a new queue with default settings (1MB memory limit, 10m TTL)
	q := queue.NewQueue("example-queue")
	defer q.Close()

	fmt.Println(q.GetName())
	// Output: example-queue
}

func ExampleQueue_Enqueue() {
	q := queue.NewQueue("tasks")
	defer q.Close()

	// Enqueue a simple string payload
	if err := q.Enqueue("process-image-123"); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Item enqueued successfully")
	// Output: Item enqueued successfully
}

func ExampleConsumer_Read() {
	q := queue.NewQueue("messages")
	defer q.Close()

	// Add a consumer BEFORE enqueuing to ensure it sees the data
	// (Though Persistent Queue semantics mean it would see it anyway,
	// this is good practice for some queue types)
	c := q.AddConsumer()
	defer c.Close()

	q.Enqueue("hello world")

	// Read the item (blocks until data is available)
	item := c.Read()
	fmt.Printf("Received: %v\n", item.Payload)
	// Output: Received: hello world
}

func ExampleQueue_TryEnqueue() {
	q := queue.NewQueue("metrics")
	defer q.Close()

	// TryEnqueue returns immediately.
	// It fails if the memory limit is reached.
	err := q.TryEnqueue(42)
	if err != nil {
		fmt.Println("Queue full")
	} else {
		fmt.Println("Metric added")
	}
	// Output: Metric added
}

func ExampleConsumer_TryRead() {
	q := queue.NewQueue("logs")
	defer q.Close()

	q.Enqueue("log entry 1")

	c := q.AddConsumer()

	// TryRead returns nil immediately if no data is available
	if item := c.TryRead(); item != nil {
		fmt.Printf("Read: %v\n", item.Payload)
	} else {
		fmt.Println("No data")
	}
	// Output: Read: log entry 1
}

func ExampleQueue_EnqueueWithContext() {
	q := queue.NewQueue("jobs")
	defer q.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Enqueue with a timeout
	err := q.EnqueueWithContext(ctx, "job-1")
	if err != nil {
		fmt.Printf("Enqueue failed: %v\n", err)
	} else {
		fmt.Println("Job enqueued")
	}
	// Output: Job enqueued
}
