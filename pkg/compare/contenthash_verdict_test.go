package compare

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// verdictPDF writes a one-page PDF, 600x800 pt, with the given extra page
// entries and content stream; it has one font, /F1, and a 2x2 gray image,
// /Im, for the content to draw.
func verdictPDF(t *testing.T, path, pageExtra, content string) {
	t.Helper()
	var b bytes.Buffer
	var offs []int
	add := func(s string) { offs = append(offs, b.Len()); b.WriteString(s) }
	b.WriteString("%PDF-1.7\n")
	add("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	add("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	add(fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] %s /Resources << /Font << /F1 5 0 R >> /XObject << /Im 6 0 R >> >> /Contents 4 0 R >>\nendobj\n", pageExtra))
	add(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content))
	add("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	add("6 0 obj\n<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\xff\xff\xff\nendstream\nendobj\n")
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, xref)
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A page that looks different but has the same text, object count, object
// types, bounds and colours as the original must not be scored identical
// without rendering: rotated or cropped, invisible text, a clip that hides
// everything, a flipped image.
func TestVisiblyDifferentPagesWithSameObjectsAreDifferent(t *testing.T) {
	const text = "BT /F1 24 Tf 30 400 Td (Hello world invoice 12345) Tj ET "
	const rect = "0 0 1 rg 30 100 200 100 re f"
	const img = "q 300 0 0 300 100 50 cm /Im Do Q"
	for _, tc := range []struct {
		name                   string
		aExtra, aBody          string
		bExtra, bBody          string
		wantSimilar, wantEqual bool
	}{
		{"identical page", "", text + rect, "", text + rect, true, true},
		{"rotated 90", "", text + rect, "/Rotate 90", text + rect, false, false},
		{"rotated 180", "", text + rect, "/Rotate 180", text + rect, false, false},
		{"crop box hides the text", "", text + rect, "/CropBox [0 0 600 300]", text + rect, false, false},
		{"invisible text", "", text + rect, "", "BT /F1 24 Tf 3 Tr 30 400 Td (Hello world invoice 12345) Tj ET " + rect, false, false},
		{"clip hides the page", "", text + rect, "", "0 0 10 10 re W n " + text + rect, false, false},
		{"flipped image", "", text + img, "", text + "q -300 0 0 -300 400 350 cm /Im Do Q", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
			verdictPDF(t, a, tc.aExtra, tc.aBody)
			verdictPDF(t, b, tc.bExtra, tc.bBody)
			c, err := New()
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			r, err := c.Compare(context.Background(), a, b)
			if err != nil {
				t.Fatal(err)
			}
			if got := r.IsSimilar(DefaultSimilarityThreshold); got != tc.wantSimilar {
				t.Errorf("IsSimilar = %v, want %v: %s", got, tc.wantSimilar, r.Summary())
			}
			if tc.wantEqual && r.SimilarityScore != 1 {
				t.Errorf("identical pages scored %v, want 1", r.SimilarityScore)
			}
		})
	}
}
