package tests

import (
	"testing"
	"time"

	"mpmc-queue/queue"
)

func TestExpiration(t *testing.T) {
	// Create queue with very short TTL for testing
	q := queue.NewQueueWithTTL("test-queue", 100*time.Millisecond)
	defer q.Close()

	// Enqueue some data
	for i := 0; i < 5; i++ {
		err := q.TryEnqueue(i)
		if err != nil {
			t.Fatalf("Failed to enqueue item %d: %v", i, err)
		}
	}

	// Verify initial state
	stats := q.GetQueueStats()
	if stats.TotalItems != 5 {
		t.Errorf("Expected 5 items initially, got %d", stats.TotalItems)
	}

	// Wait for items to expire
	time.Sleep(150 * time.Millisecond)

	// Force expiration check
	expiredCount := q.ForceExpiration()
	if expiredCount != 5 {
		t.Errorf("Expected 5 expired items, got %d", expiredCount)
	}

	// Verify queue is empty after expiration
	stats = q.GetQueueStats()
	if stats.TotalItems != 0 {
		t.Errorf("Expected 0 items after expiration, got %d", stats.TotalItems)
	}
}

func TestConsumerNotificationOnExpiration(t *testing.T) {
	// Create queue with short TTL
	q := queue.NewQueueWithTTL("test-queue", 100*time.Millisecond)
	defer q.Close()

	// Enqueue some data
	numItems := 10
	for i := 0; i < numItems; i++ {
		q.TryEnqueue(i)
	}

	// Create consumer but don't read anything
	consumer := q.AddConsumer()

	// Wait for items to expire
	time.Sleep(150 * time.Millisecond)

	// Force expiration
	q.ForceExpiration()

	// Check for notification
	select {
	case expiredCount := <-consumer.GetNotificationChannel():
		if expiredCount != numItems {
			t.Errorf("Expected notification of %d expired items, got %d", numItems, expiredCount)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected expiration notification but didn't receive one")
	}
}

func TestPartialExpiration(t *testing.T) {
	// Create queue with medium TTL
	q := queue.NewQueueWithTTL("test-queue", 200*time.Millisecond)
	defer q.Close()

	// Enqueue first batch
	for i := 0; i < 3; i++ {
		q.TryEnqueue(i)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Enqueue second batch (these shouldn't expire yet)
	for i := 3; i < 6; i++ {
		q.TryEnqueue(i)
	}

	// Wait for first batch to expire
	time.Sleep(150 * time.Millisecond)

	// Force expiration
	expiredCount := q.ForceExpiration()
	if expiredCount != 3 {
		t.Errorf("Expected 3 expired items, got %d", expiredCount)
	}

	// Verify remaining items
	stats := q.GetQueueStats()
	if stats.TotalItems != 3 {
		t.Errorf("Expected 3 remaining items, got %d", stats.TotalItems)
	}

	// Consumer should be able to read remaining items
	consumer := q.AddConsumer()
	for i := 3; i < 6; i++ {
		data := consumer.TryRead()
		if data == nil {
			t.Fatalf("Expected to read item %d, got nil", i)
		}
		if data.Payload.(int) != i {
			t.Errorf("Expected item %d, got %v", i, data.Payload)
		}
	}
}

func TestConsumerPartialRead(t *testing.T) {
	// Create queue with short TTL
	q := queue.NewQueueWithTTL("test-queue", 100*time.Millisecond)
	defer q.Close()

	// Enqueue items
	numItems := 10
	for i := 0; i < numItems; i++ {
		q.TryEnqueue(i)
	}

	// Create consumer and read some items
	consumer := q.AddConsumer()
	itemsRead := 4
	for i := 0; i < itemsRead; i++ {
		data := consumer.TryRead()
		if data == nil {
			t.Fatalf("Failed to read item %d", i)
		}
	}

	// Wait for items to expire
	time.Sleep(150 * time.Millisecond)

	// Force expiration
	q.ForceExpiration()

	// Check notification - should only notify about unread items
	expectedExpiredUnread := numItems - itemsRead
	select {
	case expiredCount := <-consumer.GetNotificationChannel():
		if expiredCount != expectedExpiredUnread {
			t.Errorf("Expected notification of %d expired unread items, got %d",
				expectedExpiredUnread, expiredCount)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected expiration notification but didn't receive one")
	}
}

func TestMultipleConsumersExpiration(t *testing.T) {
	// Create queue with short TTL
	q := queue.NewQueueWithTTL("test-queue", 100*time.Millisecond)
	defer q.Close()

	// Enqueue items
	numItems := 10
	for i := 0; i < numItems; i++ {
		q.TryEnqueue(i)
	}

	// Create multiple consumers at different read positions
	consumer1 := q.AddConsumer()
	consumer2 := q.AddConsumer()
	consumer3 := q.AddConsumer()

	// Consumer 1 reads nothing (should get notified about all items)
	// Consumer 2 reads 3 items (should get notified about 7 items)
	// Consumer 3 reads 6 items (should get notified about 4 items)

	for i := 0; i < 3; i++ {
		consumer2.Read()
	}

	for i := 0; i < 6; i++ {
		consumer3.Read()
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)
	q.ForceExpiration()

	// Check notifications
	expectedNotifications := map[string]int{
		"consumer1": 10, // Read 0, so 10 expired unread
		"consumer2": 7,  // Read 3, so 7 expired unread
		"consumer3": 4,  // Read 6, so 4 expired unread
	}

	consumers := map[string]*queue.Consumer{
		"consumer1": consumer1,
		"consumer2": consumer2,
		"consumer3": consumer3,
	}

	for name, expectedCount := range expectedNotifications {
		consumer := consumers[name]
		select {
		case expiredCount := <-consumer.GetNotificationChannel():
			if expiredCount != expectedCount {
				t.Errorf("%s: expected %d expired items, got %d",
					name, expectedCount, expiredCount)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("%s: expected expiration notification but didn't receive one", name)
		}
	}
}

func TestExpirationDisabled(t *testing.T) {
	// Create queue with short TTL but disable expiration
	q := queue.NewQueueWithTTL("test-queue", 100*time.Millisecond)
	q.DisableExpiration()
	defer q.Close()

	// Enqueue data
	for i := 0; i < 5; i++ {
		q.TryEnqueue(i)
	}

	// Wait past expiration time
	time.Sleep(200 * time.Millisecond)

	// Force expiration (should do nothing since disabled)
	expiredCount := q.ForceExpiration()
	if expiredCount != 0 {
		t.Errorf("Expected 0 expired items (expiration disabled), got %d", expiredCount)
	}

	// Items should still be there
	stats := q.GetQueueStats()
	if stats.TotalItems != 5 {
		t.Errorf("Expected 5 items (expiration disabled), got %d", stats.TotalItems)
	}

	// Re-enable expiration
	q.EnableExpiration()

	// Now force expiration
	expiredCount = q.ForceExpiration()
	if expiredCount != 5 {
		t.Errorf("Expected 5 expired items after re-enabling, got %d", expiredCount)
	}
}

func TestLongRunningExpiration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test")
	}

	// Save original interval and restore after test
	originalInterval := queue.ExpirationCheckInterval
	queue.ExpirationCheckInterval = 100 * time.Millisecond
	defer func() {
		queue.ExpirationCheckInterval = originalInterval
	}()

	// Create queue with 1-second TTL
	q := queue.NewQueueWithTTL("test-queue", 1*time.Second)
	defer q.Close()

	// Continuously add items while some expire
	consumer := q.AddConsumer()
	addedCount := 0
	readCount := 0

	// Add items for 3 seconds
	start := time.Now()
	for time.Since(start) < 3*time.Second {
		// Add an item
		q.TryEnqueue(addedCount)
		addedCount++

		// Occasionally read items (slower than we add them)
		if addedCount%3 == 0 {
			data := consumer.TryRead()
			if data != nil {
				readCount++
			}
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Wait a bit more for final cleanup
	time.Sleep(2 * time.Second)

	// Final stats
	stats := q.GetQueueStats()

	t.Logf("Added %d items, read %d items, %d items remaining",
		addedCount, readCount, stats.TotalItems)

	// Should have fewer items than we added due to expiration
	if stats.TotalItems >= int64(addedCount) {
		t.Error("Expected some items to be expired")
	}

	// Should have some notifications about expired items
	notificationCount := 0
	for {
		select {
		case <-consumer.GetNotificationChannel():
			notificationCount++
		default:
			goto checkNotifications
		}
	}

checkNotifications:
	if notificationCount == 0 {
		t.Error("Expected some expiration notifications")
	}

	t.Logf("Received %d expiration notifications", notificationCount)
}

func TestCustomTTL(t *testing.T) {
	// Test various TTL values
	testCases := []struct {
		name string
		ttl  time.Duration
	}{
		{"very short", 50 * time.Millisecond},
		{"short", 200 * time.Millisecond},
		{"medium", 500 * time.Millisecond},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := queue.NewQueueWithTTL("test-queue", tc.ttl)
			defer q.Close()

			// Add item
			q.TryEnqueue("test")

			// Wait for expiration
			time.Sleep(tc.ttl + 50*time.Millisecond)

			// Force expiration
			expiredCount := q.ForceExpiration()
			if expiredCount != 1 {
				t.Errorf("Expected 1 expired item for TTL %v, got %d", tc.ttl, expiredCount)
			}
		})
	}
}

func TestRuntimeTTLChange(t *testing.T) {
	// Create queue with long TTL
	q := queue.NewQueueWithTTL("test-queue", 5*time.Second)
	defer q.Close()

	// Add items
	for i := 0; i < 5; i++ {
		q.TryEnqueue(i)
	}

	// Change TTL to very short
	q.SetTTL(50 * time.Millisecond)

	// Verify TTL was changed
	if q.GetTTL() != 50*time.Millisecond {
		t.Errorf("Expected TTL to be 50ms, got %v", q.GetTTL())
	}

	// Wait for expiration with new TTL
	time.Sleep(100 * time.Millisecond)

	// Force expiration
	expiredCount := q.ForceExpiration()
	if expiredCount != 5 {
		t.Errorf("Expected 5 expired items with new TTL, got %d", expiredCount)
	}
}
