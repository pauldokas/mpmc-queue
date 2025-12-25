package tests

import (
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestMemoryLeakOnExpiration verifies that memory is properly released when items expire
func TestMemoryLeakOnExpiration(t *testing.T) {
	q := queue.NewQueueWithTTL("memory-leak-test", 50*time.Millisecond)
	defer q.Close()

	// Record initial memory
	initialMemory := q.GetMemoryUsage()

	// Enqueue items that will use significant memory
	largePayload := make([]byte, 10000) // 10KB each
	for i := 0; i < 10; i++ {
		err := q.Enqueue(largePayload)
		if err != nil {
			t.Fatalf("Failed to enqueue item %d: %v", i, err)
		}
	}

	// Check memory increased
	memoryAfterEnqueue := q.GetMemoryUsage()
	if memoryAfterEnqueue <= initialMemory {
		t.Errorf("Memory should have increased after enqueue. Initial: %d, After: %d",
			initialMemory, memoryAfterEnqueue)
	}

	t.Logf("Memory before expiration: %d bytes", memoryAfterEnqueue)

	// Wait for items to expire
	time.Sleep(100 * time.Millisecond)

	// Force expiration
	expiredCount := q.ForceExpiration()
	if expiredCount != 10 {
		t.Errorf("Expected 10 items to expire, got %d", expiredCount)
	}

	// Check memory was released
	memoryAfterExpiration := q.GetMemoryUsage()
	t.Logf("Memory after expiration: %d bytes", memoryAfterExpiration)

	// Memory should be back to near initial levels (within chunk overhead)
	// Allow some tolerance for chunk metadata
	if memoryAfterExpiration > initialMemory+1000 {
		t.Errorf("Memory leak detected! Initial: %d, After expiration: %d (expected ~%d)",
			initialMemory, memoryAfterExpiration, initialMemory)
	}

	// Verify we can enqueue more items (no false memory limit)
	for i := 0; i < 10; i++ {
		err := q.Enqueue(largePayload)
		if err != nil {
			t.Fatalf("Failed to enqueue after expiration (memory leak?): %v", err)
		}
	}
}

// TestConsumerPositionAfterPartialExpiration verifies consumer position is correct after partial chunk expiration
func TestConsumerPositionAfterPartialExpiration(t *testing.T) {
	q := queue.NewQueueWithTTL("position-test", 100*time.Millisecond)
	defer q.Close()

	// Enqueue 20 items
	for i := 0; i < 20; i++ {
		q.Enqueue(i)
	}

	// Create consumer and read first 5 items
	consumer := q.AddConsumer()
	for i := 0; i < 5; i++ {
		data := consumer.Read()
		if data == nil {
			t.Fatalf("Failed to read item %d", i)
		}
		if data.Payload != i {
			t.Errorf("Expected item %d, got %v", i, data.Payload)
		}
	}

	// Wait for first 5 items to expire
	time.Sleep(150 * time.Millisecond)

	// Force expiration (should remove first 5 items)
	expiredCount := q.ForceExpiration()
	if expiredCount != 5 {
		t.Errorf("Expected 5 items to expire, got %d", expiredCount)
	}

	// Consumer should now be at the beginning of the adjusted chunk
	// Next item should be 5 (since 0-4 expired and consumer already read them)
	data := consumer.Read()
	if data == nil {
		t.Fatal("Expected to read item 5 after expiration")
	}
	if data.Payload != 5 {
		t.Errorf("After expiration, expected item 5, got %v. Consumer position not adjusted correctly!", data.Payload)
	}

	// Continue reading to verify no items are skipped
	for i := 6; i < 20; i++ {
		data := consumer.Read()
		if data == nil {
			t.Fatalf("Failed to read item %d", i)
		}
		if data.Payload != i {
			t.Errorf("Expected item %d, got %v", i, data.Payload)
		}
	}
}

// TestConsumerReadingDuringExpiration tests consumer reading while items expire
func TestConsumerReadingDuringExpiration(t *testing.T) {
	q := queue.NewQueueWithTTL("read-during-expiration", 80*time.Millisecond)
	defer q.Close()

	// Enqueue items with delays to create staggered expiration
	for i := 0; i < 10; i++ {
		q.Enqueue(i)
		if i == 4 {
			// Create a gap - first 5 items will expire before the rest
			time.Sleep(100 * time.Millisecond)
		}
	}

	consumer := q.AddConsumer()

	// Read first 3 items
	for i := 0; i < 3; i++ {
		data := consumer.Read()
		if data == nil || data.Payload != i {
			t.Errorf("Failed to read item %d correctly", i)
		}
	}

	// Wait for first batch to expire
	time.Sleep(50 * time.Millisecond)
	q.ForceExpiration()

	// Consumer should still read items 3, 4 correctly (before the gap)
	data := consumer.Read()
	if data == nil {
		t.Fatal("Expected to read item 3")
	}
	if data.Payload != 3 {
		t.Errorf("Expected item 3, got %v", data.Payload)
	}

	data = consumer.Read()
	if data == nil {
		t.Fatal("Expected to read item 4")
	}
	if data.Payload != 4 {
		t.Errorf("Expected item 4, got %v", data.Payload)
	}

	// Continue with newer items that didn't expire
	for i := 5; i < 10; i++ {
		data := consumer.Read()
		if data == nil {
			t.Fatalf("Failed to read item %d", i)
		}
		if data.Payload != i {
			t.Errorf("Expected item %d, got %v", i, data.Payload)
		}
	}
}

// TestMultipleConsumersAfterExpiration verifies all consumers are correctly adjusted
func TestMultipleConsumersAfterExpiration(t *testing.T) {
	q := queue.NewQueueWithTTL("multi-consumer-expiration", 50*time.Millisecond)
	defer q.Close()

	// Enqueue 15 items
	for i := 0; i < 15; i++ {
		q.Enqueue(i)
	}

	// Create 3 consumers at different positions
	consumer1 := q.AddConsumer()
	consumer2 := q.AddConsumer()
	consumer3 := q.AddConsumer()

	// Consumer 1 reads 2 items
	consumer1.Read() // 0
	consumer1.Read() // 1

	// Consumer 2 reads 5 items
	for i := 0; i < 5; i++ {
		consumer2.Read()
	}

	// Consumer 3 reads 10 items
	for i := 0; i < 10; i++ {
		consumer3.Read()
	}

	// Wait and expire first 5 items
	time.Sleep(100 * time.Millisecond)
	expiredCount := q.ForceExpiration()
	if expiredCount != 5 {
		t.Errorf("Expected 5 items to expire, got %d", expiredCount)
	}

	// Consumer 1 (was at index 2) should now read item 2 (not expired)
	data := consumer1.Read()
	if data == nil || data.Payload != 2 {
		t.Errorf("Consumer 1: expected item 2, got %v", data)
	}

	// Consumer 2 (was at index 5) should now read item 5
	data = consumer2.Read()
	if data == nil || data.Payload != 5 {
		t.Errorf("Consumer 2: expected item 5, got %v", data)
	}

	// Consumer 3 (was at index 10) should now read item 10
	data = consumer3.Read()
	if data == nil || data.Payload != 10 {
		t.Errorf("Consumer 3: expected item 10, got %v", data)
	}
}
