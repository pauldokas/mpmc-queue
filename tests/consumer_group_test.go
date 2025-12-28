package tests

import (
	"sync"
	"testing"
	"time"

	"mpmc-queue/queue"
)

func TestConsumerGroup_Distribution(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	group := q.AddConsumerGroup("test-group")
	c1 := group.AddConsumer()
	c2 := group.AddConsumer()

	// Enqueue items
	items := []string{"A", "B", "C", "D", "E", "F"}
	for _, item := range items {
		if err := q.TryEnqueue(item); err != nil {
			t.Fatalf("Failed to enqueue %s: %v", item, err)
		}
	}

	results := make(map[string]bool)
	var mutex sync.Mutex

	// Consumers read concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	readFunc := func(c *queue.Consumer) {
		defer wg.Done()
		for {
			data := c.TryRead()
			if data == nil {
				break
			}
			mutex.Lock()
			results[data.Payload.(string)] = true
			mutex.Unlock()
			time.Sleep(time.Millisecond) // Simulate work
		}
	}

	go readFunc(c1)
	go readFunc(c2)

	wg.Wait()

	// Verify all items were processed exactly once
	if len(results) != len(items) {
		t.Errorf("Expected %d items processed, got %d", len(items), len(results))
	}

	for _, item := range items {
		if !results[item] {
			t.Errorf("Item %s was not processed", item)
		}
	}
}

func TestConsumerGroup_MixedWithIndependent(t *testing.T) {
	q := queue.NewQueue("test-queue")
	defer q.Close()

	// Independent consumer
	indC := q.AddConsumer()

	// Group consumers
	group := q.AddConsumerGroup("test-group")
	gC1 := group.AddConsumer()
	gC2 := group.AddConsumer()

	// Enqueue
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, item := range items {
		if err := q.TryEnqueue(item); err != nil {
			t.Fatalf("Failed to enqueue %d: %v", item, err)
		}
	}

	// Independent consumer should get ALL items
	countInd := 0
	for indC.TryRead() != nil {
		countInd++
	}
	if countInd != len(items) {
		t.Errorf("Independent consumer got %d items, expected %d", countInd, len(items))
	}

	// Group consumers should get ALL items combined
	countGroup := 0

	// Just read sequentially here for simplicity of verification
	for i := 0; i < 100; i++ { // Safety break
		d1 := gC1.TryRead()
		if d1 != nil {
			countGroup++
		}
		d2 := gC2.TryRead()
		if d2 != nil {
			countGroup++
		}
		if d1 == nil && d2 == nil {
			break
		}
	}

	if countGroup != len(items) {
		t.Errorf("Group consumers got %d items, expected %d", countGroup, len(items))
	}
}

func TestConsumerGroup_Expiration(t *testing.T) {
	q := queue.NewQueueWithTTL("test-queue", 100*time.Millisecond)
	defer q.Close()

	group := q.AddConsumerGroup("test-group")
	c1 := group.AddConsumer()

	// Enqueue items
	_ = q.TryEnqueue("item1")
	_ = q.TryEnqueue("item2")

	// Read one item
	data := c1.TryRead()
	if data == nil || data.Payload != "item1" {
		t.Fatalf("Expected item1, got %v", data)
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)
	q.ForceExpiration()

	// Should not be able to read item2 (expired)
	data = c1.TryRead()
	if data != nil {
		t.Errorf("Expected nil (expired), got %v", data)
	}

	// Check unread count
	if count := c1.GetUnreadCount(); count != 0 {
		t.Errorf("Expected 0 unread items, got %d", count)
	}
}
