package render

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPdfiumIntegration(t *testing.T) {
	// Search for a fixture to test with
	fixturePath := filepath.Join("../../bench/fixtures", "small_a.pdf")
	if _, err := os.Stat(fixturePath); err != nil {
		t.Skipf("Fixture not found at %s, skipping integration test", fixturePath)
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
