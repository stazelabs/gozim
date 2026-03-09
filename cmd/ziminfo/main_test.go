package main

import (
	"bytes"
	"encoding/json"
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

	cmd := &cobra.Command{Use: "test", RunE: run, Args: cobra.ExactArgs(1)}
	cmd.SetArgs([]string{path})
	cmd.SetOut(os.Stdout)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
}

func TestRunJSON(t *testing.T) {
	path := testZIM(t)

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	jsonOutput = true
	t.Cleanup(func() { jsonOutput = false })

	cmd := &cobra.Command{Use: "test", RunE: run, Args: cobra.ExactArgs(1)}
	cmd.SetArgs([]string{path})
	if err := cmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("run failed: %v", err)
	}
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var info zimInfo
	if err := json.Unmarshal(buf.Bytes(), &info); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}

	if info.File != path {
		t.Errorf("file = %q, want %q", info.File, path)
	}
	if info.UUID == "" {
		t.Error("uuid is empty")
	}
	if info.EntryCount == 0 {
		t.Error("entryCount is 0")
	}
	if len(info.MIMETypes) == 0 {
		t.Error("mimeTypes is empty")
	}
	if len(info.Namespaces) == 0 {
		t.Error("namespaces is empty")
	}
}
