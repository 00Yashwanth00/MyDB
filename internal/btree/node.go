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
