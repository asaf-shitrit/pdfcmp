// Package fixtures writes the PDF documents that pdfcmp's tests and
// benchmarks compare. Every document comes from a fixed seed, so two calls
// with the same arguments write the same content; only the creation date in
// the file differs.
package fixtures

import (
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"path/filepath"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// textLines is the number of seeded lines on a text page.
const textLines = 10

// newPDF returns an A4 document for path that never breaks pages on its own,
// so each AddPage is exactly one page, footer included. Its creation and
// modification dates come from the file name, so two files with the same
// content but different names always differ in their bytes, and comparing
// them cannot stop at the byte check. (gofpdf writes font resources in map
// order, so a file is not byte-for-byte reproducible either way.)
func newPDF(path string) *gofpdf.Fpdf {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 0)
	h := fnv.New32a()
	h.Write([]byte(filepath.Base(path)))
	offset := time.Duration(h.Sum32()%(365*24*60*60)) * time.Second
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(offset)
	pdf.SetCreationDate(date)
	pdf.SetModificationDate(date) // gofpdf otherwise writes the current time
	return pdf
}

// Text writes pages pages, each with a heading and ten lines of text that
// end in a seeded value, and a footer.
func Text(path string, pages int, title string, seed int64) error {
	return Modified(path, pages, title, seed, 0)
}

// Modified writes Text's document with page modified (1-indexed) replaced by
// a notice. Every other page matches Text's document page for page; 0 means
// no page is modified.
func Modified(path string, pages int, title string, seed int64, modified int) error {
	rng := rand.New(rand.NewSource(seed))
	pdf := newPDF(path)
	for i := 1; i <= pages; i++ {
		pdf.AddPage()
		if i == modified {
			noticePage(pdf, rng, title, i)
		} else {
			textPage(pdf, rng, title, i)
		}
		footer(pdf, i, pages)
	}
	return pdf.OutputFileAndClose(path)
}

func textPage(pdf *gofpdf.Fpdf, rng *rand.Rand, title string, page int) {
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, fmt.Sprintf("%s - Page %d", title, page))
	pdf.Ln(20)
	pdf.SetFont("Arial", "", 12)
	for j := 0; j < textLines; j++ {
		pdf.Cell(0, 10, fmt.Sprintf("Line %d: Lorem ipsum dolor sit amet, consectetur adipiscing elit. Value: %d",
			j+1, rng.Intn(1000)))
		pdf.Ln(8)
	}
}

// noticePage replaces a text page. It still draws the values the text page
// would have used, so every later page matches the unmodified document.
func noticePage(pdf *gofpdf.Fpdf, rng *rand.Rand, title string, page int) {
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, fmt.Sprintf("MODIFIED PAGE %d - %s", page, title))
	pdf.Ln(20)
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 10, "THIS PAGE HAS BEEN MODIFIED FOR TESTING")
	pdf.Ln(8)
	pdf.Cell(0, 10, "The visual comparison should detect this difference.")
	for j := 0; j < textLines; j++ {
		rng.Intn(1000)
	}
}

func footer(pdf *gofpdf.Fpdf, page, pages int) {
	pdf.SetY(-15)
	pdf.SetFont("Arial", "I", 8)
	pdf.Cell(0, 10, fmt.Sprintf("Page %d of %d", page, pages))
}

// Graphic writes pages of seeded rectangles, ellipses and lines under a
// heading, with a footer.
func Graphic(path string, pages int, seed int64) error {
	rng := rand.New(rand.NewSource(seed))
	pdf := newPDF(path)
	for i := 1; i <= pages; i++ {
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 16)
		pdf.Cell(40, 10, fmt.Sprintf("Graphic Document - Page %d", i))
		shapes(pdf, rng)
		lines(pdf, rng)
		footer(pdf, i, pages)
	}
	return pdf.OutputFileAndClose(path)
}

func shapes(pdf *gofpdf.Fpdf, rng *rand.Rand) {
	for j := 0; j < 5; j++ {
		x := float64(20 + rng.Intn(150))
		y := float64(50 + rng.Intn(180))
		w := float64(20 + rng.Intn(40))
		h := float64(20 + rng.Intn(40))
		pdf.SetFillColor(rng.Intn(256), rng.Intn(256), rng.Intn(256))
		if rng.Intn(2) == 0 {
			pdf.Rect(x, y, w, h, "F")
		} else {
			pdf.Ellipse(x+w/2, y+h/2, w/2, h/2, 0, "F")
		}
	}
}

func lines(pdf *gofpdf.Fpdf, rng *rand.Rand) {
	pdf.SetDrawColor(0, 0, 0)
	for j := 0; j < 3; j++ {
		x1 := float64(rng.Intn(200))
		y1 := float64(50 + rng.Intn(200))
		x2 := float64(rng.Intn(200))
		y2 := float64(50 + rng.Intn(200))
		pdf.Line(x1, y1, x2, y2)
	}
}

// Mixed writes pages that are, by seed, either fifteen lines of text with a
// footer or ten coloured rectangles with no text at all.
func Mixed(path string, pages int, seed int64) error {
	rng := rand.New(rand.NewSource(seed))
	pdf := newPDF(path)
	for i := 1; i <= pages; i++ {
		pdf.AddPage()
		if rng.Intn(2) == 0 {
			mixedTextPage(pdf, rng, i)
			footer(pdf, i, pages)
		} else {
			rectangles(pdf, rng)
		}
	}
	return pdf.OutputFileAndClose(path)
}

func mixedTextPage(pdf *gofpdf.Fpdf, rng *rand.Rand, page int) {
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, fmt.Sprintf("Mixed Doc (Text Page) - Page %d", page))
	pdf.Ln(20)
	pdf.SetFont("Arial", "", 12)
	for j := 0; j < 15; j++ {
		pdf.Cell(0, 10, fmt.Sprintf("Line %d: This page contains searchable text content. Seed: %d", j+1, rng.Intn(1000)))
		pdf.Ln(8)
	}
}

func rectangles(pdf *gofpdf.Fpdf, rng *rand.Rand) {
	for j := 0; j < 10; j++ {
		x := float64(10 + rng.Intn(180))
		y := float64(10 + rng.Intn(250))
		w := float64(20 + rng.Intn(50))
		h := float64(20 + rng.Intn(50))
		pdf.SetFillColor(rng.Intn(255), rng.Intn(255), rng.Intn(255))
		pdf.Rect(x, y, w, h, "F")
	}
}

// ImageOnly writes pages of seeded circles and no text, like a scan.
func ImageOnly(path string, pages int, seed int64) error {
	rng := rand.New(rand.NewSource(seed))
	pdf := newPDF(path)
	for i := 1; i <= pages; i++ {
		pdf.AddPage()
		for j := 0; j < 20; j++ {
			x := float64(rng.Intn(210))
			y := float64(rng.Intn(297))
			r := float64(5 + rng.Intn(50))
			pdf.SetFillColor(rng.Intn(255), rng.Intn(255), rng.Intn(255))
			pdf.Circle(x, y, r, "F")
		}
	}
	return pdf.OutputFileAndClose(path)
}

// Spec is one fixture file: its name and what it holds.
type Spec struct {
	Name        string
	Description string
	Pages       int
	write       func(path string) error
}

// Set is the benchmark fixture set. Files named *_a and *_b hold the same
// content, so comparing them decides every page from text without rendering.
// Files named *_changed hold the *_a text with other seeded values on every
// page, so comparing them renders every page.
var Set = []Spec{
	text("small_a.pdf", "identical", 5, "Hello World - Test Document"),
	text("small_b.pdf", "identical", 5, "Hello World - Test Document"),
	{"small_changed.pdf", "every page differs from small_a", 5, func(p string) error { return Text(p, 5, "Hello World - Test Document", 43) }},
	text("small_diff_a.pdf", "different title", 5, "Document Version A"),
	text("small_diff_b.pdf", "different title", 5, "Document Version B"),
	text("medium_a.pdf", "identical", 20, "Medium Test Document"),
	text("medium_b.pdf", "identical", 20, "Medium Test Document"),
	{"medium_modified.pdf", "page 10 modified", 20, func(p string) error { return Modified(p, 20, "Medium Test Document", 42, 10) }},
	{"medium_changed.pdf", "every page differs from medium_a", 20, func(p string) error { return Text(p, 20, "Medium Test Document", 43) }},
	text("large_a.pdf", "identical", 50, "Large Test Document"),
	text("large_b.pdf", "identical", 50, "Large Test Document"),
	{"large_changed.pdf", "every page differs from large_a", 50, func(p string) error { return Text(p, 50, "Large Test Document", 43) }},
	{"graphic_a.pdf", "shapes", 10, func(p string) error { return Graphic(p, 10, 42) }},
	{"graphic_b.pdf", "shapes", 10, func(p string) error { return Graphic(p, 10, 42) }},
	{"graphic_modified.pdf", "different shapes", 10, func(p string) error { return Graphic(p, 10, 43) }},
	text("single_a.pdf", "identical", 1, "Single Page Document"),
	text("single_b.pdf", "identical", 1, "Single Page Document"),
	text("xlarge.pdf", "stress test", 100, "Extra Large Test Document"),
	{"xlarge_changed.pdf", "every page differs from xlarge", 100, func(p string) error { return Text(p, 100, "Extra Large Test Document", 43) }},
	{"mixed_a.pdf", "text and graphics pages", 50, func(p string) error { return Mixed(p, 50, 42) }},
	{"mixed_b.pdf", "text and graphics pages", 50, func(p string) error { return Mixed(p, 50, 42) }},
	{"scanned_a.pdf", "images only", 20, func(p string) error { return ImageOnly(p, 20, 42) }},
	{"scanned_b.pdf", "images only", 20, func(p string) error { return ImageOnly(p, 20, 42) }},
}

func text(name, description string, pages int, title string) Spec {
	return Spec{name, description, pages, func(p string) error { return Text(p, pages, title, 42) }}
}

// Write writes the fixture into dir and returns its path.
func (s Spec) Write(dir string) (string, error) {
	path := filepath.Join(dir, s.Name)
	if err := s.write(path); err != nil {
		return "", fmt.Errorf("%s: %w", s.Name, err)
	}
	return path, nil
}

// Find returns the fixture named name from Set.
func Find(name string) (Spec, bool) {
	for _, s := range Set {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

// Generate writes every fixture in Set into dir, logging each to w.
func Generate(dir string, w io.Writer) error {
	for _, s := range Set {
		if _, err := s.Write(dir); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(w, "%-22s %3d pages  %s\n", s.Name, s.Pages, s.Description)
	}
	return nil
}

// Sequence writes one text page per id: ten lines of text seeded by the id,
// under a heading that names it. Pages with the same id are identical in
// every document, wherever they sit, because a page carries no number or
// footer. Inserting, dropping or swapping ids therefore changes only those
// pages, which makes Sequence the fixture for documents whose page counts
// differ.
func Sequence(path string, ids []int) error { return sequence(path, ids, false) }

// Redrawn writes Sequence's document with each page drawn as another tool
// might export it: the same text, plus a white rectangle that changes the
// page's objects and content stream but not what it shows.
func Redrawn(path string, ids []int) error { return sequence(path, ids, true) }

func sequence(path string, ids []int, redrawn bool) error {
	pdf := newPDF(path)
	for _, id := range ids {
		pdf.AddPage()
		if redrawn {
			pdf.SetFillColor(255, 255, 255)
			pdf.Rect(1, 1, 2, 2, "F")
		}
		rng := rand.New(rand.NewSource(int64(id)))
		pdf.SetFont("Arial", "B", 16)
		pdf.Cell(40, 10, fmt.Sprintf("Section %d", id))
		pdf.Ln(20)
		pdf.SetFont("Arial", "", 12)
		for j := 0; j < textLines; j++ {
			pdf.Cell(0, 10, fmt.Sprintf("Clause %d.%d: the parties agree to term %d and to schedule %d.",
				id, j+1, rng.Intn(1000), rng.Intn(1000)))
			pdf.Ln(8)
		}
	}
	return pdf.OutputFileAndClose(path)
}
