package visual

import (
	"image"
	"image/color"
	"testing"
)

// Helper to create a solid color image
func solidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestHammingDistance(t *testing.T) {
	tests := []struct {
		a, b     uint64
		expected int
	}{
		{0, 0, 0},
		{0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF, 0},
		{0, 1, 1},
		{1, 0, 1},
		{0, 3, 2},       // 3 is 11 in binary
		{0x0F, 0xF0, 8}, // 00001111 vs 11110000
		{0, 0xFFFFFFFFFFFFFFFF, 64},
	}

	for _, tt := range tests {
		got := HammingDistance(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("HammingDistance(%x, %x) = %d; want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestSimilarity(t *testing.T) {
	tests := []struct {
		dist     int
		expected float64
	}{
		{0, 1.0},
		{64, 0.0},
		{10, (64.0 - 10.0) / 64.0},
	}

	for _, tt := range tests {
		got := Similarity(tt.dist)
		// Float comparison with tolerance
		if got < tt.expected-0.001 || got > tt.expected+0.001 {
			t.Errorf("Similarity(%d) = %f; want %f", tt.dist, got, tt.expected)
		}
	}
}

func TestCombinedSimilarity(t *testing.T) {
	// Simple test: if distance is 0 for both, similarity should be 1.0
	sim := CombinedSimilarity(0, 0, 0, 0)
	if sim != 1.0 {
		t.Errorf("Expected 1.0 for identical hashes, got %f", sim)
	}
}

func TestDHash_SolidColor(t *testing.T) {
	// A solid color image has 0 difference between adjacent pixels in theory,
	// but the implementation checks left < right.
	// If left == right, bit is 0.
	// So a solid image should result in hash 0.
	img := solidImage(100, 100, color.White)
	hash := DHash(img)
	if hash != 0 {
		t.Errorf("Expected 0 hash for solid color, got %x", hash)
	}
}

func TestDHash_Gradient(t *testing.T) {
	// Create horizontal gradient (left darker than right)
	// left < right for all pixels -> all 1s
	img := image.NewGray(image.Rect(0, 0, 100, 100))
	for x := 0; x < 100; x++ {
		for y := 0; y < 100; y++ {
			img.SetGray(x, y, color.Gray{Y: uint8(x * 2)}) // increasing brightness
		}
	}

	hash := DHash(img)
	// dHash checks 8 rows of 8 comparisons.
	// We expect all 1s because left < right consistently.
	if hash != 0xFFFFFFFFFFFFFFFF {
		t.Errorf("Expected all 1s for gradient, got %x", hash)
	}
}

func TestPHash_Solid(t *testing.T) {
	// pHash of solid color matches another solid color
	img1 := solidImage(32, 32, color.White)
	img2 := solidImage(64, 64, color.White)

	h1 := PHash(img1)
	h2 := PHash(img2)

	if h1 != h2 {
		t.Errorf("Expected identical pHash for solid colors, got %x and %x", h1, h2)
	}
}

func TestPHashFast(t *testing.T) {
	// Left half dark, right half light: the four right columns of each 8x8
	// row are above the mean.
	img := image.NewGray(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if x >= 32 {
				img.SetGray(x, y, color.Gray{Y: 220})
			} else {
				img.SetGray(x, y, color.Gray{Y: 30})
			}
		}
	}
	const rightHalf uint64 = 0xf0f0f0f0f0f0f0f0
	if got := PHashFast(img); got != rightHalf {
		t.Errorf("PHashFast = %#016x, want %#016x", got, rightHalf)
	}

	solid := image.NewGray(image.Rect(0, 0, 16, 16))
	if got := PHashFast(solid); got != 0 {
		t.Errorf("solid image: PHashFast = %#016x, want 0 (no pixel above the mean)", got)
	}
}

func TestPHash_InverseIsFullyOpposite(t *testing.T) {
	// Pseudo-random pattern; the inverse negates every AC coefficient, so
	// all 64 bits must flip and similarity must be 0.
	img := image.NewGray(image.Rect(0, 0, 32, 32))
	inv := image.NewGray(image.Rect(0, 0, 32, 32))
	seed := uint32(1)
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			seed = seed*1664525 + 1013904223
			v := uint8(seed >> 24)
			img.SetGray(x, y, color.Gray{Y: v})
			inv.SetGray(x, y, color.Gray{Y: 255 - v})
		}
	}
	h1, h2 := PHash(img), PHash(inv)
	if d := HammingDistance(h1, h2); d != HashBits {
		t.Errorf("distance = %d, want %d (h1=%016x h2=%016x)", d, HashBits, h1, h2)
	}
	if s := SimilarityFromHashes(h1, h2); s != 0 {
		t.Errorf("similarity = %v, want 0", s)
	}
}
