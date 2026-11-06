package compare

import (
	"context"
	"hash/fnv"
	"testing"

	"github.com/asaf-shitrit/pdfcmp/pkg/render"
)

// textDoc is a fakeDoc whose pages report text. A page missing from text has
// none.
type textDoc struct {
	*fakeDoc
	text map[int]string
}

func (d textDoc) PageInfo(pageNum int) (render.PageInfo, error) {
	info, _ := d.fakeDoc.PageInfo(pageNum)
	if s, ok := d.text[pageNum]; ok {
		info.Text = s
		h := fnv.New64a()
		h.Write([]byte(s))
		info.TextHash = h.Sum64()
	}
	return info, nil
}

const (
	totalA = "Invoice 17 total due 1200 euros payable within thirty days of receipt by bank transfer"
	totalB = "Invoice 17 total due 1300 euros payable within thirty days of receipt by bank transfer"
)

// compareFake compares one-page fake documents with the given page texts and
// returns the Result a comparer would record.
func compareFake(t *testing.T, text1, text2 map[int]string, opts ...Option) (*Result, *comparer) {
	t.Helper()
	c := &comparer{opts: DefaultOptions()}
	c.opts.Apply(opts...)
	d1 := textDoc{&fakeDoc{pages: 1}, text1}
	d2 := textDoc{&fakeDoc{pages: 1}, text2}
	pages, err := c.comparePages(context.Background(), d1, d2, []int{0}, render.Options{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	res := &Result{textMode: c.opts.TextChange}
	res.setPages(pages, "all", c.opts.HashType, c.opts.DPI)
	return res, c
}

func TestTextChangeVerdict(t *testing.T) {
	a, b := map[int]string{0: totalA}, map[int]string{0: totalB}

	byDefault, _ := compareFake(t, a, b)
	scored, _ := compareFake(t, a, b, WithTextChange(TextChangeScored))

	if byDefault.SimilarityScore != scored.SimilarityScore {
		t.Errorf("the mode changed the score: %v vs %v", byDefault.SimilarityScore, scored.SimilarityScore)
	}
	if !byDefault.PageResults[0].TextChanged || !byDefault.TextChanged {
		t.Error("changed text not flagged on the page and the result")
	}
	if byDefault.IsSimilar(0.0) {
		t.Error("default mode: a text change must make the documents different at any threshold")
	}
	if !scored.IsSimilar(0.5) {
		t.Errorf("TextChangeScored: want similar, score %v", scored.SimilarityScore)
	}
	if got := byDefault.DifferentPages(); len(got) != 1 || got[0] != 1 {
		t.Errorf("default mode: DifferentPages = %v, want [1]", got)
	}
	if got := scored.DifferentPages(); len(got) != 0 {
		t.Errorf("TextChangeScored: DifferentPages = %v, want none", got)
	}
}

func TestTextChangedPresence(t *testing.T) {
	for _, tc := range []struct {
		name         string
		text1, text2 map[int]string
		want         bool
	}{
		{"same text", map[int]string{0: totalA}, map[int]string{0: totalA}, false},
		{"one word differs", map[int]string{0: totalA}, map[int]string{0: totalB}, true},
		{"text against none", map[int]string{0: totalA}, nil, true},
		{"none against text", nil, map[int]string{0: totalA}, true},
		{"no text on either, as in scans", nil, nil, false},
	} {
		res, _ := compareFake(t, tc.text1, tc.text2)
		if res.TextChanged != tc.want || res.PageResults[0].TextChanged != tc.want {
			t.Errorf("%s: result %v, page %v, want %v", tc.name, res.TextChanged, res.PageResults[0].TextChanged, tc.want)
		}
		if res.IsSimilar(0.5) == tc.want {
			t.Errorf("%s: IsSimilar(0.5) = %v", tc.name, res.IsSimilar(0.5))
		}
	}
}

func TestNeedsFullScanOnTextChange(t *testing.T) {
	sample := []PageResult{{Similarity: 1}, {Similarity: 0.97, TextChanged: true}}
	def := &comparer{opts: DefaultOptions()}
	scored := &comparer{opts: DefaultOptions()}
	scored.opts.TextChange = TextChangeScored

	if def.needsFullScan(sample, 10) {
		t.Error("default: a sampled text change already decides, want no full scan")
	}
	if !scored.needsFullScan(sample, 10) {
		t.Error("TextChangeScored: a similar sample still needs the full scan")
	}
}

func TestValidateTextChange(t *testing.T) {
	o := DefaultOptions()
	o.TextChange = TextChangeMode(7)
	if o.Validate() == nil {
		t.Error("want an error for an unknown text change mode")
	}
}
