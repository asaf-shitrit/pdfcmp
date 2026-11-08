package compare

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// layerPDF writes a one-page PDF with the given catalog and /Pages entries
// and content stream; object 5 is a form XObject /Fm in layer 6 (an OCG).
func layerPDF(t *testing.T, path, catalog, pagesExtra, content string) {
	t.Helper()
	var b bytes.Buffer
	var offs []int
	add := func(s string) { offs = append(offs, b.Len()); b.WriteString(s) }
	b.WriteString("%PDF-1.7\n")
	add(fmt.Sprintf("1 0 obj\n<< /Type /Catalog /Pages 2 0 R %s >>\nendobj\n", catalog))
	add(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 %s >>\nendobj\n", pagesExtra))
	add("3 0 obj\n<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 7 0 R >> /XObject << /Fm 5 0 R >> >> /Contents 4 0 R >>\nendobj\n")
	add(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content))
	form := "1 0 0 rg 100 100 300 300 re f"
	add(fmt.Sprintf("5 0 obj\n<< /Type /XObject /Subtype /Form /BBox [0 0 600 800] /OC 6 0 R /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(form), form))
	add("6 0 obj\n<< /Type /OCG /Name (DRAFT) >>\nendobj\n")
	add("7 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
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

// Two documents that differ only in a layer's default state, or in a box
// inherited from /Pages, render differently and must not be scored alike.
func TestLayerStateAndInheritedBoxesAreNotSimilar(t *testing.T) {
	const content = "BT /F1 24 Tf 30 400 Td (Hello world invoice 12345) Tj ET 0 0 1 rg 400 600 100 100 re f /Fm Do"
	ocg := func(state string) string {
		return "/OCProperties << /OCGs [6 0 R] /D << /" + state + " [6 0 R] >> >>"
	}
	const box = "/MediaBox [0 0 600 800]"
	for _, tc := range []struct {
		name                       string
		aCat, aPages, bCat, bPages string
	}{
		{"form layer on against off", ocg("ON"), box, ocg("OFF"), box},
		{"crop box on /Pages", "", box, "", box + " /CropBox [0 0 300 500]"},
		{"larger media box on /Pages", "", box, "", "/MediaBox [0 0 1200 1600]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
			layerPDF(t, a, tc.aCat, tc.aPages, content)
			layerPDF(t, b, tc.bCat, tc.bPages, content)
			c, err := New()
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			r, err := c.Compare(context.Background(), a, b)
			if err != nil {
				t.Fatal(err)
			}
			if r.IsSimilar(DefaultSimilarityThreshold) {
				t.Errorf("similar, want different: %s", r.Summary())
			}
		})
	}
}
