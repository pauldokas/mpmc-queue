package tests

import (
	"testing"
	"time"

	"mpmc-queue/queue"
)

func TestConsumer_PointerRetention(t *testing.T) {
	cfg := queue.QueueConfig{
		MaxMemory:               1024 * 1024,
		TTL:                     50 * time.Millisecond,
		ExpirationCheckInterval: 10 * time.Millisecond,
		MaxConsumerHistory:      10,
	}

	q := queue.NewQueueWithConfig("test-retention", cfg)
	defer q.Close()

	consumer := q.AddConsumer()

	if err := q.TryEnqueue("hello"); err != nil {
		t.Fatalf("TryEnqueue failed: %v", err)
	}

	data := consumer.TryRead()
	if data == nil || data.Payload.(string) != "hello" {
		t.Fatalf("Expected to read 'hello', got %v", data)
	}

	elem, _ := consumer.GetPosition()
	if elem == nil {
		t.Fatalf("Expected consumer to have a non-nil chunkElement after reading")
	}

	time.Sleep(100 * time.Millisecond)

	elemAfter, _ := consumer.GetPosition()
	if elemAfter != nil {
		t.Errorf("Expected consumer to have a nil chunkElement after queue is emptied by expiration, but got non-nil. This causes pointer retention (Issue 5).")
	}
}

func TestConsumerGroup_PointerRetention(t *testing.T) {
	cfg := queue.QueueConfig{
		MaxMemory:               1024 * 1024,
		TTL:                     50 * time.Millisecond,
		ExpirationCheckInterval: 10 * time.Millisecond,
		MaxConsumerHistory:      10,
	}

	q := queue.NewQueueWithConfig("test-group-retention", cfg)
	defer q.Close()

	cg := q.AddConsumerGroup("test-group")

	consumer := cg.AddConsumer()

	if err := q.TryEnqueue("world"); err != nil {
		t.Fatalf("TryEnqueue failed: %v", err)
	}

	data := consumer.TryRead()
	if data == nil || data.Payload.(string) != "world" {
		t.Fatalf("Expected to read 'world', got %v", data)
	}

	time.Sleep(100 * time.Millisecond)

	elemAfter, _ := consumer.GetPosition()
	if elemAfter != nil {
		t.Errorf("Expected ConsumerGroup to have a nil chunkElement after queue is emptied, got non-nil.")
	}
}
