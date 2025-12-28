# 3. Lock Hierarchy for Concurrency

Date: 2025-12-28

## Status

Accepted

## Context

The system involves multiple entities (`Queue`, `Consumer`, `ConsumerGroup`, `MemoryTracker`) that need to be accessed concurrently. Without a strict locking strategy, deadlocks are inevitable, especially given the interdependence between Queue (global state) and Consumer (local state).

## Decision

We enforce a strict **Top-Down Lock Hierarchy**:

1.  **Level 1 (Top)**: `Queue.mutex` (RWMutex)
    *   Protects: ChunkedList structure, MemoryTracker, adding/removing consumers.
2.  **Level 2**: `ConsumerGroup.mutex` (Mutex)
    *   Protects: Shared group state.
3.  **Level 3 (Bottom)**: `Consumer.mutex` (Mutex)
    *   Protects: Individual consumer cursor (`chunkElement`, `indexInChunk`).

**Rules**:
*   A thread holding a Level 1 lock can acquire Level 2 or 3 locks.
*   A thread holding a Level 2 lock can acquire Level 3 locks.
*   **NEVER** acquire a higher-level lock while holding a lower-level lock.
    *   *Example*: Do not hold `Consumer.mutex` and try to lock `Queue.mutex`.

**Snapshot Pattern**:
To perform operations requiring global state based on local state (e.g., `Read()`), we use the "Snapshot" pattern:
1.  Lock Consumer.
2.  Copy necessary state (position).
3.  Unlock Consumer.
4.  Lock Queue.
5.  Read data using copied state.
6.  (Optional) Re-lock Consumer to update state if valid.

## Consequences

*   **Positive**:
    *   **Deadlock Freedom**: Strict ordering guarantees no cycles in the resource allocation graph.
    *   **Performance**: RWMutex on Queue allows concurrent reads. Fine-grained consumer locks reduce contention.

*   **Negative**:
    *   **Complexity**: Code is more verbose due to snapshotting and explicit locking steps.
    *   **Fragility**: Accidental violation of the order (e.g., in a new feature) can introduce deadlocks. Requires rigorous code review and race detection.
