package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"mpmc-queue/queue"
)

// Pipeline represents the data processing pipeline context
type Pipeline struct {
	// We could add global cancellation, metrics, etc here
}

// NewPipeline creates a new pipeline
func NewPipeline() *Pipeline {
	return &Pipeline{}
}

// Stream represents a stage in the pipeline
type Stream struct {
	Name     string
	Queue    *queue.Queue
	Done     <-chan struct{} // Closed when upstream is finished producing
	Pipeline *Pipeline
}

// Generate creates a source stream
func (p *Pipeline) Generate(name string, generator func(func(any))) *Stream {
	q := queue.NewQueue(name)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer q.Close() // Close queue when done producing

		// Publisher function passed to generator
		publish := func(item any) {
			if err := q.Enqueue(item); err != nil {
				fmt.Printf("[%s] Enqueue failed: %v\n", name, err)
			}
		}

		generator(publish)
	}()

	return &Stream{
		Name:     name,
		Queue:    q,
		Done:     done,
		Pipeline: p,
	}
}

// Map applies a transformation function to each item
// Spawns 'workers' concurrent processors
func (s *Stream) Map(name string, workers int, transform func(any) any) *Stream {
	outQ := queue.NewQueue(name)
	outDone := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(workers)

	fmt.Printf("Pipeline: Connecting %s -> %s (%d workers)\n", s.Name, name, workers)

	// Create a consumer group for this stage to load-balance items across workers
	group := s.Queue.AddConsumerGroup(name)

	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			// Add consumer to the group
			consumer := group.AddConsumer()

			// Context that cancels when upstream is done
			ctx, cancel := context.WithCancel(context.Background())

			// Monitor upstream done signal
			go func() {
				select {
				case <-s.Done:
					cancel() // Upstream finished, cancel read context
				case <-ctx.Done():
					// Already done
				}
			}()

			// Phase 1: Read until upstream is done
			for {
				data, err := consumer.ReadWithContext(ctx)
				if err != nil {
					// Context cancelled (upstream done) or queue closed
					break
				}
				if data == nil {
					break
				}

				// Process and forward
				result := transform(data.Payload)
				if result != nil {
					outQ.Enqueue(result)
				}
			}

			// Phase 2: Drain remaining items
			// When context is cancelled, there might still be items in the queue
			for {
				data := consumer.TryRead()
				if data == nil {
					// Check if truly empty (group check)
					if !consumer.HasMoreData() {
						break
					}
					// Small backoff if HasMoreData says yes but TryRead got nil (contention)
					time.Sleep(time.Millisecond)
					continue
				}

				result := transform(data.Payload)
				if result != nil {
					outQ.Enqueue(result)
				}
			}
		}(i)
	}

	// Close output when all workers are done
	go func() {
		wg.Wait()
		close(outDone)
		outQ.Close()
	}()

	return &Stream{
		Name:     name,
		Queue:    outQ,
		Done:     outDone,
		Pipeline: s.Pipeline,
	}
}

// Filter keeps items where predicate returns true
func (s *Stream) Filter(name string, workers int, predicate func(any) bool) *Stream {
	// Filter is just a Map that returns nil for dropped items
	// (Map implementation above skips nil results)
	return s.Map(name, workers, func(item any) any {
		if predicate(item) {
			return item
		}
		return nil
	})
}

// Batch groups items into slices of specified size
func (s *Stream) Batch(name string, batchSize int, timeout time.Duration) *Stream {
	outQ := queue.NewQueue(name)
	outDone := make(chan struct{})

	// Batching is harder to parallelize while maintaining order,
	// so we use a single worker for simplicity in this example.
	// For parallel batching, you'd need a partitioner stage first.
	go func() {
		defer close(outDone)
		defer outQ.Close()

		consumer := s.Queue.AddConsumer()
		batch := make([]any, 0, batchSize)

		flush := func() {
			if len(batch) > 0 {
				outQ.Enqueue(makeCopy(batch)) // Send copy
				batch = batch[:0]
			}
		}

		ticker := time.NewTicker(timeout)
		defer ticker.Stop()

		for {
			// Try non-blocking read first
			data := consumer.TryRead()

			if data != nil {
				batch = append(batch, data.Payload)
				if len(batch) >= batchSize {
					flush()
				}
				continue
			}

			// If no data, check signals
			select {
			case <-s.Done:
				// Upstream done, drain rest
				for {
					d := consumer.TryRead()
					if d == nil {
						break
					}
					batch = append(batch, d.Payload)
					if len(batch) >= batchSize {
						flush()
					}
				}
				flush() // Final flush
				return
			case <-ticker.C:
				flush() // Time based flush
			default:
				// Short sleep to prevent busy loop since we can't select on data arrival
				time.Sleep(time.Millisecond)
			}
		}
	}()

	return &Stream{
		Name:     name,
		Queue:    outQ,
		Done:     outDone,
		Pipeline: s.Pipeline,
	}
}

func makeCopy(src []any) []any {
	dst := make([]any, len(src))
	copy(dst, src)
	return dst
}

// Sink consumes the stream
func (s *Stream) Sink(name string, workers int, consume func(any)) {
	var wg sync.WaitGroup
	wg.Add(workers)

	fmt.Printf("Pipeline: Connecting %s -> %s (Sink)\n", s.Name, name)

	// Create a consumer group for this stage
	group := s.Queue.AddConsumerGroup(name)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			consumer := group.AddConsumer()

			// Just read until closed/empty
			// Since upstream closes queue when done, we can just Read() until nil
			// (Assuming upstream uses the Map/Generate logic that closes queue)

			for {
				data := consumer.Read()
				if data == nil {
					return
				}
				consume(data.Payload)
			}
		}()
	}

	wg.Wait()
}

// --- Example Usage ---

type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Tags      map[string]string
}

func main() {
	p := NewPipeline()

	fmt.Println("=== Data Pipeline Example ===")
	fmt.Println("Flow: Generator -> Parser -> Filter -> Enricher -> Printer")

	// 1. Generator: Produces raw log strings
	source := p.Generate("RawLogs", func(emit func(any)) {
		levels := []string{"INFO", "DEBUG", "ERROR", "WARN"}
		for i := 0; i < 100; i++ {
			level := levels[i%len(levels)]
			emit(fmt.Sprintf("%s|Log message #%d|user-%d", level, i, i%5))
		}
	})

	// 2. Parser: String -> LogEntry
	parser := source.Map("Parser", 4, func(item any) any {
		s := item.(string)
		parts := strings.Split(s, "|")
		if len(parts) != 3 {
			return nil // Malformed
		}

		// Simulate parsing work
		time.Sleep(5 * time.Millisecond)

		return LogEntry{
			Timestamp: time.Now(),
			Level:     parts[0],
			Message:   parts[1],
			Tags:      map[string]string{"user": parts[2]},
		}
	})

	// 3. Filter: Drop DEBUG logs
	filter := parser.Filter("NoDebug", 2, func(item any) bool {
		entry := item.(LogEntry)
		return entry.Level != "DEBUG"
	})

	// 4. Enricher: Add region tag
	enricher := filter.Map("Enricher", 4, func(item any) any {
		entry := item.(LogEntry)
		entry.Tags["region"] = "us-east-1"

		// Simulate API call
		time.Sleep(10 * time.Millisecond)

		return entry
	})

	// 5. Sink: Print to console
	var count int
	enricher.Sink("Printer", 1, func(item any) {
		entry := item.(LogEntry)
		fmt.Printf("[%s] %s (Tags: %v)\n", entry.Level, entry.Message, entry.Tags)
		count++
	})

	fmt.Printf("\nPipeline complete! Processed %d items.\n", count)
}
