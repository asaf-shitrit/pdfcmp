package compare

import (
	"context"
	"testing"

	"github.com/asaf-shitrit/pdfcmp/internal/fixtures"
)

// writeFixture writes the named fixture from internal/fixtures into dir.
func writeFixture(t *testing.T, dir, name string) string {
	t.Helper()
	spec, ok := fixtures.Find(name)
	if !ok {
		t.Fatalf("no fixture named %s", name)
	}
	path, err := spec.Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// TestProperties checks identity and symmetry in PDF comparison.
func TestProperties(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	comparer, err := New(WithWorkers(4))
	if err != nil {
		t.Fatalf("Failed to create comparer: %v", err)
	}
	defer comparer.Close()

	// A file compared with itself, and with a regenerated copy of the same
	// content, is identical.
	for _, pair := range [][2]string{
		{"small_a.pdf", "small_a.pdf"},
		{"small_a.pdf", "small_b.pdf"},
		{"graphic_a.pdf", "graphic_b.pdf"},
		{"scanned_a.pdf", "scanned_b.pdf"},
	} {
		t.Run("Identity_"+pair[0]+"_"+pair[1], func(t *testing.T) {
			res, err := comparer.Compare(ctx, writeFixture(t, dir, pair[0]), writeFixture(t, dir, pair[1]))
			if err != nil {
				t.Fatalf("Compare failed: %v", err)
			}
			if pair[0] != pair[1] && res.ByteComparison.Identical {
				t.Fatal("the files are byte-identical, so nothing was rendered")
			}
			if res.SimilarityScore < 0.999 {
				t.Errorf("similarity %f, want at least 0.999", res.SimilarityScore)
			}
		})
	}

	t.Run("Symmetry", func(t *testing.T) {
		f1 := writeFixture(t, dir, "medium_a.pdf")
		f2 := writeFixture(t, dir, "medium_modified.pdf")

		res12, err := comparer.Compare(ctx, f1, f2)
		if err != nil {
			t.Fatalf("Compare A->B failed: %v", err)
		}
		res21, err := comparer.Compare(ctx, f2, f1)
		if err != nil {
			t.Fatalf("Compare B->A failed: %v", err)
		}
		if res12.SimilarityScore != res21.SimilarityScore {
			t.Errorf("Symmetry failed: A->B=%f, B->A=%f", res12.SimilarityScore, res21.SimilarityScore)
		}
		if res12.IsSimilar(DefaultSimilarityThreshold) {
			t.Errorf("modified page 10 passed: similarity %f", res12.SimilarityScore)
		}
	})
}
