# 2. Use Chunked List for Queue Storage

Date: 2025-12-28

## Status

Accepted

## Context

We need a data structure for the underlying queue storage that supports:
1.  Dynamic growth (unbounded until memory limit).
2.  Efficient memory allocation/deallocation.
3.  Fast enqueue/dequeue operations.
4.  Independent read positions for multiple consumers.
5.  Efficient expiration of old items.

Standard Go slices or arrays require resizing (copying) or have fixed capacity. A simple linked list has high overhead per item (pointer + value). A ring buffer is difficult to resize and manage with multiple independent consumers.

## Decision

We will use a **Chunked Linked List**.
*   The queue is a doubly-linked list (`container/list`) of "Chunks".
*   Each `ChunkNode` contains a fixed-size array of `QueueData` pointers (e.g., 1000 items).
*   Enqueuing adds to the last chunk, creating a new one if full.
*   Consumers track their position as `{ChunkElement, IndexInChunk}`.

## Consequences

*   **Positive**:
    *   **Amortized Allocation**: Allocating one chunk is cheaper than allocating 1000 individual list nodes.
    *   **Cache Locality**: Accessing items sequentially within a chunk is cache-friendly.
    *   **Efficient Expiration**: We can remove entire chunks from the head of the list when all items in them are expired.
    *   **Stable Pointers**: Unlike a resizing slice, chunk addresses don't change, simplifying consumer position tracking.

*   **Negative**:
    *   **Complexity**: More complex implementation than a slice or simple list (need to handle chunk boundaries).
    *   **Memory Overhead**: Unused slots in the tail chunk consume some memory (though minimal with pointer arrays).
