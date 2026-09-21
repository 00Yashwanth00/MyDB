package btree

import (
	"bytes"
	"encoding/binary"
)

const (
	HEADER = 4

	BTREE_PAGE_SIZE    = 4096
	BTREE_MAX_KEY_SIZE = 1000
	BTREE_MAX_VAL_SIZE = 3000
)

const (
	BNODE_NODE = 1
	BNODE_LEAF = 2
)

func init() {
	node1max := HEADER + 8 + 2 + 4 +
		BTREE_MAX_KEY_SIZE +
		BTREE_MAX_VAL_SIZE

	if node1max > BTREE_PAGE_SIZE {
		panic("maximum KV does not fit in a B+tree page")
	}
}

type BNode []byte

// --------------------
// Header
// --------------------

func (node BNode) btype() uint16 {
	return binary.LittleEndian.Uint16(node[0:2])
}

func (node BNode) nkeys() uint16 {
	return binary.LittleEndian.Uint16(node[2:4])
}

func (node BNode) setHeader(btype uint16, nkeys uint16) {
	binary.LittleEndian.PutUint16(node[0:2], btype)
	binary.LittleEndian.PutUint16(node[2:4], nkeys)
}

// --------------------
// Child pointers
// --------------------

func (node BNode) getPtr(idx uint16) uint64 {
	if idx >= node.nkeys() {
		panic("invalid pointer index")
	}

	pos := HEADER + 8*idx

	return binary.LittleEndian.Uint64(node[pos:])
}

func (node BNode) setPtr(idx uint16, val uint64) {
	if idx >= node.nkeys() {
		panic("invalid pointer index")
	}

	pos := HEADER + 8*idx

	binary.LittleEndian.PutUint64(node[pos:], val)
}

// --------------------
// Offset list
// --------------------

func offsetPos(node BNode, idx uint16) uint16 {
	if idx < 1 || idx > node.nkeys() {
		panic("invalid offset index")
	}

	return HEADER + 8*node.nkeys() + 2*(idx-1)
}

func (node BNode) getOffset(idx uint16) uint16 {
	if idx == 0 {
		return 0
	}

	return binary.LittleEndian.Uint16(
		node[offsetPos(node, idx):],
	)
}

func (node BNode) setOffset(idx uint16, offset uint16) {
	if idx < 1 || idx > node.nkeys() {
		panic("invalid offset index")
	}

	binary.LittleEndian.PutUint16(
		node[offsetPos(node, idx):],
		offset,
	)
}

// --------------------
// Key-value pairs
// --------------------

func (node BNode) kvPos(idx uint16) uint16 {
	if idx > node.nkeys() {
		panic("invalid KV index")
	}

	return HEADER +
		8*node.nkeys() +
		2*node.nkeys() +
		node.getOffset(idx)
}

func (node BNode) getKey(idx uint16) []byte {
	if idx >= node.nkeys() {
		panic("invalid KV index")
	}

	pos := node.kvPos(idx)

	klen := binary.LittleEndian.Uint16(node[pos:])

	return node[pos+4:][:klen]
}

func (node BNode) getVal(idx uint16) []byte {
	if idx >= node.nkeys() {
		panic("invalid KV index")
	}

	pos := node.kvPos(idx)

	klen := binary.LittleEndian.Uint16(node[pos:])
	vlen := binary.LittleEndian.Uint16(node[pos+2:])

	return node[pos+4+uint16(klen):][:vlen]
}

// --------------------
// Node size
// --------------------

func (node BNode) nbytes() uint16 {
	return node.kvPos(node.nkeys())
}

// --------------------
// Lookup
// --------------------

func nodeLookupLE(node BNode, key []byte) uint16 {
	nkeys := node.nkeys()
	found := uint16(0)

	for i := uint16(1); i < nkeys; i++ {
		cmp := bytes.Compare(node.getKey(i), key)

		if cmp <= 0 {
			found = i
		}

		if cmp >= 0 {
			break
		}
	}

	return found
}

// --------------------
// Node update functions
// --------------------

func nodeAppendKV(
	new BNode,
	idx uint16,
	ptr uint64,
	key []byte,
	val []byte,
) {
	new.setPtr(idx, ptr)

	pos := new.kvPos(idx)

	binary.LittleEndian.PutUint16(
		new[pos+0:],
		uint16(len(key)),
	)

	binary.LittleEndian.PutUint16(
		new[pos+2:],
		uint16(len(val)),
	)

	copy(new[pos+4:], key)

	copy(
		new[pos+4+uint16(len(key)):],
		val,
	)

	new.setOffset(
		idx+1,
		new.getOffset(idx)+
			4+
			uint16(len(key)+len(val)),
	)
}

func nodeAppendRange(
	new BNode,
	old BNode,
	dstNew uint16,
	srcOld uint16,
	n uint16,
) {
	for i := uint16(0); i < n; i++ {
		idxOld := srcOld + i
		idxNew := dstNew + i

		nodeAppendKV(
			new,
			idxNew,
			old.getPtr(idxOld),
			old.getKey(idxOld),
			old.getVal(idxOld),
		)
	}
}

func leafInsert(
	new BNode,
	old BNode,
	idx uint16,
	key []byte,
	val []byte,
) {
	new.setHeader(
		BNODE_LEAF,
		old.nkeys()+1,
	)

	nodeAppendRange(
		new,
		old,
		0,
		0,
		idx,
	)

	nodeAppendKV(
		new,
		idx,
		0,
		key,
		val,
	)

	nodeAppendRange(
		new,
		old,
		idx+1,
		idx,
		old.nkeys()-idx,
	)
}

func nodeReplaceKidN(
	tree *BTree,
	new BNode,
	old BNode,
	idx uint16,
	kids ...BNode,
) {
	inc := uint16(len(kids))

	new.setHeader(
		BNODE_NODE,
		old.nkeys()+inc-1,
	)

	nodeAppendRange(
		new,
		old,
		0,
		0,
		idx,
	)

	for i, node := range kids {
		nodeAppendKV(
			new,
			idx+uint16(i),
			tree.new(node),
			node.getKey(0),
			nil,
		)
	}

	nodeAppendRange(
		new,
		old,
		idx+inc,
		idx+1,
		old.nkeys()-(idx+1),
	)
}

func nodeSplit2(left BNode, right BNode, old BNode) {
	//code omitted
	if old.nkeys() < 2 {
		panic("cannot split a node with fewer than 2 keys")
	}

	nleft := old.nkeys() / 2

	leftBytes := func() uint16 {
		return HEADER +
			8*nleft +
			2*nleft +
			old.getOffset(nleft)
	}

	for leftBytes() > BTREE_PAGE_SIZE {
		nleft--
	}

	if nleft < 1 {
		panic("left node would have no keys")
	}

	rightBytes := func() uint16 {
		return old.nbytes() - leftBytes() + HEADER
	}

	for rightBytes() > BTREE_PAGE_SIZE {
		nleft++
	}

	if nleft >= old.nkeys() {
		panic("right node would have no keys")
	}

	nright := old.nkeys() - nleft

	left.setHeader(old.btype(), nleft)
	right.setHeader(old.btype(), nright)

	nodeAppendRange(left, old, 0, 0, nleft)

	nodeAppendRange(right, old, 0, nleft, nright)

	if right.nbytes() > BTREE_PAGE_SIZE {
		panic("right node exceeds page size")
	}
}

func nodeSplit3(old BNode) (uint16, [3]BNode) {
	if old.nbytes() <= BTREE_PAGE_SIZE {
		old = old[:BTREE_PAGE_SIZE]
		return 1, [3]BNode{old}
	}

	left := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	right := BNode(make([]byte, BTREE_PAGE_SIZE))

	nodeSplit2(left, right, old)

	if left.nbytes() <= BTREE_PAGE_SIZE {
		left = left[:BTREE_PAGE_SIZE]
		return 2, [3]BNode{left, right}
	}

	leftleft := BNode(make([]byte, BTREE_PAGE_SIZE))
	middle := BNode(make([]byte, BTREE_PAGE_SIZE))

	nodeSplit2(leftleft, middle, left)

	if leftleft.nbytes() > BTREE_PAGE_SIZE {
		panic("leftleft node exceeds page size")
	}

	return 3, [3]BNode{leftleft, middle, right}
}
