#!/bin/bash
set -e

OUTPUT_FILE=${1:-benchmark_results.txt}

echo "Running benchmarks..."
# Run benchmarks 3 times for stability
go test -tags integration -run=^$ -bench=. ./tests -count=3 -benchmem > "$OUTPUT_FILE"

echo "Benchmarks completed. Results saved to $OUTPUT_FILE"
