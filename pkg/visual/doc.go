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
// The combined score weights dHash at 40% and pHash at 60%, as pHash
// is more robust to minor visual variations.
package visual
