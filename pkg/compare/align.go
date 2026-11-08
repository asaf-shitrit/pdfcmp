package compare

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/asaf-shitrit/pdfcmp/pkg/content"
	"github.com/asaf-shitrit/pdfcmp/pkg/render"
)

// Limits that keep alignment cheap on large documents.
const (
	// maxLCSCells caps the table of the longest-common-subsequence search,
	// which is (n+1)*(m+1) cells of 4 bytes for the n and m pages left after
	// the common prefix and suffix are trimmed. Beyond it the middle is not
	// searched: its pages are paired by position, as one gap.
	maxLCSCells = 4_000_000

	// maxGapWork caps the text comparisons spent pairing the pages of one
	// gap, which are n1*n2 for gaps of n1 and n2 pages. Beyond it the gap is
	// paired by position, from its start.
	maxGapWork = 10_000
)

// fingerprint identifies a page for alignment, without rendering it. A page
// with text is identified by its text hash alone, so a document exported
// again by another tool, whose content streams differ, still lines up; the
// pairs are then judged by the usual rules. A page without text is identified
// by its object count and content hash, and so matches only when its
// graphics are unchanged: a scan saved again rarely keeps its image bytes.
// Pages with neither text nor objects, and pages whose info or content hash
// cannot be read, are not valid and never match.
type fingerprint struct {
	text    uint64
	objects int
	content uint64
	valid   bool
}

// pageFingerprints fingerprints every page of doc and returns each page's
// text, which alignment uses to line up pages inside a gap.
func pageFingerprints(doc render.Document) ([]fingerprint, []string) {
	fps := make([]fingerprint, doc.PageCount())
	texts := make([]string, len(fps))
	for i := range fps {
		info, err := doc.PageInfo(i)
		hash, herr := doc.ContentHash(i)
		if err != nil || herr != nil {
			continue
		}
		texts[i] = info.Text
		switch {
		case info.TextHash != 0:
			fps[i] = fingerprint{text: info.TextHash, valid: true}
		case info.ObjectCount > 0:
			fps[i] = fingerprint{objects: info.ObjectCount, content: hash, valid: true}
		}
	}
	return fps, texts
}

func (f fingerprint) matches(g fingerprint) bool { return f.valid && f == g }

// alignment is how the pages of two documents of different lengths line up.
// All page numbers are 0-indexed.
type alignment struct {
	exact    []pagePair // same fingerprint: the same page, not rendered
	gap      []pagePair // paired by position or text between exact pairs
	deleted  []int      // pages of document 1 with no page in document 2
	inserted []int      // pages of document 2 with no page in document 1
}

// alignPages aligns two documents' pages by fingerprint. It trims the common
// prefix and suffix, finds the longest common subsequence of what is left
// (unless that is over maxLCSCells), and pairs the pages between matches with
// pairGap. sim scores the likeness of page i of document 1 and page j of
// document 2 in 0..1.
func alignPages(fp1, fp2 []fingerprint, sim func(i, j int) float64) alignment {
	var al alignment
	anchors := anchorPairs(fp1, fp2)
	al.exact = anchors
	prev := pagePair{-1, -1}
	for _, a := range append(slices.Clone(anchors), pagePair{len(fp1), len(fp2)}) {
		al.addGap(prev.p1+1, a.p1, prev.p2+1, a.p2, sim)
		prev = a
	}
	return al
}

// anchorPairs returns the pages that match by fingerprint, in order.
func anchorPairs(fp1, fp2 []fingerprint) []pagePair {
	lo := 0
	for lo < len(fp1) && lo < len(fp2) && fp1[lo].matches(fp2[lo]) {
		lo++
	}
	hi := 0
	for hi < len(fp1)-lo && hi < len(fp2)-lo && fp1[len(fp1)-1-hi].matches(fp2[len(fp2)-1-hi]) {
		hi++
	}

	pairs := make([]pagePair, 0, lo+hi)
	for i := range lo {
		pairs = append(pairs, pagePair{i, i})
	}
	for _, p := range lcsPairs(fp1[lo:len(fp1)-hi], fp2[lo:len(fp2)-hi]) {
		pairs = append(pairs, pagePair{p.p1 + lo, p.p2 + lo})
	}
	for k := hi; k > 0; k-- {
		pairs = append(pairs, pagePair{len(fp1) - k, len(fp2) - k})
	}
	return pairs
}

// lcsPairs returns the longest common subsequence of a and b as index pairs.
// It returns nil when the search would pass maxLCSCells.
func lcsPairs(a, b []fingerprint) []pagePair {
	n, m := len(a), len(b)
	if n == 0 || m == 0 || (n+1)*(m+1) > maxLCSCells {
		return nil
	}
	// l[i*(m+1)+j] is the length of the LCS of a[i:] and b[j:].
	l := make([]int32, (n+1)*(m+1))
	at := func(i, j int) *int32 { return &l[i*(m+1)+j] }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i].matches(b[j]) {
				*at(i, j) = *at(i+1, j+1) + 1
			} else {
				*at(i, j) = max(*at(i+1, j), *at(i, j+1))
			}
		}
	}

	var pairs []pagePair
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i].matches(b[j]):
			pairs = append(pairs, pagePair{i, j})
			i++
			j++
		case *at(i+1, j) >= *at(i, j+1):
			i++
		default:
			j++
		}
	}
	return pairs
}

// addGap pairs the unmatched pages [s1,e1) of document 1 with [s2,e2) of
// document 2, with pairGap; every page left over is deleted or inserted.
func (al *alignment) addGap(s1, e1, s2, e2 int, sim func(i, j int) float64) {
	used1, used2 := map[int]bool{}, map[int]bool{}
	for _, p := range pairGap(e1-s1, e2-s2, func(i, j int) float64 { return sim(s1+i, s2+j) }) {
		al.gap = append(al.gap, pagePair{s1 + p.p1, s2 + p.p2})
		used1[s1+p.p1], used2[s2+p.p2] = true, true
	}
	for i := s1; i < e1; i++ {
		if !used1[i] {
			al.deleted = append(al.deleted, i)
		}
	}
	for j := s2; j < e2; j++ {
		if !used2[j] {
			al.inserted = append(al.inserted, j)
		}
	}
}

// pairGap pairs the pages of a gap of n1 pages in one document with a gap of
// n2 in the other, in order. The shorter side is paired in full, and the
// pairs are the ones that maximise the total of sim (the likeness of page i
// and page j), so a page dropped from the middle of a gap leaves the pages
// after it paired with their own counterparts. On a tie the earliest pairing
// wins. Past maxGapWork comparisons it pairs by position instead.
func pairGap(n1, n2 int, sim func(i, j int) float64) []pagePair {
	if n1 > n2 {
		swapped := pairGap(n2, n1, func(i, j int) float64 { return sim(j, i) })
		for k, p := range swapped {
			swapped[k] = pagePair{p.p2, p.p1}
		}
		return swapped
	}
	if n1*n2 > maxGapWork {
		pairs := make([]pagePair, n1)
		for i := range pairs {
			pairs[i] = pagePair{i, i}
		}
		return pairs
	}
	return bestPairing(n1, n2, sim)
}

// bestPairing pairs every page of a gap of n1 with a distinct page of a gap
// of n2 >= n1, keeping order, for the highest total similarity.
func bestPairing(n1, n2 int, sim func(i, j int) float64) []pagePair {
	// best[i][j] is the highest total pairing the first i pages with the
	// first j; -1 when j < i leaves a page unpaired.
	best := make([][]float64, n1+1)
	for i := range best {
		best[i] = make([]float64, n2+1)
		for j := range best[i] {
			switch {
			case i == 0:
			case j < i:
				best[i][j] = -1
			default:
				best[i][j] = best[i-1][j-1] + sim(i-1, j-1)
				if j > i && best[i][j-1] >= best[i][j] {
					best[i][j] = best[i][j-1]
				}
			}
		}
	}
	pairs := make([]pagePair, n1)
	for i, j := n1, n2; i > 0; j-- {
		if j > i && best[i][j] == best[i][j-1] {
			continue // page j-1 is left out
		}
		i--
		pairs[i] = pagePair{i, j - 1}
	}
	return pairs
}

// alignStage handles documents whose page counts differ. It aligns their
// pages, scores the matched pairs, and records the pages that were deleted
// from the first document and inserted in the second. The documents are
// different whatever the pairs score: a page with no counterpart has nothing
// to be similar to, so SimilarityScore is 0.
func (c *comparer) alignStage(ctx context.Context, result *Result, doc1, doc2 render.Document) error {
	c.opts.Logger.Debug("Page counts differ, aligning pages",
		F("pages1", doc1.PageCount()), F("pages2", doc2.PageCount()))
	result.ComparisonLayers = append(result.ComparisonLayers, "align")
	c.reportProgress("Aligning pages", 0, doc1.PageCount()+doc2.PageCount(), 0.2)

	fp1, text1 := pageFingerprints(doc1)
	fp2, text2 := pageFingerprints(doc2)
	al := alignPages(fp1, fp2, func(i, j int) float64 {
		return textLikeness(text1[i], text2[j])
	})

	// Every aligned pair goes through the usual per-page rules: planPages
	// skips rendering where text, object count and content hash all match,
	// and renders the rest, as it does when the page counts are equal.
	pairs := append(slices.Clone(al.exact), al.gap...)
	slices.SortFunc(pairs, func(a, b pagePair) int { return a.p1 - b.p1 })
	pages, err := c.comparePairs(ctx, doc1, doc2, pairs, render.Options{DPI: c.opts.DPI})
	if err != nil {
		return fmt.Errorf("aligned comparison failed: %w", err)
	}
	for i := range pages {
		pages[i].Page2 = pairs[i].p2 + 1
	}

	result.setPages(pages, "page count mismatch, aligned", c.opts.HashType, c.opts.DPI)
	result.SimilarityScore = 0.0
	result.DeletedPages = oneIndexed(al.deleted)
	result.InsertedPages = oneIndexed(al.inserted)
	return nil
}

// textLikeness scores how alike two pages' texts are in 0..1, for pairing the
// pages of a gap. Long text is scored by shingled MinHash. Shingles are too
// few on short pages for that to mean anything (content.Similarity reports
// !ok), and a score of 0 for every pair would pair the gap by position, so
// short pages fall back to the Jaccard similarity of their distinct words.
// Words with digits are left out when both pages have other words: page
// numbers and running totals change from page to page and would make unlike
// pages look alike.
func textLikeness(a, b string) float64 {
	if s, ok := content.Similarity(a, b); ok {
		return s
	}
	w1, w2 := content.Tokens(a), content.Tokens(b)
	if p1, p2 := withoutDigits(w1), withoutDigits(w2); len(p1) > 0 && len(p2) > 0 {
		w1, w2 = p1, p2
	}
	return wordJaccard(w1, w2)
}

// withoutDigits returns the words that hold no digit.
func withoutDigits(words []string) []string {
	var out []string
	for _, w := range words {
		if !strings.ContainsFunc(w, unicode.IsDigit) {
			out = append(out, w)
		}
	}
	return out
}

// wordJaccard is the Jaccard similarity of the distinct words of a and b; 0
// when either has none.
func wordJaccard(a, b []string) float64 {
	set := make(map[string]bool, len(a))
	for _, w := range a {
		set[w] = true
	}
	union := len(set)
	shared := 0
	seen := make(map[string]bool, len(b))
	for _, w := range b {
		if seen[w] {
			continue
		}
		seen[w] = true
		if set[w] {
			shared++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}

// oneIndexed turns 0-indexed page numbers into 1-indexed ones.
func oneIndexed(pages []int) []int {
	if len(pages) == 0 {
		return nil
	}
	out := make([]int, len(pages))
	for i, p := range pages {
		out[i] = p + 1
	}
	return out
}
