package btree

import (
	"bytes"
	"fmt"
	"testing"
	"unsafe"
)

// C serves as the test harness for B+ tree operations.
// It maintains a BTree instance alongside a simulated page storage map
// and an in-memory ground-truth reference map for state verification.
type C struct {
	tree  BTree             // The B+ tree instance under test.
	ref   map[string]string // Reference map storing expected key-value pairs (ground truth).
	pages map[uint64]BNode  // Simulated disk/page store mapping memory addresses to BNode byte buffers.
}

// newC initializes a new test harness with in-memory page storage callbacks.
// The callbacks mimic disk/page allocation, retrieval, and deallocation.
func newC() *C {
	pages := map[uint64]BNode{}

	return &C{
		tree: BTree{
			// Get fetches a page from the simulated storage by its 64-bit memory address pointer.
			get: func(ptr uint64) []byte {
				node, ok := pages[ptr]
				if !ok {
					panic("invalid page pointer: attempt to access unallocated page")
				}
				return node
			},

			// New allocates a new page buffer in the simulated storage.
			// It enforces the page size constraint and uses the raw memory address as a unique page ID.
			new: func(node []byte) uint64 {
				// Assert that the page buffer fits within the mandatory page size.
				if BNode(node).nbytes() > BTREE_PAGE_SIZE {
					panic("node exceeds maximum allowed page size")
				}

				// Derive a pseudo page pointer from the slice's starting memory address.
				ptr := uint64(uintptr(unsafe.Pointer(&node[0])))

				// Ensure no pointer collision or double-allocation occurs.
				if pages[ptr] != nil {
					panic("page already exists: duplicate memory address pointer")
				}

				// Store the node byte slice cast to BNode.
				pages[ptr] = BNode(node)

				return ptr
			},

			// Del removes a page from storage when it is freed (e.g., during splits or merges).
			del: func(ptr uint64) {
				if pages[ptr] == nil {
					panic("deleting non-existent page: potential double free or corrupt pointer")
				}

				delete(pages, ptr)
			},
		},

		ref:   map[string]string{},
		pages: pages,
	}
}

// add inserts a key-value pair into both the B+ tree and the reference map.
func (c *C) add(key string, val string) {
	c.tree.Insert([]byte(key), []byte(val))
	c.ref[key] = val // Update reference map to maintain ground truth.
}

// verifyNode checks structural invariants for a single B+ tree node:
// 1. Node size must not exceed the standard BTREE_PAGE_SIZE limit.
// 2. Keys within the node must be strictly sorted in ascending lexicographical order.
func verifyNode(t *testing.T, node BNode) {
	if node.nbytes() > BTREE_PAGE_SIZE {
		t.Fatalf("node exceeds page size: %d bytes (max: %d)", node.nbytes(), BTREE_PAGE_SIZE)
	}

	// Verify key order within the node buffer.
	for i := uint16(1); i < node.nkeys(); i++ {
		if bytes.Compare(node.getKey(i-1), node.getKey(i)) > 0 {
			t.Fatalf(
				"keys are not sorted: %q > %q at index %d",
				node.getKey(i-1),
				node.getKey(i),
				i,
			)
		}
	}
}

// verifyTree recursively traverses the B+ tree structure starting from ptr,
// validating node invariants and child pointer integrity across internal nodes.
func verifyTree(t *testing.T, c *C, ptr uint64) {
	if ptr == 0 {
		return // Null root / empty sub-tree.
	}

	node := BNode(c.tree.get(ptr))

	// Check page size and sorted keys for the current node.
	verifyNode(t, node)

	// If internal node, recursively validate all child nodes.
	if node.btype() == BNODE_NODE {
		for i := uint16(0); i < node.nkeys(); i++ {
			childPtr := node.getPtr(i)

			if childPtr == 0 {
				t.Fatalf("internal node at ptr %d contains null child pointer at index %d", ptr, i)
			}

			// Recurse into child page.
			verifyTree(t, c, childPtr)
		}
	}
}

// verifyData traverses the tree down to the leaf nodes and verifies that
// every stored key-value pair accurately matches the reference ground truth.
func verifyData(t *testing.T, c *C, node BNode) {
	// Base case: Leaf node containing user key-value data.
	if node.btype() == BNODE_LEAF {
		for i := uint16(0); i < node.nkeys(); i++ {
			key := node.getKey(i)
			val := node.getVal(i)

			// Skip initial empty dummy/sentinel key used by leaf initialization.
			if len(key) == 0 && len(val) == 0 {
				continue
			}

			keyStr := string(key)
			valStr := string(val)

			// Assert key exists in reference map.
			expected, ok := c.ref[keyStr]
			if !ok {
				t.Fatalf("unexpected key found in B+Tree: %q", keyStr)
			}

			// Assert value matches expected ground truth.
			if expected != valStr {
				t.Fatalf(
					"value mismatch for key %q: expected %q, got %q",
					keyStr,
					expected,
					valStr,
				)
			}
		}
		return
	}

	// Recursive case: Internal node, route to all child nodes.
	if node.btype() == BNODE_NODE {
		for i := uint16(0); i < node.nkeys(); i++ {
			child := BNode(c.tree.get(node.getPtr(i)))
			verifyData(t, c, child)
		}
		return
	}

	t.Fatalf("invalid node type encountered: %d", node.btype())
}

// verify runs full validation against the tree:
// 1. Verifies root page structure.
// 2. Traverses tree pointer integrity recursively.
// 3. Compares all leaf data against the ground truth reference map.
func (c *C) verify(t *testing.T) {
	if c.tree.root == 0 {
		return // Empty tree is valid.
	}

	root := BNode(c.tree.get(c.tree.root))

	verifyNode(t, root)
	verifyTree(t, c, c.tree.root)
	verifyData(t, c, root)
}

// TestInsert tests simple sequential key-value insertions.
func TestInsert(t *testing.T) {
	c := newC()

	c.add("a", "1")
	c.verify(t)

	c.add("b", "2")
	c.verify(t)

	c.add("c", "3")
	c.verify(t)
}

// TestInsertUpdate tests mutating the value of an existing key in place.
func TestInsertUpdate(t *testing.T) {
	c := newC()

	c.add("a", "1")
	c.add("b", "2")
	c.add("c", "3")

	// Overwrite existing key "b" with a new value.
	c.add("b", "updated")

	c.verify(t)
}

// TestInsertSplit tests massive key insertions to trigger node splitting,
// tree height increases, and internal node propagation.
func TestInsertSplit(t *testing.T) {
	c := newC()

	// Insert 1000 items to force multiple page splits.
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("%04d", i)
		val := fmt.Sprintf("value-%04d", i)

		c.add(key, val)
	}

	// Validate tree structure and data integrity across all split nodes.
	c.verify(t)
}

// TestDelete tests basic key deletion and ensures tree integrity afterwards.
func TestDelete(t *testing.T) {
	c := newC()

	c.add("a", "1")
	c.add("b", "2")
	c.add("c", "3")

	// Delete key "b" from tree and ground truth reference.
	c.tree.Delete([]byte("b"))
	delete(c.ref, "b")

	c.verify(t)
}

// TestDeleteNotFound verifies that deleting a non-existent key gracefully returns false.
func TestDeleteNotFound(t *testing.T) {
	c := newC()

	c.add("a", "1")
	c.add("b", "2")
	c.add("c", "3")

	// Attempt deleting a key that was never inserted.
	deleted := c.tree.Delete([]byte("x"))

	if deleted {
		t.Fatalf("expected Delete to return false for non-existent key")
	}

	c.verify(t)
}

// TestDeleteAll tests deleting all entries one-by-one, verifying structural
// and data integrity after every single deletion down to an empty tree.
func TestDeleteAll(t *testing.T) {
	c := newC()

	c.add("a", "1")
	c.add("b", "2")
	c.add("c", "3")
	c.add("d", "4")
	c.add("e", "5")

	keys := []string{"a", "b", "c", "d", "e"}

	for _, key := range keys {
		deleted := c.tree.Delete([]byte(key))

		if !deleted {
			t.Fatalf("failed to delete key %q", key)
		}

		delete(c.ref, key)
		c.verify(t) // Verify tree invariants after each deletion step.
	}
}
