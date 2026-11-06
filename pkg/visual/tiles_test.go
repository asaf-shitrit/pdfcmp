package visual

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"testing"

	"github.com/asaf-shitrit/pdfcmp/internal/fixtures"
)

// flat is a w x h page of one grey level.
func flat(w, h int, v uint8) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = v
	}
	return g
}

// paint sets the pixels of r in g to v.
func paint(g *image.Gray, r image.Rectangle, v uint8) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			g.SetGray(x, y, color.Gray{v})
		}
	}
}

func jpegRoundTrip(t testing.TB, img image.Image, quality int) image.Image {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	out, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTileSimilarityIdentical(t *testing.T) {
	g := flat(640, 900, 180)
	sim, tile := TileSimilarity(g, g)
	if sim != 1 || tile == nil {
		t.Fatalf("identical pages: similarity %v, tile %v, want 1 and a tile", sim, tile)
	}
}

func TestTileSimilarityLocalEdit(t *testing.T) {
	a := flat(640, 900, 180)
	b := flat(640, 900, 180)
	// A 40x20 px dark box in tile column 30, row 50 (tiles are 10x10 px).
	paint(b, image.Rect(300, 500, 340, 520), 20)
	sim, tile := TileSimilarity(a, b)
	if sim > 0.5 {
		t.Errorf("local edit: similarity %v, want it well below the 0.95 threshold", sim)
	}
	if tile == nil || tile.Cols != TileCols || tile.Rows != TileRows || tile.Row < 50 || tile.Row > 51 || tile.Col < 30 || tile.Col > 33 {
		t.Errorf("worst tile = %+v, want inside the edit (cols 30-33, rows 50-51)", tile)
	}
}

func TestTileSimilarityToleratesNoise(t *testing.T) {
	a := flat(640, 900, 150)
	b := flat(640, 900, 150)
	rng := rand.New(rand.NewSource(1))
	for i := range b.Pix {
		b.Pix[i] = uint8(150 + rng.Intn(21) - 10)
	}
	if sim, _ := TileSimilarity(a, b); sim < 0.97 {
		t.Errorf("noise of +-10 grey levels: similarity %v, want at least 0.97", sim)
	}
}

func TestTileSimilarityLandscape(t *testing.T) {
	a := flat(900, 640, 180)
	b := flat(900, 640, 180)
	paint(b, image.Rect(0, 0, 60, 40), 0)
	sim, tile := TileSimilarity(a, b)
	if sim >= 0.95 || tile == nil || tile.Cols != TileRows || tile.Rows != TileCols {
		t.Errorf("landscape: similarity %v, tile %+v, want a difference on a %dx%d grid", sim, tile, TileRows, TileCols)
	}
}

func TestTileSimilarityDifferentOrientation(t *testing.T) {
	landscape := flat(900, 640, 180)
	paint(landscape, image.Rect(0, 0, 450, 640), 20)
	sim, tile := TileSimilarity(flat(640, 900, 180), landscape)
	if sim >= 0.95 || tile == nil || tile.Cols != TileCols {
		t.Errorf("portrait against landscape: similarity %v, tile %+v, want a difference on the first page's grid", sim, tile)
	}
}

func TestTileSimilarityTinyPage(t *testing.T) {
	sim, tile := TileSimilarity(flat(10, 10, 0), flat(10, 10, 255))
	if sim != 1 || tile != nil {
		t.Errorf("page smaller than the grid: similarity %v, tile %v, want 1 and no tile", sim, tile)
	}
}

func TestTileSimilarityDPIIndependent(t *testing.T) {
	small, large := flat(320, 450, 180), flat(1280, 1800, 180)
	paint(small, image.Rect(150, 250, 170, 260), 20)
	paint(large, image.Rect(600, 1000, 680, 1040), 20)
	s1, t1 := TileSimilarity(flat(320, 450, 180), small)
	s2, t2 := TileSimilarity(flat(1280, 1800, 180), large)
	if t1 == nil || t2 == nil || *t1 != *t2 {
		t.Errorf("same edit at two sizes: tiles %+v and %+v, want the same tile", t1, t2)
	}
	if d := s1 - s2; d > 0.05 || d < -0.05 {
		t.Errorf("same edit at two sizes: similarity %v and %v, want them close", s1, s2)
	}
}

// TestTileSeparation is the held-out check behind TileTolerance: the same scan
// at two JPEG qualities against the same scan with a stamp on it, over
// several seeds, stamp sizes and positions, with the stamp over light, dark
// and mid regions. Every rescan must pass the default threshold with room to
// spare and every stamp fail it with room to spare.
func TestTileSeparation(t *testing.T) {
	if testing.Short() {
		t.Skip("builds several scan pages")
	}
	const (
		threshold = 0.95
		margin    = 0.015
	)
	qualities := [][2]int{{92, 60}, {92, 75}, {75, 50}}
	stamps := [][4]int{{100, 100, 60, 30}, {600, 900, 120, 60}, {400, 600, 200, 100}, {50, 1050, 120, 60}}
	lowestSame, highestStamp := 1.0, 0.0
	for seed := int64(1); seed <= 4; seed++ {
		base := fixtures.ScanImage(seed)
		for _, q := range qualities {
			ref := jpegRoundTrip(t, base, q[0])
			if sim, _ := TileSimilarity(ref, jpegRoundTrip(t, base, q[1])); sim < lowestSame {
				lowestSame = sim
			}
			for _, s := range stamps {
				stamped := fixtures.StampImage(base, s[0], s[1], s[2], s[3])
				if sim, _ := TileSimilarity(ref, jpegRoundTrip(t, stamped, q[1])); sim > highestStamp {
					highestStamp = sim
				}
			}
		}
	}
	t.Logf("lowest rescan %.4f, highest stamp %.4f", lowestSame, highestStamp)
	if lowestSame < threshold+margin {
		t.Errorf("a rescan scored %.4f, want at least %.4f", lowestSame, threshold+margin)
	}
	if highestStamp > threshold-margin {
		t.Errorf("a stamp scored %.4f, want at most %.4f", highestStamp, threshold-margin)
	}
}

// Every image type gives the tiles of its grey conversion, give or take the
// rounding of the luma weights.
func TestTileMeansImageTypes(t *testing.T) {
	scan := fixtures.ScanImage(3)
	decoded := jpegRoundTrip(t, scan, 90) // *image.YCbCr
	nrgba := image.NewNRGBA(decoded.Bounds())
	for y := 0; y < nrgba.Rect.Dy(); y++ {
		for x := 0; x < nrgba.Rect.Dx(); x++ {
			nrgba.Set(x, y, decoded.At(x, y))
		}
	}
	want := TileMeans(ToGrayscale(decoded), TileCols, TileRows)
	for name, img := range map[string]image.Image{"rgba": scan, "nrgba": nrgba, "ycbcr": decoded} {
		got := TileMeans(img, TileCols, TileRows)
		ref := want
		if name == "rgba" {
			ref = TileMeans(ToGrayscale(scan), TileCols, TileRows)
		}
		for i := range ref {
			if d := got[i] - ref[i]; d > 1.5 || d < -1.5 {
				t.Fatalf("%s: tile %d mean %.2f, want %.2f", name, i, got[i], ref[i])
			}
		}
	}
}

func TestTileScoreAtTolerance(t *testing.T) {
	if got := TileScore(TileTolerance); got < 0.9499 || got > 0.9501 {
		t.Errorf("TileScore(TileTolerance) = %v, want 0.95", got)
	}
	if TileScore(0) != 1 || TileScore(TileScale*2) != 0 {
		t.Errorf("TileScore(0) = %v, TileScore(2*TileScale) = %v, want 1 and 0", TileScore(0), TileScore(TileScale*2))
	}
	a, b := flat(640, 900, 100), flat(640, 900, 100+TileTolerance+1)
	if d, _ := TileDiff(a, b); d <= TileTolerance {
		t.Errorf("uniform shift of %v grey levels: diff %v, want it above the tolerance", TileTolerance+1, d)
	}
}
