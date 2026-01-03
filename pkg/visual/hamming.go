package visual

import "math/bits"

const (
	// HashBits is the number of bits in our perceptual hashes
	HashBits = 64

	// IdenticalThreshold - hashes are bit-for-bit identical
	IdenticalThreshold = 0
	// NearDuplicateThreshold - very similar (minor compression differences)
	NearDuplicateThreshold = 5
	// SimilarThreshold - visually similar
	SimilarThreshold = 10
	// DifferentThreshold - clearly different
	DifferentThreshold = 15
)

// HammingDistance calculates the number of differing bits between two hashes.
// Uses hardware POPCNT instruction via bits.OnesCount64 for maximum speed.
func HammingDistance(h1, h2 uint64) int {
	return bits.OnesCount64(h1 ^ h2)
}

// Similarity converts a Hamming distance to a similarity score (0.0 to 1.0).
func Similarity(distance int) float64 {
	return 1.0 - (float64(distance) / float64(HashBits))
}

// SimilarityFromHashes calculates similarity directly from two hashes.
func SimilarityFromHashes(h1, h2 uint64) float64 {
	return Similarity(HammingDistance(h1, h2))
}

// IsIdentical returns true if hashes match exactly.
func IsIdentical(h1, h2 uint64) bool {
	return h1 == h2
}

// IsNearDuplicate returns true if hashes are within near-duplicate threshold.
func IsNearDuplicate(h1, h2 uint64) bool {
	return HammingDistance(h1, h2) <= NearDuplicateThreshold
}

// IsSimilar returns true if hashes are within similarity threshold.
func IsSimilar(h1, h2 uint64) bool {
	return HammingDistance(h1, h2) <= SimilarThreshold
}

// IsDifferent returns true if hashes exceed the different threshold.
func IsDifferent(h1, h2 uint64) bool {
	return HammingDistance(h1, h2) > DifferentThreshold
}

// CombinedDistance calculates average Hamming distance from multiple hash pairs.
// Useful for comparing dHash and pHash together.
func CombinedDistance(dHash1, dHash2, pHash1, pHash2 uint64) int {
	d1 := HammingDistance(dHash1, dHash2)
	d2 := HammingDistance(pHash1, pHash2)
	return (d1 + d2) / 2
}

// CombinedSimilarity calculates average similarity from multiple hash pairs.
func CombinedSimilarity(dHash1, dHash2, pHash1, pHash2 uint64) float64 {
	s1 := SimilarityFromHashes(dHash1, dHash2)
	s2 := SimilarityFromHashes(pHash1, pHash2)
	return (s1 + s2) / 2
}
