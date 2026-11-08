package content

import (
	"maps"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Normalize reduces extraction noise so the same content hashes the same
// whichever tool produced or re-saved the PDF. It applies NFKC (ligature
// glyphs such as U+FB01 become "fi", NBSP becomes a space, compatibility
// forms fold), drops soft hyphens and zero-width characters, and collapses
// every whitespace run, CR and LF included, into a single space. Leading and
// trailing whitespace is trimmed.
//
// It does not rejoin words hyphenated across a line break: "co-\noperate"
// and "co-operate" stay different, because a hyphen at a line end cannot be
// told apart from a real one.
func Normalize(text string) string {
	text = norm.NFKC.String(text)
	text = strings.Map(dropInvisible, text)
	return strings.Join(strings.Fields(text), " ")
}

// dropInvisible removes characters that render as nothing: the soft hyphen,
// zero-width space, non-joiner and joiner, word joiner and the BOM.
func dropInvisible(r rune) rune {
	switch r {
	case '\u00ad', '\u200b', '\u200c', '\u200d', '\u2060', '\ufeff':
		return -1
	}
	return r
}

// Bag returns the letters and digits of text in sorted order: what is
// written on a page whatever order it was written in, and whatever symbols
// the PDF maps its glyphs to. Writing a PDF again can reorder its text runs,
// move the gaps an extractor reads as spaces, and map a glyph to another
// code point: Ghostscript turns bullets into "ï" or "¥" and broken encodings
// into control characters. Those land in punctuation, symbols, control
// characters and the Latin-1 and Latin Extended blocks, which Bag leaves out;
// a letter or digit of any other script added, removed or replaced changes
// it. A change confined to what it leaves out is the render's to judge.
func Bag(text string) string {
	var runes []rune
	for _, r := range text {
		if counted(r) {
			runes = append(runes, r)
		}
	}
	slices.Sort(runes)
	return string(runes)
}

// counted reports whether Bag keeps r: an ASCII letter or digit, or a
// letter or digit outside U+0080-U+02FF.
func counted(r rune) bool {
	if r < 0x80 {
		return r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
	}
	return r > 0x2FF && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// Swapped reports whether two texts differ by exchanging marks that Bag
// leaves out but that change what a page says: the comma, period, colon,
// slash or hyphen between two digits ("1,500" for "1.500") and a Latin-1 or
// Latin Extended letter ("Möller" for "Müller"). It holds when a class of
// marks has the same count in both texts and a different make-up, which an
// edit that replaces a mark does and a re-save does not. A re-save moves
// spaces and maps glyphs to other letters, so those marks come and go and
// their count changes.
func Swapped(a, b string) bool {
	ma, mb := marks(a), marks(b)
	for class := range ma {
		if sameTotal(ma[class], mb[class]) && !maps.Equal(ma[class], mb[class]) {
			return true
		}
	}
	return false
}

const (
	classSeparator = iota
	classLetter
)

// marks counts, per class, the separators between two digits and the Latin
// letters in U+0080-U+02FF other than the "ï" Ghostscript maps a bullet to.
func marks(text string) [2]map[rune]int {
	out := [2]map[rune]int{{}, {}}
	rs := []rune(text)
	for i, r := range rs {
		switch {
		case r > 0x7F && r <= 0x2FF && r != 'ï' && unicode.IsLetter(r):
			out[classLetter][r]++
		case i > 0 && i < len(rs)-1 && strings.ContainsRune(",.:/-", r) &&
			isDigit(rs[i-1]) && isDigit(rs[i+1]):
			out[classSeparator][r]++
		}
	}
	return out
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func sameTotal(a, b map[rune]int) bool {
	var x, y int
	for _, n := range a {
		x += n
	}
	for _, n := range b {
		y += n
	}
	return x == y
}
