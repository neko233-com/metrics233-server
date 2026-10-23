package hnsw

import "testing"

func TestLayerSearchExploresQueuedBridgeBeforeDeclaringLocalMinimum(t *testing.T) {
	start := &layerNode[int]{Node: MakeNode(1, []float32{10}), neighbors: map[int]*layerNode[int]{}}
	bridge := &layerNode[int]{Node: MakeNode(2, []float32{12}), neighbors: map[int]*layerNode[int]{}}
	nearest := &layerNode[int]{Node: MakeNode(3, []float32{0}), neighbors: map[int]*layerNode[int]{}}
	start.neighbors[2] = bridge
	bridge.neighbors[3] = nearest
	hits := start.search(1, 3, []float32{0}, EuclideanDistance)
	if len(hits) != 1 || hits[0].node.Key != 3 {
		t.Fatal("search stopped despite an unexplored bridge within the configured beam")
	}
}
