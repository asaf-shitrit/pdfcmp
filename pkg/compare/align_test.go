package compare

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/asaf-shitrit/pdfcmp/internal/fixtures"
)

// fps builds valid fingerprints, one per id; equal ids are equal pages.
func fps(ids ...int) []fingerprint {
	out := make([]fingerprint, len(ids))
	for i, id := range ids {
		out[i] = fingerprint{text: uint64(id), objects: 1, content: uint64(id), valid: id != 0}
	}
	return out
}

func noSim(i, j int) float64 { return 0 }

func TestAlignPages(t *testing.T) {
	for _, tc := range []struct {
		name         string
		a, b         []int
		exact, gap   []pagePair
		del, inserts []int
	}{
		{"insert in the middle", []int{1, 2, 3, 4}, []int{1, 2, 9, 3, 4},
			[]pagePair{{0, 0}, {1, 1}, {2, 3}, {3, 4}}, nil, nil, []int{2}},
		{"delete", []int{1, 2, 3, 4}, []int{1, 3, 4},
			[]pagePair{{0, 0}, {2, 1}, {3, 2}}, nil, []int{1}, nil},
		{"append", []int{1, 2}, []int{1, 2, 3},
			[]pagePair{{0, 0}, {1, 1}}, nil, nil, []int{2}},
		{"prepend", []int{1, 2}, []int{9, 1, 2},
			[]pagePair{{0, 1}, {1, 2}}, nil, nil, []int{0}},
		{"insert and a changed page", []int{1, 2, 3, 4, 5}, []int{1, 9, 2, 3, 8, 5},
			[]pagePair{{0, 0}, {1, 2}, {2, 3}, {4, 5}}, []pagePair{{3, 4}}, nil, []int{1}},
		{"unmatched pages pair by position", []int{1, 7, 8, 5}, []int{1, 9, 5},
			[]pagePair{{0, 0}, {3, 2}}, []pagePair{{1, 1}}, []int{2}, nil},
		{"nothing in common", []int{1, 2}, []int{3, 4, 5},
			nil, []pagePair{{0, 0}, {1, 1}}, nil, []int{2}},
		{"invalid pages never match", []int{0, 2}, []int{0, 2, 3},
			[]pagePair{{1, 1}}, []pagePair{{0, 0}}, nil, []int{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			al := alignPages(fps(tc.a...), fps(tc.b...), noSim)
			if !slices.Equal(al.exact, tc.exact) || !slices.Equal(al.gap, tc.gap) ||
				!slices.Equal(al.deleted, tc.del) || !slices.Equal(al.inserted, tc.inserts) {
				t.Errorf("got exact %v gap %v deleted %v inserted %v\nwant exact %v gap %v deleted %v inserted %v",
					al.exact, al.gap, al.deleted, al.inserted, tc.exact, tc.gap, tc.del, tc.inserts)
			}
		})
	}
}

// The shorter side of a gap lines up with the run of the longer side whose
// text is most alike, not with its start.
func TestAlignGapPairsByText(t *testing.T) {
	sim := func(i, j int) float64 {
		if i == 1 && j == 2 {
			return 0.9
		}
		return 0.1
	}
	al := alignPages(fps(1, 7, 5), fps(1, 9, 8, 5), sim)
	if want := []pagePair{{1, 2}}; !slices.Equal(al.gap, want) {
		t.Errorf("gap = %v, want %v", al.gap, want)
	}
	if !slices.Equal(al.inserted, []int{1}) {
		t.Errorf("inserted = %v, want [1]", al.inserted)
	}
}

// A middle too big for the LCS table is paired by position instead.
func TestAlignLargeMiddleFallsBack(t *testing.T) {
	n := 2100 // (n+1)*(n+2) is over maxLCSCells
	a, b := make([]int, n), make([]int, n+1)
	for i := range a {
		a[i] = i + 1
	}
	for i := range b {
		b[i] = i + 5000
	}
	a[0], b[0] = 1, 2 // no common prefix
	if pairs := lcsPairs(fps(a...), fps(b...)); pairs != nil {
		t.Errorf("lcsPairs over the cap = %d pairs, want nil", len(pairs))
	}
	al := alignPages(fps(a...), fps(b...), noSim)
	if len(al.gap) != n || len(al.inserted) != 1 || len(al.exact) != 0 {
		t.Errorf("gap %d inserted %d exact %d, want %d, 1, 0", len(al.gap), len(al.inserted), len(al.exact), n)
	}
}

func TestOneIndexed(t *testing.T) {
	if oneIndexed(nil) != nil || !slices.Equal(oneIndexed([]int{0, 4}), []int{1, 5}) {
		t.Error("oneIndexed wrong")
	}
}

// sequence writes a Sequence fixture and returns its path.
func sequence(t *testing.T, dir, name string, ids ...int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := fixtures.Sequence(path, ids); err != nil {
		t.Fatal(err)
	}
	return path
}

func compareAligned(t *testing.T, a, b string) *Result {
	t.Helper()
	cmp, err := New(WithDPI(72), WithWorkers(2))
	if err != nil {
		t.Fatal(err)
	}
	defer cmp.Close()
	res, err := cmp.Compare(context.Background(), a, b)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestCompareAlignsPageCounts(t *testing.T) {
	dir := t.TempDir()
	base := sequence(t, dir, "base.pdf", 1, 2, 3, 4, 5, 6)
	for _, tc := range []struct {
		name      string
		ids       []int
		deleted   []int
		inserted  []int
		different []int // DifferentPages, first document's numbering
	}{
		{"insert in the middle", []int{1, 2, 3, 90, 4, 5, 6}, nil, []int{4}, nil},
		{"delete", []int{1, 2, 4, 5, 6}, []int{3}, nil, []int{3}},
		{"append", []int{1, 2, 3, 4, 5, 6, 7}, nil, []int{7}, nil},
		{"prepend", []int{9, 1, 2, 3, 4, 5, 6}, nil, []int{1}, nil},
		{"insert and a modified page", []int{1, 90, 2, 3, 4, 95, 6}, nil, []int{2}, []int{5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := sequence(t, dir, strings.ReplaceAll(tc.name, " ", "_")+".pdf", tc.ids...)
			res := compareAligned(t, base, other)
			if !slices.Equal(res.DeletedPages, tc.deleted) || !slices.Equal(res.InsertedPages, tc.inserted) {
				t.Errorf("deleted %v inserted %v, want %v %v", res.DeletedPages, res.InsertedPages, tc.deleted, tc.inserted)
			}
			if got := res.changedPages(); !slices.Equal(got, modifiedOf(tc.different, tc.deleted)) {
				t.Errorf("changed pages %v, want %v", got, modifiedOf(tc.different, tc.deleted))
			}
			if res.IsSimilar(0) || res.SimilarityScore != 0 || res.Metadata.Pages1 == res.Metadata.Pages2 {
				t.Errorf("want different at any threshold: score %v similar %v", res.SimilarityScore, res.IsSimilar(0))
			}
			if !slices.Contains(res.ComparisonLayers, "align") {
				t.Errorf("layers %v lack align", res.ComparisonLayers)
			}
			for _, pr := range res.PageResults {
				if pr.Page2 == 0 {
					t.Errorf("page %d has no Page2", pr.Page)
				}
			}

			// The other way round, what was inserted is deleted.
			rev := compareAligned(t, other, base)
			if !slices.Equal(rev.DeletedPages, tc.inserted) || !slices.Equal(rev.InsertedPages, tc.deleted) {
				t.Errorf("reversed: deleted %v inserted %v", rev.DeletedPages, rev.InsertedPages)
			}
		})
	}
}

// modifiedOf drops the deleted pages from a DifferentPages list, leaving
// the matched pages that changed.
func modifiedOf(different, deleted []int) []int {
	var out []int
	for _, p := range different {
		if !slices.Contains(deleted, p) {
			out = append(out, p)
		}
	}
	return out
}

func TestAlignedResultReporting(t *testing.T) {
	dir := t.TempDir()
	base := sequence(t, dir, "base.pdf", 1, 2, 3, 4)
	other := sequence(t, dir, "other.pdf", 1, 90, 2, 3, 95)
	res := compareAligned(t, base, other)

	if !slices.Equal(res.InsertedPages, []int{2}) || !slices.Equal(res.changedPages(), []int{4}) {
		t.Fatalf("inserted %v changed %v", res.InsertedPages, res.changedPages())
	}
	last := res.PageResults[len(res.PageResults)-1]
	if last.Page != 4 || last.Page2 != 5 || !last.IsDifferent {
		t.Errorf("last pair = page %d/%d different %v, want 4/5 different", last.Page, last.Page2, last.IsDifferent)
	}
	if first := res.PageResults[0]; first.Page != 1 || first.Page2 != 1 || first.Similarity != 1 {
		t.Errorf("first pair = %+v", first)
	}
	sum := res.Summary()
	for _, want := range []string{"Status: DIFFERENT", "Modified pages: [4] (file 1 numbering)", "Inserted pages (in file 2): [2]"} {
		if !strings.Contains(sum, want) {
			t.Errorf("summary lacks %q:\n%s", want, sum)
		}
	}
	js, err := res.JSON()
	if err != nil || !strings.Contains(js, `"inserted_pages"`) || !strings.Contains(js, `"page2"`) || strings.Contains(js, `"deleted_pages"`) {
		t.Errorf("JSON fields wrong (err %v):\n%s", err, js)
	}
}

func TestDeletedPagesAreDifferentPages(t *testing.T) {
	r := &Result{DeletedPages: []int{2}, PageResults: []PageResult{{Page: 4, IsDifferent: true}, {Page: 1}}}
	if got := r.DifferentPages(); !slices.Equal(got, []int{2, 4}) {
		t.Errorf("DifferentPages = %v, want [2 4]", got)
	}
	if !strings.Contains(r.Summary(), "Deleted pages (in file 1): [2]") {
		t.Errorf("summary lacks deleted pages:\n%s", r.Summary())
	}
}

// Documents of equal length are compared as before: no alignment, no Page2.
func TestEqualCountIsNotAligned(t *testing.T) {
	dir := t.TempDir()
	a := sequence(t, dir, "a.pdf", 1, 2, 3)
	b := sequence(t, dir, "b.pdf", 1, 95, 3)
	res := compareAligned(t, a, b)
	if slices.Contains(res.ComparisonLayers, "align") || res.pagesAdded() || !slices.Equal(res.DifferentPages(), []int{2}) {
		t.Errorf("layers %v, different %v", res.ComparisonLayers, res.DifferentPages())
	}
	for _, pr := range res.PageResults {
		if pr.Page2 != 0 {
			t.Errorf("page %d has Page2 %d", pr.Page, pr.Page2)
		}
	}
}

// A page dropped from the middle of a gap leaves the pages after it paired
// with their own counterparts, not shifted by one.
func TestPairGapKeepsOrderAroundMidGapDeletion(t *testing.T) {
	// Pages 0..3 of one side against 0..4 of the other; page 2 is gone.
	same := func(i, j int) float64 {
		if (i < 2 && j == i) || (i >= 2 && j == i+1) {
			return 0.9
		}
		return 0.1
	}
	got := pairGap(4, 5, same)
	if want := []pagePair{{0, 0}, {1, 1}, {2, 3}, {3, 4}}; !slices.Equal(got, want) {
		t.Errorf("pairGap = %v, want %v", got, want)
	}
	// And the other way round: a deletion from the first side.
	got = pairGap(5, 4, func(i, j int) float64 { return same(j, i) })
	if want := []pagePair{{0, 0}, {1, 1}, {3, 2}, {4, 3}}; !slices.Equal(got, want) {
		t.Errorf("transposed pairGap = %v, want %v", got, want)
	}
	// Past the work cap, pages pair by position.
	got = pairGap(101, 102, same)
	if len(got) != 101 || got[100] != (pagePair{100, 100}) {
		t.Errorf("capped pairGap ends %v", got[len(got)-1])
	}
	if got := pairGap(0, 3, same); len(got) != 0 {
		t.Errorf("empty side paired %v", got)
	}
}

// A page with text is identified by its text alone; a page without, by its
// graphics.
func TestFingerprintsIgnoreContentOfTextPages(t *testing.T) {
	dir := t.TempDir()
	plain := sequence(t, dir, "plain.pdf", 1, 2)
	redrawn := filepath.Join(dir, "redrawn.pdf")
	if err := fixtures.Redrawn(redrawn, []int{1, 2}); err != nil {
		t.Fatal(err)
	}
	cmp, err := New(WithDPI(72))
	if err != nil {
		t.Fatal(err)
	}
	defer cmp.Close()
	c := cmp.(*comparer)
	r, err := c.getRenderer()
	if err != nil {
		t.Fatal(err)
	}
	d1, _ := r.Open(plain)
	d2, _ := r.Open(redrawn)
	defer d1.Close()
	defer d2.Close()
	f1, _, _ := pageFingerprints(context.Background(), d1)
	f2, _, _ := pageFingerprints(context.Background(), d2)
	if !f1[0].matches(f2[0]) || f1[0].matches(f2[1]) {
		t.Errorf("fingerprints %+v vs %+v", f1, f2)
	}
}

// The second document was exported again: every page has the same text but
// other content streams. Pages still line up by text, and the pairs, judged
// by the usual rules, are not reported as changed.
func TestCompareAlignsReexportedDocument(t *testing.T) {
	dir := t.TempDir()
	base := sequence(t, dir, "base.pdf", 1, 2, 3, 4, 5, 6, 7, 8)
	for _, tc := range []struct {
		name              string
		ids               []int
		deleted, inserted []int
	}{
		{"deleted", []int{1, 2, 3, 4, 6, 7, 8}, []int{5}, nil},
		{"inserted", []int{1, 2, 3, 90, 4, 5, 6, 7, 8}, nil, []int{4}},
		{"deleted two", []int{1, 2, 3, 6, 7, 8}, []int{4, 5}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_")+".pdf")
			if err := fixtures.Redrawn(other, tc.ids); err != nil {
				t.Fatal(err)
			}
			res := compareAligned(t, base, other)
			if !slices.Equal(res.DeletedPages, tc.deleted) || !slices.Equal(res.InsertedPages, tc.inserted) {
				t.Errorf("deleted %v inserted %v, want %v %v", res.DeletedPages, res.InsertedPages, tc.deleted, tc.inserted)
			}
			if got := res.changedPages(); len(got) != 0 {
				t.Errorf("changed pages %v, want none", got)
			}
		})
	}
}

// A modified page in a re-exported document is still reported.
func TestCompareAlignsReexportedWithModifiedPage(t *testing.T) {
	dir := t.TempDir()
	base := sequence(t, dir, "base.pdf", 1, 2, 3, 4, 5, 6)
	other := filepath.Join(dir, "other.pdf")
	if err := fixtures.Redrawn(other, []int{1, 2, 95, 4, 6}); err != nil {
		t.Fatal(err)
	}
	res := compareAligned(t, base, other)
	if !slices.Equal(res.DeletedPages, []int{5}) || !slices.Equal(res.changedPages(), []int{3}) {
		t.Errorf("deleted %v changed %v, want [5] [3]", res.DeletedPages, res.changedPages())
	}
}
