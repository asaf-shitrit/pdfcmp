package render

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"

	"github.com/asaf-shitrit/pdfcmp/pkg/bytewise"
	"github.com/asaf-shitrit/pdfcmp/pkg/content"
)

// pdfiumRenderer implements Renderer using PDFium WebAssembly
type pdfiumRenderer struct {
	pool    pdfium.Pool
	workers int
}

// pdfiumDocument implements Document
type pdfiumDocument struct {
	renderer  *pdfiumRenderer
	instance  pdfium.Pdfium
	doc       *responses.OpenDocument
	pageCount int
	path      string
	password  string

	ocOnce sync.Once // guards hasOC
	hasOC  bool      // the document has optional content (see hasOptionalContent)
}

// Pool size limits
const (
	MinPoolSize = 4  // Minimum number of WebAssembly instances
	MaxPoolSize = 16 // Maximum number of WebAssembly instances
)

// initPool is a singleton pool for WebAssembly instances
var (
	globalPool     pdfium.Pool
	globalPoolOnce sync.Once
	globalPoolErr  error
	poolMu         sync.Mutex
	poolSize       int
)

// openTimeout is how long Open waits for a free PDFium instance.
const openTimeout = 30 * time.Second

func getPool(maxWorkers int) (pdfium.Pool, error) {
	globalPoolOnce.Do(func() {
		// Size pool based on workers needed
		size := min(max(maxWorkers, MinPoolSize), MaxPoolSize)
		globalPool, globalPoolErr = webassembly.Init(webassembly.Config{
			MinIdle:       1,
			MaxIdle:       size,
			MaxTotal:      size,
			RuntimeConfig: runtimeConfig(),
		})
		if globalPoolErr == nil {
			poolMu.Lock()
			poolSize = size
			poolMu.Unlock()
		}
	})
	return globalPool, globalPoolErr
}

// cacheDir returns the directory for the compiled-wasm cache, or "" when
// none can be determined.
func cacheDir() string {
	if dir := os.Getenv("PDFCMP_CACHE_DIR"); dir != "" {
		return dir
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "pdfcmp", "wazero")
}

// PoolSize returns how many PDFium instances the shared pool holds, or 0
// before the first renderer is created. Each open Document holds one
// instance for as long as it is open.
func PoolSize() int {
	poolMu.Lock()
	defer poolMu.Unlock()
	return poolSize
}

// runtimeConfig builds the wazero config. pdfium.wasm needs the WebAssembly
// 2.0 features and exception handling, which go-pdfium only enables when it
// builds the config itself. It uses an on-disk compilation cache so
// pdfium.wasm is compiled once, not on every process start. Any cache
// failure falls back to an uncached runtime.
func runtimeConfig() wazero.RuntimeConfig {
	cfg := wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling)
	dir := cacheDir()
	if dir == "" {
		return cfg
	}
	cache, err := wazero.NewCompilationCacheWithDir(dir)
	if err != nil {
		return cfg
	}
	return cfg.WithCompilationCache(cache)
}

// NewPdfiumRenderer creates a new PDFium-based renderer using WebAssembly.
// Call Close() when done to release resources.
func NewPdfiumRenderer() (Renderer, error) {
	return NewPdfiumRendererWithPool(runtime.NumCPU())
}

// NewPdfiumRendererWithPool creates a renderer with a worker pool.
// Workers determines how many parallel page renders can occur.
func NewPdfiumRendererWithPool(workers int) (Renderer, error) {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	pool, err := getPool(workers)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize pdfium pool: %w", err)
	}

	return &pdfiumRenderer{
		pool:    pool,
		workers: workers,
	}, nil
}

func (r *pdfiumRenderer) Open(path string) (Document, error) {
	return r.OpenWithPassword(path, "")
}

func (r *pdfiumRenderer) OpenWithPassword(path string, password string) (Document, error) {
	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	defer cancel()
	return r.OpenContext(ctx, path, password)
}

// OpenContext opens a PDF like OpenWithPassword, waiting for a free PDFium
// instance until ctx is done instead of for a fixed time.
func (r *pdfiumRenderer) OpenContext(ctx context.Context, path, password string) (Document, error) {
	instance, err := r.pool.GetInstanceWithContext(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return nil, fmt.Errorf("failed to get pdfium instance: %w", err)
	}

	req := &requests.OpenDocument{
		FilePath: &path,
	}
	if password != "" {
		req.Password = &password
	}

	doc, err := instance.OpenDocument(req)
	if err != nil {
		_ = instance.Close()
		return nil, fmt.Errorf("failed to open PDF %s: %w", path, err)
	}

	// Get page count
	pageCountResp, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: doc.Document,
	})
	if err != nil {
		_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
		_ = instance.Close()
		return nil, fmt.Errorf("failed to get page count: %w", err)
	}

	return &pdfiumDocument{
		renderer:  r,
		instance:  instance,
		doc:       doc,
		pageCount: pageCountResp.PageCount,
		path:      path,
		password:  password,
	}, nil
}

func (r *pdfiumRenderer) Close() error {
	return nil
}

func (d *pdfiumDocument) PageCount() int {
	return d.pageCount
}

func (d *pdfiumDocument) PageInfo(pageNum int) (PageInfo, error) {
	if pageNum < 0 || pageNum >= d.pageCount {
		return PageInfo{}, fmt.Errorf("page %d out of range [0, %d)", pageNum, d.pageCount)
	}

	sizeResp, err := d.instance.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{
		Document: d.doc.Document,
		Index:    pageNum,
	})
	if err != nil {
		return PageInfo{}, fmt.Errorf("failed to get page size: %w", err)
	}

	// Get object count
	objCountResp, err := d.instance.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageNum,
			},
		},
	})
	objectCount := 0
	if err == nil {
		objectCount = objCountResp.Count
	}

	// Get text count and hash
	textChars := 0
	var textHash, textBag uint64
	var text string
	textPageResp, err := d.instance.FPDFText_LoadPage(&requests.FPDFText_LoadPage{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageNum,
			},
		},
	})
	if err == nil {
		charCountResp, err := d.instance.FPDFText_CountChars(&requests.FPDFText_CountChars{
			TextPage: textPageResp.TextPage,
		})
		if err == nil {
			textChars = charCountResp.Count
			// If there's text, get it and hash it
			if textChars > 0 {
				textResp, err := d.instance.FPDFText_GetText(&requests.FPDFText_GetText{
					TextPage:   textPageResp.TextPage,
					StartIndex: 0,
					Count:      textChars,
				})
				if err == nil && textResp.Text != "" {
					text = content.Normalize(textResp.Text)
					if text != "" {
						textHash = bytewise.HashBytes([]byte(text))
						textBag = bytewise.HashBytes([]byte(content.Bag(text)))
					}
				}
			}
		}
		_, _ = d.instance.FPDFText_ClosePage(&requests.FPDFText_ClosePage{
			TextPage: textPageResp.TextPage,
		})
	}

	return PageInfo{
		Number:      pageNum,
		WidthPts:    sizeResp.Width,
		HeightPts:   sizeResp.Height,
		WidthPx:     int(sizeResp.Width * float64(DPIStandard) / 72.0),
		HeightPx:    int(sizeResp.Height * float64(DPIStandard) / 72.0),
		TextChars:   textChars,
		ObjectCount: objectCount,
		TextHash:    textHash,
		TextBag:     textBag,
		Text:        text,
	}, nil
}

func (d *pdfiumDocument) RenderPage(ctx context.Context, pageNum int, opts Options) (image.Image, error) {
	if pageNum < 0 || pageNum >= d.pageCount {
		return nil, fmt.Errorf("page %d out of range [0, %d)", pageNum, d.pageCount)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return renderPage(d.instance, d.doc, pageNum, renderDPI(opts))
}

// renderDPI returns the DPI opts asks for, or DPIStandard when it asks for
// none.
func renderDPI(opts Options) int {
	if opts.DPI <= 0 {
		return DPIStandard
	}
	return opts.DPI
}

// renderPage renders one page of doc, which instance has open. PDFium's
// WebAssembly build renders into its own memory and hands back a view of it,
// which stays allocated until it is cleaned up; the image is copied out so
// that it can be.
func renderPage(instance pdfium.Pdfium, doc *responses.OpenDocument, pageNum, dpi int) (image.Image, error) {
	resp, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: doc.Document,
				Index:    pageNum,
			},
		},
		DPI: dpi,
		// Draw annotations and form fields, as viewers do: comments, stamps
		// and filled-in or comb fields are part of what a reader sees.
		RenderFlags: enums.FPDF_RENDER_FLAG_ANNOT,
		RenderForm:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render page %d: %w", pageNum, err)
	}
	defer resp.Cleanup()
	if resp.Result.RenderedImage == nil {
		return nil, fmt.Errorf("render returned nil image for page %d", pageNum)
	}
	return cloneImage(resp.Result.RenderedImage), nil
}

// cloneImage copies img into memory the Go runtime owns.
func cloneImage(img image.Image) image.Image {
	switch m := img.(type) {
	case *image.RGBA:
		return &image.RGBA{Pix: slices.Clone(m.Pix), Stride: m.Stride, Rect: m.Rect}
	case *image.Gray:
		return &image.Gray{Pix: slices.Clone(m.Pix), Stride: m.Stride, Rect: m.Rect}
	}
	b := img.Bounds()
	c := image.NewRGBA(b)
	draw.Draw(c, b, img, b.Min, draw.Src)
	return c
}

// RenderPages renders multiple pages - uses parallel rendering when beneficial
func (d *pdfiumDocument) RenderPages(ctx context.Context, pageNums []int, opts Options) ([]image.Image, error) {
	if len(pageNums) == 0 {
		return nil, nil
	}

	images := make([]image.Image, len(pageNums))
	out := make(chan RenderedPage, len(pageNums))

	// On the first error the stream is stopped and waited for, so it never
	// renders on after the caller closes the document.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.RenderPagesStream(ctx, pageNums, opts, out)
		close(out)
	}()

	var firstErr error
	for res := range out {
		if res.Err != nil && firstErr == nil {
			firstErr = res.Err
			cancel()
		}
		images[res.Index] = res.Image
	}
	if err := <-errCh; err != nil && firstErr == nil {
		firstErr = err
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return images, nil
}

// RenderPagesStream renders pageNums on up to the renderer's worker count of
// PDFium instances and sends each page to out as it is ready. The first
// worker renders on the document's own instance, so the stream always makes
// progress; the others borrow an instance from the shared pool only while
// one is free and pages remain, so a busy pool slows a stream down instead
// of failing it. A page that fails to render is sent with its error, and the
// stream carries on with the rest.
func (d *pdfiumDocument) RenderPagesStream(ctx context.Context, pageNums []int, opts Options, out chan<- RenderedPage) error {
	if len(pageNums) == 0 {
		return nil
	}
	numWorkers := d.renderer.workers
	if opts.Workers > 0 {
		numWorkers = opts.Workers
	}
	numWorkers = min(max(numWorkers, 1), len(pageNums), max(PoolSize(), 1))
	dpi := renderDPI(opts)

	type job struct{ index, pageNum int }
	jobs := make(chan job, len(pageNums))
	for i, pageNum := range pageNums {
		jobs <- job{index: i, pageNum: pageNum}
	}
	close(jobs)

	render := func(instance pdfium.Pdfium, doc *responses.OpenDocument) {
		for j := range jobs {
			if err := ctx.Err(); err != nil {
				out <- RenderedPage{Index: j.index, Err: err}
				continue
			}
			img, err := renderPage(instance, doc, j.pageNum, dpi)
			out <- RenderedPage{Index: j.index, Image: img, Err: err}
		}
	}

	// Helpers stop waiting for an instance once the first worker has taken
	// the last page: by then there is nothing left for them to do.
	acquireCtx, stopAcquiring := context.WithCancel(ctx)
	defer stopAcquiring()

	var wg sync.WaitGroup
	for range numWorkers - 1 {
		wg.Go(func() {
			instance, doc, ok := d.openHelper(acquireCtx)
			if !ok {
				return
			}
			defer func() {
				_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
				_ = instance.Close()
			}()
			render(instance, doc)
		})
	}
	render(d.instance, d.doc)
	stopAcquiring()
	wg.Wait()
	return nil
}

// openHelper borrows a pool instance and opens the document on it, waiting
// until ctx is done for one to be free. It reports false when it got none;
// the stream's first worker renders every page the helpers do not.
func (d *pdfiumDocument) openHelper(ctx context.Context) (pdfium.Pdfium, *responses.OpenDocument, bool) {
	instance, err := d.renderer.pool.GetInstanceWithContext(ctx)
	if err != nil {
		return nil, nil, false
	}
	req := &requests.OpenDocument{FilePath: &d.path}
	if d.password != "" {
		req.Password = &d.password
	}
	doc, err := instance.OpenDocument(req)
	if err != nil {
		_ = instance.Close()
		return nil, nil, false
	}
	return instance, doc, true
}

func (d *pdfiumDocument) Close() error {
	_, err := d.instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
		Document: d.doc.Document,
	})
	return errors.Join(err, d.instance.Close())
}
