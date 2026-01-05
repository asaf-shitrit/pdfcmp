package bench

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/asafshitrit/pdfcmp/pkg/render"
)

func BenchmarkPageInfo_Cost(b *testing.B) {
	if !fixtureExists("single_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	renderer, err := render.NewPdfiumRenderer()
	if err != nil {
		b.Fatal(err)
	}
	defer renderer.Close()

	doc, err := renderer.Open(filepath.Join(fixturesDir, "single_a.pdf"))
	if err != nil {
		b.Fatal(err)
	}
	defer doc.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := doc.PageInfo(0)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRender_LowRes_Cost(b *testing.B) {
	if !fixtureExists("single_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	renderer, err := render.NewPdfiumRenderer()
	if err != nil {
		b.Fatal(err)
	}
	defer renderer.Close()

	doc, err := renderer.Open(filepath.Join(fixturesDir, "single_a.pdf"))
	if err != nil {
		b.Fatal(err)
	}
	defer doc.Close()

	ctx := context.Background()
	opts := render.Options{DPI: 72} // Low res

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := doc.RenderPage(ctx, 0, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}
