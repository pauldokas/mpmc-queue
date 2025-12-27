# Multi-Producer Multi-Consumer Queue (MPMC-Queue)

A high-performance, thread-safe, memory-constrained multi-producer multi-consumer queue implementation in Go with advanced features including time-based expiration, independent consumer tracking, and comprehensive statistics.

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-blue)](https://golang.org/dl/)
[![Thread Safe](https://img.shields.io/badge/Thread-Safe-green)](docs/ARCHITECTURE.md)
[![Production Ready](https://img.shields.io/badge/Status-Production%20Ready-success)](TODO.md)

**Production Ready:** All critical bugs fixed and verified with comprehensive race detection and stress testing.

## Features

- ✅ **Thread Safe**: Full concurrency support for multiple producers and consumers (all race conditions fixed)
- ✅ **Memory Constrained**: Configurable memory limit (default 1MB) with real-time usage tracking
- ✅ **Time-based Expiration**: Automatic cleanup of expired items (10-minute default TTL)
- ✅ **Independent Consumers**: Each consumer reads all data at their own pace
- ✅ **Chunked Storage**: Efficient storage using doubly-linked list of 1000-item chunks
- ✅ **Immutable Data**: QueueData is immutable after creation (thread-safe without locks)
- ✅ **Consumer Notifications**: Alerts when data expires before being read
- ✅ **Batch Operations**: Atomic batch enqueue and efficient batch read operations
- ✅ **Rich Statistics**: Comprehensive metrics for queue and consumer performance
- ✅ **Position Tracking**: Accurate consumer position even during expiration
- ✅ **Atomic Operations**: Lock-free stats and RCU-based consumer management

## Architecture

### Core Components

1. **QueueData**: Immutable items with UUID, payload, enqueue event, and timestamps
2. **ChunkedList**: Doubly-linked list using `container/list` with 1000-item chunks
3. **Queue**: Main thread-safe coordinator with RWMutex synchronization
4. **Consumer**: Independent position tracking with local dequeue history
5. **MemoryTracker**: Accurate memory estimation with proper leak prevention
6. **ChunkNode**: Fixed-size arrays with atomic size tracking for thread-safety

### Data Flow

```
Producers → Queue (ChunkedList) → Consumers
     ↓           ↓                    ↓
  Memory     Expiration          Notifications
  Tracking    Worker             & Statistics
```

## Installation

```bash
go get mpmc-queue
```

## Basic Usage

```go
package main

import (
    "fmt"
    "mpmc-queue/queue"
)

func main() {
    // Create a new queue with default 10-minute TTL
    q := queue.NewQueue("my-queue")
    defer q.Close()
    
    // Enqueue data (blocks if queue is full)
    err := q.Enqueue("Hello World")
    if err != nil {
        panic(err)
    }
    
    // Create consumer and read data (blocks if no data available)
    consumer := q.AddConsumer()
    data := consumer.Read()
    if data != nil {
        fmt.Printf("Received: %s\n", data.Payload)
    }
}
```

## Non-Blocking Operations

For non-blocking operations, use the `Try*` methods:

```go
// Non-blocking enqueue - returns error immediately if queue is full
err := q.TryEnqueue("data")
if err != nil {
    // Handle full queue
}

// Non-blocking read - returns nil immediately if no data available
data := consumer.TryRead()
if data == nil {
    // No data available
}
```

## Advanced Usage

### Custom TTL

```go
// Create queue with 30-second TTL
q := queue.NewQueueWithTTL("short-ttl-queue", 30*time.Second)
defer q.Close()

// Change TTL at runtime
q.SetTTL(1*time.Minute)
```

### Multiple Consumers

```go
q := queue.NewQueue("multi-consumer-queue")
defer q.Close()

// Add data
q.Enqueue("Message 1")
q.Enqueue("Message 2")

// Create multiple consumers
consumer1 := q.AddConsumer()
consumer2 := q.AddConsumer()

// Both consumers will independently read all messages
data1 := consumer1.Read() // Gets "Message 1"
data2 := consumer2.Read() // Also gets "Message 1"
```

### Batch Operations
 
 ```go
 // Batch enqueue (blocking - waits until all items can be added)
 items := []any{"item1", "item2", "item3"}
 err := q.EnqueueBatch(items)
 
 // Non-blocking batch enqueue
 err := q.TryEnqueueBatch(items)
 
 // Context-aware batch enqueue
 ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
 defer cancel()
 err := q.EnqueueBatchWithContext(ctx, items)
 
 // Batch read (blocking - waits for at least 1 item, returns up to 10)
 consumer := q.AddConsumer()
 batch := consumer.ReadBatch(10)


// Non-blocking batch read
batch := consumer.TryReadBatch(10) // Returns immediately with available items

// Context-aware batch read (cancels if context is done)
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
batch, err := consumer.ReadBatchWithContext(ctx, 10)
```

### Expiration Notifications

```go
q := queue.NewQueueWithTTL("notify-queue", 1*time.Second)
defer q.Close()

consumer := q.AddConsumer()
q.Enqueue("Will expire")

// Wait for expiration
time.Sleep(2*time.Second)

// Check for notifications
select {
case expiredCount := <-consumer.GetNotificationChannel():
    fmt.Printf("Consumer missed %d expired items\\n", expiredCount)
default:
    fmt.Println("No expired items")
}
```

### Filtering/Selection

```go
q := queue.NewQueue("filter-queue")
defer q.Close()

// Enqueue mixed data
for i := 1; i <= 100; i++ {
    q.TryEnqueue(i)
}

consumer := q.AddConsumer()

// Filter for even numbers only (non-blocking)
data := consumer.TryReadWhere(func(d *queue.QueueData) bool {
    num, ok := d.Payload.(int)
    return ok && num%2 == 0
})
// Returns first even number (2), or nil if no match

// Filter with blocking (waits for matching item)
data = consumer.ReadWhere(func(d *queue.QueueData) bool {
    num, ok := d.Payload.(int)
    return ok && num > 50
})
// Blocks until an item > 50 is available

// Filter with context support
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
data, err := consumer.ReadWhereWithContext(ctx, func(d *queue.QueueData) bool {
    str, ok := d.Payload.(string)
    return ok && len(str) > 10
})
// Returns matching item, or error on timeout/cancellation

// Note: Filtering advances the consumer position
// Non-matching items are consumed as the filter searches
```

### Statistics and Monitoring

```go
// Queue statistics
stats := q.GetQueueStats()
fmt.Printf("Items: %d, Memory: %d bytes (%.1f%%), Consumers: %d\\n",
    stats.TotalItems, stats.MemoryUsage, stats.MemoryPercent, stats.ConsumerCount)

// Consumer statistics
consumerStats := q.GetConsumerStats()
for _, cs := range consumerStats {
    fmt.Printf("Consumer %s: read %d, unread %d\\n",
        cs.ID, cs.TotalItemsRead, cs.UnreadItems)
}
```

## API Reference

### Queue Methods

#### Creation
- `NewQueue(name string) *Queue` - Create queue with default settings
- `NewQueueWithTTL(name, ttl) *Queue` - Create queue with custom TTL
- `NewQueueWithConfig(name, config) *Queue` - Create queue with custom config (TTL, MaxMemory)

#### Data Operations (Blocking)
- `Enqueue(payload any) error` - Add single item (blocks if queue full)
- `EnqueueBatch(payloads []any) error` - Add multiple items atomically (blocks if queue full)
- `EnqueueWithContext(ctx context.Context, payload any) error` - Add single item with context support
- `EnqueueBatchWithContext(ctx context.Context, payloads []any) error` - Add multiple items with context support

#### Data Operations (Non-Blocking)
- `TryEnqueue(payload any) error` - Add single item (returns error immediately if full)
- `TryEnqueueBatch(payloads []any) error` - Add multiple items (returns error immediately if full)

#### Consumer Management
- `AddConsumer() *Consumer` - Create new consumer
- `RemoveConsumer(consumerID string) bool` - Remove consumer
- `GetConsumer(consumerID string) *Consumer` - Get consumer by ID

#### Configuration
- `SetTTL(ttl time.Duration)` - Change TTL
- `GetTTL() time.Duration` - Get current TTL
- `EnableExpiration()` - Enable automatic expiration
- `DisableExpiration()` - Disable automatic expiration

#### Statistics
- `GetQueueStats() QueueStats` - Get queue metrics
- `GetConsumerStats() []ConsumerStats` - Get all consumer metrics
- `IsEmpty() bool` - Check if queue is empty
- `GetMemoryUsage() int64` - Get current memory usage

#### Management
- `Close()` - Clean shutdown
- `ForceExpiration() int` - Manually trigger expiration

### Consumer Methods

#### Reading (Blocking)
- `Read() *QueueData` - Read next item (blocks if no data available)
- `ReadBatch(limit int) []*QueueData` - Read multiple items (blocks until at least 1 available)
- `ReadWithContext(ctx context.Context) (*QueueData, error)` - Read next item with context support
- `ReadBatchWithContext(ctx context.Context, limit int) ([]*QueueData, error)` - Read multiple items with context support

#### Reading (Non-Blocking)
- `TryRead() *QueueData` - Read next item (returns nil immediately if no data)
- `TryReadBatch(limit int) []*QueueData` - Read multiple items (returns immediately with available items)
- `HasMoreData() bool` - Check if more data available

#### Filtering
- `TryReadWhere(predicate func(*QueueData) bool) *QueueData` - Read next item matching predicate (non-blocking)
- `ReadWhere(predicate func(*QueueData) bool) *QueueData` - Read next item matching predicate (blocking)
- `ReadWhereWithContext(ctx context.Context, predicate func(*QueueData) bool) (*QueueData, error)` - Read with predicate and context support

#### Information
- `GetID() string` - Get consumer UUID
- `GetStats() ConsumerStats` - Get consumer statistics
- `GetUnreadCount() int64` - Count unread items
- `GetDequeueHistory() []DequeueRecord` - Get consumer's read history
- `GetNotificationChannel() <-chan int` - Get expiration notification channel
- `GetPosition() (*list.Element, int)` - Get current position in queue

### Data Structures

#### QueueData
```go
type QueueData struct {
    ID           string     // Unique UUID
    Payload      any        // User data (immutable)
    EnqueueEvent QueueEvent // Single enqueue event
    Created      time.Time  // Creation timestamp
}
```

**Note:** QueueData is immutable after creation for thread-safety.

#### QueueStats
```go
type QueueStats struct {
    Name          string        // Queue name
    TotalItems    int64         // Current item count
    MemoryUsage   int64         // Memory usage in bytes
    MemoryPercent float64       // Memory usage percentage
    ConsumerCount int           // Active consumer count
    CreatedAt     time.Time     // Queue creation time
    TTL           time.Duration // Current TTL setting
}
```

#### ConsumerStats
```go
type ConsumerStats struct {
    ID             string    // Consumer UUID
    TotalItemsRead int64     // Items read by this consumer
    UnreadItems    int64     // Items available to read
    LastReadTime   time.Time // Last read timestamp
}
```

## Performance Characteristics

### Memory Management
- **Limit**: Hard 1MB limit with graceful error handling
- **Tracking**: Real-time memory usage estimation
- **Efficiency**: Chunked storage reduces fragmentation

### Concurrency
- **Thread Safety**: Full concurrent access support
- **Lock Granularity**: Optimized read/write locks
- **Scalability**: Efficient with many producers/consumers

### Time Complexity
- **Enqueue/TryEnqueue**: O(1) amortized
- **Read/TryRead**: O(1) per item
- **Expiration**: O(k) where k is expired items
- **Memory Check**: O(1)
- **Blocking Operations**: Waits on channel notifications (efficient)

## Testing

### Run All Tests
```bash
go test ./tests -v
```

### Run with Race Detection
```bash
# Critical: Always test with race detection
go test ./tests -race -v

# Quick race check on key tests
go test ./tests -race -run "TestConcurrent|TestMultiple" -v
```

### Run Specific Test Categories
```bash
# Basic functionality
go test ./tests -run "TestEnqueue|TestMultiple" -v

# Blocking behavior tests
go test ./tests -run "TestBlocking|TestTry" -v

# Expiration tests
go test ./tests -run "TestExpiration" -v

# Race condition tests
go test ./tests -race -run "TestRace|TestConcurrent" -v

# Stress tests
go test ./tests -run "TestExtreme" -v -timeout 5m

# Performance benchmarks
go test ./tests -bench=. -benchmem -v
```

### Test Coverage
```bash
# Generate coverage report
go test ./tests -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# View coverage percentage
go tool cover -func=coverage.out | grep total
```

## Examples

See the `examples/` directory for complete working examples:

- `basic_usage.go` - Simple producer/consumer patterns
- `advanced_usage.go` - TTL, expiration, and concurrent access

Run examples:
```bash
cd examples
go run basic_usage.go
go run advanced_usage.go
```

## Error Handling

### Memory Limit Error
```go
_, err := q.Enqueue(largeData)
if memErr, ok := err.(*queue.MemoryLimitError); ok {
    fmt.Printf("Memory limit exceeded: current=%d, max=%d, needed=%d\\n",
        memErr.Current, memErr.Max, memErr.Needed)
}
```

### General Queue Errors
```go
_, err := q.EnqueueBatch(data)
if qErr, ok := err.(*queue.QueueError); ok {
    fmt.Printf("Queue error: %s\\n", qErr.Message)
}
```

## Best Practices

### Producer Guidelines
- Use batch operations for multiple items
- Handle memory limit errors gracefully
- Consider payload size impact on memory usage
- Use `TryEnqueue` when timeouts or non-blocking behavior is needed
- Use blocking `Enqueue` when waiting for space is acceptable

### Consumer Guidelines
- Process expiration notifications appropriately
- Use batch reads for high-throughput scenarios
- Monitor consumer lag via statistics
- Use `TryRead` in tests to avoid blocking
- Use blocking `Read` in production when waiting for data is acceptable
- Use filtering methods (`ReadWhere`, `TryReadWhere`) to selectively process items
  - Note: Filtering advances consumer position, consuming non-matching items

### Memory Management
- Monitor memory usage percentage
- Consider TTL settings based on data lifecycle
- Test with realistic payload sizes
- Configure TTL at queue creation based on your use case

### Concurrency
- Each consumer processes independently
- New consumers read from the beginning
- Producers are load-balanced automatically
- Blocking operations use efficient channel notifications

## Implementation Details

### Chunked Storage
- 1000 items per chunk using arrays
- Doubly-linked list of chunks
- Efficient traversal and cleanup

### Memory Estimation
- Recursive reflection-based size calculation
- Accounts for strings, slices, maps, and structs
- Conservative estimation approach

### Expiration System
- Background goroutine checks every 30 seconds
- Efficient cleanup from oldest items first
- Consumer notifications with exact counts

### Thread Safety
- RWMutex for queue operations
- Individual mutexes for consumer state
- Lock-free operations where possible (stats, consumer list iteration)
- Atomic counters for high-performance metrics

## Limitations

### Memory Management
- **Limit**: The queue enforces a memory limit (default 1MB, configurable) based on *estimated* usage.
- **Estimation**: Memory usage is calculated using reflection and is approximate. Type sizes are cached for performance.
- **Behavior**: When full, `Enqueue` blocks indefinitely and `TryEnqueue` returns an error immediately.

### Time-To-Live (TTL)
- **Granularity**: Expiration is checked by a background worker every 30 seconds (`ExpirationCheckInterval`).
- **Precision**: An item may remain in the queue for up to 30 seconds *after* its TTL has expired before being removed.

### Immutability
- **QueueData**: Once created, `QueueData` and its payload are immutable. This ensures thread safety without complex locking but requires creating new items for updates.

### Resource Growth
- **Event History**: Each `Consumer` maintains a local `dequeueHistory` slice that tracks every item read. This slice grows unbounded over the lifetime of the consumer and is not automatically pruned. Long-lived consumers reading millions of items will consume significant memory.
- **Persistence**: The queue is in-memory only. All data is lost upon process restart.

See [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) for solutions and workarounds.

## License

MIT License - see LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## Documentation

- **[API Reference](docs/API.md)** - Complete API documentation with examples
- **[Architecture Guide](docs/ARCHITECTURE.md)** - Internal design and implementation details
- **[Usage Guide](docs/USAGE_GUIDE.md)** - Practical patterns and real-world examples
- **[Performance Guide](docs/PERFORMANCE.md)** - Optimization strategies and benchmarks
- **[Troubleshooting](docs/TROUBLESHOOTING.md)** - Common issues and solutions
- **[Agent Development](AGENTS.md)** - Guide for AI coding agents
- **[Project Plan](PROJECT_PLAN.md)** - Original design and architecture
- **[TODO](TODO.md)** - Completed work and future enhancements

## Quality Assurance

### Bug Fixes
- ✅ **13 critical bugs fixed** - All race conditions, memory leaks, and data corruption issues resolved
- ✅ **5 race conditions eliminated** - Verified with `-race` flag
- ✅ **3 expiration bugs fixed** - Memory leak, position corruption, API violations
- ✅ **2 idempotency bugs fixed** - Queue.Close() and Consumer.Close() now safe to call multiple times
- ✅ **Comprehensive testing** - 114 tests across 14 test files with race detection and stress tests

### Test Suite (114 Tests)
- **Unit Tests**: Core functionality and edge cases (40+ tests)
- **Consumer Management Tests**: RemoveConsumer, GetConsumer, lifecycle (9 tests)
- **Lifecycle Tests**: Close, CloseWithContext, cleanup (12 tests)
- **Batch Operation Tests**: Atomicity, edge cases, cross-chunk (14 tests)
- **Error Handling Tests**: MemoryLimitError, payload types (14 tests)
- **Consumer Info Tests**: GetUnreadCount, position tracking (14 tests)
- **Blocking Tests**: Edge cases, interruptions, multiple waiters (14 tests)
- **Race Tests**: Concurrent access verification (100-200 goroutines)
- **Stress Tests**: Extreme load testing (20 producers + 20 consumers)
- **Expiration Tests**: TTL and cleanup verification (10+ tests)

### Verification
```bash
# All tests pass
go test ./tests -v

# No race conditions
go test ./tests -race -v

# Stress tested
go test ./tests -run TestExtreme -v
```

## Changelog

### v1.0.0 (Current - Production Ready)
- ✅ All critical bugs fixed (15 total)
- ✅ Race-condition free (verified with extensive testing)
- ✅ Lock ordering violations fixed (snapshot pattern implementation)
- ✅ TOCTOU issues resolved (proper lock holding during reads)
- ✅ Test race conditions fixed (atomic operations for shared counters)
- ✅ Memory leak fixed (proper cleanup on expiration)
- ✅ Immutable QueueData (thread-safe without locks)
- ✅ Atomic ChunkNode.Size (lock-free access)
- ✅ Position tracking fixed (accurate during expiration)
- ✅ Idempotent Close operations (Queue and Consumer)
- ✅ Blocking/non-blocking operations (Enqueue, Read, batch operations)
- ✅ Configurable TTL at queue creation
- ✅ Modern Go idioms (interface{} → any)
- ✅ Comprehensive documentation (5 docs files, continuously updated)
- ✅ Extensive test coverage (114 tests across 14 test files, 3500+ lines)

See [TODO.md](TODO.md) for detailed bug fix history and commit references.