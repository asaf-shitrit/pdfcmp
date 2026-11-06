package visual

import (
	"image"
	"math"
)

const (
	// TileMargin is the number of tiles along each edge of the page that
	// AlignedTileDiff does not compare. A rescan sits a few pixels off and
	// leans, so content enters and leaves at the edge and nothing there has a
	// counterpart.
	TileMargin = 1

	// softEdge is the radius, in px, of the blur applied to a page before its
	// tiles are measured. A rescan is blurred a little differently, and a
	// blur moves ink across a tile's edge; the softer the edge, the less a
	// tile feels it. Blurring does not move ink within a tile, so a mark is
	// as strong as before.
	softEdge = 2

	// alignRange is the largest shift between two pages looked for, in
	// tiles: 2 tiles is about 26 px on an 840 px wide scan.
	alignRange = 2.0

	// alignSlack is how far, in tiles, a region of the page may sit from the
	// shift of the whole page, which is what a rotation of up to a degree
	// leaves behind.
	alignSlack = 0.8

	// alignRefine is the same, when the previous field is the place to start.
	alignRefine = 0.3

	// matchSlack is how far, in tiles, a tile may sit from where the shift
	// field puts it and still match. It is what the field's error and the
	// resampling of a rescan leave over, so it is kept small: every pixel of
	// slack lets a mark hide behind the page's own gradients.
	matchSlack = 0.14

	// shiftCap is the largest difference, in grey levels, one tile adds to
	// the cost of a shift.
	shiftCap = 30.0

	// settled is a difference, in grey levels, small enough that a tile
	// needs no closer look for a better match.
	settled = 3.0

	// maxGain and maxBias bound the exposure correction: a rescan's contrast
	// and brightness move by under about 10%, and a page that differs more
	// than that is a different page, not a different exposure.
	maxGain = 1.25
	maxBias = 40.0
)

// integral is a summed-area table of a page's grey levels, so that the mean
// of any rectangle, at pixel or fractional coordinates, costs a few lookups.
type integral struct {
	w, h int
	s    []float64 // (w+1) x (h+1)
}

// grayPlane returns the grey levels of img as a w x h plane in row-major
// order with the given stride. Gray pages are read in place and RGBA pages,
// which is what the renderer produces, are converted with the luma weights
// TileMeans uses; any other image is converted once.
func grayPlane(img image.Image) (pix []uint8, stride int) {
	b := img.Bounds()
	switch m := img.(type) {
	case *image.Gray:
		return m.Pix[m.PixOffset(b.Min.X, b.Min.Y):], m.Stride
	case *image.RGBA:
		return lumaPlane(m.Pix[m.PixOffset(b.Min.X, b.Min.Y):], m.Stride, b.Dx(), b.Dy())
	case *image.NRGBA:
		return lumaPlane(m.Pix[m.PixOffset(b.Min.X, b.Min.Y):], m.Stride, b.Dx(), b.Dy())
	}
	return grayPlane(ToGrayscale(img))
}

func lumaPlane(src []uint8, stride, w, h int) ([]uint8, int) {
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		row := src[y*stride : y*stride+4*w]
		for x := 0; x < w; x++ {
			out[y*w+x] = uint8((77*int(row[4*x]) + 150*int(row[4*x+1]) + 29*int(row[4*x+2])) >> 8)
		}
	}
	return out, w
}

// newIntegral builds the summed-area table of img after averaging every
// pixel with those within softEdge px of it (taking the nearest edge pixel
// for any outside the page).
func newIntegral(img image.Image) *integral {
	pix, stride := grayPlane(img)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	r, n := softEdge, 2*softEdge+1
	clamp := func(i, size int) int { return max(0, min(size-1, i)) }

	hs := make([]int, w*h) // sums of n pixels along each row
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+w]
		sum := 0
		for k := -r; k <= r; k++ {
			sum += int(row[clamp(k, w)])
		}
		for x := 0; x < w; x++ {
			hs[y*w+x] = sum
			sum += int(row[clamp(x+r+1, w)]) - int(row[clamp(x-r, w)])
		}
	}

	in := &integral{w: w, h: h, s: make([]float64, (w+1)*(h+1))}
	col := make([]int, w) // sums of n rows of hs
	for k := -r; k <= r; k++ {
		for x, v := range hs[clamp(k, h)*w : clamp(k, h)*w+w] {
			col[x] += v
		}
	}
	scale := 1 / float64(n*n)
	for y := 0; y < h; y++ {
		run := 0.0
		above, out := in.s[y*(w+1):y*(w+1)+w+1], in.s[(y+1)*(w+1):(y+2)*(w+1)]
		for x := 0; x < w; x++ {
			run += float64(col[x]) * scale
			out[x+1] = above[x+1] + run
		}
		add, drop := hs[clamp(y+r+1, h)*w:], hs[clamp(y-r, h)*w:]
		for x := 0; x < w; x++ {
			col[x] += add[x] - drop[x]
		}
	}
	return in
}

// at is the summed area from the origin to (x, y), interpolated between
// pixel corners, which is exact for a page of constant pixels.
func (in *integral) at(x, y float64) float64 {
	x0, y0 := int(x), int(y)
	fx, fy := x-float64(x0), y-float64(y0)
	x1, y1 := min(x0+1, in.w), min(y0+1, in.h)
	w := in.w + 1
	top := in.s[y0*w+x0]*(1-fx) + in.s[y0*w+x1]*fx
	bot := in.s[y1*w+x0]*(1-fx) + in.s[y1*w+x1]*fx
	return top*(1-fy) + bot*fy
}

// mean is the mean grey level of the rectangle [x0,x1) x [y0,y1), and false
// when it is not inside the page.
func (in *integral) mean(x0, y0, x1, y1 float64) (float64, bool) {
	if x0 < 0 || y0 < 0 || x1 > float64(in.w) || y1 > float64(in.h) {
		return 0, false
	}
	total := in.at(x1, y1) - in.at(x0, y1) - in.at(x1, y0) + in.at(x0, y0)
	return total / ((x1 - x0) * (y1 - y0)), true
}

// meanInt is mean for whole pixel coordinates, four lookups.
func (in *integral) meanInt(x0, y0, x1, y1 int) (float64, bool) {
	if x0 < 0 || y0 < 0 || x1 > in.w || y1 > in.h || x1 <= x0 || y1 <= y0 {
		return 0, false
	}
	w := in.w + 1
	total := in.s[y1*w+x1] - in.s[y0*w+x1] - in.s[y1*w+x0] + in.s[y0*w+x0]
	return total / float64((x1-x0)*(y1-y0)), true
}

// tiling is the tile grid of a page w x h px.
type tiling struct {
	cols, rows int
	tw, th     float64 // tile size in px
}

func newTiling(w, h int) tiling {
	cols, rows := tileGrid(w, h)
	return tiling{cols, rows, float64(w) / float64(cols), float64(h) / float64(rows)}
}

// rect is the pixel rectangle of tile (c, r) moved by (dx, dy) px.
func (t tiling) rect(c, r int, dx, dy float64) (x0, y0, x1, y1 float64) {
	x0, y0 = float64(c)*t.tw+dx, float64(r)*t.th+dy
	return x0, y0, x0 + t.tw, y0 + t.th
}

// each calls fn for every tile outside the margin.
func (t tiling) each(fn func(c, r int)) {
	for r := TileMargin; r < t.rows-TileMargin; r++ {
		for c := TileMargin; c < t.cols-TileMargin; c++ {
			fn(c, r)
		}
	}
}

// shiftField is how far the second page sits from the first, in px, at a
// point (x, y) of the first: dx = ax + bx*(y-cy), dy = ay + by*(x-cx). A
// translation plus a small rotation, which turns into shifts that grow with
// the distance from the centre.
type shiftField struct {
	ax, bx, ay, by float64
	cx, cy         float64
}

// at is the shift at (x, y).
func (f shiftField) at(x, y float64) (dx, dy float64) {
	return f.ax + f.bx*(y-f.cy), f.ay + f.by*(x-f.cx)
}

// neg is the field seen from the other page.
func (f shiftField) neg() shiftField {
	return shiftField{-f.ax, -f.bx, -f.ay, -f.by, f.cx, f.cy}
}

// exposure maps a grey level of the first page to the second's:
// gain*v + bias, clipped like a scanner would.
type exposure struct{ gain, bias float64 }

func (e exposure) apply(v float64) float64 {
	v = e.gain*v + e.bias
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// diff is the difference between a first-page tile mean v, exposed, and a
// second-page tile mean u. Near white and black the scanner clips, and two
// clipped tiles are the same however far the straight line would take one.
func (e exposure) diff(v, u float64) float64 {
	p := e.apply(v)
	if (p >= 250 && u >= 245) || (p <= 5 && u <= 10) {
		return 0
	}
	return math.Abs(p - u)
}

// AlignedTileDiff is TileDiff for two scans of the same paper, which are
// not the same picture: the second sits a few pixels off, leans by a
// fraction of a degree and is exposed a little differently. So the second
// page's exposure is fitted to the first's (see fitExposure), the shift
// between the pages is found (see estimateShift), and a tile is compared
// with the closest tile within a pixel or so of where it should be, in both
// directions, so that a mark on either page is found. The outer TileMargin
// tiles are not compared. The pages must be the same size; otherwise the
// result is TileDiff's.
//
// Do not use it on rendered text: it forgives a page whose content moved by
// a few pixels, which is a real change there.
func AlignedTileDiff(img1, img2 image.Image) (float64, *Tile) {
	b1, b2 := img1.Bounds(), img2.Bounds()
	t := newTiling(b1.Dx(), b1.Dy())
	if b1.Dx() != b2.Dx() || b1.Dy() != b2.Dy() || b1.Dx() < t.cols || b1.Dy() < t.rows {
		return TileDiff(img1, img2)
	}
	a, b := newIntegral(img1), newIntegral(img2)
	e, f := align(a, b, t)
	d1, i1 := worstTile(a, b, t, e, f, false)
	d2, i2 := worstTile(b, a, t, e, f.neg(), true)
	if d2 > d1 {
		d1, i1 = d2, i2
	}
	return d1, &Tile{Col: i1 % t.cols, Row: i1 / t.cols, Cols: t.cols, Rows: t.rows}
}

// align fits the exposure, finds the shift field, and fits both again on
// the tiles that now line up, since a shifted page fits poorly.
func align(a, b *integral, t tiling) (exposure, shiftField) {
	var f shiftField
	e := fitExposure(tileMeans(a, t, f), tileMeans(b, t, f))
	f = estimateShift(a, b, t, e, nil)
	e = fitExposure(tileMeans(a, t, shiftField{}), tileMeans(b, t, f))
	f = estimateShift(a, b, t, e, &f)
	return fitExposure(tileMeans(a, t, shiftField{}), tileMeans(b, t, f)), f
}

// tileMeans is the mean of every tile outside the margin, row by row, each
// taken where the shift field puts it. Tiles that fall off the page are NaN.
func tileMeans(in *integral, t tiling, f shiftField) []float64 {
	var out []float64
	t.each(func(c, r int) {
		dx, dy := f.at(float64(c)*t.tw, float64(r)*t.th)
		v, ok := in.mean(t.rect(c, r, dx, dy))
		if !ok {
			v = math.NaN()
		}
		out = append(out, v)
	})
	return out
}

// worstTile returns the largest difference between a tile of q and the
// closest tile of o near where the shift field puts it, and the index of that
// tile. The exposure maps the first page to the second: q is the second
// page when swapped is true.
func worstTile(q, o *integral, t tiling, e exposure, f shiftField, swapped bool) (float64, int) {
	worst, idx := 0.0, 0
	t.each(func(c, r int) {
		v, _ := q.mean(t.rect(c, r, 0, 0))
		dx, dy := f.at(float64(c)*t.tw, float64(r)*t.th)
		if d := bestMatch(o, t, c, r, dx, dy, func(u float64) float64 {
			if swapped {
				return e.diff(u, v)
			}
			return e.diff(v, u)
		}); d > worst && d < math.MaxFloat64 {
			worst, idx = d, r*t.cols+c
		}
	})
	return worst, idx
}

// bestMatch is the smallest cost of the tile (c, r) of o moved to near
// (dx, dy) px. It tries that shift, and when it is not already a match, walks
// to a better one: three rounds over the 8 neighbours, each at half the step
// of the one before, which reaches matchSlack tiles from where it started.
func bestMatch(o *integral, t tiling, c, r int, dx, dy float64, cost func(u float64) float64) float64 {
	best := math.MaxFloat64
	try := func(ox, oy float64) float64 {
		d := math.MaxFloat64
		if u, ok := o.mean(t.rect(c, r, dx+ox, dy+oy)); ok {
			d = cost(u)
		}
		best = math.Min(best, d)
		return d
	}
	at := try(0, 0)
	cx, cy := 0.0, 0.0
	for step := matchSlack / 1.75; step >= matchSlack/7 && best > settled; step /= 2 {
		sx, sy := step*t.tw, step*t.th
		bx, by := cx, cy
		for j := -1; j <= 1; j++ {
			for i := -1; i <= 1; i++ {
				if i == 0 && j == 0 {
					continue
				}
				if d := try(cx+float64(i)*sx, cy+float64(j)*sy); d < at {
					at, bx, by = d, cx+float64(i)*sx, cy+float64(j)*sy
				}
			}
		}
		cx, cy = bx, by
	}
	return best
}

// estimateShift finds the shift field of b against a. Without a previous
// field it looks for the translation of the whole page first; then, for each
// of 3x3 regions, for the translation near that (or near the previous
// field's at the region's centre), and fits the field to those.
func estimateShift(a, b *integral, t tiling, e exposure, prev *shiftField) shiftField {
	w, h := float64(a.w), float64(a.h)
	slack := alignRefine
	var gx, gy float64
	if prev == nil {
		slack = alignSlack
		gx, gy = bestShift(a, b, t, e, region{0, 0, w, h}, 0, 0, alignRange*t.tw, alignRange*t.th, t.tw/2, t.th/2)
	}
	const n = 3
	var xs, ys, dxs, dys []float64
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			reg := region{w * float64(i) / n, h * float64(j) / n, w * float64(i+1) / n, h * float64(j+1) / n}
			x, y := (reg.x0+reg.x1)/2, (reg.y0+reg.y1)/2
			if prev != nil {
				gx, gy = prev.at(x, y)
			}
			dx, dy := bestShift(a, b, t, e, reg, gx, gy, slack*t.tw, slack*t.th, t.tw/4, t.th/4)
			dx, dy = bestShift(a, b, t, e, reg, dx, dy, t.tw/4, t.th/4, 1, 1)
			xs, ys = append(xs, x), append(ys, y)
			dxs, dys = append(dxs, dx), append(dys, dy)
		}
	}
	return fitField(xs, ys, dxs, dys, w/2, h/2)
}

// fitField fits dx against y and dy against x by least squares.
func fitField(xs, ys, dxs, dys []float64, cx, cy float64) shiftField {
	all := make([]bool, len(xs))
	cxs, cys := make([]float64, len(xs)), make([]float64, len(xs))
	for i := range xs {
		all[i], cxs[i], cys[i] = true, xs[i]-cx, ys[i]-cy
	}
	bx, ax := olsLine(cys, dxs, all)
	by, ay := olsLine(cxs, dys, all)
	return shiftField{ax: ax, bx: bx, ay: ay, by: by, cx: cx, cy: cy}
}

// region is a block of the page in px, [x0,x1) x [y0,y1).
type region struct{ x0, y0, x1, y1 float64 }

// bestShift finds the shift of b against a, within (rx, ry) px of (cx, cy)
// in steps of (sx, sy), that makes the tiles in reg agree best.
func bestShift(a, b *integral, t tiling, e exposure, reg region, cx, cy, rx, ry, sx, sy float64) (dx, dy float64) {
	best := math.MaxFloat64
	nx, ny := int(rx/sx), int(ry/sy)
	for j := -ny; j <= ny; j++ {
		for i := -nx; i <= nx; i++ {
			ox, oy := math.Round(cx+float64(i)*sx), math.Round(cy+float64(j)*sy)
			if d := shiftCost(a, b, t, e, reg, ox, oy); d < best {
				best, dx, dy = d, ox, oy
			}
		}
	}
	return dx, dy
}

// shiftCost is how badly the tiles of a in reg and the tiles of b moved by
// (ox, oy) px disagree, on every 3rd tile in each direction (a mark moves a
// few tiles, so a sample barely feels it): the variance of their difference
// in grey levels. Its mean is left out, so that a wrong exposure does not
// pull the shift off, and each difference is capped at shiftCap so that a
// mark does not either. It works in whole pixels, which is all the search
// needs and four times cheaper: the shift is rounded by the caller.
func shiftCost(a, b *integral, t tiling, e exposure, reg region, ox, oy float64) float64 {
	var sum, sum2 float64
	n := 0
	px, py := int(ox), int(oy)
	for r := 0; r < t.rows; r += 3 {
		for c := 0; c < t.cols; c += 3 {
			fx0, fy0, fx1, fy1 := t.rect(c, r, 0, 0)
			if fx0 < reg.x0 || fy0 < reg.y0 || fx1 > reg.x1 || fy1 > reg.y1 {
				continue
			}
			x0, y0, x1, y1 := int(math.Round(fx0)), int(math.Round(fy0)), int(math.Round(fx1)), int(math.Round(fy1))
			va, _ := a.meanInt(x0, y0, x1, y1)
			if vb, ok := b.meanInt(x0+px, y0+py, x1+px, y1+py); ok {
				d := math.Max(-shiftCap, math.Min(shiftCap, e.apply(va)-vb))
				sum, sum2, n = sum+d, sum2+d*d, n+1
			}
		}
	}
	if n == 0 {
		return math.MaxFloat64
	}
	mean := sum / float64(n)
	return sum2/float64(n) - mean*mean
}

// fitExposure finds the gain and bias that map the tile means x of one page
// onto those of the other, y ~ gain*x + bias, ignoring tiles that do not
// follow the fit so that a mark does not pull it, and tiles that are
// clipped, where the exposure is no longer a straight line. Both are limited
// (maxGain, maxBias); with no contrast in x the gain is 1.
func fitExposure(x, y []float64) exposure {
	keep := make([]bool, len(x))
	for i := range keep {
		keep[i] = !clipped(x[i]) && !clipped(y[i])
	}
	e := exposure{gain: 1}
	for iter := 0; iter < 3; iter++ {
		gain, _ := olsLine(x, y, keep)
		e.gain = math.Max(1/maxGain, math.Min(maxGain, gain))
		e.bias = math.Max(-maxBias, math.Min(maxBias, meanOf(y, keep)-e.gain*meanOf(x, keep)))
		rms := residualRMS(x, y, keep, e)
		for i := range keep {
			keep[i] = !clipped(x[i]) && !clipped(y[i]) && math.Abs(y[i]-e.gain*x[i]-e.bias) <= math.Max(3, 2*rms)
		}
	}
	return e
}

// clipped reports whether a mean grey level is near white or black (or
// missing), where a scanner clips.
func clipped(v float64) bool { return !(v >= 8 && v <= 245) }

func meanOf(v []float64, keep []bool) float64 {
	sum, n := 0.0, 0
	for i, k := range keep {
		if k {
			sum += v[i]
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// olsLine is the least squares line through the kept points, or the line of
// gain 1 when x has no spread.
func olsLine(x, y []float64, keep []bool) (gain, bias float64) {
	mx, my := meanOf(x, keep), meanOf(y, keep)
	var sxy, sxx float64
	for i, k := range keep {
		if k {
			sxy += (x[i] - mx) * (y[i] - my)
			sxx += (x[i] - mx) * (x[i] - mx)
		}
	}
	if sxx < 1e-9 {
		return 1, my - mx
	}
	gain = sxy / sxx
	return gain, my - gain*mx
}

func residualRMS(x, y []float64, keep []bool, e exposure) float64 {
	sum, n := 0.0, 0
	for i, k := range keep {
		if k {
			r := y[i] - e.gain*x[i] - e.bias
			sum += r * r
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(sum / float64(n))
}
