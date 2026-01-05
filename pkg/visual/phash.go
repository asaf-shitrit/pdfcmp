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

	// Use a flat slice for the 32x32 matrix to avoid 33+ allocations
	// 32*32 = 1024 floats.
	pixels := make([]float64, pHashInputSize*pHashInputSize)
	
	for y := 0; y < pHashInputSize; y++ {
		offset := y * pHashInputSize
		for x := 0; x < pHashInputSize; x++ {
			pixels[offset+x] = float64(grayscaleAt(resized, x, y))
		}
	}

	// Compute 2D DCT in-place or with minimal buffer
	dctBuffer := make([]float64, pHashInputSize*pHashInputSize)
	dct2DFlat(pixels, dctBuffer)

	// Extract top-left 8x8 (low frequencies) and compute mean
	// Skip DC component (0,0) as it just represents average brightness
	var sum float64
	count := 0
	for y := 0; y < pHashDCTSize; y++ {
		offset := y * pHashInputSize
		for x := 0; x < pHashDCTSize; x++ {
			if x != 0 || y != 0 { // Skip DC component
				sum += dctBuffer[offset+x]
				count++
			}
		}
	}
	mean := sum / float64(count)

	// Build hash based on mean comparison
	var hash uint64
	bit := 0
	for y := 0; y < pHashDCTSize; y++ {
		offset := y * pHashInputSize
		for x := 0; x < pHashDCTSize; x++ {
			if x != 0 || y != 0 { // Skip DC component
				if dctBuffer[offset+x] > mean {
					hash |= 1 << uint(bit)
				}
				bit++
			}
		}
	}

	return hash
}

// dct2DFlat computes 2D DCT on a flat 32x32 array.
// Results are stored in 'output'. 'input' is treated as row-major.
// This version allocates significantly less memory than the slice-of-slices version.
func dct2DFlat(input, output []float64) {
	size := pHashInputSize
	
	// Temporary buffer for transpose/intermediate results
	// We can reuse 'output' as intermediate storage for row DCTs, 
	// but we need a separate buffer for column processing to avoid overwriting.
	// Actually, we can do Row DCT -> Output, then Col DCT on Output.
	
	// 1. DCT on Rows
	// For each row y, read from input, write to output
	rowBuf := make([]float64, size) // Reusable line buffer
	outBuf := make([]float64, size)
	
	for y := 0; y < size; y++ {
		rowOffset := y * size
		// Copy row
		copy(rowBuf, input[rowOffset:rowOffset+size])
		
		// DCT 1D
		dct1D(rowBuf, outBuf)
		
		// Write back to output
		copy(output[rowOffset:rowOffset+size], outBuf)
	}

	// 2. DCT on Columns
	// We need to read columns from 'output', transform them, and write back.
	// Since write-back affects future reads of the same col (not really, but accessing cols is tricky),
	// we'll process one column at a time.
	colBuf := make([]float64, size)
	
	for x := 0; x < size; x++ {
		// Extract column from 'output' (which currently holds Row-DCT result)
		for y := 0; y < size; y++ {
			colBuf[y] = output[y*size+x]
		}
		
		// DCT 1D
		dct1D(colBuf, outBuf)
		
		// Write back
		for y := 0; y < size; y++ {
			output[y*size+x] = outBuf[y]
		}
	}
}

// dct1D computes 1D DCT-II using precomputed cosine table.
// Writes result to 'out'. 'in' and 'out' must be size pHashInputSize (32).
// No allocations.
func dct1D(in, out []float64) {
	n := pHashInputSize

	for k := 0; k < n; k++ {
		var sum float64
		for i := 0; i < n; i++ {
			sum += in[i] * cosTable[i][k]
		}
		// Apply scaling factor
		if k == 0 {
			out[k] = sum * math.Sqrt(1.0/float64(n))
		} else {
			out[k] = sum * math.Sqrt(2.0/float64(n))
		}
	}
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
