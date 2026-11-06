// Package visual provides perceptual hashing algorithms for image comparison.
//
// The package implements two complementary perceptual hashing algorithms:
//
// # dHash (Difference Hash)
//
// dHash is fast and effective for detecting identical or near-identical images.
// Algorithm:
//  1. Resize image to 9x8 grayscale
//  2. Compare adjacent pixels horizontally (left < right = 1, else 0)
//  3. Produce 64-bit hash (8 rows x 8 comparisons per row)
//
// # pHash (Perceptual Hash)
//
// pHash is more robust to minor variations using the Discrete Cosine Transform.
// Algorithm:
//  1. Resize image to 32x32 grayscale
//  2. Apply 2D DCT (Discrete Cosine Transform)
//  3. Extract 8x8 low-frequency components
//  4. Compute hash from median comparison
//
// # Tiles
//
// Both hashes see the page as a whole. TileDiff cuts a page into a grid of
// 64x90 tiles (90x64 for a landscape page) and returns the largest difference
// between the mean grey levels of two pages' matching tiles. A tile averages
// thousands of pixels, so JPEG noise washes out of it, while a stamp or a
// changed paragraph, which the hashes miss, moves a tile by tens of grey
// levels. A difference above TileTolerance is a change. TileSimilarity scores
// it 1 - d/TileScale, which is 0.95 at the tolerance.
//
// # Similarity Calculation
//
// Image similarity is computed using Hamming distance between hashes:
//
//	h1 := visual.DHash(img1)
//	h2 := visual.DHash(img2)
//	dist := visual.HammingDistance(h1, h2)
//	similarity := visual.Similarity(dist)  // 0.0 to 1.0
//
// # Combined Similarity
//
// For best results, use both hash types:
//
//	similarity := visual.CombinedSimilarity(dHash1, dHash2, pHash1, pHash2)
//
// The combined score is the plain average of the dHash similarity and the
// pHash similarity: 1 - (d + p)/128 for Hamming distances d and p.
package visual
