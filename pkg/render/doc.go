// Package render provides PDF rendering capabilities using PDFium WebAssembly.
//
// The package abstracts PDF rendering through the Renderer and Document interfaces,
// allowing for efficient page rendering with configurable DPI settings.
//
// # Architecture
//
// The rendering system uses a pool of PDFium WebAssembly instances for parallel
// processing. Each worker gets its own instance from the pool to avoid contention.
//
// # DPI Settings
//
// Three DPI presets are provided for different use cases:
//
//   - DPIThumbnail (72): Quick preview mode, good for initial screening
//   - DPIStandard (150): Normal comparison, balanced quality and speed
//   - DPIHighRes (300): Detailed analysis, higher accuracy
//
// # Streaming API
//
// For large documents, use RenderPagesStream to process pages as they become
// available without buffering the entire document in memory:
//
//	out := make(chan render.RenderedPage, pageCount)
//	err := doc.RenderPagesStream(ctx, pageNums, opts, out)
//	for page := range out {
//	    // Process each page as it arrives
//	}
//
// # Memory Management
//
// The package provides utilities to estimate memory usage:
//
//	mem := render.EstimateMemoryUsage(widthPts, heightPts, dpi)
//	fmt.Printf("Estimated memory: %d bytes\n", mem)
package render
