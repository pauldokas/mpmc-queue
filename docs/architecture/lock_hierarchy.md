# Lock Hierarchy

To prevent deadlocks, the system strictly adheres to a locking order.

```mermaid
graph TD
    Q[Queue.mutex (RWMutex)]
    CM[ConsumerManager.mutex (RWMutex)]
    C[Consumer.mutex (Mutex)]

    Q -->|Acquire First| CM
    Q -->|Acquire First| C
    CM -->|Acquire First| C

    style Q fill:#f9f,stroke:#333,stroke-width:2px
    style C fill:#ccf,stroke:#333,stroke-width:2px
```

## Rules

1.  **Top Level**: `Queue.mutex` protects the `ChunkedList` structure, `MemoryTracker`, and global state.
2.  **Leaf Level**: `Consumer.mutex` protects the individual consumer's cursor (`chunkElement`, `indexInChunk`) and history.
3.  **The Golden Rule**: **NEVER** wait for `Queue.mutex` while holding `Consumer.mutex`.
    *   *Correct*: Lock Queue -> Lock Consumer
    *   *Correct*: Lock Consumer -> Unlock Consumer -> Lock Queue
    *   *Deadlock*: Lock Consumer -> Lock Queue

## Common Patterns

### Snapshotting (Safe)
When a consumer needs to read data:
1.  Lock `Consumer.mutex`.
2.  Copy current position (`chunkElement`, `index`).
3.  Unlock `Consumer.mutex`.
4.  Lock `Queue.mutex` (RLock).
5.  Read data using the copied position.
