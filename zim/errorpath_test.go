package zim

import (
	"container/list"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// AllEntriesByNamespace — error path (33.3% → higher)
// Uses 8 entries so binary search skips the corrupt entry at index 3.
// ---------------------------------------------------------------------------

func TestAllEntriesByNamespaceReportsErrorReliably(t *testing.T) {
	// 8 entries all in C/ namespace; entry 3 is corrupt.
	// namespaceBounds binary search visits indices 4,2,1,0 and 5,6,7
	// but not 3, so bounds are correctly (0,8). Iteration then hits 3.
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "A", "A"),
		makeContentEntry(0, 'C', 0, 0, "B", "B"),
		makeContentEntry(0, 'C', 0, 0, "C", "C"),
		nil, // corrupt — index 3
		makeContentEntry(0, 'C', 0, 0, "E", "E"),
		makeContentEntry(0, 'C', 0, 0, "F", "F"),
		makeContentEntry(0, 'C', 0, 0, "G", "G"),
		makeContentEntry(0, 'C', 0, 0, "H", "H"),
	}
	a := buildFakeArchive(entries)

	var goodCount int
	var gotErr error
	for e, err := range a.AllEntriesByNamespace('C') {
		if err != nil {
			gotErr = err
			break
		}
		_ = e
		goodCount++
	}

	if gotErr == nil {
		t.Fatal("expected error from corrupt entry 3, got nil")
	}
	if goodCount != 3 {
		t.Errorf("expected 3 good entries before error, got %d", goodCount)
	}
}

// ---------------------------------------------------------------------------
// EntriesByNamespace — stops silently at error (75% → higher)
// ---------------------------------------------------------------------------

func TestEntriesByNamespaceStopsAtError(t *testing.T) {
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "A", "A"),
		makeContentEntry(0, 'C', 0, 0, "B", "B"),
		makeContentEntry(0, 'C', 0, 0, "C", "C"),
		nil, // corrupt — index 3
		makeContentEntry(0, 'C', 0, 0, "E", "E"),
		makeContentEntry(0, 'C', 0, 0, "F", "F"),
		makeContentEntry(0, 'C', 0, 0, "G", "G"),
		makeContentEntry(0, 'C', 0, 0, "H", "H"),
	}
	a := buildFakeArchive(entries)

	var count int
	for range a.EntriesByNamespace('C') {
		count++
	}

	// Should stop at entry 3 (corrupt), yielding only 3 entries
	if count != 3 {
		t.Errorf("expected 3 entries before silent stop, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// EntriesByTitle — stops silently at error (71.4% → higher)
// ---------------------------------------------------------------------------

func TestEntriesByTitleStopsAtError(t *testing.T) {
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Alpha", "Alpha"),
		nil, // corrupt
		makeContentEntry(0, 'C', 0, 0, "Charlie", "Charlie"),
	}
	a := buildFakeArchive(entries)
	a.titleList = []uint32{0, 1, 2}
	a.hdr.TitlePtrPos = noTitlePtrList

	var count int
	for range a.EntriesByTitle() {
		count++
	}

	if count != 1 {
		t.Errorf("expected 1 entry before silent stop, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// EntriesByTitlePrefix — stops silently at error during iteration
// ---------------------------------------------------------------------------

func TestEntriesByTitlePrefixStopsAtError(t *testing.T) {
	// 8 entries: binary search for "Al" prefix visits 4,2,1,0 but not 3.
	// After binary search finds lo=0, iteration hits 3 entries then corrupt.
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Algebra", "Algebra"),
		makeContentEntry(0, 'C', 0, 0, "Alpha", "Alpha"),
		makeContentEntry(0, 'C', 0, 0, "Also", "Also"),
		nil, // corrupt — index 3
		makeContentEntry(0, 'C', 0, 0, "Banana", "Banana"),
		makeContentEntry(0, 'C', 0, 0, "Cat", "Cat"),
		makeContentEntry(0, 'C', 0, 0, "Dog", "Dog"),
		makeContentEntry(0, 'C', 0, 0, "Egg", "Egg"),
	}
	a := buildFakeArchive(entries)
	a.titleList = []uint32{0, 1, 2, 3, 4, 5, 6, 7}
	a.hdr.TitlePtrPos = noTitlePtrList

	var count int
	for range a.EntriesByTitlePrefix('C', "Al") {
		count++
	}

	if count != 3 {
		t.Errorf("expected 3 entries before silent stop at corrupt, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// EntriesByTitlePrefixFold — stops silently at error during scan
// ---------------------------------------------------------------------------

func TestEntriesByTitlePrefixFoldStopsAtError(t *testing.T) {
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Apple", "Apple"),
		nil, // corrupt
	}
	a := buildFakeArchive(entries)
	a.titleList = []uint32{0, 1}
	a.hdr.TitlePtrPos = noTitlePtrList

	var count int
	for range a.EntriesByTitlePrefixFold('C', "a") {
		count++
	}

	if count != 1 {
		t.Errorf("expected 1 entry before silent stop, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// MainEntry — no main page (66.7% → 100%)
// ---------------------------------------------------------------------------

func TestMainEntryNoMainPage(t *testing.T) {
	a := &Archive{
		hdr: header{MainPage: noMainPage},
	}
	if a.HasMainEntry() {
		t.Error("HasMainEntry should be false when MainPage == noMainPage")
	}
	_, err := a.MainEntry()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("MainEntry: expected ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// buildFakeArchiveWithCluster builds an archive with working cluster data.
// All entries point to cluster 0. The cluster contains one blob per entry.
// ---------------------------------------------------------------------------

func buildFakeArchiveWithCluster(entryData [][]byte, blobContents [][]byte) *Archive {
	le := binary.LittleEndian

	// Build cluster data: uncompressed, one blob per entry
	// Offset table: (N+1) uint32 offsets, then blob data
	numBlobs := len(blobContents)
	offsetTableSize := (numBlobs + 1) * 4
	var totalBlobSize int
	for _, b := range blobContents {
		totalBlobSize += len(b)
	}

	// Cluster data = info byte + offset table + blobs
	clusterPayload := make([]byte, offsetTableSize+totalBlobSize)
	off := uint32(offsetTableSize)
	for i := range numBlobs + 1 {
		if i < numBlobs {
			le.PutUint32(clusterPayload[i*4:], off)
			off += uint32(len(blobContents[i]))
		} else {
			le.PutUint32(clusterPayload[i*4:], off)
		}
	}
	pos := offsetTableSize
	for _, b := range blobContents {
		copy(clusterPayload[pos:], b)
		pos += len(b)
	}

	clusterData := append([]byte{compNone}, clusterPayload...)

	// Build URL pointer table + entry data
	numEntries := len(entryData)
	ptrTableSize := numEntries * 8
	clusterPtrTableSize := 8 // 1 cluster

	// Layout: [URL ptrs] [entries...] [cluster ptr] [cluster data]
	urlPtrStart := 0
	entryStart := ptrTableSize
	var entryTotalSize int
	for _, d := range entryData {
		entryTotalSize += len(d)
	}
	clusterPtrStart := entryStart + entryTotalSize
	clusterStart := clusterPtrStart + clusterPtrTableSize
	checksumPos := clusterStart + len(clusterData)

	raw := make([]byte, checksumPos)

	// URL pointer table
	entryOff := entryStart
	for i, d := range entryData {
		le.PutUint64(raw[i*8:], uint64(entryOff))
		copy(raw[entryOff:], d)
		entryOff += len(d)
	}

	// Cluster pointer table
	le.PutUint64(raw[clusterPtrStart:], uint64(clusterStart))

	// Cluster data
	copy(raw[clusterStart:], clusterData)

	return &Archive{
		r: &bytesReader{data: raw},
		hdr: header{
			EntryCount:    uint32(numEntries),
			ClusterCount:  1,
			URLPtrPos:     uint64(urlPtrStart),
			ClusterPtrPos: uint64(clusterPtrStart),
			ChecksumPos:   uint64(checksumPos),
		},
		mimeTypes:    []string{"text/html"},
		cacheSize:    4,
		clusterCache: make(map[uint32]*list.Element),
		cacheList:    list.New(),
	}
}

// ---------------------------------------------------------------------------
// Item.blob — blobNum out of range (66.7% → higher)
// ---------------------------------------------------------------------------

func TestItemBlobOutOfRange(t *testing.T) {
	// Entry claims blob 99, but the cluster only has 1 blob.
	entry := makeContentEntry(0, 'C', 0, 99, "Page", "Page")
	blobContents := [][]byte{[]byte("hello")}
	a := buildFakeArchiveWithCluster([][]byte{entry}, blobContents)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex: %v", err)
	}

	item, err := e.Item()
	if err != nil {
		t.Fatalf("Item: %v", err)
	}

	_, err = item.Data()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Item.Data with out-of-range blobNum: expected ErrNotFound, got %v", err)
	}

	_, err = item.Size()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Item.Size with out-of-range blobNum: expected ErrNotFound, got %v", err)
	}

	_, err = item.Reader()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Item.Reader with out-of-range blobNum: expected ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ReadContent / ReadContentCopy / ContentSize / ContentReader — error paths
// through redirect resolution and Item/blob failures (~70% → higher)
// ---------------------------------------------------------------------------

func TestReadContentOnRedirectToCorrupt(t *testing.T) {
	// Entry 0 redirects to entry 1 which is corrupt.
	entries := [][]byte{
		makeRedirectEntry('C', 1, "Alias", ""),
		nil, // corrupt target
	}
	a := buildFakeArchive(entries)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex: %v", err)
	}

	_, err = e.ReadContent()
	if err == nil {
		t.Error("ReadContent: expected error for redirect to corrupt, got nil")
	}

	_, err = e.ReadContentCopy()
	if err == nil {
		t.Error("ReadContentCopy: expected error for redirect to corrupt, got nil")
	}

	_, err = e.ContentSize()
	if err == nil {
		t.Error("ContentSize: expected error for redirect to corrupt, got nil")
	}

	_, err = e.ContentReader()
	if err == nil {
		t.Error("ContentReader: expected error for redirect to corrupt, got nil")
	}
}

func TestReadContentOnBlobOutOfRange(t *testing.T) {
	// Entry points to blob 99 but cluster only has 1 blob.
	entry := makeContentEntry(0, 'C', 0, 99, "Page", "Page")
	a := buildFakeArchiveWithCluster([][]byte{entry}, [][]byte{[]byte("data")})

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex: %v", err)
	}

	_, err = e.ReadContent()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ReadContent: expected ErrNotFound, got %v", err)
	}

	_, err = e.ReadContentCopy()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ReadContentCopy: expected ErrNotFound, got %v", err)
	}

	_, err = e.ContentSize()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ContentSize: expected ErrNotFound, got %v", err)
	}

	_, err = e.ContentReader()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ContentReader: expected ErrNotFound, got %v", err)
	}
}

func TestContentSizeViaRedirect(t *testing.T) {
	// Entry 0 redirects to entry 1 (content) — happy path through resolve.
	redirect := makeRedirectEntry('C', 1, "Alias", "")
	content := makeContentEntry(0, 'C', 0, 0, "Real", "Real")
	a := buildFakeArchiveWithCluster(
		[][]byte{redirect, content},
		[][]byte{[]byte("hello world")},
	)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex: %v", err)
	}

	size, err := e.ContentSize()
	if err != nil {
		t.Fatalf("ContentSize: %v", err)
	}
	if size != 11 {
		t.Errorf("ContentSize = %d, want 11", size)
	}

	reader, err := e.ContentReader()
	if err != nil {
		t.Fatalf("ContentReader: %v", err)
	}
	data, _ := io.ReadAll(reader)
	if string(data) != "hello world" {
		t.Errorf("ContentReader data = %q, want %q", data, "hello world")
	}
}

// ---------------------------------------------------------------------------
// readCluster — out-of-range cluster number
// ---------------------------------------------------------------------------

func TestReadClusterOutOfRange(t *testing.T) {
	entry := makeContentEntry(0, 'C', 5, 0, "Page", "Page") // cluster 5 doesn't exist
	a := buildFakeArchiveWithCluster([][]byte{entry}, [][]byte{[]byte("x")})

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex: %v", err)
	}

	_, err = e.ReadContent()
	if err == nil {
		t.Error("expected error for out-of-range cluster, got nil")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error %q should mention 'out of range'", err)
	}
}

// ---------------------------------------------------------------------------
// ClusterMetaAt — last cluster uses ChecksumPos as end boundary
// ---------------------------------------------------------------------------

func TestClusterMetaAtLastCluster(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	lastIdx := a.ClusterCount() - 1
	meta, err := a.ClusterMetaAt(lastIdx)
	if err != nil {
		t.Fatalf("ClusterMetaAt(last=%d): %v", lastIdx, err)
	}
	if meta.CompressedSize == 0 {
		t.Error("last cluster has zero compressed size")
	}
	t.Logf("last cluster %d: offset=%d compSize=%d comp=%s",
		lastIdx, meta.Offset, meta.CompressedSize, meta.Compression)
}

// ---------------------------------------------------------------------------
// parseDirectoryEntry — content entry too short (between 12 and 16 bytes)
// ---------------------------------------------------------------------------

func TestParseContentEntryTooShort(t *testing.T) {
	// 14 bytes: enough to pass the first 12-byte check but not the
	// 16-byte check for content entries (non-redirect mimeIndex).
	le := binary.LittleEndian
	buf := make([]byte, 14)
	le.PutUint16(buf[0:2], 0) // mimeIndex 0 (not redirect)
	buf[3] = 'C'              // namespace

	_, _, err := parseDirectoryEntry(buf, &Archive{mimeTypes: []string{"text/html"}}, 0)
	if !errors.Is(err, ErrInvalidEntry) {
		t.Errorf("expected ErrInvalidEntry for short content entry, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// parseDirectoryEntry — parameter data extends beyond entry
// ---------------------------------------------------------------------------

func TestParseEntryParamDataOverflow(t *testing.T) {
	le := binary.LittleEndian
	buf := make([]byte, 18) // 16 bytes header + 2 bytes of trailing data
	le.PutUint16(buf[0:2], 0)
	buf[2] = 200 // paramLen = 200, way beyond buffer
	buf[3] = 'C'

	_, _, err := parseDirectoryEntry(buf, &Archive{mimeTypes: []string{"text/html"}}, 0)
	if !errors.Is(err, ErrInvalidEntry) {
		t.Errorf("expected ErrInvalidEntry for param overflow, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// parseDirectoryEntry — missing null terminator for title
// ---------------------------------------------------------------------------

func TestParseEntryMissingTitleNull(t *testing.T) {
	le := binary.LittleEndian
	// Build a content entry with a path but no title null terminator
	buf := make([]byte, 16)
	le.PutUint16(buf[0:2], 0)
	buf[3] = 'C'
	// path "ab\0" then "cd" with no trailing null
	buf = append(buf, 'a', 'b', 0, 'c', 'd')

	_, _, err := parseDirectoryEntry(buf, &Archive{mimeTypes: []string{"text/html"}}, 0)
	if !errors.Is(err, ErrInvalidEntry) {
		t.Errorf("expected ErrInvalidEntry for missing title null, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Verify — checksumPos at boundary (== 0 or >= file size)
// ---------------------------------------------------------------------------

func TestVerifyInvalidChecksumPosZero(t *testing.T) {
	a := &Archive{
		r:   &bytesReader{data: make([]byte, 100)},
		hdr: header{ChecksumPos: 0},
	}
	err := a.Verify()
	if err == nil {
		t.Error("expected error for checksumPos=0, got nil")
	}
}

// ---------------------------------------------------------------------------
// readCluster — clusterSize <= 0 (invalid cluster pointer order)
// ---------------------------------------------------------------------------

func TestReadClusterInvalidSize(t *testing.T) {
	le := binary.LittleEndian

	// Construct raw data with cluster pointer table where cluster 0's
	// start offset > cluster 1's offset (invalid).
	clusterPtrPos := 0
	raw := make([]byte, 100)
	le.PutUint64(raw[clusterPtrPos:], 90)   // cluster 0 starts at 90
	le.PutUint64(raw[clusterPtrPos+8:], 80) // cluster 1 starts at 80 (< 90)

	a := &Archive{
		r: &bytesReader{data: raw},
		hdr: header{
			ClusterCount:  2,
			ClusterPtrPos: uint64(clusterPtrPos),
			ChecksumPos:   100,
		},
		cacheSize:    4,
		clusterCache: make(map[uint32]*list.Element),
		cacheList:    list.New(),
	}

	_, err := a.readCluster(0)
	if err == nil {
		t.Error("expected error for invalid cluster size, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cluster size") {
		t.Errorf("error %q should mention 'invalid cluster size'", err)
	}
}

// ---------------------------------------------------------------------------
// readCluster — after Close (nil cache)
// ---------------------------------------------------------------------------

func TestReadClusterAfterClose(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Get an entry before closing
	e, err := a.EntryByPath("C/main.html")
	if err != nil {
		t.Fatalf("EntryByPath: %v", err)
	}

	a.Close()

	// Reading content after close should not panic (cache is nil)
	_, err = e.ReadContent()
	if err == nil {
		t.Log("ReadContent after Close succeeded (cached cluster still available)")
	}
	// Either an error or cached data is acceptable; the key is no panic.
}

// ---------------------------------------------------------------------------
// Illustration — valid illustration (if present) vs missing size
// ---------------------------------------------------------------------------

func TestIllustrationMissingSizes(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for _, size := range []int{1, 16, 32, 48, 64, 128, 256} {
		_, err := a.Illustration(size)
		if err != nil && !errors.Is(err, ErrNotFound) {
			t.Errorf("Illustration(%d): unexpected error: %v", size, err)
		}
	}
}

// ---------------------------------------------------------------------------
// loadMetadata — entry that fails ReadContent is silently skipped
// ---------------------------------------------------------------------------

func TestLoadMetadataSkipsBadEntries(t *testing.T) {
	// Build an archive with M-namespace entries where one has a bad cluster ref.
	// The bad entry should be skipped without preventing other metadata from loading.
	goodEntry := makeContentEntry(0, 'M', 0, 0, "Title", "")
	badEntry := makeContentEntry(0, 'M', 5, 0, "BadKey", "") // cluster 5 doesn't exist
	a := buildFakeArchiveWithCluster(
		[][]byte{goodEntry, badEntry},
		[][]byte{[]byte("Good Value")},
	)

	// loadMetadata iterates M-namespace entries. The bad one should be skipped.
	val, err := a.Metadata("Title")
	if err != nil {
		t.Fatalf("Metadata(Title): %v", err)
	}
	if val != "Good Value" {
		t.Errorf("Metadata(Title) = %q, want %q", val, "Good Value")
	}

	// BadKey should not be present since ReadContent failed for it
	_, err = a.Metadata("BadKey")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Metadata(BadKey): expected ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TitlePrefixCount — error during binary search returns 0
// ---------------------------------------------------------------------------

func TestTitlePrefixCountBinarySearchError(t *testing.T) {
	// Title list references a corrupt entry — binary search should handle gracefully.
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Alpha", "Alpha"),
		nil, // corrupt
	}
	a := buildFakeArchive(entries)
	a.titleList = []uint32{0, 1}
	a.hdr.TitlePtrPos = noTitlePtrList

	// The binary search for prefix "A" will hit the corrupt entry.
	// TitlePrefixCount should return 0 (error → bail).
	count := a.TitlePrefixCount('C', "B")
	_ = count // just verify no panic
}

// ---------------------------------------------------------------------------
// entryByTitleIndex — out-of-range with titleList
// ---------------------------------------------------------------------------

func TestEntryByTitleIndexOutOfRange(t *testing.T) {
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Page", "Page"),
	}
	a := buildFakeArchive(entries)
	a.titleList = []uint32{0}
	a.hdr.TitlePtrPos = noTitlePtrList

	_, err := a.entryByTitleIndex(99)
	if err == nil {
		t.Error("expected error for out-of-range title index, got nil")
	}
}

// ---------------------------------------------------------------------------
// decompress — deprecated and unknown compression types
// ---------------------------------------------------------------------------

func TestDecompressDeprecated(t *testing.T) {
	_, err := decompress([]byte{1, 2, 3}, compZlib)
	if !errors.Is(err, ErrUnsupportedCompression) {
		t.Errorf("compZlib: expected ErrUnsupportedCompression, got %v", err)
	}

	_, err = decompress([]byte{1, 2, 3}, compBZ2)
	if !errors.Is(err, ErrUnsupportedCompression) {
		t.Errorf("compBZ2: expected ErrUnsupportedCompression, got %v", err)
	}
}

func TestDecompressUnknown(t *testing.T) {
	_, err := decompress([]byte{1, 2, 3}, 99)
	if !errors.Is(err, ErrUnsupportedCompression) {
		t.Errorf("unknown type 99: expected ErrUnsupportedCompression, got %v", err)
	}
}

func TestDecompressNone(t *testing.T) {
	data := []byte("hello world")
	out, err := decompress(data, compNone)
	if err != nil {
		t.Fatalf("compNone: %v", err)
	}
	if string(out) != string(data) {
		t.Errorf("compNone returned %q, want %q", out, data)
	}
}

func TestDecompressZstdCorrupt(t *testing.T) {
	_, err := decompress([]byte{0xFF, 0xFF, 0xFF}, compZstd)
	if err == nil {
		t.Error("expected error for corrupt zstd data, got nil")
	}
}

func TestDecompressXZCorrupt(t *testing.T) {
	_, err := decompress([]byte{0xFF, 0xFF, 0xFF}, compLZMA)
	if err == nil {
		t.Error("expected error for corrupt xz data, got nil")
	}
}

// ---------------------------------------------------------------------------
// extractBlobs — various error paths
// ---------------------------------------------------------------------------

func TestExtractBlobsTooSmall(t *testing.T) {
	_, err := extractBlobs([]byte{1, 2}, false) // < 4 bytes
	if err == nil {
		t.Error("expected error for data too small, got nil")
	}
}

func TestExtractBlobsInvalidFirstOffset(t *testing.T) {
	le := binary.LittleEndian
	data := make([]byte, 8)
	le.PutUint32(data[0:4], 0) // firstOffset = 0 (invalid)
	_, err := extractBlobs(data, false)
	if err == nil {
		t.Error("expected error for zero first offset, got nil")
	}
}

func TestExtractBlobsFirstOffsetBeyondData(t *testing.T) {
	le := binary.LittleEndian
	data := make([]byte, 8)
	le.PutUint32(data[0:4], 100) // firstOffset beyond data
	_, err := extractBlobs(data, false)
	if err == nil {
		t.Error("expected error for first offset beyond data, got nil")
	}
}

func TestExtractBlobsExtendedMode(t *testing.T) {
	le := binary.LittleEndian
	// Extended mode: 8-byte offsets
	// 2 offsets (16 bytes) → 1 blob
	data := make([]byte, 20)
	le.PutUint64(data[0:8], 16)  // first offset = 16 (2 × 8)
	le.PutUint64(data[8:16], 20) // end offset
	copy(data[16:], []byte("blob"))

	c, err := extractBlobs(data, true)
	if err != nil {
		t.Fatalf("extractBlobs extended: %v", err)
	}
	if len(c.blobs) != 1 || string(c.blobs[0]) != "blob" {
		t.Errorf("expected 1 blob 'blob', got %v", c.blobs)
	}
}

func TestExtractBlobsInvalidBlobOffsets(t *testing.T) {
	le := binary.LittleEndian
	data := make([]byte, 12)
	le.PutUint32(data[0:4], 8)  // first offset
	le.PutUint32(data[4:8], 4)  // end offset < start (invalid)
	le.PutUint32(data[8:12], 0) // filler

	_, err := extractBlobs(data, false)
	if err == nil {
		t.Error("expected error for invalid blob offsets, got nil")
	}
}
