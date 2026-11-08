package render

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// rawPDF writes a one-page PDF, with a media box of mediaBox, the given
// extra page entries and content stream; it has one font, /F1, and a 2x2
// gray image, /Im, for the content to draw.
func rawPDF(mediaBox, pageExtra, content string) func(string) error {
	return func(path string) error {
		var b bytes.Buffer
		var offs []int
		add := func(s string) { offs = append(offs, b.Len()); b.WriteString(s) }
		b.WriteString("%PDF-1.7\n")
		add("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
		add("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
		add(fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [%s] %s /Resources << /Font << /F1 5 0 R >> /XObject << /Im 6 0 R >> >> /Contents 4 0 R >>\nendobj\n", mediaBox, pageExtra))
		add(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content))
		add("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
		add("6 0 obj\n<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\xff\xff\xff\nendstream\nendobj\n")
		xref := b.Len()
		fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
		for _, o := range offs {
			fmt.Fprintf(&b, "%010d 00000 n \n", o)
		}
		fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, xref)
		return os.WriteFile(path, b.Bytes(), 0o644)
	}
}

const (
	hashText  = "BT /F1 24 Tf 30 400 Td (Hello world invoice 12345) Tj ET "
	hashRect  = "0 0 1 rg 30 100 200 100 re f"
	hashImage = "q 300 0 0 300 100 50 cm /Im Do Q"
	pageBox   = "0 0 600 800"
)

// Content that renders differently must not hash like the original, since
// the planner scores a page whose text, object count and content hash match
// as identical without rendering it: page rotation and boxes, an invisible
// text render mode, a clip path and a flipped image all keep the objects'
// type, bounds and colours.
func TestContentHashGeometryAndDrawingState(t *testing.T) {
	base := hashText + hashRect
	clipped := "0 0 10 10 re W n " + base
	for _, tc := range []struct {
		name   string
		a, b   func(string) error
		differ bool
	}{
		{"same page", rawPDF(pageBox, "", base), rawPDF(pageBox, "", base), false},
		{"same rotation and crop box", rawPDF(pageBox, "/Rotate 90 /CropBox [0 0 600 300]", base), rawPDF(pageBox, "/Rotate 90 /CropBox [0 0 600 300]", base), false},
		{"rotated 90", rawPDF(pageBox, "", base), rawPDF(pageBox, "/Rotate 90", base), true},
		{"rotated 180", rawPDF(pageBox, "", base), rawPDF(pageBox, "/Rotate 180", base), true},
		{"rotated 90 against 180", rawPDF(pageBox, "/Rotate 90", base), rawPDF(pageBox, "/Rotate 180", base), true},
		{"crop box hides the text", rawPDF(pageBox, "", base), rawPDF(pageBox, "/CropBox [0 0 600 300]", base), true},
		{"different crop box", rawPDF(pageBox, "/CropBox [0 0 600 300]", base), rawPDF(pageBox, "/CropBox [0 0 600 400]", base), true},
		{"different media box", rawPDF(pageBox, "", base), rawPDF("0 0 500 700", "", base), true},
		{"invisible text", rawPDF(pageBox, "", base), rawPDF(pageBox, "", "BT /F1 24 Tf 3 Tr 30 400 Td (Hello world invoice 12345) Tj ET "+hashRect), true},
		{"clip hides everything", rawPDF(pageBox, "", base), rawPDF(pageBox, "", clipped), true},
		{"different clip", rawPDF(pageBox, "", clipped), rawPDF(pageBox, "", "0 0 20 20 re W n "+base), true},
		{"same clip", rawPDF(pageBox, "", clipped), rawPDF(pageBox, "", clipped), false},
		{"same image", rawPDF(pageBox, "", hashText+hashImage), rawPDF(pageBox, "", hashText+hashImage), false},
		{"flipped image", rawPDF(pageBox, "", hashText+hashImage), rawPDF(pageBox, "", hashText+"q -300 0 0 -300 400 350 cm /Im Do Q"), true},
		// The same look written differently must hash the same, or the
		// planner renders the page for nothing.
		{"no crop box against crop box equal to media box", rawPDF(pageBox, "", base), rawPDF(pageBox, "/CropBox [0 0 600 800]", base), false},
		{"crop box larger than media box", rawPDF(pageBox, "", base), rawPDF(pageBox, "/CropBox [-50 -50 700 900]", base), false},
		{"crop box partly outside media box", rawPDF(pageBox, "/CropBox [0 0 600 300]", base), rawPDF(pageBox, "/CropBox [-50 0 700 300]", base), false},
		{"path drawn directly against under a scaling cm", rawPDF(pageBox, "", hashText+hashRect), rawPDF(pageBox, "", hashText+"0 0 1 rg q 2 0 0 2 0 0 cm 15 50 100 50 re f Q"), false},
		{"mirrored text", rawPDF(pageBox, "", "BT /F1 24 Tf 1 0 0 1 30 400 Tm (Hello) Tj ET"), rawPDF(pageBox, "", "BT /F1 24 Tf -1 0 0 1 30 400 Tm (Hello) Tj ET"), true},
	} {
		h := contentHashes(t, tc.a, tc.b)
		if (h[0] != h[1]) != tc.differ {
			t.Errorf("%s: hashes %x and %x, want differ = %v", tc.name, h[0], h[1], tc.differ)
		}
	}
}

// BenchmarkContentHash times the hash of one page with a form object, an
// image and an annotation, since it runs on every page of every document.
func BenchmarkContentHash(b *testing.B) {
	renderer, err := NewPdfiumRenderer()
	if err != nil {
		b.Fatal(err)
	}
	defer renderer.Close()
	path := filepath.Join(b.TempDir(), "f.pdf")
	if err := figure(200, &[4]float64{10, 10, 30, 10})(path); err != nil {
		b.Fatal(err)
	}
	doc, err := renderer.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer doc.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := doc.ContentHash(0); err != nil {
			b.Fatal(err)
		}
	}
}
