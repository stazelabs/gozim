# gozim Backlog

Consolidated backlog of remaining work to make gozim the definitive pure-Go ZIM library.
Phases 1–8 of [PLAN.md](docs/PLAN.md) are complete. This document tracks all remaining items.

**Priority tiers:**
- **P0 Critical** — Correctness bugs or security issues that affect production use
- **P1 High** — Important for a credible v1.0 release
- **P2 Medium** — Meaningful improvements for competitive feature parity
- **P3 Nice-to-have** — Polish, convenience, or advanced features

**Status key:** `[ ]` not started · `[~]` in progress · `[x]` done

---

## P0 Critical

### Security & Robustness

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 0.1 | **Decompression bomb guard** — Cap decompressed output size (e.g. `io.LimitReader` / zstd `WithDecoderMaxMemory`). `decompressXZ` calls `io.ReadAll` and `decompressZstd` calls `DecodeAll` with no output limit. A small compressed cluster can expand to arbitrary size. | Prevents OOM on malicious or corrupt ZIM files. The existing `maxClusterSize` (4 GiB) only bounds the *compressed* data read from disk. | `zim/compress.go` | `[x]` |
| 0.2 | **Iterator error semantics inconsistency** — `Entries()` and `EntriesByNamespace()` silently skip errors (`continue`), while `EntriesByTitle()`, `EntriesByTitlePrefix()`, and `EntriesByTitlePrefixFold()` stop on error (`return`). Callers cannot distinguish "no more entries" from "I/O error." | A corrupt entry in the middle silently disappears from URL-order iteration but aborts title-order iteration. | `zim/iter.go`, `zim/archive.go`, `zim/search.go` | `[x]` |
| 0.3 | **`iter.Seq2[Entry, error]` iterator variants** — Add strict-mode iterators so callers can observe parse errors instead of having them silently swallowed or aborting. | Required for any caller that needs reliable iteration (e.g. verifying all entries). | `zim/iter.go`, `zim/archive.go`, `zim/search.go` | `[x]` |

---

## P1 High

### Multi-Part ZIM Support (Phase 9 from PLAN.md)

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 1.1 | **`multiReader`** — Implement a `reader` that presents split ZIM files (`.zimaa`, `.zimab`, …) as a single contiguous `io.ReaderAt`. | Wikipedia full dumps exceed 90 GB and are distributed as split files. Without this, gozim cannot open the most important ZIM files. | New `zim/multi.go` | `[ ]` |
| 1.2 | **Auto-discovery of parts** — When `Open("foo.zimaa")` is called, detect and open `.zimab`, `.zimac`, etc. | Users should not need to enumerate parts manually. | `zim/archive.go` (`Open`/`OpenWithOptions`) | `[ ]` |
| 1.3 | **Update CLI tools** — All tools should accept `.zimaa` files transparently. | Feature completeness. | `cmd/*/main.go` | `[ ]` |
| 1.4 | **Multi-part tests** — Create or obtain a split test ZIM for CI. | Test coverage. | `testdata/`, new `zim/multi_test.go` | `[ ]` |

### Security & Robustness (continued)

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 1.5 | **MIME list: read actual size** — Currently reads a fixed 64 KB chunk regardless of actual size. Should read only until the double-null terminator or validate that the list fits within the URL pointer region. | Wastes memory on tiny files; could read past intended data on unusual layouts. | `zim/archive.go` (MIME parsing in `init`) | `[x]` |
| 1.6 | **Title listing validation** — When loading `X/listing/titleOrdered/v1`, validate that every `uint32` index is `< EntryCount`. | A corrupt listing could cause out-of-range panics during title iteration. | `zim/archive.go` (`loadTitleListing`) | `[x]` |

### Testing

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 1.7 | **Fuzz compressed cluster data** — Add `FuzzParseCluster` covering the full `parseCluster` path including decompression. Current `FuzzExtractBlobs` only fuzzes offset/blob extraction. | The decompression path is the primary attack surface for malformed files. | `zim/fuzz_test.go` | `[x]` |
| 1.8 | **Concurrent read stress tests** — Multiple goroutines calling `EntryByPath`, `ReadContent`, and `Verify` concurrently on the same `Archive`. | Validates thread-safety claims beyond the existing 8-goroutine test. | `zim/archive_test.go` or new `zim/concurrent_test.go` | `[ ]` |
| 1.9 | **zimserve integration tests with real ZIM** — Add coverage for info pages, cluster detail, browse pagination, random endpoint, and error paths. | Several handlers have no test coverage. | `cmd/zimserve/main_test.go` | `[ ]` |

### Documentation

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 1.10 | **Performance characteristics doc** — O(log N) lookups, O(N) for fold search, cache behavior, mmap vs pread trade-offs, memory usage patterns. | Users need this to make informed cache size and deployment decisions. | `zim/doc.go` or `docs/performance.md` | `[ ]` |
| 1.11 | **Security model doc** — What is validated during `Open()`, what is deferred, trust assumptions, decompression limits, checksum scope. | Necessary for production deployment decisions. | `docs/security.md` | `[ ]` |

---

## P2 Medium

### Performance

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 2.1 | **LRU cache: O(1) promotion** — Replace linear scan of `cacheOrder` slice with `container/list` doubly-linked list + map. Current O(N) scan is fine for default 16 but degrades at larger cache sizes. | Users running zimserve with `--cache 256+` hit this on every cache hit. | `zim/archive.go` (`readCluster`) | `[ ]` |
| 2.2 | **Transfer compression in zimserve** — Add gzip/brotli `Accept-Encoding` middleware. Text content (HTML, CSS, JS, JSON) benefits significantly. | Standard HTTP server feature; reduces bandwidth. | `cmd/zimserve/main.go` | `[ ]` |
| 2.3 | **HEAD request optimization** — `handleContent` reads the full body even for HEAD. Skip body read after setting headers. | Avoids unnecessary decompression for HEAD (used by monitoring, link checkers). | `cmd/zimserve/handlers.go` | `[ ]` |
| 2.4 | **HTTP Range request support (206)** — For large media blobs (video, audio), support `Range` headers via `http.ServeContent`. | Required for in-browser video/audio playback from ZIM files. | `cmd/zimserve/handlers.go` | `[ ]` |
| 2.5 | **`EntriesInCluster` performance** — Currently O(EntryCount) full scan. Build a cluster-to-entries index lazily for repeated lookups. | Info page cluster detail view triggers O(N) per cluster. | `zim/archive.go` (`EntriesInCluster`) | `[ ]` |
| 2.6 | **Metadata caching** — `Metadata()` re-reads from archive every call. Cache in a `sync.Once`-initialized map. | Metadata is read repeatedly (every page load, info pages). | `zim/archive.go` (`Metadata`) | `[ ]` |

### API Gaps

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 2.7 | **Batch entry lookup** — `EntryByPaths(paths []string)` that sorts and walks the URL pointer list once. | Resolving multiple links in a page without repeated binary searches. | `zim/archive.go` | `[ ]` |
| 2.8 | **`AllMetadata() map[string]string`** — Return all M-namespace entries in one call. | Avoids callers needing to know metadata key names. | `zim/archive.go` | `[ ]` |
| 2.9 | **Cluster-level statistics** — `ClusterStats() []ClusterMeta` for all clusters without decompression. | Useful for analysis tools; complements existing `ClusterMetaAt`. | `zim/archive.go` | `[ ]` |

### zimserve Features

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 2.10 | **Request rate limiting** — Token-bucket or `x/time/rate` limiter middleware. | Basic DoS protection for public-facing deployments. | `cmd/zimserve/main.go` | `[ ]` |
| 2.11 | **Health check endpoint** — `GET /_health` returning 200 OK with JSON (uptime, archive count, cache stats). | Required for load balancers, Kubernetes probes. | `cmd/zimserve/main.go` | `[ ]` |
| 2.12 | **JSON API for info endpoints** — `?format=json` on `/{slug}/-/info` and related pages. | Enables programmatic access to archive metadata. | `cmd/zimserve/info.go` | `[ ]` |
| 2.13 | **Configurable CORS** — `--cors-origin` flag for cross-origin API access. | Needed when zimserve backs a separate frontend. | `cmd/zimserve/main.go` | `[ ]` |

### CLI Tool Enhancements

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 2.14 | **`ziminfo --json`** — JSON output for scripting and pipeline integration. | Current text output is not machine-parseable. | `cmd/ziminfo/main.go` | `[ ]` |
| 2.15 | **`zimverify --parallel N`** — Parallel checksum verification of multiple files. | Large collections take too long one-at-a-time. | `cmd/zimverify/main.go` | `[ ]` |
| 2.16 | **`zimcat --progress`** — Progress indicator for large extractions. | UX improvement for multi-GB ZIM files. | `cmd/zimcat/main.go` | `[ ]` |

### Infrastructure

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 2.17 | **Dockerfile** — Multi-stage build producing a minimal image with zimserve. | Standard deployment artifact; enables Docker Hub publishing. | New `Dockerfile` | `[ ]` |
| 2.18 | **Docker Compose example** — Example `compose.yml` mounting a ZIM volume. | Lowers adoption barrier. | New `docker-compose.yml` | `[ ]` |
| 2.19 | **systemd service file** — Example unit file for running zimserve as a daemon. | Common deployment pattern for self-hosted instances. | New `contrib/zimserve.service` | `[ ]` |

---

## P3 Nice-to-have

### Performance (advanced)

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 3.1 | **Precomputed case-folded title index** — Optional secondary index for `EntriesByTitlePrefixFold` to avoid O(N) scan. | Only matters for interactive search in very large archives. | `zim/search.go` | `[ ]` |
| 3.2 | **HTTP/2 support** — Go's `net/http` supports HTTP/2 natively over TLS. | Multiplexing benefits for pages with many sub-resources. Requires TLS. | `cmd/zimserve/main.go` | `[ ]` |
| 3.3 | **Connection concurrency limits** — `x/net/netutil.LimitListener` or similar. | Defense in depth; usually handled by reverse proxy. | `cmd/zimserve/main.go` | `[ ]` |

### API (advanced)

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 3.4 | **ZIM Writer/Create API** — `zim.Writer` for creating ZIM files from content. | Would complete the library but is a major effort. | New `zim/writer.go` | `[ ]` |
| 3.5 | **Xapian full-text search** — Pure-Go Xapian index reader for embedded `X/fulltext/xapian`. | Extremely difficult without CGo. Document as out-of-scope. | `zim/search.go` | `[ ]` |
| 3.6 | **`zimsearch --regex`** — Regex title matching mode. | Power-user feature; requires full scan. | `cmd/zimsearch/main.go` | `[ ]` |

### zimserve (advanced)

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 3.7 | **HTTPS/TLS support** — `--tls-cert` / `--tls-key` flags. | Most deployments use a reverse proxy, but useful for simple setups. | `cmd/zimserve/main.go` | `[ ]` |
| 3.8 | **Authentication** — Basic auth or token-based access control via flags. | Niche use case; reverse proxy is the standard approach. | `cmd/zimserve/main.go` | `[ ]` |
| 3.9 | **Prometheus metrics** — `/metrics` endpoint with request counts, latencies, cache stats. | Useful for monitoring production deployments. | `cmd/zimserve/main.go` | `[ ]` |
| 3.10 | **OpenAPI/Swagger spec** — Formal API documentation for JSON endpoints. | Nice for API consumers; low priority given limited surface. | `docs/openapi.yaml` | `[ ]` |

### Documentation (extended)

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 3.11 | **Deployment guide** — Reverse proxy (nginx/caddy), systemd, Docker patterns. | Practical guide for self-hosting zimserve. | `docs/deployment.md` | `[ ]` |
| 3.12 | **Migration guide** — How to migrate from `akhenakh/gozim` or other Go ZIM libraries. | Reduces adoption friction. | `docs/migration.md` | `[ ]` |
| 3.13 | **Troubleshooting guide** — Common errors, mmap limits, file descriptor limits, corrupt ZIM diagnosis. | Reduces support burden. | `docs/troubleshooting.md` | `[ ]` |

### Infrastructure (extended)

| # | Item | Rationale | Files | Status |
|---|------|-----------|-------|--------|
| 3.14 | **Homebrew formula** — `brew install stazelabs/tap/gozim`. | Standard distribution channel for CLI tools. | Separate tap repo | `[ ]` |
| 3.15 | **zimserve benchmarks** — HTTP throughput benchmarks via `testing.B` + `httptest`. | Quantifies performance for comparison and regression detection. | `cmd/zimserve/bench_test.go` | `[ ]` |
| 3.16 | **32-bit platform documentation** — Document that ZIM files > 2 GB are unsupported on 32-bit (Go `int` is 32-bit, mmap disabled). | Prevents user confusion. Not a bug to fix. | `zim/doc.go` or README | `[ ]` |

---

## Suggested Implementation Order

1. **P0** (0.1–0.3) — Fix decompression bomb risk and iterator semantics
2. **P1 multi-part** (1.1–1.4) — Unlocks the most important real-world use case
3. **P1 validation** (1.5–1.6) — Harden parsing
4. **P1 testing** (1.7–1.9) — Establish confidence before adding features
5. **P1 docs** (1.10–1.11) — Ship security and performance docs with v1.0
6. **P2 performance** (2.1–2.6) — LRU fix, transfer compression, HEAD optimization
7. **P2 server** (2.10–2.13) — Rate limiting, health check, JSON API
8. **P2 CLI** (2.14–2.16) — JSON output, parallel verify
9. **P2 infra** (2.17–2.19) — Docker, systemd
10. **P3** — As time permits post-v1.0
