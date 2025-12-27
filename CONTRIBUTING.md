# Contributing to mpmc-queue

Thank you for your interest in contributing to mpmc-queue! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Coding Standards](#coding-standards)
- [Testing Requirements](#testing-requirements)
- [Submitting Changes](#submitting-changes)
- [Reporting Bugs](#reporting-bugs)
- [Suggesting Enhancements](#suggesting-enhancements)

## Code of Conduct

This project adheres to a Code of Conduct. By participating, you are expected to uphold this code. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before contributing.

## Getting Started

### Prerequisites

- Go 1.25 or higher
- golangci-lint (for linting)
- Make (optional, but recommended)

### Setting Up Your Development Environment

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```bash
   git clone https://github.com/yourusername/mpmc-queue.git
   cd mpmc-queue
   ```

3. Add the upstream repository as a remote:
   ```bash
   git remote add upstream https://github.com/originalowner/mpmc-queue.git
   ```

4. Install dependencies:
   ```bash
   go mod download
   ```

5. Verify your setup:
   ```bash
   make build
   make test
   ```

## Development Workflow

### Before You Start

1. **Check existing issues**: Look for existing issues or discussions related to your proposed changes
2. **Create an issue**: For significant changes, create an issue first to discuss your approach
3. **Create a branch**: Create a feature branch from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```

### Making Changes

1. **Write code**: Implement your changes following our [coding standards](#coding-standards)
2. **Write tests**: Add tests for your changes (see [testing requirements](#testing-requirements))
3. **Run tests**: Ensure all tests pass with race detection:
   ```bash
   make test
   make test-all  # Includes integration tests
   ```
4. **Run linter**: Ensure code passes all linters:
   ```bash
   make lint
   ```
5. **Format code**: Format your code:
   ```bash
   make fmt
   ```

## Coding Standards

### Critical Rules for Concurrency

**READ [AGENTS.md](AGENTS.md) FIRST!** This is mandatory for all contributors. Key rules:

1. **Lock Ordering**: ALWAYS acquire `Queue.mutex` BEFORE `Consumer.mutex`
2. **Race Detection**: ALWAYS run tests with `-race` flag
3. **Immutability**: `QueueData` is immutable - NEVER modify it after creation
4. **Locking**: Use `RLock` for reads, `Lock` for writes. Defer `Unlock` immediately
5. **Memory**: Pre-validate data size with `MemoryTracker.CanAddData()` before adding

### Code Style

#### Import Grouping
```go
import (
    // Standard library
    "context"
    "sync"
    "time"

    // External dependencies
    "github.com/google/uuid"

    // Internal packages
    "mpmc-queue/queue"
)
```

#### Naming Conventions
- **Exported**: `PascalCase` with Godoc comments explaining behavior and blocking semantics
- **Private**: `camelCase`
- **Interfaces**: `-er` suffix (e.g., `Reader`) or descriptive (e.g., `MemoryTracker`)
- **Constants**: `PascalCase` for exported, `camelCase` for private

#### Error Handling
- Use specific error types (e.g., `MemoryLimitError`)
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Check errors with `errors.As` for specific types

#### Documentation
- All exported functions must have Godoc comments
- Comments should explain WHY, not just WHAT
- Include blocking semantics in function comments
- Document lock acquisition patterns

## Testing Requirements

### Test Coverage
- **Minimum**: All new code must have tests
- **Target**: Aim for 80%+ coverage
- **Check coverage**: `make coverage`

### Test Types

#### Unit Tests
```bash
make test  # Fast, excludes integration tests
```

- Create a new `Queue` for every test
- Always use `defer q.Close()`
- Use `TryEnqueue`/`TryRead` in tests to prevent deadlocks
- Use `sync.WaitGroup` for concurrent tests
- Use `atomic.Int64` for shared counters

#### Integration Tests
```bash
make test-integration  # Stress and long-running tests
```

- Mark with `//go:build integration`
- Test realistic production scenarios
- Test with high concurrency (20+ goroutines)

#### Race Detection
**MANDATORY**: All tests must pass with race detection:
```bash
go test -race ./...
```

### Test Template
```go
func TestYourFeature(t *testing.T) {
    q := queue.NewQueue("test-queue")
    defer q.Close()

    // Your test logic here
    // Use TryEnqueue/TryRead, not Enqueue/Read
}
```

## Submitting Changes

### Commit Messages

Follow the conventional commits format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Example:**
```
fix(consumer): prevent race condition in HasMoreData()

Implemented snapshot pattern to avoid holding locks incorrectly.
Consumer now locks briefly to copy state, then unlocks before
acquiring queue lock.

Fixes #123
```

### Pull Request Process

1. **Update documentation**: Update relevant docs (README, API docs, etc.)
2. **Update CHANGELOG**: Add entry to `CHANGELOG.md` under `[Unreleased]`
3. **Ensure tests pass**: All tests must pass with race detection
4. **Create PR**: Submit your pull request with:
   - Clear title describing the change
   - Description explaining WHY and HOW
   - Reference to related issues
   - Test results (especially race detection)

5. **Code review**: Address feedback from maintainers
6. **CI must pass**: GitHub Actions must pass (linter, tests, race detection)

### PR Checklist

- [ ] Code follows style guidelines (see AGENTS.md)
- [ ] Added tests for new functionality
- [ ] All tests pass with `-race` flag
- [ ] Updated documentation (README, API docs, etc.)
- [ ] Added entry to CHANGELOG.md
- [ ] Linter passes (`make lint`)
- [ ] No new race conditions introduced
- [ ] Lock hierarchy respected (Queue.mutex > Consumer.mutex)

## Reporting Bugs

### Before Submitting a Bug Report

1. Check if the bug has already been reported in [Issues](https://github.com/yourusername/mpmc-queue/issues)
2. Update to the latest version to see if the issue persists
3. Collect debug information (see below)

### Bug Report Template

```markdown
**Describe the bug**
A clear description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:
1. Create queue with config '...'
2. Enqueue items '...'
3. See error

**Expected behavior**
What you expected to happen.

**Actual behavior**
What actually happened.

**Environment**
- Go version: [e.g., 1.25.1]
- OS: [e.g., Linux, macOS, Windows]
- mpmc-queue version/commit: [e.g., v1.0.0 or commit hash]

**Race detection output** (if applicable)
```
go test -race output here
```

**Additional context**
Any other context about the problem.
```

## Suggesting Enhancements

### Before Submitting an Enhancement

1. Check [TODO.md](TODO.md) to see if it's already planned
2. Search existing issues to avoid duplicates
3. Consider if it fits the project's scope and goals

### Enhancement Proposal Template

```markdown
**Is your feature request related to a problem?**
A clear description of the problem. Ex. "I'm frustrated when..."

**Describe the solution you'd like**
A clear description of what you want to happen.

**Describe alternatives you've considered**
Other solutions or features you've considered.

**API design** (if applicable)
```go
// Proposed API
func (q *Queue) NewMethod() error
```

**Impact on existing code**
Will this break existing APIs? How?

**Additional context**
Any other context or screenshots.
```

## Development Tips

### Running a Single Test
```bash
go test -v -race -run TestName ./tests/
```

### Debugging Race Conditions
```bash
GORACE="log_path=/tmp/race" go test -race ./...
```

### Profiling
```bash
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./tests/
go tool pprof cpu.prof
```

### Common Pitfalls

Read [AGENTS.md](AGENTS.md) for detailed information, but key pitfalls:

1. **Nil checks**: `chunkElement` can be nil - always check
2. **Context propagation**: Always respect `ctx.Done()`
3. **Channel blocking**: Never send without `select` with `default` case
4. **Lock ordering**: Never acquire Consumer.mutex while holding Queue.mutex

## Questions?

- Open an issue with the `question` label
- Check existing documentation in `/docs`
- Read [AGENTS.md](AGENTS.md) for development guidelines

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
