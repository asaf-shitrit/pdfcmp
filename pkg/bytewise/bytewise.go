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

// Compare performs byte-wise comparison of two files.
// It first checks file sizes (fast early exit), then computes xxHash64.
func Compare(path1, path2 string) (Result, error) {
	// Get file info for both files
	stat1, err := os.Stat(path1)
	if err != nil {
		return Result{}, fmt.Errorf("cannot stat %s: %w", path1, err)
	}
	stat2, err := os.Stat(path2)
	if err != nil {
		return Result{}, fmt.Errorf("cannot stat %s: %w", path2, err)
	}

	result := Result{
		Size1:      stat1.Size(),
		Size2:      stat2.Size(),
		SizesMatch: stat1.Size() == stat2.Size(),
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
	defer f1.Close()

	f2, err := os.Open(path2)
	if err != nil {
		return false, err
	}
	defer f2.Close()

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
