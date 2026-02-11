package queue

import (
	"sync"
)

// chunkNodePool is a pool for recycling ChunkNode objects
// This reduces GC pressure by reusing the 8KB arrays used for chunk storage
var chunkNodePool = sync.Pool{
	New: func() any {
		return &ChunkNode{
			Data: [1000]*QueueData{},
			size: 0,
		}
	},
}

// GetChunkNode retrieves a ChunkNode from the pool
func GetChunkNode() *ChunkNode {
	return chunkNodePool.Get().(*ChunkNode)
}

// PutChunkNode returns a ChunkNode to the pool
// It clears the data to prevent memory leaks before returning
func PutChunkNode(cn *ChunkNode) {
	// Reset data array completely to avoid holding references to QueueData
	for i := range cn.Data {
		cn.Data[i] = nil
	}

	cn.setSize(0)
	chunkNodePool.Put(cn)
}
