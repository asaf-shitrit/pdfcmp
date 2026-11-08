package content

import (
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
// Latin Extended letter inside a word ("Möller" for "Müller").
//
// It compares the texts position by position with whitespace removed, so a
// space an extractor moved ("2035. 3" for "2035.3") cannot show as a
// difference. It holds when the stripped texts have the same length, differ
// somewhere, and at least one difference is one of those marks on both sides:
// a separator between digits, or an accented letter on both sides that
// touched an ASCII letter in at least one text. Touching is judged in each
// original text, so a space moved beside the letter cannot hide it, but a
// remap that touches a letter in neither text ("Þ item" for "ß item", which
// whitespace removal would otherwise glue to "i") is not taken for one. Other
// differences are tolerated only between characters Bag does not count, such
// as a bullet remapped to "ï" or "¥", which is re-save noise and must not
// hide an exchange elsewhere. A difference in a letter or digit Bag counts, or
// a glyph remapped with no exchange beside it, is not an exchange and stays
// with the render.
func Swapped(a, b string) bool {
	ra, rb := strip(a), strip(b)
	if len(ra) != len(rb) {
		return false
	}
	found := false
	for i := range ra {
		x, y := ra[i], rb[i]
		if x.r == y.r {
			continue
		}
		if separatorAt(ra, i) && separatorAt(rb, i) || x.accented() && y.accented() && (x.nextTo || y.nextTo) {
			found = true
		} else if counted(x.r) || counted(y.r) {
			return false
		}
	}
	return found
}

// glyph is a rune of whitespace-stripped text, with whether an ASCII letter
// touched it before the whitespace was removed.
type glyph struct {
	r      rune
	nextTo bool
}

// accented reports whether g is a Latin-1 or Latin Extended letter Bag may
// count differently between texts. Whether it sits inside a word is nextTo.
func (g glyph) accented() bool {
	return g.r > 0x7F && g.r <= 0x2FF && g.r != 'ï' && unicode.IsLetter(g.r)
}

func strip(text string) []glyph {
	var out []glyph
	rs := []rune(text)
	for i, r := range rs {
		if unicode.IsSpace(r) {
			continue
		}
		out = append(out, glyph{r, i > 0 && isASCIILetter(rs[i-1]) || i < len(rs)-1 && isASCIILetter(rs[i+1])})
	}
	return out
}

// separatorAt reports whether gs[i] is a separator between two digits.
func separatorAt(gs []glyph, i int) bool {
	return i > 0 && i < len(gs)-1 && strings.ContainsRune(",.:/-", gs[i].r) &&
		isDigit(gs[i-1].r) && isDigit(gs[i+1].r)
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isASCIILetter(r rune) bool { return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' }
