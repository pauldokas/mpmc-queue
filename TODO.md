# TODO - mpmc-queue Improvements

This document tracks improvements, enhancements, and issues for the mpmc-queue project.

## Priority 0 - Critical Issues

### Fix Race Condition in AddEvent
- **Status**: ✅ Completed (commit: 144bf05)
- **Problem**: Tests crash with `SIGSEGV` during race detection
- **Root Cause**: `consumer.go:89` calls `data.AddEvent()` without synchronization, potentially growing a slice concurrently
- **Impact**: Production-blocking bug, potential data corruption
- **Solution Implemented**: Made QueueData.Events immutable after enqueue (Option 3)
  - Changed `Events []QueueEvent` to single `EnqueueEvent QueueEvent`
  - Removed `AddEvent()` method completely
  - Added `DequeueRecord` tracking in each Consumer with `GetDequeueHistory()`
  - All tests pass with race detection enabled
- **Files**: `queue/data.go`, `queue/consumer.go`

### Fix Memory Tracking Inaccuracy
- **Status**: ✅ Completed (commit: 144bf05)
- **Problem**: Memory estimation doesn't track dequeue event additions to existing `QueueData.Events` slice
- **Impact**: Actual memory usage exceeds tracked memory, potentially breaking the 1MB limit
- **Solution Implemented**: QueueData is now immutable, so memory size is fixed after creation
  - Updated `EstimateQueueDataSize()` to calculate single enqueue event
  - Memory tracking is now accurate and doesn't change after enqueue
- **Files**: `queue/memory.go`, `queue/data.go`

## Priority 1 - High Priority

### Use Modern Go Idioms
- **Status**: ✅ Completed (commit: 512208d)
- **Task**: Replace `interface{}` with `any` (Go 1.18+)
- **Impact**: Improves code readability, follows modern Go conventions
- **Completed Changes**: 
  - `queue/data.go` - `Payload any`
  - `queue/queue.go` - `Enqueue(payload any)`
  - `queue/queue.go` - `EnqueueBatch(payloads []any)`
  - `queue/memory.go` - `estimatePayloadSize(payload any)`
  - `tests/*.go` - All test files updated to use `any`
  - `examples/basic_usage.go` - Updated to use `any`

### Fix go.mod Dependencies
- **Status**: ✅ Completed (commit: 6cf6ecf)
- **Task**: Make `github.com/google/uuid` a direct dependency
- **Completed**: Ran `go mod tidy` to mark as direct dependency
- **Files**: `go.mod`

### Add Context Support
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Add context-aware operations throughout API
- **New Functions**:
  ```go
  func (q *Queue) EnqueueWithContext(ctx context.Context, payload any) error
  func (q *Queue) EnqueueBatchWithContext(ctx context.Context, payloads []any) error
  func (c *Consumer) ReadWithContext(ctx context.Context) (*QueueData, error)
  func (c *Consumer) ReadBatchWithContext(ctx context.Context, limit int) ([]*QueueData, error)
  ```
- **Files**: `queue/queue.go`, `queue/consumer.go`, `tests/context_test.go`

### Add Test Coverage Reporting
- **Status**: ✅ Completed
- **Task**: Set up coverage reporting and tracking
- **Completed**: Added coverage commands to AGENTS.md
- **Commands**:
  ```bash
  go test ./tests -coverprofile=coverage.out
  go tool cover -html=coverage.out -o coverage.html
  go tool cover -func=coverage.out | grep total
  ```
- **Files**: `AGENTS.md`

### Add Missing Test Cases
- **Status**: ✅ Completed (2025-12-27)
- **Completed Tests** (83 new tests added):
  - ✅ Context support: Cancellation, timeouts, concurrent usage (11 tests)
  - ✅ Consumer management: RemoveConsumer, GetConsumer, GetAllConsumers (9 tests)
  - ✅ Lifecycle: Close, CloseWithContext, operations on closed queue (12 tests)
  - ✅ Batch operations: Atomicity, edge cases, cross-chunk (14 tests)
  - ✅ Error handling: MemoryLimitError, payload types, memory accuracy (14 tests)
  - ✅ Consumer info: GetUnreadCount, position tracking, history (14 tests)
  - ✅ Blocking edge cases: Multiple waiters, interruptions (9 tests)
- **Total Tests**: 114 (up from 42)
- **Files**: `tests/consumer_management_test.go`, `tests/lifecycle_test.go`, `tests/batch_operations_test.go`, `tests/error_handling_test.go`, `tests/consumer_info_test.go`, `tests/blocking_edge_cases_test.go`

## Priority 2 - Medium Priority

### Make Memory Limit Configurable
- **Status**: ✅ Completed (2025-12-27)
- **Current**: Memory limit hardcoded to 1MB
- **Task**: Make configurable via QueueConfig
- **New API**:
  ```go
  func NewQueueWithConfig(name string, config QueueConfig) *Queue
  
  type QueueConfig struct {
      TTL         time.Duration
      MaxMemory   int64  // Configurable (default 1MB)
  }
  ```
- **Files**: `queue/queue.go`, `queue/memory.go`, `tests/config_test.go`

### Optimize Memory Estimation with Caching
- **Status**: ✅ Completed (2025-12-27)
- **Current**: Uses reflection recursively for every item (expensive)
- **Improvement**: Cache size estimates for fixed-size types (primitives, structs of primitives)
- **Implementation**:
  ```go
  type MemoryTracker struct {
      totalMemory  int64
      maxMemory    int64
      sizeCache    map[reflect.Type]int64  // Cache type sizes
      cacheMutex   sync.RWMutex
  }
  ```
- **Files**: `queue/memory.go`

### Optimize Batch Memory Validation
- **Status**: ✅ Completed (2025-12-27)
- **Current**: `EnqueueBatch` validates each item sequentially
- **Improvement**: Calculate total size once before validation (including chunk overhead)
- **Files**: `queue/queue.go`

### Reduce Lock Contention
- **Status**: ✅ Completed (2025-12-27)
- **Completed Improvements**:
  - Implemented RCU (Read-Copy-Update) for `ConsumerManager` using `atomic.Value`
  - Converted `ChunkedList.totalItems` to `atomic.Int64` (lock-free `IsEmpty`)
  - Converted `Consumer.totalItemsRead` to `atomic.Int64`
- **Impact**: 
  - `GetAllConsumers` (used by expiration worker) is now O(1) and lock-free
  - Queue stats and empty checks are lock-free
  - Reduced lock hold times in critical paths
- **Files**: `queue/consumer.go`, `queue/chunked_list.go`, `queue/queue.go`

### Add CI/CD Pipeline
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Create GitHub Actions workflow
- **Files**: `.github/workflows/test.yml`
- **Workflow**: Runs linter, unit tests, and integration tests on push/PR

### Add Makefile
- **Status**: ✅ Completed (2025-12-27)
- **File**: `Makefile`
- **Targets**: `build`, `test`, `test-integration`, `lint`, `fmt`, `clean`, `coverage`

### Add golangci-lint Configuration
- **Status**: ✅ Completed (2025-12-27)
- **File**: `.golangci.yml`
- **Linters**: govet, errcheck, staticcheck, gosimple, ineffassign, unused, typecheck, gofmt, goimports

### Separate Unit and Integration Tests
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Add build tags to separate test types
- **Tags**: `//go:build integration` added to stress tests
- **Files**: `tests/benchmark_test.go`, `tests/extreme_stress_test.go`
- **Usage**: `make test` (unit only) vs `make test-integration`


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

### ✅ Fix Race Condition in AddEvent (commit: 144bf05)
- Made QueueData immutable after creation
- Eliminated SIGSEGV crashes during concurrent dequeue operations
- All tests pass with race detection

### ✅ Fix Memory Tracking Inaccuracy (commit: 144bf05)  
- Memory size is now fixed after QueueData creation
- Accurate tracking without runtime changes

### ✅ Fix ChunkNode.Size Race Condition (commit: 8823a44)
- Changed Size field to atomic int32 with atomic operations
- Added GetSize(), setSize(), incrementSize() methods
- Updated all direct Size accesses across codebase
- Safe concurrent access by producers and consumers

### ✅ Fix List Traversal Race Condition (commit: 8823a44)
- Consumer.Read() now holds queue.mutex.RLock() when accessing list elements
- Prevents data races with producers modifying list structure
- Added comprehensive race tests (TestConcurrentDequeueNoRace, TestMassiveConcurrentDequeue)

### ✅ Fix HasMoreData() Race Condition (commit: ac15607)
- Now holds queue.mutex.RLock() when accessing element.Value and element.Next()
- Added tests: TestHasMoreDataRace, TestHasMoreDataWithReading, TestMultipleConsumersHasMoreData

### ✅ Fix UpdatePositionAfterExpiration() Race Condition (commit: ac15607)
- Documented that caller must hold queue.mutex (already did)
- Safe list traversal during expiration

### ✅ Fix Memory Leak on Expiration (commit: 2c85032)
- RemoveExpired() now returns removed QueueData items
- ChunkedList properly calls memoryTracker.RemoveData() for each expired item
- Memory is correctly released when items expire
- Prevents queue from becoming unusable after expiration
- Test: TestMemoryLeakOnExpiration

### ✅ Fix ForceExpiration() Ignoring expirationEnabled Flag (commit: 2c85032)
- cleanupExpiredItems() now checks expirationEnabled flag
- Returns 0 when expiration is disabled
- Respects API contract
- Test: TestExpirationDisabled now passes

### ✅ Fix Consumer Position Corruption on Partial Chunk Expiration (commit: 2c85032)
- Added ChunkRemovalInfo to track per-chunk item removals
- UpdatePositionAfterExpiration() adjusts consumer indexInChunk
- Prevents data loss when items expire from middle of consumer's position
- Consumers maintain correct position after chunk compaction

### ✅ Fix Deadlock in Lock Hierarchy (commit: e0ed843)
- Fixed deadlock between expiration worker and consumers
- Added getPositionUnsafe() for internal use
- calculateExpiredCountsPerConsumer() now locks consumer briefly without nesting
- Refactored Consumer.Read() to use fine-grained locking
- Fixed TestHighThroughputStress busy-wait loop

### ✅ Fix Lock Ordering Violations (Latest)
- Fixed HasMoreData() lock ordering violation using snapshot pattern
- Fixed GetUnreadCount() lock ordering violation
- Fixed GetStats() lock ordering violation
- All consumer methods now follow queue→consumer lock hierarchy
- Prevents deadlocks in all consumer operations

### ✅ Fix TOCTOU Issue in Consumer.Read() (Latest)
- Queue lock now held during chunk.Get() to prevent race with expiration
- Prevents reading stale data when expiration modifies chunks
- Ensures data consistency during concurrent operations

### ✅ Fix Test Race Conditions (Latest)
- TestHighThroughputStress now uses atomic.AddInt64/LoadInt64 for shared counters
- Added sync/atomic import to benchmark_test.go
- All tests pass with -race flag

### ✅ Use Modern Go Idioms (commit: 512208d)
- Replaced all `interface{}` with `any` throughout codebase
- Updated 6 files across queue package, tests, and examples

### ✅ Fix go.mod Dependencies (commit: 6cf6ecf)
- Made github.com/google/uuid a direct dependency

### ✅ Add Project Documentation (commit: e6e8ca6)
- Added AGENTS.md, RACE_CONDITION_FIX_STRATEGY.md, TODO.md

### ✅ Comprehensive Testing and Validation
- All race conditions fixed and verified
- Added stress tests: TestExtremeProducerConsumerStress, TestExpirationDuringHeavyLoad
- Added position integrity test: TestConsumerPositionIntegrityUnderLoad
- All tests pass with -race flag
- Verified with 20+ concurrent producers/consumers
- No data corruption or position errors detected

---

## Legend

- ⚠️ Not Started
- 🚧 In Progress
- ✅ Completed
- ❌ Cancelled

## Notes

- Items are organized by priority (P0-P3)
- **All P0 critical issues have been resolved** - queue is production-ready
- P1 items significantly improve code quality and usability
- P2 items add useful features and improvements
- P3 items are nice-to-have enhancements

## Bug Fix Summary

**Total Bugs Fixed**: 13 critical bugs
- 8 race conditions (SIGSEGV crashes, data corruption, lock ordering, TOCTOU, test races)
- 3 expiration bugs (memory leak, API violation, position corruption)
- 2 memory tracking bugs

**Verification**: 
- All tests pass with -race flag
- Stress tested with 20 producers + 20 consumers
- Zero data corruption detected
- Zero deadlocks detected
- Production-ready with comprehensive concurrency fixes

**Latest Fixes (2025-12-26)**:
- Lock ordering violations in HasMoreData(), GetUnreadCount(), GetStats()
- TOCTOU issue in Consumer.Read() chunk access
- Test race condition in TestHighThroughputStress

**Latest Enhancements (2025-12-27)**:
- ✅ Added blocking/non-blocking operation support
- ✅ Configurable TTL at queue creation
- ✅ Blocking Enqueue/Read operations (default)
- ✅ Non-blocking TryEnqueue/TryRead operations
- ✅ Blocking/non-blocking batch operations
- ✅ Channel-based notification system for blocking operations
- ✅ Comprehensive blocking behavior tests (tests/blocking_test.go)
- ✅ Updated all documentation (README, API docs, USAGE_GUIDE, ARCHITECTURE, AGENTS)
- ✅ All tests pass with -race flag
- ✅ TestHighThroughputStress now passes

**Test Suite Expansion (2025-12-27)**:
- ✅ Added 72 new comprehensive tests (42 → 114 tests)
- ✅ 6 new test files covering critical and high-priority scenarios
- ✅ Fixed Queue.Close() and Consumer.Close() idempotency bugs
- ✅ Fixed notification channel size for multiple blocked waiters
- ✅ All critical and high-priority test gaps filled
- ✅ Consumer management fully tested (RemoveConsumer, GetConsumer, etc.)
- ✅ Lifecycle extensively tested (Close, CloseWithContext, edge cases)
- ✅ Batch operations atomicity verified
- ✅ Error handling comprehensive (MemoryLimitError fields, payload types)
- ✅ Consumer info methods fully tested (GetUnreadCount, position, history)
- ✅ Blocking edge cases covered (multiple waiters, interruptions, spurious wakeups)

Last Updated: 2025-12-27
