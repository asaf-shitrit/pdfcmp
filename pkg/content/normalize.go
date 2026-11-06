package content

import (
	"strings"

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
