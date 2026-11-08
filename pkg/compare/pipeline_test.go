package compare

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asaf-shitrit/pdfcmp/internal/fixtures"
	"github.com/asaf-shitrit/pdfcmp/pkg/render"
)

// fakeDoc is a render.Document whose pages render as a flat grey image. Pages
// in blank render black instead, pages in fail fail to render, and a non-nil
// streamErr makes RenderPagesStream fail before sending anything. Each page
// takes delay to render; rendered counts the pages rendered.
type fakeDoc struct {
	pages     int
	fail      map[int]bool
	blank     map[int]bool
	streamErr error
	delay     time.Duration
	rendered  atomic.Int32

	textHash   uint64 // reported by PageInfo; 0 means no text
	content    uint64 // reported by ContentHash
	contentErr error
	draw       func(pageNum int, img *image.Gray) // draws on each render, when set
}

// errBrokenPage is what a fakeDoc page in fail renders with.
var errBrokenPage = errors.New("page is broken")

func (d *fakeDoc) PageCount() int { return d.pages }

func (d *fakeDoc) PageInfo(pageNum int) (render.PageInfo, error) {
	return render.PageInfo{Number: pageNum, WidthPts: 72, HeightPts: 72, TextHash: d.textHash}, nil
}

func (d *fakeDoc) ContentHash(pageNum int) (uint64, error) { return d.content, d.contentErr }

func (d *fakeDoc) RenderPage(ctx context.Context, pageNum int, opts render.Options) (image.Image, error) {
	if d.fail[pageNum] {
		return nil, fmt.Errorf("page %d: %w", pageNum, errBrokenPage)
	}
	time.Sleep(d.delay)
	d.rendered.Add(1)
	img := image.NewGray(image.Rect(0, 0, 32, 32))
	if d.blank[pageNum] {
		return img, nil
	}
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	img.Set(pageNum%32, 0, color.White) // pages differ from each other, not across documents
	if d.draw != nil {
		d.draw(pageNum, img)
	}
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
	if d.streamErr != nil {
		return d.streamErr
	}
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
	var multi *renderErrors
	if !errors.As(err, &multi) || len(multi.errs) != n {
		t.Fatalf("errors.As: want a renderErrors of %d errors, got %v", n, multi)
	}
	// The page errors sit inside the joined error: only Unwrap reaches them.
	if !errors.Is(err, errBrokenPage) {
		t.Error("errors.Is does not reach the page errors inside the joined error")
	}
}

func TestComparePagesStopsTheOtherStreamOnFailure(t *testing.T) {
	const n = 200
	errOpen := errors.New("cannot open")
	slow := &fakeDoc{pages: n, delay: time.Millisecond}
	broken := &fakeDoc{pages: n, streamErr: errOpen}
	_, err := runComparePages(t, context.Background(), slow, broken, allPageNums(n))
	if !errors.Is(err, errOpen) {
		t.Fatalf("err = %v, want it to wrap doc2's stream error", err)
	}
	if strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %q, want no errors for pages cancelled because doc2 failed", err)
	}
	if got := slow.rendered.Load(); got >= n/2 {
		t.Errorf("doc1 rendered %d of %d pages after doc2 failed, want it stopped early", got, n)
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

// TestComparePagesCancelledTextDecided checks that a done context stops a
// comparison whose pages text decides, which never reaches the renderer.
func TestComparePagesCancelledTextDecided(t *testing.T) {
	for _, tc := range []struct {
		name string
		mk   func() (context.Context, context.CancelFunc)
		want error
	}{
		{"canceled", func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, context.Canceled},
		{"deadline", func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), time.Nanosecond)
		}, context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.mk()
			defer cancel()
			<-ctx.Done()
			mk := func() *fakeDoc { return &fakeDoc{pages: 20, textHash: 7, content: 9} }
			_, err := runComparePages(t, ctx, mk(), mk(), allPageNums(20))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
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
		if res.SimilarityScore != 0 || res.VisualComparison.SamplingStrategy != "page count mismatch, aligned" {
			t.Errorf("%s vs %s: similarity %.4f, strategy %q; want 0 and page count mismatch, aligned",
				filepath.Base(pair[0]), filepath.Base(pair[1]), res.SimilarityScore, res.VisualComparison.SamplingStrategy)
		}
	}
}

func TestNewDoesNotStartRenderer(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if c.(*comparer).renderer != nil {
		t.Fatal("renderer created eagerly")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close without renderer: %v", err)
	}
}

func TestComparePagesContentHashGatesTheTextShortcut(t *testing.T) {
	cases := []struct {
		name       string
		content2   uint64
		contentErr error
		wantRender bool
		wantNotOne bool
	}{
		{"same text and content skips rendering", 7, nil, false, false},
		{"same text, different content renders", 8, nil, true, true},
		{"content hash error renders", 7, errors.New("no hash"), true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc1 := &fakeDoc{pages: 1, textHash: 5, content: 7}
			doc2 := &fakeDoc{pages: 1, textHash: 5, content: tc.content2, contentErr: tc.contentErr, blank: map[int]bool{0: true}}
			results, err := runComparePages(t, context.Background(), doc1, doc2, allPageNums(1))
			if err != nil {
				t.Fatal(err)
			}
			rendered := doc1.rendered.Load()+doc2.rendered.Load() > 0
			if rendered != tc.wantRender {
				t.Errorf("rendered = %v, want %v", rendered, tc.wantRender)
			}
			if (results[0].Similarity < 1.0) != tc.wantNotOne {
				t.Errorf("similarity = %v, wantNotOne = %v", results[0].Similarity, tc.wantNotOne)
			}
		})
	}
}

func TestRedrawnGraphicsAreNotSimilar(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
	if err := fixtures.Graphic(a, 10, 42); err != nil {
		t.Fatal(err)
	}
	if err := fixtures.Graphic(b, 10, 43); err != nil {
		t.Fatal(err)
	}
	cmp, err := New(WithDPI(72), WithWorkers(2))
	if err != nil {
		t.Fatal(err)
	}
	defer cmp.Close()
	res, err := cmp.Compare(context.Background(), a, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, pr := range res.PageResults {
		t.Logf("page %d similarity %.4f", pr.Page, pr.Similarity)
	}
	if res.IsSimilar(DefaultSimilarityThreshold) {
		t.Errorf("redrawn graphics passed: similarity %.4f", res.SimilarityScore)
	}
}

func TestPagePairerBoundsUnpairedRenders(t *testing.T) {
	p := newPagePairer(16, 3)
	img := image.NewGray(image.Rect(0, 0, 1, 1))
	var added atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for j := range 10 {
			p.add(j, 0, img)
			added.Add(1)
		}
	}()

	waitFor := func(want int32) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for added.Load() != want && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		time.Sleep(20 * time.Millisecond) // let a pairer that does not block run on
		if got := added.Load(); got != want {
			t.Fatalf("pairer took %d renders of the document ahead, want %d", got, want)
		}
	}
	waitFor(3)
	p.add(0, 1, img) // pairs page 0 and frees a slot
	waitFor(4)
	p.finish(1) // the other document will send nothing more
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pairer kept waiting after the other document finished")
	}
}

func TestComparePagesOneDocumentFarAhead(t *testing.T) {
	const n = 60
	results, err := runComparePages(t, context.Background(), &fakeDoc{pages: n}, &fakeDoc{pages: n, delay: time.Millisecond}, allPageNums(n))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != n {
		t.Fatalf("got %d results, want %d", len(results), n)
	}
}

// TestComparePagesPartialFailures fails more pages of one document than the
// pairer holds renders of the other: the renders whose partner failed must
// not use up its space.
func TestComparePagesPartialFailures(t *testing.T) {
	const n = 20
	fail := map[int]bool{}
	for p := range 8 {
		fail[p] = true
	}
	_, err := runComparePages(t, context.Background(), &fakeDoc{pages: n}, &fakeDoc{pages: n, fail: fail}, allPageNums(n))
	if !errors.Is(err, errBrokenPage) {
		t.Fatalf("got %v, want the broken pages' errors", err)
	}
}

// TestPagePairerAbortFreesWaiters checks that an aborted pairer stops a
// render waiting for space, even though the other document never finishes.
func TestPagePairerAbortFreesWaiters(t *testing.T) {
	p := newPagePairer(16, 1)
	img := image.NewGray(image.Rect(0, 0, 1, 1))
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.add(0, 0, img)
		p.add(1, 0, img) // waits: no space, and doc 2 sends nothing
	}()
	time.Sleep(20 * time.Millisecond)
	p.abort()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pairer kept waiting after it was aborted")
	}
}
