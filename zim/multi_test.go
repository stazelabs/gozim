package zim

import (
	"os"
	"path/filepath"
	"testing"
)

// splitTestZIMDir returns the path to the first split part and skips
// the test if the split test files are not present.
func splitTestZIMPath(t *testing.T) string {
	t.Helper()
	path := testdataPath("small_split.zimaa")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("split test ZIM not found: run zimsplit on testdata/small.zim")
	}
	return path
}

func TestIsSplitZIM(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"foo.zimaa", true},
		{"foo.zimzz", true},
		{"foo.zimab", true},
		{"/path/to/archive.zimaa", true},
		{"foo.zim", false},
		{"foo.zimAA", false},
		{"foo.zima", false},   // only one letter
		{"foo.zimaaa", false}, // three letters
		{"foo.txt", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isSplitZIM(tt.path); got != tt.want {
			t.Errorf("isSplitZIM(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestDiscoverParts(t *testing.T) {
	path := splitTestZIMPath(t)
	parts, err := discoverParts(path)
	if err != nil {
		t.Fatalf("discoverParts: %v", err)
	}
	if len(parts) < 2 {
		t.Fatalf("expected at least 2 parts, got %d", len(parts))
	}
	t.Logf("discovered %d parts", len(parts))
	for _, p := range parts {
		t.Logf("  %s", filepath.Base(p))
	}
}

func TestDiscoverPartsInvalidSuffix(t *testing.T) {
	_, err := discoverParts("/tmp/foo.zim")
	if err == nil {
		t.Fatal("expected error for non-split suffix")
	}
}

func TestOpenSplitZIM(t *testing.T) {
	path := splitTestZIMPath(t)

	splitArchive, err := Open(path)
	if err != nil {
		t.Fatalf("Open split: %v", err)
	}
	defer splitArchive.Close()

	// Also open the original for comparison.
	origPath := testdataPath("small.zim")
	skipIfNoTestdata(t, origPath)
	origArchive, err := Open(origPath)
	if err != nil {
		t.Fatalf("Open original: %v", err)
	}
	defer origArchive.Close()

	// Compare basic properties.
	if splitArchive.EntryCount() != origArchive.EntryCount() {
		t.Errorf("entry count: split=%d, orig=%d", splitArchive.EntryCount(), origArchive.EntryCount())
	}
	if splitArchive.ClusterCount() != origArchive.ClusterCount() {
		t.Errorf("cluster count: split=%d, orig=%d", splitArchive.ClusterCount(), origArchive.ClusterCount())
	}
	if splitArchive.UUID() != origArchive.UUID() {
		t.Errorf("UUID mismatch: split=%x, orig=%x", splitArchive.UUID(), origArchive.UUID())
	}
	if splitArchive.MajorVersion() != origArchive.MajorVersion() {
		t.Errorf("major version: split=%d, orig=%d", splitArchive.MajorVersion(), origArchive.MajorVersion())
	}

	t.Logf("split archive: %d entries, %d clusters", splitArchive.EntryCount(), splitArchive.ClusterCount())
}

func TestSplitZIMReadContent(t *testing.T) {
	path := splitTestZIMPath(t)

	splitArchive, err := Open(path)
	if err != nil {
		t.Fatalf("Open split: %v", err)
	}
	defer splitArchive.Close()

	origPath := testdataPath("small.zim")
	skipIfNoTestdata(t, origPath)
	origArchive, err := Open(origPath)
	if err != nil {
		t.Fatalf("Open original: %v", err)
	}
	defer origArchive.Close()

	// Compare content of every entry.
	for i := range origArchive.EntryCount() {
		origEntry, err := origArchive.EntryByIndex(i)
		if err != nil {
			t.Fatalf("orig entry %d: %v", i, err)
		}
		splitEntry, err := splitArchive.EntryByIndex(i)
		if err != nil {
			t.Fatalf("split entry %d: %v", i, err)
		}

		if origEntry.FullPath() != splitEntry.FullPath() {
			t.Errorf("entry %d path: orig=%q, split=%q", i, origEntry.FullPath(), splitEntry.FullPath())
		}

		if origEntry.IsRedirect() {
			continue
		}

		origData, err := origEntry.ReadContent()
		if err != nil {
			t.Fatalf("orig read %d (%s): %v", i, origEntry.FullPath(), err)
		}
		splitData, err := splitEntry.ReadContent()
		if err != nil {
			t.Fatalf("split read %d (%s): %v", i, splitEntry.FullPath(), err)
		}

		if len(origData) != len(splitData) {
			t.Errorf("entry %d (%s) content length: orig=%d, split=%d",
				i, origEntry.FullPath(), len(origData), len(splitData))
		} else {
			for j := range origData {
				if origData[j] != splitData[j] {
					t.Errorf("entry %d (%s) content differs at byte %d", i, origEntry.FullPath(), j)
					break
				}
			}
		}
	}
}

func TestSplitZIMWithPread(t *testing.T) {
	path := splitTestZIMPath(t)

	a, err := OpenWithOptions(path, WithMmap(false))
	if err != nil {
		t.Fatalf("Open split (pread): %v", err)
	}
	defer a.Close()

	if a.EntryCount() == 0 {
		t.Error("expected non-zero entry count with pread reader")
	}

	// Read at least one content entry to verify pread path works.
	if a.HasMainEntry() {
		main, err := a.MainEntry()
		if err != nil {
			t.Fatalf("MainEntry: %v", err)
		}
		if !main.IsRedirect() {
			data, err := main.ReadContent()
			if err != nil {
				t.Fatalf("ReadContent: %v", err)
			}
			if len(data) == 0 {
				t.Error("main entry content is empty")
			}
		}
	}
}

func TestMultiReaderReadAtBoundary(t *testing.T) {
	// Create two temp files and read across their boundary.
	dir := t.TempDir()
	f1 := filepath.Join(dir, "test.zimaa")
	f2 := filepath.Join(dir, "test.zimab")

	data1 := []byte("AAAA") // 4 bytes
	data2 := []byte("BBBB") // 4 bytes

	if err := os.WriteFile(f1, data1, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, data2, 0o644); err != nil {
		t.Fatal(err)
	}

	mr, err := newMultiReader([]string{f1, f2}, false)
	if err != nil {
		t.Fatalf("newMultiReader: %v", err)
	}
	defer mr.Close()

	if mr.Size() != 8 {
		t.Fatalf("Size = %d, want 8", mr.Size())
	}

	// Read across boundary: offset 2, length 4 → "AABB"
	buf := make([]byte, 4)
	n, err := mr.ReadAt(buf, 2)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if n != 4 {
		t.Fatalf("ReadAt n = %d, want 4", n)
	}
	if string(buf) != "AABB" {
		t.Errorf("ReadAt = %q, want %q", string(buf), "AABB")
	}

	// Read entire content.
	all := make([]byte, 8)
	if _, err = mr.ReadAt(all, 0); err != nil {
		t.Fatalf("ReadAt full: %v", err)
	}
	if string(all) != "AAAABBBB" {
		t.Errorf("ReadAt full = %q, want %q", string(all), "AAAABBBB")
	}
}
