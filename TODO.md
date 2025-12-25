# TODO - mpmc-queue Improvements

This document tracks improvements, enhancements, and issues for the mpmc-queue project.

## Priority 0 - Critical Issues

### Fix Race Condition in AddEvent
- **Status**: ⚠️ Not Started
- **Problem**: Tests crash with `SIGSEGV` during race detection
- **Root Cause**: `consumer.go:89` calls `data.AddEvent()` without synchronization, potentially growing a slice concurrently
- **Impact**: Production-blocking bug, potential data corruption
- **Solutions**:
  - Option 1: Make Events append thread-safe with mutex in QueueData
  - Option 2: Pre-allocate event capacity and document as unsafe after Enqueue
  - Option 3: Make QueueData.Events immutable after enqueue (recommended)
- **Files**: `queue/data.go`, `queue/consumer.go`

### Fix Memory Tracking Inaccuracy
- **Status**: ⚠️ Not Started
- **Problem**: Memory estimation doesn't track dequeue event additions to existing `QueueData.Events` slice
- **Impact**: Actual memory usage exceeds tracked memory, potentially breaking the 1MB limit
- **Solution**: Update memory tracking when events are added, or freeze `QueueData` after creation
- **Files**: `queue/memory.go`, `queue/data.go`

## Priority 1 - High Priority

### Use Modern Go Idioms
- **Status**: ⚠️ Not Started
- **Task**: Replace `interface{}` with `any` (Go 1.18+)
- **Impact**: Improves code readability, follows modern Go conventions
- **Files**: 
  - `queue/data.go` - `Payload interface{}`
  - `queue/queue.go` - `Enqueue(payload interface{})`
  - `queue/queue.go` - `EnqueueBatch(payloads []interface{})`
  - `tests/*.go` - All test files with `interface{}` usage

### Fix go.mod Dependencies
- **Status**: ⚠️ Not Started
- **Task**: Make `github.com/google/uuid` a direct dependency
- **Command**: `go mod tidy` and remove `// indirect` comment
- **Files**: `go.mod`

### Add Context Support
- **Status**: ⚠️ Not Started
- **Task**: Add context-aware operations throughout API
- **New Functions**:
  ```go
  func (q *Queue) EnqueueWithContext(ctx context.Context, payload any) error
  func (c *Consumer) ReadWithContext(ctx context.Context) (*QueueData, error)
  func (c *Consumer) ReadBatchWithContext(ctx context.Context, limit int) ([]*QueueData, error)
  ```
- **Files**: `queue/queue.go`, `queue/consumer.go`

### Add Test Coverage Reporting
- **Status**: ⚠️ Not Started
- **Task**: Set up coverage reporting and tracking
- **Commands**:
  ```bash
  go test ./... -coverprofile=coverage.out
  go tool cover -html=coverage.out -o coverage.html
  go tool cover -func=coverage.out | grep total
  ```
- **Update**: Add coverage commands to `AGENTS.md`

### Add Missing Test Cases
- **Status**: ⚠️ Not Started
- **Tests Needed**:
  - Test consumer removal during active reads
  - Test memory tracking accuracy with complex nested structures
  - Test graceful degradation when memory limit is reached during batch operations
  - Test expiration with nanosecond precision edge cases
  - Fuzz testing for memory estimation
- **Files**: `tests/queue_test.go`, new test files as needed

## Priority 2 - Medium Priority

### Make Memory Limit Configurable
- **Status**: ⚠️ Not Started
- **Current**: Memory limit hardcoded to 1MB
- **Task**: Make configurable via QueueConfig
- **New API**:
  ```go
  func NewQueueWithConfig(name string, config QueueConfig) *Queue
  
  type QueueConfig struct {
      TTL         time.Duration
      MaxMemory   int64  // Currently hardcoded to 1MB
      ChunkSize   int    // Currently hardcoded to 1000
  }
  ```
- **Files**: `queue/queue.go`, `queue/memory.go`

### Optimize Memory Estimation with Caching
- **Status**: ⚠️ Not Started
- **Current**: Uses reflection recursively for every item (expensive)
- **Improvement**: Cache size estimates for common payload types
- **Implementation**:
  ```go
  type MemoryTracker struct {
      totalMemory  int64
      maxMemory    int64
      sizeCache    map[reflect.Type]int64  // Cache type sizes
      mu           sync.RWMutex
  }
  ```
- **Files**: `queue/memory.go`

### Optimize Batch Memory Validation
- **Status**: ⚠️ Not Started
- **Current**: `EnqueueBatch` validates each item sequentially
- **Improvement**: Calculate total size once before validation
- **Files**: `queue/queue.go`

### Reduce Lock Contention
- **Status**: ⚠️ Not Started
- **Tasks**:
  - Consider lock-free alternatives for consumer position tracking
  - Use atomic operations for statistics counters
  - Implement read-copy-update (RCU) for consumer list
- **Files**: `queue/consumer.go`, `queue/queue.go`

### Add CI/CD Pipeline
- **Status**: ⚠️ Not Started
- **Task**: Create GitHub Actions workflow
- **Files**: `.github/workflows/test.yml` (new)
- **Workflow**:
  ```yaml
  name: Test
  on: [push, pull_request]
  jobs:
    test:
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v3
        - uses: actions/setup-go@v4
          with:
            go-version: '1.25'
        - run: go test -v -race -coverprofile=coverage.out ./...
        - run: go vet ./...
  ```

### Add Makefile
- **Status**: ⚠️ Not Started
- **File**: `Makefile` (new)
- **Targets**: test, bench, lint, coverage, build

### Add golangci-lint Configuration
- **Status**: ⚠️ Not Started
- **File**: `.golangci.yml` (new)
- **Linters**: govet, errcheck, staticcheck, gosimple, ineffassign, unused, typecheck

### Add Peek Operations
- **Status**: ⚠️ Not Started
- **Task**: Add non-destructive read operations
- **New Functions**:
  ```go
  func (c *Consumer) Peek() *QueueData
  func (c *Consumer) PeekBatch(limit int) []*QueueData
  ```
- **Files**: `queue/consumer.go`

### Add Consumer Seek/Reset
- **Status**: ⚠️ Not Started
- **Task**: Allow consumers to reset to beginning or seek to position
- **New Functions**:
  ```go
  func (c *Consumer) Reset() error
  func (c *Consumer) Seek(position int64) error
  func (c *Consumer) GetPosition() int64
  ```
- **Files**: `queue/consumer.go`

### Add Metrics/Observability
- **Status**: ⚠️ Not Started
- **Task**: Add prometheus-style metrics
- **New API**:
  ```go
  func (q *Queue) GetMetrics() QueueMetrics
  
  type QueueMetrics struct {
      EnqueueCount       int64
      DequeueCount       int64
      MemoryLimitHits    int64
      ExpirationRuns     int64
      ItemsExpired       int64
      AveragePayloadSize int64
  }
  ```
- **Files**: `queue/queue.go`, `queue/metrics.go` (new)

### Separate Unit and Integration Tests
- **Status**: ⚠️ Not Started
- **Task**: Add build tags to separate test types
- **Tags**: `//go:build unit` and `//go:build integration`
- **Files**: All test files in `tests/`

## Priority 3 - Low Priority / Future Enhancements

### Add Godoc Examples
- **Status**: ⚠️ Not Started
- **Task**: Add runnable examples to godoc
- **Files**: `queue/example_test.go` (new)

### Add Architecture Diagrams
- **Status**: ⚠️ Not Started
- **Task**: Create visual diagrams showing:
  - Memory layout of chunked list
  - Consumer position tracking
  - Expiration workflow
  - Lock hierarchy
- **Files**: `docs/architecture/` (new directory)

### Document Limitations More Clearly
- **Status**: ⚠️ Not Started
- **Task**: Add limitations section to README.md
- **Content**:
  - Maximum 1MB total memory (not configurable in v1.0)
  - Memory estimation is approximate (±10-20% typical)
  - TTL granularity: 30 seconds (expiration check interval)
  - QueueData is not safe to modify after enqueue
  - Events slice grows unbounded with many consumers
- **Files**: `README.md`

### Add More Specific Error Types
- **Status**: ⚠️ Not Started
- **Task**: Create specific error types for better error handling
- **New Types**:
  ```go
  type QueueClosedError struct{}
  type ConsumerNotFoundError struct{ ID string }
  type InvalidPositionError struct{ Position int64 }
  ```
- **Files**: `queue/errors.go` (new)

### Add Error Wrapping
- **Status**: ⚠️ Not Started
- **Task**: Use `fmt.Errorf` with `%w` for error wrapping
- **Files**: All queue package files

### Add Priority Queue Support
- **Status**: ⚠️ Not Started
- **Task**: Add priority levels for queue items
- **New API**:
  ```go
  type Priority int
  
  const (
      PriorityLow Priority = iota
      PriorityNormal
      PriorityHigh
  )
  
  func (q *Queue) EnqueueWithPriority(payload any, priority Priority) error
  ```
- **Files**: `queue/queue.go`, `queue/data.go`

### Add Filtering/Selection
- **Status**: ⚠️ Not Started
- **Task**: Allow consumers to filter items
- **New Functions**:
  ```go
  func (c *Consumer) ReadWhere(predicate func(*QueueData) bool) *QueueData
  ```
- **Files**: `queue/consumer.go`

### Add Persistence Option
- **Status**: ⚠️ Not Started
- **Task**: Add optional disk persistence
- **New Functions**:
  ```go
  func NewPersistentQueue(name, dataDir string) *Queue
  func (q *Queue) Snapshot() error
  func (q *Queue) Restore(snapshotPath string) error
  ```
- **Files**: `queue/persistence.go` (new)

### Add Project Maintenance Files
- **Status**: ⚠️ Not Started
- **Files to Add**:
  - `CHANGELOG.md` - Following Keep a Changelog format
  - `CONTRIBUTING.md` - Development guidelines
  - `CODE_OF_CONDUCT.md` - Community guidelines
  - `LICENSE` - Currently mentioned but missing
- **Task**: Add semantic versioning tags

## Completed Items

_No items completed yet_

---

## Legend

- ⚠️ Not Started
- 🚧 In Progress
- ✅ Completed
- ❌ Cancelled

## Notes

- Items are organized by priority (P0-P3)
- P0 issues are blocking and should be addressed before any production use
- P1 items significantly improve code quality and usability
- P2 items add useful features and improvements
- P3 items are nice-to-have enhancements

Last Updated: 2025-12-25
