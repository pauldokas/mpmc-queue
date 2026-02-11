# Benchmarking Guide

This project uses Go's built-in benchmarking tools and a custom regression detection script.

## Running Benchmarks Locally

To run all benchmarks and save the output:

```bash
./scripts/run-benchmarks.sh results.txt
```

This runs benchmarks with the `-tags integration` flag to ensure all scenarios are covered.

## Comparing Results

To compare two benchmark results (e.g., before and after a change):

```bash
# 1. Run baseline
git checkout main
./scripts/run-benchmarks.sh base.txt

# 2. Run new code
git checkout my-feature-branch
./scripts/run-benchmarks.sh new.txt

# 3. Compare
go run scripts/compare-benchmarks.go -base base.txt -new new.txt
```

The tool will report:
- Percentage change for each benchmark
- Red/Green coloring for regressions/improvements
- **FAILURE** if any regression exceeds 10% (configurable via `-threshold`)

## CI/CD Integration

Benchmarks run automatically on every Pull Request via GitHub Actions (`.github/workflows/benchmark.yml`).
The workflow:
1. Runs benchmarks on the PR branch.
2. Runs benchmarks on the base branch (e.g., main).
3. Compares the two and fails the build if significant regressions are detected.

## Benchmark Descriptions

- **BenchmarkEnqueue**: Single item enqueue throughput.
- **BenchmarkEnqueueBatch**: Batch enqueue throughput.
- **BenchmarkRead**: Single item read throughput.
- **BenchmarkConcurrent**: Multi-producer/consumer scenarios.
- **BenchmarkMemoryEstimation**: Performance of memory tracking (Reflection vs Sizeable).
