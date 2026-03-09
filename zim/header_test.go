package zim

import (
	"encoding/binary"
	"errors"
	"testing"
)

func makeValidHeader() []byte {
	buf := make([]byte, headerSize)
	le := binary.LittleEndian

	le.PutUint32(buf[0:4], zimMagic)
	le.PutUint16(buf[4:6], 6) // major version
	le.PutUint16(buf[6:8], 1) // minor version
	// UUID at 8:24 left as zeros
	le.PutUint32(buf[24:28], 100)        // entry count
	le.PutUint32(buf[28:32], 10)         // cluster count
	le.PutUint64(buf[32:40], 80)         // url ptr pos
	le.PutUint64(buf[40:48], 880)        // title ptr pos
	le.PutUint64(buf[48:56], 1280)       // cluster ptr pos
	le.PutUint64(buf[56:64], 80)         // mime list pos
	le.PutUint32(buf[64:68], 0)          // main page
	le.PutUint32(buf[68:72], noMainPage) // layout page
	le.PutUint64(buf[72:80], 2000)       // checksum pos

	return buf
}

func TestParseHeader(t *testing.T) {
	buf := makeValidHeader()
	h, err := parseHeader(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.Magic != zimMagic {
		t.Errorf("magic = 0x%X, want 0x%X", h.Magic, zimMagic)
	}
	if h.MajorVersion != 6 {
		t.Errorf("major version = %d, want 6", h.MajorVersion)
	}
	if h.MinorVersion != 1 {
		t.Errorf("minor version = %d, want 1", h.MinorVersion)
	}
	if h.EntryCount != 100 {
		t.Errorf("entry count = %d, want 100", h.EntryCount)
	}
	if h.ClusterCount != 10 {
		t.Errorf("cluster count = %d, want 10", h.ClusterCount)
	}
	if h.MainPage != 0 {
		t.Errorf("main page = %d, want 0", h.MainPage)
	}
}

func TestParseHeaderTooShort(t *testing.T) {
	_, err := parseHeader(make([]byte, 40))
	if !errors.Is(err, ErrInvalidMagic) {
		t.Errorf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestParseHeaderBadMagic(t *testing.T) {
	buf := makeValidHeader()
	binary.LittleEndian.PutUint32(buf[0:4], 0xDEADBEEF)
	_, err := parseHeader(buf)
	if !errors.Is(err, ErrInvalidMagic) {
		t.Errorf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestParseHeaderUnsupportedVersion(t *testing.T) {
	buf := makeValidHeader()
	binary.LittleEndian.PutUint16(buf[4:6], 4) // unsupported version
	_, err := parseHeader(buf)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestParseHeaderVersion5(t *testing.T) {
	buf := makeValidHeader()
	binary.LittleEndian.PutUint16(buf[4:6], 5)
	h, err := parseHeader(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.MajorVersion != 5 {
		t.Errorf("major version = %d, want 5", h.MajorVersion)
	}
}
