package compare

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/asaf-shitrit/pdfcmp/pkg/bytewise"
	"github.com/asaf-shitrit/pdfcmp/pkg/content"
	"github.com/asaf-shitrit/pdfcmp/pkg/render"
	"github.com/asaf-shitrit/pdfcmp/pkg/visual"
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

	rendererOnce sync.Once
	rendererErr  error
}

// New creates a new Comparer with the given options
func New(opts ...Option) (Comparer, error) {
	options := DefaultOptions()
	options.Apply(opts...)

	// Validate options before proceeding
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	return &comparer{opts: options}, nil
}

// getRenderer starts PDFium on first use, so comparisons that never reach
// the visual stages skip its startup cost.
func (c *comparer) getRenderer() (render.Renderer, error) {
	c.rendererOnce.Do(func() {
		if c.renderer != nil {
			return
		}
		r, err := render.NewPdfiumRenderer()
		if err != nil {
			c.rendererErr = fmt.Errorf("failed to create renderer: %w", err)
			return
		}
		c.renderer = r
	})
	return c.renderer, c.rendererErr
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
		textMode:         c.opts.TextChange,
		tiles:            c.opts.Tiles,
	}
	if err := c.runStages(ctx, result, pdf1, pdf2); err != nil {
		return nil, err
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

// runStages fills in result one stage at a time, cheapest first, and stops at
// the first stage that decides the comparison.
func (c *comparer) runStages(ctx context.Context, result *Result, pdf1, pdf2 string) error {
	if sameFile(pdf1, pdf2) {
		c.opts.Logger.Debug("Self-comparison detected, returning identical")
		result.setIdentical("skipped (same file)")
		return nil
	}
	if err := result.statFiles(pdf1, pdf2); err != nil {
		return err
	}
	if c.usesBytes() {
		done, err := c.byteStage(result, pdf1, pdf2)
		if err != nil || done {
			return err
		}
	}
	return c.visualStages(ctx, result, pdf1, pdf2)
}

// sameFile reports whether both paths name the same file.
func sameFile(pdf1, pdf2 string) bool {
	abs1, err1 := filepath.Abs(pdf1)
	abs2, err2 := filepath.Abs(pdf2)
	return err1 == nil && err2 == nil && abs1 == abs2
}

func (c *comparer) usesBytes() bool {
	switch c.opts.Mode {
	case ModeAuto, ModeByte, ModeFull:
		return true
	}
	return false
}

func (c *comparer) usesThumbnails() bool {
	return c.opts.Sampling == SamplingAuto || c.opts.Sampling == SamplingThumbnailOnly
}

// byteStage hashes both files. It decides the comparison when the files are
// byte-identical, or when only a byte comparison was asked for.
func (c *comparer) byteStage(result *Result, pdf1, pdf2 string) (bool, error) {
	log := c.opts.Logger
	result.ComparisonLayers = append(result.ComparisonLayers, "bytewise")
	c.reportProgress("Byte comparison", 0, 1, 0.1)

	byteResult, err := bytewise.Compare(pdf1, pdf2)
	if err != nil {
		return false, fmt.Errorf("byte comparison failed: %w", err)
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

	if byteResult.Identical {
		log.Info("Files are byte-identical, skipping visual comparison")
		result.setIdentical("skipped (byte identical)")
		return true, nil
	}
	if c.opts.Mode == ModeByte {
		result.SimilarityScore = 0.0
		return true, nil
	}
	return false, nil
}

// visualStages opens both documents and compares their pages: thumbnails
// first, then a sample, then every page when the sample cannot decide.
func (c *comparer) visualStages(ctx context.Context, result *Result, pdf1, pdf2 string) error {
	renderer, err := c.getRenderer()
	if err != nil {
		return err
	}
	doc1, err := renderer.Open(pdf1)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", pdf1, err)
	}
	defer func() { _ = doc1.Close() }()

	doc2, err := renderer.Open(pdf2)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", pdf2, err)
	}
	defer func() { _ = doc2.Close() }()

	if c.pageCountStage(result, doc1, doc2) {
		return nil
	}
	if doc1.PageCount() != doc2.PageCount() {
		return c.alignStage(ctx, result, doc1, doc2)
	}
	if c.usesThumbnails() {
		done, err := c.thumbnailStage(ctx, result, doc1, doc2)
		if err != nil || done {
			return err
		}
	}
	return c.sampleStage(ctx, result, doc1, doc2)
}

// pageCountStage records both page counts. It decides the comparison when
// both documents are empty or only one is. Documents whose page counts differ
// are left to alignStage, which pairs their pages by content.
func (c *comparer) pageCountStage(result *Result, doc1, doc2 render.Document) bool {
	n1, n2 := doc1.PageCount(), doc2.PageCount()
	result.Metadata.Pages1, result.Metadata.Pages2 = n1, n2

	switch {
	case n1 == 0 && n2 == 0:
		c.opts.Logger.Debug("Both PDFs have 0 pages, treating as identical")
		result.setIdentical("empty documents")
		return true
	case n1 == 0 || n2 == 0:
		c.opts.Logger.Debug("One PDF has 0 pages", F("pages1", n1), F("pages2", n2))
		result.setMismatch("empty document mismatch")
		return true
	}
	return false
}

// thumbnailStage compares the first and last pages at thumbnail DPI. It
// decides the comparison when a thumbnail is very different, or when only
// thumbnails were asked for.
func (c *comparer) thumbnailStage(ctx context.Context, result *Result, doc1, doc2 render.Document) (bool, error) {
	result.ComparisonLayers = append(result.ComparisonLayers, "thumbnail")
	c.reportProgress("Thumbnail check", 0, 2, 0.2)

	pages := []int{0}
	if n := doc1.PageCount(); n > 1 {
		pages = append(pages, n-1)
	}
	results, err := c.comparePages(ctx, doc1, doc2, pages, render.Options{DPI: render.DPIThumbnail})
	if err != nil {
		return false, fmt.Errorf("thumbnail comparison failed: %w", err)
	}

	switch {
	case worstSimilarity(results) < ThumbnailEarlyExitThresh:
		result.setPages(results, "thumbnail (early exit)", c.opts.HashType, render.DPIThumbnail)
		return true, nil
	case c.opts.Sampling == SamplingThumbnailOnly:
		result.setPages(results, "thumbnail only", c.opts.HashType, render.DPIThumbnail)
		return true, nil
	}
	return false, nil
}

// sampleStage compares a sample of pages, then every page when needsFullScan
// says the sample cannot decide.
func (c *comparer) sampleStage(ctx context.Context, result *Result, doc1, doc2 render.Document) error {
	pageCount := doc1.PageCount()
	pages, strategy := c.samplePages(pageCount)
	renderOpts := render.Options{DPI: c.opts.DPI}

	result.ComparisonLayers = append(result.ComparisonLayers, "sample")
	c.reportProgress("Sampling pages", 0, len(pages), 0.4)
	results, err := c.comparePages(ctx, doc1, doc2, pages, renderOpts)
	if err != nil {
		return fmt.Errorf("sample comparison failed: %w", err)
	}

	if !c.needsFullScan(results, pageCount) {
		result.setPages(results, strategy, c.opts.HashType, c.opts.DPI)
		return nil
	}

	result.ComparisonLayers = append(result.ComparisonLayers, "full")
	c.reportProgress("Full comparison", 0, pageCount, 0.6)
	results, err = c.comparePages(ctx, doc1, doc2, allPages(pageCount), renderOpts)
	if err != nil {
		return fmt.Errorf("full comparison failed: %w", err)
	}
	result.setPages(results, "full", c.opts.HashType, c.opts.DPI)
	return nil
}

// needsFullScan reports whether a sample leaves the comparison open: sampling
// is automatic, it left pages out, and every sampled page is similar. One
// sampled page below the threshold already makes the documents different,
// and so does one whose text changed, unless TextChangeScored is set, or one
// whose tile changed, when tiles are on.
func (c *comparer) needsFullScan(sample []PageResult, pageCount int) bool {
	return c.opts.Sampling == SamplingAuto && len(sample) < pageCount &&
		worstSimilarity(sample) >= c.opts.SimilarityThreshold &&
		!(c.opts.TextChange == TextChangeFails && anyTextChanged(sample)) &&
		!(c.opts.Tiles && anyTileChanged(sample))
}

// anyTileChanged reports whether the worst tile of any page changed.
func anyTileChanged(pages []PageResult) bool {
	for _, p := range pages {
		if p.TileChanged {
			return true
		}
	}
	return false
}

// anyTextChanged reports whether the text of any page changed.
func anyTextChanged(pages []PageResult) bool {
	for _, p := range pages {
		if p.TextChanged {
			return true
		}
	}
	return false
}

// samplePages returns the pages the sample stage compares and the name of
// the strategy that chose them.
func (c *comparer) samplePages(pageCount int) ([]int, string) {
	switch c.opts.Sampling {
	case SamplingStrategic, SamplingAuto:
		return strategicSample(pageCount), "strategic"
	default:
		return allPages(pageCount), "all"
	}
}

// comparePages compares specific pages between two documents. Pages whose
// text decides them are scored without rendering; the rest are rendered in
// both documents and scored as soon as both renders of a page are ready.
func (c *comparer) comparePages(ctx context.Context, doc1, doc2 render.Document, pages []int, opts render.Options) ([]PageResult, error) {
	pairs := make([]pagePair, len(pages))
	for i, p := range pages {
		pairs[i] = pagePair{p, p}
	}
	return c.comparePairs(ctx, doc1, doc2, pairs, opts)
}

// pagePair is a page of the first document and the page of the second that
// it is compared with, both 0-indexed.
type pagePair struct{ p1, p2 int }

// comparePairs is comparePages for pages that may sit at different indexes
// in the two documents. Each result's Page is the pair's first-document page.
func (c *comparer) comparePairs(ctx context.Context, doc1, doc2 render.Document, pairs []pagePair, opts render.Options) ([]PageResult, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	plan := c.planPages(doc1, doc2, pairs)
	if len(plan.render) == 0 {
		return plan.results, nil
	}
	c.reportProgress("Pipelined processing", len(pairs)-len(plan.render), len(pairs), 0.3)

	nums1 := make([]int, len(plan.render))
	nums2 := make([]int, len(plan.render))
	for j, i := range plan.render {
		nums1[j], nums2[j] = pairs[i].p1, pairs[i].p2
	}
	err := c.renderPairs(ctx, doc1, doc2, nums1, nums2, opts, func(j int, img1, img2 image.Image) {
		i := plan.render[j]
		plan.results[i] = c.scorePage(pairs[i].p1, img1, img2, plan.infos[i], plan.textSims[i])
	})
	if err != nil {
		return nil, err
	}
	return plan.results, nil
}

// pagePlan is what comparePages knows about each page before rendering.
// results, infos and textSims are indexed like comparePairs' pairs.
type pagePlan struct {
	results  []PageResult   // filled in now for pages that text decides
	render   []int          // indexes of the pages still to render
	infos    []pageInfoPair // page info of the pages to render, text dropped
	textSims []*float64     // text similarity of the pages to render
}

// planPages reads every page's info before anything is rendered, which is
// about 16x faster than rendering. A page that text decides is scored now;
// every other page is scheduled for rendering, with its text similarity
// scored now so that rendering workers never call back into the PDF engine.
func (c *comparer) planPages(doc1, doc2 render.Document, pairs []pagePair) pagePlan {
	plan := pagePlan{
		results:  make([]PageResult, len(pairs)),
		infos:    make([]pageInfoPair, len(pairs)),
		textSims: make([]*float64, len(pairs)),
	}
	for i, pr := range pairs {
		info1, err1 := doc1.PageInfo(pr.p1)
		info2, err2 := doc2.PageInfo(pr.p2)
		ok := err1 == nil && err2 == nil

		if ok && textDecides(info1, info2) && contentMatches(doc1, doc2, pr) {
			plan.results[i] = PageResult{Page: pr.p1 + 1, Similarity: 1.0}
			continue
		}
		if ok {
			plan.textSims[i] = c.textSimilarity(info1, info2)
		}
		info1.Text, info2.Text = "", "" // scored; keep only the metadata
		plan.infos[i] = pageInfoPair{info1: info1, info2: info2, ok: ok}
		plan.render = append(plan.render, i)
	}
	return plan
}

// contentMatches reports whether the pair's two pages have the same content
// hash. A hash error counts as no match, so the page is rendered.
func contentMatches(doc1, doc2 render.Document, pr pagePair) bool {
	h1, err1 := doc1.ContentHash(pr.p1)
	h2, err2 := doc2.ContentHash(pr.p2)
	return err1 == nil && err2 == nil && h1 == h2
}

// textDecides reports whether two pages are candidates to be identical
// without rendering: both have text, it hashes the same, and so does their
// object count. It is only a candidate test; planPages also requires equal
// content hashes, which catch graphics redrawn beside unchanged text.
func textDecides(info1, info2 render.PageInfo) bool {
	return info1.TextHash != 0 && info1.TextHash == info2.TextHash && info1.ObjectCount == info2.ObjectCount
}

// renderPairs renders nums1 in the first document and nums2 in the second,
// page j of one against page j of the other, and calls score with each
// pair's two renders as soon as both are ready, from up to the
// memory-adjusted worker count of goroutines at once. j is the pair's index. A page that fails to render in either document is not scored;
// the errors are returned together once every page is done. When one
// document's stream fails outright, the other stops rendering too.
//
// Memory is not bounded: a render waits in the pairer until the other
// document's render of the same page arrives, so a document that renders
// much faster can hold most of its pages there.
func (c *comparer) renderPairs(ctx context.Context, doc1, doc2 render.Document, nums1, nums2 []int, opts render.Options, score func(j int, img1, img2 image.Image)) error {
	workers := c.adjustWorkersForMemory(doc1)
	// Twice the workers lets rendering run ahead of scoring.
	bufferSize := min(workers*2, len(nums1))
	renderCtx, stopRendering := context.WithCancel(ctx)
	defer stopRendering()
	pairer := &pagePairer{renders: map[int]*[2]image.Image{}, jobs: make(chan int, bufferSize)}

	// Page errors are collected while the pipeline runs, so a page that fails
	// to render can never block a receiver.
	pageErrCh := make(chan error)
	var pageErrs []error
	collected := make(chan struct{})
	go func() {
		for err := range pageErrCh {
			pageErrs = append(pageErrs, err)
		}
		close(collected)
	}()

	streamErrCh := make(chan error, 2)
	var receivers sync.WaitGroup
	for d, doc := range []render.Document{doc1, doc2} {
		pageNums := [][]int{nums1, nums2}[d]
		out := make(chan render.RenderedPage, bufferSize)
		go func() {
			if err := doc.RenderPagesStream(renderCtx, pageNums, opts, out); err != nil {
				streamErrCh <- fmt.Errorf("doc%d render failed: %w", d+1, err)
				stopRendering() // the other document's pages can no longer be scored
			}
			close(out)
		}()
		receivers.Add(1)
		go func() {
			defer receivers.Done()
			pairer.receive(d, out, pageNums, pageErrCh)
		}()
	}

	var scorers sync.WaitGroup
	for range workers {
		scorers.Add(1)
		go func() {
			defer scorers.Done()
			for j := range pairer.jobs {
				img1, img2 := pairer.take(j)
				score(j, img1, img2)
			}
		}()
	}

	// Each receiver finishes when its stream closes. Once both have, every
	// page has been queued or has failed to render, so closing jobs lets the
	// scorers drain and stop.
	receivers.Wait()
	close(pairer.jobs)
	close(pageErrCh)
	scorers.Wait()
	<-collected

	// The streams send to streamErrCh before closing their output, and the
	// receivers have seen both outputs close, so no send remains.
	close(streamErrCh)
	if err := ctx.Err(); err != nil {
		return err
	}
	var errs []error
	for err := range streamErrCh {
		errs = append(errs, err)
	}
	if renderCtx.Err() != nil {
		// The caller did not cancel, so a failed stream did: the other
		// document's cancelled pages are not errors of their own.
		pageErrs = slices.DeleteFunc(pageErrs, func(err error) bool { return errors.Is(err, context.Canceled) })
	}
	return joinRenderErrors(append(errs, pageErrs...))
}

// pagePairer matches the two renders of each page and queues the page for
// scoring once both have arrived. Pages are keyed by their index in the
// rendered page list.
type pagePairer struct {
	mu      sync.Mutex
	renders map[int]*[2]image.Image
	jobs    chan int
}

// receive feeds document d's renders into the pairer until its stream
// closes, sending each failed page's error to errs.
func (p *pagePairer) receive(d int, out <-chan render.RenderedPage, pageNums []int, errs chan<- error) {
	for res := range out {
		if res.Err != nil {
			errs <- fmt.Errorf("doc%d page %d: %w", d+1, pageNums[res.Index], res.Err)
			continue
		}
		p.add(res.Index, d, res.Image)
	}
}

// add records document d's render of page j and queues the page when the
// other render is already there. It sends outside the lock: a scorer needs
// mu to take a job, so blocking on a full jobs channel while holding it
// would deadlock.
func (p *pagePairer) add(j, d int, img image.Image) {
	p.mu.Lock()
	pair, ok := p.renders[j]
	if !ok {
		pair = new([2]image.Image)
		p.renders[j] = pair
	}
	pair[d] = img
	ready := pair[0] != nil && pair[1] != nil
	p.mu.Unlock()

	if ready {
		p.jobs <- j
	}
}

// take removes and returns page j's two renders.
func (p *pagePairer) take(j int) (image.Image, image.Image) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pair := p.renders[j]
	delete(p.renders, j)
	return pair[0], pair[1]
}

// joinRenderErrors returns nil, the single error, or every error in one.
func joinRenderErrors(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	}
	return &renderErrors{errs: errs}
}

// renderErrors is several render errors returned as one; errors.Is and
// errors.As see each of them. It is used by pointer, as errors.Join's value
// is, so callers can compare it or use it as a map key.
type renderErrors struct {
	errs []error
}

func (e *renderErrors) Error() string {
	msg := fmt.Sprintf("%d render errors occurred:", len(e.errs))
	for i, err := range e.errs {
		msg += fmt.Sprintf("\n  [%d] %v", i+1, err)
	}
	return msg
}

func (e *renderErrors) Unwrap() []error { return e.errs }

// scorePage scores one page (0-indexed pageNum) from its two renders: a
// visual score, then the page's text folded in (see textAdjust).
func (c *comparer) scorePage(pageNum int, img1, img2 image.Image, info pageInfoPair, textSim *float64) PageResult {
	r := c.visualScore(img1, img2)
	r.Page = pageNum + 1
	r.Similarity = textAdjust(r.Similarity, info, textSim)
	r.TextSimilarity = textSim
	r.TextChanged = textChanged(info)
	if c.useTiles(info) {
		applyTiles(&r, img1, img2)
	}
	r.IsDifferent = r.Similarity < c.opts.SimilarityThreshold ||
		(r.TextChanged && c.opts.TextChange == TextChangeFails) ||
		r.TileChanged
	return r
}

// visualScore compares two renders. It checks colour histograms first, since
// pages whose colours differ this much need no hashing, then runs the
// configured perceptual hashes.
func (c *comparer) visualScore(img1, img2 image.Image) PageResult {
	histSim := visual.HistogramSimilarity(visual.ComputeHistogram(img1), visual.ComputeHistogram(img2))
	if histSim < 0.8 {
		return PageResult{Similarity: histSim * 0.5} // penalty for a colour mismatch
	}

	var r PageResult
	switch c.opts.HashType {
	case HashDHash:
		r.DHash1, r.DHash2, r.DHashDistance = visual.ComputeDHashPair(img1, img2)
		r.Similarity = visual.Similarity(r.DHashDistance)
	case HashPHash:
		r.PHash1, r.PHash2, r.PHashDistance = visual.ComputePHashPair(img1, img2)
		r.Similarity = visual.Similarity(r.PHashDistance)
	default:
		r = cascadeHashes(img1, img2)
	}
	return r
}

// useTiles reports whether a page is checked by its tiles. A page whose
// text changed is not: the text rule already judges it, and a tile cannot
// tell a reworded line from a stamp, so tiles would override a caller who
// asked for small text changes to pass (TextChangeScored).
func (c *comparer) useTiles(info pageInfoPair) bool {
	return c.opts.Tiles && !textChanged(info)
}

// applyTiles records the page's worst tile and whether it changed. Like a
// text change, a changed tile decides the verdict and leaves Similarity
// alone: the score says how much of the page changed, and a stamp is a small
// part of it, though the hashes cannot see it.
func applyTiles(r *PageResult, img1, img2 image.Image) {
	diff, tile := visual.TileDiff(img1, img2)
	if tile == nil {
		return
	}
	sim := visual.TileScore(diff)
	r.TileSimilarity, r.WorstTile = &sim, tile
	r.TileChanged = diff > visual.TileTolerance
}

// cascadeHashes runs dHash, then pHash only when dHash finds the pages at
// least 40% similar: below that the DCT is unlikely to salvage the match. A
// skipped pHash is left at 0.
func cascadeHashes(img1, img2 image.Image) PageResult {
	r := PageResult{DHash1: visual.DHash(img1), DHash2: visual.DHash(img2)}
	r.DHashDistance = visual.HammingDistance(r.DHash1, r.DHash2)
	if dSim := visual.Similarity(r.DHashDistance); dSim < 0.4 {
		r.Similarity = dSim
		return r
	}
	r.PHash1, r.PHash2 = visual.PHash(img1), visual.PHash(img2)
	r.PHashDistance = visual.HammingDistance(r.PHash1, r.PHash2)
	r.Similarity = visual.CombinedSimilarity(r.DHash1, r.DHash2, r.PHash1, r.PHash2)
	return r
}

// textAdjust folds a page's text into its visual similarity when both pages
// have text. Identical text lifts a close visual score to 0.99. Text that
// differs costs 5%, and its estimated similarity, when there is one, caps the
// score: perceptual hashes see where text sits, not what it says, so a page
// rewritten in the same layout scores as visually similar.
func textAdjust(similarity float64, info pageInfoPair, textSim *float64) float64 {
	if !info.ok || info.info1.TextHash == 0 || info.info2.TextHash == 0 {
		return similarity
	}
	if info.info1.TextHash == info.info2.TextHash {
		if similarity > 0.9 {
			return math.Max(similarity, 0.99)
		}
		return similarity
	}
	similarity *= 0.95
	if textSim != nil {
		similarity = math.Min(similarity, *textSim)
	}
	return similarity
}

// textChanged reports whether a page's text differs between the documents.
// A page with text against one without counts; two pages without text, such
// as scans, do not.
func textChanged(info pageInfoPair) bool {
	return info.ok && info.info1.TextHash != info.info2.TextHash
}

// setIdentical records documents found identical without comparing pages.
func (r *Result) setIdentical(strategy string) {
	r.Identical = true
	r.SimilarityScore = 1.0
	r.VisualComparison = VisualResult{
		OverallScore:     1.0,
		SamplingStrategy: strategy,
	}
}

// setMismatch records documents found different without comparing pages.
func (r *Result) setMismatch(strategy string) {
	r.SimilarityScore = 0.0
	r.VisualComparison = VisualResult{
		OverallScore:     0.0,
		SamplingStrategy: strategy,
	}
}

// setPages records the compared pages. The documents score as their least
// similar page; the visual result keeps the mean.
func (r *Result) setPages(pages []PageResult, strategy string, hashType HashType, dpi int) {
	r.SimilarityScore = worstSimilarity(pages)
	r.VisualComparison = VisualResult{
		OverallScore:     averageSimilarity(pages),
		HashType:         hashTypeName(hashType),
		PagesCompared:    len(pages),
		SamplingStrategy: strategy,
		DPI:              dpi,
	}
	r.PageResults = pages
	r.TextChanged = anyTextChanged(pages)
	r.TileChanged = anyTileChanged(pages)
}

// statFiles records both file sizes.
func (r *Result) statFiles(pdf1, pdf2 string) error {
	stat1, err := os.Stat(pdf1)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", pdf1, err)
	}
	stat2, err := os.Stat(pdf2)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", pdf2, err)
	}
	r.Metadata.Size1, r.Metadata.Size2 = stat1.Size(), stat2.Size()
	return nil
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

// strategicSample returns the pages to sample from a document: every page
// of a document of five pages or fewer, otherwise sqrt(n)+2 pages including
// the first, last and middle, with the rest spread evenly between. Spread
// pages that land on one already chosen make the sample smaller.
func strategicSample(pageCount int) []int {
	if pageCount <= 5 {
		return allPages(pageCount)
	}

	// For n > 5, sqrt(n)+2 is at least 4 and less than n, and first, last
	// and middle are three distinct pages, so at least one page is spread.
	sampleSize := int(math.Sqrt(float64(pageCount))) + 2
	pages := map[int]bool{0: true, pageCount - 1: true, pageCount / 2: true}
	remaining := sampleSize - len(pages)
	step := float64(pageCount-1) / float64(remaining+1)
	for i := 1; i <= remaining; i++ {
		pages[int(step*float64(i))] = true
	}

	result := make([]int, 0, len(pages))
	for i := range pageCount {
		if pages[i] {
			result = append(result, i)
		}
	}
	return result
}

func allPages(count int) []int {
	pages := make([]int, count)
	for i := 0; i < count; i++ {
		pages[i] = i
	}
	return pages
}

// pageInfoPair holds the page info of one page number in both documents.
type pageInfoPair struct {
	info1, info2 render.PageInfo
	ok           bool // both PageInfo calls succeeded
}

// textSimilarity estimates how much text two pages share. It returns nil when
// text hashing is off, when either page has no text, when both have too
// little to estimate (see content.MinShingles), or when the text is
// identical, so that the exact-hash path decides.
func (c *comparer) textSimilarity(info1, info2 render.PageInfo) *float64 {
	if c.opts.TextHash == TextHashOff || info1.TextHash == 0 || info2.TextHash == 0 || info1.TextHash == info2.TextHash {
		return nil
	}
	s, ok := content.Similarity(info1.Text, info2.Text)
	if !ok {
		return nil
	}
	return &s
}

// worstSimilarity returns the lowest similarity among page results: the
// document is only as similar as its least similar page.
func worstSimilarity(results []PageResult) float64 {
	if len(results) == 0 {
		return 1.0
	}
	worst := results[0].Similarity
	for _, r := range results[1:] {
		worst = math.Min(worst, r.Similarity)
	}
	return worst
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
