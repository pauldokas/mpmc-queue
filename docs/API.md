# mpmc-queue API Documentation

Complete API reference for the mpmc-queue package.

## Table of Contents

- [Queue API](#queue-api)
- [Consumer API](#consumer-api)
- [Data Types](#data-types)
- [Error Types](#error-types)
- [Constants](#constants)

---

## Queue API

### Creating Queues

#### `NewQueue(name string) *Queue`

Creates a new queue with default settings.

**Parameters:**
- `name` (string): Unique identifier for the queue

**Returns:**
- `*Queue`: Initialized queue instance

**Example:**
```go
q := queue.NewQueue("my-queue")
defer q.Close()
```

**Default Settings:**
- TTL: 10 minutes
- Max Memory: 1MB
- Expiration: Enabled
- Chunk Size: 1000 items

---

#### `NewQueueWithTTL(name string, ttl time.Duration) *Queue`
 
 Creates a queue with custom TTL (time-to-live).
 
 **Parameters:**
 - `name` (string): Queue identifier
 - `ttl` (time.Duration): Time-to-live for items
 
 **Returns:**
 - `*Queue`: Initialized queue instance
 
 **Example:**
 ```go
 q := queue.NewQueueWithTTL("short-lived", 5*time.Minute)
 defer q.Close()
 ```
 
 ---
 
 #### `NewQueueWithConfig(name string, config QueueConfig) *Queue`
 
 Creates a queue with full configuration control.
 
 **Parameters:**
 - `name` (string): Queue identifier
 - `config` (QueueConfig): Configuration options
 
 **Returns:**
 - `*Queue`: Initialized queue instance
 
 **QueueConfig:**
 ```go
 type QueueConfig struct {
     TTL       time.Duration // Time-to-live for items
     MaxMemory int64         // Maximum memory in bytes
 }
 ```
 
 **Example:**
 ```go
 config := queue.QueueConfig{
     TTL:       30 * time.Minute,
     MaxMemory: 10 * 1024 * 1024, // 10MB
 }
 q := queue.NewQueueWithConfig("custom-queue", config)
 defer q.Close()
 ```


---

### Enqueue Operations (Blocking)

#### `Enqueue(payload any) error`

Adds a single item to the queue, **blocking** if the queue is full until space becomes available.

**Parameters:**
- `payload` (any): Data to enqueue (any type)

**Returns:**
- `error`: nil on success, error if queue is closed

**Behavior:**
- **Blocks** when queue is full (memory limit reached)
- Waits for space to become available via expiration
- Wakes up when items are expired and removed
- Returns error if queue is closed while waiting

**Thread-Safety:** Safe for concurrent use by multiple producers

**Example:**
```go
// Will wait until space is available
err := q.Enqueue("hello world")
if err != nil {
    // Queue was closed while waiting
    log.Printf("Enqueue failed: %v", err)
}
```

**Time Complexity:** O(1) amortized (excluding wait time)

---

#### `EnqueueBatch(payloads []any) error`

Adds multiple items atomically, **blocking** if the queue is full.

**Parameters:**
- `payloads` ([]any): Slice of items to enqueue

**Returns:**
- `error`: nil on success, error if queue is closed

**Atomicity:** All items are validated before any are added. Either all succeed or waits until all can be added.

**Behavior:**
- **Blocks** when queue cannot fit all items
- Waits for sufficient space via expiration
- All-or-nothing operation

**Example:**
```go
items := []any{"item1", "item2", "item3"}
err := q.EnqueueBatch(items)
if err != nil {
    log.Printf("Batch enqueue failed: %v", err)
}
```

---

### Enqueue Operations (Context-Aware)

#### `EnqueueWithContext(ctx context.Context, payload any) error`

Adds a single item to the queue, blocking until space is available or context is cancelled.

**Parameters:**
- `ctx` (context.Context): Context for cancellation/timeout
- `payload` (any): Data to enqueue

**Returns:**
- `error`: nil on success, context error or queue closed error

**Example:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := q.EnqueueWithContext(ctx, "data")
if err != nil {
    // Handle timeout or cancellation
}
```

---

#### `EnqueueBatchWithContext(ctx context.Context, payloads []any) error`

Adds multiple items atomically, blocking until space is available or context is cancelled.

**Parameters:**
- `ctx` (context.Context): Context for cancellation/timeout
- `payloads` ([]any): Slice of items to enqueue

**Returns:**
- `error`: nil on success, context error or queue closed error

---

### Enqueue Operations (Non-Blocking)

#### `TryEnqueue(payload any) error`

Attempts to add a single item without blocking.

**Parameters:**
- `payload` (any): Data to enqueue

**Returns:**
- `error`: nil on success, MemoryLimitError if queue is full

**Behavior:**
- Returns **immediately** if queue is full
- Does not wait for space to become available

**Example:**
```go
err := q.TryEnqueue("hello world")
if err != nil {
    if _, ok := err.(*queue.MemoryLimitError); ok {
        // Queue is full, handle accordingly
        log.Printf("Queue full, dropping item")
    }
}
```

**Use Cases:**
- Tests (to avoid blocking)
- Non-critical data
- When implementing custom timeout logic

---

#### `TryEnqueueBatch(payloads []any) error`

Attempts to add multiple items without blocking.

**Parameters:**
- `payloads` ([]any): Slice of items to enqueue

**Returns:**
- `error`: nil on success, MemoryLimitError if batch won't fit

**Atomicity:** Either all items are added or none are added.

**Example:**
```go
items := []any{"item1", "item2", "item3"}
err := q.TryEnqueueBatch(items)
if err != nil {
    // Batch couldn't fit, try smaller batch or retry later
}
```

**Time Complexity:** O(n) where n is batch size

---

### Consumer Management

#### `AddConsumer() *Consumer`

Creates a new consumer starting at the beginning of the queue.

**Returns:**
- `*Consumer`: New consumer instance with unique ID

**Thread-Safety:** Safe for concurrent use

**Example:**
```go
consumer1 := q.AddConsumer()
consumer2 := q.AddConsumer() // Independent consumer
```

**Note:** Each consumer maintains independent read position.

---

#### `AddConsumerGroup(name string) *ConsumerGroup`

Creates or retrieves a consumer group for load-balanced consumption.

**Parameters:**
- `name` (string): Group name

**Returns:**
- `*ConsumerGroup`: The consumer group instance

**Example:**
```go
group := q.AddConsumerGroup("worker-pool")
```

---

#### `RemoveConsumer(consumerID string) bool`

Removes a consumer and cleans up resources.

**Parameters:**
- `consumerID` (string): Consumer's unique identifier

**Returns:**
- `bool`: true if removed, false if not found

**Example:**
```go
if q.RemoveConsumer(consumer.GetID()) {
    // Consumer removed successfully
}
```

---

#### `GetConsumer(consumerID string) *Consumer`

Retrieves a consumer by ID.

**Parameters:**
- `consumerID` (string): Consumer identifier

**Returns:**
- `*Consumer`: Consumer instance or nil if not found

---

#### `GetAllConsumers() []*Consumer`

Returns all active consumers.

**Returns:**
- `[]*Consumer`: Slice of all consumer instances

**Thread-Safety:** Returns a snapshot; safe for concurrent use

---

### Queue Information

#### `GetName() string`

Returns the queue's name.

**Returns:**
- `string`: Queue name

---

#### `IsEmpty() bool`

Checks if queue has any items.

**Returns:**
- `bool`: true if queue is empty

**Thread-Safety:** Atomic check

---

#### `GetMemoryUsage() int64`

Returns current memory usage in bytes.

**Returns:**
- `int64`: Memory usage in bytes

**Example:**
```go
usage := q.GetMemoryUsage()
fmt.Printf("Using %d bytes (%.1f%%)\n", usage, float64(usage)/10485.76)
```

---

#### `GetQueueStats() QueueStats`

Returns comprehensive queue statistics.

**Returns:**
- `QueueStats`: Statistics structure

**QueueStats Fields:**
```go
type QueueStats struct {
    Name          string        // Queue name
    TotalItems    int64         // Total items in queue
    MemoryUsage   int64         // Memory usage in bytes
    MemoryPercent float64       // Memory usage percentage
    ConsumerCount int           // Number of active consumers
    CreatedAt     time.Time     // Queue creation time
    TTL           time.Duration // Time-to-live setting
}
```

**Example:**
```go
stats := q.GetQueueStats()
fmt.Printf("%s\n", stats.String())
```

---

#### `GetConsumerStats() []ConsumerStats`

Returns statistics for all consumers.

**Returns:**
- `[]ConsumerStats`: Slice of consumer statistics

---

### Expiration Control

#### `SetTTL(ttl time.Duration)`

Updates the time-to-live for queue items.

**Parameters:**
- `ttl` (time.Duration): New TTL value

**Thread-Safety:** Safe for concurrent use

**Example:**
```go
q.SetTTL(30 * time.Minute)
```

---

#### `GetTTL() time.Duration`

Returns current TTL setting.

**Returns:**
- `time.Duration`: Current TTL

---

#### `EnableExpiration()`

Enables automatic expiration of items.

**Effect:** Background worker will remove expired items every 30 seconds.

---

#### `DisableExpiration()`

Disables automatic expiration.

**Effect:** Items will not expire automatically, but ForceExpiration() still works.

---

#### `ForceExpiration() int`

Manually triggers expiration cleanup.

**Returns:**
- `int`: Number of items removed

**Example:**
```go
removed := q.ForceExpiration()
fmt.Printf("Removed %d expired items\n", removed)
```

**Note:** Respects expirationEnabled flag. Returns 0 if expiration is disabled.

---

### Lifecycle Management

#### `Close()`

Closes the queue and releases all resources.

**Effect:**
- Stops expiration worker
- Closes all consumers
- Clears all data
- Wakes up all blocked operations

**Idempotency:**
- Safe to call multiple times
- Subsequent calls return immediately without error

**Example:**
```go
q.Close() // Or use defer q.Close()

// Safe to call again
q.Close() // No-op, no panic
```

**Thread-Safety:** Safe to call concurrently from multiple goroutines

**Note:** Always call Close() when done to prevent resource leaks. Blocked Enqueue() and Read() operations will be interrupted and return error/nil.

---

#### `CloseWithContext(ctx context.Context) error`

Closes the queue with timeout support.

**Parameters:**
- `ctx` (context.Context): Context with timeout

**Returns:**
- `error`: nil on success, context error on timeout

**Example:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := q.CloseWithContext(ctx); err != nil {
    // Handle timeout
}
```

---

## Consumer API

### Reading Data (Blocking)

#### `Read() *QueueData`

Reads the next item for this consumer, **blocking** if no data is available.

**Returns:**
- `*QueueData`: Next data item or nil if queue is closed

**Behavior:**
- **Blocks** when no data is available
- Waits for new data to be enqueued
- Returns nil if queue is closed while waiting
- Advances consumer's position
- Skips expired items automatically

**Thread-Safety:** Each consumer is independent and thread-safe

**Example:**
```go
// Will wait for data if queue is empty
data := consumer.Read()
if data == nil {
    // Queue was closed
    return
}

// Process data
fmt.Printf("ID: %s, Payload: %v\n", data.ID, data.Payload)
```

**Use Cases:**
- Worker threads that process data continuously
- Event-driven processing
- Stream processing

---

#### `ReadBatch(limit int) []*QueueData`

Reads multiple items, **blocking** until at least one item is available.

**Parameters:**
- `limit` (int): Maximum items to read

**Returns:**
- `[]*QueueData`: Slice of items (length between 1 and limit)

**Behavior:**
- **Blocks** until at least 1 item is available
- Returns immediately once data is available
- May return fewer items than limit if queue has fewer items

**Example:**
```go
// Waits for at least 1 item, then returns up to 10
batch := consumer.ReadBatch(10)
for _, data := range batch {
    fmt.Printf("Processing: %v\n", data.Payload)
}
```

---

### Reading Data (Context-Aware)

#### `ReadWithContext(ctx context.Context) (*QueueData, error)`

Reads the next item for this consumer, blocking until data is available or context is cancelled.

**Parameters:**
- `ctx` (context.Context): Context for cancellation/timeout

**Returns:**
- `*QueueData`: Next data item (nil if queue closed or context cancelled)
- `error`: nil on success, context error on timeout/cancellation

**Example:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
defer cancel()

data, err := consumer.ReadWithContext(ctx)
if err != nil {
    // Handle timeout
}
```

---

#### `ReadBatchWithContext(ctx context.Context, limit int) ([]*QueueData, error)`

Reads multiple items, blocking until at least one item is available or context is cancelled.

**Parameters:**
- `ctx` (context.Context): Context for cancellation/timeout
- `limit` (int): Maximum items to read

**Returns:**
- `[]*QueueData`: Slice of items
- `error`: nil on success (even if partial batch), context error on timeout/cancellation

---

### Reading Data (Non-Blocking)

#### `TryRead() *QueueData`

Attempts to read the next item without blocking.

**Returns:**
- `*QueueData`: Next data item or nil if no data available

**Behavior:**
- Returns **immediately** if no data is available
- Does not wait for new data
- Advances consumer's position
- Skips expired items automatically

**Example:**
```go
data := consumer.TryRead()
if data == nil {
    // No data available right now
    return
}

// Process data
fmt.Printf("ID: %s, Payload: %v\n", data.ID, data.Payload)
```

**Use Cases:**
- Tests (to avoid blocking)
- Polling-based consumers
- Optional data processing

---

#### `TryReadBatch(limit int) []*QueueData`

Attempts to read multiple items without blocking.

**Parameters:**
- `limit` (int): Maximum items to read

**Returns:**
- `[]*QueueData`: Slice of data items (may be empty or less than limit)

**Behavior:**
- Returns **immediately** with available items
- May return empty slice if no data available
- May return fewer items than limit

**Example:**
```go
items := consumer.TryReadBatch(10)
if len(items) == 0 {
    // No data available
    return
}

for _, data := range items {
    fmt.Printf("Processing: %v\n", data.Payload)
}
```

**Time Complexity:** O(n) where n is number of items read

---

#### `HasMoreData() bool`

Checks if more data is available without consuming it.

**Returns:**
- `bool`: true if data is available

**Thread-Safety:** Safe for concurrent use

**Example:**
```go
if consumer.HasMoreData() {
    data := consumer.TryRead()
    // Process data
}
```

---

### Filtering Methods

#### `TryReadWhere(predicate func(*QueueData) bool) *QueueData`

Attempts to read the next data item matching the predicate without blocking.

**Parameters:**
- `predicate` (function): Function that returns true for items to return

**Returns:**
- `*QueueData`: First matching item, or nil if no match found

**Behavior:**
- Returns **immediately** (non-blocking)
- Advances consumer position while searching
- Non-matching items are consumed
- Returns nil if predicate is nil

**Example:**
```go
// Read first even number
data := consumer.TryReadWhere(func(d *queue.QueueData) bool {
    num, ok := d.Payload.(int)
    return ok && num%2 == 0
})

if data != nil {
    fmt.Printf("Found even number: %v\n", data.Payload)
}
```

**Important Notes:**
- Consumer position advances as it searches
- Non-matching items are consumed (not skipped)
- Use for selective processing of queue items

---

#### `ReadWhere(predicate func(*QueueData) bool) *QueueData`

Reads the next data item matching the predicate, blocking until a match is found.

**Parameters:**
- `predicate` (function): Function that returns true for items to return

**Returns:**
- `*QueueData`: First matching item, or nil if queue closes

**Behavior:**
- **Blocks** until matching item is available
- Advances consumer position while searching
- Non-matching items are consumed
- Returns nil if predicate is nil
- Returns nil if queue is closed while waiting

**Example:**
```go
// Wait for high-priority item
data := consumer.ReadWhere(func(d *queue.QueueData) bool {
    priority, ok := d.Payload.(string)
    return ok && priority == "high"
})

if data != nil {
    fmt.Printf("Processing high-priority: %v\n", data.Payload)
}
```

**Use Cases:**
- Selective message processing
- Priority-based filtering
- Type-specific consumers
- Conditional data extraction

---

#### `ReadWhereWithContext(ctx context.Context, predicate func(*QueueData) bool) (*QueueData, error)`

Reads the next matching item with context support for cancellation and timeout.

**Parameters:**
- `ctx` (context.Context): Context for cancellation/timeout
- `predicate` (function): Function that returns true for items to return

**Returns:**
- `*QueueData`: First matching item
- `error`: Context error (Canceled, DeadlineExceeded) or nil

**Behavior:**
- **Blocks** until match found or context done
- Respects context cancellation
- Respects context timeout
- Returns nil data and error on context cancellation
- Returns nil data and nil error if predicate is nil

**Example:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

data, err := consumer.ReadWhereWithContext(ctx, func(d *queue.QueueData) bool {
    return d.Payload.(int) > 100
})

if err == context.DeadlineExceeded {
    fmt.Println("Timeout waiting for matching item")
} else if err == context.Canceled {
    fmt.Println("Operation cancelled")
} else if data != nil {
    fmt.Printf("Found: %v\n", data.Payload)
}
```

**Error Handling:**
- `context.Canceled`: Context was cancelled
- `context.DeadlineExceeded`: Timeout occurred
- `nil`: Success or queue closed

---

### Consumer Information

#### `GetID() string`

Returns the consumer's unique identifier.

**Returns:**
- `string`: UUID of the consumer

---

#### `GetPosition() (*list.Element, int)`

Returns the consumer's current position in the queue.

**Returns:**
- `*list.Element`: Current chunk element
- `int`: Index within the chunk

**Use Case:** For debugging or position tracking.

---

#### `GetUnreadCount() int64`

Returns the number of unread items for this consumer.

**Returns:**
- `int64`: Count of unread items

**Example:**
```go
count := consumer.GetUnreadCount()
fmt.Printf("Consumer has %d unread items\n", count)
```

---

#### `GetStats() ConsumerStats`

Returns statistics for this consumer.

**Returns:**
- `ConsumerStats`: Statistics structure

**ConsumerStats Fields:**
```go
type ConsumerStats struct {
    ID             string    // Consumer UUID
    TotalItemsRead int64     // Total items read
    UnreadItems    int64     // Remaining unread items
    LastReadTime   time.Time // Time of last read
}
```

---

#### `GetDequeueHistory() []DequeueRecord`

Returns a copy of this consumer's dequeue history.

**Returns:**
- `[]DequeueRecord`: Slice of dequeue records

**DequeueRecord Fields:**
```go
type DequeueRecord struct {
    DataID    string    // ID of dequeued item
    Timestamp time.Time // Time of dequeue
}
```

**Example:**
```go
history := consumer.GetDequeueHistory()
for _, record := range history {
    fmt.Printf("Read item %s at %s\n", record.DataID, record.Timestamp)
}
```

---

#### `GetNotificationChannel() <-chan int`

Returns the channel for expiration notifications.

**Returns:**
- `<-chan int`: Receive-only channel for expired item counts

**Example:**
```go
go func() {
    for expiredCount := range consumer.GetNotificationChannel() {
        fmt.Printf("Consumer notified: %d items expired\n", expiredCount)
    }
}()
```

---

### Position Management

#### `SetPosition(element *list.Element, index int)`

Sets the consumer's position manually.

**Parameters:**
- `element` (*list.Element): Chunk element
- `index` (int): Index within chunk

**Use Case:** Advanced position management or restoring state.

**Warning:** Use with caution. Invalid positions can cause issues.

---

#### `Close()`

Closes the consumer and releases resources.

**Effect:** Closes notification channel.

**Example:**
```go
consumer.Close()
```

**Note:** Consumer is automatically closed when queue closes.

---

## Data Types

### QueueData

Represents a single item in the queue. Immutable after creation.

**Fields:**
```go
type QueueData struct {
    ID           string     // Unique UUID
    Payload      any        // User data (any type)
    EnqueueEvent QueueEvent // Single enqueue event
    Created      time.Time  // Creation timestamp
}
```

**Methods:**

#### `GetEnqueueEvent() QueueEvent`

Returns the enqueue event for this data.

**Returns:**
- `QueueEvent`: Enqueue event details

#### `IsExpired(ttl time.Duration) bool`

Checks if data has exceeded the TTL.

**Parameters:**
- `ttl` (time.Duration): Time-to-live to check against

**Returns:**
- `bool`: true if expired

---

### QueueEvent

Represents an event in the queue's history.

**Fields:**
```go
type QueueEvent struct {
    Timestamp time.Time // When event occurred
    QueueName string    // Queue where event occurred
    EventType string    // "enqueue" or "dequeue"
}
```

---

### QueueStats

Queue statistics (see GetQueueStats).

---

### ConsumerStats

Consumer statistics (see GetStats).

---

### DequeueRecord

Dequeue history record (see GetDequeueHistory).

---

## Error Types

### MemoryLimitError

Returned when queue memory limit is exceeded.

**Fields:**
```go
type MemoryLimitError struct {
    Current int64 // Current memory usage
    Max     int64 // Maximum allowed (1MB)
    Needed  int64 // Memory needed for operation
}
```

**Example:**
```go
err := q.Enqueue(largePayload)
if memErr, ok := err.(*queue.MemoryLimitError); ok {
    fmt.Printf("Memory limit exceeded: %d/%d bytes (need %d more)\n", 
        memErr.Current, memErr.Max, memErr.Needed)
}
```

---

### QueueError

General queue error.

**Fields:**
```go
type QueueError struct {
    Message string // Error description
}
```

---

### QueueClosedError

Returned when an operation is attempted on a closed queue.

**Fields:**
```go
type QueueClosedError struct {
    Operation string // The operation that was attempted
}
```

**Example:**
```go
err := q.Enqueue("data")
if qcErr, ok := err.(*queue.QueueClosedError); ok {
    fmt.Printf("Cannot %s: queue is closed\n", qcErr.Operation)
}

// Or use errors.As for error chain support
var qcErr *queue.QueueClosedError
if errors.As(err, &qcErr) {
    fmt.Printf("Queue closed during: %s\n", qcErr.Operation)
}
```

**When Returned:**
- When blocking `Enqueue()` is interrupted by queue closure
- When blocking `EnqueueBatch()` is interrupted by queue closure
- When blocking `EnqueueBatchWithContext()` is interrupted by queue closure

---

### ConsumerNotFoundError

Returned when a consumer with the given ID cannot be found.

**Fields:**
```go
type ConsumerNotFoundError struct {
    ID string // The consumer ID that was not found
}
```

**Example:**
```go
var cnfErr *queue.ConsumerNotFoundError
if errors.As(err, &cnfErr) {
    fmt.Printf("Consumer not found: %s\n", cnfErr.ID)
}
```

**Note:** Currently reserved for future use. Consumer lookup methods return nil instead of errors.

---

### InvalidPositionError

Returned when a consumer position is invalid.

**Fields:**
```go
type InvalidPositionError struct {
    Position int64  // The invalid position
    Reason   string // Why the position is invalid
}
```

**Example:**
```go
var ipErr *queue.InvalidPositionError
if errors.As(err, &ipErr) {
    fmt.Printf("Invalid position %d: %s\n", ipErr.Position, ipErr.Reason)
}
```

**Note:** Currently reserved for future use. Position validation may use this in future versions.

---

## Constants

### DefaultTTL

Default time-to-live for queue items.

```go
const DefaultTTL = 10 * time.Minute
```

---

### ExpirationCheckInterval

How often the background worker checks for expired items.

```go
const ExpirationCheckInterval = 30 * time.Second
```

---

### MaxQueueMemory

Maximum memory allowed for a queue.

```go
const MaxQueueMemory = 1024 * 1024 // 1MB
```

---

## Thread-Safety Guarantees

- ✅ **Queue.Enqueue()**: Safe for concurrent producers
- ✅ **Queue.EnqueueBatch()**: Safe for concurrent producers
- ✅ **Queue.AddConsumer()**: Safe for concurrent use
- ✅ **Consumer.Read()**: Each consumer is independent and thread-safe
- ✅ **Consumer.ReadBatch()**: Thread-safe per consumer
- ✅ **All query methods**: Thread-safe

**Note:** QueueData is immutable after creation, making it inherently thread-safe.

---

## Best Practices

1. **Always call Close()**: Use `defer q.Close()` to prevent resource leaks
2. **Check errors**: Always check MemoryLimitError on enqueue
3. **Independent consumers**: Each consumer tracks its own position
4. **Expiration awareness**: Items expire after TTL, plan accordingly
5. **Memory management**: Monitor memory usage with GetMemoryUsage()
6. **Batch operations**: Use EnqueueBatch() for better performance

---

## Performance Characteristics

| Operation | Time Complexity | Thread-Safe | Notes |
|-----------|----------------|-------------|-------|
| Enqueue | O(1) amortized | ✅ | May create new chunk |
| EnqueueBatch | O(n) | ✅ | n = batch size |
| Read | O(1) amortized | ✅ | Per consumer |
| ReadBatch | O(n) | ✅ | n = items read |
| AddConsumer | O(1) | ✅ | Independent tracking |
| RemoveConsumer | O(1) | ✅ | Hash map lookup |
| GetMemoryUsage | O(1) | ✅ | Atomic read |
| ForceExpiration | O(m) | ✅ | m = expired items |

---

## See Also

- [Architecture Guide](./ARCHITECTURE.md)
- [Usage Guide](./USAGE_GUIDE.md)
- [Performance Guide](./PERFORMANCE.md)
- [Troubleshooting](./TROUBLESHOOTING.md)
