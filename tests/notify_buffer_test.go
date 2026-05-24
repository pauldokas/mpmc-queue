package tests

import (
	"mpmc-queue/queue"
	"sync"
	"testing"
	"time"
)

func TestNotificationBufferFull(t *testing.T) {
	q := queue.NewQueue("notify-buffer-test")
	defer q.Close()

	for i := 0; i < 200; i++ {
		q.TryEnqueue(i)
	}

	var consumers []*queue.Consumer
	for i := 0; i < 200; i++ {
		consumers = append(consumers, q.AddConsumer())
	}

	q2 := queue.NewQueue("notify-buffer-test-2")
	defer q2.Close()

	var consumers2 []*queue.Consumer
	var wg sync.WaitGroup

	for i := 0; i < 200; i++ {
		c := q2.AddConsumer()
		consumers2 = append(consumers2, c)
		wg.Add(1)
		go func(consumer *queue.Consumer) {
			defer wg.Done()
			consumer.Read()
		}(c)
	}

	time.Sleep(100 * time.Millisecond)

	var payloads []any
	for i := 0; i < 200; i++ {
		payloads = append(payloads, i)
	}
	q2.TryEnqueueBatch(payloads)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Consumers are deadlocked because of notification channel buffer size limit!")
	}
}
