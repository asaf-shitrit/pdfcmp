package visual

import (
	"image"
	"math"
)

// HistogramResult contains basic color distribution info
type HistogramResult struct {
	AverageR float64
	AverageG float64
	AverageB float64
	Luminance float64
}

// ComputeHistogram calculates basic color metrics for an image.
// This is used as a fast heuristic to skip more expensive dHash/pHash
// if the images are obviously different in color distribution.
func ComputeHistogram(img image.Image) HistogramResult {
	bounds := img.Bounds()
	
	var rSum, gSum, bSum uint64
	
	// Sample pixels to save time (every 4th pixel)
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 4 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 4 {
			r, g, b, _ := img.At(x, y).RGBA()
			// RGBA() returns values in range [0, 65535]
			rSum += uint64(r >> 8)
			gSum += uint64(g >> 8)
			bSum += uint64(b >> 8)
			count++
		}
	}
	
	if count == 0 {
		return HistogramResult{}
	}
	
	avgR := float64(rSum) / float64(count)
	avgG := float64(gSum) / float64(count)
	avgB := float64(bSum) / float64(count)
	
	// Simple relative luminance formula
	lum := 0.299*avgR + 0.587*avgG + 0.114*avgB
	
	return HistogramResult{
		AverageR:  avgR,
		AverageG:  avgG,
		AverageB:  avgB,
		Luminance: lum,
	}
}

// HistogramSimilarity compares two histograms and returns a similarity score [0, 1].
func HistogramSimilarity(h1, h2 HistogramResult) float64 {
	// Calculate Euclidean distance in RGB space (max distance is sqrt(3 * 255^2) ≈ 441.67)
	dr := h1.AverageR - h2.AverageR
	dg := h1.AverageG - h2.AverageG
	db := h1.AverageB - h2.AverageB
	
	dist := math.Sqrt(dr*dr + dg*dg + db*db)
	maxDist := 441.67
	
	similarity := 1.0 - (dist / maxDist)
	if similarity < 0 {
		return 0
	}
	return similarity
}
