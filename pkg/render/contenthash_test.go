package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// figure writes a one-page PDF whose only content is a template, which
// gofpdf writes as a form object, holding a 16x16 image whose top-left pixel
// is shade; link, when not nil, adds a link annotation at that rectangle.
func figure(shade uint8, link *[4]float64) func(string) error {
	return func(path string) error {
		img := image.NewGray(image.Rect(0, 0, 16, 16))
		for i := range img.Pix {
			img.Pix[i] = 200
		}
		img.SetGray(0, 0, color.Gray{Y: shade})
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
		pdf := gofpdf.New("P", "mm", "A4", "")
		pdf.RegisterImageOptionsReader("fig", gofpdf.ImageOptions{ImageType: "PNG"}, &buf)
		tpl := pdf.CreateTemplate(func(t *gofpdf.Tpl) {
			t.ImageOptions("fig", 0, 0, 40, 40, false, gofpdf.ImageOptions{ImageType: "PNG"}, 0, "")
		})
		pdf.AddPage()
		pdf.UseTemplate(tpl)
		if link != nil {
			pdf.LinkString(link[0], link[1], link[2], link[3], "https://example.com")
		}
		return pdf.OutputFileAndClose(path)
	}
}

// An image inside a form object, where LaTeX puts included figures, and the
// page's annotations, which are drawn with it, are part of its content.
func TestContentHashFormsAndAnnotations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		a, b   func(string) error
		differ bool
	}{
		{"same figure", figure(200, nil), figure(200, nil), false},
		{"image in a form object edited", figure(200, nil), figure(0, nil), true},
		{"annotation added", figure(200, nil), figure(200, &[4]float64{10, 10, 30, 10}), true},
		{"annotation moved", figure(200, &[4]float64{10, 10, 30, 10}), figure(200, &[4]float64{10, 60, 30, 10}), true},
	} {
		h := contentHashes(t, tc.a, tc.b)
		if (h[0] != h[1]) != tc.differ {
			t.Errorf("%s: hashes %x and %x, want differ = %v", tc.name, h[0], h[1], tc.differ)
		}
	}
}
