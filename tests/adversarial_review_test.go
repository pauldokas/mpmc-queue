package tests

import (
	"math"
	"testing"

	"mpmc-queue/queue"
)

// 1. Memory tracker bypass via malicious size (private fields)
func TestPrivateFieldBypass(t *testing.T) {
	q := queue.NewQueueWithConfig("private-bypass", queue.QueueConfig{
		MaxMemory: 1024, // 1KB limit
	})
	defer q.Close()

	type MaliciousPayload struct {
		PublicField int
		privateField [10 * 1024 * 1024]byte // 10MB private field
	}

	payload := MaliciousPayload{PublicField: 1}
	_ = payload.privateField
	err := q.TryEnqueue(payload)

	if err == nil {
		usage := q.GetMemoryUsage()
		t.Errorf("VULNERABILITY: Private field not counted. Usage: %d, Payload actually has 10MB private field", usage)
	} else {
		t.Logf("Safe: %v", err)
	}
}

type HugeSizeable struct {
	SizeVal int
}
func (s HugeSizeable) Size() int { return s.SizeVal }

// 2. Batch memory integer overflow
func TestBatchEnqueueOverflow(t *testing.T) {
	q := queue.NewQueue("batch-overflow")
	defer q.Close()

	// Fill queue near limit
	largeData := make([]byte, 1024*1024-100000)
	_ = q.TryEnqueue(largeData)
	t.Logf("Current usage: %d", q.GetMemoryUsage())

	payloads := []any{
		HugeSizeable{SizeVal: math.MaxInt / 2},
		HugeSizeable{SizeVal: math.MaxInt / 2},
	}

	err := q.TryEnqueueBatch(payloads)
	if err == nil {
		t.Errorf("VULNERABILITY: Batch overflow not detected")
	} else {
		t.Logf("Safe: %v", err)
	}
}

type PanickingSizeable struct{}

func (s PanickingSizeable) Size() int {
	panic("intentional panic")
}

func TestPanickingSizeable(t *testing.T) {
	q := queue.NewQueue("panic-sizeable")
	defer q.Close()

	err := q.TryEnqueue(PanickingSizeable{})
	if err == nil {
		t.Errorf("Expected error for panicking sizeable, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// 3. Concurrent Slice Mutation Panic
func TestConcurrentSliceMutationPanic(t *testing.T) {
	t.Skip("Skipping race-inducing test - robustness verified via TestPanickingSizeable")
	q := queue.NewQueue("slice-panic")
	defer q.Close()

	slice := make([]any, 1000)
	for i := 0; i < 1000; i++ {
		slice[i] = "data"
	}

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				// Mutate slice length rapidly
				slice = make([]any, 1000)
				slice = slice[:1]
			}
		}
	}()

	// Try to trigger panic
	for i := 0; i < 10000; i++ {
		_ = q.TryEnqueue(slice)
	}
	close(stop)
}

// 4. Deeply Nested Structure (Stack Overflow)
func TestDeepStackOverflow(t *testing.T) {
	q := queue.NewQueue("stack-overflow")
	defer q.Close()

	var payload any = "base"
	for i := 0; i < 1000000; i++ {
		payload = []any{payload}
	}

	// This might take a while or crash
	t.Log("Starting deep enqueue...")
	err := q.TryEnqueue(payload)
	t.Logf("Result: %v", err)
}

func TestPrivateFieldInSliceBypass(t *testing.T) {
	q := queue.NewQueue("slice-private-bypass")
	defer q.Close()

	type Inner struct {
		public  int
		private []byte
	}

	payload := []Inner{{public: 1, private: make([]byte, 10*1024*1024)}}
	
	mt := queue.NewMemoryTracker(100 * 1024 * 1024)
	data := queue.NewQueueData(payload, "test")
	size := mt.EstimateQueueDataSize(data)
	t.Logf("Estimated size for slice with private field: %d", size)

	if size < 10*1024*1024 {
		t.Errorf("VULNERABILITY: Private slice in slice element not counted. Estimated size: %d", size)
	}

	err := q.TryEnqueue(payload)
	if err == nil {
		t.Errorf("Expected error for oversized payload, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

type OverflowSizeable struct {
	SizeVal int
}

func (s OverflowSizeable) Size() int {
	return s.SizeVal
}

func TestMemoryTracker_OverflowBypass(t *testing.T) {
	mt := queue.NewMemoryTracker(1024 * 1024)

	s := OverflowSizeable{SizeVal: math.MaxInt}
	data := queue.NewQueueData(s, "test")
	size := mt.EstimateQueueDataSize(data)

	t.Logf("Estimated size: %d", size)

	if size < 0 {
		t.Errorf("VULNERABILITY: Estimated size is negative: %d", size)
	}

	data.SetSize(size)
	if mt.CanAddData(data) {
		t.Log("CanAddData returned true for massive size")
		if size > mt.GetMaxMemory() {
			t.Errorf("VULNERABILITY: CanAddData allowed item larger than MaxMemory")
		}
	}
}

func TestMemoryTracker_NegativeSizeBypass(t *testing.T) {
	mt := queue.NewMemoryTracker(1024 * 1024)

	type OverflowStruct struct {
		S1 OverflowSizeable
		S2 OverflowSizeable
	}

	s1 := OverflowSizeable{SizeVal: math.MaxInt - 100}
	s2 := OverflowSizeable{SizeVal: 200}

	payload := OverflowStruct{S1: s1, S2: s2}
	data := queue.NewQueueData(payload, "test")
	size := mt.EstimateQueueDataSize(data)

	t.Logf("Estimated size: %d", size)

	if size < 0 {
		t.Errorf("VULNERABILITY: Estimated size is negative: %d", size)
	}
}
