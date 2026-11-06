package render

import (
	"path/filepath"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// writeLines writes a one-page PDF with one text line per element of lines.
func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	for i, l := range lines {
		pdf.Text(20, float64(30+10*i), l)
	}
	if err := pdf.OutputFileAndClose(path); err != nil {
		t.Fatal(err)
	}
}

func pageInfo(t *testing.T, path string) PageInfo {
	t.Helper()
	r, err := NewPdfiumRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	doc, err := r.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	info, err := doc.PageInfo(0)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestTextHashIgnoresWhitespaceNoise(t *testing.T) {
	dir := t.TempDir()
	clean, noisy, digit := filepath.Join(dir, "clean.pdf"), filepath.Join(dir, "noisy.pdf"), filepath.Join(dir, "digit.pdf")
	writeLines(t, clean, "The office will find 17 regions.", "Total due 1,299.00 dollars.")
	// Trailing and doubled spaces, as a re-writer such as Ghostscript adds.
	writeLines(t, noisy, "The office will  find 17 regions. ", "Total due 1,299.00 dollars. ")
	writeLines(t, digit, "The office will find 18 regions.", "Total due 1,299.00 dollars.")

	a, b, c := pageInfo(t, clean), pageInfo(t, noisy), pageInfo(t, digit)
	if a.TextHash == 0 {
		t.Fatal("clean page has no text hash")
	}
	if a.TextHash != b.TextHash {
		t.Errorf("whitespace noise changed the hash: %q vs %q", a.Text, b.Text)
	}
	if a.TextHash == c.TextHash {
		t.Errorf("a changed digit kept the hash: %q vs %q", a.Text, c.Text)
	}
}
