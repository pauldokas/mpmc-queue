package tests

import (
	"fmt"
	"mpmc-queue/queue"
	"testing"
)

func TestCrossQueueLeak(t *testing.T) {
	q1 := queue.NewQueue("q1")
	_ = q1.Enqueue("public-data")

	c1 := q1.AddConsumer()
	data1 := c1.TryRead()
	if data1 == nil || data1.Payload != "public-data" {
		t.Fatalf("Expected public-data, got %v", data1)
	}

	q1.Close()

	q2 := queue.NewQueue("q2")
	defer q2.Close()
	_ = q2.Enqueue("SECRET-DATA-1")
	_ = q2.Enqueue("SECRET-DATA-2")

	fmt.Printf("Before TryRead: c1 HasMoreData() = %v\n", c1.HasMoreData())
	leakedData := c1.TryRead()
	if leakedData != nil {
		t.Fatalf("CRITICAL BUG: C1 leaked data from Q2: %v", leakedData.Payload)
	}
}
