package zim

import (
	"encoding/binary"
	"testing"
)

func makeUncompressedCluster(blobs ...[]byte) []byte {
	numBlobs := len(blobs)
	offsetSize := 4
	numOffsets := numBlobs + 1

	// Calculate blob offsets
	headerSize := numOffsets * offsetSize
	offsets := make([]uint32, numOffsets)
	pos := uint32(headerSize)
	for i, b := range blobs {
		offsets[i] = pos
		pos += uint32(len(b))
	}
	offsets[numBlobs] = pos

	// Build offset list
	offsetData := make([]byte, headerSize)
	for i, off := range offsets {
		binary.LittleEndian.PutUint32(offsetData[i*4:], off)
	}

	// Concatenate offsets + blob data
	data := offsetData
	for _, b := range blobs {
		data = append(data, b...)
	}

	// Prepend info byte (compNone = 1, no extended flag)
	return append([]byte{compNone}, data...)
}

func makeUncompressedClusterExtended(blobs ...[]byte) []byte {
	numBlobs := len(blobs)
	offsetSize := 8
	numOffsets := numBlobs + 1

	headerSize := numOffsets * offsetSize
	offsets := make([]uint64, numOffsets)
	pos := uint64(headerSize)
	for i, b := range blobs {
		offsets[i] = pos
		pos += uint64(len(b))
	}
	offsets[numBlobs] = pos

	offsetData := make([]byte, headerSize)
	for i, off := range offsets {
		binary.LittleEndian.PutUint64(offsetData[i*8:], off)
	}

	data := offsetData
	for _, b := range blobs {
		data = append(data, b...)
	}

	// Info byte: compNone | clusterExtendedBit
	return append([]byte{compNone | clusterExtendedBit}, data...)
}

func TestParseClusterUncompressed(t *testing.T) {
	blob1 := []byte("Hello, World!")
	blob2 := []byte("Second blob")
	blob3 := []byte("Third")

	data := makeUncompressedCluster(blob1, blob2, blob3)
	c, err := parseCluster(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.blobs) != 3 {
		t.Fatalf("got %d blobs, want 3", len(c.blobs))
	}
	if string(c.blobs[0]) != "Hello, World!" {
		t.Errorf("blob[0] = %q", string(c.blobs[0]))
	}
	if string(c.blobs[1]) != "Second blob" {
		t.Errorf("blob[1] = %q", string(c.blobs[1]))
	}
	if string(c.blobs[2]) != "Third" {
		t.Errorf("blob[2] = %q", string(c.blobs[2]))
	}
}

func TestParseClusterExtended(t *testing.T) {
	blob1 := []byte("Extended blob data")
	data := makeUncompressedClusterExtended(blob1)

	c, err := parseCluster(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.blobs) != 1 {
		t.Fatalf("got %d blobs, want 1", len(c.blobs))
	}
	if string(c.blobs[0]) != "Extended blob data" {
		t.Errorf("blob[0] = %q", string(c.blobs[0]))
	}
}

func TestParseClusterSingleBlob(t *testing.T) {
	data := makeUncompressedCluster([]byte("single"))
	c, err := parseCluster(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.blobs) != 1 || string(c.blobs[0]) != "single" {
		t.Errorf("got %v", c.blobs)
	}
}

func TestParseClusterEmpty(t *testing.T) {
	_, err := parseCluster([]byte{})
	if err == nil {
		t.Error("expected error for empty cluster")
	}
}

func TestParseClusterEmptyBlob(t *testing.T) {
	// A cluster with one empty blob
	data := makeUncompressedCluster([]byte{})
	c, err := parseCluster(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.blobs) != 1 || len(c.blobs[0]) != 0 {
		t.Errorf("expected 1 empty blob, got %v", c.blobs)
	}
}
