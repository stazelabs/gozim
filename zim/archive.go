package zim

import (
	"encoding/binary"
	"fmt"
	"iter"
	"sync"
)

const defaultCacheSize = 16

// Archive represents an opened ZIM file.
type Archive struct {
	r         reader
	hdr       header
	mimeTypes []string

	clusterMu    sync.Mutex
	clusterCache map[uint32]*cluster
	cacheSize    int
	cacheOrder   []uint32 // simple LRU tracking
}

type options struct {
	cacheSize int
	useMmap   bool
}

// Option configures how a ZIM archive is opened.
type Option func(*options)

// WithCacheSize sets the number of decompressed clusters to cache. Default: 16.
func WithCacheSize(n int) Option {
	return func(o *options) { o.cacheSize = n }
}

// WithMmap controls whether memory mapping is used. Default: true on 64-bit.
func WithMmap(enabled bool) Option {
	return func(o *options) { o.useMmap = enabled }
}

// Open opens a ZIM file for reading.
func Open(path string) (*Archive, error) {
	return OpenWithOptions(path)
}

// OpenWithOptions opens a ZIM file with the given options.
func OpenWithOptions(path string, opts ...Option) (*Archive, error) {
	o := options{
		cacheSize: defaultCacheSize,
		useMmap:   defaultUseMmap(),
	}
	for _, opt := range opts {
		opt(&o)
	}

	r, err := openReader(path, o.useMmap)
	if err != nil {
		return nil, fmt.Errorf("zim: open %s: %w", path, err)
	}

	a := &Archive{
		r:            r,
		cacheSize:    o.cacheSize,
		clusterCache: make(map[uint32]*cluster),
	}

	if err := a.init(); err != nil {
		r.Close()
		return nil, err
	}

	return a, nil
}

// init reads the header and MIME type list.
func (a *Archive) init() error {
	// Read header
	buf := make([]byte, headerSize)
	if _, err := a.r.ReadAt(buf, 0); err != nil {
		return fmt.Errorf("zim: read header: %w", err)
	}

	var err error
	a.hdr, err = parseHeader(buf)
	if err != nil {
		return err
	}

	// Read MIME type list
	// The MIME list starts at MIMEListPos and ends at the first double-null.
	// We read a reasonable chunk and parse it.
	mimeStart := int64(a.hdr.MIMEListPos)
	// Read up to 64KB for MIME list (more than enough)
	maxMIME := int64(65536)
	end := mimeStart + maxMIME
	if end > a.r.Size() {
		end = a.r.Size()
	}
	mimeBuf := make([]byte, end-mimeStart)
	if _, err := a.r.ReadAt(mimeBuf, mimeStart); err != nil {
		return fmt.Errorf("zim: read MIME list: %w", err)
	}
	a.mimeTypes = parseMIMEList(mimeBuf)

	return nil
}

// Close closes the archive and releases resources.
func (a *Archive) Close() error {
	a.clusterMu.Lock()
	a.clusterCache = nil
	a.cacheOrder = nil
	a.clusterMu.Unlock()
	return a.r.Close()
}

// UUID returns the archive's unique identifier.
func (a *Archive) UUID() [16]byte { return a.hdr.UUID }

// EntryCount returns the total number of entries.
func (a *Archive) EntryCount() uint32 { return a.hdr.EntryCount }

// ClusterCount returns the total number of clusters.
func (a *Archive) ClusterCount() uint32 { return a.hdr.ClusterCount }

// MIMETypes returns the list of MIME types in the archive.
func (a *Archive) MIMETypes() []string {
	result := make([]string, len(a.mimeTypes))
	copy(result, a.mimeTypes)
	return result
}

// HasMainEntry returns true if the archive has a designated main page.
func (a *Archive) HasMainEntry() bool { return a.hdr.MainPage != noMainPage }

// MainEntry returns the main page entry.
func (a *Archive) MainEntry() (Entry, error) {
	if !a.HasMainEntry() {
		return Entry{}, ErrNotFound
	}
	return a.EntryByIndex(a.hdr.MainPage)
}

// EntryByIndex returns the entry at the given index in the URL-ordered list.
func (a *Archive) EntryByIndex(idx uint32) (Entry, error) {
	if idx >= a.hdr.EntryCount {
		return Entry{}, ErrNotFound
	}

	// Read the URL pointer for this index (stack-allocated buffer)
	ptrOffset := int64(a.hdr.URLPtrPos) + int64(idx)*8
	var ptrBuf [8]byte
	if _, err := a.r.ReadAt(ptrBuf[:], ptrOffset); err != nil {
		return Entry{}, fmt.Errorf("zim: read URL pointer at index %d: %w", idx, err)
	}
	entryOffset := int64(binary.LittleEndian.Uint64(ptrBuf[:]))

	return a.readEntryAt(entryOffset, idx)
}

// readEntryAt reads a directory entry from the given file offset.
func (a *Archive) readEntryAt(offset int64, index uint32) (Entry, error) {
	// Read enough data for the entry. Directory entries are variable-length
	// but typically under 512 bytes. Use a stack-allocated buffer for common
	// cases and fall back to heap for entries near end of file.
	var stackBuf [512]byte
	n := int64(len(stackBuf))
	if offset+n > a.r.Size() {
		n = a.r.Size() - offset
	}
	buf := stackBuf[:n]
	if _, err := a.r.ReadAt(buf, offset); err != nil {
		return Entry{}, fmt.Errorf("zim: read directory entry at offset %d: %w", offset, err)
	}

	entry, _, err := parseDirectoryEntry(buf, a, index)
	if err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// EntryByPath looks up an entry by its full path (e.g., "C/Main_Page").
// Uses binary search over the URL pointer list.
func (a *Archive) EntryByPath(path string) (Entry, error) {
	lo, hi := uint32(0), a.hdr.EntryCount
	for lo < hi {
		mid := lo + (hi-lo)/2
		entry, err := a.EntryByIndex(mid)
		if err != nil {
			return Entry{}, err
		}
		entryPath := entry.FullPath()
		if entryPath == path {
			return entry, nil
		}
		if entryPath < path {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return Entry{}, ErrNotFound
}

// EntryByTitle looks up an entry by namespace and title using binary search
// over the title pointer list. The list is sorted by (namespace, title).
func (a *Archive) EntryByTitle(ns byte, title string) (Entry, error) {
	lo, hi := uint32(0), a.hdr.EntryCount
	for lo < hi {
		mid := lo + (hi-lo)/2
		entry, err := a.entryByTitleIndex(mid)
		if err != nil {
			return Entry{}, err
		}
		cmp := compareTitleKey(entry.Namespace(), entry.Title(), ns, title)
		if cmp == 0 {
			return entry, nil
		}
		if cmp < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return Entry{}, ErrNotFound
}

// compareTitleKey compares two (namespace, title) pairs.
// Returns -1, 0, or 1.
func compareTitleKey(ns1 byte, title1 string, ns2 byte, title2 string) int {
	if ns1 < ns2 {
		return -1
	}
	if ns1 > ns2 {
		return 1
	}
	if title1 < title2 {
		return -1
	}
	if title1 > title2 {
		return 1
	}
	return 0
}

// entryByTitleIndex reads the entry at position idx in the title pointer list.
func (a *Archive) entryByTitleIndex(idx uint32) (Entry, error) {
	ptrOffset := int64(a.hdr.TitlePtrPos) + int64(idx)*4
	var ptrBuf [4]byte
	if _, err := a.r.ReadAt(ptrBuf[:], ptrOffset); err != nil {
		return Entry{}, fmt.Errorf("zim: read title pointer at index %d: %w", idx, err)
	}
	entryIdx := binary.LittleEndian.Uint32(ptrBuf[:])
	return a.EntryByIndex(entryIdx)
}

// EntriesByTitle returns an iterator over all entries sorted by title.
func (a *Archive) EntriesByTitle() iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		for i := uint32(0); i < a.hdr.EntryCount; i++ {
			e, err := a.entryByTitleIndex(i)
			if err != nil {
				return
			}
			if !yield(e) {
				return
			}
		}
	}
}

// Metadata returns the value of a metadata entry (M namespace) by key.
// Returns ErrNotFound if the key doesn't exist.
func (a *Archive) Metadata(key string) (string, error) {
	entry, err := a.EntryByPath("M/" + key)
	if err != nil {
		return "", err
	}
	data, err := entry.ReadContent()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// readCluster reads, decompresses, and caches a cluster by number.
func (a *Archive) readCluster(clusterNum uint32) (*cluster, error) {
	a.clusterMu.Lock()
	defer a.clusterMu.Unlock()

	// Check cache
	if c, ok := a.clusterCache[clusterNum]; ok {
		return c, nil
	}

	if clusterNum >= a.hdr.ClusterCount {
		return nil, fmt.Errorf("zim: cluster %d out of range (max %d)", clusterNum, a.hdr.ClusterCount)
	}

	// Read cluster pointer
	ptrOffset := int64(a.hdr.ClusterPtrPos) + int64(clusterNum)*8
	ptrBuf := make([]byte, 16) // read current and next pointer
	if _, err := a.r.ReadAt(ptrBuf, ptrOffset); err != nil {
		return nil, fmt.Errorf("zim: read cluster pointer %d: %w", clusterNum, err)
	}

	clusterOffset := int64(binary.LittleEndian.Uint64(ptrBuf[0:8]))

	// Determine cluster end
	var clusterEnd int64
	if clusterNum+1 < a.hdr.ClusterCount {
		clusterEnd = int64(binary.LittleEndian.Uint64(ptrBuf[8:16]))
	} else {
		clusterEnd = int64(a.hdr.ChecksumPos)
	}

	clusterSize := clusterEnd - clusterOffset
	if clusterSize <= 0 {
		return nil, fmt.Errorf("zim: invalid cluster size %d", clusterSize)
	}

	// Read cluster data
	clusterData := make([]byte, clusterSize)
	if _, err := a.r.ReadAt(clusterData, clusterOffset); err != nil {
		return nil, fmt.Errorf("zim: read cluster %d: %w", clusterNum, err)
	}

	// Parse cluster
	c, err := parseCluster(clusterData)
	if err != nil {
		return nil, fmt.Errorf("zim: parse cluster %d: %w", clusterNum, err)
	}

	// Cache with simple LRU eviction
	if a.clusterCache != nil {
		if len(a.cacheOrder) >= a.cacheSize {
			evict := a.cacheOrder[0]
			a.cacheOrder = a.cacheOrder[1:]
			delete(a.clusterCache, evict)
		}
		a.clusterCache[clusterNum] = c
		a.cacheOrder = append(a.cacheOrder, clusterNum)
	}

	return c, nil
}
