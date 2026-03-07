package zim

import (
	"encoding/binary"
	"errors"
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
