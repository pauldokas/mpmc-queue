# Agent Development Guide for mpmc-queue

**Stack**: Go 1.25.1 | **Module**: `mpmc-queue` | **Deps**: `github.com/google/uuid`

## Critical Mandates
1. **Concurrency Safety**: ALWAYS acquire `Queue.mutex` BEFORE `Consumer.mutex`. NEVER hold `Consumer.mutex` while acquiring `Queue.mutex`.
2. **Testing**: ALWAYS run tests with `-race`. Use `Try*` methods (non-blocking) in tests to prevent hangs.
3. **Immutability**: `QueueData` is immutable. NEVER modify it after creation.
4. **Locking**: Use `RLock` for reads, `Lock` for writes.
5. **Memory**: Pre-validate data size with `MemoryTracker.CanAddData()` before adding.

## Development Commands
```bash
# Common Tasks (via Makefile)
make build          # Build with race detection
make test           # Run unit tests (fast)
make test-integration # Run stress/long-running tests
make test-all       # Run ALL tests
make lint           # Run linter
make fmt            # Format code
make clean          # Clean artifacts

# Manual Testing (if needed)
go test ./tests -v -race -run TestName
```

## Code Style & Conventions

### Imports
Group in order: StdLib, External, Internal.
```go
import (
    "sync"
    "time"

    "github.com/google/uuid"

    "mpmc-queue/queue"
)
```

### Naming & Documentation
- **Exported**: `PascalCase` (e.g., `Queue`, `Enqueue`). MUST have Godoc comment.
- **Private**: `camelCase` (e.g., `expirationWorker`).
- **Constants**: `PascalCase` (e.g., `DefaultTTL`).
- **Tags**: Use JSON tags `json:"field_name"`.

### Error Handling
- Use custom error types (e.g., `MemoryLimitError`).
- Always wrap errors: `fmt.Errorf("context: %w", err)`.
- Check errors with `errors.As` or type assertion.

## Concurrency Patterns

### Lock Ordering (Deadlock Prevention)
**Rule**: Queue lock > Consumer lock.
**Snapshot Pattern**: Read consumer state, release lock, THEN access queue.

```go
// ✅ CORRECT: Snapshot pattern
c.mutex.Lock()
pos := c.chunkElement
c.mutex.Unlock()
// Now safe to acquire queue lock
c.queue.mutex.RLock()
defer c.queue.mutex.RUnlock()
// ... use pos

// ❌ WRONG: Holding consumer lock while acquiring queue lock
c.mutex.Lock()
defer c.mutex.Unlock()
c.queue.mutex.RLock() // DEADLOCK RISK
```

### Blocking vs Non-Blocking
- **Blocking** (`Enqueue`, `Read`): Wait for resource. Use in production logic.
- **Non-Blocking** (`TryEnqueue`, `TryRead`): Return immediately. **MANDATORY for tests** to avoid timeouts.

## Architecture
- **Queue**: Central coordinator (RWMutex). Manages `ChunkedList` and `ConsumerManager`.
- **ChunkedList**: Linked list of fixed-size arrays (1000 items/chunk).
- **Consumer**: Independent reader with its own cursor and mutex.
- **MemoryTracker**: Enforces global 1MB memory limit.
- **Expiration**: Background worker cleans up expired items every 30s.

## Testing Standards
1. **Setup**: Create queue, `defer q.Close()`.
2. **Race Detection**: `go test -race` is non-negotiable.
3. **Parallelism**: Use `sync.WaitGroup` for concurrent consumers.
4. **Atomics**: Use `atomic.AddInt64`/`LoadInt64` for shared counters in tests.

### Example Test
```go
func TestFeature(t *testing.T) {
    q := queue.NewQueue("test")
    defer q.Close()

    // Use TryEnqueue for tests
    if err := q.TryEnqueue("data"); err != nil {
        t.Fatalf("Failed: %v", err)
    }
    
    // Use TryRead for tests
    c := q.AddConsumer()
    if data := c.TryRead(); data == nil {
        t.Fatal("Expected data")
    }
}
```
EOF

