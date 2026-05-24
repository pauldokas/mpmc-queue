package queue

// GetChunkNode creates a new ChunkNode
func GetChunkNode() *ChunkNode {
	return &ChunkNode{
		Data: [1000]*QueueData{},
		size: 0,
	}
}

// PutChunkNode clears the node to help GC and discards it
func PutChunkNode(cn *ChunkNode) {
	if cn == nil {
		return
	}
	// Reset data array completely to prevent memory leaks before discarding
	for i := range cn.Data {
		cn.Data[i] = nil
	}
	cn.setSize(0)
}
