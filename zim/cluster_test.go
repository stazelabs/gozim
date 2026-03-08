package zim

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
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

// makeRawClusterPayload builds the uncompressed offset+blob payload (without
// the info byte) so it can be compressed externally.
func makeRawClusterPayload(blobs ...[]byte) []byte {
	numBlobs := len(blobs)
	numOffsets := numBlobs + 1
	headerSize := numOffsets * 4

	offsets := make([]uint32, numOffsets)
	pos := uint32(headerSize)
	for i, b := range blobs {
		offsets[i] = pos
		pos += uint32(len(b))
	}
	offsets[numBlobs] = pos

	offsetData := make([]byte, headerSize)
	for i, off := range offsets {
		binary.LittleEndian.PutUint32(offsetData[i*4:], off)
	}

	data := offsetData
	for _, b := range blobs {
		data = append(data, b...)
	}
	return data
}

func TestParseClusterZstd(t *testing.T) {
	blob1 := []byte("Hello from zstd compressed cluster!")
	blob2 := []byte("Second zstd blob with different content")
	payload := makeRawClusterPayload(blob1, blob2)

	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("zstd.NewWriter: %v", err)
	}
	compressed := enc.EncodeAll(payload, nil)
	enc.Close()

	// info byte: compZstd (5)
	clusterData := append([]byte{compZstd}, compressed...)

	c, err := parseCluster(clusterData)
	if err != nil {
		t.Fatalf("parseCluster(zstd): %v", err)
	}
	if len(c.blobs) != 2 {
		t.Fatalf("got %d blobs, want 2", len(c.blobs))
	}
	if string(c.blobs[0]) != string(blob1) {
		t.Errorf("blob[0] = %q, want %q", c.blobs[0], blob1)
	}
	if string(c.blobs[1]) != string(blob2) {
		t.Errorf("blob[1] = %q, want %q", c.blobs[1], blob2)
	}
}

func TestParseClusterXZ(t *testing.T) {
	blob1 := []byte("Hello from XZ/LZMA compressed cluster!")
	blob2 := []byte("Another XZ blob here")
	payload := makeRawClusterPayload(blob1, blob2)

	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz.NewWriter: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("xz write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("xz close: %v", err)
	}

	// info byte: compLZMA (4)
	clusterData := append([]byte{compLZMA}, buf.Bytes()...)

	c, err := parseCluster(clusterData)
	if err != nil {
		t.Fatalf("parseCluster(xz): %v", err)
	}
	if len(c.blobs) != 2 {
		t.Fatalf("got %d blobs, want 2", len(c.blobs))
	}
	if string(c.blobs[0]) != string(blob1) {
		t.Errorf("blob[0] = %q, want %q", c.blobs[0], blob1)
	}
	if string(c.blobs[1]) != string(blob2) {
		t.Errorf("blob[1] = %q, want %q", c.blobs[1], blob2)
	}
}

func TestParseClusterZstdCorrupt(t *testing.T) {
	// info byte = zstd, followed by garbage
	clusterData := []byte{compZstd, 0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x01}
	_, err := parseCluster(clusterData)
	if err == nil {
		t.Error("expected error for corrupt zstd data")
	}
}

func TestParseClusterXZCorrupt(t *testing.T) {
	// info byte = LZMA, followed by garbage
	clusterData := []byte{compLZMA, 0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x01}
	_, err := parseCluster(clusterData)
	if err == nil {
		t.Error("expected error for corrupt xz data")
	}
}

func TestParseClusterDeprecatedCompression(t *testing.T) {
	for _, comp := range []byte{compZlib, compBZ2} {
		clusterData := []byte{comp, 0x00}
		_, err := parseCluster(clusterData)
		if err == nil {
			t.Errorf("expected error for deprecated compression type %d", comp)
		}
	}
}

func TestParseClusterUnknownCompression(t *testing.T) {
	clusterData := []byte{0x0F, 0x00} // type 15, unknown
	_, err := parseCluster(clusterData)
	if err == nil {
		t.Error("expected error for unknown compression type")
	}
}
