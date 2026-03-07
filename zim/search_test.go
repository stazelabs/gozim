package zim

import (
	"testing"
)

func TestEntriesByTitlePrefix(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// "Test ZIM file" is the title of C/main.html — prefix "Test" must match it.
	var found bool
	for e := range a.EntriesByTitlePrefix('C', "Test") {
		if e.Title() == "Test ZIM file" && e.Path() == "main.html" {
			found = true
		}
	}
	if !found {
		t.Error("EntriesByTitlePrefix('C', 'Test') did not find 'Test ZIM file'")
	}
}

func TestEntriesByTitlePrefixEmpty(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Empty prefix should iterate all entries in namespace C.
	var count int
	for range a.EntriesByTitlePrefix('C', "") {
		count++
	}
	if count == 0 {
		t.Error("EntriesByTitlePrefix with empty prefix returned no entries")
	}
}

func TestEntriesByTitlePrefixNoMatch(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	var count int
	for range a.EntriesByTitlePrefix('C', "ZZZZZZ_no_match_possible") {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 results for no-match prefix, got %d", count)
	}
}

func TestEntriesByTitlePrefixSorted(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// All results must be sorted and in the correct namespace.
	var prev string
	var count int
	for e := range a.EntriesByTitlePrefix('C', "") {
		if e.Namespace() != 'C' {
			t.Errorf("got entry in namespace %c, expected C", e.Namespace())
		}
		if count > 0 && e.Title() < prev {
			t.Errorf("titles not sorted: %q came after %q", e.Title(), prev)
		}
		prev = e.Title()
		count++
	}
}

func TestEntriesByTitlePrefixFold(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// "test" (lowercase) should fold-match "Test ZIM file".
	var found bool
	for e := range a.EntriesByTitlePrefixFold('C', "test") {
		if e.Title() == "Test ZIM file" {
			found = true
		}
	}
	if !found {
		t.Error("EntriesByTitlePrefixFold('C', 'test') did not find 'Test ZIM file'")
	}
}

func TestHasFulltextIndex(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// small.zim is a minimal test file — we just verify it doesn't panic.
	// We don't assert a specific value since the test file may or may not have an index.
	_ = a.HasFulltextIndex()
	_ = a.HasTitleIndex()
}
