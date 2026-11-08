package compare

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

// shortPages builds one text per id: two short lines of their own and a
// "Page k of N" footer, so inserting a page changes every footer.
func shortPages(ids ...string) map[int]string {
	text := map[int]string{}
	for k, id := range ids {
		text[k] = fmt.Sprintf("Section %s heading\nNotes about %s widgets\nPage %d of %d", id, id, k+1, len(ids))
	}
	return text
}

func alignFake(t *testing.T, t1, t2 map[int]string) *Result {
	t.Helper()
	c := &comparer{opts: DefaultOptions()}
	d1 := textDoc{&fakeDoc{pages: len(t1), draw: drawText(t1)}, t1}
	d2 := textDoc{&fakeDoc{pages: len(t2), draw: drawText(t2)}, t2}
	res := &Result{}
	if err := c.alignStage(context.Background(), res, d1, d2); err != nil {
		t.Fatal(err)
	}
	return res
}

// Footers that count pages change every page's text hash when a page is
// inserted, so nothing anchors. The short pages are still paired by their
// words, not by position.
func TestAlignShortPagesWithRenumberedFooters(t *testing.T) {
	res := alignFake(t, shortPages("alpha", "bravo", "charlie", "delta"),
		shortPages("xray", "alpha", "bravo", "charlie", "delta"))
	if !slices.Equal(res.InsertedPages, []int{1}) || len(res.DeletedPages) != 0 {
		t.Errorf("inserted %v deleted %v, want [1] and none", res.InsertedPages, res.DeletedPages)
	}
	for _, p := range res.PageResults {
		if p.Page2 != p.Page+1 {
			t.Errorf("page %d paired with page %d", p.Page, p.Page2)
		}
	}
}

// A page with no like partner is deleted, not paired by position.
func TestAlignShortPagesNotFalselyMatched(t *testing.T) {
	res := alignFake(t, shortPages("alpha", "bravo", "charlie"), shortPages("bravo", "charlie"))
	if !slices.Equal(res.DeletedPages, []int{1}) || len(res.InsertedPages) != 0 {
		t.Errorf("deleted %v inserted %v, want [1] and none", res.DeletedPages, res.InsertedPages)
	}
}

func TestTextLikeness(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want float64
	}{
		{"a b c", "a b c", 1},
		{"a b", "c d", 0},
		{"", "a", 0},
		{"a b", "b c", 1.0 / 3},
		{"a b page 1", "a b page 2", 1}, // numbers are left out
		{"total 1200", "total 1300", 1}, // only the word counts
		{"1200", "1300", 0},             // all numbers: compared as they are
		{"1200", "1200", 1},
	} {
		if got := textLikeness(tc.a, tc.b); got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("textLikeness(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
