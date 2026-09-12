package btree

type BTree struct {
	// pointer to the root node
	root uint64

	// callbacks for managing pages
	get func(uint64) []byte
	new func([]byte) uint64
	del func(uint64)
}
