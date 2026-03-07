# AGENTS.md — gozim

## Project Overview

`gozim` is a native Go library for reading ZIM files (the [OpenZIM](https://wiki.openzim.org/wiki/OpenZIM) archive format used by Kiwix for offline wiki content). It is a pure-Go implementation with no CGo dependencies.

- **Module:** `github.com/stazelabs/gozim`
- **Go version:** 1.22+
- **Status:** Active development — core reading and HTTP server complete

## Repository Structure

```
gozim/
├── zim/                  # Core library (single package)
│   ├── archive.go        # Archive type — Open, Close, entry lookups, cluster cache
│   ├── entry.go          # Entry type — directory entry parsing, redirect resolution
│   ├── item.go           # Item type — content access for non-redirect entries
│   ├── blob.go           # Blob type — raw byte container
│   ├── cluster.go        # Cluster reading, decompression, blob extraction
│   ├── compress.go       # Decompression dispatch (XZ, Zstd)
│   ├── header.go         # 80-byte ZIM header parsing
│   ├── mime.go           # MIME type list parsing
│   ├── io.go             # File I/O abstraction (mmap + pread backends)
│   ├── iter.go           # Iterators (Entries, EntriesByNamespace)
│   ├── search.go         # Title prefix search (EntriesByTitlePrefix, HasFulltextIndex)
│   ├── checksum.go       # MD5 integrity verification
│   ├── errors.go         # Sentinel errors
│   └── *_test.go         # Tests for each file
├── cmd/
│   ├── ziminfo/          # CLI: dump ZIM metadata
│   ├── zimcat/           # CLI: extract content by path
│   ├── zimserve/         # CLI: HTTP server for ZIM content
│   ├── zimsearch/        # CLI: title prefix search
│   └── zimverify/        # CLI: MD5 checksum verification
├── testdata/             # Test ZIM files
├── docs/                 # Design documents and notes
│   ├── PLAN.md           # Design and implementation plan
│   ├── XAPIAN.md         # Xapian full-text search feasibility study
│   ├── TUI.md            # Terminal UI browser design notes
│   └── ZIM-ISSUES.md     # ZIM compatibility notes
└── Makefile              # Build and test targets
```

## Architecture & Key Decisions

- **Single `zim` package:** All types are in one package to avoid circular dependencies and premature abstraction. Export control via Go visibility rules.
- **Pure Go:** Uses `github.com/klauspost/compress/zstd` and `github.com/ulikunitz/xz` for decompression. No CGo.
- **I/O strategy:** Memory-mapped by default on 64-bit (via `syscall.Mmap`), with a `pread`-based fallback. Controlled by the internal `reader` interface.
- **Lazy loading:** Only the header and MIME list are parsed on `Open()`. Entries, clusters, and content are read on demand.
- **Cluster cache:** Simple LRU map caching decompressed clusters (default size: 16). Protected by `sync.Mutex`.
- **Entry as value type:** `Entry` is a struct (not pointer) with a back-pointer to `*Archive` for lazy content access. Entries become invalid after `Archive.Close()`.
- **Iterators:** Uses Go 1.22+ `iter.Seq[Entry]` for range-over-func iteration.
- **Binary search:** `EntryByPath` and `EntryByTitle` use binary search over pointer lists — O(log n) reads.
- **Title prefix search:** `EntriesByTitlePrefix` uses binary search to find the lower bound in the title pointer list then iterates forward — O(log N + k). `EntriesByTitlePrefixFold` is case-insensitive but requires a full linear scan O(N).
- **No Xapian:** ZIM files embed Xapian databases in the X namespace (`X/fulltext/xapian`, `X/title/xapian`). Reading Xapian format requires CGo. `HasFulltextIndex()` / `HasTitleIndex()` detect their presence but do not parse them.

## ZIM Format Quick Reference

- **Header:** 80 bytes, little-endian. Magic `0x44D495A`. Versions 5 and 6 supported.
- **Namespaces:** `C` (content), `M` (metadata), `W` (well-known), `X` (search indexes).
- **Directory entries:** Variable-length. Content entries have cluster/blob numbers. Redirects have `mimeIndex=0xFFFF`.
- **Clusters:** Info byte (compression type + extended flag), then offset list + blob data. Compression: none (1), XZ (4), Zstd (5).
- **Pointer lists:** URL pointers are `uint64` offsets to entries (path-sorted). Title pointers are `uint32` entry indices (title-sorted).

## Coding Conventions

- **Error handling:** Return `error`, use sentinel errors from `errors.go` with `fmt.Errorf("%w", ...)` wrapping.
- **No panics** in library code.
- **Test naming:** `Test<FunctionName>` or `Test<FunctionName><Scenario>`.
- **Test data:** Use `testdataPath()` helper and `skipIfNoTestdata()` for tests requiring ZIM files.
- **Benchmarks:** `Benchmark<Operation>` in `*_test.go` files.

## Testing

```bash
go test ./...              # Run all tests
go test -race ./...        # With race detector
go test -bench=. ./zim/    # Run benchmarks
make testdata              # Download test ZIM files
```

Test ZIM files come from the [openzim/zim-testing-suite](https://github.com/openzim/zim-testing-suite). The small test file (`testdata/small.zim`) is committed to the repo.

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/klauspost/compress` | Zstd decompression (library) |
| `github.com/ulikunitz/xz` | XZ/LZMA decompression (library) |
| `github.com/spf13/cobra` | CLI framework (cmd/ tools only) |

## Implementation Phases

See [docs/PLAN.md](docs/PLAN.md) for full details. Current status:

1. **Phase 1 — Core Reading:** Complete
2. **Phase 2 — Complete API + CLI Tools:** Complete
3. **Phase 3 — Polish & Release:** Complete (except CI/CD)
4. **Phase 4 — HTTP Server (`zimserve`):** Complete
5. **Phase 5 — Search (`zimsearch`):** Complete
- Remaining: CI/CD (GitHub Actions)
