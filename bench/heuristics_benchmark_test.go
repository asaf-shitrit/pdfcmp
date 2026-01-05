package bench

import (
	"testing"

	"github.com/asafshitrit/pdfcmp/pkg/visual"
)

// ============================================================================
// Smart Heuristics Benchmarks
// ============================================================================

// Benchmarks for ComputeHistogram
func BenchmarkHistogram_Small(b *testing.B) {
	img := createTestImage(256, 256)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.ComputeHistogram(img)
	}
}

func BenchmarkHistogram_Medium(b *testing.B) {
	img := createTestImage(1024, 1024)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.ComputeHistogram(img)
	}
}

func BenchmarkHistogram_Large(b *testing.B) {
	img := createTestImage(2550, 3300) // Letter at 300 DPI
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.ComputeHistogram(img)
	}
}

// Benchmarks for HistogramSimilarity
func BenchmarkHistogramSimilarity(b *testing.B) {
	h1 := visual.HistogramResult{AverageR: 100, AverageG: 150, AverageB: 200, Luminance: 140}
	h2 := visual.HistogramResult{AverageR: 110, AverageG: 160, AverageB: 210, Luminance: 150}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.HistogramSimilarity(h1, h2)
	}
}

// Comparison between Histogram and Hash calculation (to demonstrate speedup)
func BenchmarkComparison_HistogramVsHash(b *testing.B) {
	img := createTestImage(1024, 1024)
	
	b.Run("Histogram", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			visual.ComputeHistogram(img)
		}
	})

	b.Run("DHash", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			visual.DHash(img)
		}
	})

	b.Run("PHash", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			visual.PHash(img)
		}
	})
}
