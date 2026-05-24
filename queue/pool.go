package queue

// GetChunkNode creates a new ChunkNode
func GetChunkNode() *ChunkNode {
	cn := &ChunkNode{
		size: 0,
	}
	cn.pooled.Store(false)
	return cn
}

func PutChunkNode(cn *ChunkNode) {
	if cn == nil {
		return
	}
	
	cn.pooled.Store(true)
}
