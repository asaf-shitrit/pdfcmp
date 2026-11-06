package visual

import (
	"image"
	"image/draw"
	"image/jpeg"
	"math"
	"sync"
	"testing"

	"github.com/asaf-shitrit/pdfcmp/internal/fixtures"
)

var (
	pagesOnce  sync.Once
	smoothPage image.Image
	textPage   image.Image
)

// pages builds a smooth scan and a scan of text once; both 840x1188.
func pages() (smooth, text image.Image) {
	pagesOnce.Do(func() { smoothPage, textPage = fixtures.ScanImage(2), fixtures.TextScanImage(2) })
	return smoothPage, textPage
}

// moved is img moved by (dx, dy) px, black where nothing came in.
func moved(img image.Image, dx, dy int) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), image.NewUniform(image.Black), image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds().Add(image.Pt(dx, dy)), img, img.Bounds().Min, draw.Src)
	return out
}

// exposed is img with its grey levels scaled by gain and moved by bias.
func exposed(img image.Image, gain, bias float64) *image.Gray {
	g := ToGrayscale(img)
	out := image.NewGray(g.Rect)
	for i, v := range g.Pix {
		out.Pix[i] = uint8(math.Max(0, math.Min(255, gain*float64(v)+bias)))
	}
	return out
}

func TestAlignedTileDiffIdentical(t *testing.T) {
	smooth, text := pages()
	for name, img := range map[string]image.Image{"smooth": smooth, "text": text} {
		if d, tile := AlignedTileDiff(img, img); d != 0 || tile == nil {
			t.Errorf("%s, identical pages: diff %v, tile %v, want 0 and a tile", name, d, tile)
		}
	}
}

// A page moved by a few pixels is the same scan: the strict check calls it
// different, the aligned one does not, on soft and on sharp pages alike.
func TestAlignedTileDiffShift(t *testing.T) {
	smooth, text := pages()
	for name, img := range map[string]image.Image{"smooth": smooth, "text": text} {
		for _, s := range [][2]int{{1, 0}, {0, -2}, {5, 3}, {-4, 7}, {9, 8}} {
			if d, _ := AlignedTileDiff(img, moved(img, s[0], s[1])); d > TileTolerance*0.6 {
				t.Errorf("%s shifted by %v: aligned diff %.2f, want under %.1f", name, s, d, TileTolerance/2)
			}
		}
		if d, _ := TileDiff(img, moved(img, 3, 2)); d <= TileTolerance {
			t.Errorf("%s: strict diff of a shifted scan = %.2f, want it over the tolerance (else the aligned check proves nothing)", name, d)
		}
	}
}

func TestAlignedTileDiffExposure(t *testing.T) {
	smooth, text := pages()
	for name, img := range map[string]image.Image{"smooth": smooth, "text": text} {
		for _, e := range [][2]float64{{1, 15}, {1, -15}, {1.08, 0}, {0.92, 10}} {
			if d, _ := AlignedTileDiff(ToGrayscale(img), exposed(img, e[0], e[1])); d > TileTolerance*0.6 {
				t.Errorf("%s, gain %v bias %v: aligned diff %.2f, want under %.1f", name, e[0], e[1], d, TileTolerance/2)
			}
		}
		// A page that is much darker is not the same page under another lamp.
		if d, _ := AlignedTileDiff(ToGrayscale(img), exposed(img, 0.5, 0)); d <= TileTolerance {
			t.Errorf("%s at half brightness: diff %.2f, want it over the tolerance", name, d)
		}
	}
}

func TestAlignedTileDiffFindsMarks(t *testing.T) {
	smooth, text := pages()
	for kind, base := range map[string]image.Image{"smooth": smooth, "text": text} {
		shifted := moved(base, 4, -3)
		for name, marked := range map[string]image.Image{
			"stamp":  fixtures.StampImage(shifted, 600, 900, 120, 60),
			"tick":   fixtures.MarkImage(shifted, 3, image.Pt(300, 600), image.Pt(315, 640), image.Pt(340, 600)),
			"dot":    fixtures.MarkImage(shifted, 12, image.Pt(200, 300), image.Pt(200, 300)),
			"swap":   fixtures.ScanImage(77),
			"nopage": image.NewGray(base.Bounds()),
		} {
			d, tile := AlignedTileDiff(base, marked)
			if d <= TileTolerance || tile == nil {
				t.Errorf("%s, %s: aligned diff %.2f, want it over the tolerance", kind, name, d)
			}
			// A mark on the first page is found as well as a mark on the second.
			if back, _ := AlignedTileDiff(marked, base); back <= TileTolerance {
				t.Errorf("%s, %s, pages swapped: aligned diff %.2f, want it over the tolerance", kind, name, back)
			}
		}
	}
}

func TestAlignedTileDiffOtherSizesAndTypes(t *testing.T) {
	// landscape
	a, b := flat(900, 640, 180), flat(900, 640, 180)
	paint(b, image.Rect(300, 300, 360, 340), 20)
	if d, tile := AlignedTileDiff(a, b); d <= TileTolerance || tile == nil || tile.Cols != TileRows {
		t.Errorf("landscape: diff %.2f, tile %+v, want a difference on a %dx%d grid", d, tile, TileRows, TileCols)
	}
	// pages of two sizes, and pages too small for the grid, are TileDiff's
	if d, _ := AlignedTileDiff(flat(640, 900, 180), flat(320, 450, 20)); d <= TileTolerance {
		t.Errorf("pages of two sizes: diff %.2f, want a difference", d)
	}
	if d, tile := AlignedTileDiff(flat(10, 10, 0), flat(10, 10, 255)); d != 0 || tile != nil {
		t.Errorf("page smaller than the grid: diff %v, tile %v, want 0 and no tile", d, tile)
	}
	// RGBA, NRGBA, YCbCr and Gray renders of one page agree
	smooth, _ := pages()
	var buf = jpegRoundTrip(t, smooth, 90)
	nrgba := image.NewNRGBA(buf.Bounds())
	draw.Draw(nrgba, nrgba.Bounds(), buf, buf.Bounds().Min, draw.Src)
	for name, img := range map[string]image.Image{"rgba": smooth, "nrgba": nrgba, "ycbcr": buf, "gray": ToGrayscale(buf)} {
		if d, _ := AlignedTileDiff(ToGrayscale(buf), img); d > TileTolerance*0.6 {
			t.Errorf("%s against gray: diff %.2f, want it small", name, d)
		}
	}
}

func TestFitExposure(t *testing.T) {
	var x, y []float64
	for i := 0; i < 400; i++ {
		v := 100 + float64(i%120)
		x = append(x, v)
		y = append(y, 1.05*v+8)
	}
	for i := 0; i < 8; i++ { // tiles under a mark
		y[i*50] = 10
	}
	if e := fitExposure(x, y); math.Abs(e.gain-1.05) > 0.01 || math.Abs(e.bias-8) > 1 {
		t.Errorf("fit = %+v, want gain 1.05, bias 8", e)
	}
	flatX, flatY := make([]float64, 50), make([]float64, 50)
	for i := range flatX {
		flatX[i], flatY[i] = 200, 190
	}
	if e := fitExposure(flatX, flatY); e.gain != 1 || math.Abs(e.bias+10) > 0.01 {
		t.Errorf("flat page fit = %+v, want gain 1, bias -10", e)
	}
	if e := fitExposure([]float64{100, 200, 150}, []float64{255, 255, 255}); e.gain < 1/maxGain || e.gain > maxGain || math.Abs(e.bias) > maxBias {
		t.Errorf("clipped page fit = %+v, want it inside the limits", e)
	}
	if e := fitExposure(nil, nil); e.gain != 1 || e.bias != 0 {
		t.Errorf("empty fit = %+v, want gain 1, bias 0", e)
	}
}

func TestExposureDiff(t *testing.T) {
	e := exposure{gain: 1.1, bias: 5}
	for _, tc := range []struct{ v, u, want float64 }{
		{100, 115, 0},  // on the line
		{100, 100, 15}, // off it
		{250, 255, 0},  // clipped white on both
		{240, 254, 0},  // the line takes it past white, the scanner clips
		{240, 200, 55}, // clipped on one only
		{0, 3, 0},      // clipped black
		{0, 0, 0},
	} {
		if got := e.diff(tc.v, tc.u); math.Abs(got-tc.want) > 0.01 {
			t.Errorf("diff(%v, %v) = %v, want %v", tc.v, tc.u, got, tc.want)
		}
	}
}

func TestIntegralMean(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 40, 40))
	paint(g, image.Rect(10, 10, 20, 20), 200)
	in := newIntegral(g)
	// softEdge blurs the page but not the total ink in a rectangle around it
	if m, ok := in.mean(5, 5, 25, 25); !ok || math.Abs(m-200*100.0/400) > 1 {
		t.Errorf("mean around the box = %v, %v, want 50", m, ok)
	}
	if m, _ := in.mean(5, 5, 25, 25); math.Abs(m-func() float64 { v, _ := in.meanInt(5, 5, 25, 25); return v }()) > 1e-9 {
		t.Error("mean and meanInt disagree on whole pixels")
	}
	// half a pixel is half way between its neighbours
	lo, _ := in.mean(10, 10, 20, 20)
	hi, _ := in.mean(11, 10, 21, 20)
	if mid, _ := in.mean(10.5, 10, 20.5, 20); math.Abs(mid-(lo+hi)/2) > 1e-9 {
		t.Errorf("mean at half a pixel = %v, want %v", mid, (lo+hi)/2)
	}
	for _, r := range [][4]float64{{-1, 0, 10, 10}, {0, -1, 10, 10}, {0, 0, 41, 10}, {0, 0, 10, 41}} {
		if _, ok := in.mean(r[0], r[1], r[2], r[3]); ok {
			t.Errorf("mean%v = ok, want a rectangle off the page refused", r)
		}
	}
	if _, ok := in.meanInt(5, 5, 5, 9); ok {
		t.Error("meanInt of an empty rectangle = ok, want refused")
	}
}

func TestShiftField(t *testing.T) {
	f := shiftField{ax: 3, bx: 0.01, ay: -2, by: -0.01, cx: 100, cy: 200}
	if dx, dy := f.at(100, 200); dx != 3 || dy != -2 {
		t.Errorf("at the centre = %v, %v, want 3, -2", dx, dy)
	}
	if dx, dy := f.at(0, 400); dx != 5 || dy != -1 {
		t.Errorf("at (0, 400) = %v, %v, want 5, -1", dx, dy)
	}
	if dx, dy := f.neg().at(100, 200); dx != -3 || dy != 2 {
		t.Errorf("negated at the centre = %v, %v, want -3, 2", dx, dy)
	}
}

// The field found for a simulated rescan matches the shift and lean the
// simulator drew, to within three pixels (the final match has a pixel and a half of slack more).
func TestEstimateShift(t *testing.T) {
	smooth, text := pages()
	for name, base := range map[string]image.Image{"smooth": smooth, "text": text} {
		for seed := int64(1); seed <= 3; seed++ {
			dev, _ := fixtures.RescanDeviationOf(seed, fixtures.RescanHeavy)
			a, b := newIntegral(base), newIntegral(fixtures.Rescan(base, seed, fixtures.RescanHeavy))
			_, f := align(a, b, newTiling(840, 1188))
			// the simulator maps the page's pixel p to R(p-c) + c + d, so
			// the shift at the corners differs from the centre's by the lean
			for _, p := range [][2]float64{{420, 594}, {140, 200}, {700, 990}} {
				x, y := p[0]-420, p[1]-594
				wantX := dev.DX - dev.Rot*y
				wantY := dev.DY + dev.Rot*x
				dx, dy := f.at(p[0], p[1])
				if math.Abs(dx-wantX) > 3 || math.Abs(dy-wantY) > 3 {
					t.Errorf("%s seed %d at %v: shift (%.1f, %.1f), want (%.1f, %.1f) within 3 px", name, seed, p, dx, dy, wantX, wantY)
				}
			}
		}
	}
}

func jpegBytesRoundTrip(t testing.TB, img image.Image) image.Image {
	t.Helper()
	return jpegRoundTrip(t, img, 90)
}

var _ = jpeg.DefaultQuality
