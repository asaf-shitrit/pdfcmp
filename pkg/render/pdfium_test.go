package render

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/asaf-shitrit/pdfcmp/internal/fixtures"
)

func TestPdfiumIntegration(t *testing.T) {
	spec, _ := fixtures.Find("small_a.pdf")
	fixturePath, err := spec.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	renderer, err := NewPdfiumRenderer()
	if err != nil {
		t.Fatalf("Failed to create PDFium renderer: %v", err)
	}
	defer renderer.Close()

	ctx := context.Background()

	t.Run("OpenAndCount", func(t *testing.T) {
		doc, err := renderer.Open(fixturePath)
		if err != nil {
			t.Fatalf("Failed to open PDF: %v", err)
		}
		defer doc.Close()

		if doc.PageCount() <= 0 {
			t.Errorf("Expected positive page count, got %d", doc.PageCount())
		}
	})

	t.Run("RenderPages", func(t *testing.T) {
		doc, err := renderer.Open(fixturePath)
		if err != nil {
			t.Fatalf("Failed to open PDF: %v", err)
		}
		defer doc.Close()

		imgs, err := doc.RenderPages(ctx, []int{0, doc.PageCount() - 1}, Options{DPI: DPIThumbnail})
		if err != nil {
			t.Fatal(err)
		}
		if len(imgs) != 2 || imgs[0] == nil || imgs[1] == nil {
			t.Fatalf("got %d images, want 2 rendered pages", len(imgs))
		}
		if _, err := doc.RenderPages(ctx, []int{doc.PageCount()}, Options{}); err == nil {
			t.Error("rendering past the last page: want an error")
		}
	})

	t.Run("PageInfo", func(t *testing.T) {
		doc, err := renderer.Open(fixturePath)
		if err != nil {
			t.Fatalf("Failed to open PDF: %v", err)
		}
		defer doc.Close()

		info, err := doc.PageInfo(0)
		if err != nil {
			t.Fatalf("Failed to get page info: %v", err)
		}

		if info.WidthPts <= 0 || info.HeightPts <= 0 {
			t.Errorf("Invalid page dimensions: %vx%v", info.WidthPts, info.HeightPts)
		}
	})

	t.Run("RenderPage", func(t *testing.T) {
		doc, err := renderer.Open(fixturePath)
		if err != nil {
			t.Fatalf("Failed to open PDF: %v", err)
		}
		defer doc.Close()

		img, err := doc.RenderPage(ctx, 0, DefaultOptions())
		if err != nil {
			t.Fatalf("Failed to render page: %v", err)
		}

		if img == nil {
			t.Fatal("Rendered image is nil")
		}

		bounds := img.Bounds()
		if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
			t.Errorf("Invalid image bounds: %v", bounds)
		}
	})

	t.Run("RenderPagesStream", func(t *testing.T) {
		doc, err := renderer.Open(fixturePath)
		if err != nil {
			t.Fatalf("Failed to open PDF: %v", err)
		}
		defer doc.Close()

		count := doc.PageCount()
		pageNums := make([]int, count)
		for i := 0; i < count; i++ {
			pageNums[i] = i
		}

		out := make(chan RenderedPage, count)
		errCh := make(chan error, 1)

		go func() {
			errCh <- doc.RenderPagesStream(ctx, pageNums, DefaultOptions(), out)
			close(out)
		}()

		received := 0
		for res := range out {
			if res.Err != nil {
				t.Errorf("Render page %d failed: %v", res.Index, res.Err)
			}
			if res.Image == nil {
				t.Errorf("Render page %d returned nil image", res.Index)
			}
			received++
		}

		if err := <-errCh; err != nil {
			t.Fatalf("RenderPagesStream returned error: %v", err)
		}

		if received != count {
			t.Errorf("Expected %d pages, received %d", count, received)
		}
	})
}

// contentHashes writes each fixture to its own file and returns the content
// hash of page 0 of every one.
func contentHashes(t *testing.T, writers ...func(string) error) []uint64 {
	t.Helper()
	renderer, err := NewPdfiumRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()

	dir := t.TempDir()
	var hashes []uint64
	for i, write := range writers {
		path := filepath.Join(dir, fmt.Sprintf("f%d.pdf", i))
		if err := write(path); err != nil {
			t.Fatal(err)
		}
		doc, err := renderer.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		h, err := doc.ContentHash(0)
		_ = doc.Close()
		if err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, h)
	}
	return hashes
}

func TestContentHash(t *testing.T) {
	text := func(path string) error { return fixtures.Text(path, 2, "Title", 1) }
	graphic := func(seed int64) func(string) error {
		return func(path string) error { return fixtures.Graphic(path, 2, seed) }
	}
	scan := func(seed int64) func(string) error {
		return func(path string) error { return fixtures.ImageOnly(path, 2, seed) }
	}

	t.Run("same content rebuilt is equal", func(t *testing.T) {
		h := contentHashes(t, text, text)
		if h[0] != h[1] {
			t.Errorf("hashes differ: %x vs %x", h[0], h[1])
		}
	})
	t.Run("redrawn shapes differ", func(t *testing.T) {
		h := contentHashes(t, graphic(42), graphic(43))
		if h[0] == h[1] {
			t.Errorf("hashes equal: %x", h[0])
		}
	})
	t.Run("image only pages differ by seed", func(t *testing.T) {
		h := contentHashes(t, scan(1), scan(2))
		if h[0] == h[1] {
			t.Errorf("hashes equal: %x", h[0])
		}
	})
	t.Run("out of range page fails", func(t *testing.T) {
		renderer, err := NewPdfiumRenderer()
		if err != nil {
			t.Fatal(err)
		}
		defer renderer.Close()
		path := filepath.Join(t.TempDir(), "a.pdf")
		if err := text(path); err != nil {
			t.Fatal(err)
		}
		doc, err := renderer.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer doc.Close()
		if _, err := doc.ContentHash(5); err == nil {
			t.Error("want error")
		}
	})
}

// TestRenderPagesStreamWithPoolTaken renders while every other instance of
// the pool is held by open documents: the stream renders on the document's
// own instance instead of waiting for one.
func TestRenderPagesStreamWithPoolTaken(t *testing.T) {
	spec, _ := fixtures.Find("small_a.pdf")
	path, err := spec.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewPdfiumRendererWithPool(4)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := renderer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = doc.Close() }()
	for range PoolSize() - 1 {
		held, err := renderer.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.Close() }()
	}

	pages := make([]int, doc.PageCount())
	for i := range pages {
		pages[i] = i
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	imgs, err := doc.RenderPages(ctx, pages, Options{DPI: DPIThumbnail})
	if err != nil {
		t.Fatal(err)
	}
	for i, img := range imgs {
		if img == nil {
			t.Errorf("page %d: no image", i)
		}
	}
}

// TestRenderedImagesOutliveLaterRenders checks that a render is the caller's
// own copy: rendering more pages on the same instance leaves it unchanged.
func TestRenderedImagesOutliveLaterRenders(t *testing.T) {
	spec, _ := fixtures.Find("small_diff_a.pdf")
	path, err := spec.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewPdfiumRenderer()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := renderer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = doc.Close() }()

	ctx := context.Background()
	first, err := doc.RenderPage(ctx, 0, Options{DPI: DPIThumbnail})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := slices.Clone(first.(*image.RGBA).Pix)
	for p := range doc.PageCount() {
		if _, err := doc.RenderPage(ctx, p, Options{DPI: DPIThumbnail}); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(snapshot, first.(*image.RGBA).Pix) {
		t.Error("an earlier render changed when later pages were rendered")
	}
}
