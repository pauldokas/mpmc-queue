package tests

import (
	"mpmc-queue/queue"
	"testing"
)

func TestChunkNodePooling(t *testing.T) {
	node1 := queue.GetChunkNode()
	if node1 == nil {
		t.Fatal("GetChunkNode returned nil")
	}

	data := queue.NewQueueData("test", "test")
	node1.Add(data)
	if node1.GetSize() != 1 {
		t.Errorf("Add failed, size: %d", node1.GetSize())
	}
	if node1.Data[0] != data {
		t.Errorf("Data[0] mismatch")
	}

	queue.PutChunkNode(node1)

	node2 := queue.GetChunkNode()

	if node2.GetSize() != 0 {
		t.Errorf("Pooled node size not reset: %d", node2.GetSize())
	}
	if node2.Data[0] != nil {
		t.Errorf("Pooled node Data[0] not nil")
	}
}
