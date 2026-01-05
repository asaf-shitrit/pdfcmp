//go:build ignore

package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/jung-kurt/gofpdf"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	fmt.Println("Generating test PDF fixtures...")

	// 1. Small identical PDFs (5 pages)
	fmt.Println("Creating small_a.pdf and small_b.pdf (identical, 5 pages)...")
	createTextPDF(filepath.Join(dir, "small_a.pdf"), 5, "Hello World - Test Document", 42)
	createTextPDF(filepath.Join(dir, "small_b.pdf"), 5, "Hello World - Test Document", 42)

	// 2. Small different PDFs (5 pages, different content)
	fmt.Println("Creating small_diff_a.pdf and small_diff_b.pdf (different, 5 pages)...")
	createTextPDF(filepath.Join(dir, "small_diff_a.pdf"), 5, "Document Version A", 42)
	createTextPDF(filepath.Join(dir, "small_diff_b.pdf"), 5, "Document Version B", 42)

	// 3. Medium PDFs (20 pages)
	fmt.Println("Creating medium_a.pdf and medium_b.pdf (identical, 20 pages)...")
	createTextPDF(filepath.Join(dir, "medium_a.pdf"), 20, "Medium Test Document", 42)
	createTextPDF(filepath.Join(dir, "medium_b.pdf"), 20, "Medium Test Document", 42)

	// 4. Medium PDFs with one different page
	fmt.Println("Creating medium_modified.pdf (20 pages, page 10 modified)...")
	createModifiedPDF(filepath.Join(dir, "medium_modified.pdf"), 20, "Medium Test Document", 42, 10)

	// 5. Large PDFs (50 pages)
	fmt.Println("Creating large_a.pdf and large_b.pdf (identical, 50 pages)...")
	createTextPDF(filepath.Join(dir, "large_a.pdf"), 50, "Large Test Document", 42)
	createTextPDF(filepath.Join(dir, "large_b.pdf"), 50, "Large Test Document", 42)

	// 6. PDF with images/graphics
	fmt.Println("Creating graphic_a.pdf and graphic_b.pdf (with shapes, 10 pages)...")
	createGraphicPDF(filepath.Join(dir, "graphic_a.pdf"), 10, 42)
	createGraphicPDF(filepath.Join(dir, "graphic_b.pdf"), 10, 42)

	// 7. PDF with slightly different graphics
	fmt.Println("Creating graphic_modified.pdf (with modified shapes)...")
	createGraphicPDF(filepath.Join(dir, "graphic_modified.pdf"), 10, 43) // Different seed

	// 8. Single page PDFs for quick tests
	fmt.Println("Creating single_a.pdf and single_b.pdf (1 page each)...")
	createTextPDF(filepath.Join(dir, "single_a.pdf"), 1, "Single Page Document", 42)
	createTextPDF(filepath.Join(dir, "single_b.pdf"), 1, "Single Page Document", 42)

	// 9. Very large PDF for stress testing (100 pages)
	fmt.Println("Creating xlarge.pdf (100 pages)...")
	createTextPDF(filepath.Join(dir, "xlarge.pdf"), 100, "Extra Large Test Document", 42)

	// 10. Mixed Content PDFs (Text + Graphics)
	// This tests the hybrid Text-First + Visual fallback pipeline
	fmt.Println("Creating mixed_a.pdf and mixed_b.pdf (50 pages)...")
	createMixedPDF(filepath.Join(dir, "mixed_a.pdf"), 50, 42)
	createMixedPDF(filepath.Join(dir, "mixed_b.pdf"), 50, 42)

	// 11. Scanned-Simulated PDF (Images only, no text)
	// This forces the "Text-First" check to fail and fallback to full visual match
	fmt.Println("Creating scanned_a.pdf and scanned_b.pdf (Images only)...")
	createImageOnlyPDF(filepath.Join(dir, "scanned_a.pdf"), 20, 42)
	createImageOnlyPDF(filepath.Join(dir, "scanned_b.pdf"), 20, 42)

	fmt.Println("\nAll fixtures generated successfully!")
	fmt.Println("\nGenerated files:")
	files, _ := filepath.Glob(filepath.Join(dir, "*.pdf"))
	for _, f := range files {
		info, _ := os.Stat(f)
		fmt.Printf("  %s (%d KB)\n", filepath.Base(f), info.Size()/1024)
	}
}

func createTextPDF(filename string, pages int, title string, seed int64) {
	rand.Seed(seed)
	pdf := gofpdf.New("P", "mm", "A4", "")

	for i := 1; i <= pages; i++ {
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 16)
		pdf.Cell(40, 10, fmt.Sprintf("%s - Page %d", title, i))
		pdf.Ln(20)

		pdf.SetFont("Arial", "", 12)
		// Add some consistent "random" content
		for j := 0; j < 10; j++ {
			text := fmt.Sprintf("Line %d: Lorem ipsum dolor sit amet, consectetur adipiscing elit. Value: %d",
				j+1, rand.Intn(1000))
			pdf.Cell(0, 10, text)
			pdf.Ln(8)
		}

		// Add page number
		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.Cell(0, 10, fmt.Sprintf("Page %d of %d", i, pages))
	}

	pdf.OutputFileAndClose(filename)
}

func createModifiedPDF(filename string, pages int, title string, seed int64, modifiedPage int) {
	rand.Seed(seed)
	pdf := gofpdf.New("P", "mm", "A4", "")

	for i := 1; i <= pages; i++ {
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 16)

		if i == modifiedPage {
			// Modified page
			pdf.Cell(40, 10, fmt.Sprintf("MODIFIED PAGE %d - %s", i, title))
			pdf.Ln(20)
			pdf.SetFont("Arial", "", 12)
			pdf.Cell(0, 10, "THIS PAGE HAS BEEN MODIFIED FOR TESTING")
			pdf.Ln(8)
			pdf.Cell(0, 10, "The visual comparison should detect this difference.")
		} else {
			pdf.Cell(40, 10, fmt.Sprintf("%s - Page %d", title, i))
			pdf.Ln(20)
			pdf.SetFont("Arial", "", 12)
			for j := 0; j < 10; j++ {
				text := fmt.Sprintf("Line %d: Lorem ipsum dolor sit amet, consectetur adipiscing elit. Value: %d",
					j+1, rand.Intn(1000))
				pdf.Cell(0, 10, text)
				pdf.Ln(8)
			}
		}

		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.Cell(0, 10, fmt.Sprintf("Page %d of %d", i, pages))
	}

	pdf.OutputFileAndClose(filename)
}

func createGraphicPDF(filename string, pages int, seed int64) {
	rand.Seed(seed)
	pdf := gofpdf.New("P", "mm", "A4", "")

	for i := 1; i <= pages; i++ {
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 16)
		pdf.Cell(40, 10, fmt.Sprintf("Graphic Document - Page %d", i))

		// Draw some shapes
		for j := 0; j < 5; j++ {
			x := float64(20 + rand.Intn(150))
			y := float64(50 + rand.Intn(180))
			w := float64(20 + rand.Intn(40))
			h := float64(20 + rand.Intn(40))

			// Random color
			r := rand.Intn(256)
			g := rand.Intn(256)
			b := rand.Intn(256)
			pdf.SetFillColor(r, g, b)

			if rand.Intn(2) == 0 {
				pdf.Rect(x, y, w, h, "F")
			} else {
				pdf.Ellipse(x+w/2, y+h/2, w/2, h/2, 0, "F")
			}
		}

		// Add some lines
		pdf.SetDrawColor(0, 0, 0)
		for j := 0; j < 3; j++ {
			x1 := float64(rand.Intn(200))
			y1 := float64(50 + rand.Intn(200))
			x2 := float64(rand.Intn(200))
			y2 := float64(50 + rand.Intn(200))
			pdf.Line(x1, y1, x2, y2)
		}

		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.Cell(0, 10, fmt.Sprintf("Page %d of %d", i, pages))
	}

	pdf.OutputFileAndClose(filename)
}

func createMixedPDF(filename string, pages int, seed int64) {
rand.Seed(seed)
pdf := gofpdf.New("P", "mm", "A4", "")

for i := 1; i <= pages; i++ {
pdf.AddPage()

isText := rand.Intn(2) == 0

if isText {
pdf.SetFont("Arial", "B", 16)
pdf.Cell(40, 10, fmt.Sprintf("Mixed Doc (Text Page) - Page %d", i))
pdf.Ln(20)
pdf.SetFont("Arial", "", 12)
for j := 0; j < 15; j++ {
text := fmt.Sprintf("Line %d: This page contains searchable text content. Seed: %d", j+1, rand.Intn(1000))
pdf.Cell(0, 10, text)
pdf.Ln(8)
}
} else {
for j := 0; j < 10; j++ {
x := float64(10 + rand.Intn(180))
y := float64(10 + rand.Intn(250))
w := float64(20 + rand.Intn(50))
h := float64(20 + rand.Intn(50))
pdf.SetFillColor(rand.Intn(255), rand.Intn(255), rand.Intn(255))
pdf.Rect(x, y, w, h, "F")
}
}

pdf.SetY(-15)
if isText {
pdf.SetFont("Arial", "I", 8)
pdf.Cell(0, 10, fmt.Sprintf("Page %d of %d", i, pages))
}
}

pdf.OutputFileAndClose(filename)
}

func createImageOnlyPDF(filename string, pages int, seed int64) {
rand.Seed(seed)
pdf := gofpdf.New("P", "mm", "A4", "")

for i := 1; i <= pages; i++ {
pdf.AddPage()
for j := 0; j < 20; j++ {
x := float64(rand.Intn(210))
y := float64(rand.Intn(297))
r := float64(5 + rand.Intn(50))
pdf.SetFillColor(rand.Intn(255), rand.Intn(255), rand.Intn(255))
pdf.Circle(x, y, r, "F")
}
}
pdf.OutputFileAndClose(filename)
}
