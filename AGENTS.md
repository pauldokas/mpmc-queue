# Agent Development Guide for mpmc-queue

**Stack**: Go 1.25+ | **Module**: `mpmc-queue` | **Deps**: `github.com/google/uuid`

## 🚨 Critical Mandates
1.  **Concurrency Safety**: ALWAYS acquire `Queue.mutex` BEFORE `Consumer.mutex`. NEVER hold `Consumer.mutex` while acquiring `Queue.mutex`.
2.  **Race Detection**: ALWAYS run tests with `-race`. This is non-negotiable for this codebase.
3.  **Immutability**: `QueueData` is immutable. NEVER modify it after creation.
4.  **Locking**: Use `RLock` for reads, `Lock` for writes. Defer `Unlock` immediately after locking.
5.  **Memory**: Pre-validate data size with `MemoryTracker.CanAddData()` before adding to the queue.

## 📂 Project Structure
-   `queue/`: Core logic (`Queue`, `Consumer`, `ChunkedList`, `MemoryTracker`).
-   `tests/`: Comprehensive test suite (unit, race, integration, stress).
-   `examples/`: Usage examples for end-users.
-   `docs/`: Architecture and design documentation.

## 🛠 Development Workflow

### Build & Test Commands
Use `make` for standard operations.

```bash
# Build
make build          # Builds with -race flag

# Testing
make test           # Run unit tests (fast, skips integration)
make test-integration # Run stress/long-running tests (-tags=integration)
make test-all       # Run EVERYTHING (unit + integration + race)

# Single Test Execution (Best for debugging)
# Format: go test -v -race -run <TestName> <Path>
go test -v -race -run TestQueue_Enqueue ./tests/queue_test.go
```

### Linting & Formatting
We use `golangci-lint` with strict settings.
```bash
make lint           # Run all linters
make fmt            # Format code (go fmt)
```
**Style Note**: `goimports` is enforced with local prefix `mpmc-queue`.

## 📏 Code Style & Conventions

### Imports
Group imports: StdLib, External, Internal.
```go
import (
    "context"
    "sync"
    "time"

    "github.com/google/uuid"

    "mpmc-queue/queue"
)
```

### Naming
-   **Exported**: `PascalCase` (e.g., `Queue`, `Enqueue`). Must have Godoc comments explaining *behavior* and *blocking semantics*.
-   **Private**: `camelCase` (e.g., `expirationWorker`, `cleanupExpiredItems`).
-   **Interfaces**: Name with `-er` suffix if simple (e.g., `Reader`), or descriptive (e.g., `MemoryTracker`).

### Error Handling
-   **Custom Errors**: Use specific types like `MemoryLimitError` for logic branching.
-   **Wrapping**: Always wrap errors with context: `fmt.Errorf("initializing consumer: %w", err)`.
-   **Checking**: Use `errors.As` to check for specific error types.

## 🔄 Concurrency Patterns

### Lock Ordering (Deadlock Prevention)
**Golden Rule**: `Queue.mutex` > `Consumer.mutex`.

**Snapshot Pattern**: To avoid holding locks too long or incorrectly:
1.  Lock Consumer.
2.  Copy necessary state (e.g., current index).
3.  Unlock Consumer.
4.  Lock Queue.
5.  Perform operation.

```go
// ✅ Correct Pattern
c.mutex.Lock()
pos := c.chunkElement
c.mutex.Unlock()

c.queue.mutex.RLock()
defer c.queue.mutex.RUnlock()
// ... safely access queue using pos ...
```

### Blocking vs Non-Blocking
-   **Blocking** (`Enqueue`, `Read`): Use `select` with `stopChan` and `context.Done()`. Used in production.
-   **Non-Blocking** (`TryEnqueue`, `TryRead`): Return immediately with success/fail/nil. **MANDATORY for tests** to prevent test timeouts.

## 🧪 Testing Standards

1.  **Isolation**: Create a new `Queue` for every test.
2.  **Cleanup**: Always `defer q.Close()`.
3.  **Parallelism**: Use `t.Parallel()` where appropriate, but be careful with shared resources (though `Queue` instances should be isolated).
4.  **WaitGroups**: Use `sync.WaitGroup` for testing concurrent producers/consumers.
5.  **Atomics**: Use `atomic.Int64` for counters shared across goroutines in tests.

### Example Test Template
```go
func TestConcurrency_Safe(t *testing.T) {
    q := queue.NewQueue("test-queue")
    defer q.Close()

    var wg sync.WaitGroup
    wg.Add(2)

    // Producer
    go func() {
        defer wg.Done()
        if err := q.TryEnqueue("data"); err != nil {
            t.Errorf("Enqueue failed: %v", err)
        }
    }()

    // Consumer
    go func() {
        defer wg.Done()
        c := q.AddConsumer()
        // Spin/Wait for data (simplified)
        for i := 0; i < 10; i++ {
            if val := c.TryRead(); val != nil {
                return
            }
            time.Sleep(time.Millisecond)
        }
        t.Error("Did not receive data")
    }()

    wg.Wait()
}
```

## ⚠️ Common Pitfalls
-   **Nil Checks**: `chunkElement` can be nil if the queue is empty or consumer is new. Always check before dereferencing.
-   **Context Propagation**: Always respect `ctx.Done()` in blocking operations.
-   **Channel Blocking**: Never send to `notify` channels without a `select` with `default:` case, or you risk deadlocking the system if channels are full.
