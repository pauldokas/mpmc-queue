# Multi-Producer Multi-Consumer Queue (MPMC-Queue)

A high-performance, thread-safe, memory-constrained multi-producer multi-consumer queue implementation in Go with advanced features including time-based expiration, independent consumer tracking, and comprehensive statistics.

## Features

- **Thread Safe**: Full concurrency support for multiple producers and consumers
- **Memory Constrained**: Strict 1MB memory limit with real-time usage tracking
- **Time-based Expiration**: Automatic cleanup of expired items (10-minute default TTL)
- **Independent Consumers**: Each consumer reads all data at their own pace
- **Chunked Storage**: Efficient storage using doubly-linked list of 1000-item chunks
- **Event Tracking**: Complete audit trail of enqueue/dequeue operations
- **Consumer Notifications**: Alerts when data expires before being read
- **Batch Operations**: Support for batch enqueue and dequeue operations
- **Rich Statistics**: Comprehensive metrics for queue and consumer performance

## Architecture

### Core Components

1. **QueueData**: Individual items with UUID, payload, event history, and timestamps
2. **ChunkedList**: Doubly-linked list using `container/list` with 1000-item chunks
3. **Queue**: Main thread-safe queue with memory management and expiration
4. **Consumer**: Independent position tracking with notification support
5. **MemoryTracker**: Accurate memory estimation and limit enforcement

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
    // Create a new queue
    q := queue.NewQueue("my-queue")
    defer q.Close()
    
    // Enqueue data
    err := q.Enqueue("Hello World")
    if err != nil {
        panic(err)
    }
    
    // Create consumer and read data
    consumer := q.AddConsumer()
    data := consumer.Read()
    if data != nil {
        fmt.Printf("Received: %s\\n", data.Payload)
    }
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
// Batch enqueue
items := []interface{}{"item1", "item2", "item3"}
err := q.EnqueueBatch(items)

// Batch read
consumer := q.AddConsumer()
batch := consumer.ReadBatch(10) // Read up to 10 items
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
- `NewQueue(name string) *Queue` - Create queue with default TTL
- `NewQueueWithTTL(name, ttl) *Queue` - Create queue with custom TTL

#### Data Operations
- `Enqueue(payload interface{}) error` - Add single item
- `EnqueueBatch(payloads []interface{}) error` - Add multiple items

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

#### Reading
- `Read() *QueueData` - Read next item
- `ReadBatch(limit int) []*QueueData` - Read multiple items
- `HasMoreData() bool` - Check if more data available

#### Information
- `GetID() string` - Get consumer UUID
- `GetStats() ConsumerStats` - Get consumer statistics
- `GetUnreadCount() int64` - Count unread items
- `GetNotificationChannel() <-chan int` - Get expiration notification channel

### Data Structures

#### QueueData
```go
type QueueData struct {
    ID      string        // Unique UUID
    Payload interface{}   // User data
    Events  []QueueEvent  // Event history
    Created time.Time     // Creation timestamp
}
```

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
- **Enqueue**: O(1) amortized
- **Read**: O(1) per item
- **Expiration**: O(k) where k is expired items
- **Memory Check**: O(1)

## Testing

Run all tests:
```bash
go test ./tests -v
```

Run specific test categories:
```bash
# Basic functionality
go test ./tests -run "TestNewQueue|TestEnqueue|TestMultiple" -v

# Expiration tests
go test ./tests -run "TestExpiration" -v

# Performance benchmarks
go test ./tests -bench=. -v
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

### Consumer Guidelines
- Process expiration notifications appropriately
- Use batch reads for high-throughput scenarios
- Monitor consumer lag via statistics

### Memory Management
- Monitor memory usage percentage
- Consider TTL settings based on data lifecycle
- Test with realistic payload sizes

### Concurrency
- Each consumer processes independently
- New consumers read from the beginning
- Producers are load-balanced automatically

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
- Lock-free operations where possible

## Limitations

- 1MB total memory limit (not configurable)
- TTL granularity limited to check interval (30s)
- Memory estimation is approximate
- No persistence across restarts

## License

MIT License - see LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## Changelog

### v1.0.0
- Initial release with all core features
- Thread-safe multi-producer multi-consumer support
- Memory management and TTL expiration
- Comprehensive test suite
- Full documentation and examples