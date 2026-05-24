package queue

import (
	"sync"
	"sync/atomic"
)

var chunkNodePool = sync.Pool{
	New: func() any {
		return &ChunkNode{
			size: 0,
		}
	},
}

// GetChunkNode creates a new ChunkNode
func GetChunkNode() *ChunkNode {
	cn := chunkNodePool.Get().(*ChunkNode)
	cn.pooled.Store(false)
	return cn
}

// PutChunkNode clears the node to help GC and discards it
func PutChunkNode(cn *ChunkNode) {
	if cn == nil {
		return
	}
	// Reset data array completely to prevent memory leaks before returning to pool
	for i := range cn.Data {
		cn.Data[i].Store(nil)
	}
	cn.setSize(0)
	atomic.StoreInt32(&cn.head, 0)
	cn.NextElement.Store(nil)
	cn.pooled.Store(true)
	chunkNodePool.Put(cn)
}
