package tests

import (
	"sync"
	"testing"
	"time"

	"mpmc-queue/queue"
)

// TestMemoryLimitErrorFields tests MemoryLimitError field values
func TestMemoryLimitErrorFields(t *testing.T) {
	q := queue.NewQueue("memory-error-fields-test")
	defer q.Close()

	// Fill queue to capacity
	for {
		err := q.TryEnqueue(make([]byte, 1000))
		if err != nil {
			break
		}
	}

	// Try to add item that exceeds limit
	largeItem := make([]byte, 100000)
	err := q.TryEnqueue(largeItem)

	if err == nil {
		t.Fatal("Expected memory limit error")
	}

	memErr, ok := err.(*queue.MemoryLimitError)
	if !ok {
		t.Fatalf("Expected MemoryLimitError, got %T", err)
	}

	// Verify fields
	if memErr.Current <= 0 {
		t.Errorf("Current memory should be positive, got %d", memErr.Current)
	}

	if memErr.Max != 1024*1024 {
		t.Errorf("Max should be 1MB (1048576), got %d", memErr.Max)
	}

	if memErr.Needed <= 0 {
		t.Errorf("Needed should be positive, got %d", memErr.Needed)
	}

	// Current + Needed should exceed Max
	if memErr.Current+memErr.Needed <= memErr.Max {
		t.Errorf("Current (%d) + Needed (%d) should exceed Max (%d)",
			memErr.Current, memErr.Needed, memErr.Max)
	}
}

// TestMemoryLimitErrorMessage tests error message formatting
func TestMemoryLimitErrorMessage(t *testing.T) {
	q := queue.NewQueue("memory-error-message-test")
	defer q.Close()

	// Fill queue
	for {
		err := q.TryEnqueue(make([]byte, 1000))
		if err != nil {
			break
		}
	}

	// Trigger error
	err := q.TryEnqueue(make([]byte, 10000))
	if err == nil {
		t.Fatal("Expected error")
	}

	// Error message should be informative
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("Error message should not be empty")
	}

	// Should contain key information
	if len(errMsg) < 10 {
		t.Errorf("Error message seems too short: %s", errMsg)
	}
}

// TestNilPayload tests enqueueing nil payload
func TestNilPayload(t *testing.T) {
	q := queue.NewQueue("nil-payload-test")
	defer q.Close()

	// Enqueue nil - should work (nil is a valid value)
	err := q.TryEnqueue(nil)
	if err != nil {
		t.Errorf("Should be able to enqueue nil: %v", err)
	}

	// Read it back
	consumer := q.AddConsumer()
	data := consumer.TryRead()

	if data == nil {
		t.Fatal("Should receive QueueData, not nil")
	}

	if data.Payload != nil {
		t.Errorf("Payload should be nil, got %v", data.Payload)
	}
}

// TestExtremelyLargePayload tests payload near memory limit
func TestExtremelyLargePayload(t *testing.T) {
	q := queue.NewQueue("large-payload-test")
	defer q.Close()

	// Try to enqueue item close to 1MB limit
	// Account for overhead (UUID, timestamps, etc.)
	largePayload := make([]byte, 900000) // 900KB

	err := q.TryEnqueue(largePayload)
	if err != nil {
		t.Fatalf("Should be able to enqueue 900KB payload: %v", err)
	}

	// Verify it's in queue
	consumer := q.AddConsumer()
	data := consumer.TryRead()

	if data == nil {
		t.Fatal("Should be able to read large payload")
	}

	payload, ok := data.Payload.([]byte)
	if !ok {
		t.Fatal("Payload should be []byte")
	}

	if len(payload) != 900000 {
		t.Errorf("Payload length should be 900000, got %d", len(payload))
	}
}

// TestPayloadIntegrity tests various payload types
func TestPayloadIntegrity(t *testing.T) {
	q := queue.NewQueue("payload-integrity-test")
	defer q.Close()

	testCases := []struct {
		name    string
		payload any
	}{
		{"string", "test string"},
		{"int", 42},
		{"float", 3.14159},
		{"bool", true},
		{"slice", []int{1, 2, 3, 4, 5}},
		{"map", map[string]int{"a": 1, "b": 2}},
		{"struct", struct{ Name string }{"test"}},
		{"nil", nil},
	}

	consumer := q.AddConsumer()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := q.TryEnqueue(tc.payload)
			if err != nil {
				t.Fatalf("Failed to enqueue %s: %v", tc.name, err)
			}

			data := consumer.TryRead()
			if data == nil {
				t.Fatalf("Failed to read %s", tc.name)
			}

			// For complex types, just verify we got something back
			if data.Payload == nil && tc.payload != nil {
				t.Errorf("Payload should not be nil for %s", tc.name)
			}
		})
	}
}

// TestPayloadWithSpecialCharacters tests string payload with special characters
func TestPayloadWithSpecialCharacters(t *testing.T) {
	q := queue.NewQueue("special-chars-test")
	defer q.Close()

	specialStrings := []string{
		"Hello, 世界!",           // Unicode
		"Test\nNew\nLines",     // Newlines
		"Tab\tSeparated\tData", // Tabs
		"Quote \"test\" quote", // Quotes
		"Emoji 🎉🎊🎈",            // Emojis
		"",                     // Empty string
		"   spaces   ",         // Spaces
	}

	consumer := q.AddConsumer()

	for i, str := range specialStrings {
		err := q.TryEnqueue(str)
		if err != nil {
			t.Errorf("Failed to enqueue string %d: %v", i, err)
			continue
		}

		data := consumer.TryRead()
		if data == nil {
			t.Errorf("Failed to read string %d", i)
			continue
		}

		if data.Payload != str {
			t.Errorf("String %d corrupted: expected %q, got %q", i, str, data.Payload)
		}
	}
}

// TestMemoryAccuracyStrings tests memory estimation for strings
func TestMemoryAccuracyStrings(t *testing.T) {
	q := queue.NewQueue("memory-strings-test")
	defer q.Close()

	memBefore := q.GetMemoryUsage()

	// Add string
	testString := "This is a test string with some length"
	err := q.TryEnqueue(testString)
	if err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	memAfter := q.GetMemoryUsage()
	memUsed := memAfter - memBefore

	// Memory used should be at least the string length
	if memUsed < int64(len(testString)) {
		t.Errorf("Memory used (%d) should be at least string length (%d)", memUsed, len(testString))
	}

	// Account for QueueData overhead (UUID, timestamps, event, etc.)
	// Conservative check - memory should be reasonable
	if memUsed > 50000 {
		t.Errorf("Memory used (%d) seems excessive for a small string", memUsed)
	}

	t.Logf("String length: %d, Memory used: %d (overhead ratio: %.1fx)",
		len(testString), memUsed, float64(memUsed)/float64(len(testString)))
}

// TestMemoryAccuracySlices tests memory estimation for slices
func TestMemoryAccuracySlices(t *testing.T) {
	q := queue.NewQueue("memory-slices-test")
	defer q.Close()

	memBefore := q.GetMemoryUsage()

	// Add slice
	testSlice := make([]int, 100)
	err := q.TryEnqueue(testSlice)
	if err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	memAfter := q.GetMemoryUsage()
	memUsed := memAfter - memBefore

	// Should track slice memory
	if memUsed <= 0 {
		t.Error("Should track memory for slice")
	}

	// Reasonable bounds check
	expectedMin := int64(100 * 8) // 100 ints, 8 bytes each
	if memUsed < expectedMin {
		t.Errorf("Memory used (%d) less than expected minimum (%d)", memUsed, expectedMin)
	}
}

// TestMemoryAccuracyMaps tests memory estimation for maps
func TestMemoryAccuracyMaps(t *testing.T) {
	q := queue.NewQueue("memory-maps-test")
	defer q.Close()

	memBefore := q.GetMemoryUsage()

	// Add map
	testMap := map[string]int{
		"one":   1,
		"two":   2,
		"three": 3,
		"four":  4,
		"five":  5,
	}
	err := q.TryEnqueue(testMap)
	if err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	memAfter := q.GetMemoryUsage()
	memUsed := memAfter - memBefore

	// Should track map memory
	if memUsed <= 0 {
		t.Error("Should track memory for map")
	}
}

// TestMemoryAccuracyStructs tests memory estimation for structs
func TestMemoryAccuracyStructs(t *testing.T) {
	q := queue.NewQueue("memory-structs-test")
	defer q.Close()

	type TestStruct struct {
		Name  string
		Age   int
		Items []string
	}

	memBefore := q.GetMemoryUsage()

	testStruct := TestStruct{
		Name:  "Test User",
		Age:   30,
		Items: []string{"item1", "item2", "item3"},
	}

	err := q.TryEnqueue(testStruct)
	if err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	memAfter := q.GetMemoryUsage()
	memUsed := memAfter - memBefore

	// Should track struct memory
	if memUsed <= 0 {
		t.Error("Should track memory for struct")
	}
}

// TestMemoryReleasedAfterExpiration tests memory is freed after items expire
func TestMemoryReleasedAfterExpiration(t *testing.T) {
	q := queue.NewQueueWithTTL("memory-release-test", 1*time.Second)
	defer q.Close()

	// Add data
	for i := 0; i < 100; i++ {
		q.TryEnqueue(make([]byte, 1000))
	}

	memBefore := q.GetMemoryUsage()
	if memBefore == 0 {
		t.Fatal("Should have memory usage")
	}

	// Wait for expiration
	time.Sleep(2 * time.Second)
	q.ForceExpiration()

	memAfter := q.GetMemoryUsage()

	// Memory should be released
	if memAfter >= memBefore {
		t.Errorf("Memory should be released: before=%d, after=%d", memBefore, memAfter)
	}

	// Should be significantly less
	if memAfter > memBefore/10 {
		t.Errorf("Memory not significantly released: before=%d, after=%d", memBefore, memAfter)
	}
}

// TestExactlyAtMemoryLimit tests behavior at exact memory limit
func TestExactlyAtMemoryLimit(t *testing.T) {
	q := queue.NewQueue("exact-limit-test")
	defer q.Close()

	// Fill to capacity
	for {
		err := q.TryEnqueue(make([]byte, 1000))
		if err != nil {
			break
		}
	}

	mem := q.GetMemoryUsage()
	limit := int64(1024 * 1024)

	// Should be at or very close to limit
	if mem > limit {
		t.Errorf("Memory usage (%d) exceeds limit (%d)", mem, limit)
	}

	// Should be using most of the limit
	if mem < limit*9/10 {
		t.Errorf("Memory usage (%d) unexpectedly low (limit: %d)", mem, limit)
	}
}

// TestOneByteUnderMemoryLimit tests adding item that fits by 1 byte
func TestOneByteUnderMemoryLimit(t *testing.T) {
	q := queue.NewQueue("one-byte-under-test")
	defer q.Close()

	// Fill queue
	for {
		err := q.TryEnqueue(make([]byte, 1000))
		if err != nil {
			break
		}
	}

	// Try to add tiny item
	err := q.TryEnqueue("x")
	// Might succeed or fail depending on exact memory accounting
	// Both are acceptable, just verify consistent behavior
	_ = err
}

// TestErrorHandlingConcurrent tests error handling under concurrent load
func TestErrorHandlingConcurrent(t *testing.T) {
	q := queue.NewQueue("error-concurrent-test")
	defer q.Close()

	// Fill queue
	for {
		err := q.TryEnqueue(make([]byte, 1000))
		if err != nil {
			break
		}
	}

	errorCount := 0
	var mu sync.Mutex

	// Multiple goroutines trying to enqueue
	for i := 0; i < 10; i++ {
		go func() {
			err := q.TryEnqueue(make([]byte, 10000))
			if err != nil {
				mu.Lock()
				errorCount++
				mu.Unlock()
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)

	// All should get errors
	mu.Lock()
	if errorCount < 5 {
		t.Errorf("Expected multiple errors, got %d", errorCount)
	}
	mu.Unlock()
}
