package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"mpmc-queue/queue"
)

// Job represents a work item
type Job struct {
	ID      int
	Task    string
	Payload interface{}
}

// Result represents a job result
type Result struct {
	JobID int
	Data  interface{}
	Error error
}

func main() {
	fmt.Println("=== Worker Pool Pattern ===\n")

	// Create job queue
	jobQueue := queue.NewQueue("job-queue")
	defer jobQueue.Close()

	// Create result queue
	resultQueue := queue.NewQueue("result-queue")
	defer resultQueue.Close()

	const numWorkers = 5
	const numJobs = 20

	// Enqueue jobs
	fmt.Printf("Enqueueing %d jobs...\n", numJobs)
	for i := 0; i < numJobs; i++ {
		job := Job{
			ID:      i,
			Task:    "process",
			Payload: fmt.Sprintf("data-%d", i),
		}
		if err := jobQueue.TryEnqueue(job); err != nil {
			panic(err)
		}
	}

	// Start workers
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("Starting %d workers...\n", numWorkers)
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(ctx, i, jobQueue, resultQueue, &wg)
	}

	// Collect results
	go func() {
		wg.Wait()
		resultQueue.Close()
	}()

	// Read results
	resultConsumer := resultQueue.AddConsumer()
	processedCount := 0

	fmt.Println("\nProcessing results:")
	for {
		data := resultConsumer.TryRead()
		if data == nil {
			// Check if there's more data or queue is closed
			if !resultConsumer.HasMoreData() {
				time.Sleep(100 * time.Millisecond)
				if !resultConsumer.HasMoreData() {
					break
				}
			}
			continue
		}

		result := data.Payload.(Result)
		if result.Error != nil {
			fmt.Printf("  Job %d failed: %v\n", result.JobID, result.Error)
		} else {
			fmt.Printf("  Job %d completed: %v\n", result.JobID, result.Data)
		}
		processedCount++
	}

	fmt.Printf("\nTotal jobs processed: %d/%d\n", processedCount, numJobs)
}

func worker(ctx context.Context, workerID int, jobQueue, resultQueue *queue.Queue, wg *sync.WaitGroup) {
	defer wg.Done()

	consumer := jobQueue.AddConsumer()
	fmt.Printf("  Worker %d started\n", workerID)

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("  Worker %d shutting down (context cancelled)\n", workerID)
			return
		default:
		}

		// Try to get a job
		data := consumer.TryRead()
		if data == nil {
			// No jobs available
			if !consumer.HasMoreData() {
				fmt.Printf("  Worker %d: no more jobs\n", workerID)
				return
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Process job
		job := data.Payload.(Job)
		result := processJob(workerID, job)

		// Send result
		if err := resultQueue.TryEnqueue(result); err != nil {
			fmt.Printf("  Worker %d: failed to enqueue result: %v\n", workerID, err)
		}
	}
}

func processJob(workerID int, job Job) Result {
	// Simulate work
	time.Sleep(50 * time.Millisecond)

	return Result{
		JobID: job.ID,
		Data:  fmt.Sprintf("Processed by worker %d: %v", workerID, job.Payload),
		Error: nil,
	}
}
