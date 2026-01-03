package visual

import (
	"image"
	"image/color"
	"testing"
)

func FuzzHammingDistance(f *testing.F) {
	f.Add(uint64(0), uint64(0))
	f.Add(uint64(0xFFFFFFFFFFFFFFFF), uint64(0))
	f.Add(uint64(0x5555555555555555), uint64(0xAAAAAAAAAAAAAAAA))
	
	f.Fuzz(func(t *testing.T, a uint64, b uint64) {
		dist := HammingDistance(a, b)
		if dist < 0 || dist > 64 {
			t.Errorf("Invalid hamming distance: %d", dist)
		}
		
		// Symmetry: dist(a, b) == dist(b, a)
		if dist != HammingDistance(b, a) {
			t.Errorf("Hamming distance symmetry violated")
		}
		
		// Identity: dist(a, a) == 0
		if HammingDistance(a, a) != 0 {
			t.Errorf("Hamming distance identity violated for %d", a)
		}
	})
}

func FuzzSimilarity(f *testing.F) {
	f.Add(0)
	f.Add(64)
	f.Add(32)
	
	f.Fuzz(func(t *testing.T, dist int) {
		// dist can be anything from fuzzer, but our function expects 0-64
		// We should ensure it doesn't crash even if dist is large.
		s := Similarity(dist)
		if dist < 0 || dist > 64 {
			// Expected behavior for out of range?
			return
		}
		if s < 0.0 || s > 1.0 {
			t.Errorf("Invalid similarity: %f for dist %d", s, dist)
		}
	})
}

// Fuzzing images is expensive but useful for robustness
func FuzzDHash(f *testing.F) {
	// Add a 8x8 image
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 10), uint8(y * 10), 0, 255})
		}
	}
	f.Add(8, 8, []byte(img.Pix))

	f.Fuzz(func(t *testing.T, w, h int, pix []byte) {
		if w <= 0 || h <= 0 || w > 128 || h > 128 {
			return
		}
		if len(pix) < w*h*4 {
			return
		}
		
		img := &image.RGBA{
			Pix:    pix[:w*h*4],
			Stride: w * 4,
			Rect:   image.Rect(0, 0, w, h),
		}
		
		// Ensure it doesn't panic
		_ = DHash(img)
	})
}
