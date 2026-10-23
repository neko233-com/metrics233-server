package heap

import "testing"

type integer int

func (i integer) Less(j integer) bool { return i < j }

func TestMaximumIsNotAssumedToBeTheLastHeapLeaf(t *testing.T) {
	h := Heap[integer]{}
	h.Init([]integer{1, 10, 2, 9, 3})
	for _, expected := range []integer{10, 9, 3, 2, 1} {
		if h.Max() != expected || h.PopLast() != expected {
			t.Fatal("bounded candidate pruning discarded the wrong neighbor")
		}
	}
}
