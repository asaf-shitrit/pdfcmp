package content

import "math"

// MinHashSize is the number of slots in a MinHash signature.
const MinHashSize = 128

// MinHash is a MinHash signature of a set of shingles.
type MinHash [MinHashSize]uint64

// minHashSeeds holds one seed per slot; slot i hashes each shingle with
// mix64(shingle ^ seed[i]).
var minHashSeeds = func() [MinHashSize]uint64 {
	var seeds [MinHashSize]uint64
	for i := range seeds {
		seeds[i] = mix64(uint64(i+1) * 0x9e3779b97f4a7c15)
	}
	return seeds
}()

// NewMinHash returns the MinHash signature of shingles. The signature of an
// empty set has every slot at math.MaxUint64.
func NewMinHash(shingles []uint64) MinHash {
	var m MinHash
	for i := range m {
		m[i] = math.MaxUint64
	}
	for _, s := range shingles {
		for i, seed := range minHashSeeds {
			if h := mix64(s ^ seed); h < m[i] {
				m[i] = h
			}
		}
	}
	return m
}

// Similarity estimates the Jaccard similarity of the two shingle sets: the
// fraction of slots where the signatures agree.
func (m *MinHash) Similarity(o *MinHash) float64 {
	same := 0
	for i := range m {
		if m[i] == o[i] {
			same++
		}
	}
	return float64(same) / MinHashSize
}

// MinHashSimilarity shingles both texts with DefaultShingleSize and returns
// the estimated Jaccard similarity of their shingle sets.
func MinHashSimilarity(text1, text2 string) float64 {
	m1 := NewMinHash(Shingles(text1, DefaultShingleSize))
	m2 := NewMinHash(Shingles(text2, DefaultShingleSize))
	return m1.Similarity(&m2)
}

// MinShingles is the fewest distinct shingles one of two texts needs before
// their similarity is worth estimating. One changed token removes up to three
// shingles: at 60 that costs 9% (Jaccard 57/63), and on shorter text more,
// so "Annual Report 2025" against "Annual Report 2026" shares no shingle at
// all. Below it, a score says more about the page's length than about how
// much of it changed.
const MinShingles = 60

// Similarity estimates how much text two pages share, as MinHashSimilarity
// does. ok is false when both texts have fewer than MinShingles distinct
// shingles. When only one does, a low estimate is real: most of the longer
// text is gone.
func Similarity(text1, text2 string) (s float64, ok bool) {
	s1 := Shingles(text1, DefaultShingleSize)
	s2 := Shingles(text2, DefaultShingleSize)
	if distinct(s1) < MinShingles && distinct(s2) < MinShingles {
		return 0, false
	}
	m1, m2 := NewMinHash(s1), NewMinHash(s2)
	return m1.Similarity(&m2), true
}

func distinct(shingles []uint64) int {
	seen := make(map[uint64]struct{}, len(shingles))
	for _, s := range shingles {
		seen[s] = struct{}{}
	}
	return len(seen)
}
