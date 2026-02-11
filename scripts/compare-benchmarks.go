package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// BenchmarkResult stores the parsed benchmark data
type BenchmarkResult struct {
	Name    string
	NsPerOp float64
	Count   int // Number of runs for averaging
}

func main() {
	baseFile := flag.String("base", "", "Path to baseline benchmark output")
	newFile := flag.String("new", "", "Path to new benchmark output")
	threshold := flag.Float64("threshold", 10.0, "Percentage threshold for regression failure")
	flag.Parse()

	if *baseFile == "" || *newFile == "" {
		fmt.Println("Usage: go run compare-benchmarks.go -base <file> -new <file> [-threshold <percent>]")
		os.Exit(1)
	}

	baseResults, err := parseFile(*baseFile)
	if err != nil {
		fmt.Printf("Error parsing base file: %v\n", err)
		os.Exit(1)
	}

	newResults, err := parseFile(*newFile)
	if err != nil {
		fmt.Printf("Error parsing new file: %v\n", err)
		os.Exit(1)
	}

	regressions := 0
	improvements := 0

	fmt.Printf("%-40s | %-10s | %-10s | %-10s\n", "Benchmark", "Base", "New", "Delta")
	fmt.Println(strings.Repeat("-", 80))

	for name, baseRes := range baseResults {
		if newRes, ok := newResults[name]; ok {
			delta := ((newRes.NsPerOp - baseRes.NsPerOp) / baseRes.NsPerOp) * 100

			deltaStr := fmt.Sprintf("%+.2f%%", delta)
			if delta > 0 {
				deltaStr = fmt.Sprintf("\033[31m%s\033[0m", deltaStr) // Red for slower
			} else {
				deltaStr = fmt.Sprintf("\033[32m%s\033[0m", deltaStr) // Green for faster
			}

			fmt.Printf("%-40s | %8.2fns | %8.2fns | %s\n",
				name, baseRes.NsPerOp, newRes.NsPerOp, deltaStr)

			if delta > *threshold {
				fmt.Printf("  ⚠️ REGRESSION DETECTED: %s is %.2f%% slower (threshold: %.2f%%)\n",
					name, delta, *threshold)
				regressions++
			}
			if delta < -(*threshold) {
				improvements++
			}
		}
	}

	fmt.Println(strings.Repeat("-", 80))
	if regressions > 0 {
		fmt.Printf("❌ FAILED: Found %d regressions exceeding %.2f%% threshold\n", regressions, *threshold)
		os.Exit(1)
	}

	fmt.Printf("✅ SUCCESS: No significant regressions found (%d improvements detected)\n", improvements)
}

func parseFile(path string) (map[string]*BenchmarkResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	results := make(map[string]*BenchmarkResult)
	scanner := bufio.NewScanner(file)

	// Regex to match: BenchmarkName-8 10000 1234 ns/op
	// Simplified to capture Name and ns/op
	re := regexp.MustCompile(`^(Benchmark[^\s]+)\s+\d+\s+(\d+(?:\.\d+)?)\s+ns/op`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			name := matches[1]
			// Strip CPU count suffix (e.g., -8) to compare across environments if needed
			if idx := strings.LastIndex(name, "-"); idx != -1 {
				name = name[:idx]
			}

			nsPerOp, err := strconv.ParseFloat(matches[2], 64)
			if err != nil {
				continue
			}

			if res, exists := results[name]; exists {
				// Running average
				res.NsPerOp = (res.NsPerOp*float64(res.Count) + nsPerOp) / float64(res.Count+1)
				res.Count++
			} else {
				results[name] = &BenchmarkResult{
					Name:    name,
					NsPerOp: nsPerOp,
					Count:   1,
				}
			}
		}
	}

	return results, scanner.Err()
}
