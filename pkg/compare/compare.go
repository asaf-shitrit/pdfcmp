package compare

import (
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/asafshitrit/pdfcmp/pkg/bytewise"
	"github.com/asafshitrit/pdfcmp/pkg/render"
	"github.com/asafshitrit/pdfcmp/pkg/visual"
)

// Comparer is the main interface for PDF comparison
type Comparer interface {
	// Compare compares two PDF files
	Compare(ctx context.Context, pdf1, pdf2 string) (*Result, error)

	// Close releases resources
	Close() error
}

// comparer implements Comparer
type comparer struct {
	opts     Options
	renderer render.Renderer
}

// New creates a new Comparer with the given options
func New(opts ...Option) (Comparer, error) {
	options := DefaultOptions()
	options.Apply(opts...)

	// Validate options before proceeding
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	renderer, err := render.NewPdfiumRenderer()
	if err != nil {
		return nil, fmt.Errorf("failed to create renderer: %w", err)
	}

	return &comparer{
		opts:     options,
		renderer: renderer,
	}, nil
}

// Quick performs a quick byte-wise only comparison
func Quick(pdf1, pdf2 string) (bool, error) {
	result, err := bytewise.Compare(pdf1, pdf2)
	if err != nil {
		return false, err
	}
	return result.Identical, nil
}

func (c *comparer) Compare(ctx context.Context, pdf1, pdf2 string) (*Result, error) {
	start := time.Now()
	log := c.opts.Logger

	log.Info("Starting PDF comparison",
		F("file1", pdf1),
		F("file2", pdf2),
		F("dpi", c.opts.DPI),
		F("workers", c.opts.Workers))

	result := &Result{
		Metadata: Metadata{
			File1: pdf1,
			File2: pdf2,
		},
		ComparisonLayers: []string{},
	}

	// Check for self-comparison (comparing file to itself)
	abs1, err1 := filepath.Abs(pdf1)
	abs2, err2 := filepath.Abs(pdf2)
	if err1 == nil && err2 == nil && abs1 == abs2 {
		log.Debug("Self-comparison detected, returning identical")
		// Same file - always identical
		result.Identical = true
		result.SimilarityScore = 1.0
		result.VisualComparison = VisualResult{
			OverallScore:     1.0,
			SamplingStrategy: "skipped (same file)",
		}
		result.Duration = time.Since(start)
		result.Metadata.DurationMS = result.Duration.Milliseconds()
		return result, nil
	}

	// Get file info
	stat1, err := os.Stat(pdf1)
	if err != nil {
		return nil, fmt.Errorf("cannot access %s: %w", pdf1, err)
	}
	stat2, err := os.Stat(pdf2)
	if err != nil {
		return nil, fmt.Errorf("cannot access %s: %w", pdf2, err)
	}
	result.Metadata.Size1 = stat1.Size()
	result.Metadata.Size2 = stat2.Size()

	// Layer 1: Byte-wise comparison (fastest)
	if c.opts.Mode == ModeAuto || c.opts.Mode == ModeByte || c.opts.Mode == ModeFull {
		result.ComparisonLayers = append(result.ComparisonLayers, "bytewise")
		c.reportProgress("Byte comparison", 0, 1, 0.1)

		byteResult, err := bytewise.Compare(pdf1, pdf2)
		if err != nil {
			return nil, fmt.Errorf("byte comparison failed: %w", err)
		}

		result.ByteComparison = ByteResult{
			Identical:  byteResult.Identical,
			Hash1:      byteResult.Hash1,
			Hash2:      byteResult.Hash2,
			Size1:      byteResult.Size1,
			Size2:      byteResult.Size2,
			SizesMatch: byteResult.SizesMatch,
		}

		log.Debug("Byte comparison complete",
			F("identical", byteResult.Identical),
			F("sizesMatch", byteResult.SizesMatch))

		// Early exit if byte-identical
		if byteResult.Identical {
			log.Info("Files are byte-identical, skipping visual comparison")
			result.Identical = true
			result.SimilarityScore = 1.0
			result.VisualComparison = VisualResult{
				OverallScore:     1.0,
				SamplingStrategy: "skipped (byte identical)",
			}
			result.Duration = time.Since(start)
			result.Metadata.DurationMS = result.Duration.Milliseconds()
			return result, nil
		}

		// If only byte comparison requested, we're done
		if c.opts.Mode == ModeByte {
			result.SimilarityScore = 0.0
			result.Duration = time.Since(start)
			result.Metadata.DurationMS = result.Duration.Milliseconds()
			return result, nil
		}
	}

	// Open PDFs for visual comparison
	doc1, err := c.renderer.Open(pdf1)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", pdf1, err)
	}
	defer doc1.Close()

	doc2, err := c.renderer.Open(pdf2)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", pdf2, err)
	}
	defer doc2.Close()

	result.Metadata.Pages1 = doc1.PageCount()
	result.Metadata.Pages2 = doc2.PageCount()

	// Handle empty PDFs (0 pages)
	if doc1.PageCount() == 0 && doc2.PageCount() == 0 {
		log.Debug("Both PDFs have 0 pages, treating as identical")
		result.Identical = true
		result.SimilarityScore = 1.0
		result.VisualComparison = VisualResult{
			OverallScore:     1.0,
			SamplingStrategy: "empty documents",
		}
		result.Duration = time.Since(start)
		result.Metadata.DurationMS = result.Duration.Milliseconds()
		return result, nil
	}

	if doc1.PageCount() == 0 || doc2.PageCount() == 0 {
		log.Debug("One PDF has 0 pages", F("pages1", doc1.PageCount()), F("pages2", doc2.PageCount()))
		result.SimilarityScore = 0.0
		result.VisualComparison = VisualResult{
			OverallScore:     0.0,
			SamplingStrategy: "empty document mismatch",
		}
		result.Duration = time.Since(start)
		result.Metadata.DurationMS = result.Duration.Milliseconds()
		return result, nil
	}

	// If page counts differ, we know they're different
	if doc1.PageCount() != doc2.PageCount() {
		result.SimilarityScore = 0.0
		result.VisualComparison = VisualResult{
			OverallScore:     0.0,
			SamplingStrategy: "page count mismatch",
		}
		result.Duration = time.Since(start)
		result.Metadata.DurationMS = result.Duration.Milliseconds()
		return result, nil
	}

	pageCount := doc1.PageCount()

	// Layer 2: Thumbnail comparison (quick visual check)
	if c.opts.Sampling == SamplingAuto || c.opts.Sampling == SamplingThumbnailOnly {
		result.ComparisonLayers = append(result.ComparisonLayers, "thumbnail")
		c.reportProgress("Thumbnail check", 0, 2, 0.2)

		thumbnailPages := []int{0} // First page
		if pageCount > 1 {
			thumbnailPages = append(thumbnailPages, pageCount-1) // Last page
		}

		thumbnailOpts := render.Options{DPI: render.DPIThumbnail}
		thumbnailResults, err := c.comparePages(ctx, doc1, doc2, thumbnailPages, thumbnailOpts)
		if err != nil {
			return nil, fmt.Errorf("thumbnail comparison failed: %w", err)
		}

		// Check if thumbnails are very different (early exit)
		avgSimilarity := averageSimilarity(thumbnailResults)
		if avgSimilarity < ThumbnailEarlyExitThresh {
			result.SimilarityScore = avgSimilarity
			result.VisualComparison = VisualResult{
				OverallScore:     avgSimilarity,
				HashType:         hashTypeName(c.opts.HashType),
				PagesCompared:    len(thumbnailPages),
				SamplingStrategy: "thumbnail (early exit)",
				DPI:              render.DPIThumbnail,
			}
			result.PageResults = thumbnailResults
			result.Duration = time.Since(start)
			result.Metadata.DurationMS = result.Duration.Milliseconds()
			return result, nil
		}

		if c.opts.Sampling == SamplingThumbnailOnly {
			result.SimilarityScore = avgSimilarity
			result.VisualComparison = VisualResult{
				OverallScore:     avgSimilarity,
				HashType:         hashTypeName(c.opts.HashType),
				PagesCompared:    len(thumbnailPages),
				SamplingStrategy: "thumbnail only",
				DPI:              render.DPIThumbnail,
			}
			result.PageResults = thumbnailResults
			result.Duration = time.Since(start)
			result.Metadata.DurationMS = result.Duration.Milliseconds()
			return result, nil
		}
	}

	// Layer 3: Strategic sampling
	var pagesToCompare []int
	var samplingName string

	switch c.opts.Sampling {
	case SamplingAll:
		pagesToCompare = allPages(pageCount)
		samplingName = "all"
	case SamplingStrategic, SamplingAuto:
		pagesToCompare = strategicSample(pageCount)
		samplingName = "strategic"
	default:
		pagesToCompare = allPages(pageCount)
		samplingName = "all"
	}

	result.ComparisonLayers = append(result.ComparisonLayers, "sample")
	renderOpts := render.Options{DPI: c.opts.DPI}

	// Compare sampled pages
	c.reportProgress("Sampling pages", 0, len(pagesToCompare), 0.4)
	sampleResults, err := c.comparePages(ctx, doc1, doc2, pagesToCompare, renderOpts)
	if err != nil {
		return nil, fmt.Errorf("sample comparison failed: %w", err)
	}

	sampleSimilarity := averageSimilarity(sampleResults)

	// Layer 4: Full comparison if samples are very similar
	if sampleSimilarity >= c.opts.SimilarityThreshold && c.opts.Sampling == SamplingAuto && len(pagesToCompare) < pageCount {
		result.ComparisonLayers = append(result.ComparisonLayers, "full")
		c.reportProgress("Full comparison", 0, pageCount, 0.6)

		allPageNums := allPages(pageCount)
		fullResults, err := c.comparePages(ctx, doc1, doc2, allPageNums, renderOpts)
		if err != nil {
			return nil, fmt.Errorf("full comparison failed: %w", err)
		}

		result.SimilarityScore = averageSimilarity(fullResults)
		result.VisualComparison = VisualResult{
			OverallScore:     result.SimilarityScore,
			HashType:         hashTypeName(c.opts.HashType),
			PagesCompared:    pageCount,
			SamplingStrategy: "full",
			DPI:              c.opts.DPI,
		}
		result.PageResults = fullResults
	} else {
		result.SimilarityScore = sampleSimilarity
		result.VisualComparison = VisualResult{
			OverallScore:     sampleSimilarity,
			HashType:         hashTypeName(c.opts.HashType),
			PagesCompared:    len(pagesToCompare),
			SamplingStrategy: samplingName,
			DPI:              c.opts.DPI,
		}
		result.PageResults = sampleResults
	}

	result.Duration = time.Since(start)
	result.Metadata.DurationMS = result.Duration.Milliseconds()

	log.Info("Comparison complete",
		F("similarity", result.SimilarityScore),
		F("identical", result.Identical),
		F("pagesCompared", result.VisualComparison.PagesCompared),
		F("duration", result.Duration))

	return result, nil
}

// comparePages compares specific pages between two documents using a pipelined approach.
// It renders and hashes pages concurrently, processing each page as soon as it's rendered
// in both documents.
func (c *comparer) comparePages(ctx context.Context, doc1, doc2 render.Document, pages []int, opts render.Options) ([]PageResult, error) {
	if len(pages) == 0 {
		return nil, nil
	}

	// Adjust workers based on memory constraints
	effectiveWorkers := c.adjustWorkersForMemory(doc1)

	c.reportProgress("Pipelined processing", 0, len(pages), 0.3)

	// Result storage
	results := make([]PageResult, len(pages))

	// Buffer size for backpressure: 2x workers to allow pipeline overlap
	// This prevents unbounded memory growth for large documents
	bufferSize := effectiveWorkers * 2
	if bufferSize > len(pages) {
		bufferSize = len(pages)
	}

	// Channels for rendered pages (bounded for backpressure)
	out1 := make(chan render.RenderedPage, bufferSize)
	out2 := make(chan render.RenderedPage, bufferSize)

	// Start streamers
	var wgRender sync.WaitGroup
	wgRender.Add(2)

	errCh := make(chan error, 2)

	go func() {
		defer wgRender.Done()
		if err := doc1.RenderPagesStream(ctx, pages, opts, out1); err != nil {
			errCh <- fmt.Errorf("doc1 render failed: %w", err)
		}
		close(out1)
	}()

	go func() {
		defer wgRender.Done()
		if err := doc2.RenderPagesStream(ctx, pages, opts, out2); err != nil {
			errCh <- fmt.Errorf("doc2 render failed: %w", err)
		}
		close(out2)
	}()

	// Pairing and Hashing Pipeline
	// We need to match pages by their index in the 'pages' slice.
	type pair struct {
		img1 image.Image
		img2 image.Image
	}
	pairs := make(map[int]*pair)
	var pairsMu sync.Mutex

	// Bounded channels for backpressure
	hashJobs := make(chan int, bufferSize)
	var wgReceivers sync.WaitGroup
	wgReceivers.Add(2)

	// Channel for per-page render errors (bounded)
	pageErrCh := make(chan error, bufferSize*2)

	// Receiver for Doc 1
	go func() {
		defer wgReceivers.Done()
		for res := range out1 {
			if res.Err != nil {
				pageErrCh <- fmt.Errorf("doc1 page %d: %w", pages[res.Index], res.Err)
				continue
			}
			pairsMu.Lock()
			if p, ok := pairs[res.Index]; ok {
				p.img1 = res.Image
				if p.img2 != nil {
					hashJobs <- res.Index
				}
			} else {
				pairs[res.Index] = &pair{img1: res.Image}
			}
			pairsMu.Unlock()
		}
	}()

	// Receiver for Doc 2
	go func() {
		defer wgReceivers.Done()
		for res := range out2 {
			if res.Err != nil {
				pageErrCh <- fmt.Errorf("doc2 page %d: %w", pages[res.Index], res.Err)
				continue
			}
			pairsMu.Lock()
			if p, ok := pairs[res.Index]; ok {
				p.img2 = res.Image
				if p.img1 != nil {
					hashJobs <- res.Index
				}
			} else {
				pairs[res.Index] = &pair{img2: res.Image}
			}
			pairsMu.Unlock()
		}
	}()

	// Hashing Workers (use memory-adjusted worker count)
	var wgHash sync.WaitGroup
	processedChan := make(chan int, bufferSize)

	for i := 0; i < effectiveWorkers; i++ {
		wgHash.Add(1)
		go func() {
			defer wgHash.Done()
			for idx := range hashJobs {
				pairsMu.Lock()
				p := pairs[idx]
				img1, img2 := p.img1, p.img2
				delete(pairs, idx)
				pairsMu.Unlock()

				pageNum := pages[idx]
				var dHash1, dHash2, pHash1, pHash2 uint64
				var dDist, pDist int

				switch c.opts.HashType {
				case HashDHash:
					dHash1, dHash2, dDist = visual.ComputeDHashPair(img1, img2)
				case HashPHash:
					pHash1, pHash2, pDist = visual.ComputePHashPair(img1, img2)
				default:
					dHash1, pHash1 = visual.ComputeBothHashes(img1)
					dHash2, pHash2 = visual.ComputeBothHashes(img2)
					dDist = visual.HammingDistance(dHash1, dHash2)
					pDist = visual.HammingDistance(pHash1, pHash2)
				}

				var similarity float64
				switch c.opts.HashType {
				case HashDHash:
					similarity = visual.Similarity(dDist)
				case HashPHash:
					similarity = visual.Similarity(pDist)
				default:
					similarity = visual.CombinedSimilarity(dHash1, dHash2, pHash1, pHash2)
				}

				results[idx] = PageResult{
					Page:          pageNum + 1,
					Similarity:    similarity,
					DHashDistance: dDist,
					PHashDistance: pDist,
					DHash1:        dHash1,
					DHash2:        dHash2,
					PHash1:        pHash1,
					PHash2:        pHash2,
					IsDifferent:   similarity < c.opts.SimilarityThreshold,
				}
				processedChan <- idx
			}
		}()
	}

	// Close hashJobs once all pairs are sent
	go func() {
		wgReceivers.Wait()
		close(hashJobs)
	}()

	// Wait for rendering to complete, collecting stream-level errors
	var renderErrors []error
	count := 0
	for count < len(pages) {
		select {
		case err := <-errCh:
			if err != nil {
				renderErrors = append(renderErrors, err)
			}
			// Continue processing - other pages may still succeed
		case <-processedChan:
			count++
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Wait for remaining workers to finish
	wgHash.Wait()

	// Wait for receivers to complete and close pageErrCh
	wgReceivers.Wait()

	// Collect any per-page render errors
	close(pageErrCh)
	for err := range pageErrCh {
		renderErrors = append(renderErrors, err)
	}

	// Report aggregated render errors if any
	if len(renderErrors) > 0 {
		if len(renderErrors) == 1 {
			return nil, renderErrors[0]
		}
		errMsg := fmt.Sprintf("%d render errors occurred:", len(renderErrors))
		for i, err := range renderErrors {
			errMsg += fmt.Sprintf("\n  [%d] %v", i+1, err)
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return results, nil
}

func (c *comparer) reportProgress(stage string, current, total int, pct float64) {
	if c.opts.ProgressCallback != nil {
		c.opts.ProgressCallback(Progress{
			Stage:       stage,
			CurrentPage: current,
			TotalPages:  total,
			Percent:     pct,
		})
	}
}

func (c *comparer) Close() error {
	if c.renderer != nil {
		return c.renderer.Close()
	}
	return nil
}

// adjustWorkersForMemory reduces worker count if memory limit would be exceeded.
// Each worker renders pages from both documents concurrently, so we need memory
// for 2 pages per worker at the given DPI.
func (c *comparer) adjustWorkersForMemory(doc render.Document) int {
	workers := c.opts.Workers
	maxMemBytes := int64(c.opts.MaxMemoryMB) * 1024 * 1024

	// Get page info to estimate memory per page
	pageInfo, err := doc.PageInfo(0)
	if err != nil {
		// Can't estimate, use configured workers
		return workers
	}

	// Estimate memory per page (4 bytes per pixel for RGBA)
	memPerPage := render.EstimateMemoryUsage(pageInfo.WidthPts, pageInfo.HeightPts, c.opts.DPI)

	// Each worker needs memory for 2 pages (one from each document)
	memPerWorker := memPerPage * 2

	// Calculate max workers based on memory
	if memPerWorker > 0 {
		maxWorkers := int(maxMemBytes / memPerWorker)
		if maxWorkers < 1 {
			maxWorkers = 1 // Always allow at least 1 worker
		}
		if workers > maxWorkers {
			workers = maxWorkers
		}
	}

	return workers
}

// strategicSample returns page indices for strategic sampling.
// Formula: min(sqrt(n) + 2, n) pages, including first, last, middle, and distributed samples.
func strategicSample(pageCount int) []int {
	if pageCount <= 5 {
		return allPages(pageCount)
	}

	// Calculate sample size: sqrt(n) + 2, but at least 3 and at most pageCount
	sampleSize := int(math.Sqrt(float64(pageCount))) + 2
	if sampleSize < 3 {
		sampleSize = 3
	}
	if sampleSize > pageCount {
		sampleSize = pageCount
	}

	pages := make(map[int]bool)

	// Always include first and last
	pages[0] = true
	pages[pageCount-1] = true

	// Include middle
	pages[pageCount/2] = true

	// Distribute remaining samples evenly
	remaining := sampleSize - len(pages)
	if remaining > 0 {
		step := float64(pageCount-1) / float64(remaining+1)
		for i := 1; i <= remaining; i++ {
			idx := int(step * float64(i))
			pages[idx] = true
		}
	}

	// Convert to sorted slice
	result := make([]int, 0, len(pages))
	for i := 0; i < pageCount; i++ {
		if pages[i] {
			result = append(result, i)
		}
	}

	return result
}

// allPages returns all page indices
func allPages(count int) []int {
	pages := make([]int, count)
	for i := 0; i < count; i++ {
		pages[i] = i
	}
	return pages
}

// averageSimilarity calculates the average similarity from page results
func averageSimilarity(results []PageResult) float64 {
	if len(results) == 0 {
		return 0.0
	}
	var sum float64
	for _, r := range results {
		sum += r.Similarity
	}
	return sum / float64(len(results))
}

// hashTypeName returns a string name for the hash type
func hashTypeName(ht HashType) string {
	switch ht {
	case HashDHash:
		return "dhash"
	case HashPHash:
		return "phash"
	default:
		return "both"
	}
}
