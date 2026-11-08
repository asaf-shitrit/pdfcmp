package compare

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// writeLine writes a one-page PDF with line set in 10pt Helvetica.
func writeLine(t *testing.T, path, line string) {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("") // cp1252, for the accented letters
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(0, 10, tr(line))
	if err := pdf.OutputFileAndClose(path); err != nil {
		t.Fatal(err)
	}
}

// A comma turned period inside a number, or one accented letter turned
// another, keeps the page's letters and digits, and at 10pt barely moves the
// render; the pages are still different. Marks that a re-save moves or maps
// to other symbols are not.
func TestPunctuationAndAccentChanges(t *testing.T) {
	cmp, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, a, b string
		different  bool
	}{
		{"comma to period in a number", "Amount due: 1,500 USD", "Amount due: 1.500 USD", true},
		{"time separator", "Doors open at 12:30 sharp", "Doors open at 12.30 sharp", true},
		{"accent swapped", "Dear Mr Müller", "Dear Mr Möller", true},
		{"same text", "Amount due: 1,500 USD", "Amount due: 1,500 USD", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
			writeLine(t, a, tc.a)
			writeLine(t, b, tc.b)
			res, err := cmp.Compare(context.Background(), a, b)
			if err != nil {
				t.Fatal(err)
			}
			if got := !res.IsSimilar(0.95); got != tc.different {
				t.Errorf("different = %v, want %v (score %v, text_changed %v)",
					got, tc.different, res.SimilarityScore, res.TextChanged)
			}
		})
	}
}
