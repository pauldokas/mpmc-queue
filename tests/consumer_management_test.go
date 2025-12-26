package tests

import (
	"sync"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestRemoveConsumer tests basic consumer removal
func TestRemoveConsumer(t *testing.T) {
	q := queue.NewQueue("remove-consumer-test")
	defer q.Close()

	// Add a consumer
	consumer := q.AddConsumer()
	consumerID := consumer.GetID()

	// Verify consumer exists
	retrieved := q.GetConsumer(consumerID)
	if retrieved == nil {
		t.Fatal("Consumer should exist after adding")
	}

	// Remove consumer
	removed := q.RemoveConsumer(consumerID)
	if !removed {
		t.Error("RemoveConsumer should return true for existing consumer")
	}

	// Verify consumer no longer exists
	retrieved = q.GetConsumer(consumerID)
	if retrieved != nil {
		t.Error("Consumer should not exist after removal")
	}

	// Try to remove again - should return false
	removed = q.RemoveConsumer(consumerID)
	if removed {
		t.Error("RemoveConsumer should return false for non-existent consumer")
	}
}

// TestRemoveConsumerNonExistent tests removing a consumer that doesn't exist
func TestRemoveConsumerNonExistent(t *testing.T) {
	q := queue.NewQueue("remove-nonexistent-test")
	defer q.Close()

	// Try to remove non-existent consumer
	removed := q.RemoveConsumer("non-existent-id")
	if removed {
		t.Error("RemoveConsumer should return false for non-existent consumer")
	}
}

// TestRemoveConsumerDuringRead tests removing a consumer while it's reading
func TestRemoveConsumerDuringRead(t *testing.T) {
	q := queue.NewQueue("remove-during-read-test")
	defer q.Close()

	// Add data
	for i := 0; i < 100; i++ {
		q.TryEnqueue(i)
	}

	consumer := q.AddConsumer()
	consumerID := consumer.GetID()

	var wg sync.WaitGroup
	wg.Add(1)

	// Start reading in goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			data := consumer.TryRead()
			if data == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Wait a bit for reads to start
	time.Sleep(10 * time.Millisecond)

	// Remove consumer while it's reading
	removed := q.RemoveConsumer(consumerID)
	if !removed {
		t.Error("Should be able to remove active consumer")
	}

	// Consumer should still be able to complete its current operations
	wg.Wait()

	// Verify consumer is gone
	if q.GetConsumer(consumerID) != nil {
		t.Error("Consumer should be removed")
	}
}

// TestGetConsumer tests retrieving consumers by ID
func TestGetConsumer(t *testing.T) {
	q := queue.NewQueue("get-consumer-test")
	defer q.Close()

	// Create multiple consumers
	consumer1 := q.AddConsumer()
	consumer2 := q.AddConsumer()
	consumer3 := q.AddConsumer()

	// Retrieve each consumer
	retrieved1 := q.GetConsumer(consumer1.GetID())
	retrieved2 := q.GetConsumer(consumer2.GetID())
	retrieved3 := q.GetConsumer(consumer3.GetID())

	if retrieved1 == nil || retrieved1.GetID() != consumer1.GetID() {
		t.Error("Failed to retrieve consumer1")
	}
	if retrieved2 == nil || retrieved2.GetID() != consumer2.GetID() {
		t.Error("Failed to retrieve consumer2")
	}
	if retrieved3 == nil || retrieved3.GetID() != consumer3.GetID() {
		t.Error("Failed to retrieve consumer3")
	}

	// Try to retrieve non-existent consumer
	retrieved := q.GetConsumer("non-existent")
	if retrieved != nil {
		t.Error("GetConsumer should return nil for non-existent consumer")
	}
}

// TestGetAllConsumers tests retrieving all consumers
func TestGetAllConsumers(t *testing.T) {
	q := queue.NewQueue("get-all-consumers-test")
	defer q.Close()

	// Initially no consumers
	consumers := q.GetAllConsumers()
	if len(consumers) != 0 {
		t.Errorf("Expected 0 consumers, got %d", len(consumers))
	}

	// Add consumers
	c1 := q.AddConsumer()
	c2 := q.AddConsumer()
	c3 := q.AddConsumer()

	consumers = q.GetAllConsumers()
	if len(consumers) != 3 {
		t.Errorf("Expected 3 consumers, got %d", len(consumers))
	}

	// Verify all consumers are present
	foundIDs := make(map[string]bool)
	for _, c := range consumers {
		foundIDs[c.GetID()] = true
	}

	if !foundIDs[c1.GetID()] || !foundIDs[c2.GetID()] || !foundIDs[c3.GetID()] {
		t.Error("Not all consumers were returned by GetAllConsumers")
	}

	// Remove one consumer
	q.RemoveConsumer(c2.GetID())

	consumers = q.GetAllConsumers()
	if len(consumers) != 2 {
		t.Errorf("Expected 2 consumers after removal, got %d", len(consumers))
	}
}

// TestGetAllConsumersConcurrent tests GetAllConsumers with concurrent modifications
func TestGetAllConsumersConcurrent(t *testing.T) {
	q := queue.NewQueue("get-all-concurrent-test")
	defer q.Close()

	var wg sync.WaitGroup

	// Add consumers concurrently
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			q.AddConsumer()
		}()
	}

	// Call GetAllConsumers concurrently
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			consumers := q.GetAllConsumers()
			// Should not panic and should return valid slice
			if consumers == nil {
				t.Error("GetAllConsumers should not return nil")
			}
		}()
	}

	wg.Wait()

	// Final count should be 10
	consumers := q.GetAllConsumers()
	if len(consumers) != 10 {
		t.Errorf("Expected 10 consumers, got %d", len(consumers))
	}
}

// TestConsumerCleanupOnRemoval tests that consumer is properly cleaned up
func TestConsumerCleanupOnRemoval(t *testing.T) {
	q := queue.NewQueue("cleanup-test")
	defer q.Close()

	// Add data
	for i := 0; i < 10; i++ {
		q.TryEnqueue(i)
	}

	consumer := q.AddConsumer()
	consumerID := consumer.GetID()

	// Read some data
	for i := 0; i < 5; i++ {
		consumer.TryRead()
	}

	// Get stats before removal
	statsBefore := q.GetConsumerStats()
	if len(statsBefore) != 1 {
		t.Errorf("Expected 1 consumer in stats, got %d", len(statsBefore))
	}

	// Remove consumer
	q.RemoveConsumer(consumerID)

	// Stats should no longer include this consumer
	statsAfter := q.GetConsumerStats()
	if len(statsAfter) != 0 {
		t.Errorf("Expected 0 consumers in stats after removal, got %d", len(statsAfter))
	}

	// Queue stats should reflect consumer count
	queueStats := q.GetQueueStats()
	if queueStats.ConsumerCount != 0 {
		t.Errorf("Expected consumer count 0, got %d", queueStats.ConsumerCount)
	}
}

// TestAddConsumerAfterQueueHasData tests adding a consumer when queue already has data
func TestAddConsumerAfterQueueHasData(t *testing.T) {
	q := queue.NewQueue("add-after-data-test")
	defer q.Close()

	// Add data first
	for i := 0; i < 50; i++ {
		q.TryEnqueue(i)
	}

	// Now add consumer - should start from beginning
	consumer := q.AddConsumer()

	// Read all data
	count := 0
	for {
		data := consumer.TryRead()
		if data == nil {
			break
		}
		count++
	}

	if count != 50 {
		t.Errorf("Expected to read 50 items, got %d", count)
	}
}

// TestConcurrentConsumerAddRemove tests adding and removing consumers concurrently
func TestConcurrentConsumerAddRemove(t *testing.T) {
	q := queue.NewQueue("concurrent-add-remove-test")
	defer q.Close()

	// Add some data
	for i := 0; i < 100; i++ {
		q.TryEnqueue(i)
	}

	var wg sync.WaitGroup
	consumerIDs := make(chan string, 20)

	// Add consumers
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			c := q.AddConsumer()
			consumerIDs <- c.GetID()
			// Read some data
			for j := 0; j < 10; j++ {
				c.TryRead()
			}
		}()
	}

	// Wait for consumers to be added
	wg.Wait()
	close(consumerIDs)

	// Collect IDs
	var ids []string
	for id := range consumerIDs {
		ids = append(ids, id)
	}

	// Remove consumers concurrently
	wg.Add(len(ids))
	for _, id := range ids {
		go func(consumerID string) {
			defer wg.Done()
			q.RemoveConsumer(consumerID)
		}(id)
	}

	wg.Wait()

	// Verify all consumers removed
	consumers := q.GetAllConsumers()
	if len(consumers) != 0 {
		t.Errorf("Expected 0 consumers after removal, got %d", len(consumers))
	}
}
