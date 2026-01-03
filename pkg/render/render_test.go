package render

import (
	"context"
	"image"
	"testing"
	"time"
)

// MockDocument implements Document for testing
type MockDocument struct {
	pages int
}

func (m *MockDocument) PageCount() int {
	return m.pages
}

func (m *MockDocument) PageInfo(pageNum int) (PageInfo, error) {
	return PageInfo{
		Number:    pageNum,
		WidthPts:  612,
		HeightPts: 792,
		WidthPx:   612,
		HeightPx:  792,
	}, nil
}

func (m *MockDocument) RenderPage(ctx context.Context, pageNum int, opts Options) (image.Image, error) {
	// Simulate work
	time.Sleep(1 * time.Millisecond)
	return image.NewRGBA(image.Rect(0, 0, 100, 100)), nil
}

func (m *MockDocument) RenderPages(ctx context.Context, pageNums []int, opts Options) ([]image.Image, error) {
	imgs := make([]image.Image, len(pageNums))
	for i, page := range pageNums {
		img, _ := m.RenderPage(ctx, page, opts)
		imgs[i] = img
	}
	return imgs, nil
}

func (m *MockDocument) RenderPagesStream(ctx context.Context, pageNums []int, opts Options, out chan<- RenderedPage) error {
	for i, page := range pageNums {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		img, _ := m.RenderPage(ctx, page, opts)
		out <- RenderedPage{Index: i, Image: img}
	}
	return nil
}

func (m *MockDocument) Close() error {
	return nil
}

func TestRenderPagesStream(t *testing.T) {
	doc := &MockDocument{pages: 10}
	
	t.Run("Basic stream", func(t *testing.T) {
		out := make(chan RenderedPage, 5)
		errCh := make(chan error, 1)
		
		go func() {
			errCh <- doc.RenderPagesStream(context.Background(), []int{0, 1, 2, 3, 4}, Options{}, out)
			close(out)
		}()
		
		count := 0
		for range out {
			count++
		}
		
		if err := <-errCh; err != nil {
			t.Fatalf("RenderPagesStream failed: %v", err)
		}
		
		if count != 5 {
			t.Errorf("Expected 5 pages, got %d", count)
		}
	})
}

func TestCalculatePixelDimensions(t *testing.T) {
	w, h := CalculatePixelDimensions(72, 144, 72) // 1 inch x 2 inches at 72 DPI
	if w != 72 || h != 144 {
		t.Errorf("Expected 72x144, got %dx%d", w, h)
	}
	
	w, h = CalculatePixelDimensions(72, 144, 144) // 1 inch x 2 inches at 144 DPI
	if w != 144 || h != 288 {
		t.Errorf("Expected 144x288, got %dx%d", w, h)
	}
}

func TestEstimateMemoryUsage(t *testing.T) {
	// 100x100 points at 72 DPI = 100x100 pixels.
	// 4 bytes per pixel = 40,000 bytes.
	mem := EstimateMemoryUsage(100, 100, 72)
	expected := int64(100 * 100 * 4)
	if mem != expected {
		t.Errorf("Expected %d bytes, got %d", expected, mem)
	}
}
