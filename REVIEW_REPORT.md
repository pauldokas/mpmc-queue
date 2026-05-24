# Adversarial Review: MPMC Queue
**Date:** May 23, 2026

## Executive Summary
An adversarial review involving Code, Architecture, Performance, and Security analysis of the `mpmc-queue` project was conducted. Despite the project's documentation claiming it is "production ready" and "race-condition free," several severe logic bugs, race conditions, and edge cases were identified. This review categorizes findings by severity, ranging from application-crashing panics and deadlocks to significant performance bottlenecks.

---

## 🚨 Critical Severity (Panics, Deadlocks & Memory Corruption)

### 1. Guaranteed Deadlock in Consumer Management (DoS)
There is a severe lock-ordering violation between `Queue` and `ConsumerManager` that will deadlock the entire queue system.
- **Trigger**: A client calls `q.AddConsumer()` simultaneously while the background `expirationWorker` runs `q.cleanupExpiredItems()`.
- **Mechanism**:
  - `AddConsumer()` acquires `ConsumerManager.mutex.Lock()` and then attempts to acquire `Queue.mutex.RLock()`.
  - `cleanupExpiredItems()` acquires `Queue.mutex.Lock()` and then calls `NotifyAllConsumersOfExpiration`, which attempts to acquire `ConsumerManager.mutex.RLock()`.
- **Impact**: Deadlock. All producers and consumers will hang indefinitely.
- **Remedy**: Standardize the lock order. Always acquire `Queue.mutex` *before* `ConsumerManager.mutex`.

### 2. Panic on Consumer Teardown vs Expiration (Race Condition)
- **Location:** `Consumer.Close()` and `Consumer.NotifyExpiredItems()`
- **Bug:** `Consumer.Close()` executes `close(c.notificationCh)`. However, the background expiration worker can concurrently process expired items and call `NotifyExpiredItems`, which executes a non-blocking send: `case c.notificationCh <- count`. Sending to a closed channel in Go immediately causes a **panic**, taking down the entire application.
- **Remedy:** Never close the channel from the receiver side (the consumer), or use synchronization to prevent the queue from sending notifications to closed consumers.

### 3. Use-After-Free of Recycled Chunks in Expiration (ADR 0003 Flaw)
- **Location:** `ChunkedList.RemoveExpiredData()` and the "Snapshot Pattern".
- **Bug:** ADR 0003 prescribes a "Snapshot Pattern" (unlock Consumer, copy `chunkElement`, re-lock Queue). Between the unlocking and re-locking, the expiration worker can lock `Queue.mutex`, remove the exact `chunkElement`, and recycle its `ChunkNode` to the global `sync.Pool`.
  Furthermore, in `RemoveExpiredData()`, when an expired chunk becomes empty, it is removed and recycled via `PutChunkNode(chunk)`. Directly after this, the loop evaluates `if !chunk.IsEmpty() { ... }` and calls `chunk.GetEarliestExpiry()`. Another goroutine could concurrently pull and write to this recycled chunk.
- **Impact:** Critical Data Race, Use-After-Free. Accessing memory actively mutated by another queue without shared synchronization.
- **Remedy:** Abandon the snapshot pattern for raw memory pointers. Acquire `Queue.mutex.RLock()` *first*, then lock `Consumer.mutex`. Ensure all chunk logic occurs *before* calling `PutChunkNode(chunk)`.

### 4. Stack Overflow via Cyclic Payloads (Panic / DoS)
- **Location:** `MemoryTracker.estimateValueSize` in `memory.go`
- **Bug:** `estimateValueSize` uses recursive reflection without tracking visited pointers. If a payload contains a cyclic data structure (e.g., `type Loop struct { Next *Loop }; a := &Loop{}; a.Next = a`), it will infinitely recurse.
- **Impact:** Immediate application crash (Go panics on stack overflows cannot be recovered).
- **Remedy:** Add a `visited map[uintptr]bool` to detect and break cyclic pointer chains.

### 5. Consumer Pointer Retention When Queue Empties
- **Location:** `Consumer.UpdatePositionAfterExpiration()`
- **Bug:** When expiration removes *all* remaining chunks, `newFirstElement` is `nil`. The update logic explicitly checks `if newFirstElement != nil` and skips the update. Thus, `c.chunkElement` points to the old, recycled chunk. Subsequent `TryRead()` calls read from a pooled chunk instead of recognizing an empty queue.
- **Remedy:** Handle the empty state: `if newFirstElement == nil { c.chunkElement = nil; c.indexInChunk = 0 }`.

---

## 🔴 High Severity (State Corruption, Broken Guarantees & Throughput Collapse)

### 6. The "Stop-The-World" Hot Path (Catastrophic Bottlenecks)
- **Location:** `TryEnqueueBatch`, `EnqueueBatch`, `Enqueue` in `queue.go`
- **Bug:** The global `q.mutex.Lock()` is acquired *before* initializing data. Inside the lock, the system calls `time.Now()`, `uuid.New().String()` (heap allocation & syscall), and `memoryTracker.EstimateQueueDataSize()` (reflection & acquiring `sizeCache` RWMutex).
- **Impact:** Serializes all producers across all cores. Obliterates throughput under high core counts.
- **Remedy:** Pre-compute `QueueData` and memory footprint *before* acquiring the lock. Consider dropping/replacing `uuid.New().String()` with a fast atomic `uint64` sequence.

### 7. Phantom Enqueue to Closed Queue (TOCTOU)
- **Location:** `Queue.TryEnqueue()` and `Queue.TryEnqueueBatch()`
- **Bug:** `q.closed.Load()` is checked outside the critical section, but not *inside* the lock. If `Queue.Close()` runs in this tight window, it clears/closes the queue. Enqueue then acquires the lock and adds data to a "closed" queue. Consumers are closed, so this data leaks indefinitely.
- **Remedy:** Re-check `if q.closed.Load()` immediately after acquiring `q.mutex.Lock()`.

### 8. Detached Chunk Traversal in Consumer Stats
- **Location:** `Consumer.GetUnreadCount()`, `HasMoreData()`, `GetStats()`
- **Bug:** `c.chunkElement` is snapshotted lock-free, *then* `q.queue.mutex.RLock()` is acquired. If expiration removes `c.chunkElement` in that microsecond window, the methods traverse a detached chunk (`Next()` is `nil`), returning wildly inaccurate unread counts.
- **Remedy:** Acquire `q.mutex.RLock()`, *then* `c.mutex.Lock()`.

### 9. Broken Atomicity Guarantees in Batch Operations
- **Location:** `Queue.EnqueueBatch()` and `Queue.EnqueueBatchWithContext()`
- **Bug:** Claimed to be strictly atomic. However, if `q.data.Enqueue(data)` fails midway through the loop, the method returns the error but fails to rollback the successfully enqueued items.
- **Remedy:** Implement a rollback block that pops partially inserted elements and refunds the `MemoryTracker`.

### 10. O(N * K) Structural Bottleneck in Expiration Worker
- **Location:** `Queue.cleanupExpiredItems`
- **Bug:** Executes while holding the global exclusive `Queue.mutex.Lock()`. It calculates expired counts (O(N) consumers × O(M) items) and notifies consumers (O(N)), which traverses chunks (O(K)). Under high throughput, holding the global write lock for this O(N * K) operation causes catastrophic latency spikes.
- **Remedy:** Expiration should be O(1) per chunk dropped. Consumers should handle expired chunks *lazily* during `TryRead()`.

### 11. Unbounded Consumer Group/Map Growth (Memory Leak)
- **Location:** `ConsumerManager`
- **Bug:** `AddGroup(name)` adds to `cm.groups`, but there is no `RemoveGroup()`. Dropped consumers without `RemoveConsumer()` calls leak the `Consumer` object, `notificationCh`, and history slice forever.
- **Impact:** Silent OOM crashes.
- **Remedy:** Implement `RemoveConsumerGroup(name)` and automatic eviction for inactive consumers.

### 12. OOM Allocation Panic in Batch Reads
- **Bug:** `TryReadBatch(limit)` allocates `make([]*QueueData, 0, limit)`. Passing a massive integer (e.g., `math.MaxInt`) immediately panics the Go runtime with `out of memory`.
- **Remedy:** Cap the `limit` parameter to a safe maximum.

### 13. Slice Bounds Out of Range (Panic / DoS)
- **Bug:** `QueueConfig.MaxConsumerHistory` set to a negative number (e.g., `-1`). In `Consumer.addToHistoryUnsafe()`, `excess` becomes > length, triggering a slice bounds panic.
- **Remedy:** Validate `QueueConfig` at creation (`MaxConsumerHistory >= 0`).

---

## 🟡 Medium Severity (Logic Flaws, API Risks & Contention)

### 14. Lock Contention & Granularity
- **Bug:** `Queue.mutex` is an `sync.RWMutex`. High frequency read/writes cause writer starvation or reader queuing, leading to cache invalidation.
- **Remedy:** Implement lock-free slot reservation (`atomic.AddInt32(&chunk.size, 1)`) so multiple producers can write to the current chunk concurrently.

### 15. Destructive Consumption in Filter Operations
- **Location:** `TryReadWhere` and `ReadWhere`
- **Bug:** In a `ConsumerGroup`, `ReadWhere` permanently consumes non-matching items from the shared cursor, robbing other consumers of data.
- **Remedy:** Implement a non-destructive `PeekWhere` or restrict filtering logic.

### 16. Context Cancellation Ignored on Entry
- **Location:** `Queue.EnqueueWithContext()`
- **Bug:** Enters the lock acquisition loop without checking `ctx.Done()`. Canceled contexts still enqueue data.
- **Remedy:** Check context before locking.

### 17. Duplicate Expiration Notifications for Consumer Groups
- **Location:** `Queue.calculateExpiredCountsPerConsumer()`
- **Bug:** Iterates over all consumers. 5 consumers in a group get 5 identical missed item notifications instead of 1.
- **Remedy:** Send notifications per-group, not per-consumer.

### 18. False Sharing & Memory Alignment
- **Bug:** Highly-contended structs (`Queue`, `Consumer`, `ChunkNode`) lack cache line padding. E.g., `closed.Load()` sits next to `mutex`, causing cache invalidation on every lock state change.
- **Remedy:** Add `_ [64]byte` (or `cpu.CacheLinePad`) between `sync.Mutex`/`RWMutex` fields and `atomic` counters.

### 19. Negative TTL Configuration Logic Flaw
- **Bug:** Negative TTLs cause `time.Since(qd.Created) > ttl` to instantly evaluate to true. Expiration worker aggressively wipes the queue.
- **Remedy:** Validate `TTL >= 0`.

### 20. Ghost Goroutine Leak on Cancelled Contexts
- **Location:** `CloseWithContext(ctx)`
- **Bug:** If `ctx.Done()` fires first, the method returns `ctx.Err()`, but the closure running `q.Close()` continues orphaned.
- **Remedy:** Standardize cancellation handling.

### 21. RCU Architectural Misalignment (ADR 0006)
- **Bug:** Claims RCU prevents expiration blocking, but `expirationWorker` runs inside `Queue.mutex.Lock()`, negating throughput benefits.

---

## 🟢 Low Severity (Best Practices)
- **Unbounded Global Pool Growth:** `chunkNodePool` (`sync.Pool`) is global. A spike in one queue allocates thousands of 8KB `ChunkNode` structures. While GC clears it, it circumvents `MaxMemory` guarantees across the application.
- **Massive GC Pressure:** `NewQueueData` performs heap allocation, `uuid.New().String()` performs string allocation. Use a `sync.Pool` for `QueueData`.
- **Memory Tracker Cache Contention:** Replace the `map` + `sync.RWMutex` with `sync.Map` in `MemoryTracker`.
- **Contradictory Comments:** Comments mention lock-free access to avoid deadlocks, followed immediately by `consumer.mutex.Lock()`.