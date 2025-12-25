# Race Condition Fix Strategy

## Problem Analysis

### Current Race Condition

**Location**: `queue/consumer.go:89` and `queue/queue.go:71`

**Issue**: Multiple consumers can call `data.AddEvent()` concurrently on the same `QueueData` instance, causing a race condition when appending to the `Events` slice.

```go
// In consumer.go:89 (called by multiple consumers concurrently)
data.AddEvent(c.queue.name, "dequeue")

// In data.go:41
func (qd *QueueData) AddEvent(queueName, eventType string) {
    qd.Events = append(qd.Events, event)  // ⚠️ NOT THREAD-SAFE!
}
```

**Root Cause**: 
1. A single `QueueData` instance is shared across multiple consumers
2. Each consumer calls `AddEvent()` without synchronization
3. `append()` on a slice is not atomic and can corrupt memory when called concurrently

**Related Issue**: Memory tracking doesn't account for growing `Events` slice after initial allocation.

## Solution Strategies

### Strategy 1: Make QueueData Immutable After Enqueue (RECOMMENDED)

**Approach**: Stop modifying `QueueData` after it's enqueued. Record dequeue events elsewhere.

#### Pros:
- ✅ Eliminates race condition completely
- ✅ Best performance (no locking on read path)
- ✅ Fixes memory tracking issue (no size changes after enqueue)
- ✅ Aligns with immutable data patterns
- ✅ Simplifies reasoning about data ownership

#### Cons:
- ❌ API change (no dequeue events in QueueData)
- ❌ Requires alternative tracking for dequeue events

#### Implementation Plan:

##### Step 1: Modify QueueData to only track enqueue
```go
// queue/data.go

// QueueData represents a single item in the queue
type QueueData struct {
    ID           string        `json:"id"`
    Payload      interface{}   `json:"payload"`
    EnqueueEvent QueueEvent    `json:"enqueue_event"` // Single event, not slice
    Created      time.Time     `json:"created"`
}

// NewQueueData creates a new QueueData instance with enqueue event
func NewQueueData(payload interface{}, queueName string) *QueueData {
    return &QueueData{
        ID:      uuid.New().String(),
        Payload: payload,
        EnqueueEvent: QueueEvent{
            Timestamp: time.Now(),
            QueueName: queueName,
            EventType: "enqueue",
        },
        Created: time.Now(),
    }
}

// Remove AddEvent method entirely - QueueData is now immutable after creation
```

##### Step 2: Track dequeue events in Consumer
```go
// queue/consumer.go

type Consumer struct {
    id               string
    chunkElement     *list.Element
    indexInChunk     int
    notificationCh   chan int
    mutex            sync.Mutex
    queue            *Queue
    lastReadTime     time.Time
    totalItemsRead   int64
    dequeueHistory   []DequeueRecord  // Track locally
}

type DequeueRecord struct {
    DataID    string
    Timestamp time.Time
}

func (c *Consumer) Read() *QueueData {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    // ... existing position logic ...
    
    if data != nil {
        // Record dequeue locally instead of modifying data
        c.dequeueHistory = append(c.dequeueHistory, DequeueRecord{
            DataID:    data.ID,
            Timestamp: time.Now(),
        })
        
        c.indexInChunk++
        c.lastReadTime = time.Now()
        c.totalItemsRead++
        
        return data
    }
    
    // ...
}

// GetDequeueHistory returns dequeue events for this consumer
func (c *Consumer) GetDequeueHistory() []DequeueRecord {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    // Return copy to prevent external modification
    history := make([]DequeueRecord, len(c.dequeueHistory))
    copy(history, c.dequeueHistory)
    return history
}
```

##### Step 3: Update Queue.Enqueue
```go
// queue/queue.go

func (q *Queue) Enqueue(payload interface{}) error {
    data := NewQueueData(payload, q.name)  // Pass queue name here
    
    q.mutex.Lock()
    defer q.mutex.Unlock()
    
    return q.data.Enqueue(data)
}
```

##### Step 4: Update memory estimation (one-time calculation)
```go
// queue/memory.go

func (mt *MemoryTracker) EstimateQueueDataSize(data *QueueData) int64 {
    if data == nil {
        return 0
    }
    
    size := BaseQueueDataSize
    size += int64(len(data.ID))
    size += mt.estimatePayloadSize(data.Payload)
    size += BaseQueueEventSize  // Single event, not slice
    size += int64(len(data.EnqueueEvent.QueueName))
    size += int64(len(data.EnqueueEvent.EventType))
    
    return size
}
```

##### Step 5: Update tests
```go
// tests/queue_test.go

func TestEnqueueDequeue(t *testing.T) {
    q := queue.NewQueue("test-queue")
    defer q.Close()

    testData := "test payload"
    err := q.Enqueue(testData)
    if err != nil {
        t.Fatalf("Failed to enqueue: %v", err)
    }

    consumer := q.AddConsumer()
    data := consumer.Read()
    
    if data == nil {
        t.Fatal("Expected data, got nil")
    }

    // Verify enqueue event
    if data.EnqueueEvent.EventType != "enqueue" {
        t.Errorf("Expected enqueue event, got '%s'", data.EnqueueEvent.EventType)
    }
    
    // Verify dequeue history in consumer
    history := consumer.GetDequeueHistory()
    if len(history) != 1 {
        t.Errorf("Expected 1 dequeue record, got %d", len(history))
    }
}
```

---

### Strategy 2: Add Mutex to QueueData (ALTERNATIVE)

**Approach**: Protect `Events` slice with a mutex inside `QueueData`.

#### Pros:
- ✅ Minimal API change
- ✅ Preserves event history in QueueData

#### Cons:
- ❌ Performance overhead (locking on every read)
- ❌ Complex memory tracking (need to track growing slice)
- ❌ More complex ownership semantics
- ❌ Doesn't scale well with many consumers

#### Implementation (if chosen):

```go
// queue/data.go

type QueueData struct {
    ID      string        `json:"id"`
    Payload interface{}   `json:"payload"`
    events  []QueueEvent  // Make private
    eventsMu sync.Mutex   // Protect events
    Created time.Time     `json:"created"`
}

func (qd *QueueData) AddEvent(queueName, eventType string) {
    qd.eventsMu.Lock()
    defer qd.eventsMu.Unlock()
    
    event := QueueEvent{
        Timestamp: time.Now(),
        QueueName: queueName,
        EventType: eventType,
    }
    qd.events = append(qd.events, event)
}

func (qd *QueueData) GetEvents() []QueueEvent {
    qd.eventsMu.Lock()
    defer qd.eventsMu.Unlock()
    
    // Return copy to prevent external modification
    eventsCopy := make([]QueueEvent, len(qd.events))
    copy(eventsCopy, qd.events)
    return eventsCopy
}
```

**Issues with this approach**:
1. Still need to handle dynamic memory tracking
2. Need to notify MemoryTracker when events grow
3. Performance impact on hot read path
4. JSON marshaling needs custom MarshalJSON

---

### Strategy 3: Pre-allocate Events Capacity (NOT RECOMMENDED)

**Approach**: Pre-allocate enough capacity for all expected events.

#### Cons:
- ❌ Wastes memory
- ❌ Still has race condition if capacity exceeded
- ❌ Hard to predict max consumers
- ❌ Doesn't solve the fundamental problem

---

## Recommended Implementation: Strategy 1

### Migration Path

#### Phase 1: Add new API alongside old (v1.1.0)
```go
// Deprecated: Use GetEnqueueEvent() instead. This will be removed in v2.0.
func (qd *QueueData) GetEvents() []QueueEvent {
    return []QueueEvent{qd.EnqueueEvent}
}

// GetEnqueueEvent returns the enqueue event for this data
func (qd *QueueData) GetEnqueueEvent() QueueEvent {
    return qd.EnqueueEvent
}
```

#### Phase 2: Update documentation (v1.1.0)
- Add deprecation notices
- Document new Consumer.GetDequeueHistory() method
- Provide migration examples

#### Phase 3: Remove old API (v2.0.0)
- Remove Events slice entirely
- Remove GetEvents() method

### Testing Strategy

#### 1. Add race detection test
```go
// tests/race_test.go

func TestConcurrentDequeueNoRace(t *testing.T) {
    q := queue.NewQueue("race-test")
    defer q.Close()
    
    // Enqueue single item
    q.Enqueue("shared data")
    
    // Create multiple consumers reading same item
    const numConsumers = 100
    var wg sync.WaitGroup
    wg.Add(numConsumers)
    
    for i := 0; i < numConsumers; i++ {
        go func() {
            defer wg.Done()
            consumer := q.AddConsumer()
            data := consumer.Read()
            if data != nil {
                // This should not race
                _ = data.ID
                _ = data.Payload
                _ = data.EnqueueEvent
            }
        }()
    }
    
    wg.Wait()
}
```

#### 2. Verify with race detector
```bash
go test -race ./tests -run TestConcurrentDequeueNoRace
```

#### 3. Run all existing tests with race detector
```bash
go test -race ./tests -v
```

### Verification Checklist

- [ ] All tests pass without race detector
- [ ] All tests pass with race detector (`go test -race`)
- [ ] Memory tracking is accurate (doesn't grow after enqueue)
- [ ] Benchmark performance (should improve without locking)
- [ ] Update all documentation
- [ ] Add migration guide for users
- [ ] Update examples to show new API

## Timeline

- **Week 1**: Implement Strategy 1 (immutable QueueData)
- **Week 2**: Update all tests and fix any issues
- **Week 3**: Run extensive race detection and stress tests
- **Week 4**: Update documentation and examples

## Additional Improvements

While fixing this race condition, consider:

1. **Add integration test suite**
   - High concurrency scenarios
   - Long-running stress tests
   - Memory pressure tests

2. **Add benchmark comparison**
   - Before/after performance comparison
   - Memory allocation comparison

3. **Document thread-safety guarantees**
   - Clear documentation of what's safe
   - What data can be read without synchronization
   - What operations require coordination

## References

- Go Data Race Detector: https://go.dev/doc/articles/race_detector
- Go Memory Model: https://go.dev/ref/mem
- Effective Go - Concurrency: https://go.dev/doc/effective_go#concurrency

---

Last Updated: 2025-12-25
