package zim

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEntries(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	var count int
	var prevPath string
	for e := range a.Entries() {
		fp := e.FullPath()
		if fp <= prevPath && count > 0 {
			t.Errorf("entries not sorted: %q came after %q", fp, prevPath)
		}
		prevPath = fp
		count++
	}

	if uint32(count) != a.EntryCount() {
		t.Errorf("iterated %d entries, want %d", count, a.EntryCount())
	}
}

func TestEntriesByNamespace(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	var mCount int
	for e := range a.EntriesByNamespace('M') {
		if e.Namespace() != 'M' {
			t.Errorf("expected namespace M, got %c", e.Namespace())
		}
		mCount++
	}
	t.Logf("M namespace entries: %d", mCount)

	if mCount == 0 {
		t.Error("expected at least one M namespace entry")
	}
}

func TestEntriesEarlyBreak(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	var count int
	for range a.Entries() {
		count++
		if count >= 3 {
			break
		}
	}

	if count != 3 {
		t.Errorf("expected 3 entries with early break, got %d", count)
	}
}

// bytesReader wraps a byte slice to implement the reader interface for testing.
type bytesReader struct {
	data []byte
}

func (r *bytesReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, nil
	}
	n := copy(p, r.data[off:])
	return n, nil
}

func (r *bytesReader) Size() int64    { return int64(len(r.data)) }
func (r *bytesReader) Close() error   { return nil }

// buildFakeArchive constructs a minimal in-memory Archive with the given
// directory entry blobs. Entries whose data is nil will have their URL pointer
// set to an offset containing garbage bytes, causing parseDirectoryEntry to fail.
func buildFakeArchive(entries [][]byte) *Archive {
	le := binary.LittleEndian

	// Layout: [URL pointer table] [entry data...]
	const ptrSize = 8
	numEntries := len(entries)
	ptrTableSize := numEntries * ptrSize

	var buf bytes.Buffer

	// Reserve space for pointer table
	ptrTable := make([]byte, ptrTableSize)
	buf.Write(ptrTable)

	// Write each entry and record its offset; nil entries get garbage
	offsets := make([]int64, numEntries)
	for i, data := range entries {
		offsets[i] = int64(buf.Len())
		if data == nil {
			// Write garbage that will fail to parse (too short, no null terminators)
			buf.Write([]byte{0xFF, 0xFF, 0xFF})
		} else {
			buf.Write(data)
		}
	}

	// Fill in the pointer table
	raw := buf.Bytes()
	for i, off := range offsets {
		le.PutUint64(raw[i*ptrSize:(i+1)*ptrSize], uint64(off))
	}

	return &Archive{
		r:         &bytesReader{data: raw},
		hdr:       header{EntryCount: uint32(numEntries), URLPtrPos: 0},
		mimeTypes: []string{"text/html"},
	}
}

func TestEntriesSkipsBadEntries(t *testing.T) {
	// 3 entries: good, bad, good — iterator should yield 2, not stop at 1
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "PageA", "Page A"),
		nil, // corrupt entry
		makeContentEntry(0, 'C', 0, 0, "PageC", "Page C"),
	}
	a := buildFakeArchive(entries)

	var paths []string
	for e := range a.Entries() {
		paths = append(paths, e.Path())
	}

	if len(paths) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(paths), paths)
	}
	if paths[0] != "PageA" {
		t.Errorf("first entry path = %q, want %q", paths[0], "PageA")
	}
	if paths[1] != "PageC" {
		t.Errorf("second entry path = %q, want %q", paths[1], "PageC")
	}
}

func TestEntriesSkipsBadEntryAtStart(t *testing.T) {
	// Bad entry first — should still yield the remaining entries
	entries := [][]byte{
		nil, // corrupt
		makeContentEntry(0, 'C', 0, 0, "PageB", "Page B"),
	}
	a := buildFakeArchive(entries)

	var count int
	for range a.Entries() {
		count++
	}

	if count != 1 {
		t.Errorf("expected 1 entry, got %d", count)
	}
}
