package btree

import (
	"bytes"
	"testing"
)

// TestNodeSplit2 verifies that a standard leaf node can be cleanly split into two separate nodes without losing data.
func TestNodeSplit2(t *testing.T) {
	// Allocate a double-sized node to intentionally allow an overflow state for testing.
	old := BNode(make([]byte, 2*BTREE_PAGE_SIZE))

	// Initialize the node as a leaf expecting 4 keys.
	old.setHeader(BNODE_LEAF, 4)

	// Populate the node with 4 standard key-value pairs.
	nodeAppendKV(old, 0, 0, []byte("key1"), []byte("value1"))
	nodeAppendKV(old, 1, 0, []byte("key2"), []byte("value2"))
	nodeAppendKV(old, 2, 0, []byte("key3"), []byte("value3"))
	nodeAppendKV(old, 3, 0, []byte("key4"), []byte("value4"))

	// Allocate target buffers for the split: 'left' can temporarily hold an oversized buffer, while 'right' is constrained to a standard page.
	left := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	right := BNode(make([]byte, BTREE_PAGE_SIZE))

	// Execute the split operation.
	nodeSplit2(left, right, old)

	// Verify no data was lost during the split by asserting the sum of keys equals the original key count.
	if left.nkeys()+right.nkeys() != old.nkeys() {
		t.Fatalf(
			"split lost keys: old=%d left=%d right=%d",
			old.nkeys(),
			left.nkeys(),
			right.nkeys(),
		)
	}

	// Verify the right node respects the 4096-byte page size limit.
	if right.nbytes() > BTREE_PAGE_SIZE {
		t.Fatalf("right node is too large: %d bytes", right.nbytes())
	}

	// Ensure neither resulting node is empty.
	if left.nkeys() == 0 {
		t.Fatal("left node is empty")
	}

	if right.nkeys() == 0 {
		t.Fatal("right node is empty")
	}

	// Log the final byte sizes for debugging.
	t.Logf(
		"old=%d bytes, left=%d bytes, right=%d bytes",
		old.nbytes(),
		left.nbytes(),
		right.nbytes(),
	)
}

// TestNodeSplit2WithLargeValues tests the split logic when node payloads significantly exceed the page size[cite: 5].
func TestNodeSplit2WithLargeValues(t *testing.T) {
	old := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	old.setHeader(BNODE_LEAF, 3)

	// Create three massive 1500-byte values (totaling 4500 bytes, which exceeds the 4096-byte page size)[cite: 5].
	value1 := bytes.Repeat([]byte("A"), 1500)
	value2 := bytes.Repeat([]byte("B"), 1500)
	value3 := bytes.Repeat([]byte("C"), 1500)

	nodeAppendKV(old, 0, 0, []byte("key1"), value1)
	nodeAppendKV(old, 1, 0, []byte("key2"), value2)
	nodeAppendKV(old, 2, 0, []byte("key3"), value3)

	t.Logf("old node size: %d bytes", old.nbytes())

	// Confirm the test setup successfully created an oversized node[cite: 5].
	if old.nbytes() <= BTREE_PAGE_SIZE {
		t.Fatal("test node is not oversized")
	}

	left := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	right := BNode(make([]byte, BTREE_PAGE_SIZE))

	nodeSplit2(left, right, old)

	t.Logf("left node size: %d bytes", left.nbytes())
	t.Logf("right node size: %d bytes", right.nbytes())

	// Both nodes must contain at least one key[cite: 5].
	if left.nkeys() == 0 {
		t.Fatal("left node is empty")
	}

	if right.nkeys() == 0 {
		t.Fatal("right node is empty")
	}

	// All keys must be preserved[cite: 5].
	if left.nkeys()+right.nkeys() != old.nkeys() {
		t.Fatalf(
			"key count mismatch: old=%d left=%d right=%d",
			old.nkeys(),
			left.nkeys(),
			right.nkeys(),
		)
	}

	// nodeSplit2 guarantees that the RIGHT node fits within the 4096-byte boundary[cite: 5].
	if right.nbytes() > BTREE_PAGE_SIZE {
		t.Fatalf("right node exceeds page size: %d", right.nbytes())
	}

	// Sequentially check that the keys in the left node perfectly match the first half of the old node[cite: 5].
	for i := uint16(0); i < left.nkeys(); i++ {
		if !bytes.Equal(left.getKey(i), old.getKey(i)) {
			t.Fatalf("left key %d does not match old node", i)
		}
	}

	// Sequentially check that the keys in the right node perfectly match the second half of the old node[cite: 5].
	for i := uint16(0); i < right.nkeys(); i++ {
		oldIdx := left.nkeys() + i

		if !bytes.Equal(right.getKey(i), old.getKey(oldIdx)) {
			t.Fatalf("right key %d does not match old key %d", i, oldIdx)
		}
	}
}

// TestNodeSplit3 verifies edge cases where a node is so overloaded that a 2-way split is insufficient[cite: 5].
func TestNodeSplit3(t *testing.T) {
	old := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	old.setHeader(BNODE_LEAF, 3)

	// Create an extreme payload: 1500 + 3000 + 1500 = 6000 bytes, forcing a 3-way split[cite: 5].
	small := bytes.Repeat([]byte("S"), 1500)
	large := bytes.Repeat([]byte("L"), 3000)
	small2 := bytes.Repeat([]byte("T"), 1500)

	nodeAppendKV(old, 0, 0, []byte("key1"), small)
	nodeAppendKV(old, 1, 0, []byte("key2"), large)
	nodeAppendKV(old, 2, 0, []byte("key3"), small2)

	if old.nbytes() <= BTREE_PAGE_SIZE {
		t.Fatal("test node is not oversized")
	}

	// Execute the split, which should detect the left node is still oversized and split it again[cite: 5].
	nsplit, nodes := nodeSplit3(old)

	// Verify that exactly 3 nodes were generated[cite: 5].
	if nsplit != 3 {
		t.Fatalf("expected 3 nodes, got %d", nsplit)
	}

	// Iterate over the resulting array to ensure no single node exceeds the page size limit[cite: 5].
	for i := uint16(0); i < nsplit; i++ {
		if nodes[i].nbytes() > BTREE_PAGE_SIZE {
			t.Fatalf(
				"node %d exceeds page size: %d bytes",
				i,
				nodes[i].nbytes(),
			)
		}
	}

	// Verify that the sum of the keys in all 3 nodes equals the original total[cite: 5].
	if nodes[0].nkeys()+nodes[1].nkeys()+nodes[2].nkeys() != old.nkeys() {
		t.Fatal("keys were lost during 3-way split")
	}
}

// TestLeafUpdate verifies the immutability (copy-on-write) mechanics of updating an existing key's value in a leaf node[cite: 5].
func TestLeafUpdate(t *testing.T) {
	// Create the old leaf node[cite: 5].
	old := BNode(make([]byte, BTREE_PAGE_SIZE))
	old.setHeader(BNODE_LEAF, 3)

	// Populate the old node with baseline data[cite: 5].
	nodeAppendKV(old, 0, 0, []byte("apple"), []byte("red"))
	nodeAppendKV(old, 1, 0, []byte("banana"), []byte("yellow"))
	nodeAppendKV(old, 2, 0, []byte("cherry"), []byte("red"))

	// Create a newly allocated byte slice to act as the destination node[cite: 5].
	new := BNode(make([]byte, 2*BTREE_PAGE_SIZE))

	// Update "banana" at index 1 to the value "green"[cite: 5].
	leafUpdate(new, old, 1, []byte("banana"), []byte("green"))

	// The total number of keys should remain unchanged (3 keys)[cite: 5].
	if new.nkeys() != old.nkeys() {
		t.Fatalf("key count changed: old=%d new=%d", old.nkeys(), new.nkeys())
	}

	// Check key/value 0 to ensure preceding data was copied flawlessly[cite: 5].
	if string(new.getKey(0)) != "apple" {
		t.Fatalf("unexpected key 0: %s", new.getKey(0))
	}
	if string(new.getVal(0)) != "red" {
		t.Fatalf("unexpected value 0: %s", new.getVal(0))
	}

	// Check the updated key/value to ensure the target modification succeeded[cite: 5].
	if string(new.getKey(1)) != "banana" {
		t.Fatalf("unexpected key 1: %s", new.getKey(1))
	}
	if string(new.getVal(1)) != "green" {
		t.Fatalf("unexpected value 1: %s", new.getVal(1))
	}

	// Check key/value 2 to ensure succeeding data was copied flawlessly[cite: 5].
	if string(new.getKey(2)) != "cherry" {
		t.Fatalf("unexpected key 2: %s", new.getKey(2))
	}
	if string(new.getVal(2)) != "red" {
		t.Fatalf("unexpected value 2: %s", new.getVal(2))
	}

	// Verify that the old node was NOT modified in place[cite: 5].
	// This proves the system successfully uses a copy-on-write design, leaving the original memory buffer pristine[cite: 5].
	if string(old.getVal(1)) != "yellow" {
		t.Fatalf("old node was modified: expected yellow, got %s", old.getVal(1))
	}
}
