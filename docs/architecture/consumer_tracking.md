# Consumer Position Tracking

Each consumer maintains its own cursor into the shared `ChunkedList`, allowing for independent reading speeds.

```mermaid
graph TD
    subgraph Shared Data
        List[ChunkedList]
        C1[Chunk 1]
        C2[Chunk 2]
        C3[Chunk 3]
        
        List --> C1
        C1 --> C2
        C2 --> C3
    end

    subgraph Consumers
        ConsumerA[Consumer A]
        ConsumerB[Consumer B]
    end

    ConsumerA -- "chunkElement" --> C1
    ConsumerA -- "indexInChunk: 500" --> C1

    ConsumerB -- "chunkElement" --> C2
    ConsumerB -- "indexInChunk: 10" --> C2
```

## Mechanism

1.  **Independent Cursors**: Every `Consumer` struct stores:
    *   `chunkElement`: A pointer to the `list.Element` holding the current `ChunkNode`.
    *   `indexInChunk`: An integer index (0-999) within that chunk.
2.  **Lock-Free(ish) Reads**: Consumers track their position using their own mutex (`Consumer.mutex`), only acquiring the global `Queue.mutex` (RLock) to read the actual data pointer.
3.  **Compaction Safety**: When expired items are removed from the head of the list, all consumer positions are automatically adjusted to ensure they don't point to invalid or shifted data.
