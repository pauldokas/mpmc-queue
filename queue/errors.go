package queue

import "fmt"

// QueueClosedError is returned when an operation is attempted on a closed queue
type QueueClosedError struct {
	Operation string // The operation that was attempted (e.g., "enqueue", "read")
}

func (e *QueueClosedError) Error() string {
	if e.Operation != "" {
		return fmt.Sprintf("queue closed: cannot perform %s operation", e.Operation)
	}
	return "queue closed"
}

// ConsumerNotFoundError is returned when a consumer with the given ID cannot be found
type ConsumerNotFoundError struct {
	ID string // The consumer ID that was not found
}

func (e *ConsumerNotFoundError) Error() string {
	return fmt.Sprintf("consumer not found: %s", e.ID)
}

// InvalidPositionError is returned when a consumer position is invalid
type InvalidPositionError struct {
	Position int64  // The invalid position
	Reason   string // Why the position is invalid
}

func (e *InvalidPositionError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("invalid position %d: %s", e.Position, e.Reason)
	}
	return fmt.Sprintf("invalid position: %d", e.Position)
}

// MemoryLimitError represents a memory limit exceeded error
// Moved from chunked_list.go for centralized error handling
type MemoryLimitError struct {
	Current int64 // Current memory usage
	Max     int64 // Maximum allowed memory
	Needed  int64 // Additional memory needed
}

func (e *MemoryLimitError) Error() string {
	return fmt.Sprintf("memory limit exceeded: current=%d, max=%d, needed=%d", e.Current, e.Max, e.Needed)
}

// QueueError represents a general queue error with optional error wrapping
// Moved from chunked_list.go and enhanced with error wrapping support
type QueueError struct {
	Message string // Error message
}

func (e *QueueError) Error() string {
	return e.Message
}
