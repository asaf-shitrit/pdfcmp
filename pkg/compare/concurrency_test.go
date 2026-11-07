package compare

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/asaf-shitrit/pdfcmp/internal/fixtures"
	"github.com/asaf-shitrit/pdfcmp/pkg/render"
)

// TestConcurrentCompares runs more comparisons at once than the PDFium pool
// has instances for two documents each. Every comparison must finish, and
// none may wait out the pool's open timeout.
func TestConcurrentCompares(t *testing.T) {
	if testing.Short() {
		t.Skip("renders PDFs")
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
	if err := fixtures.Graphic(a, 4, 1); err != nil {
		t.Fatal(err)
	}
	if err := fixtures.Graphic(b, 4, 2); err != nil {
		t.Fatal(err)
	}
	cmp, err := New(WithSampling(SamplingAll), WithWorkers(2))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmp.Close() }()

	// Start the pool so its size is known.
	if _, err := cmp.Compare(context.Background(), a, b); err != nil {
		t.Fatal(err)
	}
	n := 2*render.PoolSize() + 1

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			if _, err := cmp.Compare(ctx, a, b); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
