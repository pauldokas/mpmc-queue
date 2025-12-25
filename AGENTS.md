# Agent Development Guide for mpmc-queue

This guide provides essential information for AI coding agents working on the mpmc-queue project - a high-performance, thread-safe multi-producer multi-consumer queue implementation in Go.

## Project Overview

Multi-producer, multi-consumer queue with:
- Thread-safe concurrent operations
- 1MB memory limit with real-time tracking
- Time-based expiration (10-minute default TTL)
- Independent consumer position tracking
- Chunked storage (1000 items per chunk using doubly-linked lists)
- Comprehensive event tracking and statistics

## Build, Test, and Lint Commands

### Running Tests

```bash
# Run all tests
go test ./tests -v

# Run all tests with race detection
go test ./tests -v -race

# Run a single specific test
go test ./tests -v -run TestEnqueueDequeue

# Run tests matching a pattern
go test ./tests -v -run "TestMultiple.*"

# Run tests in a specific file (use pattern matching)
go test ./tests -v -run TestExpiration  # Tests in expiration_test.go

# Run benchmarks
go test ./tests -bench=. -v

# Run benchmarks with memory stats
go test ./tests -bench=. -benchmem -v

# Run specific benchmark
go test ./tests -bench=BenchmarkEnqueue -v
```

### Building

```bash
# Build all packages
go build ./...

# Build with race detection
go build -race ./...

# Verify module dependencies
go mod verify

# Tidy dependencies
go mod tidy
```

### Linting and Formatting

```bash
# Format all Go files
go fmt ./...

# Run go vet
go vet ./...

# Check for race conditions (run with tests)
go test ./tests -race
```

### Running Examples

```bash
# Run basic usage example
go run examples/basic_usage.go

# Run advanced usage example
go run examples/advanced_usage.go
```

## Code Style Guidelines

### Package Structure

```
queue/          # Main package with all queue implementation
├── queue.go          # Main Queue type and operations
├── consumer.go       # Consumer and ConsumerManager types
├── data.go          # QueueData, QueueEvent, ChunkNode types
├── chunked_list.go  # ChunkedList implementation
└── memory.go        # MemoryTracker and memory estimation

tests/          # Test package
├── queue_test.go
├── expiration_test.go
└── benchmark_test.go

examples/       # Example programs (package main)
```

### Import Ordering

Follow standard Go convention:
1. Standard library imports
2. External dependencies
3. Internal packages

```go
import (
    "container/list"
    "context"
    "fmt"
    "sync"
    "time"
    
    "github.com/google/uuid"
    
    "mpmc-queue/queue"
)
```

### Naming Conventions

- **Types**: PascalCase (e.g., `Queue`, `Consumer`, `QueueData`)
- **Exported functions**: PascalCase (e.g., `NewQueue`, `GetStats`)
- **Private functions**: camelCase (e.g., `expirationWorker`, `cleanupExpiredItems`)
- **Constants**: PascalCase (e.g., `DefaultTTL`, `MaxQueueMemory`)
- **Interfaces**: `-er` suffix when appropriate (not heavily used in this codebase)

### Constants

Define package constants at the top of relevant files:

```go
const (
    // DefaultTTL is the default time-to-live for queue items (10 minutes)
    DefaultTTL = 10 * time.Minute
    
    // ExpirationCheckInterval is how often to check for expired items
    ExpirationCheckInterval = 30 * time.Second
)
```

### Types and Structs

- Use struct tags for JSON serialization: `json:"field_name"`
- Add godoc comments for all exported types
- Group related fields logically

```go
// QueueData represents a single item in the queue
type QueueData struct {
    ID      string        `json:"id"`      // UUID
    Payload interface{}   `json:"payload"` // Arbitrary data
    Events  []QueueEvent  `json:"events"`  // History of queue events
    Created time.Time     `json:"created"` // For expiration tracking
}
```

### Function Documentation

Every exported function must have a godoc comment:

```go
// Enqueue adds data to the queue
func (q *Queue) Enqueue(payload interface{}) error {
    // implementation
}
```

### Error Handling

1. **Custom Error Types**: Define specific error types for different failure modes

```go
// MemoryLimitError represents a memory limit exceeded error
type MemoryLimitError struct {
    Current int64
    Max     int64
    Needed  int64
}

func (e *MemoryLimitError) Error() string {
    return fmt.Sprintf("memory limit exceeded: current=%d, max=%d, needed=%d", 
        e.Current, e.Max, e.Needed)
}
```

2. **Error Checking**: Always check and propagate errors appropriately

```go
err := q.Enqueue(payload)
if err != nil {
    return fmt.Errorf("failed to enqueue: %w", err)
}
```

3. **Type Assertions**: Use type assertion for custom error types

```go
if memErr, ok := err.(*queue.MemoryLimitError); ok {
    // Handle memory limit error specifically
}
```

### Concurrency Patterns

1. **Mutex Usage**: Use `sync.RWMutex` for read-heavy operations

```go
// Read operations
q.mutex.RLock()
defer q.mutex.RUnlock()

// Write operations
q.mutex.Lock()
defer q.mutex.Unlock()
```

2. **Lock Ordering**: Always use consistent lock ordering to prevent deadlocks
   - Queue lock first, then consumer lock
   - Never acquire consumer lock while holding queue lock if the consumer might need queue lock

3. **WaitGroups**: Use for background goroutines

```go
queue.wg.Add(1)
go queue.expirationWorker()

// In Close()
close(q.stopChan)
q.wg.Wait()
```

4. **Channels**: Use buffered channels for notifications to avoid blocking

```go
notificationCh: make(chan int, 100), // Buffered channel
```

### Memory Management

1. Track all allocations through `MemoryTracker`
2. Use `EstimateQueueDataSize()` before adding data
3. Update memory usage on both addition and removal
4. Conservative estimation preferred over underestimation

```go
if !q.memoryTracker.CanAddData(data) {
    return &MemoryLimitError{...}
}
```

### Testing Practices

1. **Test Names**: Use descriptive names starting with `Test`

```go
func TestEnqueueDequeue(t *testing.T) { ... }
func TestMultipleConsumers(t *testing.T) { ... }
func TestConcurrentProducers(t *testing.T) { ... }
```

2. **Cleanup**: Always defer `Close()` in tests

```go
q := queue.NewQueue("test-queue")
defer q.Close()
```

3. **Error Messages**: Use informative error messages with context

```go
if data.Payload != testData {
    t.Errorf("Expected payload '%v', got '%v'", testData, data.Payload)
}
```

4. **Concurrent Tests**: Use `sync.WaitGroup` and proper synchronization

```go
var wg sync.WaitGroup
wg.Add(numProducers)
// ... spawn goroutines
wg.Wait()
```

## Architecture Patterns

### Component Interaction

```
Queue (thread-safe coordinator)
├── ChunkedList (storage)
│   ├── container/list (doubly-linked list)
│   └── ChunkNode arrays (1000 items each)
├── ConsumerManager (consumer tracking)
│   └── Consumer instances (independent positions)
└── MemoryTracker (memory accounting)
```

### Key Design Principles

1. **Immutability**: QueueData created once, events appended
2. **Independence**: Each consumer tracks its own position
3. **Lock Granularity**: Separate locks for queue and consumers
4. **Expiration**: Background worker with consumer notifications
5. **Memory Safety**: Pre-validation before allocation

## Common Development Tasks

### Adding a New Queue Method

1. Add method to `Queue` type in `queue/queue.go`
2. Use appropriate locking (RLock for reads, Lock for writes)
3. Add godoc comment
4. Update tests in `tests/queue_test.go`

### Adding Consumer Functionality

1. Add method to `Consumer` type in `queue/consumer.go`
2. Lock consumer state appropriately
3. Consider interaction with queue state
4. Test with multiple concurrent consumers

### Modifying Memory Tracking

1. Update estimation logic in `queue/memory.go`
2. Ensure both addition and removal update tracking
3. Test memory limit enforcement
4. Validate with large payloads

## Performance Considerations

- **Chunk Size**: 1000 items per chunk balances memory and traversal
- **Lock Contention**: RWMutex favors read-heavy workloads
- **Memory Estimation**: Reflection-based, not real allocation size
- **Expiration**: Background checks every 30 seconds
- **Channel Buffering**: Notification channels buffered to prevent blocking

## Module Information

- **Module**: `mpmc-queue`
- **Go Version**: 1.25.1
- **Dependencies**: `github.com/google/uuid v1.6.0`

## Additional Resources

- See `README.md` for API documentation and usage examples
- See `PROJECT_PLAN.md` for architecture details and implementation phases
- Check `examples/` directory for complete working code samples
