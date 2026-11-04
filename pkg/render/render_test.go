package render

import "testing"

func TestCalculatePixelDimensions(t *testing.T) {
	w, h := CalculatePixelDimensions(72, 144, 72) // 1 inch x 2 inches at 72 DPI
	if w != 72 || h != 144 {
		t.Errorf("Expected 72x144, got %dx%d", w, h)
	}

	w, h = CalculatePixelDimensions(72, 144, 144) // 1 inch x 2 inches at 144 DPI
	if w != 144 || h != 288 {
		t.Errorf("Expected 144x288, got %dx%d", w, h)
	}
}

func TestEstimateMemoryUsage(t *testing.T) {
	// 100x100 points at 72 DPI = 100x100 pixels.
	// 4 bytes per pixel = 40,000 bytes.
	mem := EstimateMemoryUsage(100, 100, 72)
	expected := int64(100 * 100 * 4)
	if mem != expected {
		t.Errorf("Expected %d bytes, got %d", expected, mem)
	}
}
