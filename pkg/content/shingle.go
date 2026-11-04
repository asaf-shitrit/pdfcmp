package content

import (
	"strings"
	"unicode"

	"github.com/cespare/xxhash/v2"
)

// DefaultShingleSize is the number of consecutive tokens in a shingle.
const DefaultShingleSize = 3

// Tokens lowercases text, splits it on whitespace and trims punctuation from
// each token's edges. Tokens that are only punctuation are dropped.
func Tokens(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	tokens := fields[:0]
	for _, f := range fields {
		f = strings.TrimFunc(f, unicode.IsPunct)
		if f != "" {
			tokens = append(tokens, f)
		}
	}
	return tokens
}

// Shingles returns the 64-bit hashes of every run of k consecutive tokens in
// text. Text with fewer than k tokens yields one shingle of all its tokens;
// text with none yields nil. The result may contain duplicates.
func Shingles(text string, k int) []uint64 {
	if k < 1 {
		k = 1
	}
	tokens := Tokens(text)
	if len(tokens) == 0 {
		return nil
	}
	if len(tokens) < k {
		k = len(tokens)
	}

	hashes := make([]uint64, len(tokens))
	for i, t := range tokens {
		hashes[i] = xxhash.Sum64String(t)
	}

	shingles := make([]uint64, 0, len(tokens)-k+1)
	for i := 0; i+k <= len(tokens); i++ {
		var h uint64
		for _, th := range hashes[i : i+k] {
			// Order-sensitive combine: "a b c" and "c b a" differ.
			h = mix64(h*0x9e3779b97f4a7c15 ^ th)
		}
		shingles = append(shingles, h)
	}
	return shingles
}

// mix64 is the splitmix64 finalizer: a fast bijection that spreads every input
// bit across the output.
func mix64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}
