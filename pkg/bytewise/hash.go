package bytewise

import (
	"fmt"
	"io"

	"github.com/cespare/xxhash/v2"
)

const (
	// bufferSize is the chunk size for streaming file reads (32KB)
	bufferSize = 32 * 1024
)

// HashResult contains the hash and file metadata
type HashResult struct {
	Hash uint64
	Size int64
}

// HashFile computes xxHash64 of a file using streaming reads.
// This is memory-efficient for large files. Only regular files are accepted;
// a FIFO or device returns an error instead of blocking or never ending.
func HashFile(path string) (HashResult, error) {
	f, size, err := openRegular(path)
	if err != nil {
		return HashResult{}, err
	}
	defer func() { _ = f.Close() }()

	hash, err := HashReader(f)
	if err != nil {
		return HashResult{}, fmt.Errorf("cannot hash %s: %w", path, err)
	}

	return HashResult{Hash: hash, Size: size}, nil
}

// HashReader computes xxHash64 from an io.Reader using streaming reads.
func HashReader(r io.Reader) (uint64, error) {
	h := xxhash.New()
	buf := make([]byte, bufferSize)

	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, writeErr := h.Write(buf[:n]); writeErr != nil {
				return 0, writeErr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}

	return h.Sum64(), nil
}

// HashBytes computes xxHash64 of a byte slice.
func HashBytes(data []byte) uint64 {
	return xxhash.Sum64(data)
}
