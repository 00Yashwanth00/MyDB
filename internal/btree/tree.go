// tree_2.go
package btree

// BTree represents a B+ tree structure, abstracting away the physical storage (disk or memory) through callback functions[cite: 4].
type BTree struct {
	// root stores the 64-bit address or ID of the tree's root node page[cite: 4].
	root uint64

	// callbacks for managing pages
	// get retrieves a node's raw byte data given its 64-bit pointer/ID[cite: 4].
	get func(uint64) []byte
	// new allocates or persists a new node page, returning its newly assigned 64-bit pointer/ID[cite: 4].
	new func([]byte) uint64
	// del marks a node page for deletion or reclaims its space given its 64-bit pointer/ID[cite: 4].
	del func(uint64)
}

// Insert adds a new key-value pair into the B+ tree, handling empty trees, recursive insertions, and root node splits[cite: 4].
func (tree *BTree) Insert(key []byte, val []byte) {
	// Base Case: The tree is completely empty (root pointer is 0)[cite: 4].
	if tree.root == 0 {
		// Allocate a fresh byte slice for the initial root node using the maximum page size[cite: 4].
		root := BNode(make([]byte, BTREE_PAGE_SIZE))
		// Initialize the root as a leaf node containing 2 keys[cite: 4].
		root.setHeader(BNODE_LEAF, 2)

		// The first key at index 0 is a dummy (nil) key[cite: 4].
		// This acts as a left-most sentinel bound, which is standard in many B+ tree implementations.
		nodeAppendKV(root, 0, 0, nil, nil)

		// Append the actual key and value provided by the user at index 1[cite: 4].
		nodeAppendKV(root, 1, 0, key, val)

		// Persist the newly constructed root node via the 'new' callback and update the tree's root pointer[cite: 4].
		tree.root = tree.new(root)
		return
	}

	// Recursive Case: Fetch the current root node via the 'get' callback and perform the insertion[cite: 4].
	// treeInsert handles the traversal and insertion, returning a new, potentially oversized node[cite: 4].
	node := treeInsert(tree, tree.get(tree.root), key, val)

	// Attempt to split the updated root node[cite: 4].
	// If the new data fits, nsplit will be 1; if it overflows the page limit, it splits into 2 or 3 nodes[cite: 4].
	nsplit, split := nodeSplit3(node)

	// The old root is now obsolete (its data was copied/updated into the new nodes), so delete it[cite: 4].
	tree.del(tree.root)

	// If nsplit is greater than 1, the old root overflowed and fractured into multiple child nodes[cite: 4].
	// A completely new root (an internal node) must be created to link to these fragments[cite: 4].
	if nsplit > 1 {
		// Allocate a new internal node page[cite: 4].
		root := BNode(make([]byte, BTREE_PAGE_SIZE))
		// Set the header to indicate it is an internal node (BNODE_NODE) holding 'nsplit' number of pointers[cite: 4].
		root.setHeader(BNODE_NODE, nsplit)

		// Iterate through each of the split fragment nodes[cite: 4].
		for i, knode := range split[:nsplit] {
			// Persist the fragment via 'new' to get its pointer, and extract its first key for routing[cite: 4].
			ptr, key := tree.new(knode), knode.getKey(0)
			// Append the pointer and routing key into the new internal root node[cite: 4].
			nodeAppendKV(root, uint16(i), ptr, key, nil)
		}

		// Persist the new internal root node and update the tree's root pointer[cite: 4].
		tree.root = tree.new(root)
	} else {
		// If nsplit is 1, the root did not overflow during the insert[cite: 4].
		// Simply persist the updated node as the new root[cite: 4].
		tree.root = tree.new(split[0])
	}
}
