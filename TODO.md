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
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Add runnable examples to godoc
- **Files**: `queue/example_test.go`

### Add Architecture Diagrams
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Create visual diagrams
- **Files**: `docs/architecture/*.md` (Memory Layout, Consumer Tracking, Expiration, Lock Hierarchy)

### Document Limitations More Clearly
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Add limitations section to README.md
- **Files**: `README.md`

### Add More Specific Error Types
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Create specific error types for better error handling
- **New Types**:
  ```go
  type QueueClosedError struct{ Operation string }
  type ConsumerNotFoundError struct{ ID string }
  type InvalidPositionError struct{ Position int64; Reason string }
  ```
- **Features**:
  - QueueClosedError returned when operations fail due to queue closure
  - ConsumerNotFoundError for consumer lookup failures (ready for future use)
  - InvalidPositionError for position validation (ready for future use)
  - Centralized error handling in `queue/errors.go`
  - Comprehensive test suite (8 tests)
- **Files**: `queue/errors.go`, `tests/error_types_test.go`

### Add Error Wrapping
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Use `fmt.Errorf` with `%w` for error wrapping throughout queue package
- **Implementation**:
  - Added error wrapping in EnqueueWithContext
  - Specific error types replace generic fmt.Errorf calls
  - Improved error context for debugging
- **Files**: `queue/queue.go`, `queue/errors.go`

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
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Allow consumers to filter items based on predicates
- **New Functions**:
  ```go
  func (c *Consumer) TryReadWhere(predicate func(*QueueData) bool) *QueueData
  func (c *Consumer) ReadWhere(predicate func(*QueueData) bool) *QueueData
  func (c *Consumer) ReadWhereWithContext(ctx context.Context, predicate func(*QueueData) bool) (*QueueData, error)
  ```
- **Features**:
  - Non-blocking filtering with `TryReadWhere`
  - Blocking filtering with `ReadWhere`
  - Context-aware filtering with cancellation/timeout support
  - Comprehensive test suite (13 tests)
- **Note**: Filtering advances consumer position, consuming non-matching items
- **Files**: `queue/consumer.go`, `tests/filtering_test.go`, documentation updated

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
- **Status**: ✅ Completed (2025-12-27)
- **Files Added**:
  - `LICENSE` - MIT License
  - `CHANGELOG.md` - Following Keep a Changelog format with full version history
  - `CONTRIBUTING.md` - Comprehensive development guidelines including workflow, testing, and PR process
- **Note**: Semantic versioning established in CHANGELOG.md (v1.0.0 as production-ready release)

---

## Priority 4 - Quality & Testing Improvements

### Improve Test Coverage
- **Status**: ✅ Completed (2025-12-27)
- **Previous**: 25.2% test coverage
- **Improvements**: Added 21 new tests covering previously untested methods
- **Covered Areas**:
  - ChunkedList methods: `GetTotalItems()`, `GetLastElement()`, `GetChunk()`, `IsEmpty()`, `GetMemoryUsage()`, `IterateFrom()`, `CountItemsFrom()`, multi-chunk operations
  - Consumer methods: `GetNotificationChannel()`, `GetPosition()`, `ReadWithContext()`, `TryReadBatch()`, `ReadBatch()`, `ReadBatchWithContext()`, `HasMoreData()`, `GetUnreadCount()`, `GetStats()`, `GetDequeueHistory()`, `ReadWhere()`
- **New Tests**: 21 comprehensive coverage tests (10 ChunkedList + 11 Consumer)
- **Impact**: Better code coverage and confidence in correctness
- **Files**: `tests/coverage_chunked_list_test.go`, `tests/coverage_consumer_test.go`

### Add Integration Tests for Real-World Scenarios
- **Status**: ✅ Completed (2025-12-27)
- **Task**: Add tests simulating real-world usage patterns
- **Scenarios Implemented**:
  - Job queue simulation with worker pool
  - Event streaming with multiple subscribers
  - Graceful shutdown with in-flight messages
  - Rate limiting pattern
  - Backpressure handling with memory limits
  - Multi-stage processing pipeline
  - Context cancellation propagation
- **New Tests**: 7 integration scenario tests
- **Impact**: Validates production readiness and real-world behavior
- **Files**: `tests/integration_scenarios_test.go`

---

## Priority 5 - Developer Experience

### Add Examples for Common Use Cases
- **Status**: ✅ Completed (2025-12-27)
- **Examples Added**:
  - Worker pool pattern (job processing with multiple workers)
  - Request-response pattern (correlation IDs and response queues)
- **Impact**: Demonstrates practical usage patterns
- **Files**: `examples/patterns/worker_pool.go`, `examples/patterns/request_response.go`

### Add Observability/Metrics Export
- **Status**: ✅ Completed (2025-12-27)
- **Features Implemented**:
  - Metrics tracking system (`queue.Metrics`)
  - Prometheus metrics exporter (queue depth, throughput, latency, memory)
  - MetricsSnapshot for point-in-time statistics
  - Latency percentiles (average and P95)
  - Operation counters (enqueued, dequeued, expired, errors)
  - Example showing metrics usage and Prometheus format
- **Metrics Available**:
  - Queue depth, memory usage/percent, consumer count
  - Total operations (enqueued, dequeued, expired)
  - Error counts (enqueue errors, memory limit hits)
  - Latency stats (average, P95 for enqueue/dequeue)
- **Impact**: Production-ready monitoring and debugging capability
- **Files**: `queue/metrics.go`, `examples/observability/metrics_example.go`

### Create Getting Started Tutorial
- **Status**: ✅ Completed (2025-12-27)
- **Contents**:
  - Installation and setup instructions
  - First queue tutorial with step-by-step examples
  - Understanding MPMC behavior (broadcast vs load balancing)
  - Error handling patterns with examples
  - Production patterns (blocking/non-blocking, context, graceful shutdown)
  - Monitoring and observability guide
  - Common pitfalls and how to avoid them
  - Production checklist
  - Quick reference guide
- **Impact**: Significantly lowers barrier to entry for new users
- **Files**: `docs/GETTING_STARTED.md`

---

## Priority 6 - Performance & Optimization

### Optimize Memory Estimation for Common Types
- **Status**: ⚠️ Not Started
- **Current**: Uses reflection for all types (slower)
- **Optimization**:
  - Pre-compute sizes for common primitives (int, string, bool, etc.)
  - Add fast path for known struct types
  - Cache type sizes more aggressively
  - Consider `unsafe.Sizeof` for known types
- **Impact**: 10-30% faster enqueue operations
- **Files**: `queue/memory.go`

### Add Benchmarking CI/CD Integration
- **Status**: ⚠️ Not Started
- **Task**: Automated performance regression detection
- **Features**:
  - Run benchmarks on every PR
  - Compare performance against baseline
  - Track performance trends over time
  - Alert on significant regressions (>10% slower)
  - Generate performance comparison reports
- **Impact**: Prevents performance regressions, tracks improvements
- **Files**: `.github/workflows/benchmark.yml`, `scripts/compare-benchmarks.sh`

### Add Memory Pooling for QueueData
- **Status**: ⚠️ Not Started
- **Task**: Use `sync.Pool` to reduce allocations
- **Optimization**:
  - Pool QueueData objects for reuse
  - Pool common payload types if possible
  - Reduce GC pressure in high-throughput scenarios
- **Impact**: 20-40% better performance under high load, reduced GC pauses
- **Complexity**: Medium - need to ensure proper reset/cleanup
- **Files**: `queue/data.go`, `queue/pool.go`

---

## Priority 7 - Advanced Features

### Add Queue Pause/Resume
- **Status**: ⚠️ Not Started
- **Task**: Ability to temporarily pause/resume queue operations
- **Use Cases**:
  - Backpressure management
  - Maintenance windows
  - Rate limiting at queue level
  - Circuit breaker pattern
- **New API**:
  ```go
  func (q *Queue) Pause()
  func (q *Queue) Resume()
  func (q *Queue) IsPaused() bool
  ```
- **Impact**: Better operational control, backpressure handling
- **Files**: `queue/queue.go`

### Add Consumer Groups
- **Status**: ✅ Completed (2025-12-28)
- **Task**: Load-balanced consumers (each item read by one consumer in group)
- **Difference**: Current model has each consumer read all items independently
- **Features**:
  - `Queue.AddConsumerGroup(name)` creates a shared group
  - `Group.AddConsumer()` creates consumers that share the group's position
  - Thread-safe distribution of items among group members
  - Fully integrated with expiration logic
  - Fixed bug in `Consumer.TryRead` where position was lost at end of chunk
- **Use Cases**:
  - Load balancing across workers
  - Parallel processing with work distribution
  - Scaling consumers horizontally
- **New API**:
  ```go
  func (q *Queue) AddConsumerGroup(groupName string) *ConsumerGroup
  func (cg *ConsumerGroup) AddConsumer() *Consumer
  ```
- **Files**: `queue/consumer_group.go`, `queue/consumer.go`, `queue/queue.go`, `tests/consumer_group_test.go`

### Add Message Acknowledgment Pattern
- **Status**: ⚠️ Not Started
- **Task**: Reliable message processing with ack/nack
- **Features**:
  - Consumers must acknowledge message processing
  - Nack (negative ack) for failed processing
  - Redelivery on failure (configurable retries)
  - Timeout for automatic nack
- **New API**:
  ```go
  func (c *Consumer) ReadWithAck() (*QueueData, func() error, func() error)
  // Returns: data, ackFunc, nackFunc
  ```
- **Impact**: Enables at-least-once delivery semantics
- **Complexity**: High - significant architectural change
- **Files**: `queue/ack.go`

### Add Dead Letter Queue Support
- **Status**: ⚠️ Not Started
- **Task**: Failed messages go to separate queue for inspection
- **Features**:
  - Items that fail processing after N attempts → DLQ
  - Configurable max retry count
  - DLQ monitoring and management
  - Replay from DLQ after fixing issues
- **Use Cases**:
  - Error handling and debugging
  - Monitoring problematic messages
  - Manual intervention workflows
- **Impact**: Better error handling and observability
- **Files**: `queue/dlq.go`

---

## Priority 8 - Documentation & Ecosystem

### Add Architecture Decision Records (ADRs)
- **Status**: ✅ Completed (2025-12-28)
- **Task**: Document why certain design choices were made
- **Topics**:
  - Why chunked list vs other data structures
  - Lock hierarchy design decisions
  - Immutable QueueData rationale
  - Memory estimation approach
  - RCU for consumer management
- **Impact**: Helps future contributors understand context
- **Files**: `docs/adr/` directory

### Add Comparison with Other Queue Implementations
- **Status**: ✅ Completed (2025-12-28)
- **Task**: Benchmark and feature comparison
- **Comparisons**:
  - vs Go channels (when to use each)
  - vs popular message queue libraries (RabbitMQ, Kafka, NATS)
  - Performance comparisons (throughput, latency, memory)
  - Feature matrix (TTL, persistence, ordering, etc.)
  - Trade-offs and use case recommendations
- **Impact**: Helps users choose right tool for their needs
- **Files**: `docs/COMPARISON.md`

### Add CLI Tool
- **Status**: ✅ Completed (2025-12-28)
- **Task**: Command-line tool for queue management
- **Features**:
  - Inspect queue stats (`qctl stats <queue-name>`)
  - Monitor consumers (`qctl consumers <queue-name>`)
  - Enqueue/dequeue items for testing
  - Clear/purge queue
  - Export metrics
- **Use Cases**: Debugging, monitoring, testing
- **Files**: `cmd/qctl/`

---

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
