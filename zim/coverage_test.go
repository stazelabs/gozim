package zim

import (
	"errors"
	"io"
	"testing"
)

// ---------------------------------------------------------------------------
// ClusterMetaAt
// ---------------------------------------------------------------------------

func TestClusterMetaAt(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for i := range a.ClusterCount() {
		meta, err := a.ClusterMetaAt(i)
		if err != nil {
			t.Fatalf("ClusterMetaAt(%d): %v", i, err)
		}
		if meta.Offset == 0 && i > 0 {
			t.Errorf("cluster %d: unexpected zero offset", i)
		}
		if meta.CompressedSize == 0 {
			t.Errorf("cluster %d: zero compressed size", i)
		}
		validComp := map[string]bool{"none": true, "xz": true, "zstd": true, "unknown": true}
		if !validComp[meta.Compression] {
			t.Errorf("cluster %d: unexpected compression %q", i, meta.Compression)
		}
		t.Logf("cluster %d: offset=%d compSize=%d comp=%s extended=%v",
			i, meta.Offset, meta.CompressedSize, meta.Compression, meta.Extended)
	}
}

func TestClusterMetaAtOutOfRange(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	_, err = a.ClusterMetaAt(a.ClusterCount())
	if err == nil {
		t.Error("expected error for out-of-range cluster, got nil")
	}
}

// ---------------------------------------------------------------------------
// ClusterBlobSizes
// ---------------------------------------------------------------------------

func TestClusterBlobSizes(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for i := range a.ClusterCount() {
		sizes, err := a.ClusterBlobSizes(i)
		if err != nil {
			t.Fatalf("ClusterBlobSizes(%d): %v", i, err)
		}
		if len(sizes) == 0 {
			t.Errorf("cluster %d: no blobs", i)
		}
		for j, s := range sizes {
			if s < 0 {
				t.Errorf("cluster %d blob %d: negative size %d", i, j, s)
			}
		}
		t.Logf("cluster %d: %d blobs, sizes=%v", i, len(sizes), sizes)
	}
}

func TestClusterBlobSizesMatchContent(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// For each content entry, verify BlobSize matches the corresponding
	// ClusterBlobSizes element.
	for i := range a.EntryCount() {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.IsRedirect() {
			continue
		}
		sizes, err := a.ClusterBlobSizes(e.ClusterNum())
		if err != nil {
			t.Fatalf("ClusterBlobSizes(%d): %v", e.ClusterNum(), err)
		}
		blobIdx := int(e.BlobNum())
		if blobIdx >= len(sizes) {
			t.Errorf("entry %s: blobNum %d >= cluster blob count %d", e.FullPath(), blobIdx, len(sizes))
			continue
		}
		blobSize, err := e.BlobSize()
		if err != nil {
			t.Fatalf("BlobSize for %s: %v", e.FullPath(), err)
		}
		if int(blobSize) != sizes[blobIdx] {
			t.Errorf("entry %s: BlobSize=%d, ClusterBlobSizes[%d]=%d", e.FullPath(), blobSize, blobIdx, sizes[blobIdx])
		}
	}
}

// ---------------------------------------------------------------------------
// EntriesInCluster
// ---------------------------------------------------------------------------

func TestEntriesInCluster(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Verify every content entry appears in exactly one cluster's result.
	seen := make(map[uint32]bool)
	for i := range a.ClusterCount() {
		entries, err := a.EntriesInCluster(i)
		if err != nil {
			t.Fatalf("EntriesInCluster(%d): %v", i, err)
		}
		for _, e := range entries {
			if e.IsRedirect() {
				t.Errorf("EntriesInCluster(%d) returned redirect entry %s", i, e.FullPath())
			}
			if e.ClusterNum() != i {
				t.Errorf("entry %s: ClusterNum=%d, expected %d", e.FullPath(), e.ClusterNum(), i)
			}
			if seen[e.Index()] {
				t.Errorf("entry %s (index %d) seen in multiple clusters", e.FullPath(), e.Index())
			}
			seen[e.Index()] = true
		}
	}

	// Count all non-redirect entries and verify they were all found.
	var contentCount int
	for i := range a.EntryCount() {
		e, _ := a.EntryByIndex(i)
		if !e.IsRedirect() {
			contentCount++
		}
	}
	if len(seen) != contentCount {
		t.Errorf("found %d entries in clusters, expected %d content entries", len(seen), contentCount)
	}
}

func TestEntriesInClusterOutOfRange(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	_, err = a.EntriesInCluster(a.ClusterCount())
	if err == nil {
		t.Error("expected error for out-of-range cluster, got nil")
	}
}

// ---------------------------------------------------------------------------
// AllEntriesByTitle
// ---------------------------------------------------------------------------

func TestAllEntriesByTitleNoErrors(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	var count int
	var prevNs byte
	var prevTitle string
	for e, err := range a.AllEntriesByTitle() {
		if err != nil {
			t.Fatalf("AllEntriesByTitle error at entry %d: %v", count, err)
		}
		ns := e.Namespace()
		title := e.Title()
		if count > 0 && compareTitleKey(ns, title, prevNs, prevTitle) < 0 {
			t.Errorf("title order violated: (%c,%q) after (%c,%q)", ns, title, prevNs, prevTitle)
		}
		prevNs = ns
		prevTitle = title
		count++
	}

	if uint32(count) != a.EntryCount() {
		t.Errorf("iterated %d entries, want %d", count, a.EntryCount())
	}
}

func TestAllEntriesByTitleReportsError(t *testing.T) {
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Alpha", "Alpha"),
		nil, // corrupt
		makeContentEntry(0, 'C', 0, 0, "Charlie", "Charlie"),
	}
	a := buildFakeArchive(entries)
	a.titleList = []uint32{0, 1, 2}
	a.hdr.TitlePtrPos = noTitlePtrList

	var paths []string
	var gotErr error
	for e, err := range a.AllEntriesByTitle() {
		if err != nil {
			gotErr = err
			break
		}
		paths = append(paths, e.Path())
	}

	if len(paths) != 1 || paths[0] != "Alpha" {
		t.Errorf("expected [Alpha] before error, got %v", paths)
	}
	if gotErr == nil {
		t.Error("expected error for corrupt entry, got nil")
	}
}

func TestAllEntriesByTitleEarlyBreak(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	var count int
	for _, err := range a.AllEntriesByTitle() {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
		if count >= 3 {
			break
		}
	}
	if count != 3 && a.EntryCount() >= 3 {
		t.Errorf("expected 3 entries with early break, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// AllEntriesByTitlePrefix
// ---------------------------------------------------------------------------

func TestAllEntriesByTitlePrefixNoErrors(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	var found bool
	for e, err := range a.AllEntriesByTitlePrefix('C', "Test") {
		if err != nil {
			t.Fatalf("AllEntriesByTitlePrefix error: %v", err)
		}
		if e.Title() == "Test ZIM file" {
			found = true
		}
	}
	if !found {
		t.Error("AllEntriesByTitlePrefix('C', 'Test') did not find 'Test ZIM file'")
	}
}

func TestAllEntriesByTitlePrefixReportsError(t *testing.T) {
	// Build archive where the second entry (in title order) is corrupt.
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Apple", "Apple"),
		nil, // corrupt
		makeContentEntry(0, 'C', 0, 0, "Avocado", "Avocado"),
	}
	a := buildFakeArchive(entries)
	a.titleList = []uint32{0, 1, 2}
	a.hdr.TitlePtrPos = noTitlePtrList

	var gotErr error
	for _, err := range a.AllEntriesByTitlePrefix('C', "A") {
		if err != nil {
			gotErr = err
			break
		}
	}
	if gotErr == nil {
		t.Error("expected error for corrupt entry, got nil")
	}
}

// ---------------------------------------------------------------------------
// AllEntriesByTitlePrefixFold
// ---------------------------------------------------------------------------

func TestAllEntriesByTitlePrefixFoldNoErrors(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	var found bool
	for e, err := range a.AllEntriesByTitlePrefixFold('C', "test") {
		if err != nil {
			t.Fatalf("AllEntriesByTitlePrefixFold error: %v", err)
		}
		if e.Title() == "Test ZIM file" {
			found = true
		}
	}
	if !found {
		t.Error("AllEntriesByTitlePrefixFold('C', 'test') did not find 'Test ZIM file'")
	}
}

func TestAllEntriesByTitlePrefixFoldReportsError(t *testing.T) {
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Apple", "Apple"),
		nil, // corrupt — scanned during linear search
	}
	a := buildFakeArchive(entries)
	a.titleList = []uint32{0, 1}
	a.hdr.TitlePtrPos = noTitlePtrList

	var gotErr error
	for _, err := range a.AllEntriesByTitlePrefixFold('C', "a") {
		if err != nil {
			gotErr = err
			break
		}
	}
	if gotErr == nil {
		t.Error("expected error for corrupt entry, got nil")
	}
}

// ---------------------------------------------------------------------------
// FulltextIndexFormat / TitleIndexFormat / HasTitleIndex
// ---------------------------------------------------------------------------

func TestIndexFormats(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// small.zim likely doesn't have Xapian indexes — just verify no panic
	// and that the return values are sane.
	hasFT := a.HasFulltextIndex()
	hasTI := a.HasTitleIndex()
	ftFmt := a.FulltextIndexFormat()
	tiFmt := a.TitleIndexFormat()

	t.Logf("HasFulltextIndex=%v format=%q", hasFT, ftFmt)
	t.Logf("HasTitleIndex=%v format=%q", hasTI, tiFmt)

	// If there's no index, format should be empty.
	if !hasFT && ftFmt != "" {
		t.Errorf("no fulltext index but format=%q", ftFmt)
	}
	if !hasTI && tiFmt != "" {
		t.Errorf("no title index but format=%q", tiFmt)
	}
}

// ---------------------------------------------------------------------------
// Item methods: Size, Reader, MIMEType, Path, FullPath
// ---------------------------------------------------------------------------

func TestItemMethods(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Find a non-redirect content entry.
	for i := range a.EntryCount() {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.IsRedirect() {
			continue
		}

		item, err := e.Item()
		if err != nil {
			t.Fatalf("Item for %s: %v", e.FullPath(), err)
		}

		// Size
		size, err := item.Size()
		if err != nil {
			t.Fatalf("Item.Size: %v", err)
		}
		data, err := e.ReadContent()
		if err != nil {
			t.Fatalf("ReadContent: %v", err)
		}
		if size != int64(len(data)) {
			t.Errorf("Item.Size=%d, ReadContent len=%d", size, len(data))
		}

		// Reader
		reader, err := item.Reader()
		if err != nil {
			t.Fatalf("Item.Reader: %v", err)
		}
		readData, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(readData) != string(data) {
			t.Error("Item.Reader content differs from ReadContent")
		}

		// MIMEType
		if item.MIMEType() != e.MIMEType() {
			t.Errorf("Item.MIMEType=%q, Entry.MIMEType=%q", item.MIMEType(), e.MIMEType())
		}

		// Path
		if item.Path() != e.Path() {
			t.Errorf("Item.Path=%q, Entry.Path=%q", item.Path(), e.Path())
		}

		// FullPath
		if item.FullPath() != e.FullPath() {
			t.Errorf("Item.FullPath=%q, Entry.FullPath=%q", item.FullPath(), e.FullPath())
		}

		break // one entry is sufficient
	}
}

// ---------------------------------------------------------------------------
// Metadata edge cases
// ---------------------------------------------------------------------------

func TestMetadataAllKeys(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Collect all M-namespace entries and verify Metadata returns each one.
	var keys []string
	for e := range a.EntriesByNamespace('M') {
		keys = append(keys, e.Path())
	}

	for _, key := range keys {
		val, err := a.Metadata(key)
		if err != nil {
			t.Errorf("Metadata(%q): %v", key, err)
			continue
		}
		if val == "" {
			t.Logf("Metadata(%q) is empty (valid but notable)", key)
		} else {
			t.Logf("Metadata(%q) = %q", key, val)
		}
	}

	// Verify caching: second call should also work.
	for _, key := range keys {
		_, err := a.Metadata(key)
		if err != nil {
			t.Errorf("Metadata(%q) on second call: %v", key, err)
		}
	}
}

func TestMetadataMissingKeyAfterCacheLoad(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Force cache load by requesting a known key.
	_, _ = a.Metadata("Title")

	// Now request a non-existent key — should still return ErrNotFound.
	_, err = a.Metadata("ThisKeyDefinitelyDoesNotExist")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Entry.BlobSize
// ---------------------------------------------------------------------------

func TestBlobSize(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for i := range a.EntryCount() {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.IsRedirect() {
			// BlobSize on redirect should return ErrIsRedirect.
			_, err = e.BlobSize()
			if !errors.Is(err, ErrIsRedirect) {
				t.Errorf("BlobSize on redirect %s: expected ErrIsRedirect, got %v", e.FullPath(), err)
			}
			continue
		}
		size, err := e.BlobSize()
		if err != nil {
			t.Fatalf("BlobSize for %s: %v", e.FullPath(), err)
		}
		data, err := e.ReadContent()
		if err != nil {
			t.Fatalf("ReadContent for %s: %v", e.FullPath(), err)
		}
		if size != int64(len(data)) {
			t.Errorf("BlobSize for %s: got %d, want %d", e.FullPath(), size, len(data))
		}
	}
}

// ---------------------------------------------------------------------------
// Blob methods: String, Size, Reader
// ---------------------------------------------------------------------------

func TestBlobMethods(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for i := range a.EntryCount() {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.IsRedirect() {
			continue
		}
		item, err := e.Item()
		if err != nil {
			t.Fatalf("Item: %v", err)
		}
		blob, err := item.Data()
		if err != nil {
			t.Fatalf("Data: %v", err)
		}

		// Size
		if blob.Size() != len(blob.Bytes()) {
			t.Errorf("Blob.Size=%d, len(Bytes)=%d", blob.Size(), len(blob.Bytes()))
		}

		// String
		if blob.String() != string(blob.Bytes()) {
			t.Error("Blob.String differs from string(Bytes)")
		}

		// Reader
		readData, err := io.ReadAll(blob.Reader())
		if err != nil {
			t.Fatalf("ReadAll from Blob.Reader: %v", err)
		}
		if string(readData) != string(blob.Bytes()) {
			t.Error("Blob.Reader content differs from Bytes")
		}

		break // one entry is sufficient
	}
}

func TestBlobMethodsEmpty(t *testing.T) {
	b := Blob{}
	if b.Size() != 0 {
		t.Errorf("empty Blob.Size = %d, want 0", b.Size())
	}
	if b.String() != "" {
		t.Errorf("empty Blob.String = %q, want empty", b.String())
	}
	data, err := io.ReadAll(b.Reader())
	if err != nil {
		t.Fatalf("ReadAll from empty Blob.Reader: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("empty Blob.Reader returned %d bytes", len(data))
	}
}

// ---------------------------------------------------------------------------
// Version and MIME type accessors
// ---------------------------------------------------------------------------

func TestVersionAccessors(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	major := a.MajorVersion()
	minor := a.MinorVersion()
	if major != 5 && major != 6 {
		t.Errorf("unexpected major version %d", major)
	}
	t.Logf("Version: %d.%d", major, minor)
}

func TestMIMETypesAccessor(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	types := a.MIMETypes()
	if len(types) == 0 {
		t.Error("expected at least one MIME type")
	}

	// Verify it's a copy (modifying returned slice shouldn't affect archive).
	original := a.MIMETypes()
	types[0] = "modified/test"
	if a.MIMETypes()[0] == "modified/test" {
		t.Error("MIMETypes returned aliased slice instead of copy")
	}
	_ = original
}

// ---------------------------------------------------------------------------
// IsSplit / SplitParts on non-split archive
// ---------------------------------------------------------------------------

func TestIsSplitNonSplit(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	if a.IsSplit() {
		t.Error("small.zim should not be a split archive")
	}
	if parts := a.SplitParts(); parts != nil {
		t.Errorf("SplitParts should return nil for non-split, got %v", parts)
	}
}

// ---------------------------------------------------------------------------
// HasMainEntry / MainEntry
// ---------------------------------------------------------------------------

func TestMainEntry(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	if !a.HasMainEntry() {
		t.Skip("small.zim has no main entry")
	}

	main, err := a.MainEntry()
	if err != nil {
		t.Fatalf("MainEntry: %v", err)
	}
	t.Logf("MainEntry: %s (redirect=%v)", main.FullPath(), main.IsRedirect())
}

// ---------------------------------------------------------------------------
// Entry.Index / Entry.ClusterNum / Entry.BlobNum accessors
// ---------------------------------------------------------------------------

func TestEntryAccessors(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for i := range a.EntryCount() {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.Index() != i {
			t.Errorf("entry %d: Index()=%d", i, e.Index())
		}
		if !e.IsRedirect() {
			// ClusterNum and BlobNum should be within valid ranges.
			if e.ClusterNum() >= a.ClusterCount() {
				t.Errorf("entry %s: ClusterNum=%d >= ClusterCount=%d", e.FullPath(), e.ClusterNum(), a.ClusterCount())
			}
		}
	}
}

// ---------------------------------------------------------------------------
// MIMEType edge case: out-of-range mimeIndex
// ---------------------------------------------------------------------------

func TestMIMETypeOutOfRange(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html"}}
	// mimeIndex 5 is out of range for a 1-element MIME list.
	data := makeContentEntry(5, 'C', 0, 0, "Page", "Page")
	e, _, err := parseDirectoryEntry(data, archive, 0)
	if err != nil {
		t.Fatalf("parseDirectoryEntry: %v", err)
	}
	if mime := e.MIMEType(); mime != "" {
		t.Errorf("expected empty MIME for out-of-range index, got %q", mime)
	}
}

func TestMIMETypeNilArchive(t *testing.T) {
	// Entry with nil archive should return empty MIME type.
	data := makeContentEntry(0, 'C', 0, 0, "Page", "Page")
	e, _, err := parseDirectoryEntry(data, nil, 0)
	if err != nil {
		t.Fatalf("parseDirectoryEntry: %v", err)
	}
	if mime := e.MIMEType(); mime != "" {
		t.Errorf("expected empty MIME for nil archive, got %q", mime)
	}
}
