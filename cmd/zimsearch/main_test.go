package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testZIM(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "small.zim")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		t.Skipf("test ZIM not found: %s", p)
	}
	return p
}

func TestRunSearch(t *testing.T) {
	path := testZIM(t)
	if err := run(path, "main", 'C', 10, false); err != nil {
		t.Fatalf("search failed: %v", err)
	}
}

func TestRunSearchInsensitive(t *testing.T) {
	path := testZIM(t)
	if err := run(path, "Main", 'C', 10, true); err != nil {
		t.Fatalf("case-insensitive search failed: %v", err)
	}
}
