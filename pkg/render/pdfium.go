package render

import (
	"context"
	"fmt"
	"image"
	"runtime"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/webassembly"
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
	poolSize       int
)

func getPool(maxWorkers int) (pdfium.Pool, error) {
	globalPoolOnce.Do(func() {
		// Size pool based on workers needed
		poolSize = maxWorkers
		if poolSize < MinPoolSize {
			poolSize = MinPoolSize
		}
		if poolSize > MaxPoolSize {
			poolSize = MaxPoolSize
		}
		globalPool, globalPoolErr = webassembly.Init(webassembly.Config{
			MinIdle:  1,
			MaxIdle:  poolSize,
			MaxTotal: poolSize,
		})
	})
	return globalPool, globalPoolErr
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
	// Get an instance from the pool
	instance, err := r.pool.GetInstance(time.Second * 30)
	if err != nil {
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
		instance.Close()
		return nil, fmt.Errorf("failed to open PDF %s: %w", path, err)
	}

	// Get page count
	pageCountResp, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: doc.Document,
	})
	if err != nil {
		instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
		instance.Close()
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

	return PageInfo{
		Number:    pageNum,
		WidthPts:  sizeResp.Width,
		HeightPts: sizeResp.Height,
		WidthPx:   int(sizeResp.Width * float64(DPIStandard) / 72.0),
		HeightPx:  int(sizeResp.Height * float64(DPIStandard) / 72.0),
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

	dpi := opts.DPI
	if dpi <= 0 {
		dpi = DPIStandard
	}

	renderResp, err := d.instance.RenderPageInDPI(&requests.RenderPageInDPI{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageNum,
			},
		},
		DPI: dpi,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render page %d: %w", pageNum, err)
	}

	if renderResp.Result.Image == nil {
		return nil, fmt.Errorf("render returned nil image for page %d", pageNum)
	}

	return renderResp.Result.Image, nil
}

// RenderPages renders multiple pages - uses parallel rendering when beneficial
func (d *pdfiumDocument) RenderPages(ctx context.Context, pageNums []int, opts Options) ([]image.Image, error) {
	if len(pageNums) == 0 {
		return nil, nil
	}

	images := make([]image.Image, len(pageNums))
	out := make(chan RenderedPage, len(pageNums))

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.RenderPagesStream(ctx, pageNums, opts, out)
		close(out)
	}()

	for res := range out {
		if res.Err != nil {
			return nil, res.Err
		}
		images[res.Index] = res.Image
	}

	if err := <-errCh; err != nil {
		return nil, err
	}

	return images, nil
}

func (d *pdfiumDocument) RenderPagesStream(ctx context.Context, pageNums []int, opts Options, out chan<- RenderedPage) error {
	if len(pageNums) == 0 {
		return nil
	}

	numWorkers := d.renderer.workers
	if numWorkers > len(pageNums) {
		numWorkers = len(pageNums)
	}
	if numWorkers > poolSize {
		numWorkers = poolSize
	}

	// Job channel
	type job struct {
		index   int
		pageNum int
	}
	jobs := make(chan job, len(pageNums))

	// Send all jobs
	for i, pageNum := range pageNums {
		jobs <- job{index: i, pageNum: pageNum}
	}
	close(jobs)

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers)

	// Worker function
	worker := func(workerID int) {
		defer wg.Done()

		// Each worker gets its own PDFium instance and opens the document
		instance, err := d.renderer.pool.GetInstance(time.Second * 30)
		if err != nil {
			errors <- fmt.Errorf("worker %d: failed to get instance: %w", workerID, err)
			return
		}
		defer instance.Close()

		// Open the same document
		req := &requests.OpenDocument{
			FilePath: &d.path,
		}
		if d.password != "" {
			req.Password = &d.password
		}

		doc, err := instance.OpenDocument(req)
		if err != nil {
			errors <- fmt.Errorf("worker %d: failed to open document: %w", workerID, err)
			return
		}
		defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

		dpi := opts.DPI
		if dpi <= 0 {
			dpi = DPIStandard
		}

		// Process jobs
		for j := range jobs {
			select {
			case <-ctx.Done():
				out <- RenderedPage{Index: j.index, Err: ctx.Err()}
				return
			default:
			}

			renderResp, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
				Page: requests.Page{
					ByIndex: &requests.PageByIndex{
						Document: doc.Document,
						Index:    j.pageNum,
					},
				},
				DPI: dpi,
			})
			if err != nil {
				out <- RenderedPage{Index: j.index, Err: fmt.Errorf("failed to render page %d: %w", j.pageNum, err)}
				continue
			}

			if renderResp.Result.Image == nil {
				out <- RenderedPage{Index: j.index, Err: fmt.Errorf("render returned nil image for page %d", j.pageNum)}
				continue
			}

			out <- RenderedPage{
				Index: j.index,
				Image: renderResp.Result.Image,
			}
		}
	}

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(i)
	}

	// Wait for all workers to complete in a separate goroutine
	go func() {
		wg.Wait()
		close(errors)
	}()

	// Collect all errors
	var allErrors []error
	for err := range errors {
		if err != nil {
			allErrors = append(allErrors, err)
		}
	}

	// Return aggregated errors if any occurred
	if len(allErrors) > 0 {
		if len(allErrors) == 1 {
			return allErrors[0]
		}
		errMsg := fmt.Sprintf("%d worker errors occurred:", len(allErrors))
		for i, err := range allErrors {
			errMsg += fmt.Sprintf("\n  [%d] %v", i+1, err)
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}


func (d *pdfiumDocument) Close() error {
	_, err := d.instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
		Document: d.doc.Document,
	})
	d.instance.Close()
	return err
}
