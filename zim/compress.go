package zim

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

const (
	compNone = 1
	compZlib = 2 // deprecated
	compBZ2  = 3 // deprecated
	compLZMA = 4
	compZstd = 5
)

// zstdPool reuses zstd decoders across decompressions.
var zstdPool = sync.Pool{
	New: func() any {
		d, _ := zstd.NewReader(nil)
		return d
	},
}

func decompress(data []byte, compType uint8) ([]byte, error) {
	switch compType {
	case compNone:
		return data, nil
	case compLZMA:
		return decompressXZ(data)
	case compZstd:
		return decompressZstd(data)
	case compZlib, compBZ2:
		return nil, fmt.Errorf("%w: type %d (deprecated)", ErrUnsupportedCompression, compType)
	default:
		return nil, fmt.Errorf("%w: type %d", ErrUnsupportedCompression, compType)
	}
}

func decompressXZ(data []byte) ([]byte, error) {
	r, err := xz.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zim: xz decompression failed: %w", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("zim: xz decompression failed: %w", err)
	}
	return out, nil
}

func decompressZstd(data []byte) ([]byte, error) {
	dec := zstdPool.Get().(*zstd.Decoder)
	defer zstdPool.Put(dec)

	out, err := dec.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("zim: zstd decompression failed: %w", err)
	}
	return out, nil
}
