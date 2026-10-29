package verthash

import (
	"encoding/binary"
	"errors"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/sha3"
)

const hashSize = 32
const nodeSize = hashSize

type Graph struct {
	pk    []byte
	fn    string
	f     *os.File
	data  []byte // the file, memory-mapped; node n is at bfsToPost(n)*nodeSize
	index int64
	log2  int64
	pow2  int64
	size  int64
	done  int64 // nodes generated so far, read by the progress reporter

	in [4 * hashSize]byte // hash input: pk, varint id, up to two parents
}

type Node struct {
	H []byte // hash at the file
}

func (n *Node) MarshalBinary() ([]byte, error) {
	return n.H, nil
}

func (n *Node) UnmarshalBinary(data []byte) error {
	n.H = data
	return nil
}

func Log2(x int64) int64 {
	r := int64(0)
	for ; x > 1; x >>= 1 {
		r++
	}

	return r
}

func newGraphParams(index int64, fn string, pk []byte) *Graph {
	size := numXi(index)
	log2 := Log2(size) + 1
	pow2 := int64(1 << log2)

	g := &Graph{
		pk:    pk,
		fn:    fn,
		index: index,
		log2:  log2,
		size:  size,
		pow2:  pow2,
	}
	copy(g.in[:hashSize], pk)
	return g
}

// Generate a new PoS graph of index
// Currently only supports the weaker PoS graph
// Note that this graph will have O(2^index) nodes
// If fn already exists it is opened as is, without checking it.
func NewGraph(index int64, fn string, pk []byte, progress chan float64) *Graph {
	if _, err := os.Stat(fn); err != nil {
		if err := generateGraph(index, fn, pk, progress); err != nil {
			panic(err)
		}
	}
	g, err := openGraph(index, fn, pk)
	if err != nil {
		panic(err)
	}
	return g
}

// generateGraph writes the graph to fn+".tmp" through a memory map and renames
// it to fn once it is complete, so an interrupted run never leaves a partial fn.
func generateGraph(index int64, fn string, pk []byte, progress chan float64) (err error) {
	tmp := fn + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		if f != nil {
			f.Close()
		}
		if err != nil {
			os.Remove(tmp)
		}
	}()

	g := newGraphParams(index, fn, pk)
	length := g.size * nodeSize
	if err = f.Truncate(length); err != nil {
		return err
	}
	if g.data, err = mapFile(f, length); err != nil {
		return err
	}

	done := make(chan bool, 1)
	go func() {
		g.XiGraphIter(index)
		done <- true
	}()

	ticker := time.NewTicker(1 * time.Second)
	for genDone := false; !genDone; {
		select {
		case genDone = <-done:
		case <-ticker.C:
			if progress != nil {
				progress <- float64(atomic.LoadInt64(&g.done)) / float64(g.size)
			}
		}
	}
	ticker.Stop()

	err = flushMap(f, g.data)
	if uerr := unmapFile(g.data); err == nil {
		err = uerr
	}
	g.data = nil
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	f = nil
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, fn); err != nil {
		return err
	}
	// the ticker's last report is up to a second before the end
	if progress != nil {
		progress <- 1
	}
	return nil
}

// openGraph maps an existing graph file read-write.
func openGraph(index int64, fn string, pk []byte) (*Graph, error) {
	f, err := os.OpenFile(fn, os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if st.Size() == 0 {
		f.Close()
		return nil, errors.New("verthash: graph file is empty")
	}
	g := newGraphParams(index, fn, pk)
	if g.data, err = mapFile(f, st.Size()); err != nil {
		f.Close()
		return nil, err
	}
	g.f = f
	return g, nil
}

// compute parents of nodes
func (g *Graph) GetParents(node, index int64) []int64 {
	if node < int64(1<<uint64(index)) {
		return nil
	}

	offset0, offset1 := g.GetGraph(node, index)

	var res []int64
	if offset0 != 0 {
		res = append(res, node-offset0)
	}
	if offset1 != 0 {
		res = append(res, node-offset1)
	}
	return res
}

// compute the offsets for the two parents in the butterfly graph
func (g *Graph) ButterflyParents(begin, node, index int64) (int64, int64) {
	pow2index_1 := int64(1 << uint64(index-1))
	level := (node - begin) / pow2index_1
	var prev int64
	shift := (index - 1) - level
	if level > (index - 1) {
		shift = level - (index - 1)
	}
	i := (node - begin) % pow2index_1
	if (i>>uint64(shift))&1 == 0 {
		prev = i + (1 << uint64(shift))
	} else {
		prev = i - (1 << uint64(shift))
	}
	parent0 := begin + (level-1)*pow2index_1 + prev
	parent1 := node - pow2index_1
	return parent0, parent1
}

// get graph that node belongs to, so i can find the parents
func (g *Graph) GetGraph(node, index int64) (int64, int64) {
	if index == 1 {
		if node < 2 {
			return 2, 0
		} else if node == 2 {
			return 1, 2
		} else if node == 3 {
			return 3, 2
		}
	}

	pow2index := int64(1 << uint64(index))
	pow2index_1 := int64(1 << uint64(index-1))
	sources := pow2index
	firstButter := sources + numButterfly(index-1)
	firstXi := firstButter + numXi(index-1)
	secondXi := firstXi + numXi(index-1)
	secondButter := secondXi + numButterfly(index-1)
	sinks := secondButter + sources

	if node < sources {
		return pow2index, 0
	} else if node >= sources && node < firstButter {
		if node < sources+pow2index_1 {
			return pow2index, pow2index_1
		} else {
			parent0, parent1 := g.ButterflyParents(sources, node, index)
			return node - parent0, node - parent1
		}
	} else if node >= firstButter && node < firstXi {
		node = node - firstButter
		return g.GetGraph(node, index-1)
	} else if node >= firstXi && node < secondXi {
		node = node - firstXi
		return g.GetGraph(node, index-1)
	} else if node >= secondXi && node < secondButter {
		if node < secondXi+pow2index_1 {
			return pow2index_1, 0
		} else {
			parent0, parent1 := g.ButterflyParents(secondXi, node, index)
			return node - parent0, node - parent1
		}
	} else if node >= secondButter && node < sinks {
		offset := (node - secondButter) % pow2index_1
		parent1 := sinks - numXi(index) + offset
		if offset+secondButter == node {
			return pow2index_1, node - parent1
		} else {
			return pow2index, node - parent1 - pow2index_1
		}
	} else {
		return 0, 0
	}
}

func (g *Graph) NewNodeById(id int64, hash []byte) {
	node := &Node{
		H: hash,
	}
	g.WriteId(node, id)
}

func (g *Graph) NewNode(id int64, hash []byte) {
	node := &Node{
		H: hash,
	}
	g.WriteNode(node, id)
}

func (g *Graph) GetId(id int64) *Node {
	data := make([]byte, nodeSize)
	copy(data, g.data[id*nodeSize:(id+1)*nodeSize])
	return &Node{H: data}
}

func (g *Graph) WriteId(node *Node, id int64) {
	if len(node.H) != nodeSize {
		panic("verthash: node is not 32 bytes")
	}
	copy(g.data[id*nodeSize:(id+1)*nodeSize], node.H)
}

func (g *Graph) GetNode(id int64) *Node {
	idx := g.bfsToPost(id)
	return g.GetId(idx)
}

func (g *Graph) WriteNode(node *Node, id int64) {
	idx := g.bfsToPost(id)
	g.WriteId(node, idx)
}

func (g *Graph) Close() {
	if g.data != nil {
		unmapFile(g.data)
		g.data = nil
	}
	if g.f != nil {
		g.f.Close()
		g.f = nil
	}
}

// post-order is better for disk than bfs~
func (g *Graph) bfsToPost(node int64) int64 {
	return node & (^(g.pow2))
}

// node returns node id's bytes in the mapped file.
func (g *Graph) node(id int64) []byte {
	off := g.bfsToPost(id) * nodeSize
	return g.data[off : off+nodeSize : off+nodeSize]
}

// hashNode sets node id to SHA3-256(pk || varint(id) padded to 32 bytes || parents...).
func (g *Graph) hashNode(id int64, parents ...[]byte) {
	vi := g.in[hashSize : 2*hashSize]
	for i := range vi {
		vi[i] = 0
	}
	binary.PutVarint(vi, id)
	n := 2 * hashSize
	for _, p := range parents {
		n += copy(g.in[n:], p)
	}
	hash := sha3.Sum256(g.in[:n])
	copy(g.node(id), hash[:])
}

func numXi(index int64) int64 {
	return (1 << uint64(index)) * (index + 1) * index
}

func numButterfly(index int64) int64 {
	return 2 * (1 << uint64(index)) * index
}

func (g *Graph) ButterflyGraph(index int64, count *int64) {
	if index == 0 {
		index = 1
	}
	numLevel := 2 * index
	perLevel := int64(1 << uint64(index))
	begin := *count - perLevel // level 0 created outside
	// no parents at level 0
	var level, i int64
	for level = 1; level < numLevel; level++ {
		for i = 0; i < perLevel; i++ {
			var prev int64
			shift := index - level
			if level > numLevel/2 {
				shift = level - numLevel/2
			}
			if (i>>uint64(shift))&1 == 0 {
				prev = i + (1 << uint64(shift))
			} else {
				prev = i - (1 << uint64(shift))
			}
			parent0 := g.node(begin + (level-1)*perLevel + prev)
			parent1 := g.node(*count - perLevel)

			g.hashNode(*count, parent0, parent1)
			*count++
		}
		atomic.StoreInt64(&g.done, *count-g.pow2)
	}
}

// Iterative generation of the graph
func (g *Graph) XiGraphIter(index int64) {
	count := g.pow2

	stack := []int64{index, index, index, index, index}
	graphStack := []int{4, 3, 2, 1, 0}

	var i int64
	graph := 0
	pow2index := int64(1 << uint64(index))
	for i = 0; i < pow2index; i++ { //sources at this level
		g.hashNode(count)
		count++
	}

	if index == 1 {
		g.ButterflyGraph(index, &count)
		return
	}

	for len(stack) != 0 && len(graphStack) != 0 {
		index, stack = stack[len(stack)-1], stack[:len(stack)-1]
		graph, graphStack = graphStack[len(graphStack)-1], graphStack[:len(graphStack)-1]

		indices := []int64{index - 1, index - 1, index - 1, index - 1, index - 1}
		graphs := []int{4, 3, 2, 1, 0}

		pow2index := int64(1 << uint64(index))
		pow2index_1 := int64(1 << uint64(index-1))

		if graph == 0 {
			sources := count - pow2index
			// sources to sources of first butterfly
			// create sources of first butterly
			for i = 0; i < pow2index_1; i++ {
				g.hashNode(count, g.node(sources+i), g.node(sources+i+pow2index_1))
				count++
			}
		} else if graph == 1 {
			firstXi := count
			// sinks of first butterfly to sources of first xi graph
			for i = 0; i < pow2index_1; i++ {
				nodeId := firstXi + i
				// index is the last level; i.e., sinks
				g.hashNode(nodeId, g.node(firstXi-pow2index_1+i))
				count++
			}
		} else if graph == 2 {
			secondXi := count
			// sinks of first xi to sources of second xi
			for i = 0; i < pow2index_1; i++ {
				nodeId := secondXi + i
				g.hashNode(nodeId, g.node(secondXi-pow2index_1+i))
				count++
			}
		} else if graph == 3 {
			secondButter := count
			// sinks of second xi to sources of second butterfly
			for i = 0; i < pow2index_1; i++ {
				nodeId := secondButter + i
				g.hashNode(nodeId, g.node(secondButter-pow2index_1+i))
				count++
			}
		} else {
			sinks := count
			sources := sinks + pow2index - numXi(index)
			for i = 0; i < pow2index_1; i++ {
				nodeId0 := sinks + i
				nodeId1 := sinks + i + pow2index_1
				parent0 := g.node(sinks - pow2index_1 + i)

				g.hashNode(nodeId0, parent0, g.node(sources+i))
				g.hashNode(nodeId1, parent0, g.node(sources+i+pow2index_1))
				count += 2
			}
		}
		atomic.StoreInt64(&g.done, count-g.pow2)

		if (graph == 0 || graph == 3) ||
			((graph == 1 || graph == 2) && index == 2) {
			g.ButterflyGraph(index-1, &count)
		} else if graph == 1 || graph == 2 {
			stack = append(stack, indices...)
			graphStack = append(graphStack, graphs...)
		}
	}
}

func (g *Graph) XiGraph(index int64, count *int64) {
	// recursively generate graphs
	// compute hashes along the way

	pow2index := int64(1 << uint64(index))

	// the first sources
	// if index == 1, then this will generate level 0 of the butterfly
	var i int64

	if *count == g.pow2 {
		for i = 0; i < pow2index; i++ {
			g.hashNode(*count)
			*count++
		}
	}

	if index == 1 {
		g.ButterflyGraph(index, count)
		return
	}

	sources := *count - pow2index
	firstButter := sources + pow2index
	firstXi := firstButter + numButterfly(index-1)
	secondXi := firstXi + numXi(index-1)
	secondButter := secondXi + numXi(index-1)
	sinks := secondButter + numButterfly(index-1)
	pow2index_1 := int64(1 << uint64(index-1))

	// sources to sources of first butterfly
	// create sources of first butterly
	for i = 0; i < pow2index_1; i++ {
		g.hashNode(*count, g.node(sources+i), g.node(sources+i+pow2index_1))
		*count++
	}

	g.ButterflyGraph(index-1, count)
	// sinks of first butterfly to sources of first xi graph
	for i = 0; i < pow2index_1; i++ {
		nodeId := firstXi + i
		g.hashNode(nodeId, g.node(firstXi-pow2index_1+i))
		*count++
	}

	g.XiGraph(index-1, count)
	// sinks of first xi to sources of second xi
	for i = 0; i < pow2index_1; i++ {
		nodeId := secondXi + i
		g.hashNode(nodeId, g.node(secondXi-pow2index_1+i))
		*count++
	}

	g.XiGraph(index-1, count)
	// sinks of second xi to sources of second butterfly
	for i = 0; i < pow2index_1; i++ {
		nodeId := secondButter + i
		g.hashNode(nodeId, g.node(secondButter-pow2index_1+i))
		*count++
	}

	// generate sinks
	// sinks of second butterfly to sinks
	// and sources to sinks directly
	g.ButterflyGraph(index-1, count)
	for i = 0; i < pow2index_1; i++ {
		nodeId0 := sinks + i
		nodeId1 := sinks + i + pow2index_1
		parent0 := g.node(sinks - pow2index_1 + i)

		g.hashNode(nodeId0, parent0, g.node(sources+i))
		g.hashNode(nodeId1, parent0, g.node(sources+i+pow2index_1))
		*count += 2
	}
}
