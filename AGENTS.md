# Agent Development Guide for mpmc-queue

Essential guide for AI coding agents working on mpmc-queue - a high-performance, thread-safe multi-producer multi-consumer queue in Go with 1MB memory limit, time-based expiration, independent consumer tracking, and blocking/non-blocking operations.

## Quick Reference

**Module**: `mpmc-queue` | **Go**: 1.25.1 | **Dependencies**: `github.com/google/uuid v1.6.0`

**Key Files**: `queue/queue.go` (main ops) | `queue/consumer.go` (consumers) | `queue/data.go` (data types) | `queue/chunked_list.go` (storage) | `queue/memory.go` (tracking)

**Critical Rules**: ALWAYS acquire queue lock before consumer lock | ALWAYS run tests with `-race` flag | NEVER modify QueueData after creation | Use Try* methods in tests to avoid blocking

## Commands

### Testing
```bash
go test ./tests -v                           # All tests (114 tests)
go test ./tests -v -race                     # Race detection (ALWAYS USE)
go test ./tests -v -run TestEnqueueDequeue  # Single test
go test ./tests -v -run "TestMultiple.*"    # Pattern match
go test ./tests -bench=. -benchmem -v       # Benchmarks with memory
go test ./tests -v -timeout 5m               # Increase timeout for stress tests
go test ./tests -race -run TestExtreme -v    # Stress test with race detection

# Test specific categories
go test ./tests -v -run "TestConsumer.*"     # Consumer management tests
go test ./tests -v -run "TestClose.*"        # Lifecycle and close tests
go test ./tests -v -run "TestBlocking.*"     # Blocking behavior tests
go test ./tests -v -run "Test.*Batch.*"      # Batch operation tests
go test ./tests -v -run "TestMemory.*"       # Memory and error tests
```

### Building & Linting
```bash
go build ./...        # Build all
go build -race ./...  # Build with race detection
go fmt ./...          # Format code
go vet ./...          # Static analysis
go mod tidy           # Clean dependencies
```

### Coverage
```bash
go test ./tests -coverprofile=coverage.out           # Generate coverage
go tool cover -html=coverage.out -o coverage.html   # HTML report
go tool cover -func=coverage.out                     # Function coverage
go tool cover -func=coverage.out | grep total       # Total coverage %
```

### Examples
```bash
go run examples/basic_usage.go
go run examples/advanced_usage.go
```

## Code Style

### Import Order
1. Standard library (alphabetical)
2. External dependencies
3. Internal packages (`mpmc-queue/queue`)

```go
import (
    "container/list"
    "sync"
    "time"
    
    "github.com/google/uuid"
    
    "mpmc-queue/queue"
)
```

### Naming
- **Types/Exported**: `PascalCase` (Queue, Consumer, QueueData)
- **Private functions**: `camelCase` (expirationWorker, cleanupExpiredItems)  
- **Constants**: `PascalCase` (DefaultTTL, MaxQueueMemory)

### Documentation
ALL exported types/functions MUST have godoc comments:
```go
// Enqueue adds data to the queue and tracks memory usage
func (q *Queue) Enqueue(payload any) error
```

### Error Handling
Use custom error types for different failures:
```go
type MemoryLimitError struct {
    Current int64
    Max     int64
    Needed  int64
}

// Always wrap errors with context
return fmt.Errorf("failed to enqueue: %w", err)

// Type assert for specific handling
if memErr, ok := err.(*queue.MemoryLimitError); ok {
    // Handle memory-specific error
}
```

### Struct Tags
Always use JSON tags: `json:"field_name"`

## Concurrency Rules (CRITICAL)

### Mutex Usage
```go
// Read operations - use RLock
q.mutex.RLock()
defer q.mutex.RUnlock()

// Write operations - use Lock
q.mutex.Lock()
defer q.mutex.Unlock()
```

### Lock Ordering (PREVENT DEADLOCKS)
1. **ALWAYS** acquire queue lock before consumer lock
2. **NEVER** hold consumer lock while acquiring queue lock
3. Use `getPositionUnsafe()` when caller already holds locks
4. Use **snapshot pattern** to avoid lock ordering violations

**Snapshot Pattern** (Recommended for most consumer methods):
```go
// ✅ CORRECT: Snapshot consumer state, then acquire queue lock
c.mutex.Lock()
chunkElement := c.chunkElement
indexInChunk := c.indexInChunk
c.mutex.Unlock()

// Now safe to acquire queue lock
c.queue.mutex.RLock()
// ... use chunkElement and indexInChunk
c.queue.mutex.RUnlock()

// ❌ WRONG: Consumer lock held while acquiring queue lock
c.mutex.Lock()
defer c.mutex.Unlock()
c.queue.mutex.RLock() // DEADLOCK POSSIBLE!
```

**Nested Locking** (Only when queue lock acquired first):
```go
// ✅ CORRECT: Queue lock first, then consumer lock
q.mutex.Lock()
consumer.mutex.Lock()
// ... do work
consumer.mutex.Unlock()
q.mutex.Unlock()

// ❌ WRONG: Consumer lock first creates deadlock risk
consumer.mutex.Lock()
q.mutex.Lock() // DEADLOCK POSSIBLE!
```

**Methods Using Snapshot Pattern**:
- `Consumer.Read()` - Snapshots position, releases lock before queue access
- `Consumer.HasMoreData()` - Snapshots position before checking queue
- `Consumer.GetUnreadCount()` - Snapshots position before counting
- `Consumer.GetStats()` - Snapshots all fields before queue operations

### Background Workers
```go
// Start
q.wg.Add(1)
go q.expirationWorker()

// Shutdown (in Close())
close(q.stopChan)
q.wg.Wait()
```

### Channels
Use buffered channels to prevent blocking: `make(chan int, 100)`

## Blocking vs Non-Blocking Operations

### Blocking API (Default)
**Enqueue/Read block** when queue is full/empty respectively:
```go
// Blocks until space available
err := q.Enqueue(data)           

// Blocks until data available
data := consumer.Read()          

// Blocks until at least 1 item available
batch := consumer.ReadBatch(10)  
```

**When to use**: Production code where waiting is acceptable

### Non-Blocking API (Try* methods)
**Try* methods return immediately**:
```go
// Returns error immediately if full
err := q.TryEnqueue(data)              

// Returns nil immediately if empty
data := consumer.TryRead()             

// Returns partial/empty batch immediately
batch := consumer.TryReadBatch(10)     
```

**When to use**: 
- Tests (to avoid hanging tests)
- Non-critical operations
- When timeout handling is needed
- Performance-critical tight loops

### Testing Guidelines
**ALWAYS use Try* methods in tests** unless specifically testing blocking behavior:
```go
// ✅ GOOD: Won't hang test
err := q.TryEnqueue(payload)
data := consumer.TryRead()

// ❌ BAD: May hang test if queue full/empty
err := q.Enqueue(payload)
data := consumer.Read()
```

## Memory Management

1. **Pre-validate** before adding: `MemoryTracker.CanAddData()`
2. **Track additions**: Automatically tracked in `ChunkedList.Enqueue()`
3. **Track removals**: Update on expiration and cleanup
4. **Conservative estimates**: Prefer over-estimation to prevent overruns

```go
if !q.memoryTracker.CanAddData(data) {
    return &MemoryLimitError{...}
}
```

## Testing Standards

### Test Structure
```go
func TestFeatureName(t *testing.T) {
    q := queue.NewQueue("test-queue")
    defer q.Close()  // ALWAYS cleanup
    
    // Test logic with descriptive errors
    if got != want {
        t.Errorf("Expected %v, got %v", want, got)
    }
}
```

### Concurrent Tests
```go
var wg sync.WaitGroup
wg.Add(numGoroutines)
for i := 0; i < numGoroutines; i++ {
    go func(id int) {
        defer wg.Done()
        // Test logic
    }(i)
}
wg.Wait()
```

### Race Detection
**ALWAYS** run tests with `-race` flag before committing changes involving:
- Lock modifications
- Shared state access
- Consumer position updates
- Expiration logic

### Atomic Operations in Tests
When testing concurrent code with shared counters, use atomic operations:
```go
var counter int64
// In goroutine: atomic.AddInt64(&counter, 1)
// To read: atomic.LoadInt64(&counter)
```
This prevents race conditions in the test code itself.

## Architecture Essentials

### Component Hierarchy
```
Queue (RWMutex-protected coordinator)
├── ChunkedList (container/list with 1000-item chunks)
├── ConsumerManager (tracks all consumers)  
│   └── Consumer (independent position, own mutex)
└── MemoryTracker (1MB limit enforcement)
```

### Design Principles
1. **Immutability**: QueueData never modified after creation
2. **Independence**: Each consumer reads all data at own pace
3. **Lock Granularity**: Separate mutexes for queue and consumers
4. **Snapshot Pattern**: Read state without holding locks to avoid deadlocks
5. **Background Expiration**: 30-second intervals, configurable TTL (default 10 minutes)
6. **Pre-validation**: Check limits before allocating
7. **TOCTOU Prevention**: Hold locks during critical read operations
8. **Blocking Operations**: Enqueue/Read block by default; Try* methods for non-blocking

### Key Constants
```go
DefaultTTL = 10 * time.Minute          // Item expiration
ExpirationCheckInterval = 30 * time.Second
MaxQueueMemory = 1024 * 1024           // 1MB limit
ChunkSize = 1000                        // Items per chunk
```

## Common Tasks

### Adding Queue Method
1. Add to `queue/queue.go`
2. Use `RLock` for reads, `Lock` for writes
3. Add godoc comment
4. Write test in `tests/queue_test.go`
5. Run `go test ./tests -v -race`

### Adding Consumer Method  
1. Add to `queue/consumer.go`
2. Lock consumer state with `c.mutex`
3. Consider queue interaction (may need `q.mutex.RLock()`)
4. Test with multiple concurrent consumers
5. Verify no deadlocks with `-race`

### Modifying Memory Logic
1. Update `queue/memory.go`
2. Ensure additions AND removals tracked
3. Test limit enforcement
4. Validate with large payloads (>100KB)

## Common Pitfalls

❌ **DON'T**: Acquire consumer lock then queue lock (deadlock)  
✅ **DO**: Use snapshot pattern or acquire queue lock first

❌ **DON'T**: Hold consumer lock while acquiring queue lock  
✅ **DO**: Release consumer lock before acquiring queue lock

❌ **DON'T**: Release queue lock before reading chunk data  
✅ **DO**: Keep queue lock held during `chunk.Get()` to prevent TOCTOU issues

❌ **DON'T**: Modify QueueData after creation  
✅ **DO**: Create immutable QueueData with NewQueueData()

❌ **DON'T**: Forget to defer Close() in tests  
✅ **DO**: Always `defer q.Close()` immediately after creation

❌ **DON'T**: Skip race detection on concurrent code  
✅ **DO**: Run `go test ./tests -v -race` on ALL changes

❌ **DON'T**: Use unprotected shared variables in tests  
✅ **DO**: Use `atomic.AddInt64()` and `atomic.LoadInt64()` for shared counters

## Resources

- `README.md` - API docs and usage examples
- `ARCHITECTURE.md` - Detailed design and component interaction
- `PROJECT_PLAN.md` - Implementation phases and decisions
- `RACE_CONDITION_FIX_STRATEGY.md` - Concurrency solutions
- `examples/` - Working code samples
