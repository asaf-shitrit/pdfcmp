package fixtures

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"math/rand"

	"github.com/jung-kurt/gofpdf"
)

// ScanWidth and ScanHeight are the pixel size of a generated scan page.
const (
	ScanWidth  = 840
	ScanHeight = 1188
)

// Scans writes a PDF of image-only pages, as a scanner would: page(p) for
// each page index p, saved as a JPEG at the given quality and laid over an
// A4 page.
func Scans(path string, pages, quality int, page func(p int) image.Image) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	for p := 0; p < pages; p++ {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, page(p), &jpeg.Options{Quality: quality}); err != nil {
			return err
		}
		name := fmt.Sprintf("scan%d", p)
		pdf.RegisterImageOptionsReader(name, gofpdf.ImageOptions{ImageType: "JPG"}, &buf)
		pdf.AddPage()
		pdf.ImageOptions(name, 0, 0, 210, 297, false, gofpdf.ImageOptions{ImageType: "JPG"}, 0, "")
	}
	return pdf.OutputFileAndClose(path)
}

// ScanImage is a page of soft gradients, blobs and grain, seeded.
func ScanImage(seed int64) image.Image {
	r := rand.New(rand.NewSource(seed))
	type blob struct{ x, y, rad, c float64 }
	blobs := make([]blob, 12)
	for i := range blobs {
		blobs[i] = blob{r.Float64() * ScanWidth, r.Float64() * ScanHeight, 60 + r.Float64()*180, r.Float64()*160 - 80}
	}
	img := image.NewRGBA(image.Rect(0, 0, ScanWidth, ScanHeight))
	for y := 0; y < ScanHeight; y++ {
		for x := 0; x < ScanWidth; x++ {
			v := 200 - 60*float64(y)/ScanHeight + 20*math.Sin(float64(x)/90)
			for _, b := range blobs {
				d := math.Hypot(float64(x)-b.x, float64(y)-b.y)
				v += b.c * math.Exp(-d*d/(2*b.rad*b.rad))
			}
			v += r.NormFloat64() * 6
			g := uint8(math.Max(0, math.Min(255, v)))
			img.Set(x, y, color.RGBA{g, uint8(float64(g) * 0.96), uint8(float64(g) * 0.9), 255})
		}
	}
	return img
}

// StampImage draws a w x h px stamp, a dark outlined box with a filled bar
// inside, with its top left corner at (x, y) on a copy of img.
func StampImage(img image.Image, x, y, w, h int) image.Image {
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	dark := color.RGBA{20, 20, 20, 255}
	rect := func(x0, y0, x1, y1 int) {
		draw.Draw(out, image.Rect(x0, y0, x1, y1), image.NewUniform(dark), image.Point{}, draw.Src)
	}
	t := max(1, h/15)
	rect(x, y, x+w, y+t)
	rect(x, y+h-t, x+w, y+h)
	rect(x, y, x+t, y+h)
	rect(x+w-t, y, x+w, y+h)
	rect(x+w/9, y+h*22/60, x+w-w/9, y+h*38/60)
	return out
}

// RescanLevel sets how far a simulated rescan strays from the original.
type RescanLevel int

// The strength levels of Rescan. Heavy reaches the limits of a careful flatbed
// rescan of the same paper: a 6 px shift, 1 degree of rotation, 8% exposure.
const (
	RescanLight RescanLevel = iota
	RescanMedium
	RescanHeavy
)

// rescanParams are the largest deviations of a level; each is drawn between
// half of it and all of it, with a random sign.
type rescanParams struct {
	shift, rot, bright, contrast, noise, blur float64
	quality                                   int
}

var rescanLevels = [...]rescanParams{
	RescanLight:  {2, 0.25, 0.03, 0.03, 2, 0.4, 90},
	RescanMedium: {4, 0.6, 0.05, 0.05, 4, 0.7, 80},
	RescanHeavy:  {6, 1.0, 0.08, 0.08, 6, 1.0, 70},
}

// String names the level.
func (l RescanLevel) String() string { return [...]string{"light", "medium", "heavy"}[l] }

// RescanDeviation is what a simulated rescan did to its page: the shift in
// px, the rotation in radians, and the exposure as v' = Contrast*(v-128) +
// 128 + Bright.
type RescanDeviation struct {
	DX, DY, Rot      float64
	Bright, Contrast float64
}

// RescanDeviationOf is the deviation Rescan draws for a seed and level, and
// the random source it carries on with.
func RescanDeviationOf(seed int64, level RescanLevel) (RescanDeviation, *rand.Rand) {
	p := rescanLevels[level]
	r := rand.New(rand.NewSource(seed))
	pick := func(limit float64) float64 {
		v := limit * (0.5 + 0.5*r.Float64())
		if r.Intn(2) == 0 {
			return -v
		}
		return v
	}
	return RescanDeviation{
		DX: pick(p.shift), DY: pick(p.shift), Rot: pick(p.rot) * math.Pi / 180,
		Bright: pick(p.bright) * 255, Contrast: 1 + pick(p.contrast),
	}, r
}

// Rescan simulates scanning the same paper again: the page shifted and
// rotated about its centre, its exposure and contrast changed, blurred a
// little, grainier, and JPEG-encoded again. It is deterministic in seed.
// Pixels pulled in from outside the page take the nearest edge pixel.
func Rescan(img image.Image, seed int64, level RescanLevel) image.Image {
	p := rescanLevels[level]
	dev, r := RescanDeviationOf(seed, level)
	out := blurRGBA(warp(img, dev.DX, dev.DY, dev.Rot), p.blur)
	for i := 0; i+3 < len(out.Pix); i += 4 {
		n := r.NormFloat64() * p.noise
		for c := 0; c < 3; c++ {
			v := (float64(out.Pix[i+c])-128)*dev.Contrast + 128 + dev.Bright + n
			out.Pix[i+c] = uint8(math.Max(0, math.Min(255, v)))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: p.quality}); err != nil {
		panic(err)
	}
	dec, err := jpeg.Decode(&buf)
	if err != nil {
		panic(err)
	}
	return dec
}

// warp rotates img by rot radians about its centre and moves it by (dx, dy),
// sampling bilinearly.
func warp(img image.Image, dx, dy, rot float64) *image.RGBA {
	b := img.Bounds()
	src := image.NewRGBA(b)
	draw.Draw(src, b, img, b.Min, draw.Src)
	out := image.NewRGBA(b)
	cx, cy := float64(b.Dx())/2, float64(b.Dy())/2
	sin, cos := math.Sincos(rot)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			u, v := float64(x)-cx-dx, float64(y)-cy-dy
			sx, sy := cos*u+sin*v+cx, -sin*u+cos*v+cy
			copy(out.Pix[out.PixOffset(x, y):], bilinear(src, sx, sy))
		}
	}
	return out
}

// bilinear samples src at (x, y), clamped to the image.
func bilinear(src *image.RGBA, x, y float64) []uint8 {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	x = math.Max(0, math.Min(float64(w-1), x))
	y = math.Max(0, math.Min(float64(h-1), y))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, w-1), min(y0+1, h-1)
	fx, fy := x-float64(x0), y-float64(y0)
	px := make([]uint8, 4)
	for c := 0; c < 4; c++ {
		top := float64(src.Pix[src.PixOffset(x0, y0)+c])*(1-fx) + float64(src.Pix[src.PixOffset(x1, y0)+c])*fx
		bot := float64(src.Pix[src.PixOffset(x0, y1)+c])*(1-fx) + float64(src.Pix[src.PixOffset(x1, y1)+c])*fx
		px[c] = uint8(top*(1-fy) + bot*fy + 0.5)
	}
	return px
}

// blurRGBA blurs img with a separable Gaussian of the given sigma.
func blurRGBA(img *image.RGBA, sigma float64) *image.RGBA {
	rad := int(math.Ceil(sigma * 3))
	k := make([]float64, 2*rad+1)
	sum := 0.0
	for i := range k {
		d := float64(i - rad)
		k[i] = math.Exp(-d * d / (2 * sigma * sigma))
		sum += k[i]
	}
	for i := range k {
		k[i] /= sum
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	pass := func(in *image.RGBA, horizontal bool) *image.RGBA {
		out := image.NewRGBA(in.Rect)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var acc [3]float64
				for i, wt := range k {
					sx, sy := x, y
					if horizontal {
						sx = max(0, min(w-1, x+i-rad))
					} else {
						sy = max(0, min(h-1, y+i-rad))
					}
					o := in.PixOffset(sx, sy)
					for c := 0; c < 3; c++ {
						acc[c] += wt * float64(in.Pix[o+c])
					}
				}
				o := out.PixOffset(x, y)
				out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = uint8(acc[0]+0.5), uint8(acc[1]+0.5), uint8(acc[2]+0.5), 255
			}
		}
		return out
	}
	return pass(pass(img, true), false)
}

// MarkImage draws a dark stroke of the given thickness in px from (x0, y0) to
// (x1, y1) on a copy of img: a thin line, a tick (two strokes) or a
// handwritten-like squiggle are all strokes.
func MarkImage(img image.Image, thickness int, pts ...image.Point) image.Image {
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	dark := color.RGBA{20, 20, 20, 255}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		n := max(abs(b.X-a.X), abs(b.Y-a.Y), 1)
		for s := 0; s <= n; s++ {
			cx, cy := a.X+(b.X-a.X)*s/n, a.Y+(b.Y-a.Y)*s/n
			r := image.Rect(cx-thickness/2, cy-thickness/2, cx-thickness/2+thickness, cy-thickness/2+thickness)
			draw.Draw(out, r.Intersect(out.Bounds()), image.NewUniform(dark), image.Point{}, draw.Src)
		}
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TextScanImage is a page of printed text as a scanner sees it, seeded: pale
// paper with a faint uneven light, grain, and rows of dark glyph-like
// strokes 2 px wide. Unlike ScanImage, it is made of sharp edges.
func TextScanImage(seed int64) image.Image {
	r := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, ScanWidth, ScanHeight))
	for y := 0; y < ScanHeight; y++ {
		for x := 0; x < ScanWidth; x++ {
			v := 236 - 10*float64(y)/ScanHeight + 5*math.Sin(float64(x)/120) + r.NormFloat64()*3
			g := uint8(math.Max(0, math.Min(255, v)))
			img.Set(x, y, color.RGBA{g, g, uint8(float64(g) * 0.95), 255})
		}
	}
	ink := image.NewUniform(color.RGBA{35, 35, 38, 255})
	stroke := func(x0, y0, x1, y1 int) { draw.Draw(img, image.Rect(x0, y0, x1, y1), ink, image.Point{}, draw.Src) }
	for y := 70; y < ScanHeight-70; y += 26 {
		for x := 60; x < ScanWidth-80; {
			for n := 3 + r.Intn(6); n > 0 && x < ScanWidth-80; n-- { // a word of n glyphs
				switch r.Intn(3) {
				case 0:
					stroke(x, y, x+2, y+14)
				case 1:
					stroke(x, y, x+2, y+14)
					stroke(x, y+6, x+8, y+8)
					stroke(x+8, y, x+10, y+14)
				default:
					stroke(x, y+6, x+9, y+8)
					stroke(x, y+6, x+2, y+14)
					stroke(x, y+12, x+9, y+14)
				}
				x += 12
			}
			x += 14
		}
	}
	return img
}
