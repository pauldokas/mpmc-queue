# Troubleshooting Guide

Common issues and solutions for mpmc-queue.

## Table of Contents

- [Memory Issues](#memory-issues)
- [Performance Issues](#performance-issues)
- [Data Issues](#data-issues)
- [Concurrency Issues](#concurrency-issues)
- [Expiration Issues](#expiration-issues)
- [Debugging Tips](#debugging-tips)

---

## Memory Issues

### Memory Limit Exceeded

**Symptom:**
```go
err := q.Enqueue(data)
// Error: memory limit exceeded: current=1048000, max=1048576, needed=1000
```

**Diagnosis:**
```go
stats := q.GetQueueStats()
fmt.Printf("Memory: %d/%d (%.1f%%)\n", 
    stats.MemoryUsage, queue.MaxQueueMemory, stats.MemoryPercent)
```

**Solutions:**

1. **Force expiration:**
```go
removed := q.ForceExpiration()
fmt.Printf("Removed %d expired items\n", removed)
```

2. **Reduce TTL:**
```go
q.SetTTL(5 * time.Minute) // Expire faster
```

3. **Smaller payloads:**
```go
// Instead of storing entire object
type LargeObject struct {
    Data [100000]byte
}
q.Enqueue(largeObj) // Uses too much memory

// Store reference instead
type Reference struct {
    ID   string
    Path string
}
q.Enqueue(Reference{ID: "123", Path: "/data/obj123"})
```

4. **Multiple queues:**
```go
// Split across queues
queues := []*queue.Queue{
    queue.NewQueue("queue-0"),
    queue.NewQueue("queue-1"),
}

queueIndex := hash(key) % len(queues)
queues[queueIndex].Enqueue(data)
```

---

### Memory Not Released After Expiration

**Symptom:** Memory usage stays high after items expire

**Check:**
```go
before := q.GetMemoryUsage()
removed := q.ForceExpiration()
after := q.GetMemoryUsage()

fmt.Printf("Removed %d items, freed %d bytes\n", removed, before-after)
```

**Diagnosis:**
- If memory didn't decrease: Bug in memory tracking (should be fixed in commit 2c85032)
- If items not removed: Check TTL and creation time

**Solution:**
```go
// Verify expiration is enabled
q.EnableExpiration()

// Check TTL
ttl := q.GetTTL()
fmt.Printf("TTL: %v\n", ttl)

// Force expiration
removed := q.ForceExpiration()
```

---

### Growing Dequeue History

**Symptom:** Consumer memory grows over time

**Check:**
```go
history := consumer.GetDequeueHistory()
fmt.Printf("History size: %d entries\n", len(history))
```

**Solution:**
```go
// Recreate consumer periodically
if len(consumer.GetDequeueHistory()) > 10000 {
    oldID := consumer.GetID()
    position, index := consumer.GetPosition()
    
    // Remove old consumer
    q.RemoveConsumer(oldID)
    
    // Create new consumer at same position
    newConsumer := q.AddConsumer()
    newConsumer.SetPosition(position, index)
}
```

---

## Performance Issues

### Slow Enqueue Operations

**Symptom:** Enqueue takes >1ms consistently

**Diagnosis:**
```go
start := time.Now()
err := q.Enqueue(data)
duration := time.Since(start)

if duration > 1*time.Millisecond {
    fmt.Printf("Slow enqueue: %v\n", duration)
}
```

**Possible Causes:**

1. **Lock contention:**
```go
// Too many concurrent producers
// Solution: Use batch operations
```

2. **Large payload estimation:**
```go
// Deep nested structures slow down reflection
// Solution: Simplify payload structure
```

3. **Chunk allocation:**
```go
// Creating new chunks
// This is expected and amortized
```

---

### Slow Consumer Reads

**Symptom:** Read operations take >1ms

**Check:**
```go
start := time.Now()
data := consumer.Read()
duration := time.Since(start)

if duration > 1*time.Millisecond {
    fmt.Printf("Slow read: %v\n", duration)
}
```

**Causes:**

1. **Many expired items:**
```go
// Consumer skipping many expired items
// Solution: Increase consumer read frequency
```

2. **Lock contention:**
```go
// Rare, but possible during expiration
// Solution: Use batch reads
```

---

### Low Throughput

**Symptom:** Can't achieve expected ops/sec

**Measurement:**
```go
start := time.Now()
count := 0

for time.Since(start) < 10*time.Second {
    if err := q.Enqueue(data); err == nil {
        count++
    }
}

throughput := float64(count) / 10.0
fmt.Printf("Throughput: %.0f ops/sec\n", throughput)
```

**Solutions:**

1. **Use batch operations:**
```go
// Can increase throughput 10-20x
batchSize := 100
q.EnqueueBatch(batch)
```

2. **Reduce producer count:**
```go
// Less contention
optimalProducers := 4
```

3. **Profile and optimize:**
```bash
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof
```

---

## Data Issues

### Missing Items

**Symptom:** Consumer doesn't receive all items

**Diagnosis:**
```go
// Track enqueue count
var enqueued atomic.Int64
enqueued.Add(1) // After each enqueue

// Track dequeue count  
var dequeued atomic.Int64
dequeued.Add(1) // After each read

// Compare
fmt.Printf("Enqueued: %d, Dequeued: %d\n", 
    enqueued.Load(), dequeued.Load())
```

**Possible Causes:**

1. **Items expired:**
```go
// Check consumer's notification channel
for expiredCount := range consumer.GetNotificationChannel() {
    log.Printf("Lost %d items to expiration", expiredCount)
}
```

2. **Consumer started late:**
```go
// Consumer added after items were already enqueued and expired
// Solution: Add consumers before producing
```

3. **Consumer not reading fully:**
```go
// Check if HasMoreData before stopping
if consumer.HasMoreData() {
    fmt.Println("Warning: Consumer stopped with unread data")
}
```

---

### Duplicate Items

**Symptom:** Consumer reads same item multiple times

**Check:**
```go
seen := make(map[string]bool)

for consumer.HasMoreData() {
    data := consumer.Read()
    if data != nil {
        if seen[data.ID] {
            fmt.Printf("Duplicate! ID: %s\n", data.ID)
        }
        seen[data.ID] = true
    }
}
```

**Cause:** This should not happen - indicates a bug.

**Debug:**
```go
// Check consumer history
history := consumer.GetDequeueHistory()
ids := make(map[string]int)
for _, record := range history {
    ids[record.DataID]++
}

for id, count := range ids {
    if count > 1 {
        fmt.Printf("ID %s read %d times\n", id, count)
    }
}
```

---

### Wrong Data Order

**Symptom:** Items received out of order

**Expected Behavior:** Items are FIFO (First-In-First-Out)

**Check:**
```go
type SequenceData struct {
    Seq int
}

// Enqueue sequentially
for i := 0; i < 100; i++ {
    q.Enqueue(SequenceData{Seq: i})
}

// Verify order
consumer := q.AddConsumer()
lastSeq := -1
for consumer.HasMoreData() {
    data := consumer.Read()
    if data != nil {
        seq := data.Payload.(SequenceData).Seq
        if seq <= lastSeq {
            fmt.Printf("Out of order: %d -> %d\n", lastSeq, seq)
        }
        lastSeq = seq
    }
}
```

**If order is wrong:** This indicates a critical bug - report it.

---

## Concurrency Issues

### Race Condition Detected

**Symptom:**
```
WARNING: DATA RACE
Read at 0x00c0001a8000 by goroutine 7:
...
```

**Action:** This should not happen if using latest version.

**Verify version:**
```bash
git log --oneline | head -5
# Should include: "Fix race condition" commits
```

**If race detected:**
1. Update to latest version
2. Run tests with `-race` flag
3. Report issue with reproduction steps

---

### Deadlock

**Symptom:** Program hangs indefinitely

**Diagnosis:**
```bash
# Send SIGQUIT to get stack trace
kill -QUIT <pid>

# Or use pprof
curl http://localhost:6060/debug/pprof/goroutine?debug=2
```

**Possible Causes:**

1. **Forgot to Close():**
```go
// ❌ Wrong
q := queue.NewQueue("test")
// ... program ends, expiration worker still running

// ✅ Correct
q := queue.NewQueue("test")
defer q.Close()
```

2. **Blocking on full channel:**
```go
// Check if notification channel is being read
notifCh := consumer.GetNotificationChannel()
// Must consume from this channel or it may block
```

---

### Goroutine Leak

**Symptom:** Number of goroutines keeps increasing

**Check:**
```go
before := runtime.NumGoroutine()

// Do queue operations
q := queue.NewQueue("test")
// ... use queue
q.Close()

after := runtime.NumGoroutine()

if after > before {
    fmt.Printf("Leaked %d goroutines\n", after-before)
}
```

**Solution:**
```go
// Always call Close()
q.Close() // Stops expiration worker
```

---

## Expiration Issues

### Items Expiring Too Fast

**Symptom:** Items disappear before consumers read them

**Check TTL:**
```go
ttl := q.GetTTL()
fmt.Printf("Current TTL: %v\n", ttl)
```

**Solutions:**

1. **Increase TTL:**
```go
q.SetTTL(30 * time.Minute)
```

2. **Disable expiration temporarily:**
```go
q.DisableExpiration()
// Process critical items
q.EnableExpiration()
```

3. **Faster consumers:**
```go
// Add more consumer workers
for i := 0; i < 10; i++ {
    go consumerWorker(q)
}
```

---

### Items Not Expiring

**Symptom:** Old items remain in queue

**Check expiration status:**
```go
// Expiration should be enabled by default
// But verify:
stats := q.GetQueueStats()
fmt.Printf("Items: %d\n", stats.TotalItems)

// Force expiration
removed := q.ForceExpiration()
fmt.Printf("Removed: %d\n", removed)

if removed == 0 {
    // Check if expiration is disabled
    // Or items haven't reached TTL yet
}
```

**Solutions:**

1. **Enable expiration:**
```go
q.EnableExpiration()
```

2. **Check TTL:**
```go
// Items might not be old enough
ttl := q.GetTTL()
// Wait at least TTL duration before expecting expiration
```

3. **Manual cleanup:**
```go
q.ForceExpiration()
```

---

### Expiration Disabled Not Working

**Symptom:** Items still expire after DisableExpiration()

**Verify:**
```go
q.DisableExpiration()
time.Sleep(2 * q.GetTTL())

// This should NOT remove items
removed := q.ForceExpiration()
if removed > 0 {
    // BUG: Should be fixed in commit 2c85032
    fmt.Printf("Bug: ForceExpiration ignored disabled flag")
}
```

**Solution:** Update to commit 2c85032 or later.

---

## Debugging Tips

### Enable Verbose Logging

```go
func debugEnqueue(q *queue.Queue, payload any) error {
    before := q.GetMemoryUsage()
    
    err := q.Enqueue(payload)
    
    after := q.GetMemoryUsage()
    delta := after - before
    
    if err != nil {
        log.Printf("[ENQUEUE] FAILED: %v, memory: %d bytes", err, before)
    } else {
        log.Printf("[ENQUEUE] OK: added %d bytes, total: %d", delta, after)
    }
    
    return err
}
```

---

### Track Consumer Progress

```go
func monitorConsumer(consumer *queue.Consumer, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for range ticker.C {
        stats := consumer.GetStats()
        
        log.Printf("Consumer %s: read=%d, unread=%d, last_read=%v",
            stats.ID[:8], // First 8 chars of UUID
            stats.TotalItemsRead,
            stats.UnreadItems,
            time.Since(stats.LastReadTime))
        
        // Alert if consumer is stuck
        if time.Since(stats.LastReadTime) > 5*time.Minute {
            log.Printf("WARNING: Consumer %s appears stuck", stats.ID)
        }
    }
}
```

---

### Dump Queue State

```go
func dumpQueueState(q *queue.Queue) {
    stats := q.GetQueueStats()
    
    fmt.Printf("=== Queue State: %s ===\n", stats.Name)
    fmt.Printf("Items: %d\n", stats.TotalItems)
    fmt.Printf("Memory: %d bytes (%.1f%%)\n", stats.MemoryUsage, stats.MemoryPercent)
    fmt.Printf("Consumers: %d\n", stats.ConsumerCount)
    fmt.Printf("TTL: %v\n", stats.TTL)
    fmt.Printf("Created: %v ago\n", time.Since(stats.CreatedAt))
    
    // Consumer details
    fmt.Printf("\n=== Consumers ===\n")
    for i, consumer := range q.GetAllConsumers() {
        cStats := consumer.GetStats()
        fmt.Printf("%d. ID=%s, Read=%d, Unread=%d, LastRead=%v ago\n",
            i+1,
            cStats.ID[:8],
            cStats.TotalItemsRead,
            cStats.UnreadItems,
            time.Since(cStats.LastReadTime))
    }
}
```

---

### Test with Race Detection

```bash
# Always test concurrent code with race detector
go test -race -v ./tests

# Run specific test
go test -race -run TestConcurrentProducers -v

# With timeout for long-running tests
go test -race -timeout 5m -v ./tests
```

---

### Memory Profiling

```bash
# Generate memory profile
go test -memprofile=mem.prof -bench=BenchmarkEnqueue

# Analyze
go tool pprof mem.prof

# In pprof interactive mode:
(pprof) top10        # Top 10 allocations
(pprof) list Enqueue # Show allocations in Enqueue
(pprof) web          # Visual graph (requires graphviz)
```

---

### CPU Profiling

```bash
# Generate CPU profile
go test -cpuprofile=cpu.prof -bench=.

# Analyze
go tool pprof cpu.prof

# In pprof:
(pprof) top10
(pprof) list Read
(pprof) web
```

---

## Common Error Messages

### "memory limit exceeded"

**Full Error:**
```
memory limit exceeded: current=1048000, max=1048576, needed=2000
```

**Meaning:** Not enough space for the operation.

**Solutions:** See [Memory Limit Exceeded](#memory-limit-exceeded)

---

### "Failed to add data to chunk"

**Full Error:**
```
Failed to add data to chunk
```

**Meaning:** Internal error - chunk rejected data despite space check.

**Action:** This should not happen. Report as bug.

---

### panic: "runtime error: invalid memory address"

**Likely Cause:** Accessing nil consumer or queue.

**Fix:**
```go
// ✅ Always check nil
if consumer == nil {
    return
}
data := consumer.Read()

// ✅ Check returned consumer
consumer := q.GetConsumer(id)
if consumer == nil {
    log.Printf("Consumer %s not found", id)
    return
}
```

---

## Testing Issues

### Tests Hang

**Cause:** Usually waiting for consumers that never finish.

**Fix:**
```go
// ✅ Use timeouts in tests
func TestWithTimeout(t *testing.T) {
    done := make(chan struct{})
    
    go func() {
        // Test code
        close(done)
    }()
    
    select {
    case <-done:
        // Test completed
    case <-time.After(10 * time.Second):
        t.Fatal("Test timeout")
    }
}

// ✅ Or use testing timeout flag
// go test -timeout 30s
```

---

### Flaky Tests

**Symptom:** Tests pass sometimes, fail others

**Diagnosis:**
```bash
# Run test 100 times
for i in {1..100}; do
    go test -run TestMyTest || break
done
```

**Common Causes:**

1. **Race conditions:**
```bash
# Test with race detector
go test -race -run TestMyTest
```

2. **Timing assumptions:**
```go
// ❌ Bad
time.Sleep(100 * time.Millisecond)
// Assume operation completed

// ✅ Good
for i := 0; i < 10; i++ {
    if operationComplete() {
        break
    }
    time.Sleep(50 * time.Millisecond)
}
```

3. **Shared state:**
```go
// ✅ Create fresh queue for each test
func TestSomething(t *testing.T) {
    q := queue.NewQueue("test-" + t.Name())
    defer q.Close()
    // ... test code
}
```

---

## Getting Help

### Collect Debug Information

```go
func collectDebugInfo(q *queue.Queue) string {
    var buf bytes.Buffer
    
    // Queue info
    stats := q.GetQueueStats()
    fmt.Fprintf(&buf, "Queue: %s\n", stats.Name)
    fmt.Fprintf(&buf, "Items: %d\n", stats.TotalItems)
    fmt.Fprintf(&buf, "Memory: %d/%d (%.1f%%)\n", 
        stats.MemoryUsage, queue.MaxQueueMemory, stats.MemoryPercent)
    fmt.Fprintf(&buf, "Consumers: %d\n", stats.ConsumerCount)
    fmt.Fprintf(&buf, "TTL: %v\n", stats.TTL)
    
    // Consumer details
    for _, consumer := range q.GetAllConsumers() {
        cStats := consumer.GetStats()
        fmt.Fprintf(&buf, "  Consumer %s: read=%d, unread=%d\n",
            cStats.ID[:8], cStats.TotalItemsRead, cStats.UnreadItems)
    }
    
    // Go runtime info
    fmt.Fprintf(&buf, "Goroutines: %d\n", runtime.NumGoroutine())
    
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    fmt.Fprintf(&buf, "Go Memory: %d MB\n", m.Alloc/1024/1024)
    
    return buf.String()
}
```

---

### Report Issues

When reporting issues, include:

1. **Go version:** `go version`
2. **Commit hash:** `git log -1 --oneline`
3. **Minimal reproduction:** Simplest code that shows the issue
4. **Debug info:** Output from collectDebugInfo()
5. **Race detector output:** If applicable
6. **Expected vs actual behavior**

---

## See Also

- [API Documentation](./API.md)
- [Architecture Guide](./ARCHITECTURE.md)
- [Usage Guide](./USAGE_GUIDE.md)
- [Performance Guide](./PERFORMANCE.md)
