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

func TestRunList(t *testing.T) {
	path := testZIM(t)
	if err := run([]string{path}, true, false); err != nil {
		t.Fatalf("run --list failed: %v", err)
	}
}

func TestRunMeta(t *testing.T) {
	path := testZIM(t)
	if err := run([]string{path}, false, true); err != nil {
		t.Fatalf("run --meta failed: %v", err)
	}
}

func TestRunExtract(t *testing.T) {
	path := testZIM(t)
	if err := run([]string{path, "C/main.html"}, false, false); err != nil {
		t.Fatalf("run extract failed: %v", err)
	}
}

func TestRunNoArgs(t *testing.T) {
	path := testZIM(t)
	err := run([]string{path}, false, false)
	if err == nil {
		t.Fatal("expected error when no path and no flags given")
	}
}
