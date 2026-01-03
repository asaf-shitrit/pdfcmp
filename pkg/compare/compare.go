package compare

import (
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"sync"
	"time"

	"github.com/asafshitrit/similar-pdf/pkg/bytewise"
	"github.com/asafshitrit/similar-pdf/pkg/render"
	"github.com/asafshitrit/similar-pdf/pkg/visual"
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

	result := &Result{
		Metadata: Metadata{
			File1: pdf1,
			File2: pdf2,
		},
		ComparisonLayers: []string{},
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

		// Early exit if byte-identical
		if byteResult.Identical {
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
		if avgSimilarity < 0.5 {
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

	return result, nil
}

// comparePages compares specific pages between two documents using a pipelined approach.
// It renders and hashes pages concurrently, processing each page as soon as it's rendered
// in both documents.
func (c *comparer) comparePages(ctx context.Context, doc1, doc2 render.Document, pages []int, opts render.Options) ([]PageResult, error) {
	if len(pages) == 0 {
		return nil, nil
	}

	c.reportProgress("Pipelined processing", 0, len(pages), 0.3)

	// Result storage
	results := make([]PageResult, len(pages))
	
	// Channels for rendered pages
	out1 := make(chan render.RenderedPage, len(pages))
	out2 := make(chan render.RenderedPage, len(pages))

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
	
	hashJobs := make(chan int, len(pages))
	var wgReceivers sync.WaitGroup
	wgReceivers.Add(2)
	
	// Receiver for Doc 1
	go func() {
		defer wgReceivers.Done()
		for res := range out1 {
			if res.Err != nil {
				continue // Error will be handled by errCh
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

	// Hashing Workers
	var wgHash sync.WaitGroup
	processedChan := make(chan int, len(pages))
	
	for i := 0; i < c.opts.Workers; i++ {
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

	// Wait for rendering to complete or error
	count := 0
	for count < len(pages) {
		select {
		case err := <-errCh:
			return nil, err
		case <-processedChan:
			count++
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Wait for remaining workers to finish (though they should be done by now)
	wgHash.Wait()

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
