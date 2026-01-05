package render

import (
	"context"
	"image"
)

// DPI constants for different comparison modes
const (
	DPIThumbnail = 72  // Quick preview mode
	DPIStandard  = 150 // Normal comparison
	DPIHighRes   = 300 // Detailed analysis
)

// Options configures PDF rendering behavior
type Options struct {
	DPI int // Dots per inch for rendering
}

// DefaultOptions returns sensible default rendering options
func DefaultOptions() Options {
	return Options{
		DPI: DPIThumbnail, // 72 DPI is sufficient for hashing and much faster
	}
}

// PageInfo contains metadata about a PDF page
type PageInfo struct {
	Number      int     // 0-indexed page number
	WidthPts    float64 // Width in points (1/72 inch)
	HeightPts   float64 // Height in points
	WidthPx     int     // Width in pixels at current DPI
	HeightPx    int     // Height in pixels at current DPI
}

// RenderedPage contains a rendered image and its index
type RenderedPage struct {
	Index int
	Image image.Image
	Err   error
}

// Document represents a PDF document that can be rendered
type Document interface {
	// PageCount returns the total number of pages
	PageCount() int

	// PageInfo returns information about a specific page
	PageInfo(pageNum int) (PageInfo, error)

	// RenderPage renders a page to an image
	RenderPage(ctx context.Context, pageNum int, opts Options) (image.Image, error)

	// RenderPages renders multiple pages concurrently
	RenderPages(ctx context.Context, pageNums []int, opts Options) ([]image.Image, error)

	// RenderPagesStream renders pages and sends them to a channel as they are ready
	RenderPagesStream(ctx context.Context, pageNums []int, opts Options, out chan<- RenderedPage) error

	// Close releases resources associated with the document
	Close() error
}

// Renderer is the interface for PDF rendering backends
type Renderer interface {
	// Open opens a PDF file and returns a Document
	Open(path string) (Document, error)

	// OpenWithPassword opens a password-protected PDF
	OpenWithPassword(path string, password string) (Document, error)

	// Close releases resources held by the renderer
	Close() error
}

// DPI limits to prevent overflow
const (
	MinAllowedDPI = 1
	MaxAllowedDPI = 2400 // Beyond this, memory usage becomes excessive
)

// CalculatePixelDimensions calculates pixel dimensions for a page at given DPI.
// DPI is clamped to safe bounds to prevent integer overflow.
func CalculatePixelDimensions(widthPts, heightPts float64, dpi int) (widthPx, heightPx int) {
	// Clamp DPI to safe bounds
	if dpi < MinAllowedDPI {
		dpi = MinAllowedDPI
	}
	if dpi > MaxAllowedDPI {
		dpi = MaxAllowedDPI
	}

	widthPx = int(widthPts * float64(dpi) / 72.0)
	heightPx = int(heightPts * float64(dpi) / 72.0)

	// Ensure non-negative dimensions
	if widthPx < 0 {
		widthPx = 0
	}
	if heightPx < 0 {
		heightPx = 0
	}

	return
}

// EstimateMemoryUsage estimates memory needed for a page at given DPI (in bytes)
// Assumes 4 bytes per pixel (RGBA)
func EstimateMemoryUsage(widthPts, heightPts float64, dpi int) int64 {
	w, h := CalculatePixelDimensions(widthPts, heightPts, dpi)
	return int64(w) * int64(h) * 4
}
