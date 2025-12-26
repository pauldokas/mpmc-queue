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
		err := q.TryEnqueue(largePayload)
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
		err := q.TryEnqueue(largePayload)
		if err != nil {
			t.Fatalf("Failed to enqueue after expiration (memory leak?): %v", err)
		}
	}
}

// TestConsumerPositionAfterPartialExpiration verifies consumer position is correct after partial chunk expiration
func TestConsumerPositionAfterPartialExpiration(t *testing.T) {
	q := queue.NewQueueWithTTL("position-test", 100*time.Millisecond)
	defer q.Close()

	// Enqueue first 5 items
	for i := 0; i < 5; i++ {
		q.TryEnqueue(i)
	}

	// Wait a bit to ensure these items have different creation times
	time.Sleep(20 * time.Millisecond)

	// Enqueue remaining 15 items (these will be newer)
	for i := 5; i < 20; i++ {
		q.TryEnqueue(i)
	}

	// Create consumer and read first 5 items
	consumer := q.AddConsumer()
	for i := 0; i < 5; i++ {
		data := consumer.TryRead()
		if data == nil {
			t.Fatalf("Failed to read item %d", i)
		}
		if data.Payload != i {
			t.Errorf("Expected item %d, got %v", i, data.Payload)
		}
	}

	// Wait for first 5 items to expire (100ms TTL + 20ms already elapsed)
	time.Sleep(90 * time.Millisecond)

	// Force expiration (should remove first 5 items only)
	expiredCount := q.ForceExpiration()
	if expiredCount != 5 {
		t.Errorf("Expected 5 items to expire, got %d", expiredCount)
	}

	// Consumer should now be at the beginning of the adjusted chunk
	// Next item should be 5 (since 0-4 expired and consumer already read them)
	data := consumer.TryRead()
	if data == nil {
		t.Fatal("Expected to read item 5 after expiration")
	}
	if data.Payload != 5 {
		t.Errorf("After expiration, expected item 5, got %v. Consumer position not adjusted correctly!", data.Payload)
	}

	// Continue reading to verify no items are skipped
	for i := 6; i < 20; i++ {
		data := consumer.TryRead()
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

	// Enqueue first 5 items
	for i := 0; i < 5; i++ {
		q.TryEnqueue(i)
	}

	// Create a time gap - first 5 items will expire before the rest
	time.Sleep(100 * time.Millisecond)

	// Enqueue remaining 5 items (these are fresh)
	for i := 5; i < 10; i++ {
		q.TryEnqueue(i)
	}

	consumer := q.AddConsumer()

	// Read first 3 items
	for i := 0; i < 3; i++ {
		data := consumer.TryRead()
		if data == nil || data.Payload != i {
			t.Errorf("Failed to read item %d correctly", i)
		}
	}

	// Force expiration - first 5 items should expire
	expiredCount := q.ForceExpiration()
	if expiredCount != 5 {
		t.Errorf("Expected 5 items to expire, got %d", expiredCount)
	}

	// Consumer was at position 3, but items 0-4 expired
	// So the consumer should now continue from item 5 (the first non-expired item)
	// But the consumer already read 0, 1, 2, so it should be positioned to read what comes after its last read
	// Since items 3 and 4 were removed, the next available item is 5
	data := consumer.TryRead()
	if data == nil {
		t.Fatal("Expected to read item 5 (first non-expired)")
	}
	if data.Payload != 5 {
		t.Errorf("Expected item 5, got %v", data.Payload)
	}

	// Continue with remaining items
	for i := 6; i < 10; i++ {
		data := consumer.TryRead()
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
	q := queue.NewQueueWithTTL("multi-consumer-expiration", 100*time.Millisecond)
	defer q.Close()

	// Enqueue first 5 items that will be old
	for i := 0; i < 5; i++ {
		q.TryEnqueue(i)
	}

	// Wait to create age difference
	time.Sleep(50 * time.Millisecond)

	// Enqueue remaining 10 items (newer)
	for i := 5; i < 15; i++ {
		q.TryEnqueue(i)
	}

	// Create 3 consumers at different positions
	consumer1 := q.AddConsumer()
	consumer2 := q.AddConsumer()
	consumer3 := q.AddConsumer()

	// Consumer 1 reads 2 items (0, 1)
	consumer1.Read() // 0
	consumer1.Read() // 1

	// Consumer 2 reads 5 items (0-4)
	for i := 0; i < 5; i++ {
		consumer2.Read()
	}

	// Consumer 3 reads 10 items (0-9)
	for i := 0; i < 10; i++ {
		consumer3.Read()
	}

	// Wait for first 5 items to expire (50ms already passed + 60ms = 110ms > 100ms TTL)
	time.Sleep(60 * time.Millisecond)
	expiredCount := q.ForceExpiration()
	if expiredCount != 5 {
		t.Errorf("Expected 5 items to expire, got %d", expiredCount)
	}

	// Consumer 1 was at position 2, but items 0-4 expired
	// Items 2, 3, 4 were removed before consumer could read them
	// Next available item is 5
	data := consumer1.Read()
	if data == nil || data.Payload != 5 {
		t.Errorf("Consumer 1: expected item 5 (first non-expired), got %v", data)
	}

	// Consumer 2 was at position 5, which is the first non-expired item
	data = consumer2.Read()
	if data == nil || data.Payload != 5 {
		t.Errorf("Consumer 2: expected item 5, got %v", data)
	}

	// Consumer 3 was at position 10, unaffected by expiration
	data = consumer3.Read()
	if data == nil || data.Payload != 10 {
		t.Errorf("Consumer 3: expected item 10, got %v", data)
	}
}
