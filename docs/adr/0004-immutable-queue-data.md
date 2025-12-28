# 4. Immutable QueueData

Date: 2025-12-28

## Status

Accepted

## Context

Originally, `QueueData` contained a slice of `Events` to track history (enqueues/dequeues). Modifying this slice when consumers read data introduced race conditions. Protecting individual data items with mutexes would add massive overhead (one lock per item).

## Decision

`QueueData` is **Immutable** after creation.

*   Once an item is enqueued, its fields (`Payload`, `ID`, `Created`, `EnqueueEvent`) never change.
*   Dequeue tracking is moved to the **Consumer**, which maintains its own local history (`DequeueRecord`).
*   There is no shared mutable state for a data item once it enters the queue.

## Consequences

*   **Positive**:
    *   **Thread Safety**: Reading `QueueData` requires no locks once the pointer is obtained.
    *   **Performance**: Zero lock contention on data access.
    *   **Memory Accuracy**: The size of `QueueData` is constant, making memory tracking precise and simple.

*   **Negative**:
    *   **Loss of Centralized History**: We cannot ask "Who read item X?" by inspecting item X. We must query all consumers or aggregate logs. This is an acceptable trade-off for performance and safety.
