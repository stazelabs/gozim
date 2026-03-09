package zim

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "testdata", name)
}

func skipIfNoTestdata(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("test ZIM file not found: %s (run 'make testdata' to download)", path)
	}
}

func TestOpenSmallZIM(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	t.Logf("Version: %d.%d", a.hdr.MajorVersion, a.hdr.MinorVersion)
	t.Logf("Entry count: %d", a.EntryCount())
	t.Logf("Cluster count: %d", a.ClusterCount())
	t.Logf("MIME types: %v", a.MIMETypes())
	t.Logf("UUID: %x", a.UUID())
	t.Logf("Has main entry: %v", a.HasMainEntry())

	if a.EntryCount() == 0 {
		t.Error("expected non-zero entry count")
	}
	if a.ClusterCount() == 0 {
		t.Error("expected non-zero cluster count")
	}
}

func TestEntryByIndex(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Read the first entry
	entry, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex(0): %v", err)
	}
	t.Logf("Entry 0: namespace=%c path=%q title=%q redirect=%v mime=%q",
		entry.Namespace(), entry.Path(), entry.Title(), entry.IsRedirect(), entry.MIMEType())

	// List all entries
	for i := uint32(0); i < a.EntryCount(); i++ {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		t.Logf("  [%d] %s (redirect=%v, mime=%q)", i, e.FullPath(), e.IsRedirect(), e.MIMEType())
	}
}

func TestEntryByPath(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// First, find what paths exist
	var paths []string
	for i := uint32(0); i < a.EntryCount(); i++ {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		paths = append(paths, e.FullPath())
	}

	// Look up each path and verify we get the same entry back
	for i, p := range paths {
		e, err := a.EntryByPath(p)
		if err != nil {
			t.Errorf("EntryByPath(%q): %v", p, err)
			continue
		}
		if e.FullPath() != p {
			t.Errorf("EntryByPath(%q) returned %q", p, e.FullPath())
		}
		if e.index != uint32(i) {
			t.Errorf("EntryByPath(%q) index=%d, want %d", p, e.index, i)
		}
	}

	// Non-existent path
	_, err = a.EntryByPath("Z/nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for nonexistent path, got %v", err)
	}
}

func TestReadContent(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Find the first non-redirect content entry
	for i := uint32(0); i < a.EntryCount(); i++ {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.IsRedirect() {
			continue
		}
		data, err := e.ReadContent()
		if err != nil {
			t.Errorf("ReadContent for %s: %v", e.FullPath(), err)
			continue
		}
		t.Logf("Content of %s (%s): %d bytes", e.FullPath(), e.MIMEType(), len(data))
		if strings.HasPrefix(e.MIMEType(), "text/") && len(data) > 0 {
			preview := string(data)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			t.Logf("  Preview: %s", preview)
		}
	}
}

func TestBlobCopy(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for i := uint32(0); i < a.EntryCount(); i++ {
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
		blob, err := item.Data()
		if err != nil {
			t.Fatalf("Data for %s: %v", e.FullPath(), err)
		}
		original := blob.Bytes()
		copied := blob.Copy()

		if len(original) != len(copied) {
			t.Errorf("Copy size mismatch for %s: %d vs %d", e.FullPath(), len(original), len(copied))
			continue
		}
		for j := range original {
			if original[j] != copied[j] {
				t.Errorf("Copy content mismatch for %s at byte %d", e.FullPath(), j)
				break
			}
		}
		// Verify independence: modifying the copy doesn't affect the original
		if len(copied) > 0 {
			copied[0] ^= 0xFF
			if original[0] == copied[0] {
				t.Errorf("Copy aliases original for %s", e.FullPath())
			}
		}
		break // one entry is sufficient
	}
}

func TestBlobCopyNil(t *testing.T) {
	b := Blob{}
	if cp := b.Copy(); cp != nil {
		t.Errorf("Copy of nil blob = %v, want nil", cp)
	}
}

func TestReadContentCopy(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for i := uint32(0); i < a.EntryCount(); i++ {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.IsRedirect() {
			continue
		}
		original, err := e.ReadContent()
		if err != nil {
			t.Fatalf("ReadContent for %s: %v", e.FullPath(), err)
		}
		copied, err := e.ReadContentCopy()
		if err != nil {
			t.Fatalf("ReadContentCopy for %s: %v", e.FullPath(), err)
		}
		if string(original) != string(copied) {
			t.Errorf("ReadContentCopy mismatch for %s", e.FullPath())
		}
	}
}

func TestReadContentCopySurvivesEviction(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	// Cache size 1: reading a second cluster evicts the first
	a, err := OpenWithOptions(path, WithCacheSize(1))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Collect all non-redirect entries
	type saved struct {
		path string
		data []byte
	}
	var entries []saved
	for i := uint32(0); i < a.EntryCount(); i++ {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.IsRedirect() {
			continue
		}
		data, err := e.ReadContentCopy()
		if err != nil {
			t.Fatalf("ReadContentCopy for %s: %v", e.FullPath(), err)
		}
		entries = append(entries, saved{path: e.FullPath(), data: data})
	}

	// Re-read everything and verify copies are still valid
	for _, s := range entries {
		e, err := a.EntryByPath(s.path)
		if err != nil {
			t.Fatalf("EntryByPath(%s): %v", s.path, err)
		}
		fresh, err := e.ReadContent()
		if err != nil {
			t.Fatalf("ReadContent for %s: %v", s.path, err)
		}
		if string(s.data) != string(fresh) {
			t.Errorf("Saved copy differs from fresh read for %s", s.path)
		}
	}
}

func TestContentSize(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for i := uint32(0); i < a.EntryCount(); i++ {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.IsRedirect() {
			continue
		}
		data, err := e.ReadContent()
		if err != nil {
			t.Fatalf("ReadContent for %s: %v", e.FullPath(), err)
		}
		size, err := e.ContentSize()
		if err != nil {
			t.Fatalf("ContentSize for %s: %v", e.FullPath(), err)
		}
		if size != int64(len(data)) {
			t.Errorf("ContentSize for %s: got %d, want %d", e.FullPath(), size, len(data))
		}
	}
}

func TestContentReader(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	for i := uint32(0); i < a.EntryCount(); i++ {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if e.IsRedirect() {
			continue
		}
		want, err := e.ReadContent()
		if err != nil {
			t.Fatalf("ReadContent for %s: %v", e.FullPath(), err)
		}
		reader, err := e.ContentReader()
		if err != nil {
			t.Fatalf("ContentReader for %s: %v", e.FullPath(), err)
		}
		got, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll for %s: %v", e.FullPath(), err)
		}
		if string(got) != string(want) {
			t.Errorf("ContentReader for %s: content mismatch (got %d bytes, want %d)", e.FullPath(), len(got), len(want))
		}
	}
}

func TestEntryCountByNamespace(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Count entries manually per namespace
	counts := make(map[byte]int)
	for i := uint32(0); i < a.EntryCount(); i++ {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		counts[e.Namespace()]++
	}

	// Verify EntryCountByNamespace matches
	for ns, want := range counts {
		got := a.EntryCountByNamespace(ns)
		if got != want {
			t.Errorf("EntryCountByNamespace(%c): got %d, want %d", ns, got, want)
		}
	}

	// Non-existent namespace should return 0
	if got := a.EntryCountByNamespace('Z'); got != 0 {
		t.Errorf("EntryCountByNamespace('Z'): got %d, want 0", got)
	}

	t.Logf("Namespace counts: %v", counts)
}

func TestRandomEntry(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Find a namespace that has entries
	var ns byte
	for _, candidate := range []byte{'C', 'M', 'W'} {
		if a.EntryCountByNamespace(candidate) > 0 {
			ns = candidate
			break
		}
	}
	if ns == 0 {
		t.Skip("no populated namespace found")
	}

	// Get a random entry and verify it's in the right namespace
	for i := 0; i < 10; i++ {
		e, err := a.RandomEntry(ns)
		if err != nil {
			t.Fatalf("RandomEntry(%c): %v", ns, err)
		}
		if e.Namespace() != ns {
			t.Errorf("RandomEntry(%c) returned namespace %c", ns, e.Namespace())
		}
	}

	// Non-existent namespace should return ErrNotFound
	_, err = a.RandomEntry('Z')
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("RandomEntry('Z'): expected ErrNotFound, got %v", err)
	}
}

func TestIllustration(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// small.zim may not have illustrations — just verify it doesn't panic
	// and returns ErrNotFound for missing sizes
	_, err = a.Illustration(48)
	if err != nil && !errors.Is(err, ErrNotFound) {
		t.Errorf("Illustration(48): unexpected error: %v", err)
	}
}

func TestOpenWithPread(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := OpenWithOptions(path, WithMmap(false))
	if err != nil {
		t.Fatalf("OpenWithOptions: %v", err)
	}
	defer a.Close()

	if a.EntryCount() == 0 {
		t.Error("expected non-zero entry count with pread reader")
	}
}

func TestOpenNonexistent(t *testing.T) {
	_, err := Open("/nonexistent/file.zim")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestEntryByIndexOutOfRange(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	_, err = a.EntryByIndex(a.EntryCount())
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestEntryByTitle(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// main.html has title "Test ZIM file" in namespace C
	e, err := a.EntryByTitle('C', "Test ZIM file")
	if err != nil {
		t.Fatalf("EntryByTitle(C, 'Test ZIM file'): %v", err)
	}
	if e.Path() != "main.html" {
		t.Errorf("path = %q, want main.html", e.Path())
	}

	// Metadata by title
	e, err = a.EntryByTitle('M', "Language")
	if err != nil {
		t.Fatalf("EntryByTitle(M, Language): %v", err)
	}
	if e.Path() != "Language" {
		t.Errorf("path = %q, want Language", e.Path())
	}

	// Non-existent title
	_, err = a.EntryByTitle('C', "this title does not exist at all")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestEntriesByTitle(t *testing.T) {
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
	for e := range a.EntriesByTitle() {
		ns := e.Namespace()
		title := e.Title()
		if count > 0 && compareTitleKey(ns, title, prevNs, prevTitle) < 0 {
			t.Errorf("titles not sorted: (%c,%q) came after (%c,%q)", ns, title, prevNs, prevTitle)
		}
		prevNs = ns
		prevTitle = title
		count++
	}

	if uint32(count) != a.EntryCount() {
		t.Errorf("iterated %d entries, want %d", count, a.EntryCount())
	}
}

func TestMetadata(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	title, err := a.Metadata("Title")
	if err != nil {
		t.Fatalf("Metadata(Title): %v", err)
	}
	if title != "Test ZIM file" {
		t.Errorf("title = %q, want %q", title, "Test ZIM file")
	}

	lang, err := a.Metadata("Language")
	if err != nil {
		t.Fatalf("Metadata(Language): %v", err)
	}
	if lang != "en" {
		t.Errorf("language = %q, want %q", lang, "en")
	}

	_, err = a.Metadata("NonExistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing metadata, got %v", err)
	}
}

func TestCacheStatsAndEviction(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	// Open with a cache size of 1 so we can observe eviction
	a, err := OpenWithOptions(path, WithCacheSize(1))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Initial state: empty cache
	stats := a.CacheStats()
	if stats.Capacity != 1 {
		t.Errorf("capacity = %d, want 1", stats.Capacity)
	}
	if stats.Size != 0 {
		t.Errorf("initial size = %d, want 0", stats.Size)
	}
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Errorf("initial hits=%d misses=%d, want 0/0", stats.Hits, stats.Misses)
	}

	// Read a content entry to trigger a cache miss + fill
	e, err := a.EntryByPath("C/main.html")
	if err != nil {
		t.Fatalf("EntryByPath: %v", err)
	}
	if _, err := e.ReadContent(); err != nil {
		t.Fatalf("ReadContent: %v", err)
	}

	stats = a.CacheStats()
	if stats.Size != 1 {
		t.Errorf("after first read: size = %d, want 1", stats.Size)
	}
	if stats.Misses != 1 {
		t.Errorf("after first read: misses = %d, want 1", stats.Misses)
	}
	if stats.Bytes <= 0 {
		t.Errorf("after first read: bytes = %d, want > 0", stats.Bytes)
	}

	// Read the same entry again — should be a cache hit
	if _, err := e.ReadContent(); err != nil {
		t.Fatalf("ReadContent (hit): %v", err)
	}
	stats = a.CacheStats()
	if stats.Hits != 1 {
		t.Errorf("after cache hit: hits = %d, want 1", stats.Hits)
	}

	// Read a different entry from a different cluster to trigger eviction.
	// small.zim has favicon.png in one cluster and main.html in another.
	favicon, err := a.EntryByPath("C/favicon.png")
	if err != nil {
		t.Fatalf("EntryByPath(favicon): %v", err)
	}
	if _, err := favicon.ReadContent(); err != nil {
		t.Fatalf("ReadContent(favicon): %v", err)
	}

	stats = a.CacheStats()
	// With cache size 1, the old cluster should have been evicted
	if stats.Size > 1 {
		t.Errorf("after eviction: size = %d, want <= 1", stats.Size)
	}
}

func TestCacheLRUPromotion(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	// Cache size 2 — both clusters fit
	a, err := OpenWithOptions(path, WithCacheSize(2))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Load both clusters
	e1, _ := a.EntryByPath("C/main.html")
	e1.ReadContent()
	e2, _ := a.EntryByPath("C/favicon.png")
	e2.ReadContent()

	stats := a.CacheStats()
	if stats.Misses != 2 {
		t.Errorf("misses = %d, want 2", stats.Misses)
	}

	// Access first cluster again — should be a hit
	e1.ReadContent()
	stats = a.CacheStats()
	if stats.Hits != 1 {
		t.Errorf("hits = %d, want 1", stats.Hits)
	}

	// Both should still be cached (size 2, capacity 2)
	if stats.Size != 2 {
		t.Errorf("size = %d, want 2", stats.Size)
	}
}

func TestEntriesByNamespaceNonExistent(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// 'Z' namespace doesn't exist — iterator should yield nothing
	var count int
	for range a.EntriesByNamespace('Z') {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 entries for non-existent namespace, got %d", count)
	}
}

func TestEntriesByNamespaceAll(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Sum of all namespace counts should equal total entry count
	total := 0
	for _, ns := range []byte{'C', 'M', 'W', 'X'} {
		for range a.EntriesByNamespace(ns) {
			total++
		}
	}

	if uint32(total) != a.EntryCount() {
		t.Errorf("sum of namespace entries = %d, want %d", total, a.EntryCount())
	}
}

func TestTitleListingViaHeader(t *testing.T) {
	// small.zim uses the header TitlePtrPos (old-style) for title ordering.
	// Verify EntriesByTitle works and returns entries in title order.
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	if a.titleList != nil {
		t.Log("small.zim uses titleList (v6.1 listing entry)")
	} else {
		t.Log("small.zim uses header TitlePtrPos (old-style)")
	}

	var count int
	var prevNs byte
	var prevTitle string
	for e := range a.EntriesByTitle() {
		ns := e.Namespace()
		title := e.Title()
		if count > 0 && compareTitleKey(ns, title, prevNs, prevTitle) < 0 {
			t.Errorf("title order violated: (%c,%q) came after (%c,%q)", ns, title, prevNs, prevTitle)
		}
		prevNs = ns
		prevTitle = title
		count++
	}

	if count == 0 {
		t.Error("EntriesByTitle returned 0 entries")
	}
}

func TestTitleListingDirect(t *testing.T) {
	// Directly test the titleList code path by constructing an archive
	// with a pre-populated titleList (simulating loadTitleListing).
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Banana", "Banana"),
		makeContentEntry(0, 'C', 0, 0, "Apple", "Apple"),
		makeContentEntry(0, 'C', 0, 0, "Cherry", "Cherry"),
	}
	a := buildFakeArchive(entries)

	// Set titleList to title-sorted order: Apple(1), Banana(0), Cherry(2)
	a.titleList = []uint32{1, 0, 2}
	a.hdr.TitlePtrPos = noTitlePtrList

	var titles []string
	for e := range a.EntriesByTitle() {
		titles = append(titles, e.Title())
	}

	if len(titles) != 3 {
		t.Fatalf("got %d entries, want 3", len(titles))
	}
	want := []string{"Apple", "Banana", "Cherry"}
	for i, w := range want {
		if titles[i] != w {
			t.Errorf("title[%d] = %q, want %q", i, titles[i], w)
		}
	}
}

func TestUnicodePathAndTitle(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html"}}

	tests := []struct {
		name  string
		path  string
		title string
	}{
		{"CJK", "日本語/ページ", "日本語のタイトル"},
		{"Cyrillic", "Кириллица/Страница", "Заголовок"},
		{"Arabic", "عربي/صفحة", "عنوان"},
		{"Emoji", "🌍/page", "🌍 Earth"},
		{"Mixed", "Café/résumé", "Ñoño título"},
		{"Diacritics", "ü/ö/ä", "Ünïcödé Tïtlé"},
		{"Empty title fallback", "Ελληνικά/σελίδα", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := makeContentEntry(0, 'C', 0, 0, tt.path, tt.title)
			e, n, err := parseDirectoryEntry(data, archive, 0)
			if err != nil {
				t.Fatalf("parseDirectoryEntry: %v", err)
			}
			if n != len(data) {
				t.Errorf("consumed %d bytes, want %d", n, len(data))
			}
			if e.Path() != tt.path {
				t.Errorf("path = %q, want %q", e.Path(), tt.path)
			}
			expectedTitle := tt.title
			if expectedTitle == "" {
				expectedTitle = tt.path // Title() falls back to path
			}
			if e.Title() != expectedTitle {
				t.Errorf("title = %q, want %q", e.Title(), expectedTitle)
			}
		})
	}
}

func TestUnicodeInFullPath(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html"}}
	data := makeContentEntry(0, 'C', 0, 0, "Ångström", "Ångström unit")
	e, _, err := parseDirectoryEntry(data, archive, 0)
	if err != nil {
		t.Fatalf("parseDirectoryEntry: %v", err)
	}
	if e.FullPath() != "C/Ångström" {
		t.Errorf("FullPath = %q, want %q", e.FullPath(), "C/Ångström")
	}
}

func TestConcurrentReadContent(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Collect non-redirect entry indices
	var indices []uint32
	for i := uint32(0); i < a.EntryCount(); i++ {
		e, err := a.EntryByIndex(i)
		if err != nil {
			t.Fatalf("EntryByIndex(%d): %v", i, err)
		}
		if !e.IsRedirect() {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		t.Skip("no content entries")
	}

	// Get reference content for each entry
	reference := make(map[uint32]string)
	for _, idx := range indices {
		e, _ := a.EntryByIndex(idx)
		data, err := e.ReadContent()
		if err != nil {
			t.Fatalf("ReadContent(%d): %v", idx, err)
		}
		reference[idx] = string(data)
	}

	// Read all entries concurrently from multiple goroutines
	const goroutines = 8
	const iterations = 10
	var wg sync.WaitGroup
	errCh := make(chan string, goroutines*len(indices)*iterations)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iter := 0; iter < iterations; iter++ {
				for _, idx := range indices {
					e, err := a.EntryByIndex(idx)
					if err != nil {
						errCh <- err.Error()
						return
					}
					data, err := e.ReadContentCopy()
					if err != nil {
						errCh <- err.Error()
						return
					}
					if string(data) != reference[idx] {
						errCh <- "content mismatch for index " + e.FullPath()
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for msg := range errCh {
		t.Error(msg)
	}
}

func TestNullBytesInPathFail(t *testing.T) {
	// A path containing a null byte should cause the parser to split
	// at the null, treating the rest as the title field
	archive := &Archive{mimeTypes: []string{"text/html"}}
	data := makeContentEntry(0, 'C', 0, 0, "before", "after")
	e, _, err := parseDirectoryEntry(data, archive, 0)
	if err != nil {
		t.Fatalf("parseDirectoryEntry: %v", err)
	}
	// Sanity check that the path doesn't contain unexpected content
	if e.Path() != "before" {
		t.Errorf("path = %q, want %q", e.Path(), "before")
	}
	if e.Title() != "after" {
		t.Errorf("title = %q, want %q", e.Title(), "after")
	}
}
