package compare

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// Text recoloured from red to blue keeps its darkest and brightest channel,
// so a render check that saw only those found nothing. The pair must still
// be DIFFERENT.
func TestRecolouredTextIsDifferent(t *testing.T) {
	write := func(path string, r, g, b int) {
		pdf := gofpdf.New("P", "mm", "A4", "")
		pdf.AddPage()
		pdf.SetFont("Arial", "", 11)
		for j := 0; j < 30; j++ {
			pdf.Cell(0, 6, fmt.Sprintf("Line %d of the agreement text that stays the same in both files.", j))
			pdf.Ln(7)
		}
		pdf.SetTextColor(r, g, b)
		pdf.SetXY(20, 240)
		pdf.SetFont("Arial", "B", 14)
		pdf.Cell(0, 8, "OVERDUE - PAYMENT REQUIRED BY 30 NOV")
		if err := pdf.OutputFileAndClose(path); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	red, blue := filepath.Join(dir, "red.pdf"), filepath.Join(dir, "blue.pdf")
	write(red, 255, 0, 0)
	write(blue, 0, 0, 255)
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	res, err := c.Compare(context.Background(), red, blue)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict().Status != "DIFFERENT" {
		t.Errorf("red to blue text: verdict %v, want DIFFERENT (tile changed = %v)", res.Verdict().Status, res.PageResults[0].TileChanged)
	}
}
