# MyDB

A database system implemented from scratch in Go, following *Build Your Own Database From Scratch in Go (2nd Edition)*.

## 1. Project Overview

The goal of this project is to understand and implement the fundamental components of a database, including:

- Persistent storage
- B+Tree-based indexing
- Copy-on-write updates
- Page-based storage
- Tree insertion and deletion
- Transactions
- Query processing

The project is being developed incrementally, with each component implemented and tested before moving to the next.

## 2. File Storage

### Overview

This section introduces the basic mechanism for storing database data on disk and safely updating persisted data. Two approaches are implemented:

1. Direct file writing
2. Atomic file replacement

### Functions

#### `SaveData1`

```go
func SaveData1(path string, data []byte) error
```

**Purpose:** Writes data directly to a file.

| Parameter | Description |
|---|---|
| `path` | Destination file path |
| `data` | Data to write |

**Returns:** `error` — error encountered during file creation, writing, or synchronization.

#### `SaveData2`

```go
func SaveData2(path string, data []byte) error
```

**Purpose:** Safely replaces an existing file by writing to a temporary file first and then renaming it over the original file.

| Parameter | Description |
|---|---|
| `path` | Destination file path |
| `data` | New data |

**Returns:** `error`

### Design Techniques

- **Atomic file replacement** — temporary file followed by `os.Rename()`
- **File synchronization** — `Sync()` is used before replacing the original file
- **Failure cleanup** — temporary files are removed when an error occurs
- **Crash safety** — the original file remains untouched until the new version is ready

## 3. B+Tree Node Representation

### Overview

This section introduces the internal representation of B+Tree nodes. The database uses fixed-size 4096-byte pages to represent nodes. A node is stored as a byte array rather than as a normal Go structure:

```go
type BNode []byte
```

The node layout is:

| Header | Pointers | Offsets | Key-Values |
|---|---|---|---|

The header stores the node type and number of keys.

### Functions

#### `btype`

```go
func (node BNode) btype() uint16
```

Returns the type of the node.
**Input:** `BNode` · **Output:** `uint16`

#### `nkeys`

```go
func (node BNode) nkeys() uint16
```

Returns the number of keys stored in the node.
**Input:** `BNode` · **Output:** `uint16`

#### `setHeader`

```go
func (node BNode) setHeader(btype uint16, nkeys uint16)
```

Sets the node type and key count.
**Input:** node type and key count · **Output:** none

#### `getPtr` / `setPtr`

```go
func (node BNode) getPtr(idx uint16) uint64
func (node BNode) setPtr(idx uint16, val uint64)
```

Read and write child/page pointers.
**Input:** node/index (and pointer value for `setPtr`) · **Output:** pointer for `getPtr`

#### `getOffset` / `setOffset`

```go
func (node BNode) getOffset(idx uint16) uint16
func (node BNode) setOffset(idx uint16, offset uint16)
```

Read and write offsets used to locate variable-sized key-value entries.

#### `kvPos`

```go
func (node BNode) kvPos(idx uint16) uint16
```

Calculates the position of a key-value entry inside the node.
**Output:** byte position.

#### `getKey` / `getVal`

```go
func (node BNode) getKey(idx uint16) []byte
func (node BNode) getVal(idx uint16) []byte
```

Retrieve a key or value from a node.
**Output:** `[]byte`

#### `nbytes`

```go
func (node BNode) nbytes() uint16
```

Returns the number of bytes currently occupied by the node.

### Design Techniques

- **Fixed-size pages** — 4096-byte B+Tree pages
- **Binary serialization** — node metadata is stored directly in a byte array
- **Offset-based storage** — allows variable-sized keys and values
- **Little-endian encoding** — used for integer serialization
- **Page-oriented design** — prepares the tree for disk-based storage

## 4. B+Tree Node Operations

### Overview

This section implements the basic operations required to modify B+Tree nodes. These operations construct new nodes rather than modifying existing nodes directly.

### Functions

#### `nodeLookupLE`

```go
func nodeLookupLE(node BNode, key []byte) uint16
```

Finds the appropriate key position for a given key.

| Parameter | Description |
|---|---|
| `node` | The node to search |
| `key` | Search key |

**Returns:** key index

#### `nodeAppendKV`

```go
func nodeAppendKV(
    new BNode,
    idx uint16,
    ptr uint64,
    key []byte,
    val []byte,
)
```

Appends a key-value entry to a node.

**Input:** destination node, index, pointer, key, value · **Output:** none

#### `nodeAppendRange`

```go
func nodeAppendRange(
    new BNode,
    old BNode,
    dstNew uint16,
    srcOld uint16,
    n uint16,
)
```

Copies a range of entries from one node to another.

#### `leafInsert`

```go
func leafInsert(
    new BNode,
    old BNode,
    idx uint16,
    key []byte,
    val []byte,
)
```

Creates a new leaf node with a new key-value pair inserted at the specified position.

#### `nodeReplaceKidN`

```go
func nodeReplaceKidN(
    tree *BTree,
    new BNode,
    old BNode,
    idx uint16,
    kids ...BNode,
)
```

Replaces an internal node's child with one or more new child nodes. This is required when a child node is split.

#### `nodeSplit2`

```go
func nodeSplit2(left BNode, right BNode, old BNode)
```

Splits an oversized node into two nodes.

#### `nodeSplit3`

```go
func nodeSplit3(old BNode) (uint16, [3]BNode)
```

Splits a node into one, two, or three nodes depending on its size.

#### `leafUpdate`

```go
func leafUpdate(
    new BNode,
    old BNode,
    idx uint16,
    key []byte,
    val []byte,
)
```

Creates a new leaf node with an existing key's value replaced. Unlike `leafInsert`, it does not increase the number of keys.

### Design Techniques

The main design technique used in this section is **Copy-on-Write (COW)**.

Instead of modifying an existing node in place:

```
Old Node
   |
   X  ← modify directly
```

...a new node is created, leaving the original unchanged:

```
Old Node ──────────── unchanged

New Node ──────────── modified version
```

This is important for maintaining consistent database state and will later allow tree updates to be made safely.

Other techniques used:

- Immutable-style node updates
- Page-based storage
- Binary serialization
- Node splitting
- Variable-sized key-value storage

## 5. Testing

The implemented B+Tree node operations are tested using Go's standard `testing` package. The tests verify:

- Node headers
- Key/value storage
- Node lookup
- Leaf insertion
- Node splitting
- Leaf updating
- Copy-on-write behavior

All current tests are passing.

### Current Status

| Component | Status |
|---|---|
| File persistence | ✅ |
| Atomic file updates | ✅ |
| B+Tree node representation | ✅ |
| Node operations | ✅ |
| Leaf insertion | ✅ |
| Node splitting | ✅ |
| Leaf update | ✅ |
| Tests | ✅ |

## 6. Current Design Principles

| Technique | Purpose |
|---|---|
| Page-based storage | Organize database data into fixed-size pages |
| B+Tree | Efficiently organize and search database keys |
| Binary serialization | Store nodes compactly as bytes |
| Copy-on-Write | Modify data without changing existing nodes |
| Atomic replacement | Protect persisted data from partial updates |
| Node splitting | Keep B+Tree nodes within page-size limits |
| Offset-based layout | Support variable-sized keys and values |
| Unit testing | Verify each database component independently |
