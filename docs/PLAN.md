# gozim - Native Go ZIM File Reader Library

## Context

ZIM is the open archive format used by OpenZIM/Kiwix for storing offline wiki content (Wikipedia, Wikimedia projects, etc.). The canonical implementation is libzim (C++). One existing Go library (github.com/akhenakh/gozim) exists but is old and unmaintained. This project creates a modern, pure-Go ZIM reader library with clean, idiomatic APIs, comprehensive tests, and CLI tools — with an eventual goal of building a Go clone of kiwix-serve.

**Key decisions:**
- Module: `github.com/stazelabs/gozim`
- Go 1.22+ (enables `iter.Seq` range-over-func)
- Pure Go only — no CGo
- Full-text search deferred to a later phase
- Read-only focus first

---

## Package Structure

```
github.com/stazelabs/gozim/
├── go.mod
├── go.sum
├── LICENSE
├── README.md
├── docs/
│   ├── PLAN.md                # This file
│   └── ZIM-ISSUES.md          # Compatibility notes for Kiwix ecosystem
├── zim/                       # Core library package
│   ├── archive.go             # Archive: Open, Close, header parsing, entry lookup
│   ├── entry.go               # Entry type (content + redirect), directory parsing
│   ├── item.go                # Item: content entries with data access
│   ├── blob.go                # Blob: raw byte container
│   ├── cluster.go             # Cluster reading, decompression, blob extraction
│   ├── compress.go            # Compression dispatch (none/xz/zstd)
│   ├── header.go              # 80-byte fixed header struct and parsing
│   ├── mime.go                # MIME type list parsing
│   ├── io.go                  # File I/O abstraction (mmap + pread backends)
│   ├── iter.go                # Iterator types for entries (by path, by title)
│   ├── search.go              # Title prefix search (binary search + fold)
│   ├── errors.go              # Sentinel errors
│   ├── checksum.go            # MD5 integrity verification
│   ├── doc.go                 # Package documentation
│   └── *_test.go              # Tests for each file
├── cmd/
│   ├── ziminfo/main.go        # Dump ZIM metadata
│   ├── zimcat/main.go         # Extract entry content by path
│   ├── zimsearch/main.go      # Title prefix search CLI
│   ├── zimverify/main.go      # MD5 checksum verification
│   └── zimserve/main.go       # HTTP server for browsing ZIM content
└── testdata/                  # Small ZIM test files
```

Single `zim` package — avoids premature splitting. Split only if it grows beyond ~3000 lines.

---

## Core Types

### Archive (main entry point)

```go
func Open(path string) (*Archive, error)
func OpenWithOptions(path string, opts ...Option) (*Archive, error)
func (a *Archive) Close() error

// Metadata
func (a *Archive) UUID() [16]byte
func (a *Archive) EntryCount() uint32
func (a *Archive) ClusterCount() uint32
func (a *Archive) HasMainEntry() bool
func (a *Archive) MainEntry() (Entry, error)
func (a *Archive) MIMETypes() []string

// Entry access (binary search over pointer lists)
func (a *Archive) EntryByPath(path string) (Entry, error)
func (a *Archive) EntryByIndex(idx uint32) (Entry, error)
func (a *Archive) EntryByTitle(ns byte, title string) (Entry, error)

// Iteration (Go 1.22+ iter.Seq)
func (a *Archive) Entries() iter.Seq[Entry]
func (a *Archive) EntriesByTitle() iter.Seq[Entry]

// Integrity
func (a *Archive) Verify() error
```

### Entry (value type with *Archive back-pointer for lazy loading)

```go
func (e Entry) Path() string
func (e Entry) Title() string           // returns Path() if empty
func (e Entry) FullPath() string         // namespace/path
func (e Entry) Namespace() byte
func (e Entry) IsRedirect() bool         // mimeIndex == 0xFFFF
func (e Entry) MIMEType() string
func (e Entry) Item() (Item, error)      // error if redirect
func (e Entry) ReadContent() ([]byte, error)  // resolves redirects
func (e Entry) RedirectTarget() (Entry, error)
func (e Entry) Resolve() (Entry, error)  // follows full redirect chain
```

### Item & Blob

```go
func (i Item) Data() (Blob, error)
func (i Item) Size() (int64, error)
func (i Item) MIMEType() string

func (b Blob) Bytes() []byte
func (b Blob) String() string
func (b Blob) Reader() io.Reader
```

### Options

```go
func WithCacheSize(n int) Option    // LRU cluster cache size (default: 16)
func WithMmap(enabled bool) Option  // mmap toggle (default: true on 64-bit)
```

---

## File I/O Strategy

Internal `reader` interface decouples I/O from parsing:

- **mmapReader** (default on 64-bit): Maps entire file via `syscall.Mmap`, leverages OS page cache, zero-copy reads.
- **preadReader** (fallback): Uses `*os.File.ReadAt`. Default on 32-bit or when mmap disabled.

**Lazy loading:** Header + MIME list parsed eagerly on `Open()`. Pointer lists, directory entries, and clusters read on demand. Decompressed clusters cached in an LRU map protected by `sync.Mutex`.

---

## Compression

| Type | Value | Go Library |
|------|-------|------------|
| None | 1 | direct copy |
| LZMA/XZ | 4 | `github.com/ulikunitz/xz` |
| Zstd | 5 | `github.com/klauspost/compress/zstd` |

- Zstd decoder created once per Archive, reused (goroutine-safe with `DecodeAll`)
- XZ reader created per decompression call (lightweight, stateful)
- Types 2/3 (zlib, bzip2) are deprecated — return `ErrUnsupportedCompression`

---

## ZIM Format Key Details

**Header:** 80 bytes, little-endian. Magic `0x44D495A`. Versions 5 and 6.

**Clusters:** Info byte → low 4 bits = compression type, bit 5 = extended offsets (v6). Offset list with N+1 entries (4-byte standard, 8-byte extended). Decompressed data follows offsets.

**Directory entries:** Content entries have cluster/blob numbers. Redirects have `mimeIndex=0xFFFF` and a redirect target index. Paths are null-terminated UTF-8.

**Namespaces:** C (content), M (metadata), W (well-known), X (search indexes).

**Path lookup:** Binary search over URL pointer list — O(log n) reads. With mmap, these are simple memory accesses.

---

## Errors

```go
var (
    ErrInvalidMagic           = errors.New("zim: invalid magic number")
    ErrNotFound               = errors.New("zim: entry not found")
    ErrIsRedirect             = errors.New("zim: entry is a redirect")
    ErrNotRedirect            = errors.New("zim: entry is not a redirect")
    ErrChecksumMismatch       = errors.New("zim: checksum verification failed")
    ErrUnsupportedVersion     = errors.New("zim: unsupported format version")
    ErrUnsupportedCompression = errors.New("zim: unsupported compression type")
)
```

---

## Dependencies

```
github.com/klauspost/compress   # zstd decompression
github.com/ulikunitz/xz         # LZMA/XZ decompression
github.com/spf13/cobra          # CLI framework (cmd/ tools only)
```

---

## Testing Strategy

### Test Files
- Commit a tiny handcrafted ZIM file (~few KB) to `testdata/` for unit tests
- Script or Makefile target to download larger test ZIMs for integration tests
- Use `testing.Short()` to skip slow/large-file tests

### Unit Tests (per file)
- `header_test.go` — parse known bytes, reject bad magic, boundary values
- `mime_test.go` — parse MIME list, empty list, malformed input
- `entry_test.go` — content entries, redirect entries, namespace handling
- `cluster_test.go` — each compression type, standard & extended offsets, blob extraction
- `compress_test.go` — round-trip each compression type
- `io_test.go` — both mmap and pread backends produce identical results
- `iter_test.go` — iteration order, early termination
- `checksum_test.go` — pass and fail cases

### Integration Tests
- Open real ZIM, verify entry count, read specific articles
- Navigate to main page, verify HTML content
- Redirect chain resolution
- Multi-cluster reading

### Fuzz Tests
- `FuzzParseHeader` — must not panic on arbitrary input
- `FuzzParseDirectoryEntry` — must not panic
- `FuzzDecompressCluster` — must not panic

### Benchmarks
- `BenchmarkEntryByPath` — binary search performance
- `BenchmarkReadContent` — end-to-end content retrieval
- `BenchmarkClusterDecompress` — per-compression-type throughput

---

## Phased Implementation

### Phase 1: Core Reading ✓ Done
**Goal:** Open a ZIM file, parse header, look up entries, read content.

1. `go.mod` init
2. `errors.go` — sentinel errors
3. `header.go` + tests — 80-byte header parsing
4. `io.go` + tests — reader interface, pread impl, then mmap impl
5. `mime.go` + tests — MIME type list parsing
6. `entry.go` + tests — directory entry parsing (content + redirect)
7. `compress.go` + tests — decompression dispatch
8. `cluster.go` + tests — cluster reading, blob extraction, standard + extended offsets
9. `blob.go` + `item.go` — Blob and Item types
10. `archive.go` + tests — `Open`, `Close`, `EntryByIndex`, `EntryByPath` (binary search), `MainEntry`

### Phase 2: Complete API + CLI Tools ✓ Done
**Goal:** Full read API, iteration, CLI tools.

1. `iter.go` + tests — `Entries()`, `EntriesByTitle()` via `iter.Seq`
2. `archive.go` additions — `EntryByTitle`, metadata getters
3. `entry.go` additions — `Resolve()`, `ReadContent()`
4. `checksum.go` + tests — MD5 verification
5. LRU cluster cache integration
6. `Option` pattern for `OpenWithOptions`
7. `cmd/ziminfo/main.go`
8. `cmd/zimcat/main.go`
9. Integration tests with real ZIM files
10. Benchmarks

### Phase 3: Polish & Release (partially done)
1. ✓ Fuzz tests
2. ✓ Performance profiling & optimization
3. Cross-platform testing (Linux, macOS, Windows)
4. ✓ Documentation: README with examples, GoDoc comments
5. CI/CD (GitHub Actions: multi-OS, race detector, fuzz)
6. Tag v0.1.0

### Phase 4: HTTP Server ✓ Done
1. `cmd/zimserve` — HTTP server via `net/http`
2. Content-Type from MIME types, URL routing, redirect handling
3. Static file serving for ZIM resources
4. Multi-ZIM library support

### Phase 5: Search ✓ Done
**Goal:** Title prefix search using the sorted title pointer list; `zimsearch` CLI tool.

**Why not Xapian full-text search:** ZIM files embed Xapian databases (`X/fulltext/xapian`, `X/title/xapian`) in the X namespace. Parsing Xapian's on-disk format in pure Go would mean reimplementing a complex C++ database engine. Incompatible with the pure-Go/no-CGo constraint.

**What we implement instead (no new dependencies):**

1. `zim/search.go` — new search methods on `*Archive`:
   - `EntriesByTitlePrefix(ns byte, prefix string) iter.Seq[Entry]` — O(log N + k), binary search to lower bound, iterate while prefix matches
   - `EntriesByTitlePrefixFold(ns byte, prefix string) iter.Seq[Entry]` — O(N), case-insensitive Unicode fold, full linear scan (documented as slower)
   - `HasFulltextIndex() bool` — checks for `X/fulltext/xapian`
   - `HasTitleIndex() bool` — checks for `X/title/xapian`

2. `zim/search_test.go` — tests against `testdata/small.zim`

3. `cmd/zimsearch/main.go` — Cobra CLI:
   ```
   zimsearch <file.zim> <query> [-n namespace] [-l limit] [-i]
   ```
   Outputs matching `FullPath\tTitle` lines, one per result.

### Phase 6: Streaming & Content Size ✓ Done
**Goal:** Support efficient serving of large content without full buffering.

1. `Entry.ContentSize() (int64, error)` — return blob size from offset table without decompressing
2. `Entry.ContentReader() (io.Reader, error)` — streaming content access, resolves redirects
3. `Item.Reader() (io.Reader, error)` — streaming blob access
4. Update zimserve to use streaming responses with correct `Content-Length`

**Rationale:** `ReadContent()` loads the entire blob into memory. ZIM files can contain video, PDF, and other large media — a single blob can be hundreds of MB. Streaming avoids OOM and reduces latency to first byte.

**Implementation notes:**
- Cluster decompression is all-or-nothing (XZ/Zstd require full decompression), so the "stream" reads from the already-decompressed cached cluster
- `ContentSize()` can be computed from the blob offset table (`offsets[i+1] - offsets[i]`) without copying data
- The cluster cache already holds decompressed data, so `ContentReader()` wraps `bytes.NewReader` over the blob slice

### Phase 7: Convenience API ✓ Done
**Goal:** Fill common API gaps.

1. `Archive.RandomEntry(ns byte) (Entry, error)` — random entry within a namespace (for "random article" feature)
2. `Archive.Illustration(size int) ([]byte, error)` — read `M/Illustration_{size}x{size}@1` (favicon/icon)
3. `Archive.EntryCountByNamespace(ns byte) int` — count entries in a namespace without full iteration (uses binary search to find namespace bounds)

### Phase 8: zimserve Enhancements ✓ Done
**Goal:** Bring zimserve closer to kiwix-serve feature parity.

1. **Title search endpoint** — `GET /{slug}/-/search?q=term&limit=25`
   - Uses `EntriesByTitlePrefix` / `EntriesByTitlePrefixFold`
   - Returns HTML results page with links to matching entries
   - JSON API variant: `GET /{slug}/-/search?q=term&format=json`

2. **Browse-by-letter** — `GET /{slug}/-/browse?letter=A`
   - Uses title iteration to list entries starting with a given letter
   - Paginated HTML listing

3. **Favicon route** — `GET /{slug}/favicon.ico`
   - Serves the ZIM's `M/Illustration_48x48@1` entry
   - Falls back to a default icon

4. **Cache headers** — ZIM content is immutable
   - `Cache-Control: public, max-age=31536000, immutable` on content responses
   - `ETag` derived from archive UUID + entry path

5. **Graceful shutdown** — signal handling (SIGINT/SIGTERM)
   - Close all archives cleanly on shutdown
   - Drain in-flight requests with configurable timeout

6. **Random article** — `GET /{slug}/-/random`
   - Uses `Archive.RandomEntry('C')`, redirects to the result

### Phase 9: Multi-Part ZIM Support
**Goal:** Support split ZIM archives (`.zimaa`, `.zimab`, …).

Large ZIM files (e.g., full English Wikipedia, 90+ GB) are commonly distributed as split archives due to filesystem and hosting limitations.

1. Detect `.zimaa` extension and discover all parts (`.zimab`, `.zimac`, …)
2. Implement a `multiReader` that presents split files as a single contiguous `reader`
3. All existing API works transparently — split vs single is an I/O detail
4. Update CLI tools to accept `.zimaa` and auto-discover parts

**Implementation notes:**
- Each part is a fixed-size chunk (typically 2 GB) except the last
- The `multiReader` maps `(offset, length)` → `(partIndex, partOffset)` and delegates to per-part pread/mmap readers
- Parts can be memory-mapped individually on 64-bit systems

---

## Verification

After each phase, verify by:
1. `go test ./...` — all tests pass
2. `go test -race ./...` — no data races
3. `go vet ./...` — no issues
4. Phase 1: Write a small program that opens a ZIM, looks up an article, prints its content
5. Phase 2: Run `ziminfo` and `zimcat` against real ZIM files, compare output with `zimdump` from zim-tools
6. Phase 3: `go test -fuzz` runs for 30+ seconds without crashes
