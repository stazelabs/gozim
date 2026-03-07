package zim

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
