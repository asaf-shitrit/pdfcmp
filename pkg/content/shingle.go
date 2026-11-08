package content

import (
	"strings"
	"unicode"

	"github.com/cespare/xxhash/v2"
)

// DefaultShingleSize is the number of consecutive tokens in a shingle.
const DefaultShingleSize = 3

// Tokens lowercases text, splits it on whitespace and trims punctuation from
// each token's edges. Tokens that are only punctuation are dropped. Scripts
// written without spaces between words (Han, Hiragana, Katakana, Hangul, Thai
// and the like) would otherwise make a whole page one token, so each of their
// characters is a token of its own. Text without such characters is split on
// whitespace alone.
func Tokens(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.IndexFunc(f, isUnspaced) < 0 {
			if f = strings.TrimFunc(f, unicode.IsPunct); f != "" {
				tokens = append(tokens, f)
			}
			continue
		}
		tokens = appendUnspaced(tokens, f)
	}
	return tokens
}

// appendUnspaced splits one whitespace-free field that holds characters of
// unspaced scripts: each such character is a token, and the runs between them
// are tokens trimmed of punctuation.
func appendUnspaced(tokens []string, f string) []string {
	start := 0
	flush := func(end int) {
		if t := strings.TrimFunc(f[start:end], unicode.IsPunct); t != "" {
			tokens = append(tokens, t)
		}
	}
	for i, r := range f {
		if !isUnspaced(r) {
			continue
		}
		flush(i)
		tokens = append(tokens, string(r))
		start = i + len(string(r))
	}
	flush(len(f))
	return tokens
}

// unspacedScripts are the scripts whose words are not separated by spaces.
var unspacedScripts = []*unicode.RangeTable{
	unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul,
	unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar, unicode.Tibetan,
}

func isUnspaced(r rune) bool {
	return r >= 0x0E00 && unicode.IsOneOf(unspacedScripts, r)
}

// Shingles returns the 64-bit hashes of every run of k consecutive tokens in
// text. Text with fewer than k tokens yields one shingle of all its tokens;
// text with none yields nil. The result may contain duplicates.
func Shingles(text string, k int) []uint64 {
	return ShinglesOf(Tokens(text), k)
}

// ShinglesOf is Shingles for text already split by Tokens.
func ShinglesOf(tokens []string, k int) []uint64 {
	if k < 1 {
		k = 1
	}
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
