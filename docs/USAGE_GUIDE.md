# mpmc-queue Usage Guide

Practical guide for using mpmc-queue in your applications.

## Table of Contents

- [Quick Start](#quick-start)
- [Common Patterns](#common-patterns)
- [Advanced Usage](#advanced-usage)
- [Error Handling](#error-handling)
- [Best Practices](#best-practices)
- [Migration Guide](#migration-guide)

---

## Quick Start

### Basic Producer-Consumer

```go
package main

import (
    "fmt"
    "mpmc-queue/queue"
)

func main() {
    // Create queue
    q := queue.NewQueue("my-queue")
    defer q.Close()

    // Produce
    q.Enqueue("Hello, World!")

    // Consume
    consumer := q.AddConsumer()
    data := consumer.Read()
    
    if data != nil {
        fmt.Println(data.Payload) // Output: Hello, World!
    }
}
```

---

### Multiple Consumers

```go
q := queue.NewQueue("broadcast-queue")
defer q.Close()

// Add data
q.Enqueue("Message 1")
q.Enqueue("Message 2")

// Create 3 independent consumers
consumer1 := q.AddConsumer()
consumer2 := q.AddConsumer()
consumer3 := q.AddConsumer()

// Each consumer reads all items independently
for i := 0; i < 2; i++ {
    data1 := consumer1.Read()
    data2 := consumer2.Read()
    data3 := consumer3.Read()
    
    // All three get the same data
    fmt.Printf("C1: %v, C2: %v, C3: %v\n", 
        data1.Payload, data2.Payload, data3.Payload)
}
```

---

### Concurrent Producers

```go
q := queue.NewQueue("worker-queue")
defer q.Close()

var wg sync.WaitGroup

// Start 10 producers
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        for j := 0; j < 100; j++ {
            payload := fmt.Sprintf("Producer %d, Item %d", id, j)
            if err := q.Enqueue(payload); err != nil {
                log.Printf("Enqueue failed: %v", err)
            }
        }
    }(i)
}

wg.Wait()
```

---

## Common Patterns

### Worker Pool Pattern

```go
func processWithWorkerPool(q *queue.Queue, numWorkers int) {
    var wg sync.WaitGroup

    // Start worker goroutines
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            consumer := q.AddConsumer()
            
            for {
                data := consumer.Read()
                if data == nil {
                    time.Sleep(100 * time.Millisecond)
                    if !consumer.HasMoreData() {
                        break
                    }
                    continue
                }
                
                // Process item
                processItem(data.Payload)
            }
        }(i)
    }

    wg.Wait()
}
```

---

### Batch Processing Pattern

```go
func batchProcessor(consumer *queue.Consumer, batchSize int) {
    for {
        batch := consumer.ReadBatch(batchSize)
        if len(batch) == 0 {
            break // No more data
        }
        
        // Process batch efficiently
        processBatch(batch)
    }
}

func produceBatch(q *queue.Queue, items []any) error {
    // Atomic batch enqueue
    return q.EnqueueBatch(items)
}
```

---

### Event Streaming Pattern

```go
func eventStream(q *queue.Queue) {
    consumer := q.AddConsumer()
    
    for {
        data := consumer.Read()
        if data == nil {
            time.Sleep(50 * time.Millisecond)
            continue
        }
        
        // Stream events in order
        event := data.Payload.(Event)
        handleEvent(event)
    }
}
```

---

### Notification Pattern

```go
func consumerWithNotifications(consumer *queue.Consumer) {
    // Monitor expiration notifications
    go func() {
        for expiredCount := range consumer.GetNotificationChannel() {
            log.Printf("Warning: %d items expired before being read", expiredCount)
        }
    }()

    // Read data
    for {
        data := consumer.Read()
        if data != nil {
            processData(data)
        }
    }
}
```

---

### Memory-Aware Enqueue

```go
func enqueueWithMemoryCheck(q *queue.Queue, payload any) error {
    // Check memory before enqueue
    stats := q.GetQueueStats()
    if stats.MemoryPercent > 90.0 {
        log.Printf("Warning: Queue memory at %.1f%%", stats.MemoryPercent)
    }

    err := q.Enqueue(payload)
    if memErr, ok := err.(*queue.MemoryLimitError); ok {
        log.Printf("Memory limit: %d/%d bytes (need %d more)",
            memErr.Current, memErr.Max, memErr.Needed)
        
        // Option 1: Wait for expiration
        time.Sleep(1 * time.Second)
        return enqueueWithMemoryCheck(q, payload)
        
        // Option 2: Force expiration
        // q.ForceExpiration()
        // return q.Enqueue(payload)
    }
    
    return err
}
```

---

### Graceful Shutdown Pattern

```go
func gracefulShutdown(q *queue.Queue, consumers []*queue.Consumer) {
    // Signal consumers to stop
    done := make(chan struct{})
    
    // Wait for consumers to finish reading
    var wg sync.WaitGroup
    for _, consumer := range consumers {
        wg.Add(1)
        go func(c *queue.Consumer) {
            defer wg.Done()
            
            // Drain remaining items
            for c.HasMoreData() {
                data := c.Read()
                if data != nil {
                    processData(data)
                }
            }
        }(consumer)
    }
    
    // Wait with timeout
    go func() {
        wg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        log.Println("All consumers finished")
    case <-time.After(30 * time.Second):
        log.Println("Timeout waiting for consumers")
    }
    
    // Close queue
    q.Close()
}
```

---

## Advanced Usage

### Custom TTL per Queue

```go
// Short-lived cache
cache := queue.NewQueueWithTTL("cache", 5*time.Minute)
defer cache.Close()

// Long-lived audit log
auditLog := queue.NewQueueWithTTL("audit", 24*time.Hour)
defer auditLog.Close()
```

---

### Dynamic TTL Adjustment

```go
q := queue.NewQueue("adaptive-queue")
defer q.Close()

// Start with short TTL
q.SetTTL(5 * time.Minute)

// Later, increase based on load
if q.GetQueueStats().MemoryPercent < 50.0 {
    q.SetTTL(30 * time.Minute) // Keep items longer
}
```

---

### Disable Expiration for Critical Data

```go
q := queue.NewQueue("critical-queue")
defer q.Close()

// Disable automatic expiration
q.DisableExpiration()

// Process critical data without time pressure
consumer := q.AddConsumer()
for consumer.HasMoreData() {
    data := consumer.Read()
    processCriticalData(data)
}

// Manually clean up when done
q.EnableExpiration()
q.ForceExpiration()
```

---

### Consumer Position Tracking

```go
consumer := q.AddConsumer()

// Read some items
for i := 0; i < 10; i++ {
    consumer.Read()
}

// Check position
element, index := consumer.GetPosition()
fmt.Printf("Consumer at chunk element, index %d\n", index)

// Check unread count
remaining := consumer.GetUnreadCount()
fmt.Printf("%d items remaining\n", remaining)

// Get read history
history := consumer.GetDequeueHistory()
fmt.Printf("Read %d items\n", len(history))
```

---

### Type-Safe Enqueue/Dequeue

```go
type Message struct {
    ID   int
    Text string
}

// Enqueue
msg := Message{ID: 1, Text: "Hello"}
q.Enqueue(msg)

// Dequeue with type assertion
consumer := q.AddConsumer()
data := consumer.Read()
if data != nil {
    if msg, ok := data.Payload.(Message); ok {
        fmt.Printf("Message: %s\n", msg.Text)
    }
}
```

---

## Error Handling

### Memory Limit Errors

```go
err := q.Enqueue(largePayload)

switch e := err.(type) {
case *queue.MemoryLimitError:
    fmt.Printf("Out of memory: %d/%d bytes\n", e.Current, e.Max)
    
    // Strategy 1: Wait and retry
    time.Sleep(1 * time.Second)
    q.ForceExpiration()
    err = q.Enqueue(largePayload)
    
    // Strategy 2: Drop oldest items manually
    // (Not directly supported - let expiration handle it)
    
    // Strategy 3: Reject and return error to caller
    return fmt.Errorf("queue full: %w", err)
    
case *queue.QueueError:
    fmt.Printf("Queue error: %s\n", e.Message)
    
default:
    if err != nil {
        fmt.Printf("Unexpected error: %v\n", err)
    }
}
```

---

### Consumer Not Found

```go
consumerID := consumer.GetID()

// Later...
if !q.RemoveConsumer(consumerID) {
    log.Printf("Consumer %s not found (may have been removed)", consumerID)
}

// Check before use
if c := q.GetConsumer(consumerID); c != nil {
    data := c.Read()
} else {
    log.Printf("Consumer no longer exists")
}
```

---

### Nil Data Checks

```go
consumer := q.AddConsumer()

// Always check for nil
data := consumer.Read()
if data == nil {
    // No data available or consumer caught up
    return
}

// Safe to access
payload := data.Payload
```

---

## Best Practices

### 1. Always Use defer Close()

```go
// ✅ Correct
q := queue.NewQueue("my-queue")
defer q.Close()

// ❌ Wrong - may leak resources
q := queue.NewQueue("my-queue")
// ... forgot to close
```

---

### 2. Check Memory Before Batch Operations

```go
// ✅ Correct
if q.GetMemoryUsage() < queue.MaxQueueMemory * 0.8 {
    q.EnqueueBatch(largeBatch)
} else {
    // Split batch or wait
}

// ❌ Wrong - may fail unexpectedly
q.EnqueueBatch(hugeBatch) // Might fail if too large
```

---

### 3. Monitor Consumer Lag

```go
// ✅ Correct
for _, consumer := range q.GetAllConsumers() {
    stats := consumer.GetStats()
    if stats.UnreadItems > 1000 {
        log.Printf("Consumer %s is lagging: %d unread items", 
            stats.ID, stats.UnreadItems)
    }
}
```

---

### 4. Use Batch Operations for Efficiency

```go
// ✅ Efficient
items := []any{"a", "b", "c", "d", "e"}
q.EnqueueBatch(items) // Single lock acquisition

// ❌ Inefficient  
for _, item := range items {
    q.Enqueue(item) // Multiple lock acquisitions
}
```

---

### 5. Handle Expiration Gracefully

```go
// ✅ Correct - Monitor notifications
go func() {
    for expiredCount := range consumer.GetNotificationChannel() {
        metrics.IncrementExpiredItems(expiredCount)
        log.Printf("Missed %d items due to expiration", expiredCount)
    }
}()

// ✅ Alternative - Adjust TTL if too many expirations
stats := q.GetQueueStats()
if tooManyExpirations() {
    q.SetTTL(stats.TTL * 2) // Double the TTL
}
```

---

### 6. Proper Consumer Cleanup

```go
// ✅ Correct
consumer := q.AddConsumer()
consumerID := consumer.GetID()

// When done with consumer
q.RemoveConsumer(consumerID)
consumer.Close()

// ❌ Wrong - consumer leaks
consumer := q.AddConsumer()
// ... forgot to remove
```

---

## Migration Guide

### From channel-based queues

**Before (Go channels):**
```go
ch := make(chan any, 100)

// Producer
ch <- data

// Consumer
data := <-ch
```

**After (mpmc-queue):**
```go
q := queue.NewQueue("my-queue")
defer q.Close()

// Producer
q.Enqueue(data)

// Consumer
consumer := q.AddConsumer()
data := consumer.Read()
```

**Benefits:**
- ✅ Multiple independent consumers
- ✅ Memory limit enforcement
- ✅ Automatic expiration
- ✅ Statistics and monitoring

---

### From sync.Map or custom queues

**Before:**
```go
var mu sync.Mutex
var items []any

// Enqueue
mu.Lock()
items = append(items, data)
mu.Unlock()

// Dequeue
mu.Lock()
if len(items) > 0 {
    data = items[0]
    items = items[1:]
}
mu.Unlock()
```

**After:**
```go
q := queue.NewQueue("my-queue")
defer q.Close()

// Enqueue
q.Enqueue(data)

// Dequeue
consumer := q.AddConsumer()
data := consumer.Read()
```

**Benefits:**
- ✅ No slice growth/shrink overhead
- ✅ Chunked storage
- ✅ Built-in memory limits
- ✅ Multiple consumers support

---

## Real-World Examples

### Message Queue

```go
type Message struct {
    Topic   string
    Body    []byte
    Headers map[string]string
}

func main() {
    q := queue.NewQueueWithTTL("messages", 1*time.Hour)
    defer q.Close()

    // Publisher
    go func() {
        for msg := range incomingMessages {
            if err := q.Enqueue(msg); err != nil {
                log.Printf("Failed to enqueue: %v", err)
            }
        }
    }()

    // Subscribers
    for i := 0; i < 5; i++ {
        go func(id int) {
            consumer := q.AddConsumer()
            for {
                data := consumer.Read()
                if data == nil {
                    time.Sleep(100 * time.Millisecond)
                    continue
                }
                
                msg := data.Payload.(Message)
                handleMessage(msg)
            }
        }(i)
    }
}
```

---

### Event Log

```go
type Event struct {
    Timestamp time.Time
    Level     string
    Message   string
}

func main() {
    // Keep events for 24 hours
    eventLog := queue.NewQueueWithTTL("events", 24*time.Hour)
    defer eventLog.Close()

    // Log events
    logEvent := func(level, msg string) {
        event := Event{
            Timestamp: time.Now(),
            Level:     level,
            Message:   msg,
        }
        eventLog.Enqueue(event)
    }

    // Query recent events
    queryEvents := func() []Event {
        consumer := eventLog.AddConsumer()
        defer eventLog.RemoveConsumer(consumer.GetID())
        
        events := []Event{}
        for consumer.HasMoreData() {
            data := consumer.Read()
            if data != nil {
                events = append(events, data.Payload.(Event))
            }
        }
        return events
    }

    logEvent("INFO", "System started")
    logEvent("ERROR", "Connection failed")
    
    events := queryEvents()
    fmt.Printf("Found %d events\n", len(events))
}
```

---

### Task Queue with Retry

```go
type Task struct {
    ID      string
    Payload any
    Retries int
}

func taskWorker(q *queue.Queue, maxRetries int) {
    consumer := q.AddConsumer()
    
    for {
        data := consumer.Read()
        if data == nil {
            time.Sleep(100 * time.Millisecond)
            continue
        }
        
        task := data.Payload.(Task)
        
        // Try to execute
        if err := executeTask(task); err != nil {
            // Retry logic
            if task.Retries < maxRetries {
                task.Retries++
                q.Enqueue(task) // Re-enqueue
                log.Printf("Task %s failed, retry %d/%d", 
                    task.ID, task.Retries, maxRetries)
            } else {
                log.Printf("Task %s failed permanently", task.ID)
            }
        }
    }
}
```

---

### Cache with Expiration

```go
type CacheEntry struct {
    Key   string
    Value any
}

func main() {
    // 5-minute cache
    cache := queue.NewQueueWithTTL("cache", 5*time.Minute)
    defer cache.Close()

    // Write to cache
    set := func(key string, value any) {
        entry := CacheEntry{Key: key, Value: value}
        cache.Enqueue(entry)
    }

    // Read from cache (scan for key)
    get := func(key string) any {
        consumer := cache.AddConsumer()
        defer cache.RemoveConsumer(consumer.GetID())
        
        for consumer.HasMoreData() {
            data := consumer.Read()
            if data != nil {
                entry := data.Payload.(CacheEntry)
                if entry.Key == key {
                    return entry.Value
                }
            }
        }
        return nil
    }

    set("user:123", User{Name: "Alice"})
    user := get("user:123")
}
```

---

## Performance Tips

### 1. Use Batch Operations

```go
// Fast: Single lock acquisition
items := make([]any, 100)
for i := range items {
    items[i] = generateItem(i)
}
q.EnqueueBatch(items)

// Slow: 100 lock acquisitions
for i := 0; i < 100; i++ {
    q.Enqueue(generateItem(i))
}
```

---

### 2. Pre-allocate Consumer Slice

```go
// Efficient
batch := consumer.ReadBatch(1000)

// Less efficient
var items []*queue.QueueData
for i := 0; i < 1000; i++ {
    data := consumer.Read()
    if data != nil {
        items = append(items, data)
    }
}
```

---

### 3. Monitor Queue Stats

```go
ticker := time.NewTicker(10 * time.Second)
go func() {
    for range ticker.C {
        stats := q.GetQueueStats()
        log.Printf("%s", stats.String())
        
        // Adjust based on stats
        if stats.MemoryPercent > 80 {
            q.SetTTL(q.GetTTL() / 2) // Expire faster
        }
    }
}()
```

---

### 4. Limit Consumer Count

```go
const maxConsumers = 10

func addConsumerSafe(q *queue.Queue) (*queue.Consumer, error) {
    if len(q.GetAllConsumers()) >= maxConsumers {
        return nil, fmt.Errorf("max consumers reached")
    }
    return q.AddConsumer(), nil
}
```

---

## Testing Your Code

### Mock Queue for Testing

```go
func TestMyFunction(t *testing.T) {
    // Create test queue
    q := queue.NewQueue("test-queue")
    defer q.Close()

    // Add test data
    q.Enqueue("test-item-1")
    q.Enqueue("test-item-2")

    // Test your function
    consumer := q.AddConsumer()
    result := myFunction(consumer)

    // Verify
    if result != expected {
        t.Errorf("Expected %v, got %v", expected, result)
    }
}
```

---

### Race Detection

Always test with race detection:

```bash
go test -race ./...
```

---

## See Also

- [API Documentation](./API.md)
- [Architecture Guide](./ARCHITECTURE.md)
- [Performance Guide](./PERFORMANCE.md)
- [Troubleshooting](./TROUBLESHOOTING.md)
