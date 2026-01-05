// Package bytewise provides fast byte-wise file comparison using xxHash64.
//
// The package implements streaming hash comparison with early-exit optimization
// for detecting file differences as quickly as possible.
//
// # Hash Algorithm
//
// xxHash64 is used for its exceptional speed (~9 GB/s on modern hardware)
// while maintaining good collision resistance for file comparison.
//
// # Comparison Strategies
//
// The package provides multiple comparison strategies:
//
//   - Compare: Full hash comparison with size check
//   - CompareWithEarlyExit: Streaming comparison that stops at first difference
//
// # Usage
//
//	result, err := bytewise.Compare("file1.pdf", "file2.pdf")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if result.Identical {
//	    fmt.Println("Files are byte-identical")
//	}
//
// # Early Exit
//
// For files that may differ early in content, use CompareWithEarlyExit:
//
//	identical, err := bytewise.CompareWithEarlyExit("file1.pdf", "file2.pdf")
//
// This avoids reading entire files when a difference is found early.
package bytewise
