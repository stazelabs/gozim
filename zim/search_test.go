package zim

import (
	"strings"
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

func TestTitlePrefixCountBasic(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Empty prefix for namespace C should equal the total C entries
	cCount := a.TitlePrefixCount('C', "")
	cExpected := a.EntryCountByNamespace('C')
	if cCount != cExpected {
		t.Errorf("TitlePrefixCount('C', \"\") = %d, want %d", cCount, cExpected)
	}

	// "Test" should match "Test ZIM file" (main.html)
	testCount := a.TitlePrefixCount('C', "Test")
	if testCount < 1 {
		t.Errorf("TitlePrefixCount('C', 'Test') = %d, want >= 1", testCount)
	}

	// No-match prefix
	zeroCount := a.TitlePrefixCount('C', "ZZZZZZ_impossible_prefix")
	if zeroCount != 0 {
		t.Errorf("TitlePrefixCount('C', no-match) = %d, want 0", zeroCount)
	}
}

func TestTitlePrefixCountNonExistentNamespace(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	count := a.TitlePrefixCount('Z', "anything")
	if count != 0 {
		t.Errorf("TitlePrefixCount('Z', ...) = %d, want 0", count)
	}
}

func TestTitlePrefixCountAllFF(t *testing.T) {
	// Build a fake archive with entries and test the all-0xFF prefix path.
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Alpha", "Alpha"),
		makeContentEntry(0, 'C', 0, 0, "Beta", "Beta"),
	}
	a := buildFakeArchive(entries)
	// Use old-style title pointers: just point each title index to itself.
	// We need TitlePtrPos set to a valid location with uint32 pointers.
	// For simplicity, use the titleList path.
	a.titleList = []uint32{0, 1}
	a.hdr.TitlePtrPos = noTitlePtrList

	// All-0xFF prefix — triggers the special "all bytes are 0xFF" branch
	allFF := string([]byte{0xFF, 0xFF, 0xFF})
	count := a.TitlePrefixCount('C', allFF)
	// No title starts with 0xFF bytes, so count should be 0
	if count != 0 {
		t.Errorf("TitlePrefixCount with all-0xFF prefix = %d, want 0", count)
	}
}

func TestTitlePrefixCountConsistentWithIterator(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	// Count via TitlePrefixCount should match count from EntriesByTitlePrefix
	for _, ns := range []byte{'C', 'M'} {
		for _, prefix := range []string{"", "T", "La"} {
			countAPI := a.TitlePrefixCount(ns, prefix)
			var countIter int
			for range a.EntriesByTitlePrefix(ns, prefix) {
				countIter++
			}
			if countAPI != countIter {
				t.Errorf("TitlePrefixCount(%c, %q) = %d, iterator count = %d",
					ns, prefix, countAPI, countIter)
			}
		}
	}
}

func TestHasPrefixFold(t *testing.T) {
	tests := []struct {
		s, prefix string
		want      bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "HELLO", true},
		{"Hello World", "hello world", true},
		{"Hello World", "hello world!", false},
		{"Ångström", "ångström", true},
		{"ÅNGSTRÖM", "ångström", true},
		{"café", "café", true},
		{"CAFÉ", "café", true},
		{"test", "testing", false},
		{"", "", true},
		{"anything", "", true},
		{"", "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.prefix, func(t *testing.T) {
			// hasPrefixFold expects prefix to be pre-lowered
			got := hasPrefixFold(tt.s, strings.ToLower(tt.prefix))
			if got != tt.want {
				t.Errorf("hasPrefixFold(%q, %q) = %v, want %v", tt.s, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestEntriesByTitlePrefixFoldUnicode(t *testing.T) {
	// Build a fake archive with Unicode titles
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "page1", "Ångström"),
		makeContentEntry(0, 'C', 0, 0, "page2", "ångström unit"),
		makeContentEntry(0, 'C', 0, 0, "page3", "ÅNGSTRÖM LIMIT"),
		makeContentEntry(0, 'C', 0, 0, "page4", "Banana"),
	}
	a := buildFakeArchive(entries)
	a.titleList = []uint32{0, 1, 2, 3}
	a.hdr.TitlePtrPos = noTitlePtrList

	var found []string
	for e := range a.EntriesByTitlePrefixFold('C', "ångström") {
		found = append(found, e.Title())
	}

	if len(found) != 3 {
		t.Fatalf("expected 3 matches, got %d: %v", len(found), found)
	}
}
