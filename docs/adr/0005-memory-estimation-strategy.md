# 5. Reflection-Based Memory Estimation

Date: 2025-12-28

## Status

Accepted

## Context

We need to enforce a hard memory limit (default 1MB) to prevent the queue from exhausting system memory. Since Go handles memory management automatically, we don't know the exact size of arbitrary `any` payloads.

## Decision

We use **Reflection (`reflect` package)** to traverse payloads and estimate their size.

*   **Recursive Walk**: We walk through structs, slices, arrays, and maps.
*   **Size Caching**: We cache the calculated size for fixed-size types (primitives, structs of primitives) to avoid repeated reflection overhead.
*   **Estimation**: This is an *estimation*, not an exact measurement of heap usage (allocator overhead, alignment padding are approximated).

## Consequences

*   **Positive**:
    *   **Safety**: Prevents OOM crashes by rejecting large items before enqueue.
    *   **Flexibility**: Works with any user-provided type.

*   **Negative**:
    *   **Performance Overhead**: Reflection is slow. Heavily nested structs or large maps incur CPU cost on Enqueue.
    *   **Accuracy**: Estimates may drift from actual GC heap usage.

*   **Mitigation**: We added `sizeCache` to `MemoryTracker` to mitigate the performance impact for common types.
