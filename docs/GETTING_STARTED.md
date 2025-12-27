# Getting Started with mpmc-queue

This tutorial will guide you from installation to building a production-ready application using mpmc-queue.

## Table of Contents

- [Installation](#installation)
- [First Queue](#first-queue)
- [Understanding MPMC Behavior](#understanding-mpmc-behavior)
- [Error Handling](#error-handling)
- [Production Patterns](#production-patterns)
- [Monitoring and Observability](#monitoring-and-observability)
- [Common Pitfalls](#common-pitfalls)
- [Next Steps](#next-steps)

---

## Installation

### Prerequisites

- Go 1.25 or higher
- Basic understanding of Go concurrency (goroutines, channels)

### Install the Module

```bash
go get mpmc-queue
```

Or add to your `go.mod`:

```go
require mpmc-queue v1.0.0
```

Then run:

```bash
go mod download
```

---

## First Queue

Let's create your first queue and perform basic operations.

### Step 1: Create a Queue

```go
package main

import (
    "fmt"
    "mpmc-queue/queue"
)

func main() {
    // Create a new queue with default settings
    // - 10-minute TTL (items auto-expire after 10 minutes)
    // - 1MB memory limit
    q := queue.NewQueue("my-first-queue")
    defer q.Close() // Always close when done
    
    fmt.Println("Queue created:", q.GetName())
}
```

### Step 2: Add Data (Enqueue)

```go
// Simple enqueue
err := q.TryEnqueue("Hello, World!")
if err != nil {
    fmt.Printf("Failed to enqueue: %v\n", err)
    return
}

// Enqueue different types
q.TryEnqueue(42)
q.TryEnqueue([]string{"apple", "banana", "cherry"})
q.TryEnqueue(map[string]int{"count": 100})

fmt.Println("4 items enqueued")
```

### Step 3: Read Data (Dequeue)

```go
// Create a consumer
consumer := q.AddConsumer()

// Read an item
data := consumer.TryRead()
if data != nil {
    fmt.Printf("Read: %v\n", data.Payload)
    fmt.Printf("ID: %s\n", data.ID)
    fmt.Printf("Enqueued at: %v\n", data.EnqueueEvent.Timestamp)
}
```

### Step 4: Run It!

```bash
go run main.go
```

**Output:**
```
Queue created: my-first-queue
4 items enqueued
Read: Hello, World!
ID: a1b2c3d4-...
Enqueued at: 2025-12-27 10:30:45.123 +0000 UTC
```

---

## Understanding MPMC Behavior

**Important:** mpmc-queue is a **broadcast** queue, not a work distribution queue.

### Key Concept: Each Consumer Reads ALL Items

```go
q := queue.NewQueue("broadcast")
defer q.Close()

// Add 3 items
q.TryEnqueue("Item 1")
q.TryEnqueue("Item 2")
q.TryEnqueue("Item 3")

// Create 2 consumers
consumer1 := q.AddConsumer()
consumer2 := q.AddConsumer()

// Consumer 1 reads all 3 items
for i := 0; i < 3; i++ {
    data := consumer1.TryRead()
    fmt.Printf("Consumer 1 read: %v\n", data.Payload)
}

// Consumer 2 also reads all 3 items (independent position)
for i := 0; i < 3; i++ {
    data := consumer2.TryRead()
    fmt.Printf("Consumer 2 read: %v\n", data.Payload)
}
```

**Output:**
```
Consumer 1 read: Item 1
Consumer 1 read: Item 2
Consumer 1 read: Item 3
Consumer 2 read: Item 1
Consumer 2 read: Item 2
Consumer 2 read: Item 3
```

### Use Cases for MPMC

✅ **Good for:**
- Event broadcasting to multiple subscribers
- Log/audit trail distribution
- Multi-stage processing pipelines
- Event sourcing patterns

❌ **Not ideal for:**
- Load balancing work across workers (use consumer groups pattern or separate queues)

---

## Error Handling

### Memory Limit Errors

The queue enforces a configurable memory limit (default 1MB).

```go
err := q.TryEnqueue(largeData)
if err != nil {
    // Check for specific error types
    if memErr, ok := err.(*queue.MemoryLimitError); ok {
        fmt.Printf("Memory limit exceeded:\n")
        fmt.Printf("  Current: %d bytes\n", memErr.Current)
        fmt.Printf("  Max: %d bytes\n", memErr.Max)
        fmt.Printf("  Needed: %d bytes\n", memErr.Needed)
        
        // Handle backpressure (wait, drop, etc.)
    }
}
```

### Queue Closed Errors

```go
err := q.Enqueue("data") // Blocking enqueue
if err != nil {
    if qcErr, ok := err.(*queue.QueueClosedError); ok {
        fmt.Printf("Queue closed during: %s\n", qcErr.Operation)
    }
}
```

### Best Practice: Use errors.As

```go
import "errors"

var memErr *queue.MemoryLimitError
if errors.As(err, &memErr) {
    // Handle memory error
}

var qcErr *queue.QueueClosedError
if errors.As(err, &qcErr) {
    // Handle closed queue
}
```

---

## Production Patterns

### Pattern 1: Blocking vs Non-Blocking

```go
// NON-BLOCKING (returns immediately)
err := q.TryEnqueue(data)
if err != nil {
    // Queue is full, handle backpressure
    log.Printf("Queue full, dropping message")
}

data := consumer.TryRead()
if data == nil {
    // No data available
}

// BLOCKING (waits for space/data)
err = q.Enqueue(data) // Waits until space available
data = consumer.Read() // Waits until data available
```

**When to use:**
- **Non-blocking (`Try*`)**: In tests, when you need timeouts, or when dropping data is acceptable
- **Blocking**: In production when waiting is acceptable

### Pattern 2: Context-Aware Operations

```go
import "context"

// With timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := q.EnqueueWithContext(ctx, data)
if err == context.DeadlineExceeded {
    log.Println("Timeout: queue still full after 5 seconds")
}

// With cancellation
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

data, err := consumer.ReadWithContext(ctx)
if err == context.Canceled {
    log.Println("Read cancelled")
}
```

### Pattern 3: Graceful Shutdown

```go
func runWorkers(q *queue.Queue) {
    ctx, cancel := context.WithCancel(context.Background())
    var wg sync.WaitGroup
    
    // Start workers
    for i := 0; i < 5; i++ {
        wg.Add(1)
        go worker(ctx, q, &wg)
    }
    
    // Wait for shutdown signal (e.g., SIGTERM)
    // <-shutdownSignal
    
    // Cancel context to signal workers
    cancel()
    
    // Wait for workers to finish current tasks
    wg.Wait()
    
    // Close queue
    q.Close()
}

func worker(ctx context.Context, q *queue.Queue, wg *sync.WaitGroup) {
    defer wg.Done()
    consumer := q.AddConsumer()
    
    for {
        data, err := consumer.ReadWithContext(ctx)
        if err == context.Canceled {
            return // Graceful shutdown
        }
        if data == nil {
            return // Queue closed
        }
        
        processData(data)
    }
}
```

---

## Monitoring and Observability

### Built-in Statistics

```go
// Queue stats
stats := q.GetQueueStats()
fmt.Printf("Queue: %d items, %.1f%% memory\n", 
    stats.TotalItems, stats.MemoryPercent)

// Consumer stats
for _, cs := range q.GetConsumerStats() {
    fmt.Printf("Consumer %s: read %d, lag %d\n", 
        cs.ID, cs.TotalItemsRead, cs.UnreadItems)
}
```

### Metrics Tracking

```go
import "mpmc-queue/queue"

metrics := queue.NewMetrics(q)

// Track enqueue
start := time.Now()
err := q.TryEnqueue(data)
metrics.RecordEnqueue(time.Since(start), err)

// Track dequeue
start = time.Now()
data := consumer.TryRead()
if data != nil {
    metrics.RecordDequeue(time.Since(start))
}

// Get Prometheus metrics
fmt.Print(metrics.GetPrometheusMetrics())
```

### Expiration Notifications

```go
q := queue.NewQueueWithTTL("short-ttl", 1*time.Second)
defer q.Close()

consumer := q.AddConsumer()

// Monitor expiration notifications
go func() {
    for expiredCount := range consumer.GetNotificationChannel() {
        log.Printf("WARNING: %d items expired before being read", expiredCount)
        // Alert, metric, etc.
    }
}()
```

---

## Common Pitfalls

### ❌ Pitfall 1: Using Blocking Operations in Tests

```go
// DON'T DO THIS IN TESTS
func TestMyQueue(t *testing.T) {
    q := queue.NewQueue("test")
    defer q.Close()
    
    q.Enqueue("data") // Can hang forever if queue is full!
}
```

```go
// DO THIS INSTEAD
func TestMyQueue(t *testing.T) {
    q := queue.NewQueue("test")
    defer q.Close()
    
    err := q.TryEnqueue("data") // Returns immediately
    if err != nil {
        t.Fatalf("Enqueue failed: %v", err)
    }
}
```

### ❌ Pitfall 2: Forgetting to Close

```go
// DON'T FORGET defer q.Close()
func processData() {
    q := queue.NewQueue("temp")
    // ... use queue ...
    // Leaks goroutines and resources!
}
```

```go
// ALWAYS CLOSE
func processData() {
    q := queue.NewQueue("temp")
    defer q.Close() // ✅
    // ... use queue ...
}
```

### ❌ Pitfall 3: Expecting Load Balancing

```go
// This does NOT distribute work!
consumer1 := q.AddConsumer()
consumer2 := q.AddConsumer()

// Both read ALL items, not split between them
```

**Solution:** Use separate queues or filtering:

```go
// Option 1: Separate queues per worker
queues := []*queue.Queue{
    queue.NewQueue("worker-0"),
    queue.NewQueue("worker-1"),
}

// Distribute work
workerID := jobID % 2
queues[workerID].TryEnqueue(job)

// Option 2: Use filtering
consumer.ReadWhere(func(d *queue.QueueData) bool {
    job := d.Payload.(Job)
    return job.WorkerID == myWorkerID
})
```

---

## Next Steps

### 1. Explore Examples

```bash
cd examples
go run basic_usage.go
go run advanced_usage.go
go run patterns/worker_pool.go
go run observability/metrics_example.go
```

### 2. Read Documentation

- [API Reference](API.md) - Complete API documentation
- [Architecture](ARCHITECTURE.md) - Internal design and data structures
- [Usage Guide](USAGE_GUIDE.md) - Comprehensive usage examples
- [Performance Guide](PERFORMANCE.md) - Optimization tips
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions

### 3. Customize Your Queue

```go
// Custom TTL
q := queue.NewQueueWithTTL("fast-expire", 30*time.Second)

// Custom memory limit
q := queue.NewQueueWithConfig("big-queue", queue.QueueConfig{
    TTL:       10 * time.Minute,
    MaxMemory: 10 * 1024 * 1024, // 10MB
})

// Disable expiration
q.DisableExpiration()
```

### 4. Production Checklist

Before deploying to production:

- [ ] Configured appropriate TTL for your use case
- [ ] Set memory limit based on available resources
- [ ] Implemented proper error handling (especially `MemoryLimitError`)
- [ ] Using context-aware operations for cancellation
- [ ] Monitoring queue stats (depth, memory, consumer lag)
- [ ] Set up alerts for high memory usage or consumer lag
- [ ] Tested graceful shutdown behavior
- [ ] Verified race condition testing (`go test -race`)

---

## Quick Reference

```go
// Create
q := queue.NewQueue("name")
defer q.Close()

// Enqueue (non-blocking)
err := q.TryEnqueue(data)

// Enqueue (blocking)
err = q.Enqueue(data)

// Enqueue with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
err = q.EnqueueWithContext(ctx, data)

// Create consumer
consumer := q.AddConsumer()

// Read (non-blocking)
data := consumer.TryRead()

// Read (blocking)
data = consumer.Read()

// Read with filtering
data = consumer.TryReadWhere(func(d *queue.QueueData) bool {
    return d.Payload.(string) == "important"
})

// Batch operations
q.TryEnqueueBatch([]any{"a", "b", "c"})
batch := consumer.TryReadBatch(10)

// Stats
stats := q.GetQueueStats()
consumerStats := q.GetConsumerStats()

// Monitoring
metrics := queue.NewMetrics(q)
snapshot := metrics.GetSnapshot()
```

---

## Help and Support

- **Documentation**: Check `docs/` directory
- **Examples**: See `examples/` directory  
- **Issues**: Report bugs or ask questions
- **Contributing**: See [CONTRIBUTING.md](../CONTRIBUTING.md)

Happy queueing! 🚀
