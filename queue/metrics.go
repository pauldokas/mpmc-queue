package queue

import (
	"fmt"
	"sync"
	"time"
)

// Metrics provides observability into queue operations
type Metrics struct {
	queue *Queue
	mu    sync.RWMutex

	// Counters
	totalEnqueued   int64
	totalDequeued   int64
	totalExpired    int64
	enqueueErrors   int64
	memoryLimitHits int64

	// Latency tracking
	enqueueLatency    []time.Duration
	dequeueLatency    []time.Duration
	maxLatencySamples int
}

// NewMetrics creates a new metrics tracker for a queue
func NewMetrics(q *Queue) *Metrics {
	return &Metrics{
		queue:             q,
		enqueueLatency:    make([]time.Duration, 0, 1000),
		dequeueLatency:    make([]time.Duration, 0, 1000),
		maxLatencySamples: 1000,
	}
}

// MetricsSnapshot represents a point-in-time metrics snapshot
type MetricsSnapshot struct {
	// Queue stats
	QueueName     string
	TotalItems    int64
	MemoryUsage   int64
	MemoryPercent float64
	ConsumerCount int

	// Operation counters
	TotalEnqueued   int64
	TotalDequeued   int64
	TotalExpired    int64
	EnqueueErrors   int64
	MemoryLimitHits int64

	// Latency stats
	AvgEnqueueLatency time.Duration
	P95EnqueueLatency time.Duration
	AvgDequeueLatency time.Duration
	P95DequeueLatency time.Duration

	// Rates (if duration provided)
	EnqueueRate float64 // items per second
	DequeueRate float64 // items per second

	Timestamp time.Time
}

// RecordEnqueue records an enqueue operation
func (m *Metrics) RecordEnqueue(duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalEnqueued++
	if err != nil {
		m.enqueueErrors++
		if _, ok := err.(*MemoryLimitError); ok {
			m.memoryLimitHits++
		}
	}

	// Record latency (keep last N samples)
	if len(m.enqueueLatency) >= m.maxLatencySamples {
		m.enqueueLatency = m.enqueueLatency[1:]
	}
	m.enqueueLatency = append(m.enqueueLatency, duration)
}

// RecordDequeue records a dequeue operation
func (m *Metrics) RecordDequeue(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalDequeued++

	// Record latency (keep last N samples)
	if len(m.dequeueLatency) >= m.maxLatencySamples {
		m.dequeueLatency = m.dequeueLatency[1:]
	}
	m.dequeueLatency = append(m.dequeueLatency, duration)
}

// RecordExpired records expired items
func (m *Metrics) RecordExpired(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalExpired += int64(count)
}

// GetSnapshot returns a point-in-time snapshot of metrics
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := m.queue.GetQueueStats()

	snapshot := MetricsSnapshot{
		QueueName:       m.queue.GetName(),
		TotalItems:      stats.TotalItems,
		MemoryUsage:     stats.MemoryUsage,
		MemoryPercent:   stats.MemoryPercent,
		ConsumerCount:   stats.ConsumerCount,
		TotalEnqueued:   m.totalEnqueued,
		TotalDequeued:   m.totalDequeued,
		TotalExpired:    m.totalExpired,
		EnqueueErrors:   m.enqueueErrors,
		MemoryLimitHits: m.memoryLimitHits,
		Timestamp:       time.Now(),
	}

	// Calculate latency percentiles
	if len(m.enqueueLatency) > 0 {
		snapshot.AvgEnqueueLatency = calculateAvg(m.enqueueLatency)
		snapshot.P95EnqueueLatency = calculateP95(m.enqueueLatency)
	}

	if len(m.dequeueLatency) > 0 {
		snapshot.AvgDequeueLatency = calculateAvg(m.dequeueLatency)
		snapshot.P95DequeueLatency = calculateP95(m.dequeueLatency)
	}

	return snapshot
}

// GetPrometheusMetrics returns metrics in Prometheus text format
func (m *Metrics) GetPrometheusMetrics() string {
	snapshot := m.GetSnapshot()

	metrics := ""

	// Queue depth
	metrics += formatPrometheusMetric("mpmc_queue_depth", snapshot.QueueName, float64(snapshot.TotalItems))

	// Memory usage
	metrics += formatPrometheusMetric("mpmc_queue_memory_bytes", snapshot.QueueName, float64(snapshot.MemoryUsage))
	metrics += formatPrometheusMetric("mpmc_queue_memory_percent", snapshot.QueueName, snapshot.MemoryPercent)

	// Consumers
	metrics += formatPrometheusMetric("mpmc_queue_consumers", snapshot.QueueName, float64(snapshot.ConsumerCount))

	// Operations
	metrics += formatPrometheusMetric("mpmc_queue_enqueued_total", snapshot.QueueName, float64(snapshot.TotalEnqueued))
	metrics += formatPrometheusMetric("mpmc_queue_dequeued_total", snapshot.QueueName, float64(snapshot.TotalDequeued))
	metrics += formatPrometheusMetric("mpmc_queue_expired_total", snapshot.QueueName, float64(snapshot.TotalExpired))
	metrics += formatPrometheusMetric("mpmc_queue_enqueue_errors_total", snapshot.QueueName, float64(snapshot.EnqueueErrors))
	metrics += formatPrometheusMetric("mpmc_queue_memory_limit_hits_total", snapshot.QueueName, float64(snapshot.MemoryLimitHits))

	// Latency
	if snapshot.AvgEnqueueLatency > 0 {
		metrics += formatPrometheusMetric("mpmc_queue_enqueue_latency_avg_seconds", snapshot.QueueName, snapshot.AvgEnqueueLatency.Seconds())
		metrics += formatPrometheusMetric("mpmc_queue_enqueue_latency_p95_seconds", snapshot.QueueName, snapshot.P95EnqueueLatency.Seconds())
	}

	if snapshot.AvgDequeueLatency > 0 {
		metrics += formatPrometheusMetric("mpmc_queue_dequeue_latency_avg_seconds", snapshot.QueueName, snapshot.AvgDequeueLatency.Seconds())
		metrics += formatPrometheusMetric("mpmc_queue_dequeue_latency_p95_seconds", snapshot.QueueName, snapshot.P95DequeueLatency.Seconds())
	}

	return metrics
}

func formatPrometheusMetric(name, queueName string, value float64) string {
	return fmt.Sprintf("%s{queue=\"%s\"} %f\n", name, queueName, value)
}

func calculateAvg(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var total time.Duration
	for _, d := range durations {
		total += d
	}

	return total / time.Duration(len(durations))
}

func calculateP95(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Simple approximation - sort and get 95th percentile
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	// Bubble sort (simple for small samples)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := int(float64(len(sorted)) * 0.95)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

// Reset resets all metrics counters
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalEnqueued = 0
	m.totalDequeued = 0
	m.totalExpired = 0
	m.enqueueErrors = 0
	m.memoryLimitHits = 0
	m.enqueueLatency = make([]time.Duration, 0, m.maxLatencySamples)
	m.dequeueLatency = make([]time.Duration, 0, m.maxLatencySamples)
}
