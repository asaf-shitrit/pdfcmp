package visual

import (
	"image"
	"math"
)

const (
	// TileCols and TileRows are the tile grid of a portrait page. A
	// landscape page is cut the other way round, so a tile is about 1/64 of
	// the long side wide.
	TileCols = 64
	TileRows = 90

	// TileScale is the mean-intensity difference, in grey levels, that
	// scores a tile 0. A tile scores 1 - diff/TileScale, so the default
	// threshold of 0.95 falls at a difference of 6 grey levels.
	//
	// Measured through pdfium on scans of eight seeds, re-encoding between
	// JPEG quality 92 and 50 moved no tile by more than 2.1 grey levels at
	// 72, 150 and 300 DPI, while a stamp of 60x30 px or more on an 840x1188 px
	// scan moved one tile by at least 16 (TestTileSeparation checks the same
	// split on decoded JPEGs).
	TileScale = 120.0
)

// Tile is one cell of a page's tile grid.
type Tile struct {
	Col, Row   int
	Cols, Rows int
}

// TileMeans cuts a page into a cols x rows grid of tiles and returns the
// mean grey level of each, row by row. Tiles are fractions of the page, so
// the result does not depend on the render DPI. It returns nil when the page
// has fewer pixels than tiles.
//
// A tile averages thousands of pixels, so JPEG noise and ringing wash out
// of it while a local edit moves it. The perceptual hashes cannot see such
// an edit: they reduce the whole page to 9x8 or 32x32 samples.
func TileMeans(img image.Image, cols, rows int) []float64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < cols || h < rows {
		return nil
	}
	sum := rowSummer(img)
	means := make([]float64, cols*rows)
	for r := 0; r < rows; r++ {
		y0, y1 := r*h/rows, (r+1)*h/rows
		for c := 0; c < cols; c++ {
			x0, x1 := c*w/cols, (c+1)*w/cols
			total := 0
			for y := y0; y < y1; y++ {
				total += sum(b.Min.X+x0, b.Min.X+x1, b.Min.Y+y)
			}
			means[r*cols+c] = float64(total) / float64((x1-x0)*(y1-y0))
		}
	}
	return means
}

// rowSummer returns a function that adds up the grey levels of the pixels
// [x0,x1) of row y. Gray and RGBA pages, which is what the renderer
// produces, are read in place; any other image is converted once.
func rowSummer(img image.Image) func(x0, x1, y int) int {
	switch m := img.(type) {
	case *image.Gray:
		return func(x0, x1, y int) int {
			sum := 0
			for _, v := range m.Pix[m.PixOffset(x0, y):m.PixOffset(x1, y)] {
				sum += int(v)
			}
			return sum
		}
	case *image.RGBA:
		return func(x0, x1, y int) int { return rgbaRowSum(m.Pix[m.PixOffset(x0, y):m.PixOffset(x1, y)]) }
	case *image.NRGBA:
		return func(x0, x1, y int) int { return rgbaRowSum(m.Pix[m.PixOffset(x0, y):m.PixOffset(x1, y)]) }
	}
	return rowSummer(ToGrayscale(img))
}

// rgbaRowSum adds up the luma, 0.299R + 0.587G + 0.114B, of 4-byte pixels.
func rgbaRowSum(pix []uint8) int {
	sum := 0
	for i := 0; i+3 < len(pix); i += 4 {
		sum += (77*int(pix[i]) + 150*int(pix[i+1]) + 29*int(pix[i+2])) >> 8
	}
	return sum
}

// tileGrid returns the tile grid for a page of w x h pixels.
func tileGrid(w, h int) (cols, rows int) {
	if w > h {
		return TileRows, TileCols
	}
	return TileCols, TileRows
}

// TileSimilarity scores the pages by their worst tile: 1 - d/TileScale for
// the largest difference d between two tiles' mean grey levels, floored at 0.
// Both pages are cut on the grid of the first, so a portrait page against a
// landscape one differs everywhere. It returns the worst tile, and
// similarity 1 with no tile when a page is too small to cut into tiles.
func TileSimilarity(img1, img2 image.Image) (float64, *Tile) {
	cols, rows := tileGrid(img1.Bounds().Dx(), img1.Bounds().Dy())
	m1, m2 := TileMeans(img1, cols, rows), TileMeans(img2, cols, rows)
	if m1 == nil || m2 == nil {
		return 1, nil
	}
	worst, idx := 0.0, 0
	for i := range m1 {
		if d := math.Abs(m1[i] - m2[i]); d > worst {
			worst, idx = d, i
		}
	}
	return math.Max(0, 1-worst/TileScale), &Tile{Col: idx % cols, Row: idx / cols, Cols: cols, Rows: rows}
}
