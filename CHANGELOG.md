# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Project maintenance files (LICENSE, CHANGELOG.md, CONTRIBUTING.md)
- Filtering/selection methods for consumers:
  - `TryReadWhere(predicate)` - Non-blocking filtered read
  - `ReadWhere(predicate)` - Blocking filtered read
  - `ReadWhereWithContext(ctx, predicate)` - Filtered read with context support
- Comprehensive filtering tests (13 tests covering all filtering scenarios)
- Specific error types for better error handling:
  - `QueueClosedError` - Returned when operations fail due to queue closure
  - `ConsumerNotFoundError` - For consumer lookup failures
  - `InvalidPositionError` - For position validation errors
- `EnqueueWithContext()` method for single-item context-aware enqueue
- Error wrapping with `%w` for improved error chains
- Comprehensive error type tests (8 tests)

### Changed
- Moved error types (`MemoryLimitError`, `QueueError`) to centralized `queue/errors.go`
- Replaced generic error messages with specific error types where appropriate
- Enhanced error context with wrapped errors

## [1.0.0] - 2025-12-27

### Added
- Initial production-ready release
- Multi-producer multi-consumer queue with full thread safety
- Configurable memory limit (default 1MB) with real-time tracking
- Time-based expiration with configurable TTL (default 10 minutes)
- Independent consumer tracking with position management
- Chunked storage using doubly-linked list (1000-item chunks)
- Blocking operations: `Enqueue`, `Read`, `EnqueueBatch`, `ReadBatch`
- Non-blocking operations: `TryEnqueue`, `TryRead`, `TryEnqueueBatch`, `TryReadBatch`
- Context support: `EnqueueWithContext`, `ReadWithContext`, and batch variants
- Consumer notifications when data expires before being read
- Comprehensive statistics for queue and consumer performance
- Batch operations with atomicity guarantees
- RCU-based consumer management for lock-free reads
- Atomic operations for stats tracking
- Channel-based notification system for blocking operations
- Comprehensive test suite (114 tests) with race detection
- CI/CD pipeline with GitHub Actions
- golangci-lint configuration with strict linting
- Makefile with build, test, lint, and coverage targets
- Complete documentation (README, API docs, Architecture, Usage Guide, Troubleshooting)
- Architecture diagrams for memory layout, consumer tracking, expiration, lock hierarchy
- Example code for basic and advanced usage
- Agent development guide (AGENTS.md)

### Fixed
- All race conditions (13 critical bugs fixed):
  - Race condition in AddEvent causing SIGSEGV crashes
  - Memory tracking inaccuracy
  - ChunkNode.Size race condition
  - List traversal race condition
  - HasMoreData() race condition
  - UpdatePositionAfterExpiration() race condition
  - Lock ordering violations (deadlocks)
  - TOCTOU issue in Consumer.Read()
  - Test race conditions
- Memory leak on expiration
- ForceExpiration() ignoring expirationEnabled flag
- Consumer position corruption on partial chunk expiration
- Queue.Close() and Consumer.Close() idempotency bugs
- Notification channel size for multiple blocked waiters

### Changed
- Made QueueData immutable after creation (thread-safe without locks)
- Replaced `interface{}` with `any` (Go 1.18+ idioms)
- Converted ChunkNode.Size to atomic int32
- Converted ChunkedList.totalItems to atomic.Int64
- Converted Consumer.totalItemsRead to atomic.Int64
- Optimized memory estimation with type caching
- Improved batch memory validation (single calculation)
- Separated unit and integration tests with build tags
- Made github.com/google/uuid a direct dependency

### Security
- All operations verified with race detection (`-race` flag)
- Stress tested with 20+ concurrent producers/consumers
- Zero data corruption detected
- Zero deadlocks detected

## [0.9.0] - 2025-12-26

### Fixed
- Lock ordering violations in HasMoreData(), GetUnreadCount(), GetStats()
- TOCTOU issue in Consumer.Read() chunk access
- Test race condition in TestHighThroughputStress

## [0.8.0] - 2025-12-20

### Added
- Blocking and non-blocking operation support
- Configurable TTL at queue creation
- Channel-based notification system

### Fixed
- Deadlock in lock hierarchy between expiration worker and consumers
- Lock ordering violations

## [0.7.0] - 2025-12-15

### Fixed
- Consumer position corruption on partial chunk expiration
- ForceExpiration() ignoring expirationEnabled flag
- Memory leak on expiration

## [0.6.0] - 2025-12-10

### Fixed
- HasMoreData() race condition
- UpdatePositionAfterExpiration() race condition

## [0.5.0] - 2025-12-05

### Fixed
- ChunkNode.Size race condition
- List traversal race condition

## [0.4.0] - 2025-12-01

### Fixed
- Race condition in AddEvent causing SIGSEGV crashes
- Memory tracking inaccuracy

### Changed
- Made QueueData immutable after creation

## [0.3.0] - 2025-11-25

### Added
- Memory constraints with configurable limit
- Memory tracking and estimation

## [0.2.0] - 2025-11-20

### Added
- Time-based expiration with background worker
- Consumer notifications for expired items

## [0.1.0] - 2025-11-15

### Added
- Initial implementation of MPMC queue
- Basic enqueue/dequeue operations
- Multiple consumer support
- Chunked storage implementation

[Unreleased]: https://github.com/yourusername/mpmc-queue/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/yourusername/mpmc-queue/compare/v0.9.0...v1.0.0
[0.9.0]: https://github.com/yourusername/mpmc-queue/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/yourusername/mpmc-queue/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/yourusername/mpmc-queue/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/yourusername/mpmc-queue/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/yourusername/mpmc-queue/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/yourusername/mpmc-queue/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/yourusername/mpmc-queue/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/yourusername/mpmc-queue/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/yourusername/mpmc-queue/releases/tag/v0.1.0
