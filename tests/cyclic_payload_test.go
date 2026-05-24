package tests

import (
	"testing"
	"mpmc-queue/queue"
)

type CyclicStruct struct {
	Value int
	Next  *CyclicStruct
}

func TestCyclicPayloadMemoryEstimation(t *testing.T) {
	q := queue.NewQueue("cyclic-test")
	defer q.Close()

	node1 := &CyclicStruct{Value: 1}
	node2 := &CyclicStruct{Value: 2}
	node1.Next = node2
	node2.Next = node1

	err := q.Enqueue(node1)
	if err != nil {
		t.Fatalf("Failed to enqueue cyclic payload: %v", err)
	}

	var cyclicSlice []interface{}
	cyclicSlice = append(cyclicSlice, 1)
	cyclicSlice = append(cyclicSlice, &cyclicSlice)

	err = q.Enqueue(cyclicSlice)
	if err != nil {
		t.Fatalf("Failed to enqueue cyclic slice: %v", err)
	}
}
