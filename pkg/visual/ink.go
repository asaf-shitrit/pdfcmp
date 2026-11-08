package visual

import (
	"image"
	"math"
)

const (
	// InkContrast is how far, in levels of a colour channel, a pixel must
	// stand from a flat area of the other page to count as changed: new
	// ink, or ink gone.
	InkContrast = 64

	// inkFlat is the most the pixels of an area may differ, in levels of a
	// colour channel, and the area still count as flat: blank paper, a
	// tinted background, the inside of a filled shape.
	inkFlat = 24

	// InkTolerance is the most changed pixels a block of 2x2 tiles may hold
	// and still count as the same. A pixel changed when it stands out from
	// the flat area around the same place on the other page (see InkDiff).
	// Two renders of the same content written by different tools differ by
	// sub-pixel moves, the weight of hairlines and the grey of form-field
	// borders, all at edges, where the other page is not flat; Ghostscript
	// and MuPDF rewrites of eight real documents (forms, papers, a 230-page
	// manual) changed at most 13 pixels of a block at 150 DPI. A 6pt pen
	// tick on paper changed 16 to 36, two swapped digits 18 to 103.
	InkTolerance = 15

	// InkScale is the changed-pixel count that scores a block 0:
	// InkTolerance scores 0.95, as TileTolerance does for TileScore.
	InkScale = 20 * InkTolerance

	// PhotoTolerance is the largest difference, in grey levels, between two
	// tiles' mean intensities that InkDiff's verdict stands against. Inside
	// a photo nothing is flat, so InkDiff sees no edit there; a tile whose
	// mean moved by more than this changed whatever InkDiff says. Rewrites
	// and rasterised copies of eight real documents moved no tile by more
	// than 23 at 150 DPI.
	PhotoTolerance = 30.0
)

// InkDiff finds where ink appeared or disappeared between two renders of a
// born-digital page and returns how many pixels changed in the worst block
// of 2x2 tiles, and its top-left tile. A pixel changed when the pixels
// around the same place on the other page, one pixel each way, are flat and
// it stands at least InkContrast from them, in either direction, on any
// colour channel (red, green or blue): dark ink on paper, a yellow
// highlight, light text on a dark slide. Edges that moved by a pixel, or
// lines drawn a little heavier, sit next to edges on the other page and do
// not count. Pages up to maxSizeSlack pixels apart in size are compared on their common
// top-left area. It reports false, and no tile, for pages of other sizes or
// too small to cut into tiles.
//
// Use it on rendered pages, not scans: a rescan moves by more than a pixel
// (see AlignedTileDiff).
func InkDiff(img1, img2 image.Image) (int, *Tile, bool) {
	w, h, ok := commonSize(img1, img2)
	if !ok {
		return 0, nil, false
	}
	cols, rows := tileGrid(w, h)
	if w < cols || h < rows {
		return 0, nil, false
	}
	p1, p2 := matchPlanes(colourPlanes(img1, w, h), colourPlanes(img2, w, h))
	counts := make([]int, cols*rows)
	for y := 0; y < h; y++ {
		r := y * rows / h
		for x := 0; x < w; x++ {
			for k := range p1 {
				if inkChanged(p1[k], p2[k], w, h, x, y) {
					counts[r*cols+x*cols/w]++
					break
				}
			}
		}
	}
	// A mark on the corner of four tiles is split between them, so tiles
	// are counted in overlapping blocks of 2x2.
	worst, idx := 0, 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			n := counts[r*cols+c]
			if c+1 < cols {
				n += counts[r*cols+c+1]
			}
			if r+1 < rows {
				n += counts[(r+1)*cols+c]
				if c+1 < cols {
					n += counts[(r+1)*cols+c+1]
				}
			}
			if n > worst {
				worst, idx = n, r*cols+c
			}
		}
	}
	return worst, &Tile{Col: idx % cols, Row: idx / cols, Cols: cols, Rows: rows}, true
}

// inkChanged reports whether pixel (x, y) of either plane stands InkContrast
// or more from a flat neighbourhood of the other. A pixel that does is at
// least InkContrast-inkFlat from the other plane's pixel at the same place,
// which rules out almost every pixel before its neighbourhood is read.
func inkChanged(a, b []uint8, w, h, x, y int) bool {
	i := y*w + x
	if d := int(a[i]) - int(b[i]); d < InkContrast-inkFlat && d > inkFlat-InkContrast {
		return false
	}
	return standsOut(a[i], b, w, h, x, y) || standsOut(b[i], a, w, h, x, y)
}

// standsOut reports whether the pixels of p within one pixel of (x, y) are
// flat and v stands InkContrast or more from them.
func standsOut(v uint8, p []uint8, w, h, x, y int) bool {
	lo, hi := uint8(255), uint8(0)
	for yy := max(0, y-1); yy <= min(h-1, y+1); yy++ {
		row := p[yy*w:]
		for xx := max(0, x-1); xx <= min(w-1, x+1); xx++ {
			lo, hi = min(lo, row[xx]), max(hi, row[xx])
		}
	}
	if int(hi)-int(lo) > inkFlat {
		return false
	}
	return int(v)+InkContrast <= int(lo) || int(v) >= int(hi)+InkContrast
}

// maxSizeSlack is the most pixels, in either direction, that two pages may
// differ in size and still be compared on their common top-left area: the
// same A4 page is 595.28x841.89 pt in one writer and 595x842 in another,
// which renders a pixel apart. The rest of the larger page is a one-pixel
// margin of paper.
const maxSizeSlack = 2

// commonSize returns the size of the area both images share, and false when
// they differ by more than maxSizeSlack in either direction.
func commonSize(img1, img2 image.Image) (w, h int, ok bool) {
	b1, b2 := img1.Bounds(), img2.Bounds()
	if absInt(b1.Dx()-b2.Dx()) > maxSizeSlack || absInt(b1.Dy()-b2.Dy()) > maxSizeSlack {
		return 0, 0, false
	}
	return min(b1.Dx(), b2.Dx()), min(b1.Dy(), b2.Dy()), true
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// matchPlanes gives two sets of planes the same number of planes: a grey
// image has one, a colour image three, and the grey one is compared on each.
func matchPlanes(a, b [][]uint8) ([][]uint8, [][]uint8) {
	for len(a) < len(b) {
		a = append(a, a[0])
	}
	for len(b) < len(a) {
		b = append(b, b[0])
	}
	return a, b
}

// colourPlanes returns the top-left w x h area of img as planes of row-by-row
// levels: one for a grey image, the red, green and blue channels otherwise.
// Each channel is judged on its own, so ink that changes hue while keeping its
// darkest and brightest channel, red to blue, still moves a plane.
func colourPlanes(img image.Image, w, h int) [][]uint8 {
	b := img.Bounds()
	switch m := img.(type) {
	case *image.Gray:
		p := make([]uint8, w*h)
		for y := 0; y < h; y++ {
			copy(p[y*w:(y+1)*w], m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):])
		}
		return [][]uint8{p}
	case *image.RGBA:
		r, g, bl := make([]uint8, w*h), make([]uint8, w*h), make([]uint8, w*h)
		for y := 0; y < h; y++ {
			row := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
			for x := 0; x < w; x++ {
				i := y*w + x
				r[i], g[i], bl[i] = row[4*x], row[4*x+1], row[4*x+2]
			}
		}
		return [][]uint8{r, g, bl}
	}
	r, g, bl := make([]uint8, w*h), make([]uint8, w*h), make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pr, pg, pb, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := y*w + x
			r[i], g[i], bl[i] = uint8(pr>>8), uint8(pg>>8), uint8(pb>>8)
		}
	}
	return [][]uint8{r, g, bl}
}

// InkScore converts a changed-pixel count to a score from 0 to 1:
// 1 - n/InkScale, so InkTolerance scores 0.95.
func InkScore(n int) float64 { return math.Max(0, 1-float64(n)/InkScale) }

// ChangedPixels counts the pixels that differ by InkContrast or more on a
// colour channel between two renders. It reports false for pages of
// different sizes.
func ChangedPixels(img1, img2 image.Image) (int, bool) {
	b1, b2 := img1.Bounds(), img2.Bounds()
	if b1.Dx() != b2.Dx() || b1.Dy() != b2.Dy() {
		return 0, false
	}
	p1, p2 := matchPlanes(colourPlanes(img1, b1.Dx(), b1.Dy()), colourPlanes(img2, b2.Dx(), b2.Dy()))
	n := 0
	for i := range p1[0] {
		for k := range p1 {
			if absDiff(p1[k][i], p2[k][i]) >= InkContrast {
				n++
				break
			}
		}
	}
	return n, true
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
