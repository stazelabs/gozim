package zim

import (
	"encoding/binary"
	"testing"
)

func FuzzParseHeader(f *testing.F) {
	f.Add(makeValidHeader())
	f.Add(make([]byte, 0))
	f.Add(make([]byte, 40))
	f.Add(make([]byte, 80))

	// Header with bad magic
	bad := makeValidHeader()
	binary.LittleEndian.PutUint32(bad[0:4], 0xDEADBEEF)
	f.Add(bad)

	f.Fuzz(func(t *testing.T, data []byte) {
		parseHeader(data) // must not panic
	})
}

func FuzzParseDirectoryEntry(f *testing.F) {
	// Content entry seed
	f.Add(makeContentEntry(0, 'C', 5, 3, "Main_Page", "Main Page"))
	// Redirect entry seed
	f.Add(makeRedirectEntry('C', 42, "Old_Page", "Old Page"))
	// Minimal data
	f.Add(make([]byte, 0))
	f.Add(make([]byte, 16))

	f.Fuzz(func(t *testing.T, data []byte) {
		parseDirectoryEntry(data, nil, 0) // must not panic
	})
}

func FuzzParseMIMEList(f *testing.F) {
	f.Add([]byte("text/html\x00image/png\x00\x00"))
	f.Add([]byte{0})
	f.Add([]byte{})
	f.Add([]byte("text/plain\x00"))

	f.Fuzz(func(t *testing.T, data []byte) {
		parseMIMEList(data) // must not panic
	})
}

func FuzzExtractBlobs(f *testing.F) {
	// Valid uncompressed cluster (without the info byte)
	blob := []byte("hello")
	offsets := make([]byte, 8) // 2 offsets * 4 bytes
	binary.LittleEndian.PutUint32(offsets[0:4], 8)
	binary.LittleEndian.PutUint32(offsets[4:8], 13)
	f.Add(append(offsets, blob...), false)

	// Extended offsets
	extOffsets := make([]byte, 16) // 2 offsets * 8 bytes
	binary.LittleEndian.PutUint64(extOffsets[0:8], 16)
	binary.LittleEndian.PutUint64(extOffsets[8:16], 21)
	f.Add(append(extOffsets, blob...), true)

	f.Add([]byte{}, false)
	f.Add(make([]byte, 4), false)

	f.Fuzz(func(t *testing.T, data []byte, extended bool) {
		extractBlobs(data, extended) // must not panic
	})
}
