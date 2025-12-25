# Multi-Producer Multi-Consumer Queue - Project Plan

## Overview
This project implements a thread-safe, memory-constrained, time-aware multi-producer multi-consumer queue in Go with advanced consumer management and notification features.

## Architecture

### Core Components

1. **QueueData Structure**
   - UUID for unique identification
   - Generic payload (interface{})
   - Event history with timestamps and queue names
   - Creation timestamp for expiration tracking

2. **ChunkedList**
   - Doubly linked list using `container/list`
   - Each node contains an array of 1000 QueueData entries
   - Efficient memory usage and traversal
   - Supports both append and prepend operations

3. **Queue**
   - Thread-safe operations using sync.RWMutex
   - Named queues with unique identifiers
   - Memory usage tracking (1MB limit)
   - Consumer management and independent processing
   - Background expiration management

4. **Consumer**
   - Independent reading positions
   - Notification channel for expired data
   - Thread-safe state management

## Data Structures

### QueueData
```go
type QueueEvent struct {
    Timestamp time.Time
    QueueName string
    EventType string // "enqueue" or "dequeue"
}

type QueueData struct {
    ID       string        // UUID
    Payload  interface{}   // Arbitrary data
    Events   []QueueEvent  // History of queue events
    Created  time.Time     // For expiration tracking
}
```

### ChunkedList Node
```go
type ChunkNode struct {
    Data [1000]*QueueData
    Size int // Current number of items in this chunk
}
```

### Queue
```go
type Queue struct {
    name         string
    chunks       *list.List        // Doubly linked list of ChunkNode
    consumers    map[string]*Consumer
    mutex        sync.RWMutex
    memoryUsage  int64
    maxMemory    int64             // 1MB limit
    stopChan     chan struct{}
    wg           sync.WaitGroup
}
```

### Consumer
```go
type Consumer struct {
    id              string
    chunkElement    *list.Element  // Current chunk position
    indexInChunk    int           // Position within current chunk
    notificationCh  chan int      // Notification of expired items
    mutex          sync.Mutex
}
```

## Implementation Plan

### Phase 1: Core Data Structures
- [ ] Implement `QueueData` with UUID generation
- [ ] Implement `ChunkNode` structure
- [ ] Implement `ChunkedList` wrapper around `container/list`
- [ ] Basic memory size estimation functions

### Phase 2: Queue Infrastructure
- [ ] Implement `Queue` struct with thread safety
- [ ] Basic enqueue/dequeue operations
- [ ] Memory usage tracking and enforcement
- [ ] Queue naming and identification

### Phase 3: Consumer Management
- [ ] Implement `Consumer` struct
- [ ] Independent consumer position tracking
- [ ] Consumer registration/deregistration
- [ ] New consumer initialization (reads all existing data)

### Phase 4: Advanced Features
- [ ] Background expiration goroutine (10-minute TTL)
- [ ] Consumer notification system for expired data
- [ ] Exact count tracking for unread expired items
- [ ] Event history management (enqueue/dequeue timestamps)

### Phase 5: Testing and Optimization
- [ ] Unit tests for all components
- [ ] Thread safety tests
- [ ] Memory limit enforcement tests
- [ ] Performance benchmarks
- [ ] Stress tests for high-throughput scenarios

## File Structure

```
/
├── go.mod
├── go.sum
├── PROJECT_PLAN.md
├── README.md
├── queue/
│   ├── queue.go           # Main queue implementation
│   ├── consumer.go        # Consumer management
│   ├── data.go           # QueueData and related structures
│   ├── chunked_list.go   # ChunkedList implementation
│   └── memory.go         # Memory tracking utilities
├── examples/
│   ├── basic_usage.go    # Simple producer/consumer example
│   ├── multiple_consumers.go # Multiple consumer example
│   └── stress_test.go    # High-load scenario
└── tests/
    ├── queue_test.go     # Core functionality tests
    ├── consumer_test.go  # Consumer behavior tests
    ├── memory_test.go    # Memory management tests
    ├── expiration_test.go # Time-based expiration tests
    └── benchmark_test.go # Performance benchmarks
```

## Key Features Implementation Details

### Thread Safety
- Use `sync.RWMutex` for queue operations
- Separate mutexes for consumer state
- Lock-free where possible using atomic operations

### Memory Management
- Track memory usage using `unsafe.Sizeof()` and reflection
- Implement memory estimation for arbitrary payloads
- Enforce 1MB limit with graceful handling

### Time-based Expiration
- Background goroutine checks for expired data every 30 seconds
- Efficient cleanup using chunk-level timestamps
- Consumer notification with exact unread counts

### Consumer Independence
- Each consumer maintains its own position
- No blocking between consumers
- Support for dynamic consumer addition/removal

### Order Preservation
- FIFO ordering maintained through chunked list structure
- Consistent ordering across all consumers
- Event history tracking for audit trail

## Performance Considerations

### Memory Efficiency
- Chunked storage reduces memory fragmentation
- Lazy deletion for expired items
- Efficient position tracking for consumers

### Concurrency
- Reader-writer locks for optimal read performance
- Minimal critical sections
- Lock-free consumer position updates where possible

### Scalability
- Support for many concurrent producers/consumers
- Efficient cleanup of expired data
- Memory-bounded operation

## Testing Strategy

### Unit Tests
- Individual component testing
- Edge case handling
- Error condition testing

### Integration Tests
- End-to-end producer/consumer scenarios
- Multiple queue interaction
- Consumer notification accuracy

### Performance Tests
- High-throughput benchmarks
- Memory usage under load
- Concurrent access performance

### Stress Tests
- Long-running stability tests
- Memory limit enforcement
- Large number of consumers

## Success Criteria

1. **Functionality**: All specified features working correctly
2. **Thread Safety**: No race conditions under concurrent access
3. **Memory Management**: Strict 1MB limit enforcement
4. **Performance**: Handles high-throughput scenarios efficiently
5. **Reliability**: Stable operation over extended periods
6. **Testing**: Comprehensive test coverage (>90%)

## Timeline Estimate

- **Phase 1-2**: Core implementation (2-3 days)
- **Phase 3**: Consumer management (1-2 days)  
- **Phase 4**: Advanced features (2-3 days)
- **Phase 5**: Testing and optimization (2-3 days)

**Total**: 7-11 days for complete implementation and testing.