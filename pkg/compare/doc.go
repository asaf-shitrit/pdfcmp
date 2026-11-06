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
//
// # Tiles
//
// A whole-page hash reduces a page to 9x8 or 32x32 samples, so a small local
// edit, such as a stamp on a scanned page, barely moves it. Each rendered
// page is therefore also cut into a grid of tiles (see package visual) and
// the mean grey level of each tile is compared. If the most changed tile
// moves by more than visual.TileTolerance grey levels, PageResult.TileChanged
// is set, the page is different and Result.IsSimilar is false, whatever the
// score. Like a text change, a tile change decides the verdict and leaves the
// score alone: Similarity says how much of the page changed, and a stamp is a
// small part of it. PageResult.TileSimilarity, 1 - d/visual.TileScale for the
// worst tile's difference d (0.95 at the tolerance), and PageResult.WorstTile
// show where. Tile means average out JPEG noise, so a rescan at another
// quality does not trip it. Pages whose text changed are left to the text
// rule. Turn tiles off with WithTiles(false).
//
// # Text Similarity
//
// Perceptual hashes record where text sits on a page, not what it says. When
// both pages have text and it differs, a MinHash signature of its word
// shingles estimates how much the pages share, and the page's similarity is
// the lower of its visual and text scores (see package content). Turn it off
// with WithTextHash(TextHashOff).
//
// # Scoring
//
// Result.SimilarityScore is the lowest page similarity: one changed page
// makes the documents different, however many pages surround it.
//
// # Verdict
//
// Result.IsSimilar(threshold) is false when any compared page's extracted
// text changed (Result.TextChanged), whatever the score: a changed invoice
// total looks like the same page to a perceptual hash, and the score stays
// graded to show how much changed. WithTextChange(TextChangeScored) restores
// a verdict from the score and the threshold alone. Before this verdict, a
// page with a one-digit text change could pass the default threshold.
package compare
