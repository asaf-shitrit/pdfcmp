package compare

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asafshitrit/pdfcmp/pkg/render"
)

// fakeDoc is a render.Document whose pages render as a flat grey image, or
// fail for the page numbers in fail.
type fakeDoc struct {
	pages int
	fail  map[int]bool
}

func (d *fakeDoc) PageCount() int { return d.pages }

func (d *fakeDoc) PageInfo(pageNum int) (render.PageInfo, error) {
	return render.PageInfo{Number: pageNum, WidthPts: 72, HeightPts: 72}, nil
}

func (d *fakeDoc) RenderPage(ctx context.Context, pageNum int, opts render.Options) (image.Image, error) {
	if d.fail[pageNum] {
		return nil, fmt.Errorf("page %d is broken", pageNum)
	}
	img := image.NewGray(image.Rect(0, 0, 32, 32))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	img.Set(pageNum%32, 0, color.White) // pages differ from each other, not across documents
	return img, nil
}

func (d *fakeDoc) RenderPages(ctx context.Context, pageNums []int, opts render.Options) ([]image.Image, error) {
	imgs := make([]image.Image, len(pageNums))
	for i, p := range pageNums {
		img, err := d.RenderPage(ctx, p, opts)
		if err != nil {
			return nil, err
		}
		imgs[i] = img
	}
	return imgs, nil
}

func (d *fakeDoc) RenderPagesStream(ctx context.Context, pageNums []int, opts render.Options, out chan<- render.RenderedPage) error {
	for i, p := range pageNums {
		if err := ctx.Err(); err != nil {
			out <- render.RenderedPage{Index: i, Err: err}
			return nil
		}
		img, err := d.RenderPage(ctx, p, opts)
		out <- render.RenderedPage{Index: i, Image: img, Err: err}
	}
	return nil
}

func (d *fakeDoc) Close() error { return nil }

func allPageNums(n int) []int {
	pages := make([]int, n)
	for i := range pages {
		pages[i] = i
	}
	return pages
}

// runComparePages calls comparePages and fails the test if it does not return
// within ten seconds, so a hang fails fast instead of timing out the suite.
func runComparePages(t *testing.T, ctx context.Context, doc1, doc2 render.Document, pages []int) ([]PageResult, error) {
	t.Helper()
	c := &comparer{opts: DefaultOptions()}
	c.opts.Workers = 2

	type outcome struct {
		results []PageResult
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := c.comparePages(ctx, doc1, doc2, pages, render.Options{DPI: 72})
		done <- outcome{r, err}
	}()
	select {
	case o := <-done:
		return o.results, o.err
	case <-time.After(10 * time.Second):
		t.Fatal("comparePages did not return within 10s")
		return nil, nil
	}
}

func TestComparePagesAllRender(t *testing.T) {
	const n = 40
	results, err := runComparePages(t, context.Background(), &fakeDoc{pages: n}, &fakeDoc{pages: n}, allPageNums(n))
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range results {
		if r.Page != i+1 || r.Similarity < 0.99 {
			t.Errorf("result %d: page %d similarity %.4f, want page %d near 1", i, r.Page, r.Similarity, i+1)
		}
	}
}

func TestComparePagesReturnsRenderErrors(t *testing.T) {
	// More failing pages than the pipeline buffers, in one document only:
	// none of them can be paired, and every error has to be collected.
	const n = 64
	fail := map[int]bool{}
	for p := 0; p < n; p++ {
		fail[p] = true
	}
	_, err := runComparePages(t, context.Background(), &fakeDoc{pages: n}, &fakeDoc{pages: n, fail: fail}, allPageNums(n))
	if err == nil {
		t.Fatal("want an error for pages that failed to render")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d render errors occurred", n)) {
		t.Errorf("error = %.120q, want all %d page errors reported", err, n)
	}
}

func TestComparePagesCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runComparePages(t, ctx, &fakeDoc{pages: 20}, &fakeDoc{pages: 20}, allPageNums(20))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestPageCountMismatch(t *testing.T) {
	dir := t.TempDir()
	five := filepath.Join(dir, "five.pdf")
	three := filepath.Join(dir, "three.pdf")
	writeTextPDF(t, five, 5, false, 1)
	writeTextPDF(t, three, 3, false, 1)

	cmp, err := New(WithDPI(72), WithWorkers(2))
	if err != nil {
		t.Fatal(err)
	}
	defer cmp.Close()

	for _, pair := range [][2]string{{five, three}, {three, five}} {
		res, err := cmp.Compare(context.Background(), pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if res.SimilarityScore != 0 || res.VisualComparison.SamplingStrategy != "page count mismatch" {
			t.Errorf("%s vs %s: similarity %.4f, strategy %q; want 0 and page count mismatch",
				filepath.Base(pair[0]), filepath.Base(pair[1]), res.SimilarityScore, res.VisualComparison.SamplingStrategy)
		}
	}
}
