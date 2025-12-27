package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"mpmc-queue/queue"
)

// Request represents an incoming request
type Request struct {
	ID            string
	Operation     string
	Data          interface{}
	ResponseQueue *queue.Queue // Queue to send response back to
}

// Response represents a processed response
type Response struct {
	RequestID string
	Result    interface{}
	Error     error
}

func main() {
	fmt.Println("=== Request-Response Pattern ===\n")

	// Create request queue
	requestQueue := queue.NewQueue("requests")
	defer requestQueue.Close()

	// Create individual response queues for each requester
	const numRequesters = 3
	responseQueues := make([]*queue.Queue, numRequesters)
	for i := 0; i < numRequesters; i++ {
		responseQueues[i] = queue.NewQueue(fmt.Sprintf("response-%d", i))
		defer responseQueues[i].Close()
	}

	// Start request processor (server)
	go requestProcessor(requestQueue)

	// Start requesters (clients)
	var wg sync.WaitGroup
	for i := 0; i < numRequesters; i++ {
		wg.Add(1)
		go requester(i, requestQueue, responseQueues[i], &wg)
	}

	wg.Wait()
	fmt.Println("\nAll requests processed successfully!")
}

func requester(requesterID int, requestQueue, responseQueue *queue.Queue, wg *sync.WaitGroup) {
	defer wg.Done()

	const numRequests = 5

	fmt.Printf("Requester %d: sending %d requests\n", requesterID, numRequests)

	for i := 0; i < numRequests; i++ {
		requestID := fmt.Sprintf("req-%d-%d", requesterID, i)

		// Send request
		req := Request{
			ID:            requestID,
			Operation:     "calculate",
			Data:          i * 10,
			ResponseQueue: responseQueue,
		}

		if err := requestQueue.TryEnqueue(req); err != nil {
			fmt.Printf("Requester %d: failed to send request %s: %v\n", requesterID, requestID, err)
			continue
		}

		// Wait for response with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		consumer := responseQueue.AddConsumer()

		response, err := consumer.ReadWhereWithContext(ctx, func(d *queue.QueueData) bool {
			resp := d.Payload.(Response)
			return resp.RequestID == requestID
		})

		cancel()

		if err != nil {
			fmt.Printf("Requester %d: timeout waiting for response %s\n", requesterID, requestID)
		} else if response != nil {
			resp := response.Payload.(Response)
			fmt.Printf("Requester %d: received response for %s: %v\n",
				requesterID, resp.RequestID, resp.Result)
		}
	}
}

func requestProcessor(requestQueue *queue.Queue) {
	consumer := requestQueue.AddConsumer()

	for {
		data := consumer.TryRead()
		if data == nil {
			time.Sleep(10 * time.Millisecond)
			if !consumer.HasMoreData() {
				return
			}
			continue
		}

		req := data.Payload.(Request)

		// Process request
		result := processRequest(req)

		// Send response back
		response := Response{
			RequestID: req.ID,
			Result:    result,
			Error:     nil,
		}

		if err := req.ResponseQueue.TryEnqueue(response); err != nil {
			fmt.Printf("Processor: failed to send response for %s: %v\n", req.ID, err)
		}
	}
}

func processRequest(req Request) interface{} {
	// Simulate processing
	time.Sleep(20 * time.Millisecond)

	// Simple calculation
	if num, ok := req.Data.(int); ok {
		return num * 2
	}

	return "processed"
}
