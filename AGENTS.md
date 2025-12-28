# Agent Guide: mpmc-queue

**Context**: High-performance, concurrent, in-memory MPMC queue in Go 1.25+.
**Dependencies**: `github.com/google/uuid` (direct).

## 🚨 Critical Mandates

1.  **Concurrency Safety**:
    *   **Lock Order**: ALWAYS acquire `Queue.mutex` BEFORE `Consumer.mutex` if both are needed. Reverse order causes deadlocks.
    *   **Snapshots**: To avoid holding locks, copy state (e.g., `consumer.chunkElement`) under a short lock, then use the copy.
    *   **Defer**: Immediately `defer mutex.Unlock()` (or `RUnlock()`) after locking.

2.  **Race Detection**:
    *   **MANDATORY**: Always run tests with the `-race` flag. No exceptions.
    *   **Atomics**: Use `atomic.Int64`/`atomic.Bool` for counters/flags accessed without mutexes.

3.  **Memory Management**:
    *   **Immutability**: `QueueData` is immutable once created.
    *   **Pre-validation**: Check memory limits (`MemoryTracker`) before adding data.
    *   **Unsafe**: `unsafe.Sizeof` is used for accurate memory tracking; modify structs with caution.

4.  **Blocking Semantics**:
    *   **Production**: Use blocking methods (`Enqueue`, `Read`) with `context.Context` cancellation.
    *   **Testing**: Use non-blocking methods (`TryEnqueue`, `TryRead`) to prevent test timeouts/hangs.

## 🛠 Build & Test Commands

**Standard Workflow** (Makefile):
```bash
make build             # Build with -race
make test              # Run unit tests (fast)
make test-integration  # Run integration/stress tests
make test-all          # Run ALL tests
make lint              # Run golangci-lint
make fmt               # Format code
```

**Specific Test Scenarios** (Debugging):
```bash
# Run a SINGLE test function (Crucial for debugging)
go test -v -race -run TestQueue_Enqueue ./tests/

# Run a specific test file
go test -v -race ./tests/queue_test.go

# Run tests matching a pattern
go test -v -race -run "TestConsumer_.*" ./tests/
```

**Linting Configuration**:
*   **Tool**: `golangci-lint` (govet, errcheck, staticcheck, gosimple, ineffassign, unused, typecheck).
*   **Formatting**: `goimports` with `-local mpmc-queue` is enforced.

## 📏 Code Style & Conventions

### Imports
Group imports into three blocks separated by newlines:
1.  Standard Library
2.  External Dependencies (`github.com/...`)
3.  Internal Project Imports (`mpmc-queue/...`)

```go
import (
    "context"
    "sync"

    "github.com/google/uuid"

    "mpmc-queue/queue"
)
```

### Naming Conventions
*   **Exported**: `PascalCase`. Must have Godoc comments explaining *behavior* and *blocking semantics*.
*   **Private**: `camelCase`.
*   **Getters**: Use `Get` prefix (e.g., `GetName`, `GetStats`) - project-specific convention.
*   **Interfaces**: `-er` suffix (e.g., `Reader`, `MemoryTracker`).

### Error Handling
*   **Typed Errors**: Use specific types for logic branching (e.g., `&MemoryLimitError{}`).
*   **Wrapping**: Wrap errors to provide context: `fmt.Errorf("initializing consumer: %w", err)`.
*   **Checks**: Use `errors.As` to check for specific error types.

### Types & Generics
*   **Generics**: Use `any` for payload data instead of `interface{}`.
*   **Structs**: Keep structs aligned for memory efficiency where possible.

## 🔄 Concurrency Patterns

### The "Snapshot" Pattern
To avoid holding locks across complex operations or calling into other locked components:
1.  Lock the component (e.g., Consumer).
2.  Copy the necessary state (e.g., current index/pointer).
3.  Unlock the component.
4.  Lock the parent/dependency (e.g., Queue) using the copied state.

### Notification Channels
*   **Non-Blocking Sends**: Never block on notification channels. Use `select` with `default`.
```go
select {
case c.notifyChan <- struct{}{}:
default: // Channel full, receiver already notified
}
```

## 🧪 Testing Standards

1.  **Isolation**: Create a `NewQueue` for EVERY test case. Do not share queues.
2.  **Cleanup**: Always `defer q.Close()` immediately after creation.
3.  **Parallelism**: Use `t.Parallel()` where appropriate.
4.  **WaitGroups**: Use `sync.WaitGroup` to synchronize concurrent producers/consumers.
5.  **Assertions**: Fail fast with `t.Fatalf` for setup/critical errors; use `t.Errorf` for logic checks.

### Example Test Template
```go
func TestConcurrency_Safe(t *testing.T) {
    q := queue.NewQueue("test-queue")
    defer q.Close()

    var wg sync.WaitGroup
    wg.Add(1)

    go func() {
        defer wg.Done()
        // Use TryEnqueue in tests to avoid deadlocks/hangs
        if err := q.TryEnqueue("data"); err != nil {
            t.Errorf("Enqueue failed: %v", err)
        }
    }()
    
    wg.Wait()
}
```

## ⚠️ Common Pitfalls

*   **Nil Pointers**: `chunkElement` can be nil if the queue is empty. Always check before dereferencing.
*   **Context**: Respect `ctx.Done()` in all blocking loops.
*   **Memory Leaks**: Ensure `Close()` is called to stop background workers (`expirationWorker`).
*   **Deadlocks**: Never acquire `Consumer` lock *inside* a `Queue` lock critical section if the `Queue` lock was acquired *after* a `Consumer` lock elsewhere.
