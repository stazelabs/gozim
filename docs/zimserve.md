# zimserve

HTTP server for browsing ZIM file content. Serves one or more ZIM files with a web interface, search, browsing, diagnostic introspection tools, and a JSON API.

## Usage

```
zimserve [file.zim ...] [--dir <dir>] [flags]
```

ZIM files can be specified as positional arguments, via `--dir`, or both.

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--addr` | `-a` | `:8080` | Listen address (`host:port`) |
| `--cache` | `-c` | `64` | Decompressed cluster LRU cache size per ZIM file (see [Cluster Cache](#cluster-cache)) |
| `--dir` | `-d` | | Directory of ZIM files to serve (repeatable) |
| `--recursive` | `-r` | `false` | Scan `--dir` directories recursively |
| `--no-info` | | `false` | Disable all `/-/info` pages and hide the info icon in the library index |

### Examples

```sh
# Serve a single ZIM file
zimserve wikipedia_en.zim

# Serve all ZIM files in a directory
zimserve --dir /data/zim

# Custom port, recursive scan, larger cache
zimserve -a :9090 -c 128 -r -d /data/zim -d /more/zims

# Public-facing server: disable diagnostic info pages
zimserve --no-info --dir /data/zim
```

## URL Slugs

Each ZIM file is assigned a URL slug derived from its filename:

- The `.zim` extension is stripped
- Trailing underscore-separated segments that start with a digit are removed
- Example: `wikipedia_en_all_2024-01.zim` becomes `wikipedia_en_all`

Duplicate slugs are disambiguated with a numeric suffix (`_2`, `_3`, etc.). Slugs are assigned in sorted path order for deterministic results.

## Routes

### Library Index

```
GET /
```

HTML page listing all loaded ZIM files in a sortable table. Columns: Title, File, Date, Entries. Each row also has:

- **Action links** under the title: Browse, Search, Random
- **Info link** (an "i" icon) linking to the ZIM's diagnostic info page

The page includes:

- **Instant search box** — queries the JSON search API with 200ms debounce, displaying results in a dropdown overlay. A loading spinner appears during requests.
- **ZIM dropdown** — when multiple ZIMs are loaded, a dropdown scopes search to a specific archive or "All". When only one ZIM is loaded, the dropdown is hidden.
- **Random button** — navigates to a random article. Respects the ZIM dropdown selection, using `/_random` (all ZIMs) or `/{slug}/-/random` (specific ZIM).

The table is sortable by clicking column headers (Title, File, Date, Entries). An arrow indicator shows current sort column and direction.

### Global Favicon

```
GET /favicon.ico
GET /_favicon.svg
```

Both routes serve the same embedded SVG: a 📚 book emoji. `/favicon.ico` is used by browsers for the tab icon; `/_favicon.svg` is a stable URL referenced by the `<link rel="icon">` tag injected into every HTML page served by zimserve.

Includes `Cache-Control: public, max-age=31536000, immutable`.

### Documentation

```
GET /_docs
```

Renders `zimserve.md` (this document) as an HTML page with table support. Content is pre-rendered at startup from the embedded Markdown source.

### ZIM Root / Main Page

```
GET /{slug}/
```

Redirects (302) to the ZIM's main page entry. Returns 404 if the archive has no main entry.

### Content

```
GET /{slug}/{path...}
```

Serves content from the ZIM's `C` (content) namespace. The `{path}` maps to `C/{path}` inside the archive.

**Behavior:**

- ZIM-internal redirects are followed and returned as HTTP 302 redirects
- `Content-Type` is set from the entry's MIME type. For `text/*` types that lack a charset, `; charset=utf-8` is appended automatically (ZIM MIME types typically omit charset and browsers may guess wrong without it)
- `Content-Length` is set for all responses

**HTML content injection:** For `text/html` entries, the full body is read into memory so that a navigation header bar and footer bar can be injected (see [Navigation UI](#navigation-ui) below). Non-HTML content is streamed directly via `io.Copy`.

**Caching headers** (ZIM content is immutable):

- `Cache-Control: public, max-age=31536000, immutable`
- `ETag` derived from `MD5(archive_uuid_hex + entry_full_path)`
- Supports `If-None-Match` conditional requests (returns 304)

### Favicon

```
GET /{slug}/favicon.ico
```

Serves the ZIM's `M/Illustration_48x48@1` metadata entry as `image/png`. Returns 404 if the illustration is not present.

Includes `Cache-Control: public, max-age=31536000, immutable`.

### Title Search (JSON API)

```
GET /{slug}/_search?q={query}
GET /_search?q={query}
```

Returns a JSON array of search results. The per-slug endpoint searches a single archive; the global `/_search` endpoint searches across all loaded ZIMs (iterating in slug order, stopping once the limit is reached).

**How search works:**

1. The query is expanded into capitalization variants (original, upper-cased first letter, lower-cased first letter) to cover common cases without a full case-insensitive scan
2. Each variant does a binary search on the sorted title index for O(log N + k) performance
3. Only `text/html` entries are returned — JS, CSS, images, and other assets in the C namespace are filtered out
4. Duplicate paths (from overlapping prefix variants) are deduplicated

**Response format:**

```json
[
  {"path": "/{slug}/Article_Title", "title": "Article Title"},
  ...
]
```

Returns `[]` for empty queries. Limited to 20 results.

### Title Search (HTML Page)

```
GET /{slug}/-/search?q={query}&limit={n}&format={html|json}
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `q` | | Search query (title prefix) |
| `limit` | `25` | Max results (1-100) |
| `format` | `html` | Response format: `html` or `json` |

**HTML format** (default): Renders a search page with a form and clickable results list. Shows result count, or "No results found" for zero matches. Includes navigation links to Library, Main page, Browse, and Random article.

**JSON format** (`format=json`): Same response format as the `/_search` endpoint.

### Browse by Letter

```
GET /{slug}/-/browse?letter={A-Z|#}&offset={n}&limit={n}
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `letter` | | Letter to browse: `A`-`Z` or `#` (non-alphabetic) |
| `offset` | `0` | Pagination offset |
| `limit` | `50` | Results per page (1-200) |

HTML page with an A-Z + `#` letter bar. Letters with zero matching entries appear greyed out (not clickable). The currently selected letter is highlighted.

- **A-Z letters**: Entries are collected for both upper and lower case (e.g. "A" and "a") and merged
- **`#`**: Shows entries whose title starts with a non-letter character (numbers, symbols, etc.). This requires a full scan of all C-namespace entries

The page shows a total article count and per-letter count. Paginated with Previous/Next links showing current position (e.g. "1-50 of 1,234").

### Random Article

```
GET /{slug}/-/random
GET /_random
```

Redirects (302) to a random article.

**Per-slug** (`/{slug}/-/random`): Picks a random `text/html` entry from the C namespace. Retries up to 50 times to skip non-article entries (JS, CSS, images). Returns 404 if no HTML article is found.

**Global** (`/_random`): Picks a random ZIM, then picks a random article from it using the same retry logic.

Both use Go's `math/rand/v2` package, which auto-seeds from the runtime's cryptographic source.

### ZIM Info / Diagnostics

```
GET /{slug}/-/info
GET /{slug}/-/info/ns
GET /{slug}/-/info/mime
GET /{slug}/-/info/entry
GET /{slug}/-/info/cluster
```

All `/-/info` routes are disabled when `--no-info` is set. The info icon column in the library index is also hidden in that case.

`/{slug}/-/info` is a diagnostic overview page for a single ZIM file — useful for understanding the internal structure of a ZIM file without external tooling.

**Format section** — ZIM file metadata:

| Field | Description |
|-------|-------------|
| Filename | Original filename on disk |
| UUID | Archive UUID formatted as `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` |
| ZIM Version | Major.minor version from the header |
| Entry Count | Total number of directory entries |
| Cluster Count | Links to the cluster list page |
| Cluster Cache | Runtime cache stats: slots used / capacity, hit rate (hits / misses), bytes held |
| Has Main Entry | Yes/No badge; if yes, links to the main page |
| Full-text Index | Yes/No badge with format string if present |
| Title Index | Yes/No badge with format string if present |

**Metadata section** — Reads and displays these ZIM metadata keys (if present): Title, Creator, Publisher, Date, Description, LongDescription, Language, License, Tags, Relation, Flavour, Source, Counter, Scraper. Long values (>200 chars) get a scrollable container.

**Namespaces section** — Table of populated namespaces with entry counts. Each count links to the namespace browser. Known namespaces are labelled:

| Namespace | Description |
|-----------|-------------|
| `C` | Content |
| `M` | Metadata |
| `X` | Indexes / Special |
| `W` | Well-known |
| `V` | User content (deprecated) |
| `A` | Articles (legacy ZIM v5) |
| `I` | Images (legacy ZIM v5) |
| `-` | Misc (legacy ZIM v5) |

**Content Types section** — MIME type breakdown for the C namespace, sorted by count (descending). Each count links to the MIME type browser. Redirects are shown separately at the bottom.

**Registered MIME Types section** — The MIME type list from the ZIM header, with index numbers. Each type links to the namespace browser filtered to that type.

### Namespace Browser

```
GET /{slug}/-/info/ns?ns={char}&type={mime|redirect}&offset={n}&limit={n}
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ns` | (required) | Single-character namespace (e.g. `C`, `M`, `X`) |
| `type` | | Filter: a MIME type string or `redirect` |
| `offset` | `0` | Pagination offset |
| `limit` | `100` | Results per page (1-500) |

Lists entries in a namespace with columns: Path, Title, Type. Each entry links to its detail page. Content entries (namespace `C`, non-redirect) also have a direct link to view the content.

A **type filter dropdown** lets you narrow results to a specific MIME type or redirects. The dropdown is populated from the ZIM's registered MIME type list.

Without a type filter, the total count and pagination use efficient binary search (not a full scan).

### MIME Type Browser

```
GET /{slug}/-/info/mime?type={mime|redirect}&offset={n}&limit={n}
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `type` | (required) | MIME type string or `redirect` |
| `offset` | `0` | Pagination offset |
| `limit` | `100` | Results per page (1-500) |

Lists C-namespace entries matching a specific MIME type (or all redirects). Shows Path and Title columns. For `text/html` and `image/*` types, paths link directly to viewable content alongside the entry detail link.

### Entry Detail

```
GET /{slug}/-/info/entry?idx={n}
```

Detail page for a single directory entry by index number. Shows:

| Field | Description |
|-------|-------------|
| Index | Entry index in the archive |
| Full Path | Namespace-prefixed path (e.g. `C/Article_Name`) |
| Namespace | Single character |
| Path | Path within namespace |
| Title | Display title |
| Is Redirect | true/false |

**For redirects:** Shows the immediate redirect target (with link) and the final resolved entry (with link), including their index numbers.

**For content entries:** Shows MIME type and decompressed content size (human-readable with exact byte count).

**Action links:** "View content" link for C-namespace entries (follows redirects automatically for redirect entries).

**Navigation:** Previous/Next entry links for sequential browsing through the entry list, plus breadcrumb links back to the namespace browser and info page.

### Cluster Browser

```
GET /{slug}/-/info/cluster?n={n}&offset={n}&limit={n}
```

**List view** (no `n` parameter): Paginated table of all clusters showing:
- Cluster number (links to detail view)
- File offset (hex)
- Compressed size (human-readable)
- Compression type (e.g. `zstd`, `xz`, `none`)
- Whether extended (64-bit) offsets are used

Default: 100 per page, max 500.

**Detail view** (`n={cluster_number}`): Full diagnostic view of a single cluster:

- **Cluster info table**: cluster number, file offset (hex + decimal), compressed size, compression type, extended offsets flag
- **Blob table** (requires decompression): Lists each blob with its decompressed size. Shows total blob count and total decompressed size. If decompression fails, an error message is shown instead.
- **Entries table**: Directory entries stored in this cluster, with links to view content and entry details
- **Navigation**: Previous/Next cluster links for sequential browsing

## Navigation UI

zimserve injects UI elements into served pages to aid navigation.

### Header Bar

A sticky navigation bar is injected after the opening `<body>` tag of every `text/html` content page. It contains:

- Library link (book icon) — returns to the library index
- ZIM title link — returns to the ZIM's main page
- Search form — submits to `/{slug}/-/search`
- Random button — links to `/{slug}/-/random`
- A-Z letter bar — links to `/{slug}/-/browse?letter=X`. Letters with no matching entries appear greyed out

The bar uses `position: sticky` with `z-index: 999999` so it stays visible while scrolling.

### Footer Bar

A fixed footer bar is injected before the closing `</body>` tag of every HTML page (both content pages and zimserve's own UI pages). It contains:

- A link to the gozim GitHub repository with a GitHub icon
- A link to the Apache 2.0 license

The bar uses `position: fixed` at the bottom with `z-index: 999998`. A `padding-bottom` rule is added to `body` to prevent content from being obscured.

### Injection Behavior

- Tag matching is case-insensitive (`<body>`, `<BODY>`, `<Body>` all work)
- If no `<body>` tag is found, the header bar is prepended to the content
- If no `</body>` tag is found, the footer bar is appended to the content
- Both bars are fully self-contained (inline CSS, no external dependencies)

## HTTP Details

### Allowed Methods

Only `GET` and `HEAD` are accepted. All other methods return `405 Method Not Allowed` with an `Allow: GET, HEAD` header.

### Security Headers

Applied to every response:

| Header | Value |
|--------|-------|
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `SAMEORIGIN` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |

The library index page also sets:

```
Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline';
  script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'
```

### Server Timeouts

| Timeout | Value |
|---------|-------|
| Read | 30s |
| Write | 60s |
| Idle | 120s |

### Graceful Shutdown

On `SIGINT` or `SIGTERM`, the server:

1. Stops accepting new connections
2. Drains in-flight requests (10-second timeout)
3. Closes all ZIM archives cleanly
4. Exits

## ZIM Loading

- Positional arguments are **hard failures** — if a file can't be opened, zimserve exits with an error
- Files discovered via `--dir` are **soft failures** — invalid files are logged and skipped
- At least one valid ZIM must be loaded, otherwise zimserve exits with an error
- Recursive directory scanning (`-r`) does not follow symlinked directories to avoid cycles
- Paths are deduplicated by absolute path and sorted for deterministic slug assignment
- Library is sorted alphabetically by title (case-insensitive) for display

On startup, each loaded ZIM is logged with its slug, title, and entry count:

```
loaded: /wikipedia_en_all/ — Wikipedia (English) (1234567 entries)
```

## Cluster Cache

ZIM content entries are stored in compressed clusters (XZ or Zstd). Each cluster must be decompressed in full to read any single entry inside it. The `--cache` flag (`-c`) sets the size of a per-archive LRU cache of decompressed clusters. The default is 64.

**Memory cost:** The cache holds decompressed data. Per-cluster decompressed sizes vary significantly by ZIM:

| ZIM type | Typical decompressed cluster size | Cache=64 cost |
|----------|-----------------------------------|---------------|
| Small / test ZIMs | 10–200 KB | < 13 MiB |
| Medium wikis (Wiktionary, etc.) | 0.5–2 MiB | 32–128 MiB |
| Wikipedia full (en_all) | 1–4 MiB | 64–256 MiB |

**Sizing guidance:** A typical article page access touches 1 cluster for the HTML body plus several more for CSS, JS, and images. A cache of 32–64 is usually adequate for single-user browsing. For concurrent users or large ZIMs with heavy image use, 128–256 may be appropriate.

**Runtime diagnostics:** The `/-/info` page for each ZIM shows a **Cluster Cache** row with live stats:

```
2 / 64 slots used  ·  83.3% hit rate (10 hits, 2 misses)  ·  4.2 MiB (4404224 bytes) cached
```

- **Slots used / capacity**: how much of the cache is filled. If slots used < capacity consistently, the cache is larger than needed.
- **Hit rate**: fraction of cluster reads served from cache. A low hit rate with full slots indicates the working set exceeds the cache size; increase `--cache`.
- **Bytes cached**: actual decompressed memory held. Use this to estimate per-ZIM RAM overhead.

Counters reset when the server restarts. Refresh the info page to see updated stats.

## Metadata

zimserve reads these ZIM metadata keys (all optional) for the library index:

| Key | Used for |
|-----|----------|
| `Title` | Display name (falls back to slug if absent) |
| `Language` | Language tag shown in brackets in the file column |
| `Description` | Subtitle under title in the library index |
| `Date` | Date column in the library index |
| `Creator` | Shown in file column subtitle |
| `Flavour` | Shown in file column subtitle (joined with Creator by " · ") |

The info page reads additional metadata keys: Publisher, LongDescription, License, Tags, Relation, Source, Counter, Scraper.
