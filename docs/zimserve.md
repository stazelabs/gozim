# zimserve

HTTP server for browsing ZIM file content. Serves one or more ZIM files with a web interface, search, browsing, and a JSON API.

## Usage

```
zimserve [file.zim ...] [--dir <dir>] [flags]
```

ZIM files can be specified as positional arguments, via `--dir`, or both.

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--addr` | `-a` | `:8080` | Listen address (`host:port`) |
| `--cache` | `-c` | `64` | Cluster cache size per ZIM file |
| `--dir` | `-d` | | Directory of ZIM files to serve (repeatable) |
| `--recursive` | `-r` | `false` | Scan `--dir` directories recursively |

### Examples

```sh
# Serve a single ZIM file
zimserve wikipedia_en.zim

# Serve all ZIM files in a directory
zimserve --dir /data/zim

# Custom port, recursive scan, larger cache
zimserve -a :9090 -c 128 -r -d /data/zim -d /more/zims
```

## URL Slugs

Each ZIM file is assigned a URL slug derived from its filename:

- The `.zim` extension is stripped
- Trailing date segments (underscore-separated numeric parts) are removed
- Example: `wikipedia_en_all_2024-01.zim` becomes `wikipedia_en_all`

Duplicate slugs are disambiguated with a numeric suffix (`_2`, `_3`, etc.).

## Routes

### Library Index

```
GET /
```

HTML page listing all loaded ZIM files in a sortable table. Columns: Title, File, Date, Entries. Includes an instant search box that queries the JSON search API with 200ms debounce. When multiple ZIMs are loaded, a dropdown lets you scope search to a specific archive or search all.

### ZIM Root / Main Page

```
GET /{slug}/
```

Redirects (302) to the ZIM's main page. Returns 404 if the archive has no main entry.

### Content

```
GET /{slug}/{path...}
```

Serves content from the ZIM's `C` (content) namespace. The `{path}` maps to `C/{path}` inside the archive.

**Behavior:**
- ZIM-internal redirects are followed and returned as HTTP 302 redirects
- `Content-Type` is set from the entry's MIME type
- `Content-Length` is set for all responses
- Content is streamed via `io.Copy` from the cluster cache

**Caching headers** (ZIM content is immutable):
- `Cache-Control: public, max-age=31536000, immutable`
- `ETag` derived from `MD5(archive_uuid + entry_full_path)`
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

Returns a JSON array of search results using title prefix matching on the `C` namespace. The global `/_search` endpoint searches across all loaded ZIMs.

Search tries multiple capitalization variants of the query (original, uppercased first letter, lowercased first letter) using the sorted title index for O(log N + k) performance.

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

**HTML format** (default): Renders a search page with a form and clickable results list.

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

HTML page with an A-Z + # letter bar. Selecting a letter lists `C`-namespace entries whose title starts with that letter. `#` shows entries starting with non-letter characters.

Paginated with Previous/Next links.

### Random Article

```
GET /{slug}/-/random
```

Redirects (302) to a random article from the `C` namespace. Uses `crypto/rand` for unbiased selection. Returns 404 if the namespace is empty.

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

- Positional arguments are **hard failures** -- if a file can't be opened, zimserve exits with an error
- Files discovered via `--dir` are **soft failures** -- invalid files are logged and skipped
- At least one valid ZIM must be loaded, otherwise zimserve exits with an error
- Recursive directory scanning (`-r`) does not follow symlinked directories to avoid cycles
- Paths are deduplicated by absolute path and sorted for deterministic slug assignment

## Metadata

zimserve reads these ZIM metadata keys (all optional) for the library index:

| Key | Used for |
|-----|----------|
| `Title` | Display name (falls back to slug) |
| `Language` | Language tag shown in file column |
| `Description` | Subtitle under title |
| `Date` | Date column |
| `Creator` | Shown in file column subtitle |
| `Flavour` | Shown in file column subtitle |
