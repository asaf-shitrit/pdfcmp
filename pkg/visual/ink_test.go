package visual

import (
	"image"
	"image/color"
	"testing"
)

// inkPage is a 640x900 page of paper colour bg with ten lines of "text":
// rows of 3 px dark bars, starting at x offset dx.
func inkPage(bg, ink color.RGBA, dx int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 640, 900))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bg.R, bg.G, bg.B, 255
	}
	for line := 0; line < 10; line++ {
		y0 := 100 + line*60
		for x := 60 + dx; x < 560+dx; x += 9 {
			fill(img, x, y0, 3, 14, ink)
		}
	}
	return img
}

func fill(img *image.RGBA, x0, y0, w, h int, c color.RGBA) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

var (
	paper  = color.RGBA{255, 255, 255, 255}
	black  = color.RGBA{20, 20, 20, 255}
	slate  = color.RGBA{40, 44, 52, 255}
	yellow = color.RGBA{255, 240, 60, 255}
)

func TestInkDiff(t *testing.T) {
	tick := func(img *image.RGBA, c color.RGBA) *image.RGBA {
		for i := 0; i < 12; i++ { // a 2 px stroke, 12 px long, on blank paper
			fill(img, 300+i, 70+i/2, 2, 2, c)
		}
		return img
	}
	heavier := inkPage(paper, black, 0)
	for line := 0; line < 10; line++ {
		for x := 60; x < 560; x += 9 {
			fill(heavier, x+3, 100+line*60, 1, 14, color.RGBA{120, 120, 120, 255})
		}
	}
	for _, tc := range []struct {
		name    string
		a, b    image.Image
		changed bool
	}{
		{"same page", inkPage(paper, black, 0), inkPage(paper, black, 0), false},
		{"text moved by a pixel", inkPage(paper, black, 0), inkPage(paper, black, 1), false},
		{"strokes drawn heavier", inkPage(paper, black, 0), heavier, false},
		{"pen tick on paper", inkPage(paper, black, 0), tick(inkPage(paper, black, 0), black), true},
		{"yellow highlight", inkPage(paper, black, 0), tick(inkPage(paper, black, 0), yellow), true},
		{"mark on a dark slide", inkPage(slate, paper, 0), tick(inkPage(slate, paper, 0), paper), true},
		{"text moved by 4 px", inkPage(paper, black, 0), inkPage(paper, black, 4), true},
	} {
		n, tile, ok := InkDiff(tc.a, tc.b)
		if !ok || tile == nil {
			t.Fatalf("%s: no result", tc.name)
		}
		if (n > InkTolerance) != tc.changed {
			t.Errorf("%s: %d changed pixels, want changed = %v", tc.name, n, tc.changed)
		}
	}
	if _, _, ok := InkDiff(image.NewRGBA(image.Rect(0, 0, 640, 900)), image.NewRGBA(image.Rect(0, 0, 900, 640))); ok {
		t.Error("pages of different sizes: want no result")
	}
}

func TestInkDiffColour(t *testing.T) {
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	band := func(square bool) *image.RGBA {
		img := inkPage(red, red, 0)
		if square { // 4 mm at 150 DPI
			fill(img, 300, 40, 24, 24, blue)
		}
		return img
	}
	for _, tc := range []struct {
		name    string
		a, b    image.Image
		changed bool
	}{
		{"red line to blue", inkPage(paper, red, 0), inkPage(paper, blue, 0), true},
		{"blue square on a red band", band(false), band(true), true},
		{"red line unchanged", inkPage(paper, red, 0), inkPage(paper, red, 0), false},
	} {
		n, _, ok := InkDiff(tc.a, tc.b)
		if !ok || (n > InkTolerance) != tc.changed {
			t.Errorf("%s: ok = %v, %d changed pixels, want changed = %v", tc.name, ok, n, tc.changed)
		}
	}
}

func TestInkDiffOnePixelSizeMismatch(t *testing.T) {
	crop := func(img *image.RGBA, w, h int) image.Image { return img.SubImage(image.Rect(0, 0, w, h)) }
	mark := func(img *image.RGBA) *image.RGBA {
		fill(img, 300, 40, 12, 12, black)
		return img
	}
	same, _, ok := InkDiff(crop(inkPage(paper, black, 0), 639, 900), crop(inkPage(paper, black, 0), 640, 899))
	if !ok || same > InkTolerance {
		t.Errorf("same content 1 px apart: ok = %v, %d changed pixels, want none", ok, same)
	}
	n, _, ok := InkDiff(crop(inkPage(paper, black, 0), 639, 900), crop(mark(inkPage(paper, black, 0)), 640, 899))
	if !ok || n <= InkTolerance {
		t.Errorf("real mark 1 px apart: ok = %v, %d changed pixels, want a change", ok, n)
	}
	if _, _, ok := InkDiff(crop(inkPage(paper, black, 0), 637, 900), inkPage(paper, black, 0)); ok {
		t.Error("pages 3 px apart: want no result")
	}
}

func BenchmarkInkDiff(b *testing.B) {
	p1, p2 := inkPage(paper, black, 0), inkPage(paper, black, 1)
	for b.Loop() {
		InkDiff(p1, p2)
	}
}

func BenchmarkTileDiff(b *testing.B) {
	p1, p2 := inkPage(paper, black, 0), inkPage(paper, black, 1)
	for b.Loop() {
		TileDiff(p1, p2)
	}
}
