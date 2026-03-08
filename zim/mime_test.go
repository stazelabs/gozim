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

func TestParseMIMEListWithParams(t *testing.T) {
	// MIME types can include parameters like charset
	data := []byte("text/html; charset=utf-8\x00application/json\x00\x00")
	types := parseMIMEList(data)
	if len(types) != 2 {
		t.Fatalf("got %d types, want 2", len(types))
	}
	if types[0] != "text/html; charset=utf-8" {
		t.Errorf("types[0] = %q, want %q", types[0], "text/html; charset=utf-8")
	}
}

func TestParseMIMEListConsecutiveNulls(t *testing.T) {
	// Two consecutive nulls after a type means: type + terminator
	// The second null starts an empty string which terminates the list
	data := []byte("text/html\x00\x00extra/ignored\x00\x00")
	types := parseMIMEList(data)
	if len(types) != 1 {
		t.Errorf("got %d types, want 1 (list should stop at empty string): %v", len(types), types)
	}
}

func TestParseMIMEListLongType(t *testing.T) {
	// Very long MIME type name
	longType := "application/" + string(make([]byte, 300))
	for i := 12; i < len(longType); i++ {
		longType = longType[:i] + string(rune('a'+i%26)) + longType[i+1:]
	}
	// Build it properly
	buf := make([]byte, 300)
	for i := range buf {
		buf[i] = 'a' + byte(i%26)
	}
	longMIME := "application/" + string(buf)
	data := append([]byte(longMIME), 0, 0)
	types := parseMIMEList(data)
	if len(types) != 1 {
		t.Fatalf("got %d types, want 1", len(types))
	}
	if types[0] != longMIME {
		t.Errorf("type length = %d, want %d", len(types[0]), len(longMIME))
	}
}

func TestParseMIMEListNoData(t *testing.T) {
	types := parseMIMEList(nil)
	if len(types) != 0 {
		t.Errorf("got %d types for nil input, want 0", len(types))
	}

	types = parseMIMEList([]byte{})
	if len(types) != 0 {
		t.Errorf("got %d types for empty input, want 0", len(types))
	}
}

func TestParseMIMEListNoTrailingNull(t *testing.T) {
	// Data that doesn't end with a null — no complete type can be parsed
	data := []byte("text/html")
	types := parseMIMEList(data)
	if len(types) != 0 {
		t.Errorf("got %d types for unterminated data, want 0", len(types))
	}
}
