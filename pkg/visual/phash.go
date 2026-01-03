package visual

import (
	"image"
	"math"

	"github.com/nfnt/resize"
)

const (
	// pHashInputSize is the size to resize images before DCT
	pHashInputSize = 32
	// pHashDCTSize is the size of the low-frequency DCT block to use
	pHashDCTSize = 8
)

// precomputed cosine values for DCT
var cosTable [pHashInputSize][pHashInputSize]float64

func init() {
	// Precompute cosine table for DCT
	for i := 0; i < pHashInputSize; i++ {
		for j := 0; j < pHashInputSize; j++ {
			cosTable[i][j] = math.Cos(math.Pi * float64(2*i+1) * float64(j) / float64(2*pHashInputSize))
		}
	}
}

// PHash computes a perceptual hash for an image using DCT.
// The algorithm:
// 1. Resize image to 32x32 grayscale
// 2. Compute 2D DCT (Discrete Cosine Transform)
// 3. Extract top-left 8x8 block (low frequencies)
// 4. Compute mean of DCT values (excluding DC component)
// 5. Set bits based on whether each value exceeds the mean
//
// pHash is more robust to minor image changes (compression, scaling)
// than dHash, but slower to compute due to DCT.
func PHash(img image.Image) uint64 {
	// Resize to 32x32 using NearestNeighbor for speed
	resized := resize.Resize(pHashInputSize, pHashInputSize, img, resize.NearestNeighbor)

	// Convert to grayscale float matrix
	pixels := make([][]float64, pHashInputSize)
	for y := 0; y < pHashInputSize; y++ {
		pixels[y] = make([]float64, pHashInputSize)
		for x := 0; x < pHashInputSize; x++ {
			pixels[y][x] = float64(grayscaleAt(resized, x, y))
		}
	}

	// Compute 2D DCT
	dct := dct2D(pixels)

	// Extract top-left 8x8 (low frequencies) and compute mean
	// Skip DC component (0,0) as it just represents average brightness
	var sum float64
	count := 0
	for y := 0; y < pHashDCTSize; y++ {
		for x := 0; x < pHashDCTSize; x++ {
			if x != 0 || y != 0 { // Skip DC component
				sum += dct[y][x]
				count++
			}
		}
	}
	mean := sum / float64(count)

	// Build hash based on mean comparison
	var hash uint64
	bit := 0
	for y := 0; y < pHashDCTSize; y++ {
		for x := 0; x < pHashDCTSize; x++ {
			if x != 0 || y != 0 { // Skip DC component
				if dct[y][x] > mean {
					hash |= 1 << uint(bit)
				}
				bit++
			}
		}
	}

	return hash
}

// dct2D computes 2D DCT using separable 1D DCT (rows then columns).
// This is more efficient than naive 2D DCT: O(n^3) vs O(n^4).
func dct2D(matrix [][]float64) [][]float64 {
	size := len(matrix)

	// DCT on rows
	intermediate := make([][]float64, size)
	for y := 0; y < size; y++ {
		intermediate[y] = dct1D(matrix[y])
	}

	// DCT on columns (transpose, apply DCT, transpose back)
	result := make([][]float64, size)
	for y := 0; y < size; y++ {
		result[y] = make([]float64, size)
	}

	for x := 0; x < size; x++ {
		// Extract column
		col := make([]float64, size)
		for y := 0; y < size; y++ {
			col[y] = intermediate[y][x]
		}
		// Apply DCT to column
		dctCol := dct1D(col)
		// Put back
		for y := 0; y < size; y++ {
			result[y][x] = dctCol[y]
		}
	}

	return result
}

// dct1D computes 1D DCT-II using precomputed cosine table.
func dct1D(input []float64) []float64 {
	n := len(input)
	output := make([]float64, n)

	for k := 0; k < n; k++ {
		var sum float64
		for i := 0; i < n; i++ {
			sum += input[i] * cosTable[i][k]
		}
		// Apply scaling factor
		if k == 0 {
			output[k] = sum * math.Sqrt(1.0/float64(n))
		} else {
			output[k] = sum * math.Sqrt(2.0/float64(n))
		}
	}

	return output
}

// PHashFast computes pHash using a faster but slightly less accurate approach.
// Uses only the first 8x8 pixels of a 32x32 resize instead of full DCT.
// Suitable for quick preliminary comparisons.
func PHashFast(img image.Image) uint64 {
	// Resize to 8x8 directly
	resized := resize.Resize(pHashDCTSize, pHashDCTSize, img, resize.Bilinear)

	// Compute mean grayscale value
	var sum int
	for y := 0; y < pHashDCTSize; y++ {
		for x := 0; x < pHashDCTSize; x++ {
			sum += int(grayscaleAt(resized, x, y))
		}
	}
	mean := uint8(sum / (pHashDCTSize * pHashDCTSize))

	// Build hash based on mean comparison
	var hash uint64
	bit := 0
	for y := 0; y < pHashDCTSize; y++ {
		for x := 0; x < pHashDCTSize; x++ {
			if grayscaleAt(resized, x, y) > mean {
				hash |= 1 << uint(bit)
			}
			bit++
		}
	}

	return hash
}

// ComputePHashPair computes pHashes for two images and their hamming distance.
func ComputePHashPair(img1, img2 image.Image) (uint64, uint64, int) {
	h1 := PHash(img1)
	h2 := PHash(img2)
	return h1, h2, HammingDistance(h1, h2)
}

// ComputeBothHashes computes both dHash and pHash for an image.
// This is slightly more efficient than calling them separately
// as we can share the grayscale conversion.
func ComputeBothHashes(img image.Image) (dHash, pHash uint64) {
	dHash = DHash(img)
	pHash = PHash(img)
	return
}
