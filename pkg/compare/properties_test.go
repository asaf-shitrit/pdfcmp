package compare

import (
	"context"
	"path/filepath"
	"testing"
)

// TestComparisonProperties verifies fundamental properties of the comparison logic:
// 1. Identity: Compare(A, A) should be 100% similarity.
// 2. Symmetry: Compare(A, B) should equal Compare(B, A).
func TestComparisonProperties(t *testing.T) {
	fixturesDir := "../../bench/fixtures"
	pdfA := filepath.Join(fixturesDir, "small_a.pdf")
	pdfB := filepath.Join(fixturesDir, "small_b.pdf")

	// Use a small set of fixtures. Check if they exist.
	// In a real environment, we'd ensure these are generated.
	// For this test, if they don't exist, we skip.
	
	ctx := context.Background()
	cmp, err := New(WithMode(ModeVisual), WithSampling(SamplingAll))
	if err != nil {
		t.Fatalf("Failed to create comparer: %v", err)
	}
	defer cmp.Close()

	// 1. Identity
	t.Run("Identity", func(t *testing.T) {
		res, err := cmp.Compare(ctx, pdfA, pdfA)
		if err != nil {
			t.Fatalf("Identity compare failed: %v", err)
		}
		if res.SimilarityScore < 0.999 {
			t.Errorf("Identity similarity too low: %f", res.SimilarityScore)
		}
	})

	// 2. Symmetry
	t.Run("Symmetry", func(t *testing.T) {
		resAB, err := cmp.Compare(ctx, pdfA, pdfB)
		if err != nil {
			t.Fatalf("Compare(A, B) failed: %v", err)
		}

		resBA, err := cmp.Compare(ctx, pdfB, pdfA)
		if err != nil {
			t.Fatalf("Compare(B, A) failed: %v", err)
		}

		// Tolerance for floating point
		diff := resAB.SimilarityScore - resBA.SimilarityScore
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.0001 {
			t.Errorf("Symmetry violated: A->B=%f, B->A=%f", resAB.SimilarityScore, resBA.SimilarityScore)
		}
	})
}
