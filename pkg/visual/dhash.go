package visual

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/nfnt/resize"
)

const (
	// dHashWidth is the width for dHash (9 pixels to produce 8 comparisons)
	dHashWidth = 9
	// dHashHeight is the height for dHash (8 rows for 64 bits total)
	dHashHeight = 8
)

// DHash computes a difference hash for an image.
// The algorithm:
// 1. Resize image to 9x8 grayscale
// 2. Compare adjacent pixels horizontally (left < right = 1, else 0)
// 3. Produce 64-bit hash (8 rows x 8 comparisons per row)
//
// dHash is fast and good at detecting identical or near-identical images.
// It's less robust to rotation/scaling than pHash but faster to compute.
func DHash(img image.Image) uint64 {
	// Resize to 9x8 using NearestNeighbor for speed
	resized := resize.Resize(dHashWidth, dHashHeight, img, resize.NearestNeighbor)

	// Convert to grayscale and compute hash
	var hash uint64
	bit := 0

	for y := 0; y < dHashHeight; y++ {
		for x := 0; x < dHashWidth-1; x++ {
			// Get grayscale values for adjacent pixels
			left := grayscaleAt(resized, x, y)
			right := grayscaleAt(resized, x+1, y)

			// Set bit if left < right
			if left < right {
				hash |= 1 << uint(bit)
			}
			bit++
		}
	}

	return hash
}

// ComputeDHashPair computes dHashes for two images and their hamming distance.
func ComputeDHashPair(img1, img2 image.Image) (uint64, uint64, int) {
	h1 := DHash(img1)
	h2 := DHash(img2)
	return h1, h2, HammingDistance(h1, h2)
}

// grayscaleAt returns the grayscale value (0-255) at the given pixel.
func grayscaleAt(img image.Image, x, y int) uint8 {
	c := img.At(x, y)

	// Fast path for already grayscale images
	if g, ok := c.(color.Gray); ok {
		return g.Y
	}

	// Convert to grayscale using standard luminosity formula
	r, g, b, _ := c.RGBA()
	// RGBA() returns 16-bit values, scale to 8-bit
	// Using standard luminosity: 0.299R + 0.587G + 0.114B
	gray := (299*r + 587*g + 114*b) / 1000
	return uint8(gray >> 8)
}

// DHashFromGray computes dHash from an already-grayscale image.
// This is faster when you know the image is already grayscale.
func DHashFromGray(img *image.Gray) uint64 {
	resized := resize.Resize(dHashWidth, dHashHeight, img, resize.Bilinear)

	var hash uint64
	bit := 0

	for y := 0; y < dHashHeight; y++ {
		for x := 0; x < dHashWidth-1; x++ {
			left := grayscaleAt(resized, x, y)
			right := grayscaleAt(resized, x+1, y)

			if left < right {
				hash |= 1 << uint(bit)
			}
			bit++
		}
	}

	return hash
}

// ToGrayscale converts any image to grayscale.
// Uses draw.Draw for efficient bulk conversion instead of pixel-by-pixel iteration.
func ToGrayscale(img image.Image) *image.Gray {
	// Fast path: already grayscale
	if g, ok := img.(*image.Gray); ok {
		return g
	}

	bounds := img.Bounds()
	gray := image.NewGray(bounds)

	// Use draw.Draw for efficient bulk conversion
	// The draw package handles color model conversion internally
	draw.Draw(gray, bounds, img, bounds.Min, draw.Src)

	return gray
}
