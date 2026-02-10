# Performance & Stability Improvement Plan

**Date**: February 10, 2026
**Status**: In Progress
**Goal**: Address critical memory leaks and performance bottlenecks to reach true production readiness.

---

## 🚨 Phase 1: Fix Critical Memory Leak (Priority 0)

**Problem**: `Consumer.dequeueHistory` grows unbounded. A consumer reading 1M items will permanently store 1M `DequeueRecord` structs, eventually causing OOM.
**Impact**: Application crash on long-running consumers.

### Execution Steps
1.  **Configuration**: Add `MaxConsumerHistory` to `QueueConfig` (default: 1000).
2.  **Implementation**:
    - Modify `Consumer` struct to treat `dequeueHistory` as a circular buffer or limit its append operation.
    - When limit reached: Drop oldest record or strictly cap size.
    - *Decision*: Use a simple slice truncation for simplicity first (keep last N), or a ring buffer if high-freq allocs are a concern. Given `DequeueRecord` is small, a ring buffer is better for GC.
3.  **Verification**:
    - Create `TestConsumerHistoryLimit` in `tests/consumer_info_test.go`.
    - Read N > Limit items.
    - Assert `len(history) == Limit`.

---

## ⚡ Phase 2: Optimize Hot-Path Performance (Priority 1)

**Problem**: `MemoryTracker.EstimateQueueDataSize` uses `reflect.ValueOf` for every single item enqueued. Reflection is slow.
**Impact**: reduced throughput for high-volume producers.

### Execution Steps
1.  **Interface Definition**:
    ```go
    // Sizeable allow structs to report their own size, bypassing reflection.
    type Sizeable interface {
        Size() int
    }
    ```
2.  **MemoryTracker Update**:
    - Check `if s, ok := payload.(Sizeable); ok { return s.Size() }` *before* reflection.
3.  **Benchmarks**:
    - Add `BenchmarkEnqueue_Sizeable` vs `BenchmarkEnqueue_Reflection`.
    - Expect >2x improvement for complex structs.

---

## 🧪 Phase 3: Robust Testing (Priority 2)

**Problem**: Weak assertions on memory usage and closed queue errors.

### Execution Steps
1.  **Exact Memory Accounting**:
    - Create `tests/memory_accuracy_test.go`.
    - Enqueue known payload (e.g., 1KB byte slice).
    - Assert `q.GetMemoryUsage()` increases by exactly `BaseQueueDataSize + 1024`.
2.  **Closed Queue Assertions**:
    - Update `tests/lifecycle_test.go`.
    - Use `errors.As(err, &target)` to verify `*QueueClosedError` specifically.

---

## 🧹 Phase 4: Configuration & Cleanup (Priority 3)

**Problem**: `ExpirationCheckInterval` is a global variable, making it hard to test or tune per-instance.

### Execution Steps
1.  **Refactor**: Move `ExpirationCheckInterval` into `QueueConfig`.
2.  **Update**: `NewQueueWithConfig` to accept this value.
3.  **Migration**: Ensure `NewQueue` uses the default (30s) to maintain backward compatibility.

---

## Success Criteria
- [ ] No unbounded memory growth in consumers (verified by test).
- [ ] Enqueue throughput improved by >20% for `Sizeable` payloads (verified by benchmark).
- [ ] All tests pass with `-race`.
- [ ] Memory usage tracking is mathematically proven correct.
