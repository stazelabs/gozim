package zim

import (
	"bytes"
	"io"
)

// Blob is a reference to raw content bytes from a ZIM entry.
type Blob struct {
	data []byte
}

// Bytes returns the raw byte content.
func (b Blob) Bytes() []byte { return b.data }

// String returns the content as a string.
func (b Blob) String() string { return string(b.data) }

// Size returns the number of bytes in the blob.
func (b Blob) Size() int { return len(b.data) }

// Reader returns an io.Reader over the blob content.
func (b Blob) Reader() io.Reader { return bytes.NewReader(b.data) }
