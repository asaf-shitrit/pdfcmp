package compare

import (
	"context"
	"image"
	"path/filepath"
	"testing"

	"github.com/asaf-shitrit/pdfcmp/internal/fixtures"
	"github.com/asaf-shitrit/pdfcmp/pkg/render"
)

func TestTilesOption(t *testing.T) {
	if !DefaultOptions().Tiles {
		t.Error("tiles are off by default, want on")
	}
	opts := DefaultOptions()
	opts.Apply(WithTiles(false))
	if opts.Tiles {
		t.Error("WithTiles(false) left tiles on")
	}
	if err := opts.Validate(); err != nil {
		t.Errorf("tiles off is a valid option, got %v", err)
	}
}

// halves returns two flat portrait pages, the second with its left side
// painted over when dark is true.
func halves(dark bool) (image.Image, image.Image) {
	a, b := image.NewGray(image.Rect(0, 0, 640, 900)), image.NewGray(image.Rect(0, 0, 640, 900))
	for i := range a.Pix {
		a.Pix[i], b.Pix[i] = 200, 200
	}
	if dark {
		for y := 0; y < 900; y++ {
			for x := 0; x < 40; x++ {
				b.Pix[y*b.Stride+x] = 20
			}
		}
	}
	return a, b
}

func TestApplyTiles(t *testing.T) {
	same1, same2 := halves(false)
	edit1, edit2 := halves(true)
	r := PageResult{Similarity: 0.99}
	applyTiles(&r, same1, same2)
	if r.Similarity != 0.99 || r.TileSimilarity == nil || *r.TileSimilarity != 1 || r.WorstTile == nil || r.TileChanged {
		t.Errorf("same pages: %+v, want a tile score of 1 and no change", r)
	}

	r = PageResult{Similarity: 0.99}
	applyTiles(&r, edit1, edit2)
	if r.TileSimilarity == nil || *r.TileSimilarity >= 0.95 || !r.TileChanged {
		t.Errorf("edited page: tile %v, changed %v, want a tile score below 0.95 and a change", r.TileSimilarity, r.TileChanged)
	}
	if r.Similarity != 0.99 {
		t.Errorf("edited page: similarity %v, want it left at 0.99: a tile decides the verdict, not the score", r.Similarity)
	}

	r = PageResult{Similarity: 0.99}
	applyTiles(&r, image.NewGray(image.Rect(0, 0, 8, 8)), image.NewGray(image.Rect(0, 0, 8, 8)))
	if r.TileSimilarity != nil || r.WorstTile != nil || r.TileChanged {
		t.Errorf("page too small for tiles: %+v, want no tile score", r)
	}
}

// A changed tile makes a page different, a result not similar and a sample
// open to no further scan, whatever the scores say, until tiles are off.
func TestTileVerdict(t *testing.T) {
	page := PageResult{Similarity: 0.99, TileChanged: true}
	for _, tiles := range []bool{true, false} {
		c := &comparer{opts: Options{Tiles: tiles, Sampling: SamplingAuto, SimilarityThreshold: 0.95}}
		if got := c.needsFullScan([]PageResult{page}, 10); got == tiles {
			t.Errorf("tiles=%v: needsFullScan = %v, want %v", tiles, got, !tiles)
		}
		res := &Result{SimilarityScore: 0.99, TileChanged: true, tiles: tiles}
		if got := res.IsSimilar(0.95); got == tiles {
			t.Errorf("tiles=%v: IsSimilar = %v, want %v", tiles, got, !tiles)
		}
	}
}

func TestUseTiles(t *testing.T) {
	text := func(h1, h2 uint64) pageInfoPair {
		return pageInfoPair{info1: render.PageInfo{TextHash: h1}, info2: render.PageInfo{TextHash: h2}, ok: true}
	}
	for _, tc := range []struct {
		name  string
		tiles bool
		info  pageInfoPair
		want  bool
	}{
		{"on, no text", true, pageInfoPair{}, true},
		{"on, same text", true, text(1, 1), true},
		{"on, text changed", true, text(1, 2), false},
		{"off", false, text(1, 1), false},
	} {
		c := &comparer{opts: Options{Tiles: tc.tiles}}
		if got := c.useTiles(tc.info); got != tc.want {
			t.Errorf("%s: useTiles = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// scanPDF writes pages scans at a JPEG quality; page stampAt (0-based, -1 for
// none) carries a stamp and page swapAt (-1 for none) is a different scan.
func scanPDF(t *testing.T, path string, quality, stampAt, swapAt int) {
	t.Helper()
	err := fixtures.Scans(path, 3, quality, func(p int) image.Image {
		switch {
		case p == swapAt:
			return fixtures.ScanImage(1000)
		case p == stampAt:
			return fixtures.StampImage(fixtures.ScanImage(int64(p)), 600, 900, 120, 60)
		}
		return fixtures.ScanImage(int64(p))
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A rescan at another JPEG quality is the same document; a small stamp or a
// swapped page is not. The whole-page hashes alone pass the stamp.
func TestScansEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("renders scan pages")
	}
	dir := t.TempDir()
	q92, q60 := filepath.Join(dir, "q92.pdf"), filepath.Join(dir, "q60.pdf")
	stamp, swap := filepath.Join(dir, "stamp.pdf"), filepath.Join(dir, "swap.pdf")
	scanPDF(t, q92, 92, -1, -1)
	scanPDF(t, q60, 60, -1, -1)
	scanPDF(t, stamp, 92, 1, -1)
	scanPDF(t, swap, 92, -1, 1)

	for _, tc := range []struct {
		name     string
		other    string
		tiles    bool
		wantDiff bool
		page     int // 1-based page expected to be different, 0 for none
	}{
		{"rescan", q60, true, false, 0},
		{"stamp", stamp, true, true, 2},
		{"swap", swap, true, true, 2},
		{"swap without tiles", swap, false, true, 2},
		{"stamp without tiles", stamp, false, false, 0},
	} {
		cmp, err := New(WithSampling(SamplingAll), WithTiles(tc.tiles))
		if err != nil {
			t.Fatal(err)
		}
		res, err := cmp.Compare(context.Background(), q92, tc.other)
		cmp.Close()
		if err != nil {
			t.Fatal(err)
		}
		flagged := 0
		for _, p := range res.PageResults {
			if p.IsDifferent {
				flagged = p.Page
			}
			if (p.TileSimilarity != nil) != tc.tiles {
				t.Errorf("%s: page %d tile score set = %v, want %v", tc.name, p.Page, p.TileSimilarity != nil, tc.tiles)
			}
			if p.TileChanged && p.Similarity < 0 {
				t.Errorf("%s: page %d: negative similarity", tc.name, p.Page)
			}
		}
		t.Logf("%s: score %.4f, tile_changed %v, flagged page %d", tc.name, res.SimilarityScore, res.TileChanged, flagged)
		if res.IsSimilar(DefaultSimilarityThreshold) == tc.wantDiff || flagged != tc.page {
			t.Errorf("%s: score %.4f, flagged page %d, want different=%v on page %d",
				tc.name, res.SimilarityScore, flagged, tc.wantDiff, tc.page)
		}
		if tc.name == "stamp" && (!res.TileChanged || res.SimilarityScore < 0.95) {
			t.Errorf("stamp: tile_changed %v, score %.4f, want a changed tile and the hashes' score kept above 0.95", res.TileChanged, res.SimilarityScore)
		}
	}
}
