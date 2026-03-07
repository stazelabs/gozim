package zim

import (
	"testing"
)

func TestParseMIMEList(t *testing.T) {
	// "text/html\0image/png\0application/javascript\0\0"
	data := []byte("text/html\x00image/png\x00application/javascript\x00\x00")
	types := parseMIMEList(data)

	want := []string{"text/html", "image/png", "application/javascript"}
	if len(types) != len(want) {
		t.Fatalf("got %d types, want %d", len(types), len(want))
	}
	for i, w := range want {
		if types[i] != w {
			t.Errorf("types[%d] = %q, want %q", i, types[i], w)
		}
	}
}

func TestParseMIMEListEmpty(t *testing.T) {
	// Just a terminating null byte
	data := []byte{0}
	types := parseMIMEList(data)
	if len(types) != 0 {
		t.Errorf("got %d types, want 0", len(types))
	}
}

func TestParseMIMEListSingleEntry(t *testing.T) {
	data := []byte("text/plain\x00\x00")
	types := parseMIMEList(data)
	if len(types) != 1 || types[0] != "text/plain" {
		t.Errorf("got %v, want [text/plain]", types)
	}
}

func TestParseMIMEListNoTerminator(t *testing.T) {
	// Missing final empty string - should still parse what's available
	data := []byte("text/html\x00image/png\x00")
	types := parseMIMEList(data)
	if len(types) != 2 {
		t.Errorf("got %d types, want 2", len(types))
	}
}
