package btree

import (
	"bytes"
	"encoding/binary"
)

// Constants define the fixed sizes and constraints for the B+ tree layout.
const (
	HEADER = 4 // The node header consumes 4 bytes.

	BTREE_PAGE_SIZE    = 4096 // Nodes are constrained to a 4KB page size.
	BTREE_MAX_KEY_SIZE = 1000 // Keys are limited to 1000 bytes.
	BTREE_MAX_VAL_SIZE = 3000 // Values are limited to 3000 bytes.
)

// Node type identifiers.
const (
	BNODE_NODE = 1 // Represents an internal node with child pointers.
	BNODE_LEAF = 2 // Represents a leaf node holding key-value pairs.
)

func init() {
	// Validates that a node with at least one maximum-sized key-value pair will fit safely within a single 4096-byte page.
	node1max := HEADER + 8 + 2 + 4 +
		BTREE_MAX_KEY_SIZE +
		BTREE_MAX_VAL_SIZE

	if node1max > BTREE_PAGE_SIZE {
		panic("maximum KV does not fit in a B+tree page") // Triggers a panic on startup if page size constraints are violated.
	}
}

// BNode acts as a byte-slice wrapper representing a single page/node in the B+ tree.
type BNode []byte

// --------------------
// Header
// --------------------

// btype reads the first 2 bytes of the header as a LittleEndian uint16 to determine if it is a leaf or internal node.
func (node BNode) btype() uint16 {
	return binary.LittleEndian.Uint16(node[0:2])
}

// nkeys reads bytes 2-4 of the header as a LittleEndian uint16 to get the total number of keys in this node.
func (node BNode) nkeys() uint16 {
	return binary.LittleEndian.Uint16(node[2:4])
}

// setHeader writes the node type and key count into the 4-byte header space.
func (node BNode) setHeader(btype uint16, nkeys uint16) {
	binary.LittleEndian.PutUint16(node[0:2], btype)
	binary.LittleEndian.PutUint16(node[2:4], nkeys)
}

// --------------------
// Child pointers
// --------------------

// getPtr retrieves the 8-byte uint64 pointer for a child node at the given index[cite: 2].
// Pointers are stored immediately following the 4-byte header[cite: 2].
func (node BNode) getPtr(idx uint16) uint64 {
	if idx >= node.nkeys() {
		panic("invalid pointer index")
	}

	pos := HEADER + 8*idx

	return binary.LittleEndian.Uint64(node[pos:])
}

// setPtr writes an 8-byte uint64 pointer to a child node at the specified index[cite: 2].
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

// offsetPos calculates the byte position of a KV offset[cite: 2].
// Offsets are stored after the header and all 8-byte child pointers[cite: 2].
func offsetPos(node BNode, idx uint16) uint16 {
	if idx < 1 || idx > node.nkeys() {
		panic("invalid offset index")
	}

	return HEADER + 8*node.nkeys() + 2*(idx-1)
}

// getOffset returns the byte offset for the KV pair at idx relative to the start of the KV section[cite: 2].
// Index 0 always has an offset of 0[cite: 2].
func (node BNode) getOffset(idx uint16) uint16 {
	if idx == 0 {
		return 0
	}

	return binary.LittleEndian.Uint16(
		node[offsetPos(node, idx):],
	)
}

// setOffset writes a 2-byte relative offset for a specific KV pair[cite: 2].
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

// kvPos calculates the absolute byte position of a KV pair within the node by summing the header, pointers, offset list sizes, and the specific KV's relative offset[cite: 2].
func (node BNode) kvPos(idx uint16) uint16 {
	if idx > node.nkeys() {
		panic("invalid KV index")
	}

	return HEADER +
		8*node.nkeys() +
		2*node.nkeys() +
		node.getOffset(idx)
}

// getKey extracts the key byte slice at the given index[cite: 2].
func (node BNode) getKey(idx uint16) []byte {
	if idx >= node.nkeys() {
		panic("invalid KV index")
	}

	pos := node.kvPos(idx)

	// The first 2 bytes of the KV structure indicate the key length[cite: 2].
	klen := binary.LittleEndian.Uint16(node[pos:])

	// Returns a slice referencing the key data which starts 4 bytes into the KV structure[cite: 2].
	return node[pos+4:][:klen]
}

// getVal extracts the value byte slice at the given index[cite: 2].
func (node BNode) getVal(idx uint16) []byte {
	if idx >= node.nkeys() {
		panic("invalid KV index")
	}

	pos := node.kvPos(idx)

	// KV structure: 2 bytes for key length, 2 bytes for value length[cite: 2].
	klen := binary.LittleEndian.Uint16(node[pos:])
	vlen := binary.LittleEndian.Uint16(node[pos+2:])

	// Returns a slice referencing the value data, which sits immediately after the key data[cite: 2].
	return node[pos+4+uint16(klen):][:vlen]
}

// --------------------
// Node size
// --------------------

// nbytes returns the total used size of the node by calculating the position just past the final key[cite: 2].
func (node BNode) nbytes() uint16 {
	return node.kvPos(node.nkeys())
}

// --------------------
// Lookup
// --------------------

// nodeLookupLE performs a linear search to find the index of the first key that is greater than or equal to the target key[cite: 2].
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

// nodeAppendKV writes a new child pointer, key, and value to the specified index in the new node, updating the offset list accordingly[cite: 2].
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

// nodeAppendRange copies multiple contiguous KV pairs and pointers from an old node to a new node[cite: 2].
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

// leafInsert inserts a new KV pair into a leaf node[cite: 2].
// It utilizes copy-on-write, appending preceding keys, the new key, and subsequent keys into a newly allocated node slice[cite: 2].
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

	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendKV(new, idx, 0, key, val)
	nodeAppendRange(new, old, idx+1, idx, old.nkeys()-idx)
}

// nodeReplaceKidN replaces a single child link in an internal node with multiple new child links (typically after a split)[cite: 2].
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

	nodeAppendRange(new, old, 0, 0, idx)

	for i, node := range kids {
		nodeAppendKV(
			new,
			idx+uint16(i),
			tree.new(node), // Persists the new child node and obtains its pointer.
			node.getKey(0),
			nil,
		)
	}

	nodeAppendRange(new, old, idx+inc, idx+1, old.nkeys()-(idx+1))
}

// nodeSplit2 takes an oversized node and divides it roughly in half by byte size, placing the contents into left and right nodes[cite: 2].
func nodeSplit2(left BNode, right BNode, old BNode) {
	if old.nkeys() < 2 {
		panic("cannot split a node with fewer than 2 keys")
	}

	nleft := old.nkeys() / 2

	// Calculates the byte size of the left partition[cite: 2].
	leftBytes := func() uint16 {
		return HEADER +
			8*nleft +
			2*nleft +
			old.getOffset(nleft)
	}

	// Adjusts the split point if the left side exceeds the page limit[cite: 2].
	for leftBytes() > BTREE_PAGE_SIZE {
		nleft--
	}

	if nleft < 1 {
		panic("left node would have no keys")
	}

	// Adjusts the split point if the right side exceeds the page limit[cite: 2].
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

// nodeSplit3 handles extreme cases where a single insertion causes an oversized node to split into up to three separate nodes[cite: 2].
func nodeSplit3(old BNode) (uint16, [3]BNode) {
	// If the node hasn't exceeded limits, return it un-split[cite: 2].
	if old.nbytes() <= BTREE_PAGE_SIZE {
		old = old[:BTREE_PAGE_SIZE]
		return 1, [3]BNode{old}
	}

	left := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	right := BNode(make([]byte, BTREE_PAGE_SIZE))

	nodeSplit2(left, right, old)

	// If splitting in two resolved the overflow, return 2 nodes[cite: 2].
	if left.nbytes() <= BTREE_PAGE_SIZE {
		left = left[:BTREE_PAGE_SIZE]
		return 2, [3]BNode{left, right}
	}

	// If the left node is still oversized, split it again to yield 3 total nodes[cite: 2].
	leftleft := BNode(make([]byte, BTREE_PAGE_SIZE))
	middle := BNode(make([]byte, BTREE_PAGE_SIZE))

	nodeSplit2(leftleft, middle, left)

	if leftleft.nbytes() > BTREE_PAGE_SIZE {
		panic("leftleft node exceeds page size")
	}

	return 3, [3]BNode{leftleft, middle, right}
}

// leafUpdate modifies an existing key's value in a leaf node by copying surrounding data and writing the new value into the new slice[cite: 2].
func leafUpdate(
	new BNode,
	old BNode,
	idx uint16,
	key []byte,
	val []byte,
) {
	new.setHeader(BNODE_LEAF, old.nkeys())

	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendKV(new, idx, 0, key, val)
	nodeAppendRange(new, old, idx+1, idx+1, old.nkeys()-(idx+1))
}

// treeInsert acts as the recursive entrypoint for inserts, traversing down to the leaf and propagating splits upward[cite: 2].
func treeInsert(
	tree *BTree,
	node BNode,
	key []byte,
	val []byte,
) BNode {

	// Allocate a temporarily oversized node to hold the insertion before a potential split[cite: 2].
	new := BNode(make([]byte, 2*BTREE_PAGE_SIZE))

	idx := nodeLookupLE(node, key)

	switch node.btype() {
	case BNODE_LEAF:
		// If at a leaf, either update in-place or insert a new KV pair[cite: 2].
		if bytes.Equal(key, node.getKey(idx)) {
			leafUpdate(new, node, idx, key, val)
		} else {
			leafInsert(new, node, idx+1, key, val)
		}

	case BNODE_NODE:
		// If at an internal node, recursively traverse downwards[cite: 2].
		nodeInsert(tree, new, node, idx, key, val)

	default:
		panic("bad node type")
	}

	return new
}

// nodeInsert handles insertions into internal nodes, tracking splits propagated from children[cite: 2].
func nodeInsert(
	tree *BTree,
	new BNode,
	node BNode,
	idx uint16,
	key []byte,
	val []byte,
) {
	kptr := node.getPtr(idx)

	// Traverse into the child pointer[cite: 2].
	knode := treeInsert(
		tree,
		tree.get(kptr),
		key,
		val,
	)

	// Check if the child exceeded page limits and needs splitting[cite: 2].
	nsplit, split := nodeSplit3(knode)

	tree.del(kptr) // Clean up the old child page[cite: 2].

	// Replace the old child link with links to the newly split children[cite: 2].
	nodeReplaceKidN(
		tree,
		new,
		node,
		idx,
		split[:nsplit]...,
	)
}

// leafDelete removes a KV pair from a leaf node by copying all data except the target index into a new node[cite: 2].
func leafDelete(new BNode, old BNode, idx uint16) {
	new.setHeader(BNODE_LEAF, old.nkeys()-1)
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendRange(new, old, idx, idx+1, old.nkeys()-(idx+1))
}

// nodeMerge combines two undersized sibling nodes into a single cohesive node[cite: 2].
func nodeMerge(new BNode, left BNode, right BNode) {
	new.setHeader(left.btype(), left.nkeys()+right.nkeys())
	nodeAppendRange(new, left, 0, 0, left.nkeys())
	nodeAppendRange(new, right, left.nkeys(), 0, right.nkeys())
}

// nodeReplace2Kid simplifies an internal node by replacing two child pointers (representing unmerged siblings) with a single pointer to a newly merged node[cite: 2].
func nodeReplace2Kid(
	new BNode,
	old BNode,
	idx uint16,
	ptr uint64,
	key []byte,
) {
	new.setHeader(BNODE_NODE, old.nkeys()-1)
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendKV(new, idx, ptr, key, nil)
	nodeAppendRange(new, old, idx+1, idx+2, old.nkeys()-(idx+2))
}

// shouldMerge checks if a recently updated child has dropped below the threshold capacity (1/4th of a page) and determines if it can safely merge with its left or right sibling[cite: 2].
func shouldMerge(
	tree *BTree,
	node BNode,
	idx uint16,
	updated BNode,
) (int, BNode) {

	// Do not merge if the updated node is larger than a quarter page[cite: 2].
	if updated.nbytes() > BTREE_PAGE_SIZE/4 {
		return 0, BNode{}
	}

	// Try the left sibling[cite: 2].
	if idx > 0 {
		sibling := BNode(tree.get(node.getPtr(idx - 1)))
		merged := sibling.nbytes() + updated.nbytes() - HEADER
		if merged <= BTREE_PAGE_SIZE {
			return -1, sibling // -1 indicates merge with left[cite: 2].
		}
	}

	// Try the right sibling[cite: 2].
	if idx+1 < node.nkeys() {
		sibling := BNode(tree.get(node.getPtr(idx + 1)))
		merged := sibling.nbytes() + updated.nbytes() - HEADER
		if merged <= BTREE_PAGE_SIZE {
			return +1, sibling // +1 indicates merge with right[cite: 2].
		}
	}

	return 0, BNode{}
}

// treeDelete is the recursive entrypoint for deleting a key[cite: 2].
func treeDelete(
	tree *BTree,
	node BNode,
	key []byte,
) BNode {

	idx := nodeLookupLE(node, key)

	switch node.btype() {
	case BNODE_LEAF:
		if !bytes.Equal(node.getKey(idx), key) {
			return BNode{} // Key not found, return empty node[cite: 2].
		}

		new := BNode(make([]byte, BTREE_PAGE_SIZE))
		leafDelete(new, node, idx)
		return new
	case BNODE_NODE:
		return nodeDelete(tree, node, idx, key)
	default:
		panic("bad node!")
	}
}

// nodeDelete handles deletions within internal nodes, recursively calling treeDelete on children and triggering merges if necessary[cite: 2].
func nodeDelete(
	tree *BTree,
	node BNode,
	idx uint16,
	key []byte,
) BNode {
	kptr := node.getPtr(idx)
	updated := treeDelete(tree, tree.get(kptr), key)

	if len(updated) == 0 {
		return BNode{}
	}

	tree.del(kptr) // Mark old child for deletion[cite: 2].
	new := BNode(make([]byte, BTREE_PAGE_SIZE))
	mergeDir, sibling := shouldMerge(tree, node, idx, updated)

	switch {
	case mergeDir < 0:
		// Logic to merge the updated node with its left sibling[cite: 2].
		merged := BNode(make([]byte, BTREE_PAGE_SIZE))
		nodeMerge(merged, sibling, updated)
		tree.del(node.getPtr(idx - 1))
		nodeReplace2Kid(new, node, idx-1, tree.new(merged), merged.getKey(0))

	case mergeDir > 0:
		// Logic to merge the updated node with its right sibling[cite: 2].
		merged := BNode(make([]byte, BTREE_PAGE_SIZE))
		nodeMerge(merged, updated, sibling)
		tree.del(node.getPtr(idx + 1))
		nodeReplace2Kid(new, node, idx, tree.new(merged), merged.getKey(0))

	case mergeDir == 0 && updated.nkeys() == 0:
		new.setHeader(BNODE_NODE, 0) // Propagate an empty node upward if no sibling exists[cite: 2].

	case mergeDir == 0 && updated.nkeys() > 0:
		nodeReplaceKidN(tree, new, node, idx, updated) // Simply replace the updated child without merging[cite: 2].
	}

	return new
}
