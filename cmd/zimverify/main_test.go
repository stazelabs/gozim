package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
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

func TestRun(t *testing.T) {
	path := testZIM(t)

	cmd := &cobra.Command{Use: "test", RunE: run, Args: cobra.MinimumNArgs(1)}
	cmd.SetArgs([]string{path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
}
