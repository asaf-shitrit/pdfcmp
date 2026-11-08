package bench

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asaf-shitrit/pdfcmp/pkg/compare"
	"github.com/asaf-shitrit/pdfcmp/pkg/render"
	"github.com/asaf-shitrit/pdfcmp/pkg/visual"
)

const fixturesDir = "fixtures"

func fixtureExists(name string) bool {
	_, err := os.Stat(filepath.Join(fixturesDir, name))
	return err == nil
}

// requireFixture stops a benchmark that has no fixture. A benchmark that
// skipped quietly let `make bench` pass on a fresh clone while measuring
// nothing, so a missing fixture fails unless PDFCMP_BENCH_SKIP_MISSING=1.
func requireFixture(b *testing.B, names ...string) {
	b.Helper()
	for _, name := range names {
		if fixtureExists(name) {
			continue
		}
		if os.Getenv("PDFCMP_BENCH_SKIP_MISSING") == "1" {
			b.Skipf("fixture %s not generated", name)
		}
		b.Fatalf("fixture %s not generated: run `make fixtures` (or set PDFCMP_BENCH_SKIP_MISSING=1)", name)
	}
}

// benchCompare times cmp.Compare of two fixtures. With rendered set, it first
// checks that every page was scored from a render: pages decided from text
// score exactly 1.0, so a benchmark meant to render fails if any page does.
func benchCompare(b *testing.B, cmp compare.Comparer, name1, name2 string, rendered bool) {
	b.Helper()
	requireFixture(b, name1, name2)
	pdf1 := filepath.Join(fixturesDir, name1)
	pdf2 := filepath.Join(fixturesDir, name2)
	ctx := context.Background()

	if rendered {
		result, err := cmp.Compare(ctx, pdf1, pdf2)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.PageResults) == 0 {
			b.Fatalf("%s vs %s: no page results", name1, name2)
		}
		for _, pr := range result.PageResults {
			if pr.Similarity >= 1.0 {
				b.Fatalf("%s vs %s: page %d was not rendered (similarity %v); use a pair whose pages differ",
					name1, name2, pr.Page, pr.Similarity)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cmp.Compare(ctx, pdf1, pdf2); err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// PDF Rendering Benchmarks
// ============================================================================

func BenchmarkRender_72DPI(b *testing.B) {
	requireFixture(b, "single_a.pdf")

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
	requireFixture(b, "single_a.pdf")

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
	requireFixture(b, "single_a.pdf")

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
//
// *_ByteIdentical compares a file with itself and stops at the byte hash.
// *_PlanOnly compares two files with the same text and different bytes: every
// page is decided from text and content hash, and nothing is rendered.
// The unsuffixed and _Visual benchmarks compare files whose pages all differ
// with SamplingAll, so every page is rendered; benchCompare checks that.
// ============================================================================

// Same file twice: the byte hash answers.
func BenchmarkFullCompare_Small_ByteIdentical(b *testing.B) {
	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "small_a.pdf", "small_a.pdf", false)
}

// ModeByte on two files with the same text.
func BenchmarkFullCompare_Small_ByteOnly(b *testing.B) {
	cmp, err := compare.New(compare.WithMode(compare.ModeByte))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "small_a.pdf", "small_b.pdf", false)
}

// Same text, different bytes: page plan only, no render.
func BenchmarkFullCompare_Small_PlanOnly(b *testing.B) {
	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "small_a.pdf", "small_b.pdf", false)
}

// Same text, different bytes: page plan only, no render.
func BenchmarkFullCompare_Medium_PlanOnly(b *testing.B) {
	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "medium_a.pdf", "medium_b.pdf", false)
}

// Same text, different bytes: page plan only, no render.
func BenchmarkFullCompare_Large_PlanOnly(b *testing.B) {
	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "large_a.pdf", "large_b.pdf", false)
}

// 5 pages, every page rendered.
func BenchmarkFullCompare_Small_Visual(b *testing.B) {
	cmp, err := compare.New(compare.WithSampling(compare.SamplingAll))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "small_a.pdf", "small_changed.pdf", true)
}

// 20 pages, every page rendered.
func BenchmarkFullCompare_Medium(b *testing.B) {
	cmp, err := compare.New(compare.WithSampling(compare.SamplingAll))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "medium_a.pdf", "medium_changed.pdf", true)
}

// 50 pages, every page rendered.
func BenchmarkFullCompare_Large(b *testing.B) {
	cmp, err := compare.New(compare.WithSampling(compare.SamplingAll))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "large_a.pdf", "large_changed.pdf", true)
}

// 100 pages, every page rendered.
func BenchmarkFullCompare_XLarge(b *testing.B) {
	cmp, err := compare.New(compare.WithSampling(compare.SamplingAll))
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "xlarge.pdf", "xlarge_changed.pdf", true)
}

// Same file twice: the byte hash answers.
func BenchmarkFullCompare_XLarge_ByteIdentical(b *testing.B) {
	cmp, err := compare.New()
	if err != nil {
		b.Fatal(err)
	}
	defer cmp.Close()

	benchCompare(b, cmp, "xlarge.pdf", "xlarge.pdf", false)
}

// ============================================================================
// Sampling Strategy Benchmarks
// ============================================================================

func BenchmarkSampling_All_Medium(b *testing.B) {
	requireFixture(b, "medium_a.pdf", "medium_changed.pdf")

	pdf1 := filepath.Join(fixturesDir, "medium_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "medium_changed.pdf")

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
	requireFixture(b, "medium_a.pdf", "medium_changed.pdf")

	pdf1 := filepath.Join(fixturesDir, "medium_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "medium_changed.pdf")

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
	requireFixture(b, "medium_a.pdf", "medium_changed.pdf")

	pdf1 := filepath.Join(fixturesDir, "medium_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "medium_changed.pdf")

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
	requireFixture(b, "small_a.pdf", "small_changed.pdf")

	pdf1 := filepath.Join(fixturesDir, "small_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "small_changed.pdf")

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
	requireFixture(b, "small_a.pdf", "small_changed.pdf")

	pdf1 := filepath.Join(fixturesDir, "small_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "small_changed.pdf")

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
	requireFixture(b, "small_a.pdf", "small_changed.pdf")

	pdf1 := filepath.Join(fixturesDir, "small_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "small_changed.pdf")

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
	requireFixture(b, "single_a.pdf")

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
	requireFixture(b, "large_a.pdf", "large_changed.pdf")

	pdf1 := filepath.Join(fixturesDir, "large_a.pdf")
	pdf2 := filepath.Join(fixturesDir, "large_changed.pdf")

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
	for _, pr := range result.PageResults {
		if pr.Similarity >= 1.0 {
			b.Fatalf("page %d was not rendered; use a pair whose pages differ", pr.Page)
		}
	}

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
