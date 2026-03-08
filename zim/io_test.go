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

func TestMmapReader(t *testing.T) {
	data := []byte("hello, world!")
	path := writeTempFile(t, data)

	r, err := newMmapReader(path)
	if err != nil {
		t.Fatalf("newMmapReader: %v", err)
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

func TestMmapReaderDoubleClose(t *testing.T) {
	data := []byte("test data")
	path := writeTempFile(t, data)

	r, err := newMmapReader(path)
	if err != nil {
		t.Fatalf("newMmapReader: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second close should be no-op: %v", err)
	}
}

func TestReadersProduceSameResults(t *testing.T) {
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}
	path := writeTempFile(t, data)

	pr, err := newPreadReader(path)
	if err != nil {
		t.Fatalf("newPreadReader: %v", err)
	}
	defer pr.Close()

	mr, err := newMmapReader(path)
	if err != nil {
		t.Fatalf("newMmapReader: %v", err)
	}
	defer mr.Close()

	offsets := []int64{0, 1, 100, 1000, 4000}
	for _, off := range offsets {
		pbuf := make([]byte, 64)
		mbuf := make([]byte, 64)

		pn, perr := pr.ReadAt(pbuf, off)
		mn, merr := mr.ReadAt(mbuf, off)

		if pn != mn {
			t.Errorf("offset %d: pread read %d, mmap read %d", off, pn, mn)
		}
		if perr != merr {
			t.Errorf("offset %d: pread err=%v, mmap err=%v", off, perr, merr)
		}
		if string(pbuf[:pn]) != string(mbuf[:mn]) {
			t.Errorf("offset %d: data mismatch", off)
		}
	}
}

func TestMmapReaderReadAtEOF(t *testing.T) {
	data := []byte("hello")
	path := writeTempFile(t, data)

	r, err := newMmapReader(path)
	if err != nil {
		t.Fatalf("newMmapReader: %v", err)
	}
	defer r.Close()

	// Read at exact file size — should return EOF
	buf := make([]byte, 1)
	_, err = r.ReadAt(buf, int64(len(data)))
	if err != io.EOF {
		t.Errorf("ReadAt(size) err = %v, want io.EOF", err)
	}

	// Read past file size — should return EOF
	_, err = r.ReadAt(buf, int64(len(data)+100))
	if err != io.EOF {
		t.Errorf("ReadAt(past size) err = %v, want io.EOF", err)
	}

	// Negative offset — should return EOF
	_, err = r.ReadAt(buf, -1)
	if err != io.EOF {
		t.Errorf("ReadAt(-1) err = %v, want io.EOF", err)
	}
}

func TestMmapReaderReadPartial(t *testing.T) {
	data := []byte("hello")
	path := writeTempFile(t, data)

	r, err := newMmapReader(path)
	if err != nil {
		t.Fatalf("newMmapReader: %v", err)
	}
	defer r.Close()

	// Request more bytes than available from offset 3 — should get partial + EOF
	buf := make([]byte, 10)
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

func TestMmapReaderReadAfterClose(t *testing.T) {
	data := []byte("hello")
	path := writeTempFile(t, data)

	r, err := newMmapReader(path)
	if err != nil {
		t.Fatalf("newMmapReader: %v", err)
	}
	r.Close()

	// Read after close should return an error, not panic
	buf := make([]byte, 5)
	_, err = r.ReadAt(buf, 0)
	if err == nil {
		t.Error("expected error reading from closed mmap reader")
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
