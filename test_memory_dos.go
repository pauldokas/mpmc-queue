package main

import (
	"fmt"
	"mpmc-queue/queue"
)

type MaliciousPayload struct {
	data []byte
}

func (m MaliciousPayload) Size() int {
	return -100000000 // Very negative size
}

func main() {
	q := queue.NewQueueWithConfig("test", queue.QueueConfig{
		MaxMemory: 1024, // 1KB limit
	})

	// Add normal item to verify limit works
	err := q.TryEnqueue(make([]byte, 2048))
	fmt.Printf("Normal large item error: %v\n", err)

	// Add malicious item
	err = q.TryEnqueue(MaliciousPayload{})
	fmt.Printf("Malicious item error: %v\n", err)

	// Now we can bypass the limit
	err = q.TryEnqueue(make([]byte, 2048))
	fmt.Printf("Bypass large item error: %v\n", err)

	fmt.Printf("Current Memory Usage: %d\n", q.GetMemoryUsage())
}
