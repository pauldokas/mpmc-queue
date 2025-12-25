# Performance Guide

Optimization strategies and performance characteristics for mpmc-queue.

## Benchmark Results

### Hardware
- Go 1.25.1
- Results from actual benchmarks in `tests/benchmark_test.go`

### Single-Threaded Performance

```
BenchmarkEnqueue-8              1000000    ~1200 ns/op
BenchmarkEnqueueBatch-8          100000   ~12000 ns/op (100 items)
BenchmarkRead-8                 2000000     ~600 ns/op
```

### Multi-Threaded Performance

```
10 Producers + 10 Consumers:
- Throughput: ~500,000 ops/sec
- No data corruption
- No race conditions
```

---

## Optimization Strategies

### 1. Batch Operations

**Impact:** 10-20x faster than individual operations

```go
// Slow: ~1200 ns/op × 100 = 120,000 ns
for i := 0; i < 100; i++ {
    q.Enqueue(items[i])
}

// Fast: ~12,000 ns for 100 items
q.EnqueueBatch(items)
```

**Why?**
- Single lock acquisition
- Single memory validation
- Better CPU cache utilization

---

### 2. Pre-allocate Batches

```go
// Fast
items := make([]any, 100)
for i := range items {
    items[i] = generateData(i)
}
q.EnqueueBatch(items)

// Slower
var items []any
for i := 0; i < 100; i++ {
    items = append(items, generateData(i)) // Multiple allocations
}
q.EnqueueBatch(items)
```

---

### 3. Reuse Consumers

```go
// Efficient: Reuse consumer
consumer := q.AddConsumer()
defer q.RemoveConsumer(consumer.GetID())

for _, batch := range batches {
    data := consumer.ReadBatch(100)
    processBatch(data)
}

// Inefficient: Create/destroy consumers
for _, batch := range batches {
    consumer := q.AddConsumer()
    data := consumer.ReadBatch(100)
    processBatch(data)
    q.RemoveConsumer(consumer.GetID()) // Overhead
}
```

---

### 4. Appropriate Batch Sizes

```go
// Optimal batch sizes based on use case

// High throughput: Large batches
batchSize := 1000
items := consumer.ReadBatch(batchSize)

// Low latency: Small batches
batchSize := 10
items := consumer.ReadBatch(batchSize)

// Balanced
batchSize := 100
```

**Recommendations:**
- High throughput: 500-1000 items
- Balanced: 50-200 items
- Low latency: 10-50 items

---

### 5. Monitor Memory Usage

```go
// Check before large operations
stats := q.GetQueueStats()
if stats.MemoryPercent > 80 {
    // Wait or force expiration
    q.ForceExpiration()
}

// Then enqueue
q.EnqueueBatch(largeBatch)
```

---

## Performance Bottlenecks

### Lock Contention

**Symptom:** Throughput doesn't scale with goroutines

**Measurement:**
```go
start := time.Now()
q.Enqueue(data)
duration := time.Since(start)
if duration > 1*time.Millisecond {
    log.Printf("High lock contention detected: %v", duration)
}
```

**Solutions:**
1. Use batch operations (reduces lock acquisitions)
2. Reduce consumer count if excessive
3. Separate queues for different priorities

---

### Memory Limit Reached

**Symptom:** Frequent MemoryLimitError

**Solutions:**

**Option 1: Increase TTL**
```go
q.SetTTL(30 * time.Minute) // Keep items longer
```

**Option 2: Force Expiration**
```go
q.ForceExpiration() // Clean up immediately
```

**Option 3: Multiple Queues**
```go
// Split load across queues
q1 := queue.NewQueue("queue-1")
q2 := queue.NewQueue("queue-2")

// Round-robin or hash-based distribution
queueIndex := hash(key) % 2
queues[queueIndex].Enqueue(data)
```

---

### Slow Consumers

**Symptom:** Consumers falling behind producers

**Detection:**
```go
for _, consumer := range q.GetAllConsumers() {
    stats := consumer.GetStats()
    if stats.UnreadItems > threshold {
        log.Printf("Consumer %s is slow: %d unread", 
            stats.ID, stats.UnreadItems)
    }
}
```

**Solutions:**

**Option 1: More Consumer Goroutines**
```go
// Add more workers
for i := 0; i < additionalWorkers; i++ {
    go func() {
        consumer := q.AddConsumer()
        // Process items
    }()
}
```

**Option 2: Batch Processing**
```go
// Process in larger batches
batch := consumer.ReadBatch(500)
processBatchEfficiently(batch)
```

**Option 3: Remove Slow Consumers**
```go
// Remove consumers that haven't read recently
for _, consumer := range q.GetAllConsumers() {
    stats := consumer.GetStats()
    if time.Since(stats.LastReadTime) > 5*time.Minute {
        q.RemoveConsumer(consumer.GetID())
    }
}
```

---

## Memory Optimization

### Payload Size Matters

```go
// Bad: Large payloads
type HugePayload struct {
    Data [100000]byte // 100KB each = ~10 items max
}

// Good: Keep payloads small
type SmallPayload struct {
    ID   string
    Data []byte // Variable size, keep small
}

// Best: Reference large data
type RefPayload struct {
    ID       string
    DataPath string // Point to external storage
}
```

---

### Dequeue History Growth

**Issue:** Each consumer's dequeue history grows unbounded.

**Mitigation:**
```go
consumer := q.AddConsumer()

// Periodically check history size
if len(consumer.GetDequeueHistory()) > 10000 {
    // Remove and recreate consumer
    oldID := consumer.GetID()
    q.RemoveConsumer(oldID)
    
    consumer = q.AddConsumer()
    // New consumer, fresh history
}
```

---

### Chunk Overhead

**Memory per chunk:** ~8KB (1000 * 8-byte pointers)

**Efficiency:**
- High: >500 items per chunk (>95% utilization)
- Medium: 100-500 items per chunk
- Low: <100 items per chunk (<10% utilization)

**Tip:** Keep queue well-fed to maximize chunk utilization.

---

## Concurrency Tuning

### Producer Concurrency

```go
// Too many producers = lock contention
const optimalProducers = 4-8  // For most workloads

// Measure contention
start := time.Now()
var wg sync.WaitGroup
wg.Add(numProducers)

for i := 0; i < numProducers; i++ {
    go func() {
        defer wg.Done()
        for j := 0; j < 1000; j++ {
            q.Enqueue(j)
        }
    }()
}

wg.Wait()
duration := time.Since(start)

throughput := float64(numProducers*1000) / duration.Seconds()
fmt.Printf("Throughput: %.0f ops/sec\n", throughput)
```

**Sweet spot:** 4-8 producers for most workloads.

---

### Consumer Concurrency

**Consumers are independent** - no contention between them.

```go
// Scale consumers based on processing speed
numConsumers := runtime.NumCPU() * 2

for i := 0; i < numConsumers; i++ {
    go func() {
        consumer := q.AddConsumer()
        // Each consumer reads independently
        for consumer.HasMoreData() {
            data := consumer.Read()
            processItem(data)
        }
    }()
}
```

---

## Profiling

### CPU Profiling

```go
import _ "net/http/pprof"

func main() {
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()

    // Run your queue operations
    runQueueWorkload()
}
```

Then:
```bash
go tool pprof http://localhost:6060/debug/pprof/profile
```

---

### Memory Profiling

```bash
go test -memprofile=mem.prof -bench=.
go tool pprof mem.prof
```

**Look for:**
- Allocation hot spots
- Unexpected growth
- GC pressure

---

### Race Detection

```bash
go test -race -v ./...
```

**Always test with race detection before production!**

---

## Scalability Limits

### Theoretical Limits

**Memory Limit:** 1MB total
- ~2,000-10,000 items (depending on payload size)
- ~10-100 chunks

**Consumer Limit:** Practical limit ~100-1000 consumers
- Each consumer: ~200 bytes overhead
- All share same queue data (no duplication)

**Producer Limit:** No hard limit, but contention increases
- Optimal: 4-8 concurrent producers
- Max tested: 20 producers (good performance)

---

### Recommended Limits

| Metric | Recommended | Maximum Tested | Notes |
|--------|-------------|----------------|-------|
| Queue Items | 1,000-5,000 | 10,000 | Depends on payload size |
| Concurrent Producers | 4-8 | 20 | Lock contention beyond this |
| Concurrent Consumers | 10-50 | 200 | Independent, no contention |
| Batch Size | 50-200 | 1,000 | Larger = better throughput |
| Chunk Utilization | >50% | 100% | For efficiency |

---

## Benchmarking Your Workload

### Sample Benchmark

```go
func BenchmarkMyWorkload(b *testing.B) {
    q := queue.NewQueue("bench")
    defer q.Close()

    // Pre-populate
    for i := 0; i < 1000; i++ {
        q.Enqueue(myData(i))
    }

    consumer := q.AddConsumer()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        data := consumer.Read()
        processData(data)
    }
}
```

Run:
```bash
go test -bench=BenchmarkMyWorkload -benchmem
```

---

## See Also

- [API Documentation](./API.md)
- [Architecture Guide](./ARCHITECTURE.md)
- [Usage Guide](./USAGE_GUIDE.md)
- [Troubleshooting](./TROUBLESHOOTING.md)
