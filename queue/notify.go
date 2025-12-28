package queue

// notifyWaitingConsumers notifies waiting consumers based on queue depth and consumer count
// This method is non-blocking and safe to call with or without queue lock
func (q *Queue) notifyWaitingConsumers() {
	// Get active consumers (lock-free)
	consumers := q.consumers.GetAllConsumers()
	consumerCount := len(consumers)

	if consumerCount == 0 {
		return
	}

	// Get queue depth (atomic)
	// We want to wake up enough consumers to handle the backlog,
	// but not more than exist.
	queueDepth := q.data.GetTotalItems()

	// Determine how many signals to send
	// Formula: min(ActiveConsumers, QueueDepth)
	var signals int
	if queueDepth > int64(consumerCount) {
		signals = consumerCount
	} else {
		signals = int(queueDepth)
	}

	// Ensure at least 1 signal if there is data
	if signals < 1 && queueDepth > 0 {
		signals = 1
	}

	// Notify waiting consumers
	// We use a non-blocking send to avoid holding up the producer
	for i := 0; i < signals; i++ {
		select {
		case q.enqueueNotify <- struct{}{}:
		default:
			// Channel full or no receivers ready, stop sending
			return
		}
	}
}
