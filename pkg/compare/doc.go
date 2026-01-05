// Package compare provides high-performance PDF comparison functionality.
//
// The package implements a multi-layer comparison pipeline that combines
// byte-wise and visual comparison techniques to efficiently detect both
// identical and visually similar PDF documents.
//
// # Comparison Pipeline
//
// The comparison process uses a fail-fast approach with multiple layers:
//
//  1. Byte-wise comparison: Uses xxHash64 for fast binary comparison (~9 GB/s)
//  2. Thumbnail check: Renders first/last pages at 72 DPI for quick visual rejection
//  3. Strategic sampling: Compares sqrt(n)+2 pages for large documents
//  4. Full comparison: Compares all pages only if samples match above threshold
//
// # Basic Usage
//
//	cmp, err := compare.New()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cmp.Close()
//
//	result, err := cmp.Compare(ctx, "file1.pdf", "file2.pdf")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Printf("Similarity: %.2f%%\n", result.SimilarityScore * 100)
//
// # Configuration
//
// The comparison behavior can be customized using functional options:
//
//	cmp, err := compare.New(
//	    compare.WithDPI(300),           // Higher DPI for more accuracy
//	    compare.WithWorkers(8),         // Parallel processing
//	    compare.WithThreshold(0.90),    // Similarity threshold
//	    compare.WithSampling(compare.SamplingAll), // Compare all pages
//	)
//
// # Visual Hashing
//
// The package uses perceptual hashing algorithms for visual comparison:
//
//   - dHash (difference hash): Fast, good for near-identical images
//   - pHash (perceptual hash): More robust, uses DCT for better tolerance
//
// Both hashes produce 64-bit values compared using Hamming distance.
package compare
