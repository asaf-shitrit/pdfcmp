package bench

import (
	"fmt"
	"math"
	"math/bits"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/asaf-shitrit/pdfcmp/pkg/content"
)

// SimHash was the alternative to MinHash for pdfcmp's text similarity. It
// lives here, not in pkg/content, so the comparison that ruled it out stays
// reproducible: PDFCMP_MATRIX=1 go test ./bench -run 'TestContentMatrix|TestTemplatedPage' -v

// simHash returns the 64-bit SimHash fingerprint of shingles: bit b is set
// when more than half of the shingle hashes have bit b set. Unlike MinHash,
// a repeated shingle votes once per occurrence.
func simHash(shingles []uint64) uint64 {
	var votes [64]int
	for _, s := range shingles {
		h := splitmix64(s)
		for b := range votes {
			if h&(1<<b) != 0 {
				votes[b]++
			} else {
				votes[b]--
			}
		}
	}
	var fp uint64
	for b, v := range votes {
		if v > 0 {
			fp |= 1 << b
		}
	}
	return fp
}

// simHashSimilarity estimates the cosine similarity of two texts' shingle
// count vectors as cos(pi*d/64), clamped to [0, 1], where d is the Hamming
// distance between their fingerprints.
func simHashSimilarity(a, b string) float64 {
	fa := simHash(content.Shingles(a, content.DefaultShingleSize))
	fb := simHash(content.Shingles(b, content.DefaultShingleSize))
	d := bits.OnesCount64(fa ^ fb)
	return math.Max(0, math.Cos(math.Pi*float64(d)/64))
}

func splitmix64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// templatedPage mirrors the medium_* fixtures: a title and ten lines of the
// same sentence, each ending in a value.
func templatedPage(seed int64) string {
	r := rand.New(rand.NewSource(seed))
	var b strings.Builder
	b.WriteString("Medium Test Document - Page 1\n")
	for j := 0; j < 10; j++ {
		fmt.Fprintf(&b, "Line %d: Lorem ipsum dolor sit amet, consectetur adipiscing elit. Value: %d\n", j+1, r.Intn(1000))
	}
	return b.String()
}

// TestTemplatedPage compares exact Jaccard, MinHash and SimHash on templated
// pages whose ten values all changed. MinHash tracks the set; SimHash counts
// the repeated sentence and reads the pages as nearly identical.
func TestTemplatedPage(t *testing.T) {
	if os.Getenv("PDFCMP_MATRIX") == "" {
		t.Skip("set PDFCMP_MATRIX=1 to run the estimator comparison")
	}
	fmt.Printf("%-6s %8s %8s %8s\n", "pair", "jaccard", "minhash", "simhash")
	for i := int64(0); i < 5; i++ {
		a, b := templatedPage(2*i+1), templatedPage(2*i+2)
		fmt.Printf("%-6d %8.4f %8.4f %8.4f\n", i+1,
			exactJaccard(a, b), content.MinHashSimilarity(a, b), simHashSimilarity(a, b))
	}
}
