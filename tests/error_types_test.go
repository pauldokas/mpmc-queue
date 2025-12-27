package tests

import (
	"errors"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestQueueClosedError tests the QueueClosedError type
func TestQueueClosedError(t *testing.T) {
	// Test with operation specified
	err := &queue.QueueClosedError{Operation: "enqueue"}
	expected := "queue closed: cannot perform enqueue operation"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}

	// Test without operation
	err2 := &queue.QueueClosedError{}
	expected2 := "queue closed"
	if err2.Error() != expected2 {
		t.Errorf("Expected %q, got %q", expected2, err2.Error())
	}

	// Test type assertion
	var testErr error = &queue.QueueClosedError{Operation: "test"}
	var qcErr *queue.QueueClosedError
	if !errors.As(testErr, &qcErr) {
		t.Error("Failed to assert QueueClosedError type")
	}
	if qcErr.Operation != "test" {
		t.Errorf("Expected operation 'test', got %q", qcErr.Operation)
	}
}

// TestConsumerNotFoundError tests the ConsumerNotFoundError type
func TestConsumerNotFoundError(t *testing.T) {
	err := &queue.ConsumerNotFoundError{ID: "consumer-123"}
	expected := "consumer not found: consumer-123"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}

	// Test type assertion
	var testErr error = &queue.ConsumerNotFoundError{ID: "test-id"}
	var cnfErr *queue.ConsumerNotFoundError
	if !errors.As(testErr, &cnfErr) {
		t.Error("Failed to assert ConsumerNotFoundError type")
	}
	if cnfErr.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got %q", cnfErr.ID)
	}
}

// TestInvalidPositionError tests the InvalidPositionError type
func TestInvalidPositionError(t *testing.T) {
	// Test with reason
	err := &queue.InvalidPositionError{Position: 42, Reason: "out of bounds"}
	expected := "invalid position 42: out of bounds"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}

	// Test without reason
	err2 := &queue.InvalidPositionError{Position: 100}
	expected2 := "invalid position: 100"
	if err2.Error() != expected2 {
		t.Errorf("Expected %q, got %q", expected2, err2.Error())
	}

	// Test type assertion
	var testErr error = &queue.InvalidPositionError{Position: 10, Reason: "test"}
	var ipErr *queue.InvalidPositionError
	if !errors.As(testErr, &ipErr) {
		t.Error("Failed to assert InvalidPositionError type")
	}
	if ipErr.Position != 10 {
		t.Errorf("Expected position 10, got %d", ipErr.Position)
	}
	if ipErr.Reason != "test" {
		t.Errorf("Expected reason 'test', got %q", ipErr.Reason)
	}
}

// TestMemoryLimitError tests the MemoryLimitError type
func TestMemoryLimitError(t *testing.T) {
	err := &queue.MemoryLimitError{
		Current: 900000,
		Max:     1000000,
		Needed:  150000,
	}
	expected := "memory limit exceeded: current=900000, max=1000000, needed=150000"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}

	// Test type assertion
	var testErr error = &queue.MemoryLimitError{Current: 1, Max: 2, Needed: 3}
	var mlErr *queue.MemoryLimitError
	if !errors.As(testErr, &mlErr) {
		t.Error("Failed to assert MemoryLimitError type")
	}
	if mlErr.Current != 1 || mlErr.Max != 2 || mlErr.Needed != 3 {
		t.Errorf("Expected Current=1, Max=2, Needed=3, got Current=%d, Max=%d, Needed=%d",
			mlErr.Current, mlErr.Max, mlErr.Needed)
	}
}

// TestQueueError tests the QueueError type
func TestQueueError(t *testing.T) {
	err := &queue.QueueError{Message: "test error"}
	expected := "test error"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}

	// Test type assertion
	var testErr error = &queue.QueueError{Message: "test"}
	var qErr *queue.QueueError
	if !errors.As(testErr, &qErr) {
		t.Error("Failed to assert QueueError type")
	}
	if qErr.Message != "test" {
		t.Errorf("Expected message 'test', got %q", qErr.Message)
	}
}

// TestQueueClosedError_Actual tests that QueueClosedError is returned when blocking enqueue is interrupted by queue closure
func TestQueueClosedError_Actual(t *testing.T) {
	// Create small queue that will fill up
	q := queue.NewQueueWithConfig("small-queue", queue.QueueConfig{
		TTL:       queue.DefaultTTL,
		MaxMemory: 2048, // Small memory limit
	})

	// Fill the queue to capacity
	largeData := make([]byte, 1500)
	if err := q.TryEnqueue(largeData); err != nil {
		t.Fatalf("Failed to fill queue: %v", err)
	}

	// Start goroutine that will block trying to enqueue
	errChan := make(chan error, 1)
	go func() {
		err := q.Enqueue(largeData) // Will block waiting for space
		errChan <- err
	}()

	// Give goroutine time to block
	// time.Sleep(50 * time.Millisecond)

	// Close the queue while enqueue is blocking
	q.Close()

	// Should get QueueClosedError
	select {
	case err := <-errChan:
		if err == nil {
			t.Fatal("Expected error when queue closed during blocking enqueue")
		}

		var qcErr *queue.QueueClosedError
		if !errors.As(err, &qcErr) {
			t.Errorf("Expected QueueClosedError, got %T: %v", err, err)
		} else if qcErr.Operation != "enqueue" {
			t.Errorf("Expected operation 'enqueue', got %q", qcErr.Operation)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for enqueue to return")
	}
}

// TestQueueClosedError_Batch tests that QueueClosedError is returned when blocking batch enqueue is interrupted
func TestQueueClosedError_Batch(t *testing.T) {
	// Create small queue that will fill up
	q := queue.NewQueueWithConfig("small-queue", queue.QueueConfig{
		TTL:       queue.DefaultTTL,
		MaxMemory: 2048, // Small memory limit
	})

	// Fill the queue to capacity
	largeData := make([]byte, 1500)
	if err := q.TryEnqueue(largeData); err != nil {
		t.Fatalf("Failed to fill queue: %v", err)
	}

	// Start goroutine that will block trying to enqueue batch
	errChan := make(chan error, 1)
	go func() {
		err := q.EnqueueBatch([]any{largeData, largeData}) // Will block waiting for space
		errChan <- err
	}()

	// Close the queue while enqueue is blocking
	q.Close()

	// Should get QueueClosedError
	select {
	case err := <-errChan:
		if err == nil {
			t.Fatal("Expected error when queue closed during blocking batch enqueue")
		}

		var qcErr *queue.QueueClosedError
		if !errors.As(err, &qcErr) {
			t.Errorf("Expected QueueClosedError, got %T: %v", err, err)
		} else if qcErr.Operation != "enqueue batch" {
			t.Errorf("Expected operation 'enqueue batch', got %q", qcErr.Operation)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for batch enqueue to return")
	}
}

// TestMemoryLimitError_Actual tests that MemoryLimitError is actually returned
func TestMemoryLimitError_Actual(t *testing.T) {
	// Create small queue (1KB max memory)
	q := queue.NewQueueWithConfig("small-queue", queue.QueueConfig{
		TTL:       queue.DefaultTTL,
		MaxMemory: 1024, // 1KB
	})
	defer q.Close()

	// Try to enqueue large data that exceeds limit
	largeData := make([]byte, 2048) // 2KB
	err := q.TryEnqueue(largeData)

	if err == nil {
		t.Fatal("Expected MemoryLimitError for oversized data")
	}

	var mlErr *queue.MemoryLimitError
	if !errors.As(err, &mlErr) {
		t.Errorf("Expected MemoryLimitError, got %T: %v", err, err)
	}

	// Verify fields are populated
	if mlErr.Max == 0 {
		t.Error("Expected Max to be set")
	}
	if mlErr.Needed == 0 {
		t.Error("Expected Needed to be set")
	}
}
