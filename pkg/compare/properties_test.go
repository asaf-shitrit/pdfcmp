package compare

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestProperties checks for identity and symmetry in PDF comparison.
func TestProperties(t *testing.T) {
	// Find some fixtures to test with
	fixtures := []string{
		"small_a.pdf",
		"medium_a.pdf",
		"graphic_a.pdf",
	}

	ctx := context.Background()
	comparer, err := New(WithWorkers(4))
	if err != nil {
		t.Fatalf("Failed to create comparer: %v", err)
	}
	defer comparer.Close()

	for _, f := range fixtures {
		path := filepath.Join("../../bench/fixtures", f)
		if _, err := os.Stat(path); err != nil {
			t.Logf("Skipping fixture %s: %v", f, err)
			continue
		}

		t.Run("Identity_"+f, func(t *testing.T) {
			res, err := comparer.Compare(ctx, path, path)
			if err != nil {
				t.Errorf("Compare failed: %v", err)
				return
			}
			if !res.Identical {
				t.Errorf("Identity failed for %s: expected identical, got similarity %f", f, res.SimilarityScore)
			}
			if res.SimilarityScore < 0.999 {
				t.Errorf("Similarity score too low for identity: %f", res.SimilarityScore)
			}
		})
	}

	// Symmetry test
	t.Run("Symmetry", func(t *testing.T) {
		f1 := filepath.Join("../../bench/fixtures", "small_a.pdf")
		f2 := filepath.Join("../../bench/fixtures", "medium_a.pdf")

		if _, err := os.Stat(f1); err != nil {
			return
		}
		if _, err := os.Stat(f2); err != nil {
			return
		}

		res12, err := comparer.Compare(ctx, f1, f2)
		if err != nil {
			t.Fatalf("Compare A->B failed: %v", err)
		}
		res21, err := comparer.Compare(ctx, f2, f1)
		if err != nil {
			t.Fatalf("Compare B->A failed: %v", err)
		}

		// Use a small epsilon for floating point comparison if necessary, 
		// but since it's the same math it should be identical or very close.
		if res12.SimilarityScore != res21.SimilarityScore {
			t.Errorf("Symmetry failed: A->B=%f, B->A=%f", res12.SimilarityScore, res21.SimilarityScore)
		}
	})
}
