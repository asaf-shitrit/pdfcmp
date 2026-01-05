package bench

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asafshitrit/pdfcmp/pkg/compare"
	"github.com/asafshitrit/pdfcmp/pkg/render"
	"github.com/asafshitrit/pdfcmp/pkg/visual"
)

const fixturesDir = "fixtures"

func fixtureExists(name string) bool {
	_, err := os.Stat(filepath.Join(fixturesDir, name))
	return err == nil
}

// ============================================================================
// PDF Rendering Benchmarks
// ============================================================================

func BenchmarkRender_72DPI(b *testing.B) {
	if !fixtureExists("single_a.pdf") {
		b.Skip("Fixtures not generated. Run: go run bench/fixtures/generate.go bench/fixtures/")
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
	opts := render.Options{DPI: 72}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := doc.RenderPage(ctx, 0, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRender_150DPI(b *testing.B) {
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
	opts := render.Options{DPI: 150}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := doc.RenderPage(ctx, 0, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRender_300DPI(b *testing.B) {
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
	opts := render.Options{DPI: 300}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := doc.RenderPage(ctx, 0, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Full Pipeline Benchmarks
// ============================================================================

func BenchmarkFullCompare_Small_Identical(b *testing.B) {
	if !fixtureExists("small_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "small_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "small_a.pdf") // Compare to self

	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFullCompare_Small_ByteOnly(b *testing.B) {
	if !fixtureExists("small_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "small_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "small_b.pdf")

	cmp, err := compare.New(compare.WithMode(compare.ModeByte))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFullCompare_Small_Visual(b *testing.B) {
	if !fixtureExists("small_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "small_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "small_b.pdf")

	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFullCompare_Medium(b *testing.B) {
	if !fixtureExists("medium_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "medium_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "medium_b.pdf")

	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFullCompare_Large(b *testing.B) {
	if !fixtureExists("large_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "large_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "large_b.pdf")

	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFullCompare_XLarge(b *testing.B) {
	if !fixtureExists("xlarge.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "xlarge.pdf")
	pdf2 := filepath.Join(fixturesDir, "xlarge.pdf") // Compare to self

	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Sampling Strategy Benchmarks
// ============================================================================

func BenchmarkSampling_All_Medium(b *testing.B) {
	if !fixtureExists("medium_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "medium_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "medium_modified.pdf")

	cmp, err := compare.New(compare.WithSampling(compare.SamplingAll))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSampling_Strategic_Medium(b *testing.B) {
	if !fixtureExists("medium_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "medium_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "medium_modified.pdf")

	cmp, err := compare.New(compare.WithSampling(compare.SamplingStrategic))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSampling_Thumbnail_Medium(b *testing.B) {
	if !fixtureExists("medium_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "medium_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "medium_modified.pdf")

	cmp, err := compare.New(compare.WithSampling(compare.SamplingThumbnailOnly))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Hash Type Benchmarks
// ============================================================================

func BenchmarkHashType_DHashOnly(b *testing.B) {
	if !fixtureExists("small_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "small_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "small_b.pdf")

	cmp, err := compare.New(compare.WithHashType(compare.HashDHash))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashType_PHashOnly(b *testing.B) {
	if !fixtureExists("small_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "small_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "small_b.pdf")

	cmp, err := compare.New(compare.WithHashType(compare.HashPHash))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashType_Both(b *testing.B) {
	if !fixtureExists("small_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "small_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "small_b.pdf")

	cmp, err := compare.New(compare.WithHashType(compare.HashBoth))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// End-to-End Page Processing Benchmark
// ============================================================================

func BenchmarkE2E_RenderAndHash_SinglePage(b *testing.B) {
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
	opts := render.Options{DPI: 150}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		img, err := doc.RenderPage(ctx, 0, opts)
		if err != nil {
			b.Fatal(err)
		}
		visual.ComputeBothHashes(img)
	}
}

// ============================================================================
// Throughput Measurement
// ============================================================================

func BenchmarkThroughput_PagesPerSecond(b *testing.B) {
	if !fixtureExists("large_a.pdf") {
		b.Skip("Fixtures not generated")
	}

	pdf1 := filepath.Join(fixturesDir, "large_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "large_b.pdf")

	cmp, err := compare.New(compare.WithSampling(compare.SamplingAll))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	ctx := context.Background()

	// Warm up
	result, err := cmp.Compare(ctx, pdf1, pdf2)
	if err != nil {
		b.Fatal(err)
	}

	pagesCompared := result.VisualComparison.PagesCompared

	b.ResetTimer()
	start := time.Now()
	iterations := 0

	for i := 0; i < b.N; i++ {
		_, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
		iterations++
	}

	elapsed := time.Since(start)
	totalPages := iterations * pagesCompared
	pagesPerSec := float64(totalPages) / elapsed.Seconds()

	b.ReportMetric(pagesPerSec, "pages/sec")
}
