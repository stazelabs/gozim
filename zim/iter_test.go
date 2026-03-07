package zim

import (
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
