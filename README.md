# gozim

A native Go library for reading [ZIM files](https://wiki.openzim.org/wiki/ZIM_file_format) — the open archive format used by [Kiwix](https://kiwix.org) for offline access to Wikipedia and other web content.

Pure Go. No CGo dependencies. Supports ZIM format versions 5 and 6.

## Features

- Open and read ZIM archives with a clean, idiomatic Go API
- Binary search for entries by path or title
- Iterate entries by path order, title order, or namespace
- Automatic decompression of XZ (LZMA) and Zstandard clusters
- Memory-mapped I/O on 64-bit systems with pread fallback
- LRU caching of decompressed clusters
- Redirect chain resolution
- MD5 checksum verification
- Title prefix search with O(log N) binary search bounds
- CLI tools: `ziminfo`, `zimcat`, `zimserve`, `zimsearch`, and `zimverify`
- HTTP server for browsing ZIM content in a web browser

## Install

```bash
go get github.com/stazelabs/gozim
```

## Usage

```go
package main

import (
    "fmt"
    "log"

    "github.com/stazelabs/gozim/zim"
)

func main() {
    a, err := zim.Open("wikipedia.zim")
    if err != nil {
        log.Fatal(err)
    }
    defer a.Close()

    // Read metadata
    title, _ := a.Metadata("Title")
    fmt.Println("Archive:", title)
    fmt.Println("Entries:", a.EntryCount())

    // Look up an entry by path
    entry, err := a.EntryByPath("C/Main_Page")
    if err != nil {
        log.Fatal(err)
    }

    // Read content (resolves redirects automatically)
    data, err := entry.ReadContent()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Content-Type: %s\n", entry.MIMEType())
    fmt.Printf("Size: %d bytes\n", len(data))

    // Iterate all content entries
    for e := range a.EntriesByNamespace('C') {
        fmt.Println(e.Path())
    }
}
```

## CLI Tools

### ziminfo

Dump metadata and structure of a ZIM file:

```bash
go run ./cmd/ziminfo wikipedia.zim
```

### zimcat

Extract content from a ZIM file:

```bash
# Extract an article to stdout
go run ./cmd/zimcat wikipedia.zim C/Main_Page

# List all entries
go run ./cmd/zimcat -l wikipedia.zim

# Show metadata
go run ./cmd/zimcat -m wikipedia.zim
```

### zimsearch

Search entries in a ZIM file by title prefix:

```bash
# Search for titles starting with "France" in the C namespace (default)
go run ./cmd/zimsearch wikipedia.zim France

# Case-insensitive search (full linear scan, slower)
go run ./cmd/zimsearch -i wikipedia.zim france

# Limit results and search a different namespace
go run ./cmd/zimsearch -n M -l 5 wikipedia.zim ""
```

### zimverify

Verify ZIM file integrity via MD5 checksum:

```bash
go run ./cmd/zimverify wikipedia.zim
```

### zimserve

Serve ZIM files over HTTP (kiwix-serve alternative):

```bash
# Serve a single ZIM file
go run ./cmd/zimserve wikipedia.zim

# Serve multiple ZIMs with custom port
go run ./cmd/zimserve -a :9090 wikipedia.zim wikivoyage.zim

# With larger cluster cache for better performance
go run ./cmd/zimserve -c 128 wikipedia.zim
```

Browse to `http://localhost:8080` to view the content. With multiple ZIM files, a library index page lists all available archives.

## API Overview

### Archive

```go
zim.Open(path) (*Archive, error)
zim.OpenWithOptions(path, ...Option) (*Archive, error)

archive.EntryByPath("C/Main_Page") (Entry, error)
archive.EntryByTitle('C', "Main Page") (Entry, error)
archive.EntryByIndex(0) (Entry, error)
archive.MainEntry() (Entry, error)
archive.Metadata("Title") (string, error)
archive.Entries() iter.Seq[Entry]
archive.EntriesByTitle() iter.Seq[Entry]
archive.EntriesByNamespace('C') iter.Seq[Entry]
archive.EntriesByTitlePrefix('C', "Fra") iter.Seq[Entry]   // O(log N + k)
archive.EntriesByTitlePrefixFold('C', "fra") iter.Seq[Entry] // O(N), case-insensitive
archive.HasFulltextIndex() bool
archive.HasTitleIndex() bool
archive.Verify() error
```

### Entry

```go
entry.Path() string
entry.Title() string
entry.FullPath() string           // "C/Main_Page"
entry.Namespace() byte
entry.IsRedirect() bool
entry.MIMEType() string
entry.ReadContent() ([]byte, error)  // resolves redirects
entry.Resolve() (Entry, error)       // follow redirect chain
entry.Item() (Item, error)           // content access
```

### Options

```go
zim.WithMmap(false)     // disable memory mapping
zim.WithCacheSize(32)   // cluster cache size (default: 16)
```

## Development

```bash
make test        # run tests
make test-race   # run with race detector
make bench       # run benchmarks
make testdata    # download test ZIM files
```

## License

Apache 2.0 — see [LICENSE](LICENSE) for details.
