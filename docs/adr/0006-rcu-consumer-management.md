# 6. RCU for Consumer Management

Date: 2025-12-28

## Status

Accepted

## Context

The `Queue` maintains a list of active consumers. The background expiration worker needs to iterate over ALL consumers periodically to check for expired items. If we use a standard `Mutex` to protect the consumer map, the expiration worker (O(N)) could block Enqueue/Dequeue operations (which might need `RLock` on the same map) for significant time.

## Decision

We use a **Read-Copy-Update (RCU)** style pattern using `atomic.Value` for the consumer list.

*   The "source of truth" is still a map protected by a mutex, used for Add/Remove.
*   However, we also maintain a `[]*Consumer` slice stored in an `atomic.Value`.
*   On Add/Remove: We update the map, regenerate the slice, and atomically store the new slice.
*   On Read (Expiration Worker): We load the slice atomically. This is wait-free.

## Consequences

*   **Positive**:
    *   **Zero Contention**: The expiration worker (read-only) never blocks producers or consumers adding/removing themselves.
    *   **Performance**: `GetAllConsumers()` becomes extremely cheap O(1) atomic load.

*   **Negative**:
    *   **Memory**: We duplicate the consumer pointers in a slice. Given the low number of consumers (typically <1000), this overhead is negligible.
    *   **Update Cost**: Adding/removing a consumer involves copying the slice. This is acceptable as these are infrequent operations compared to Enqueue/Read.
