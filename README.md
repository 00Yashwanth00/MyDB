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

1. Direct file writing (`SaveData1`)
2. Atomic file replacement (`SaveData2`)

### Functions

#### `SaveData1`

```go
func SaveData1(path string, data []byte) error
```

**Purpose:** Writes data directly to a file, overwriting whatever is already there. This is the simplest possible persistence strategy, but it is unsafe: if the process crashes mid-write, the file can be left half-written and corrupted.

**Parameters:**
| Parameter | Type | Purpose |
|---|---|---|
| `path` | `string` | The destination file path. Identifies which file on disk should receive the data. |
| `data` | `[]byte` | The raw bytes to persist. This is the payload the caller wants saved. |

**What it computes:**
1. Opens (or creates) the file at `path` using `os.O_WRONLY\|os.O_CREATE\|os.O_TRUNC` with permissions `0664`. `O_TRUNC` discards any existing content immediately, which is the source of the crash-safety problem — the old data is gone before the new data is confirmed written.
2. Writes `data` to the file with `fp.Write(data)`.
3. Calls `fp.Sync()` to force the OS to flush its in-memory buffers to physical disk, rather than trusting the OS to do this on its own schedule.

**Returns:** `error` — non-nil if the file could not be opened, if the write failed, or if the sync failed. The caller needs this to know whether the data is actually durable on disk; a nil error is the only guarantee that `Sync()` succeeded and the bytes are safely persisted.

#### `SaveData2`

```go
func SaveData2(path string, data []byte) error
```

**Purpose:** Safely replaces an existing file by writing the new content to a temporary file first, and only swapping it into place once the write is confirmed complete. This avoids the corruption risk of `SaveData1` because the original file is never touched until the replacement is fully ready.

**Parameters:**
| Parameter | Type | Purpose |
|---|---|---|
| `path` | `string` | The final destination path that should end up containing the new data. |
| `data` | `[]byte` | The new content to persist. |

**What it computes:**
1. Builds a unique temporary file name: `path + ".tmp." + os.Getpid()`. Including the process ID avoids collisions if multiple instances run concurrently.
2. Opens the temporary file with `O_WRONLY\|O_CREATE\|O_EXCL`. `O_EXCL` guarantees this call *creates* the file and fails if it already exists, preventing two writers from clobbering the same temp file.
3. Writes `data` into the temporary file and calls `Sync()` to flush it to disk — this ensures the temp file is completely and durably written *before* it replaces anything.
4. Calls `os.Rename(tmp, path)`. On POSIX systems, rename is atomic: any reader either sees the old file in full or the new file in full, never a partial mix.
5. A deferred cleanup closes the temp file descriptor and, if any step failed, removes the temp file so it doesn't linger as orphaned garbage on disk.

**Returns:** `error` — non-nil if creating, writing, syncing, or renaming the temp file failed. Returning the error (rather than swallowing it) is what triggers the deferred cleanup to delete the temp file, and lets the caller know the atomic swap did not happen and the original file is still intact.

### Design Techniques

- **Atomic file replacement** — temporary file followed by `os.Rename()`
- **File synchronization** — `Sync()` is used before replacing the original file
- **Failure cleanup** — temporary files are removed when an error occurs
- **Crash safety** — the original file remains untouched until the new version is ready

## 3. B+Tree Node Representation

### Overview

This section introduces the internal representation of B+Tree nodes. The database uses fixed-size 4096-byte pages to represent nodes. A node is stored as a raw byte array rather than as a normal Go structure, so every field must be read and written at a precisely calculated byte offset:

```go
type BNode []byte
```

The node layout is:

| Header (4 bytes) | Pointers (8 bytes × nkeys) | Offsets (2 bytes × nkeys) | Key-Values (variable) |
|---|---|---|---|

Two additional layout constants govern this representation:

```go
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
```

An `init()` function panics at startup if a single maximum-sized key-value pair (`HEADER + 8 + 2 + 4 + BTREE_MAX_KEY_SIZE + BTREE_MAX_VAL_SIZE`) would not fit inside one 4096-byte page — this is a safety check that the page-size and max-KV-size constants are mutually consistent.

### Functions

#### `btype`

```go
func (node BNode) btype() uint16
```

**Purpose:** Identifies whether this node is an internal node (`BNODE_NODE`) or a leaf node (`BNODE_LEAF`), since the two require different traversal and update logic.

**Parameters:** none (method receiver is `node BNode`, the raw page bytes to read from).

**What it computes:** Reads the first 2 bytes of the node (`node[0:2]`) as a little-endian `uint16`, since the node type is the first field stored in the header.

**Returns:** `uint16` — the node type constant (`BNODE_NODE` or `BNODE_LEAF`), used by callers such as `treeInsert`/`treeDelete` to decide whether to recurse into a child or operate directly on key-value pairs.

#### `nkeys`

```go
func (node BNode) nkeys() uint16
```

**Purpose:** Reports how many keys (and, for internal nodes, child pointers) the node currently holds. Almost every other node function needs this value to know where the pointer list, offset list, and key-value section end.

**Parameters:** none (reads from the receiver `node`).

**What it computes:** Reads bytes `2:4` of the node as a little-endian `uint16` — the second field of the 4-byte header.

**Returns:** `uint16` — the key count, used to bound loops, validate indices (`idx >= node.nkeys()` checks), and compute the size of the pointer/offset sections.

#### `setHeader`

```go
func (node BNode) setHeader(btype uint16, nkeys uint16)
```

**Purpose:** Initializes a freshly allocated node by writing both header fields at once. Every node-construction function (`leafInsert`, `nodeSplit2`, `nodeMerge`, etc.) calls this first, before appending any key-value data, so that subsequent calls to `nkeys()`/`btype()` on the new node return correct values.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `btype` | The node type to record (`BNODE_NODE` or `BNODE_LEAF`), determining how the node will be interpreted. |
| `nkeys` | The number of keys the node will contain, so downstream offset/pointer calculations are correct even before all entries are appended. |

**What it computes:** Writes `btype` into bytes `0:2` and `nkeys` into bytes `2:4`, both little-endian.

**Returns:** nothing — it mutates the node's underlying byte slice in place, which is safe here because `setHeader` is only ever called on a newly allocated node, not on one that is shared/visible elsewhere (preserving the copy-on-write guarantee at the node level).

#### `getPtr` / `setPtr`

```go
func (node BNode) getPtr(idx uint16) uint64
func (node BNode) setPtr(idx uint16, val uint64)
```

**Purpose:** Read and write the 64-bit child-page pointer stored at a given slot. Pointers live immediately after the header, one 8-byte slot per key, and are used by internal nodes to reference their children (for leaf nodes they are typically unused/zero).

**Parameters:**
| Parameter | Function | Purpose |
|---|---|---|
| `idx` | both | Which pointer slot to access. Must be `< nkeys()`; otherwise the function panics with `"invalid pointer index"`, guarding against reading/writing outside the allocated pointer region. |
| `val` | `setPtr` only | The 64-bit page ID/address to store at that slot. |

**What it computes:** Both compute the byte position `pos := HEADER + 8*idx` (the header size plus 8 bytes per preceding pointer), then either read (`getPtr`) or write (`setPtr`) a little-endian `uint64` at that position.

**Returns:** `getPtr` returns the `uint64` pointer value (the child page's ID, later passed to `tree.get()` to fetch that page). `setPtr` returns nothing; it mutates the node in place.

#### `offsetPos`

```go
func offsetPos(node BNode, idx uint16) uint16
```

**Purpose:** A helper that computes where a given offset entry lives in the node, shared by both `getOffset` and `setOffset` so the layout math isn't duplicated.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `node` | The node whose layout is being addressed (needed to know `nkeys()`, since the offset list comes after all the pointers). |
| `idx` | The 1-based offset slot being located. Must satisfy `1 <= idx <= nkeys()`; otherwise it panics with `"invalid offset index"`. Index 0 is intentionally excluded because offset 0 is always implicitly zero and is never stored. |

**What it computes:** `HEADER + 8*node.nkeys() + 2*(idx-1)` — skips past the header and the full pointer list, then advances 2 bytes for each offset slot before `idx`.

**Returns:** `uint16` — the absolute byte position of offset slot `idx` within the node, consumed internally by `getOffset`/`setOffset`.

#### `getOffset` / `setOffset`

```go
func (node BNode) getOffset(idx uint16) uint16
func (node BNode) setOffset(idx uint16, offset uint16)
```

**Purpose:** Offsets record where each key-value pair *starts*, relative to the beginning of the KV section, which is what makes variable-length keys/values possible — without them, the position of the 5th entry couldn't be found without scanning the first four.

**Parameters:**
| Parameter | Function | Purpose |
|---|---|---|
| `idx` | both | Which KV entry's offset to read/write. |
| `offset` | `setOffset` only | The relative byte offset (from the start of the KV section) at which entry `idx` begins. |

**What it computes:**
- `getOffset(0)` is special-cased to always return `0` (the first KV entry always starts at the beginning of the KV section, so no value needs to be stored for it).
- For `idx > 0`, `getOffset` reads a little-endian `uint16` at `offsetPos(node, idx)`.
- `setOffset` writes a little-endian `uint16` at `offsetPos(node, idx)`, after validating `1 <= idx <= nkeys()`.

**Returns:** `getOffset` returns the `uint16` relative byte offset of entry `idx`, which `kvPos` uses to locate the entry. `setOffset` returns nothing; it mutates the node. Offsets are updated incrementally by `nodeAppendKV`, each new offset being the previous offset plus the size of the entry just written.

#### `kvPos`

```go
func (node BNode) kvPos(idx uint16) uint16
```

**Purpose:** Computes the absolute byte position where key-value entry `idx` begins in the node, combining the fixed-size header/pointer/offset regions with the variable-sized offset for this specific entry.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `idx` | The KV entry index to locate. Must satisfy `idx <= nkeys()` (equality is allowed because this function is also used to compute the end-of-data position for `nbytes()`). |

**What it computes:** `HEADER + 8*nkeys() + 2*nkeys() + node.getOffset(idx)` — the header size, plus the full pointer list, plus the full offset list, plus this entry's relative offset within the KV section.

**Returns:** `uint16` — the absolute byte position of entry `idx`, used by `getKey`, `getVal`, and `nbytes` to know exactly where to read from or measure to.

#### `getKey` / `getVal`

```go
func (node BNode) getKey(idx uint16) []byte
func (node BNode) getVal(idx uint16) []byte
```

**Purpose:** Extract the actual key or value bytes stored at a given entry, decoding the small length-prefixed structure each KV pair uses.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `idx` | Which entry to read. Must be `< nkeys()`; otherwise panics with `"invalid KV index"`. |

**What it computes:** Each KV entry is laid out as `[2-byte key length][2-byte value length][key bytes][value bytes]`.
- `getKey` finds the entry's start via `kvPos(idx)`, reads the 2-byte key length (`klen`) from the first 2 bytes, then slices out `klen` bytes starting 4 bytes into the entry (skipping the two length fields).
- `getVal` reads both `klen` and `vlen` (the next 2 bytes), then slices out `vlen` bytes starting after the header and the key (`pos+4+klen`).

**Returns:** `[]byte` — a slice referencing the key or value data directly inside the node's underlying byte array (no copy). This is why callers that need the data to outlive the node's lifetime (e.g. across further mutation) must be careful — but within COW's "build a whole new node" pattern this is safe and efficient.

#### `nbytes`

```go
func (node BNode) nbytes() uint16
```

**Purpose:** Reports how many bytes of the node are actually in use, which is essential for deciding whether a node has overflowed the 4096-byte page limit and needs to be split, or has shrunk small enough to be merged.

**Parameters:** none (reads from the receiver `node`).

**What it computes:** Calls `kvPos(node.nkeys())` — i.e., the position *just past* the last real KV entry, which equals the total number of bytes the node currently occupies.

**Returns:** `uint16` — the node's used size in bytes. Compared against `BTREE_PAGE_SIZE` in `nodeSplit3` (to trigger a split) and against `BTREE_PAGE_SIZE/4` in `shouldMerge` (to decide if merging is worthwhile).

### Design Techniques

- **Fixed-size pages** — 4096-byte B+Tree pages
- **Binary serialization** — node metadata is stored directly in a byte array
- **Offset-based storage** — allows variable-sized keys and values
- **Little-endian encoding** — used for integer serialization
- **Page-oriented design** — prepares the tree for disk-based storage

## 4. B+Tree Node Operations

### Overview

This section implements the operations required to modify B+Tree nodes. Every one of these functions builds a **new** node rather than modifying an existing one directly (copy-on-write), and each takes the specific inputs it needs to know exactly where to write and what to copy.

### Functions

#### `nodeLookupLE`

```go
func nodeLookupLE(node BNode, key []byte) uint16
```

**Purpose:** Finds the correct position for `key` within `node` — specifically, the index of the last key that is less than or equal to `key`. This single function drives both insertion (find where to insert) and deletion (find what to delete) and internal-node traversal (find which child to descend into).

**Parameters:**
| Parameter | Purpose |
|---|---|
| `node` | The node whose keys are being searched (assumed already sorted). |
| `key` | The target key being looked up. |

**What it computes:** Performs a linear scan starting at index 1 (index 0 is always the sentinel/lowest key and is implicitly `<= key`). For each key, it compares against `key` using `bytes.Compare`: if the node's key is `<= key`, it records that index as the current best match (`found`); once a key is found that is `>= key`, the loop stops early, since keys are sorted and no later key could be a better "largest key `<=` target" answer.

**Returns:** `uint16` — the index of the last key `<= key`. For internal nodes this tells the caller which child pointer to follow; for leaf nodes it tells the caller either the position of an existing key (for update/delete) or the position after which a new key should be inserted.

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

**Purpose:** The fundamental "write one entry" primitive that every node-construction function (`leafInsert`, `nodeSplit2`, `nodeMerge`, `nodeReplaceKidN`, etc.) is built on top of. It writes a pointer, key, and value into slot `idx` of a node and keeps the offset list consistent.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `new` | The destination node being constructed. It must already have its header set (via `setHeader`) so `kvPos`/`getOffset` compute correctly. |
| `idx` | The slot to write into. |
| `ptr` | The child pointer for this slot (meaningful for internal nodes; typically `0` for leaf entries). |
| `key` | The key bytes to store. |
| `val` | The value bytes to store (empty/nil for internal-node routing entries, which only need a key). |

**What it computes:**
1. `new.setPtr(idx, ptr)` — records the pointer for this slot.
2. Computes `pos := new.kvPos(idx)` — where this entry begins.
3. Writes `len(key)` and `len(val)` as two little-endian `uint16` length prefixes at `pos` and `pos+2`.
4. Copies `key` into place at `pos+4`, then copies `val` immediately after the key.
5. Updates the *next* slot's offset (`new.setOffset(idx+1, ...)`) to be the current slot's offset plus `4 + len(key) + len(val)` — i.e., it advances the running offset counter so the next entry knows where it starts.

**Returns:** nothing — it mutates `new` in place. This step-by-step offset bookkeeping is what allows entries to be variable-sized while still being locatable in O(1) via `kvPos`.

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

**Purpose:** Copies a contiguous block of `n` existing entries from `old` into `new`, used whenever a construction function needs to carry forward unchanged entries (e.g. everything before/after an insertion or deletion point) without rewriting the copy logic each time.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `new` | The destination node being built. |
| `old` | The source node to copy entries from. |
| `dstNew` | The starting slot index in `new` to begin writing at. |
| `srcOld` | The starting slot index in `old` to begin reading from. |
| `n` | How many entries to copy. |

**What it computes:** Loops `i` from `0` to `n-1`, and for each `i` calls `nodeAppendKV(new, dstNew+i, old.getPtr(srcOld+i), old.getKey(srcOld+i), old.getVal(srcOld+i))` — i.e., re-reads each old entry's pointer/key/value and re-appends it into the new node at the shifted index.

**Returns:** nothing — mutates `new` in place. This is the workhorse used to "shift" entries left or right around an insertion or deletion point.

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

**Purpose:** Produces a brand-new leaf node equal to `old` but with a new key-value pair inserted at position `idx`, without ever mutating `old`.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `new` | The destination node to build (pre-allocated by the caller, oversized to tolerate a temporary overflow before splitting). |
| `old` | The original leaf node, left untouched. |
| `idx` | The position at which the new key should be inserted. |
| `key`, `val` | The new key-value pair to insert. |

**What it computes:**
1. `new.setHeader(BNODE_LEAF, old.nkeys()+1)` — one more key than before.
2. `nodeAppendRange(new, old, 0, 0, idx)` — copies everything before the insertion point unchanged.
3. `nodeAppendKV(new, idx, 0, key, val)` — writes the new entry (pointer `0`, since leaves don't route to children).
4. `nodeAppendRange(new, old, idx+1, idx, old.nkeys()-idx)` — copies everything from `idx` onward in `old`, shifted one slot to the right in `new`.

**Returns:** nothing — the result is left in `new`. `old` is completely unmodified, which is what lets the caller safely mark it for deletion (`tree.del`) only after the new node has been persisted.

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

**Purpose:** Rewrites an internal node so that one child link (at `idx`) is replaced by one or more new child links — used both when a child splits into multiple pages during insertion, and (with exactly one kid) when a child is simply updated without splitting or merging.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `tree` | Needed to call `tree.new(node)` to persist each new child and obtain its page pointer. |
| `new` | The internal node being constructed. |
| `old` | The original internal node whose child link is being replaced. |
| `idx` | The slot in `old` whose child is being replaced. |
| `kids` | One or more new child nodes to link in `idx`'s place. |

**What it computes:**
1. `inc := len(kids)`; sets `new`'s header to `old.nkeys() + inc - 1` keys (removing the old single link, adding `inc` new ones).
2. Copies everything before `idx` unchanged (`nodeAppendRange(new, old, 0, 0, idx)`).
3. For each new child `kids[i]`, persists it with `tree.new(node)` to get its page pointer, then appends an entry with that pointer and the child's first key (`node.getKey(0)`) as the routing key — internal-node entries route by their child's smallest key, so no value is needed.
4. Copies everything after `idx` unchanged, shifted by `inc-1` slots to account for the net change in entry count (`nodeAppendRange(new, old, idx+inc, idx+1, old.nkeys()-(idx+1))`).

**Returns:** nothing — result is left in `new`. This is the insertion-side counterpart to `nodeReplace2Kid` (which does the reverse: collapsing multiple links into one during deletion).

#### `nodeSplit2`

```go
func nodeSplit2(left BNode, right BNode, old BNode)
```

**Purpose:** Divides an oversized node roughly in half by byte size (not just by key count, since keys/values are variable-length), producing two nodes that each individually fit within one page.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `left` | Destination for the first half of `old`'s entries. |
| `right` | Destination for the second half of `old`'s entries. |
| `old` | The oversized node being split. Must have at least 2 keys (panics otherwise, since a single-key node cannot be split). |

**What it computes:**
1. Starts with a naive split point `nleft := old.nkeys() / 2`.
2. Computes the byte size the left node would occupy for that split point, and decrements `nleft` in a loop until the left side actually fits within `BTREE_PAGE_SIZE`. Panics if this drives `nleft` below 1 (an unsplittable node).
3. Computes the byte size the right side would occupy for the resulting split point, and increments `nleft` in a loop until the right side also fits within `BTREE_PAGE_SIZE`. Panics if this drives `nleft >= old.nkeys()` (the right side would then have no keys left).
4. Sets both `left` and `right` headers with the same node type as `old` and their respective key counts, then uses `nodeAppendRange` to copy the first `nleft` entries into `left` and the remaining `nright` into `right`.
5. A final sanity check panics if `right` somehow still exceeds `BTREE_PAGE_SIZE`.

**Returns:** nothing — the two halves are left in `left` and `right`. The iterative adjustment (rather than a fixed 50/50 split) is necessary because entries have variable size, so an even key-count split doesn't guarantee an even byte-size split.

#### `nodeSplit3`

```go
func nodeSplit3(old BNode) (uint16, [3]BNode)
```

**Purpose:** Handles the full range of outcomes a single insertion can produce: a node might not need splitting at all, might split into two, or — in the rare case where even half of an oversized node is still too big — might need to split into three.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `old` | The node to potentially split, already containing the newly inserted entry and possibly oversized as a result. |

**What it computes:**
1. If `old.nbytes() <= BTREE_PAGE_SIZE`, no split is needed: truncates `old` to exactly `BTREE_PAGE_SIZE` and returns it as the sole result.
2. Otherwise allocates `left` (double-sized, to tolerate a temporary overflow) and `right`, and calls `nodeSplit2(left, right, old)`.
3. If the resulting `left` now fits in one page, truncates it and returns 2 nodes (`left`, `right`).
4. If `left` is *still* oversized (a rare edge case with many small keys), it is split again into `leftleft` and `middle` via a second `nodeSplit2` call, and the function returns all 3 nodes (`leftleft`, `middle`, `right`). A final check panics if `leftleft` is still oversized, which should not be reachable given the page-size constraints validated at startup.

**Returns:** `(uint16, [3]BNode)` — the count of valid nodes produced (1, 2, or 3) and an array holding them (only the first `count` entries are meaningful). The caller (`BTree.Insert`, `nodeInsert`) uses the count to decide whether the parent needs one, two, or three child links, and whether the root itself needs to grow a new internal level.

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

**Purpose:** Produces a new leaf node identical to `old` except that the value at an *existing* key (`idx`) is replaced — used when `Insert` is called with a key that already exists, rather than a brand-new key. Unlike `leafInsert`, the key count does not change.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `new` | The destination node being built. |
| `old` | The original leaf, left unmodified. |
| `idx` | The index of the existing key whose value is being replaced. |
| `key`, `val` | The (unchanged) key and its new value. |

**What it computes:**
1. `new.setHeader(BNODE_LEAF, old.nkeys())` — same key count as before.
2. Copies everything before `idx` unchanged.
3. Writes the updated entry at `idx` via `nodeAppendKV`.
4. Copies everything after `idx` unchanged (note the copy starts at `idx+1` in both source and destination, since no slot was added or removed).

**Returns:** nothing — result is in `new`. Because `old` is untouched, this preserves copy-on-write even for simple value updates.

#### `treeInsert`

```go
func treeInsert(
    tree *BTree,
    node BNode,
    key []byte,
    val []byte,
) BNode
```

**Purpose:** The recursive engine that walks the tree from a given node down to the correct leaf and performs the insertion or update, returning a (possibly oversized) replacement for `node`.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `tree` | Provides the `get`/`new`/`del` callbacks needed to fetch children and persist new nodes. |
| `node` | The current node being processed (root of the subtree being updated). |
| `key`, `val` | The key-value pair being inserted or updated. |

**What it computes:**
1. Allocates `new` at *double* the page size — deliberately oversized so a temporary overflow (before `nodeSplit3` trims it) doesn't overrun the buffer.
2. Finds `idx := nodeLookupLE(node, key)`.
3. If `node` is a leaf: if `key` already exists at `idx` (`bytes.Equal`), calls `leafUpdate`; otherwise calls `leafInsert` at `idx+1` (inserting after the found position, since `idx` is the last key `<=` the target).
4. If `node` is internal: delegates to `nodeInsert`, which recurses into the appropriate child.
5. Panics on any other node type, which should be unreachable.

**Returns:** `BNode` — the updated (potentially oversized) node, which the caller is responsible for passing through `nodeSplit3` to bring back within page-size limits.

#### `nodeInsert`

```go
func nodeInsert(
    tree *BTree,
    new BNode,
    node BNode,
    idx uint16,
    key []byte,
    val []byte,
)
```

**Purpose:** Handles the internal-node half of insertion: descend into the correct child, let it insert/split, then rewrite this node's child links to reflect the result.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `tree` | Callbacks for fetching/persisting/deleting pages. |
| `new` | The destination internal node being built to replace `node`. |
| `node` | The current (old) internal node. |
| `idx` | The child slot to descend into, as found by `nodeLookupLE`. |
| `key`, `val` | The key-value pair being inserted. |

**What it computes:**
1. `kptr := node.getPtr(idx)` — the child page's pointer.
2. Recursively calls `treeInsert(tree, tree.get(kptr), key, val)` to insert into that child, yielding a possibly oversized `knode`.
3. Calls `nodeSplit3(knode)` to normalize it into 1–3 page-sized nodes.
4. Deletes the old child page (`tree.del(kptr)`), since it has been superseded.
5. Calls `nodeReplaceKidN(tree, new, node, idx, split[:nsplit]...)` to rewrite this node's link(s) at `idx` to point at the new child(ren).

**Returns:** nothing — the result is written into `new` (passed by the caller). This is the mechanism by which a split three levels down eventually causes a parent (and potentially the root) to gain new links.

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

## 5. High-Level BTree Interface (`tree.go`)

### Overview

`tree.go` exposes the B+Tree through a small public API (`Insert`/`Delete`) and owns the special-case logic for creating and growing/shrinking the root.

```go
type BTree struct {
    root uint64
    get  func(uint64) []byte
    new  func([]byte) uint64
    del  func(uint64)
}
```

The three callback fields (`get`, `new`, `del`) deliberately decouple the tree's logic from how pages are actually stored — they could be backed by disk pages, an in-memory map, or anything else.

#### `BTree.Insert`

```go
func (tree *BTree) Insert(key []byte, val []byte)
```

**Purpose:** The single public entry point for adding or updating a key-value pair, including the special handling required when the tree is empty or when the root itself needs to split.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `key` | The key to insert or update. |
| `val` | The value to associate with `key`. |

**What it computes:**
1. **Empty-tree case** (`tree.root == 0`): allocates a fresh page-sized leaf, sets its header to `BNODE_LEAF` with 2 keys, appends a sentinel empty key at index 0 (`nodeAppendKV(root, 0, 0, nil, nil)`), then appends the real key-value pair at index 1. Persists it with `tree.new(root)` and stores the returned pointer as the new root. This early-returns since there is nothing further to do.
2. **General case:** fetches the current root (`tree.get(tree.root)`) and calls `treeInsert(tree, root, key, val)`, producing a possibly oversized updated root node.
3. Calls `nodeSplit3(node)` to normalize the result into 1–3 page-sized nodes, and deletes the old root page (`tree.del(tree.root)`), since it has been superseded.
4. If `nsplit > 1` (the root itself overflowed and had to split), a brand-new internal root is created: its header is set to `BNODE_NODE` with `nsplit` keys, and for each split fragment, the fragment is persisted (`tree.new(knode)`) and an entry linking to it (using the fragment's first key as the routing key) is appended. This new internal node becomes the tree's root — this is how the tree grows taller by one level.
5. If `nsplit == 1` (no root-level split), the single updated node is simply persisted and becomes the new root.

**Returns:** nothing — the effect is entirely the side effect of updating `tree.root` (and the underlying storage via `tree.new`/`tree.del`). This function is what keeps the root pointer always valid and pointing at a properly-sized page after every insertion, no matter how deep the underlying changes were.

## 6. Testing

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

## 7. Chapter 5 — B+Tree Deletion and Testing

### Overview

Chapter 5 completes the B+Tree modification operations by adding deletion and the high-level interfaces around the tree. It covers:

- High-level `BTree` interfaces
- Maintaining the root node
- Sentinel value
- Merging B+Tree nodes
- Recursive B+Tree deletion
- Testing the B+Tree using in-memory page callbacks

The core idea is that deletion is the counterpart to insertion:

```
Insertion:
    node becomes too large
            ↓
          split

Deletion:
    node becomes too small
            ↓
          merge
```

The book describes §5.3 as similar to insertion, with splitting replaced by merging.

### 7.1 Sentinel value

When the first root is created (see `BTree.Insert` above), the tree inserts an empty key at index 0:

```go
nodeAppendKV(root, 0, 0, nil, nil)
```

**Purpose:** Ensures `nodeLookupLE()` always finds a valid position, even for a search key smaller than every real key in the node. Without it, a key smaller than everything else would have no key `<= target` to return.

```
Without sentinel:

[A B C]
 ^
 key < A

No key <= search key


With sentinel:

["" A B C]
 ^
 key < A

"" <= key
```

The empty key is the lowest possible key by sort order, allowing `nodeLookupLE()` to always find a containing position.

### 7.2 Node merging

Deletion can leave a node with very little data. Instead of allowing many nearly-empty nodes to remain in the tree, a small node is merged with a sibling.

#### `leafDelete`

```go
func leafDelete(new BNode, old BNode, idx uint16)
```

**Purpose:** Produces a new leaf node equal to `old` but with the entry at `idx` removed, following the same copy-on-write approach as `leafInsert`.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `new` | The destination node being built. |
| `old` | The original leaf, left untouched. |
| `idx` | The index of the entry to remove. |

**What it computes:**
1. `new.setHeader(BNODE_LEAF, old.nkeys()-1)` — one fewer key.
2. `nodeAppendRange(new, old, 0, 0, idx)` — copies everything before `idx` unchanged.
3. `nodeAppendRange(new, old, idx, idx+1, old.nkeys()-(idx+1))` — copies everything *after* `idx` in `old`, shifted left by one slot in `new`, effectively skipping over the deleted entry.

**Returns:** nothing — result is in `new`.

```
Old leaf:
[A B C D]

Delete C

New leaf:
[A B D]
```

#### `nodeMerge`

```go
func nodeMerge(new BNode, left BNode, right BNode)
```

**Purpose:** Combines two adjacent, undersized sibling nodes into a single node, reducing the number of nearly-empty pages in the tree.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `new` | The destination node that will hold the combined contents. |
| `left` | The sibling whose contents come first. |
| `right` | The sibling whose contents come second. Order matters — `left`'s keys must all be less than `right`'s keys to preserve sort order. |

**What it computes:**
1. `new.setHeader(left.btype(), left.nkeys()+right.nkeys())` — combined key count, same node type as the (identical-typed) siblings.
2. `nodeAppendRange(new, left, 0, 0, left.nkeys())` — copies all of `left`'s entries first.
3. `nodeAppendRange(new, right, left.nkeys(), 0, right.nkeys())` — appends all of `right`'s entries immediately after.

**Returns:** nothing — result is in `new`.

```
Left:     [A B]
Right:    [C D]

        ↓

Merged:   [A B C D]
```

#### `nodeReplace2Kid`

```go
func nodeReplace2Kid(
    new BNode,
    old BNode,
    idx uint16,
    ptr uint64,
    key []byte,
)
```

**Purpose:** Rewrites an internal node so that two adjacent child links (representing the two siblings that were just merged) are collapsed into a single link pointing at the merged node. This is the deletion counterpart of `nodeReplaceKidN()`, which expands one link into several during insertion.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `new` | The internal node being constructed. |
| `old` | The original internal node whose two child links are being collapsed. |
| `idx` | The index of the first of the two adjacent links being replaced (the pair `idx` and `idx+1`). |
| `ptr` | The page pointer of the newly persisted merged child. |
| `key` | The routing key for the merged child — its first key, so lookups still route correctly. |

**What it computes:**
1. `new.setHeader(BNODE_NODE, old.nkeys()-1)` — one fewer child link than before (two collapsed into one).
2. Copies everything before `idx` unchanged.
3. Appends a single new entry at `idx` with the merged child's pointer and routing key.
4. Copies everything after the *pair* (`idx+2` onward in `old`) into `new` starting at `idx+1`, skipping over both of the old links that were just replaced.

**Returns:** nothing — result is in `new`.

```
Before:                 After merging:

Parent                  Parent
    [Left] [Right]            [Merged]

2 child pointers  →  1 child pointer
```

### 7.3 `shouldMerge`

```go
func shouldMerge(
    tree *BTree,
    node BNode,
    idx uint16,
    updated BNode,
) (int, BNode)
```

**Purpose:** After a child has been updated by deletion, decides whether that child is now small enough that it should be merged with a neighboring sibling, and if so, which one — avoiding the accumulation of many nearly-empty nodes.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `tree` | Needed to fetch sibling pages via `tree.get()`. |
| `node` | The parent internal node, used to look up sibling pointers. |
| `idx` | The index of the child (`updated`) within `node`, used to find its left (`idx-1`) and right (`idx+1`) siblings. |
| `updated` | The child node as it looks *after* its own deletion has already been applied. |

**What it computes:**
1. If `updated.nbytes() > BTREE_PAGE_SIZE/4`, the child is not small enough to bother merging — returns `(0, BNode{})` immediately. This quarter-page threshold is a soft minimum-size policy.
2. Otherwise, if a left sibling exists (`idx > 0`), fetches it and checks whether `sibling.nbytes() + updated.nbytes() - HEADER <= BTREE_PAGE_SIZE` (subtracting one `HEADER` because the merged node only needs a single header, not two). If it fits, returns `(-1, sibling)`.
3. Otherwise, if a right sibling exists (`idx+1 < node.nkeys()`), performs the same size check against it. If it fits, returns `(+1, sibling)`.
4. If neither sibling can accommodate a merge, returns `(0, BNode{})`.

**Returns:** `(int, BNode)` — the first value indicates the merge direction: `-1` for "merge with left sibling", `0` for "don't merge", `+1` for "merge with right sibling"; the second value is the sibling node itself (empty if no merge is happening), which the caller (`nodeDelete`) passes directly into `nodeMerge`.

### 7.4 B+Tree deletion

```go
func treeDelete(
    tree *BTree,
    node BNode,
    key []byte,
) BNode
```

**Purpose:** The recursive entry point that walks the tree from a given node down to the leaf containing `key`, deletes it if present, and propagates the resulting structural changes back up.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `tree` | Provides `get`/`new`/`del` callbacks. |
| `node` | The current node being processed. |
| `key` | The key to delete. |

**What it computes:**
1. `idx := nodeLookupLE(node, key)` — finds the candidate position/child.
2. If `node` is a leaf: checks `bytes.Equal(node.getKey(idx), key)`. If the key isn't actually present, returns an empty `BNode{}` as a "not found" signal. Otherwise allocates a page-sized `new`, calls `leafDelete(new, node, idx)`, and returns it.
3. If `node` is internal: delegates to `nodeDelete(tree, node, idx, key)`.
4. Panics on any other node type.

**Returns:** `BNode` — either the updated node (non-empty) or an empty `BNode{}` meaning the key wasn't found anywhere in this subtree. This empty-slice convention is what lets `nodeDelete` detect a failed lookup partway down the tree and abort cleanly without doing any merge/replace work.

```
treeDelete(root, key)
       |
       v
 nodeLookupLE()
       |
       v
 determine node type
       |
   +---+---+
   |       |
 Leaf    Internal
   |       |
   |       v
   |   nodeDelete()
   |       |
   |       v
   |   recurse into child
   |
   v
leafDelete()
```

### 7.5 Deleting from an internal node

```go
func nodeDelete(
    tree *BTree,
    node BNode,
    idx uint16,
    key []byte,
) BNode
```

**Purpose:** Handles the internal-node half of deletion: recurse into the correct child, then decide whether the resulting (smaller) child should simply replace the old one, be merged with a sibling, or (if it's now empty with no sibling available) have its emptiness propagated further up the tree.

**Parameters:**
| Parameter | Purpose |
|---|---|
| `tree` | Callbacks for fetching/persisting/deleting pages. |
| `node` | The current internal node. |
| `idx` | The child slot to recurse into, from `nodeLookupLE`. |
| `key` | The key to delete. |

**What it computes:**
1. `kptr := node.getPtr(idx)` — the child's page pointer.
2. `updated := treeDelete(tree, tree.get(kptr), key)` — recursively deletes from that child.
3. If `len(updated) == 0`, the key wasn't found anywhere below — immediately returns an empty `BNode{}` to propagate the "not found" result upward without modifying anything.
4. Otherwise, the old child page is deleted (`tree.del(kptr)`), since it's been superseded, and a new page-sized `new` node is allocated for the rewritten parent.
5. Calls `shouldMerge(tree, node, idx, updated)` to get `mergeDir` and `sibling`, then handles four cases:
   - **`mergeDir < 0` (merge left):** allocates `merged`, calls `nodeMerge(merged, sibling, updated)` (left sibling's contents first), deletes the old left sibling's page (`tree.del(node.getPtr(idx-1))`), persists `merged` and calls `nodeReplace2Kid(new, node, idx-1, tree.new(merged), merged.getKey(0))` to collapse both old links into one.
   - **`mergeDir > 0` (merge right):** the same steps but with argument order reversed — `nodeMerge(merged, updated, sibling)`, since `updated` is now the left-hand side of the merge — deleting the old right sibling and calling `nodeReplace2Kid(new, node, idx, ...)`.
   - **`mergeDir == 0 && updated.nkeys() == 0`:** the child became completely empty but has no sibling it can merge into. Rather than crash or leave a dangling reference, `new.setHeader(BNODE_NODE, 0)` propagates the emptiness upward — this node's own parent will handle merging it away on a subsequent pass.
   - **`mergeDir == 0 && updated.nkeys() > 0`:** no merge is needed or possible; `nodeReplaceKidN(tree, new, node, idx, updated)` simply swaps in the updated (smaller but non-empty) child in place of the old one.

**Returns:** `BNode` — the rewritten parent node (or an empty `BNode{}` if the key was never found), reflecting whichever of the four cases applied.

#### Case 1 — Merge with left sibling

```go
merged := BNode(make([]byte, BTREE_PAGE_SIZE))

nodeMerge(
    merged,
    sibling,
    updated,
)

tree.del(node.getPtr(idx - 1))

nodeReplace2Kid(
    new,
    node,
    idx-1,
    tree.new(merged),
    merged.getKey(0),
)
```

#### Case 2 — Merge with right sibling

```go
nodeMerge(
    merged,
    updated,
    sibling,
)

tree.del(node.getPtr(idx + 1))

nodeReplace2Kid(
    new,
    node,
    idx,
    tree.new(merged),
    merged.getKey(0),
)
```

#### Case 3 — Empty child with no sibling

```go
case mergeDir == 0 && updated.nkeys() == 0:
    new.setHeader(BNODE_NODE, 0)
```

#### Case 4 — No merge required

```go
case mergeDir == 0 && updated.nkeys() > 0:
    nodeReplaceKidN(
        tree,
        new,
        node,
        idx,
        updated,
    )
```

### 7.6 Complete deletion flow

```
                    Delete(key)
                        |
                        v
                 treeDelete(root)
                        |
                        v
                 nodeLookupLE()
                        |
             +----------+----------+
             |                     |
           LEAF                 INTERNAL
             |                     |
       key exists?            nodeDelete()
        /       \                  |
      No        Yes                v
      |          |           treeDelete(child)
      |          |                  |
      |       leafDelete()          v
      |                         updated child
      |                              |
      |                              v
      |                         shouldMerge()
      |                              |
      |                 +------------+------------+
      |                 |            |            |
      |               left         none        right
      |                 |            |            |
      |               merge      replace       merge
      |                 |            |            |
      +-----------------+------------+------------+
                                   |
                                   v
                             updated parent
```

### 7.7 Copy-on-write throughout deletion

Deletion remains copy-on-write — existing tree nodes are never modified directly.

```
Old tree                     New tree

       Root                        New Root
      /    \                      /       \
   Node    Node        Delete  New Node  New Node
           |            ==>               |
          Leaf                          New Leaf
```

Old nodes are eventually passed to `tree.del(...)`, while new nodes are allocated using `tree.new(...)`. This maintains the same architecture established during insertion.

### 7.8 Chapter 5 testing

Section 5.4 introduces testing by replacing actual disk-page management with an in-memory page map:

```go
pages map[uint64]BNode
```

The three B+Tree callbacks operate on this map:

| Callback | Behavior |
|---|---|
| `get` | read node from map |
| `new` | allocate/store node in map |
| `del` | remove node from map |

This lets the B+Tree be tested independently of the database's disk-storage layer.

### 7.9 What is verified

Two main categories of correctness are checked:

1. **Structure** — keys are sorted; node sizes are within limits.
2. **Data** — the contents of the B+Tree match a reference data structure, using a Go map (`ref map[string]string`) as the reference:

```
B+Tree                 Reference map

"A" → "10"             "A" → "10"
"B" → "20"       ==    "B" → "20"
"C" → "30"             "C" → "30"
```

The test cases themselves are left as an exercise in the book.

### 7.10 Functions implemented in Chapter 5

| Function | Purpose |
|---|---|
| `BTree.Insert()` | Insert/update a key-value pair, including root creation and splitting |
| `BTree.Delete()` | Delete a key |
| `leafDelete()` | Remove a KV from a leaf |
| `nodeMerge()` | Merge two nodes |
| `nodeReplace2Kid()` | Replace two child links with one |
| `shouldMerge()` | Decide whether/which sibling to merge |
| `treeDelete()` | Recursively delete a key |
| `nodeDelete()` | Handle deletion from an internal node |

### 7.11 Design techniques used

- **Copy-on-write** — nodes are replaced rather than modified in-place.
- **Recursive tree modification** — insertion and deletion recursively descend to the leaf and propagate changes upward.
- **Split/merge symmetry** — Insertion → Split, Deletion → Merge.
- **Callback-based page management** — the B+Tree doesn't know whether nodes are stored in memory, on disk, or elsewhere; it only uses `get()`, `new()`, `del()`.
- **Sentinel value** — an empty key ensures `nodeLookupLE()` always has a valid starting position.
- **Reference-based testing** — the B+Tree can be compared against a simple Go map to verify its logical contents.

### 7.12 Chapter 5 status in our implementation

At this point, the implementation has the following Chapter 5 components:

```
internal/btree/
├── node.go
│   ├── leafDelete()
│   ├── nodeMerge()
│   ├── nodeReplace2Kid()
│   └── shouldMerge()
│
└── tree.go
    ├── BTree.Insert()
    ├── treeDelete()
    └── nodeDelete()
```

The main remaining work from the chapter is the §5.4 testing infrastructure and tests. The book specifically leaves the actual test cases as an exercise.

> **Chapter 5 in one sentence:** Chapter 5 completes the copy-on-write B+Tree by adding high-level insertion/deletion interfaces, recursively deleting keys, merging under-filled nodes, and testing the tree independently of disk storage.

## 8. Current Design Principles

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
| Node merging | Reduce nearly-empty nodes during deletion |
| Recursive deletion | Propagate structural changes up the tree |
