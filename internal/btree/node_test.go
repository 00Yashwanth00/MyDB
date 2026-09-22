package btree

import (
	"bytes"
	"testing"
)

func TestNodeSplit2(t *testing.T) {
	old := BNode(make([]byte, 2*BTREE_PAGE_SIZE))

	old.setHeader(BNODE_LEAF, 4)

	nodeAppendKV(
		old,
		0,
		0,
		[]byte("key1"),
		[]byte("value1"),
	)

	nodeAppendKV(
		old,
		1,
		0,
		[]byte("key2"),
		[]byte("value2"),
	)

	nodeAppendKV(
		old,
		2,
		0,
		[]byte("key3"),
		[]byte("value3"),
	)

	nodeAppendKV(
		old,
		3,
		0,
		[]byte("key4"),
		[]byte("value4"),
	)

	left := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	right := BNode(make([]byte, BTREE_PAGE_SIZE))

	nodeSplit2(left, right, old)

	if left.nkeys()+right.nkeys() != old.nkeys() {
		t.Fatalf(
			"split lost keys: old=%d left=%d right=%d",
			old.nkeys(),
			left.nkeys(),
			right.nkeys(),
		)
	}

	if right.nbytes() > BTREE_PAGE_SIZE {
		t.Fatalf(
			"right node is too large: %d bytes",
			right.nbytes(),
		)
	}

	if left.nkeys() == 0 {
		t.Fatal("left node is empty")
	}

	if right.nkeys() == 0 {
		t.Fatal("right node is empty")
	}

	t.Logf(
		"old=%d bytes, left=%d bytes, right=%d bytes",
		old.nbytes(),
		left.nbytes(),
		right.nbytes(),
	)
}

func TestNodeSplit2WithLargeValues(t *testing.T) {
	old := BNode(make([]byte, 2*BTREE_PAGE_SIZE))

	old.setHeader(BNODE_LEAF, 3)

	value1 := bytes.Repeat([]byte("A"), 1500)
	value2 := bytes.Repeat([]byte("B"), 1500)
	value3 := bytes.Repeat([]byte("C"), 1500)

	nodeAppendKV(
		old,
		0,
		0,
		[]byte("key1"),
		value1,
	)

	nodeAppendKV(
		old,
		1,
		0,
		[]byte("key2"),
		value2,
	)

	nodeAppendKV(
		old,
		2,
		0,
		[]byte("key3"),
		value3,
	)

	t.Logf("old node size: %d bytes", old.nbytes())

	if old.nbytes() <= BTREE_PAGE_SIZE {
		t.Fatal("test node is not oversized")
	}

	left := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
	right := BNode(make([]byte, BTREE_PAGE_SIZE))

	nodeSplit2(left, right, old)

	t.Logf("left node size: %d bytes", left.nbytes())
	t.Logf("right node size: %d bytes", right.nbytes())

	// Both nodes must contain at least one key.
	if left.nkeys() == 0 {
		t.Fatal("left node is empty")
	}

	if right.nkeys() == 0 {
		t.Fatal("right node is empty")
	}

	// All keys must be preserved.
	if left.nkeys()+right.nkeys() != old.nkeys() {
		t.Fatalf(
			"key count mismatch: old=%d left=%d right=%d",
			old.nkeys(),
			left.nkeys(),
			right.nkeys(),
		)
	}

	// nodeSplit2 guarantees that the RIGHT node fits.
	if right.nbytes() > BTREE_PAGE_SIZE {
		t.Fatalf(
			"right node exceeds page size: %d",
			right.nbytes(),
		)
	}

	// Check that the keys remain in order.
	for i := uint16(0); i < left.nkeys(); i++ {
		if !bytes.Equal(left.getKey(i), old.getKey(i)) {
			t.Fatalf("left key %d does not match old node", i)
		}
	}

	for i := uint16(0); i < right.nkeys(); i++ {
		oldIdx := left.nkeys() + i

		if !bytes.Equal(right.getKey(i), old.getKey(oldIdx)) {
			t.Fatalf(
				"right key %d does not match old key %d",
				i,
				oldIdx,
			)
		}
	}
}

func TestNodeSplit3(t *testing.T) {
	old := BNode(make([]byte, 2*BTREE_PAGE_SIZE))

	old.setHeader(BNODE_LEAF, 3)

	small := bytes.Repeat([]byte("S"), 1500)
	large := bytes.Repeat([]byte("L"), 3000)
	small2 := bytes.Repeat([]byte("T"), 1500)

	nodeAppendKV(
		old,
		0,
		0,
		[]byte("key1"),
		small,
	)

	nodeAppendKV(
		old,
		1,
		0,
		[]byte("key2"),
		large,
	)

	nodeAppendKV(
		old,
		2,
		0,
		[]byte("key3"),
		small2,
	)

	if old.nbytes() <= BTREE_PAGE_SIZE {
		t.Fatal("test node is not oversized")
	}

	nsplit, nodes := nodeSplit3(old)

	if nsplit != 3 {
		t.Fatalf(
			"expected 3 nodes, got %d",
			nsplit,
		)
	}

	for i := uint16(0); i < nsplit; i++ {
		if nodes[i].nbytes() > BTREE_PAGE_SIZE {
			t.Fatalf(
				"node %d exceeds page size: %d bytes",
				i,
				nodes[i].nbytes(),
			)
		}
	}

	if nodes[0].nkeys()+nodes[1].nkeys()+nodes[2].nkeys() != old.nkeys() {
		t.Fatal("keys were lost during 3-way split")
	}
}

func TestLeafUpdate(t *testing.T) {
	// Create the old leaf node.
	old := BNode(make([]byte, BTREE_PAGE_SIZE))

	old.setHeader(BNODE_LEAF, 3)

	nodeAppendKV(
		old,
		0,
		0,
		[]byte("apple"),
		[]byte("red"),
	)

	nodeAppendKV(
		old,
		1,
		0,
		[]byte("banana"),
		[]byte("yellow"),
	)

	nodeAppendKV(
		old,
		2,
		0,
		[]byte("cherry"),
		[]byte("red"),
	)

	// Create the new node.
	new := BNode(make([]byte, 2*BTREE_PAGE_SIZE))

	// Update "banana".
	leafUpdate(
		new,
		old,
		1,
		[]byte("banana"),
		[]byte("green"),
	)

	// The number of keys should remain unchanged.
	if new.nkeys() != old.nkeys() {
		t.Fatalf(
			"key count changed: old=%d new=%d",
			old.nkeys(),
			new.nkeys(),
		)
	}

	// Check key/value 0.
	if string(new.getKey(0)) != "apple" {
		t.Fatalf("unexpected key 0: %s", new.getKey(0))
	}

	if string(new.getVal(0)) != "red" {
		t.Fatalf("unexpected value 0: %s", new.getVal(0))
	}

	// Check updated key/value.
	if string(new.getKey(1)) != "banana" {
		t.Fatalf("unexpected key 1: %s", new.getKey(1))
	}

	if string(new.getVal(1)) != "green" {
		t.Fatalf("unexpected value 1: %s", new.getVal(1))
	}

	// Check key/value 2.
	if string(new.getKey(2)) != "cherry" {
		t.Fatalf("unexpected key 2: %s", new.getKey(2))
	}

	if string(new.getVal(2)) != "red" {
		t.Fatalf("unexpected value 2: %s", new.getVal(2))
	}

	// Verify that the old node was not modified.
	if string(old.getVal(1)) != "yellow" {
		t.Fatalf(
			"old node was modified: expected yellow, got %s",
			old.getVal(1),
		)
	}
}
