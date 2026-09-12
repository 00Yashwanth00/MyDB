package btree

const (
	HEADER = 4

	BTREE_PAGE_SIZE    = 4096
	BTREE_MAX_KEY_SIZE = 1000
	BTREE_MAX_VAL_SIZE = 3000
)

const (
	BNODE_NODE = 1 // internal node
	BNODE_LEAF = 2 // leaf node
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
