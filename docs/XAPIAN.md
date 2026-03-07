# Full-Text Search: Reading Xapian Indexes from ZIM Files

## Status

**Feasibility study** -- this document evaluates whether gozim can natively read
the Xapian full-text search indexes embedded in ZIM files, and compares
alternative approaches for providing search functionality.

## Background

### What Xapian Is

[Xapian](https://xapian.org) is a C++ full-text search engine library. It
provides term indexing, boolean and probabilistic retrieval, phrase search,
spelling correction, and faceting. The canonical implementation is roughly
150,000 lines of C++.

### Where Xapian Indexes Live in ZIM Files

ZIM files created by libzim 7+ embed Xapian databases as binary blobs in the
`X` namespace:

| ZIM Path             | Purpose                                |
|----------------------|----------------------------------------|
| `X/fulltext/xapian`  | Full-text search index (article body)  |
| `X/title/xapian`     | Title/suggestion index                 |

These are standard ZIM content entries. Their raw bytes can be read with
`archive.EntryByPath("X/fulltext/xapian")` and `entry.ReadContent()` -- gozim
can already do this today. The challenge is *interpreting* those bytes, which are
a serialized Xapian database.

### What gozim Already Provides

- `HasFulltextIndex() bool` -- checks whether `X/fulltext/xapian` exists
- `HasTitleIndex() bool` -- checks whether `X/title/xapian` exists
- `EntriesByTitlePrefix(ns, prefix)` -- O(log N + k) binary search over the
  sorted title pointer list (no Xapian needed)
- `EntriesByTitlePrefixFold(ns, prefix)` -- O(N) case-insensitive variant

Title prefix search covers the "suggestion/autocomplete" use case adequately.
The gap is **full-text search** -- finding articles that contain specific words
in their body text.

---

## Xapian On-Disk Format Complexity

No formal binary specification for the Xapian database format exists. The format
is defined entirely by the C++ source code. What follows is distilled from
reading that source.

### Database Backends

| Backend | Introduced | Default In    | Notes                                        |
|---------|-----------|---------------|----------------------------------------------|
| Glass   | 1.3.2     | Xapian 1.4.x  | Copy-on-write B+-trees, 8 KB blocks          |
| Honey   | 1.4.10    | Not yet default | 30-40% smaller, read-only optimized          |
| Chert   | 1.2.x     | Xapian 1.2.x  | Deprecated predecessor to Glass              |

ZIM files use the **single-file compacted format** -- a read-only
representation where all tables are concatenated into one byte stream with an
offset table. This is a subset of the full format (no WAL, no per-table files),
but still requires parsing the same B+-tree structures.

### Table Structure

A Xapian database contains up to six B+-tree tables:

| Table      | Contents                                       |
|------------|------------------------------------------------|
| `postlist` | Term -> list of document IDs + term frequency  |
| `termlist` | Document ID -> list of terms in that document  |
| `docdata`  | Document ID -> arbitrary stored data           |
| `position` | Term + document -> list of word positions      |
| `spelling` | Spelling correction data                       |
| `synonym`  | Synonym mappings                               |

For read-only search, the minimum requirement is the `postlist` table (term
lookup) and optionally `position` (phrase queries) and `docdata` (retrieving
stored metadata like document titles).

### What Parsing Requires

1. **B+-tree traversal** -- Each table is a B+-tree with 8 KB blocks. Internal
   nodes contain keys and child pointers. Leaf nodes contain key-value pairs
   with prefix compression.

2. **Postings list decoding** -- Document ID lists are delta-encoded using
   variable-length integers. Term frequencies and within-document frequencies
   are interleaved. The encoding has changed across Xapian versions.

3. **Single-file offset resolution** -- The compacted single-file format
   prepends a header mapping table names to byte offsets within the file. These
   offsets are relative to the start of the Xapian data, which is itself
   embedded at an arbitrary offset within the ZIM file's cluster storage.

4. **Version detection** -- The format revision is embedded in the file header.
   Glass and Honey use different encodings. A reader must handle whichever
   version the ZIM creator used.

5. **Unicode and stemming** -- Xapian lowercases and stems terms during
   indexing. A query engine must apply the same transformations to match.

### Estimated Scope

A minimal read-only implementation covering Glass single-file format, basic term
lookup, and postings list decoding: **3,000 - 5,000+ lines of Go**. This
estimate is based on the roughly 50,000 lines of C++ in Xapian's backend code,
accounting for the fact that a read-only subset omits transaction handling,
compaction, and write paths.

---

## Options

### Option 1: Do Nothing Extra

Keep the current title-prefix search. Document that full-text search is not
supported. Users who need it can use external tools (kiwix-serve, xapian-delve).

**Effort:** Zero.

**Pros:**
- No maintenance burden
- Title prefix search already covers autocomplete/suggestion use cases
- Honest about scope -- gozim is a ZIM reader, not a search engine

**Cons:**
- No full-text search at all
- Users expecting kiwix-serve parity will be disappointed

### Option 2: Minimal Pure Go Xapian Reader

Implement a read-only parser for the Glass single-file B+-tree format. Support
basic term lookup and postings list decoding. No phrase queries, no spelling, no
Honey backend.

**Effort:** 3,000 - 5,000 lines of Go. Estimated 4 - 8 weeks for a single
developer. Ongoing maintenance as Xapian evolves.

**Pros:**
- Pure Go, zero external dependencies
- Reads the indexes already embedded in ZIM files
- No first-load delay or extra disk usage

**Cons:**
- No specification to code against -- must reverse-engineer from C++ source
- High risk of subtle bugs (off-by-one in delta decoding, version mismatches)
- Maintenance nightmare: Xapian format changes break the reader silently
- Only Glass single-file supported -- Honey adoption will require a second
  implementation pass
- No stemming or Unicode normalization means queries may not match the indexed
  terms unless the same algorithms are reimplemented
- Untestable at scale without a large corpus of known-good Xapian databases

### Option 3: Build-Your-Own Index at Serve Time

On first access (or via explicit CLI command), extract all article text from the
ZIM, feed it into a pure Go search engine (Bleve or Bluge), and persist the
index to disk. Subsequent searches use the pre-built index.

**Effort:** 500 - 1,000 lines of Go. 1 - 2 weeks. Bleve/Bluge are
well-maintained with stable APIs.

**Pros:**
- Pure Go, well-tested search engine with active communities
- Full-featured: stemming, fuzzy matching, faceting, relevance ranking
- Works with any ZIM file, even those without embedded Xapian indexes
- Index format is controlled by gozim, not by upstream Xapian

**Cons:**
- First-load penalty: indexing a large Wikipedia ZIM takes minutes to tens of
  minutes and requires reading + decompressing every article
- Disk usage: a Bleve index for English Wikipedia is roughly 2 - 4 GB
- Duplicates data already present in the ZIM's Xapian index
- Requires HTML parsing to extract article text (adds a dependency like
  `golang.org/x/net/html`)
- Cache invalidation: if the ZIM file changes, the index is stale

### Option 4: Optional CGo Xapian Bindings

Provide Xapian search behind a build tag (`//go:build cgo && xapian`). Users
who want full-text search install the Xapian C++ library and build with
`-tags xapian`. The default build remains pure Go.

**Effort:** 300 - 600 lines of Go (thin wrapper). 1 - 2 weeks. Requires
familiarity with CGo and Xapian's C API.

**Pros:**
- Uses the canonical, battle-tested Xapian implementation
- Reads the exact indexes embedded in ZIM files with perfect fidelity
- Default build is still pure Go -- CGo is opt-in
- Low maintenance: Xapian's C API is stable

**Cons:**
- CGo complicates cross-compilation and static builds
- Users must install `libxapian-dev` (or equivalent) system package
- Build tag splits the API surface -- callers must handle "search not available"
- Defeats the project's core "pure Go, no CGo" principle (even if optional)

### Option 5: External Process / Sidecar

Shell out to `xapian-delve` or a custom sidecar process that reads the Xapian
data extracted from the ZIM and responds over stdin/stdout or a local socket.

**Effort:** 200 - 400 lines of Go. 1 week. Plus a small C++ or Python helper.

**Pros:**
- gozim binary stays pure Go
- Leverages existing Xapian tooling

**Cons:**
- Requires users to install external tools
- IPC overhead and complexity (process lifecycle, error handling, streaming)
- Fragile: version mismatches between the helper and ZIM's Xapian format
- Not a library solution -- unsuitable for embedding gozim in other Go programs
- Worst developer experience of all options

---

## Summary

| Option              | Effort    | Pure Go | Reads ZIM Index | First-Load Cost    | Maintenance | Risk   |
|---------------------|-----------|---------|-----------------|---------------------|-------------|--------|
| 1. Do nothing       | None      | Yes     | N/A             | None                | None        | None   |
| 2. Xapian reader    | 4-8 weeks | Yes     | Yes             | None                | High        | High   |
| 3. Build own index  | 1-2 weeks | Yes     | No (rebuilds)   | Minutes + GB disk   | Medium      | Low    |
| 4. CGo bindings     | 1-2 weeks | No*     | Yes             | None                | Low         | Low    |
| 5. External process | 1 week    | Yes*    | Yes             | None                | Medium      | Medium |

*Option 4 default build is pure Go; CGo is opt-in. Option 5 requires external tools at runtime.

---

## Recommendation

**Short term: Option 1 (do nothing).** The existing title-prefix search covers
the most common interactive use case -- autocomplete and suggestion. Full-text
search is a significant scope expansion with no low-risk pure-Go path.

**Medium term: Option 3 (build-your-own index) if demand materializes.** This
is the only option that is both pure Go and practically maintainable. The
first-load cost is real but acceptable for a self-hosted server (`zimserve`
could build the index in the background on startup and serve an "indexing in
progress" response until ready). A `zimsearch --reindex` CLI command could
pre-build the index.

**Option 2 (native Xapian reader) is not recommended.** The absence of a formal
specification, the complexity of the B+-tree format, and the ongoing maintenance
burden make this a poor investment. The risk of silent data corruption or query
result divergence from canonical Xapian is too high for a library that
prioritizes correctness.

**Option 4 (CGo) is viable but conflicts with the project's identity.** If the
pure-Go constraint is ever relaxed, this becomes the obvious choice.

---

## References

- [Xapian documentation](https://xapian.org/docs/)
- [Xapian Glass backend source](https://github.com/xapian/xapian/tree/master/xapian-core/backends/glass)
- [Xapian Honey backend source](https://github.com/xapian/xapian/tree/master/xapian-core/backends/honey)
- [OpenZIM specification](https://wiki.openzim.org/wiki/ZIM_file_format)
- [libzim search integration](https://github.com/openzim/libzim/tree/main/src/search)
- [Bleve search library](https://github.com/blevesearch/bleve)
- [Bluge search library](https://github.com/blugelabs/bluge)
