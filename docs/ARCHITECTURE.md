# mpmc-queue Architecture

This document describes the internal architecture, design decisions, and implementation details of the mpmc-queue.

## Table of Contents

- [Overview](#overview)
- [Core Components](#core-components)
- [Data Structures](#data-structures)
- [Threading Model](#threading-model)
- [Memory Management](#memory-management)
- [Expiration System](#expiration-system)
- [Lock Hierarchy](#lock-hierarchy)
- [Design Decisions](#design-decisions)

---

## Overview

mpmc-queue is a multi-producer, multi-consumer (MPMC) queue implementation optimized for:

- **High Concurrency**: Multiple producers and consumers can operate simultaneously
- **Memory Constraints**: Configurable memory limit (default 1MB) with real-time tracking
- **Time-based Expiration**: Automatic cleanup of expired items
- **Independent Consumers**: Each consumer maintains its own read position
- **Thread Safety**: All operations are safe for concurrent use

### Key Characteristics

- **Lock-Based**: Uses RWMutex for synchronization
- **Chunked Storage**: Items stored in 1000-item chunks
- **Immutable Data**: QueueData cannot be modified after creation
- **Atomic Operations**: Size tracking and statistics use atomic types (lock-free reads)
- **Blocking/Non-Blocking**: Supports both blocking and non-blocking operations
- **Configurable TTL**: Time-to-live set at queue creation (default 10 minutes)

---

## Core Components

### 1. Queue

The central coordinator that manages data, consumers, and expiration.

```
┌──────────────────────────────────────────────┐
│              Queue                           │
│  ┌────────────────────────────────────────┐  │
│  │  name, ttl, mutex, stopChan            │  │
│  │  enqueueNotify, dequeueNotify channels │  │
│  └────────────────────────────────────────┘  │
│                                              │
│  ┌─────────────┐  ┌──────────────────┐       │
│  │ ChunkedList │  │ ConsumerManager  │       │
│  └─────────────┘  └──────────────────┘       │
│                                              │
│  ┌─────────────┐  ┌──────────────────┐       │
│  │MemoryTracker│  │ExpirationWorker  │       │
│  └─────────────┘  └──────────────────┘       │
└──────────────────────────────────────────────┘
```

**Responsibilities:**
- Coordinate all queue operations
- Manage concurrent access with RWMutex
- Track memory usage
- Run expiration background worker
- Notify consumers when data is enqueued (enqueueNotify channel)
- Notify producers when space is freed (dequeueNotify channel)

---

### 2. ChunkedList

Manages storage of QueueData in fixed-size chunks.

```
ChunkedList
    ↓
container/list.List
    ↓
[ChunkNode] ←→ [ChunkNode] ←→ [ChunkNode]
    ↓              ↓              ↓
[Data array]   [Data array]   [Data array]
 1000 items     1000 items     1000 items
```

**Why Chunked?**
- Efficient memory allocation (allocate 1000 at a time)
- Better cache locality
- Easy chunk removal when empty
- Prevents frequent small allocations

---

### 3. ChunkNode

Fixed-size array container for QueueData items.

```go
type ChunkNode struct {
    Data [1000]*QueueData  // Fixed array of pointers
    size int32             // Atomic size counter
}
```

**Key Features:**
- Fixed 1000-item capacity
- Atomic size operations (thread-safe)
- Pointer array (minimal memory per empty slot)
- Compaction support for expiration

---

### 4. QueueData

Immutable container for user data.

```go
type QueueData struct {
    ID           string     // UUID
    Payload      any        // User data (immutable)
    EnqueueEvent QueueEvent // Single enqueue event
    Created      time.Time  // For expiration
}
```

**Immutability Benefits:**
- Thread-safe without locks
- Can be shared by multiple consumers
- Memory size fixed after creation
- No race conditions on data access

---

### 5. Consumer

Independent reader with its own position tracking.

```
Consumer 1:  Chunk A[5] ─┐
Consumer 2:  Chunk B[200]├─→ All reading same queue
Consumer 3:  Chunk A[0] ─┘
```

**Features:**
- UUID-based identification
- Independent position tracking
- Local dequeue history
- Notification channel for expirations

---

### 6. ConsumerManager

Manages the collection of active consumers.

```
ConsumerManager
    ↓
map[string]*Consumer
    ↓
{
    "uuid-1": Consumer 1,
    "uuid-2": Consumer 2,
    "uuid-3": Consumer 3
}
```

**Responsibilities:**
- Consumer lifecycle management
- Notification distribution
- Position updates during expiration

---

### 7. ConsumerGroup

Manages shared state for a group of consumers.

- **Shared Position**: All consumers in the group read from the same `chunkElement` and `index`.
- **Load Balancing**: `TryRead` atomically advances the shared position, ensuring each item is delivered to exactly one consumer in the group.
- **Thread Safety**: Uses mutex to protect shared position updates.

---

### 8. MemoryTracker

Tracks memory usage with configurable limit (default 1MB).


```
MemoryTracker
    ↓
totalMemory: atomic counter
    ↓
[Chunk overhead] + [QueueData 1] + [QueueData 2] + ...
```

**Tracking:**
- Chunk structure overhead
- QueueData structure size
- Payload size (estimated via reflection)
- String and slice sizes

---

## Data Structures

### Memory Layout

```
Queue (root)
    │
    ├─→ ChunkedList
    │       │
    │       ├─→ container/list.List (doubly-linked)
    │       │       │
    │       │       ├─→ Element ──→ ChunkNode
    │       │       │                   └─→ Data[1000]*QueueData
    │       │       │
    │       │       ├─→ Element ──→ ChunkNode
    │       │       │                   └─→ Data[1000]*QueueData
    │       │       └─→ ...
    │       │
    │       └─→ MemoryTracker (atomic counters)
    │
    ├─→ ConsumerManager
    │       └─→ map[string]*Consumer
    │               │
    │               ├─→ Consumer (chunkElement, indexInChunk)
    │               ├─→ Consumer (chunkElement, indexInChunk)
    │               └─→ ...
    │
    └─→ ExpirationWorker (background goroutine)
```

---

## Threading Model

### Synchronization Strategy

**Queue-Level Lock (RWMutex):**
- Protects queue data structures
- Write lock for modifications (Enqueue, Expiration)
- Read lock for queries (GetStats, iteration)

**Consumer-Level Lock (Mutex):**
- Protects consumer state
- Independent per consumer
- Does not block other consumers

**Atomic Operations:**
- ChunkNode.size (int32)
- Lock-free size checks

### Lock Hierarchy

```
1. Queue.mutex (outermost)
   ├─ Can acquire while holding this:
   │  └─ Consumer.mutex (inner)
   │
   └─ Cannot acquire: None (deadlock-free)

2. Consumer.mutex (inner)
   └─ Never acquires Queue.mutex while holding
```

**Deadlock Prevention:**
- Consistent lock ordering: queue → consumer (never consumer → queue)
- Never acquire outer lock while holding inner
- Read locks don't block other readers
- **Snapshot Pattern**: Consumer methods snapshot state before acquiring queue lock

**Critical Pattern - Snapshot Before Queue Lock:**
```go
// Consumer methods that need queue data
c.mutex.Lock()
chunkElement := c.chunkElement  // Snapshot
indexInChunk := c.indexInChunk
c.mutex.Unlock()

// Now safe to acquire queue lock
c.queue.mutex.RLock()
// Use snapshots: chunkElement, indexInChunk
c.queue.mutex.RUnlock()
```

**Methods Using Snapshot Pattern:**
- `Consumer.Read()` - Fine-grained locking with position snapshots
- `Consumer.HasMoreData()` - Snapshots before queue access
- `Consumer.GetUnreadCount()` - Snapshots position fields
- `Consumer.GetStats()` - Snapshots all consumer state

**TOCTOU Protection:**
Consumer.Read() keeps queue lock held during chunk.Get() to prevent time-of-check-to-time-of-use issues with expiration modifying chunks.

---

### Concurrent Operations

**Multiple Producers (Enqueue):**
```
Producer 1 ────┐
Producer 2 ────┼──→ Queue.mutex.Lock() ──→ Serialize
Producer 3 ────┘
```

**Multiple Consumers (Read):**
```
Consumer 1 ────┐
Consumer 2 ────┼──→ Independent (no blocking)
Consumer 3 ────┘
```

**Producer + Consumer:**
```
Producer: Queue.mutex.Lock()  ──→ Enqueue ──→ Unlock
    │
    └─→ Consumer: Queue.mutex.RLock() ──→ Get chunk ──→ RUnlock
                      │
                      └─→ Read chunk data (lock-free via atomics)
```

---

## Blocking Operations

### Design Overview

The queue supports both blocking and non-blocking operations using efficient channel-based notifications.

### Notification Channels

**enqueueNotify** (buffered, capacity 1):
- Signals consumers when new data is enqueued
- Buffered to avoid blocking producers
- Non-blocking send (select with default)

**dequeueNotify** (buffered, capacity 1):
- Signals producers when space becomes available
- Triggered by expiration worker
- Buffered to avoid blocking cleanup

### Blocking Enqueue Flow

```
Producer calls Enqueue(data)
    │
    ├─→ Lock queue
    ├─→ Try to add data
    │   ├─ Success? ──→ Notify consumers ──→ Return
    │   └─ Memory full?
    │       │
    │       ├─→ Unlock queue
    │       ├─→ Wait on dequeueNotify channel
    │       │   ├─ Data expired? ──→ Retry from top
    │       │   └─ Queue closed? ──→ Return error
    │       └─→ Loop
```

### Blocking Read Flow

```
Consumer calls Read()
    │
    ├─→ Try TryRead()
    │   ├─ Got data? ──→ Return data
    │   └─ No data?
    │       │
    │       ├─→ Wait on enqueueNotify channel
    │       │   ├─ Data enqueued? ──→ Retry from top
    │       │   └─ Queue closed? ──→ Return nil
    │       └─→ Loop
```

### Non-Blocking Operations

**TryEnqueue() / TryRead():**
- Attempt operation once
- Return immediately with error/nil if can't proceed
- No waiting on channels
- Used in tests to avoid blocking

### Efficiency Considerations

**Channel Notifications:**
- Buffered channels prevent blocking on send
- Multiple waiting goroutines woken efficiently
- Non-blocking send pattern (select with default)

**Spurious Wakeups:**
- Multiple consumers may wake on single notification
- Each retries TryRead() - no data race
- False wakeups are cheap (just retry)

---

## Memory Management

### Estimation Strategy

Memory usage = Chunk overhead + Sum(QueueData sizes)

**QueueData Size:**
```go
BaseSize +
len(ID) +                    // UUID string
EstimatePayload(Payload) +   // Recursive reflection
len(QueueName) +             // Event field
len(EventType)               // Event field
```

**Payload Estimation:**
- Primitives: Type size (int, float, bool)
- Strings: Length in bytes
- Slices/Arrays: Recursive sum of elements
- Maps: Sum of keys + values
- Structs: Recursive sum of fields
- Pointers: Size of pointed-to value

**Limitations:**
- Estimates, not exact (±10-20% typical)
- Conservative (better to overestimate)
- No actual memory introspection

---

### Memory Lifecycle

**On Enqueue:**
```
1. Create QueueData (fixed size after creation)
2. Estimate size
3. Check: currentMemory + estimatedSize <= 1MB
4. If ok: Add to chunk, update memory tracker
5. If not: Return MemoryLimitError
```

**On Expiration:**
```
1. Identify expired items
2. Remove from chunks (set pointers to nil)
3. Compact chunk arrays
4. Call memoryTracker.RemoveData() for each
5. Update totalMemory counter
```

**Memory Leak Prevention:**
- All additions tracked
- All removals tracked
- Proper cleanup on expiration
- Chunk removal when empty

---

## Expiration System

### Background Worker

```go
func expirationWorker() {
    ticker := time.NewTicker(30 * time.Second)
    for {
        select {
        case <-ticker.C:
            if expirationEnabled {
                cleanupExpiredItems()
            }
        case <-stopChan:
            return
        }
    }
}
```

**Interval:** 30 seconds (configurable via const)

---

### Expiration Process

```
1. Lock queue (exclusive)
2. Calculate per-consumer expired counts
3. Iterate chunks from oldest:
   a. Check each item's Created time
   b. Mark expired items as nil
   c. Collect removed items
   d. Stop at first non-expired item
4. Compact chunk arrays (shift data)
5. Update memory tracker (remove each item)
6. Remove empty chunks
7. Notify consumers:
   a. Send expiredCount to notification channel
   b. Update consumer positions (adjust for removals)
8. Unlock queue
```

**Optimizations:**
- Items ordered by creation time
- Stop checking after first non-expired
- Only process chunks with expired items

---

### Consumer Position Updates

**Challenge:** When items expire from a chunk, indices shift.

**Solution:**
```
Before: [Item0, Item1, Item2, Item3, Item4]
        Consumer at index 3 (Item3)

Items 0-1 expire:
After:  [Item2, Item3, Item4, null, null]
        Consumer needs to be at index 1 (still Item3)

Update: indexInChunk -= removedCount
```

**Implementation:**
- Track ChunkRemovalInfo per chunk
- Update each consumer's indexInChunk
- Handle chunk deletion (move to next chunk)

---

## Lock Hierarchy

### Detailed Locking Rules

**Rule 1: Queue Lock for Structural Changes**
```go
// Enqueue
q.mutex.Lock()              // Exclusive access
  cl.Enqueue(data)          // Modify list structure
  chunk.Add(data)           // Modify chunk
  memoryTracker.AddData()   // Update counters
q.mutex.Unlock()
```

**Rule 2: Consumer Lock for Position (Snapshot Pattern)**
```go
// Consumer.Read() - Updated implementation
c.mutex.Lock()
currentElement := c.chunkElement  // Snapshot position
currentIndex := c.indexInChunk
c.mutex.Unlock()

// Now safe to acquire queue lock
c.queue.mutex.RLock()
  chunk = currentElement.Value.(*ChunkNode)
  size = chunk.GetSize()           // Atomic read
  data = chunk.Get(currentIndex)   // Read while holding queue lock (TOCTOU protection)
c.queue.mutex.RUnlock()

// Update position with double-check
c.mutex.Lock()
if c.chunkElement == currentElement && c.indexInChunk == currentIndex {
  c.indexInChunk++  // Position hasn't changed, safe to update
}
c.mutex.Unlock()
```

**TOCTOU Protection:**
The queue lock must be held during `chunk.Get()` to prevent expiration from modifying the chunk between the size check and data read.

**Rule 3: Never Hold Both**
```go
// WRONG - Deadlock risk
c.mutex.Lock()
  q.mutex.Lock()  // ❌ Never do this

// CORRECT
q.mutex.RLock()
  c.mutex.Lock()  // ✅ OK: inner lock while holding outer
  c.mutex.Unlock()
q.mutex.RUnlock()
```

---

## Design Decisions

### Why RWMutex Instead of Lock-Free?

**Reasons:**
1. Simpler reasoning about correctness
2. Go's RWMutex is highly optimized
3. Read-heavy workload benefits from RLock
4. Lock-free adds complexity without clear benefit for this use case

**Trade-offs:**
- ✅ Easier to maintain and debug
- ✅ Predictable performance
- ❌ Some lock contention under extreme load
- ❌ Not wait-free

---

### Why Immutable QueueData?

**Original Problem:**
- Multiple consumers reading same data
- Appending dequeue events caused races
- Needed synchronization for every access

**Solution Benefits:**
1. **Thread-safe without locks:** Can be shared freely
2. **Fixed memory size:** Accurate tracking
3. **No data races:** Impossible to modify
4. **Simpler reasoning:** No defensive copying needed

**Trade-off:**
- ❌ Can't track which consumers read an item
- ✅ Solution: Each consumer tracks its own history

---

### Why Chunked List Instead of Ring Buffer?

**Ring Buffer Issues:**
1. Fixed total capacity (can't grow)
2. Consumer tracking complex with wraparound
3. Memory wasted for slow consumers
4. Difficult to implement expiration

**Chunked List Benefits:**
1. ✅ Dynamic growth (up to memory limit)
2. ✅ Easy consumer position tracking
3. ✅ Efficient expiration (remove old chunks)
4. ✅ Memory released incrementally

**Trade-off:**
- ❌ Slightly more overhead per item
- ✅ More flexible and maintainable

---

### Why 1000 Items Per Chunk?

**Considerations:**
- Too small (100): Frequent allocations, list overhead
- Too large (10000): Memory waste, slower iteration

**1000-Item Rationale:**
1. Good balance of allocation overhead
2. Reasonable iteration time for expiration
3. ~8KB per chunk (assuming 8-byte pointers)
4. Easy to work with (power-of-10-ish)

**Configurable?** Could be, but constant keeps code simpler.

---

### Why 1MB Memory Limit?

**Design Goal:** Embedded/resource-constrained environments

**Benefits:**
1. Predictable memory footprint
2. Prevents runaway growth
3. Forces deliberate backpressure handling
4. Suitable for microservices

**Trade-offs:**
- ❌ Not suitable for large queues
- ✅ Excellent for message buffering
- ✅ Forces good memory hygiene

**Future:** Could make configurable via QueueConfig.

---

### Why 30-Second Expiration Interval?

**Rationale:**
1. Low CPU overhead (not too frequent)
2. Reasonable cleanup latency for 10-minute TTL
3. Reduces lock contention
4. Most use cases tolerate this granularity

**Formula:** ~5% of default TTL (10 min / 30 sec = 20 checks per TTL)

**Trade-off:**
- ❌ Items may linger up to 30 seconds past TTL
- ✅ Low CPU impact
- ✅ Can call ForceExpiration() for immediate cleanup

---

## Performance Characteristics

### Time Complexity

| Operation | Best | Average | Worst | Notes |
|-----------|------|---------|-------|-------|
| Enqueue | O(1) | O(1) | O(1) | May allocate chunk |
| Read | O(1) | O(1) | O(n) | n = expired items skipped |
| AddConsumer | O(1) | O(1) | O(1) | Hash map insert |
| RemoveConsumer | O(1) | O(1) | O(1) | Hash map delete |
| Expiration | O(e) | O(e) | O(e) | e = expired items |
| GetStats | O(1) | O(1) | O(1) | Atomic reads |

---

### Space Complexity

```
Total Memory =
    Queue overhead +
    n * ChunkNode overhead +
    m * QueueData size +
    c * Consumer overhead

Where:
    n = number of chunks (≈ items / 1000)
    m = number of items
    c = number of consumers
```

**Typical:**
- Queue: ~200 bytes
- ChunkNode: ~8KB (1000 * 8-byte pointers)
- QueueData: ~100-500 bytes (depends on payload)
- Consumer: ~200 bytes

---

## Limitations

1. **Memory Limit:** Configurable limit (default 1MB)
2. **Expiration Granularity:** 30-second intervals
3. **Memory Estimation:** Approximate (±10-20%)
4. **No Persistence:** In-memory only
5. **No Priority:** FIFO only
6. **No Filtering:** Consumers read sequentially

---

## Future Improvements

### Potential Enhancements

1. **Configurable Memory Limit**
   ```go
   type QueueConfig struct {
       MaxMemory int64
       ChunkSize int
       TTL       time.Duration
   }
   ```

2. **Adaptive Chunk Size**
   - Small chunks for low volume
   - Large chunks for high volume

3. **Lock-Free Fast Path**
   - Use atomics for read-only operations
   - Fall back to locks for modifications

4. **Memory Pool**
   - Reuse ChunkNode allocations
   - Reduce GC pressure

5. **Metrics Instrumentation**
   - Prometheus-style metrics
   - Enqueue/dequeue rates
   - Memory trends

---

## See Also

- [API Documentation](./API.md)
- [Usage Guide](./USAGE_GUIDE.md)
- [Performance Guide](./PERFORMANCE.md)
- [Troubleshooting](./TROUBLESHOOTING.md)
- [AGENTS.md](../AGENTS.md) - For AI agent developers
