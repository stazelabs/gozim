package zim

import (
	"crypto/md5"
	"errors"
	"testing"
)

func TestVerify(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	if err := a.Verify(); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestVerifyMismatch(t *testing.T) {
	// Build a fake archive with a valid checksum position but wrong checksum.
	// Layout: [content bytes] [16-byte MD5 checksum]
	content := []byte("this is some fake ZIM content that will be checksummed")
	wrongChecksum := make([]byte, md5.Size) // all zeros — won't match

	data := make([]byte, 0, len(content)+md5.Size)
	data = append(data, content...)
	data = append(data, wrongChecksum...)

	a := &Archive{
		r:   &bytesReader{data: data},
		hdr: header{ChecksumPos: uint64(len(content))},
	}

	err := a.Verify()
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got: %v", err)
	}
}

func TestVerifyCorrectChecksum(t *testing.T) {
	// Build a fake archive with a correct checksum.
	content := []byte("this is the content to be checksummed correctly")
	checksum := md5.Sum(content)

	data := make([]byte, 0, len(content)+md5.Size)
	data = append(data, content...)
	data = append(data, checksum[:]...)

	a := &Archive{
		r:   &bytesReader{data: data},
		hdr: header{ChecksumPos: uint64(len(content))},
	}

	if err := a.Verify(); err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestVerifyInvalidChecksumPos(t *testing.T) {
	data := []byte("short")
	a := &Archive{
		r:   &bytesReader{data: data},
		hdr: header{ChecksumPos: 0}, // position 0 is invalid (checksumPos <= 0)
	}

	err := a.Verify()
	if err == nil {
		t.Fatal("expected error for invalid checksum position")
	}
}

func TestVerifyChecksumPosBeyondFile(t *testing.T) {
	data := []byte("short")
	a := &Archive{
		r:   &bytesReader{data: data},
		hdr: header{ChecksumPos: 9999},
	}

	err := a.Verify()
	if err == nil {
		t.Fatal("expected error for checksum position beyond file")
	}
}
