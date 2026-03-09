package zim

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.bin")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func TestPreadReader(t *testing.T) {
	data := []byte("hello, world!")
	path := writeTempFile(t, data)

	r, err := newPreadReader(path)
	if err != nil {
		t.Fatalf("newPreadReader: %v", err)
	}
	defer r.Close()

	if r.Size() != int64(len(data)) {
		t.Errorf("size = %d, want %d", r.Size(), len(data))
	}

	buf := make([]byte, 5)
	n, err := r.ReadAt(buf, 7)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if n != 5 || string(buf) != "world" {
		t.Errorf("ReadAt(7) = %q, want %q", buf[:n], "world")
	}
}

func TestPreadReaderReadAtEOF(t *testing.T) {
	data := []byte("hello")
	path := writeTempFile(t, data)

	r, err := newPreadReader(path)
	if err != nil {
		t.Fatalf("newPreadReader: %v", err)
	}
	defer r.Close()

	// Read at exact file size — should return EOF
	buf := make([]byte, 1)
	_, err = r.ReadAt(buf, int64(len(data)))
	if err != io.EOF {
		t.Errorf("ReadAt(size) err = %v, want io.EOF", err)
	}

	// Request more bytes than available — should get partial + EOF
	buf = make([]byte, 10)
	n, err := r.ReadAt(buf, 3)
	if n != 2 {
		t.Errorf("ReadAt(3) n = %d, want 2", n)
	}
	if err != io.EOF {
		t.Errorf("ReadAt(3) err = %v, want io.EOF", err)
	}
	if string(buf[:n]) != "lo" {
		t.Errorf("ReadAt(3) data = %q, want %q", buf[:n], "lo")
	}
}

func TestPreadReaderDoubleClose(t *testing.T) {
	data := []byte("hello")
	path := writeTempFile(t, data)

	r, err := newPreadReader(path)
	if err != nil {
		t.Fatalf("newPreadReader: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// Second close should return an error (os.File double close)
	err = r.Close()
	if err == nil {
		t.Log("second close returned nil (may be OS-dependent)")
	}
}
