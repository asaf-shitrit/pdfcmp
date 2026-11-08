package compare

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// gradientDoc writes one page per id: a gradient header (a shading, which
// cannot be content-hashed) over a few lines of text unique to the id.
func gradientDoc(t *testing.T, path string, ids []int) {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	for _, id := range ids {
		pdf.AddPage()
		pdf.LinearGradient(0, 0, 210, 15, 200, 220, 255, 255, 255, 255, 0, 0, 1, 0)
		rng := rand.New(rand.NewSource(int64(id)))
		pdf.SetFont("Arial", "B", 16)
		pdf.Cell(40, 10, fmt.Sprintf("Section %d", id))
		pdf.Ln(20)
		pdf.SetFont("Arial", "", 12)
		for j := 0; j < 20; j++ {
			pdf.Cell(0, 10, fmt.Sprintf("Clause %d.%d: the parties agree to term %d and to schedule %d.", id, j+1, rng.Intn(1000), rng.Intn(1000)))
			pdf.Ln(8)
		}
	}
	if err := pdf.OutputFileAndClose(path); err != nil {
		t.Fatal(err)
	}
}

// Pages with text anchor by their text even when their content cannot be
// hashed, so a deleted page in a long document is found, not guessed by
// position.
func TestAlignUnhashablePagesWithText(t *testing.T) {
	dir := t.TempDir()
	var a, b []int
	for i := 1; i <= 120; i++ {
		a = append(a, i)
		if i != 3 {
			b = append(b, i)
		}
	}
	p1, p2 := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
	gradientDoc(t, p1, a)
	gradientDoc(t, p2, b)
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	res, err := c.Compare(context.Background(), p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{3}; !slices.Equal(res.DeletedPages, want) {
		t.Errorf("DeletedPages = %v, want %v", res.DeletedPages, want)
	}
	if len(res.InsertedPages) != 0 {
		t.Errorf("InsertedPages = %v, want none", res.InsertedPages)
	}
	for _, p := range res.PageResults {
		if p.Similarity < 0.99 {
			t.Errorf("page %d similarity %v, want >= 0.99", p.Page, p.Similarity)
		}
	}
}
