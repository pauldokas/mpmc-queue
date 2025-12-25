# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

This is a high-performance Multi-Producer Multi-Consumer (MPMC) queue implementation in Go with advanced features including:
- Thread-safe concurrent access for multiple producers and consumers  
- Memory-constrained operation (1MB hard limit)
- Time-based item expiration with configurable TTL
- Independent consumer position tracking
- Event history and comprehensive statistics
- Chunked storage using doubly-linked lists for efficiency

## Common Commands

### Building and Testing
```bash
# Build the module
go build ./...

# Run all tests
go test ./tests -v

# Run specific test categories
go test ./tests -run "TestNewQueue|TestEnqueue|TestMultiple" -v
go test ./tests -run "TestExpiration" -v

# Run performance benchmarks
go test ./tests -bench=. -v

# Run a single test by name
go test ./tests -run TestConcurrentProducers -v

# Test with race detection
go test -race ./tests -v
```

### Examples and Development
```bash
# Run basic usage example
cd examples && go run basic_usage.go

# Run advanced usage example  
cd examples && go run advanced_usage.go

# Check module dependencies
go mod tidy
go mod download
```

### Code Quality
```bash
# Format code
go fmt ./...

# Vet code for common issues
go vet ./...

# Run staticcheck if available
staticcheck ./...
```

## Architecture Overview

### Core Components

**Queue (queue/queue.go)**: Main thread-safe queue with memory management, TTL expiration, and consumer coordination. Uses background goroutines for automatic cleanup.

**Consumer (queue/consumer.go)**: Independent position tracking for each consumer. New consumers read from the beginning of the queue. Includes notification system for expired items.

**ChunkedList (queue/chunked_list.go)**: Efficient storage using doubly-linked list of chunks. Each chunk holds up to 1000 items as arrays, providing O(1) amortized enqueue and good cache locality.

**MemoryTracker (queue/memory.go)**: Real-time memory usage estimation using reflection. Enforces strict 1MB limit with graceful error handling.

**QueueData (queue/data.go)**: Individual items with UUID, payload, event history, and timestamps. Supports arbitrary payload types.

### Data Flow Architecture

```
Producers → Queue → ChunkedList → Consumers
     ↓         ↓         ↓            ↓
   Batch    Memory   Expiration   Notifications
 Operations Tracking   Worker     & Statistics
```

### Key Design Patterns

**Memory Management**: Reflection-based size estimation with recursive traversal for complex types. Conservative approach with safety margins.

**Concurrency Model**: RWMutex for queue operations, separate mutexes for consumer state. Background expiration worker with configurable intervals.

**Consumer Independence**: Each consumer maintains position independently. Notification system alerts consumers when their unread data expires.

**Error Handling**: Custom error types (`MemoryLimitError`, `QueueError`) with detailed information for debugging.

## Implementation Details

### Memory Estimation Logic
The `MemoryTracker` uses reflection to recursively calculate memory usage:
- Base struct sizes using `unsafe.Sizeof()`  
- String lengths, slice/array traversal
- Map key-value pairs, struct fields
- Pointer dereferencing with nil checks

### Expiration System
- Background worker checks every 30 seconds (`ExpirationCheckInterval`)
- Items ordered by creation time for efficient cleanup
- Consumer position adjustment after expiration
- Exact notification counts per consumer

### Threading Model
- Queue operations use `sync.RWMutex` for concurrent reads
- Each consumer has independent `sync.Mutex` for position
- Background goroutines for expiration cleanup
- Graceful shutdown with context support

## Testing Strategy

### Test Categories
- **Basic functionality**: `TestNewQueue`, `TestEnqueueDequeue`
- **Concurrency**: `TestConcurrentProducers`, `TestConcurrentConsumers` 
- **Memory limits**: Memory tracking and enforcement tests
- **Expiration**: TTL behavior and notification accuracy
- **Consumer behavior**: Independent reading, position tracking

### Key Test Patterns
```go
// Always defer Close() in tests
q := queue.NewQueue("test-queue")
defer q.Close()

// Use goroutines with WaitGroup for concurrency tests
var wg sync.WaitGroup
wg.Add(numWorkers)
// ... start goroutines
wg.Wait()
```

## Development Guidelines

### Adding New Features
1. Consider memory impact - all additions must respect 1MB limit
2. Maintain thread safety - use appropriate locking patterns
3. Update both `QueueStats` and `ConsumerStats` if adding metrics
4. Add corresponding tests in appropriate `*_test.go` file

### Memory Considerations  
- Large payloads will quickly exhaust the 1MB limit
- Memory estimation is conservative but not exact
- Test with realistic payload sizes
- Consider chunking large data externally

### Consumer Patterns
- New consumers always start from queue beginning
- Each consumer processes independently at its own pace  
- Use notification channels for expiration awareness
- Remove consumers when done to free resources

### Error Handling
- Check for `MemoryLimitError` when memory is constrained
- Handle `QueueError` for general operational issues
- Use context-based shutdown for graceful cleanup

## API Usage Patterns

### Basic Operations
```go
q := queue.NewQueue("my-queue")
defer q.Close()

// Single operations
q.Enqueue("data")
consumer := q.AddConsumer()
data := consumer.Read()

// Batch operations  
q.EnqueueBatch([]interface{}{"a", "b", "c"})
batch := consumer.ReadBatch(10)
```

### Advanced Configuration
```go
// Custom TTL
q := queue.NewQueueWithTTL("queue", 5*time.Minute)

// Dynamic TTL changes
q.SetTTL(30*time.Second)

// Manual expiration
expired := q.ForceExpiration()

// Disable automatic expiration
q.DisableExpiration()
```

### Monitoring and Statistics
```go
// Queue metrics
stats := q.GetQueueStats()
fmt.Printf("Items: %d, Memory: %.1f%%", stats.TotalItems, stats.MemoryPercent)

// Consumer metrics
for _, cs := range q.GetConsumerStats() {
    fmt.Printf("Consumer %s: read %d, unread %d", cs.ID, cs.TotalItemsRead, cs.UnreadItems)
}
```

## File Organization

- `queue/` - Core implementation package
- `examples/` - Working usage examples  
- `tests/` - Comprehensive test suite
- `go.mod` - Module definition with minimal dependencies (only google/uuid)

The codebase follows Go conventions with clear separation of concerns and comprehensive error handling throughout.