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
