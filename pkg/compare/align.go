package compare

import (
	"context"
	"fmt"
	"slices"

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

	// maxOffsetWork caps the text comparisons spent choosing how a short
	// gap lines up with a longer one. Beyond it the gap lines up from its
	// start.
	maxOffsetWork = 2_000
)

// fingerprint identifies a page without rendering it. Two pages with equal
// valid fingerprints are the same page. Pages without text and without
// graphical objects, and pages whose info or content hash cannot be read,
// are not valid and never match: scanned pages have no text hash, and a scan
// saved again rarely has the same image bytes, so scans match only when
// their content hashes agree.
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
		fps[i] = fingerprint{
			text:    info.TextHash,
			objects: info.ObjectCount,
			content: hash,
			valid:   info.TextHash != 0 || info.ObjectCount > 0,
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
// document 2. The shorter side is paired in full, in order, against the run
// of the longer side that its text matches best; every other page is
// deleted or inserted.
func (al *alignment) addGap(s1, e1, s2, e2 int, sim func(i, j int) float64) {
	n1, n2 := e1-s1, e2-s2
	k := min(n1, n2)
	o1, o2 := 0, 0
	switch {
	case n1 > n2:
		o1 = bestOffset(n1-n2, k, func(o, t int) float64 { return sim(s1+o+t, s2+t) })
	case n2 > n1:
		o2 = bestOffset(n2-n1, k, func(o, t int) float64 { return sim(s1+t, s2+o+t) })
	}
	for t := range k {
		al.gap = append(al.gap, pagePair{s1 + o1 + t, s2 + o2 + t})
	}
	for i := s1; i < e1; i++ {
		if i < s1+o1 || i >= s1+o1+k {
			al.deleted = append(al.deleted, i)
		}
	}
	for j := s2; j < e2; j++ {
		if j < s2+o2 || j >= s2+o2+k {
			al.inserted = append(al.inserted, j)
		}
	}
}

// bestOffset returns the offset in 0..slack that gives the highest total of
// score(offset, t) over t in 0..k-1, the earliest on a tie. Past
// maxOffsetWork scores it returns 0.
func bestOffset(slack, k int, score func(o, t int) float64) int {
	if k == 0 || (slack+1)*k > maxOffsetWork {
		return 0
	}
	best, bestSum := 0, -1.0
	for o := 0; o <= slack; o++ {
		sum := 0.0
		for t := range k {
			sum += score(o, t)
		}
		if sum > bestSum {
			best, bestSum = o, sum
		}
	}
	return best
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
		s, _ := content.Similarity(text1[i], text2[j])
		return s
	})

	scored, err := c.comparePairs(ctx, doc1, doc2, al.gap, render.Options{DPI: c.opts.DPI})
	if err != nil {
		return fmt.Errorf("aligned comparison failed: %w", err)
	}
	pages := make([]PageResult, 0, len(al.exact)+len(scored))
	for _, p := range al.exact {
		pages = append(pages, PageResult{Page: p.p1 + 1, Page2: p.p2 + 1, Similarity: 1.0})
	}
	for i, r := range scored {
		r.Page2 = al.gap[i].p2 + 1
		pages = append(pages, r)
	}
	slices.SortFunc(pages, func(a, b PageResult) int { return a.Page - b.Page })

	result.setPages(pages, "page count mismatch, aligned", c.opts.HashType, c.opts.DPI)
	result.SimilarityScore = 0.0
	result.DeletedPages = oneIndexed(al.deleted)
	result.InsertedPages = oneIndexed(al.inserted)
	return nil
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
