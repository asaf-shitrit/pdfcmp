package bytewise

import (
	"bytes"
	"fmt"
	"os"
)

// Result represents the result of a byte-wise comparison
type Result struct {
	Identical  bool
	Hash1      uint64
	Hash2      uint64
	Size1      int64
	Size2      int64
	SizesMatch bool
}

// openRegular opens path for reading and returns it with its size. It rejects
// anything but a regular file: the size of a pipe, device or FIFO says nothing
// about how much it holds (a pipe reports the bytes buffered so far, /dev/zero
// reports 0 and never ends), so a size shortcut or a read-to-EOF on one gives a
// wrong answer or never returns.
func openRegular(path string) (*os.File, int64, error) {
	// Stat before opening: opening a FIFO for reading blocks until a writer
	// appears. The handle is checked again below.
	if pre, err := os.Stat(path); err == nil && !pre.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot open %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("cannot stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, 0, fmt.Errorf("%s is not a regular file", path)
	}
	return f, info.Size(), nil
}

// Compare performs byte-wise comparison of two files.
// Both files are opened first, so an unreadable file is an error whatever the
// sizes are. Only regular files are accepted; pipes, devices and FIFOs return
// an error. Then sizes are checked (fast early exit) and xxHash64 computed.
func Compare(path1, path2 string) (Result, error) {
	f1, size1, err := openRegular(path1)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = f1.Close() }()
	f2, size2, err := openRegular(path2)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = f2.Close() }()

	result := Result{
		Size1:      size1,
		Size2:      size2,
		SizesMatch: size1 == size2,
	}

	// Fast path: if sizes differ, files are definitely different
	if !result.SizesMatch {
		result.Identical = false
		return result, nil
	}

	// Compute hashes for both files
	hash1, err := HashFile(path1)
	if err != nil {
		return Result{}, fmt.Errorf("cannot hash %s: %w", path1, err)
	}
	hash2, err := HashFile(path2)
	if err != nil {
		return Result{}, fmt.Errorf("cannot hash %s: %w", path2, err)
	}

	result.Hash1 = hash1.Hash
	result.Hash2 = hash2.Hash
	result.Identical = hash1.Hash == hash2.Hash

	return result, nil
}

// QuickCompare performs a size-only comparison for fast early exit.
// Returns true only if sizes match (files might still differ in content).
func QuickCompare(path1, path2 string) (bool, error) {
	stat1, err := os.Stat(path1)
	if err != nil {
		return false, err
	}
	stat2, err := os.Stat(path2)
	if err != nil {
		return false, err
	}
	return stat1.Size() == stat2.Size(), nil
}

// CompareWithEarlyExit compares two files with streaming comparison.
// It stops as soon as a difference is found, avoiding full file reads
// when files differ early in their content.
func CompareWithEarlyExit(path1, path2 string) (bool, error) {
	// First check sizes
	sizesMatch, err := QuickCompare(path1, path2)
	if err != nil {
		return false, err
	}
	if !sizesMatch {
		return false, nil
	}

	// Open both files
	f1, err := os.Open(path1)
	if err != nil {
		return false, err
	}
	defer func() { _ = f1.Close() }()

	f2, err := os.Open(path2)
	if err != nil {
		return false, err
	}
	defer func() { _ = f2.Close() }()

	// Compare in chunks
	buf1 := make([]byte, bufferSize)
	buf2 := make([]byte, bufferSize)

	for {
		n1, err1 := f1.Read(buf1)
		n2, err2 := f2.Read(buf2)

		// Check if read sizes match
		if n1 != n2 {
			return false, nil
		}

		// Compare the chunks
		if !bytes.Equal(buf1[:n1], buf2[:n2]) {
			return false, nil
		}

		// Handle EOF
		if err1 != nil || err2 != nil {
			// Both should be EOF if files are same size
			return err1 == err2, nil
		}
	}
}
