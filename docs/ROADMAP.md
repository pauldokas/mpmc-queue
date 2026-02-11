# Roadmap - Future Improvements

This document outlines the planned future improvements and enhancements for the `mpmc-queue` project.

## 🚀 Planned Enhancements

### Generics (`Queue[T]`)
Refactor the current `any` based implementation to use Go Generics (`T`). This will provide:
- **Type Safety**: Compile-time type checking for queue payloads.
- **Performance**: Reduced boxing/unboxing overhead and better compiler optimizations.
- **Developer Experience**: Improved IDE autocompletion and clearer API usage.

### Advanced Flow Control
Implement more sophisticated flow control policies to handle high-load scenarios:
- **DiscardOldest Policy**: Add support for ring buffer behavior where the oldest items are automatically discarded when the queue reaches its memory limit. This is particularly useful for telemetry and logging use cases where the most recent data is more valuable than old data.

### Persistence (WAL)
Add a disk-backed Write-Ahead Log (WAL) to provide durability:
- **Durability**: Ensure that data is not lost in case of a process crash or restart.
- **Restart Survival**: Allow the queue to recover its state from the disk upon startup.
- **Configurable Storage**: Support for different storage backends or file-based logging.

### Observability (OpenTelemetry)
Integrate native OpenTelemetry support for better production monitoring:
- **Tracing**: Add spans for enqueue, dequeue, and expiration operations to track message lifecycle.
- **Metrics**: Export standard OTel metrics for queue depth, latency, and throughput.
- **Context Propagation**: Support for carrying trace context through the queue.

## 🛠 Performance Optimizations

### Lock Optimization
Optimize the `cleanupExpiredItems` process to reduce global lock contention:
- **Fine-grained Locking**: Move away from global RWMutex during expiration where possible.
- **Background Processing**: Further decouple the expiration worker from the main producer/consumer paths to minimize impact on throughput.

### Data Structure Improvements
Replace the current `container/list` based implementation with a custom slice-based or ring-buffer-based implementation:
- **Reduced GC Pressure**: Slices are more GC-friendly than linked lists of many small objects.
- **Cache Locality**: Better memory layout for faster traversal.
- **Memory Efficiency**: Reduced pointer overhead compared to doubly-linked lists.
