package zim

import (
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
)

func makeContentEntry(mimeIdx uint16, ns byte, cluster, blob uint32, path, title string) []byte {
	le := binary.LittleEndian
	buf := make([]byte, 16)
	le.PutUint16(buf[0:2], mimeIdx)
	buf[2] = 0 // param len
	buf[3] = ns
	le.PutUint32(buf[4:8], 0) // revision
	le.PutUint32(buf[8:12], cluster)
	le.PutUint32(buf[12:16], blob)
	buf = append(buf, []byte(path)...)
	buf = append(buf, 0) // null terminator
	buf = append(buf, []byte(title)...)
	buf = append(buf, 0) // null terminator
	return buf
}

func makeRedirectEntry(ns byte, redirectIdx uint32, path, title string) []byte {
	le := binary.LittleEndian
	buf := make([]byte, 12)
	le.PutUint16(buf[0:2], mimeRedirect)
	buf[2] = 0 // param len
	buf[3] = ns
	le.PutUint32(buf[4:8], 0) // revision
	le.PutUint32(buf[8:12], redirectIdx)
	buf = append(buf, []byte(path)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(title)...)
	buf = append(buf, 0)
	return buf
}

func TestParseContentEntry(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html", "image/png"}}
	data := makeContentEntry(0, 'C', 5, 3, "Main_Page", "Main Page")

	e, n, err := parseDirectoryEntry(data, archive, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("consumed %d bytes, want %d", n, len(data))
	}
	if e.IsRedirect() {
		t.Error("content entry reported as redirect")
	}
	if e.Path() != "Main_Page" {
		t.Errorf("path = %q, want %q", e.Path(), "Main_Page")
	}
	if e.Title() != "Main Page" {
		t.Errorf("title = %q, want %q", e.Title(), "Main Page")
	}
	if e.FullPath() != "C/Main_Page" {
		t.Errorf("full path = %q, want %q", e.FullPath(), "C/Main_Page")
	}
	if e.Namespace() != 'C' {
		t.Errorf("namespace = %c, want C", e.Namespace())
	}
	if e.MIMEType() != "text/html" {
		t.Errorf("mime = %q, want %q", e.MIMEType(), "text/html")
	}
	if e.clusterNum != 5 {
		t.Errorf("cluster = %d, want 5", e.clusterNum)
	}
	if e.blobNum != 3 {
		t.Errorf("blob = %d, want 3", e.blobNum)
	}
}

func TestParseRedirectEntry(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html"}}
	data := makeRedirectEntry('C', 42, "Old_Page", "Old Page")

	e, _, err := parseDirectoryEntry(data, archive, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !e.IsRedirect() {
		t.Error("redirect entry not reported as redirect")
	}
	if e.redirectIdx != 42 {
		t.Errorf("redirect index = %d, want 42", e.redirectIdx)
	}
	if e.MIMEType() != "" {
		t.Errorf("redirect mime type = %q, want empty", e.MIMEType())
	}
}

func TestEntryEmptyTitle(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html"}}
	data := makeContentEntry(0, 'C', 0, 0, "Some_Page", "")

	e, _, err := parseDirectoryEntry(data, archive, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Title() should fall back to path when title is empty
	if e.Title() != "Some_Page" {
		t.Errorf("title = %q, want %q (fallback to path)", e.Title(), "Some_Page")
	}
}

func TestParseEntryTooShort(t *testing.T) {
	_, _, err := parseDirectoryEntry([]byte{0, 0, 0}, nil, 0)
	if !errors.Is(err, ErrInvalidEntry) {
		t.Errorf("expected ErrInvalidEntry, got %v", err)
	}
}

func TestEntryItemOnRedirect(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html"}}
	data := makeRedirectEntry('C', 0, "Redirect", "")

	e, _, err := parseDirectoryEntry(data, archive, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = e.Item()
	if !errors.Is(err, ErrIsRedirect) {
		t.Errorf("expected ErrIsRedirect, got %v", err)
	}
}

func TestEntryRedirectTargetOnContent(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html"}}
	data := makeContentEntry(0, 'C', 0, 0, "Page", "")

	e, _, err := parseDirectoryEntry(data, archive, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = e.RedirectTarget()
	if !errors.Is(err, ErrNotRedirect) {
		t.Errorf("expected ErrNotRedirect, got %v", err)
	}
}

func TestParseLongPathAndTitle(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html"}}

	// Simulate the bug scenario: 255-char path + 245-char title
	// Fixed header for content entry = 16 bytes
	// path (255) + null (1) + title (245) + null (1) = 518 total bytes
	pathBytes := make([]byte, 255)
	titleBytes := make([]byte, 245)
	for i := range pathBytes {
		pathBytes[i] = 'a' + byte(i%26)
	}
	for i := range titleBytes {
		titleBytes[i] = 'A' + byte(i%26)
	}
	path := string(pathBytes)
	title := string(titleBytes)

	data := makeContentEntry(0, 'C', 1, 0, path, title)

	// This would fail with a 512-byte buffer (data is 518 bytes)
	if len(data) <= 512 {
		t.Fatalf("test data should exceed 512 bytes, got %d", len(data))
	}

	e, n, err := parseDirectoryEntry(data, archive, 0)
	if err != nil {
		t.Fatalf("unexpected error parsing long entry (%d bytes): %v", len(data), err)
	}
	if n != len(data) {
		t.Errorf("consumed %d bytes, want %d", n, len(data))
	}
	if e.Path() != path {
		t.Errorf("path length = %d, want %d", len(e.Path()), len(path))
	}
	if e.Title() != title {
		t.Errorf("title length = %d, want %d", len(e.Title()), len(title))
	}
}

func TestParseTruncatedLongEntry(t *testing.T) {
	archive := &Archive{mimeTypes: []string{"text/html"}}

	// Create an entry with long path+title, then truncate the buffer
	// to simulate what happened with the 512-byte buffer
	pathBytes := make([]byte, 255)
	titleBytes := make([]byte, 245)
	for i := range pathBytes {
		pathBytes[i] = 'x'
	}
	for i := range titleBytes {
		titleBytes[i] = 'y'
	}

	data := makeContentEntry(0, 'C', 1, 0, string(pathBytes), string(titleBytes))

	// Truncate to 512 bytes — should fail because title's null terminator is cut off
	truncated := data[:512]
	_, _, err := parseDirectoryEntry(truncated, archive, 0)
	if err == nil {
		t.Error("expected error when parsing truncated entry, got nil")
	}
}

func TestResolveRedirectChain(t *testing.T) {
	// Build a fake archive with: entry 0 -> redirect to 1 -> redirect to 2 -> content
	entries := [][]byte{
		makeRedirectEntry('C', 1, "Alias1", ""),
		makeRedirectEntry('C', 2, "Alias2", ""),
		makeContentEntry(0, 'C', 0, 0, "RealPage", "Real Page"),
	}
	a := buildFakeArchive(entries)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex(0): %v", err)
	}

	resolved, err := e.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.IsRedirect() {
		t.Error("resolved entry is still a redirect")
	}
	if resolved.Path() != "RealPage" {
		t.Errorf("resolved path = %q, want %q", resolved.Path(), "RealPage")
	}
}

func TestResolveDeepRedirectChain(t *testing.T) {
	// 10-entry redirect chain: 0->1->2->...->9 (content)
	n := 10
	entries := make([][]byte, n)
	for i := range n - 1 {
		entries[i] = makeRedirectEntry('C', uint32(i+1), fmt.Sprintf("Redirect%d", i), "")
	}
	entries[n-1] = makeContentEntry(0, 'C', 0, 0, "FinalPage", "Final")
	a := buildFakeArchive(entries)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex(0): %v", err)
	}

	resolved, err := e.Resolve()
	if err != nil {
		t.Fatalf("Resolve 10-deep chain: %v", err)
	}
	if resolved.Path() != "FinalPage" {
		t.Errorf("resolved path = %q, want %q", resolved.Path(), "FinalPage")
	}
}

func TestResolveRedirectLoop(t *testing.T) {
	// entry 0 -> 1 -> 0 (cycle)
	entries := [][]byte{
		makeRedirectEntry('C', 1, "LoopA", ""),
		makeRedirectEntry('C', 0, "LoopB", ""),
	}
	a := buildFakeArchive(entries)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex(0): %v", err)
	}

	_, err = e.Resolve()
	if !errors.Is(err, ErrRedirectLoop) {
		t.Errorf("expected ErrRedirectLoop, got: %v", err)
	}
}

func TestResolveSelfRedirect(t *testing.T) {
	// entry 0 -> 0 (self-loop)
	entries := [][]byte{
		makeRedirectEntry('C', 0, "SelfLoop", ""),
	}
	a := buildFakeArchive(entries)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex(0): %v", err)
	}

	_, err = e.Resolve()
	if !errors.Is(err, ErrRedirectLoop) {
		t.Errorf("expected ErrRedirectLoop for self-redirect, got: %v", err)
	}
}

func TestResolveRedirectToBrokenTarget(t *testing.T) {
	// entry 0 -> 1, but entry 1 is corrupt
	entries := [][]byte{
		makeRedirectEntry('C', 1, "GoodRedirect", ""),
		nil, // corrupt target
	}
	a := buildFakeArchive(entries)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex(0): %v", err)
	}

	_, err = e.Resolve()
	if err == nil {
		t.Error("expected error resolving redirect to corrupt entry, got nil")
	}
}

func TestResolveNonRedirect(t *testing.T) {
	// Resolve on a content entry should return itself
	entries := [][]byte{
		makeContentEntry(0, 'C', 0, 0, "Content", "Content Page"),
	}
	a := buildFakeArchive(entries)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex(0): %v", err)
	}

	resolved, err := e.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Path() != "Content" {
		t.Errorf("resolved path = %q, want %q", resolved.Path(), "Content")
	}
}

func TestRedirectTargetFollowsChain(t *testing.T) {
	// RedirectTarget should return the immediate target, not the final one
	entries := [][]byte{
		makeRedirectEntry('C', 1, "First", ""),
		makeRedirectEntry('C', 2, "Second", ""),
		makeContentEntry(0, 'C', 0, 0, "Third", ""),
	}
	a := buildFakeArchive(entries)

	e, err := a.EntryByIndex(0)
	if err != nil {
		t.Fatalf("EntryByIndex(0): %v", err)
	}

	target, err := e.RedirectTarget()
	if err != nil {
		t.Fatalf("RedirectTarget: %v", err)
	}
	if target.Path() != "Second" {
		t.Errorf("immediate target path = %q, want %q", target.Path(), "Second")
	}
	if !target.IsRedirect() {
		t.Error("immediate target should still be a redirect")
	}
}
