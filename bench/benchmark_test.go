package bench

import (
	"bytes"
	"crypto/rand"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/asafshitrit/similar-pdf/pkg/bytewise"
	"github.com/asafshitrit/similar-pdf/pkg/visual"
)

// ============================================================================
// Byte-wise Comparison Benchmarks
// ============================================================================

func BenchmarkXXHash_1MB(b *testing.B) {
	data := make([]byte, 1*1024*1024)
	rand.Read(data)
	tmpFile := createTempFile(b, data)
	defer os.Remove(tmpFile)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		_, err := bytewise.HashFile(tmpFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkXXHash_10MB(b *testing.B) {
	data := make([]byte, 10*1024*1024)
	rand.Read(data)
	tmpFile := createTempFile(b, data)
	defer os.Remove(tmpFile)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		_, err := bytewise.HashFile(tmpFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkXXHash_50MB(b *testing.B) {
	data := make([]byte, 50*1024*1024)
	rand.Read(data)
	tmpFile := createTempFile(b, data)
	defer os.Remove(tmpFile)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		_, err := bytewise.HashFile(tmpFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkByteCompare_Identical(b *testing.B) {
	data := make([]byte, 5*1024*1024)
	rand.Read(data)
	tmpFile1 := createTempFile(b, data)
	tmpFile2 := createTempFile(b, data)
	defer os.Remove(tmpFile1)
	defer os.Remove(tmpFile2)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := bytewise.Compare(tmpFile1, tmpFile2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkByteCompare_Different(b *testing.B) {
	data1 := make([]byte, 5*1024*1024)
	data2 := make([]byte, 5*1024*1024)
	rand.Read(data1)
	rand.Read(data2)
	tmpFile1 := createTempFile(b, data1)
	tmpFile2 := createTempFile(b, data2)
	defer os.Remove(tmpFile1)
	defer os.Remove(tmpFile2)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := bytewise.Compare(tmpFile1, tmpFile2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkByteCompare_EarlyExit(b *testing.B) {
	data1 := make([]byte, 5*1024*1024)
	data2 := make([]byte, 5*1024*1024)
	rand.Read(data1)
	copy(data2, data1)
	data2[0] = data2[0] ^ 0xFF // Flip first byte

	tmpFile1 := createTempFile(b, data1)
	tmpFile2 := createTempFile(b, data2)
	defer os.Remove(tmpFile1)
	defer os.Remove(tmpFile2)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := bytewise.CompareWithEarlyExit(tmpFile1, tmpFile2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Perceptual Hash Benchmarks
// ============================================================================

func BenchmarkDHash_Small(b *testing.B) {
	img := createTestImage(256, 256)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.DHash(img)
	}
}

func BenchmarkDHash_Medium(b *testing.B) {
	img := createTestImage(1024, 1024)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.DHash(img)
	}
}

func BenchmarkDHash_Large(b *testing.B) {
	img := createTestImage(2550, 3300) // Letter at 300 DPI
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.DHash(img)
	}
}

func BenchmarkPHash_Small(b *testing.B) {
	img := createTestImage(256, 256)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.PHash(img)
	}
}

func BenchmarkPHash_Medium(b *testing.B) {
	img := createTestImage(1024, 1024)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.PHash(img)
	}
}

func BenchmarkPHash_Large(b *testing.B) {
	img := createTestImage(2550, 3300) // Letter at 300 DPI
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.PHash(img)
	}
}

func BenchmarkPHashFast_Large(b *testing.B) {
	img := createTestImage(2550, 3300)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.PHashFast(img)
	}
}

func BenchmarkBothHashes_Medium(b *testing.B) {
	img := createTestImage(1024, 1024)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.ComputeBothHashes(img)
	}
}

// ============================================================================
// Hamming Distance Benchmarks
// ============================================================================

func BenchmarkHammingDistance(b *testing.B) {
	h1 := uint64(0xDEADBEEFCAFEBABE)
	h2 := uint64(0xFEEDFACEBAADF00D)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.HammingDistance(h1, h2)
	}
}

func BenchmarkSimilarityFromHashes(b *testing.B) {
	h1 := uint64(0xDEADBEEFCAFEBABE)
	h2 := uint64(0xFEEDFACEBAADF00D)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.SimilarityFromHashes(h1, h2)
	}
}

func BenchmarkCombinedSimilarity(b *testing.B) {
	dHash1 := uint64(0xDEADBEEFCAFEBABE)
	dHash2 := uint64(0xDEADBEEFCAFEBABF)
	pHash1 := uint64(0xFEEDFACEBAADF00D)
	pHash2 := uint64(0xFEEDFACEBAADF00E)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visual.CombinedSimilarity(dHash1, dHash2, pHash1, pHash2)
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func createTempFile(b *testing.B, data []byte) string {
	b.Helper()
	tmpFile, err := os.CreateTemp("", "benchmark-*.bin")
	if err != nil {
		b.Fatal(err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, bytes.NewReader(data)); err != nil {
		b.Fatal(err)
	}

	return tmpFile.Name()
}

func createTestImage(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Create a pattern for testing
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Create a gradient pattern
			r := uint8((x * 255) / width)
			g := uint8((y * 255) / height)
			b := uint8(((x + y) * 127) / (width + height))
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}

	return img
}

// ============================================================================
// Fixtures Generator (for integration tests)
// ============================================================================

// GenerateFixtures creates test PDF files in the fixtures directory.
// This is called manually, not as part of benchmark runs.
func GenerateFixtures(fixturesDir string) error {
	if err := os.MkdirAll(fixturesDir, 0755); err != nil {
		return err
	}

	// Create a marker file to indicate fixtures were generated
	marker := filepath.Join(fixturesDir, ".generated")
	return os.WriteFile(marker, []byte("Generated by pdfcompare benchmark suite\n"), 0644)
}
