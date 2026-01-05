package bench

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math/rand"
	"os"
	"testing"
	"text/tabwriter"

	"github.com/asafshitrit/pdfcmp/pkg/visual"
	"github.com/nfnt/resize"
)

// TestAccuracyReport generates a report on how well the algorithms maintain
// high similarity scores despite various image degradations.
func TestAccuracyReport(t *testing.T) {
	// 1. Create Base Image (High details)
	baseImg := createDetailedImage(500, 500)
	
	// Define transformations
	tests := []struct {
		name      string
		transform func(image.Image) image.Image
		minScore  float64 // Expected minimum similarity
	}{
		{
			name: "Identical", 
			transform: func(img image.Image) image.Image { return img },
			minScore: 1.0,
		},
		{
			name: "Shift_1px",
			transform: func(img image.Image) image.Image { return shiftImage(img, 1, 1) },
			minScore: 0.90,
		},
		{
			name: "Shift_5px",
			transform: func(img image.Image) image.Image { return shiftImage(img, 5, 5) },
			minScore: 0.70, // dHash is sensitive to alignment
		},
		{
			name: "Resize_90%",
			transform: func(img image.Image) image.Image {
				return resize.Resize(450, 450, img, resize.Lanczos3)
			},
			minScore: 0.95, // pHash should handle scaling well
		},
		{
			name: "Noise_5%",
			transform: func(img image.Image) image.Image { return addNoise(img, 0.05) },
			minScore: 0.90,
		},
		{
			name: "Brightness_+10%",
			transform: func(img image.Image) image.Image { return adjustBrightness(img, 1.1) },
			minScore: 0.95,
		},
		{
			name: "Crop_Center",
			transform: func(img image.Image) image.Image { return cropImage(img, 10) },
			minScore: 0.80,
		},
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "\n--- Accuracy Benchmark Report ---")
	fmt.Fprintln(w, "Scenario\tDHash\tPHash\tCombined\tHistogram\tPass?")
	fmt.Fprintln(w, "--------\t-----\t-----\t--------\t---------\t-----")

	for _, tc := range tests {
		modImg := tc.transform(baseImg)

		// Calculate Metrics
		d1, d2, dDist := visual.ComputeDHashPair(baseImg, modImg)
		p1, p2, pDist := visual.ComputePHashPair(baseImg, modImg)
		
		dSim := visual.Similarity(dDist)
		pSim := visual.Similarity(pDist)
		
		// Use manual combination for transparency in report
		combSim := visual.CombinedSimilarity(d1, d2, p1, p2)

		// Histogram
		h1 := visual.ComputeHistogram(baseImg)
		h2 := visual.ComputeHistogram(modImg)
		hSim := visual.HistogramSimilarity(h1, h2)

		passed := combSim >= tc.minScore || pSim >= tc.minScore // pHash is often the fallback
		status := "✅"
		if !passed {
			status = "❌"
		}

		fmt.Fprintf(w, "%s\t%.4f\t%.4f\t%.4f\t%.4f\t%s\n", 
			tc.name, dSim, pSim, combSim, hSim, status)
	}
	w.Flush()
	fmt.Println()
}

// Helpers

func createDetailedImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Draw noise/gradient
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{
				R: uint8((x ^ y) & 255),
				G: uint8((x * y) & 255),
				B: uint8(((x + y) * 128) & 255),
				A: 255,
			}
			img.Set(x, y, c)
		}
	}
	// Draw a rect to add structure
	draw.Draw(img, image.Rect(50, 50, w-50, h-50), &image.Uniform{color.RGBA{200, 50, 50, 255}}, image.Point{}, draw.Over)
	return img
}

func shiftImage(img image.Image, dx, dy int) image.Image {
	b := img.Bounds()
	newImg := image.NewRGBA(b)
	draw.Draw(newImg, b.Add(image.Pt(dx, dy)), img, b.Min, draw.Src)
	return newImg
}

func addNoise(img image.Image, factor float64) image.Image {
	// Use a fixed seed for reproducible noise in benchmarks
	r := rand.New(rand.NewSource(42))
	
	b := img.Bounds()
	newImg := image.NewRGBA(b)
	draw.Draw(newImg, b, img, b.Min, draw.Src)
	
	limit := int(float64(b.Dx()*b.Dy()) * factor)
	for i := 0; i < limit; i++ {
		x := r.Intn(b.Dx())
		y := r.Intn(b.Dy())
		newImg.Set(x, y, color.RGBA{uint8(r.Intn(255)), uint8(r.Intn(255)), uint8(r.Intn(255)), 255})
	}
	return newImg
}

func adjustBrightness(img image.Image, factor float64) image.Image {
	b := img.Bounds()
	newImg := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			// Convert back to uint8
			newImg.Set(x, y, color.RGBA{
				R: uint8(checkBounds(float64(r>>8) * factor)),
				G: uint8(checkBounds(float64(g>>8) * factor)),
				B: uint8(checkBounds(float64(b>>8) * factor)),
				A: uint8(a >> 8),
			})
		}
	}
	return newImg
}

func cropImage(img image.Image, border int) image.Image {
	b := img.Bounds()
	// Create new smaller image
	w, h := b.Dx()-border*2, b.Dy()-border*2
	newImg := image.NewRGBA(image.Rect(0, 0, w, h))
	// Draw cropped logic
	draw.Draw(newImg, newImg.Bounds(), img, image.Pt(border, border), draw.Src)
	return newImg
}

func checkBounds(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
