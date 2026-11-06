package compare

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/asaf-shitrit/pdfcmp/pkg/render"
	"github.com/jung-kurt/gofpdf"
)

// writeTextPDF writes a text PDF of the given page count. With rewrite set,
// every line of page 3 is different text in the same layout. day sets the
// creation date so that two files with the same content still differ
// byte-wise.
func writeTextPDF(t *testing.T, path string, pages int, rewrite bool, day int) {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetCreationDate(time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC))
	for p := 0; p < pages; p++ {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 11)
		seed := int64(p)
		if rewrite && p == 2 {
			seed += 100
		}
		r := rand.New(rand.NewSource(seed))
		for l := 0; l < 25; l++ {
			words := make([]string, 9)
			for i := range words {
				words[i] = fmt.Sprintf("word%d", r.Intn(400))
			}
			pdf.CellFormat(0, 9, strings.Join(words, " "), "", 1, "L", false, 0, "")
		}
	}
	if err := pdf.OutputFileAndClose(path); err != nil {
		t.Fatal(err)
	}
}

func TestRewrittenPageInSameLayout(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.pdf")
	same := filepath.Join(dir, "same.pdf")
	rewritten := filepath.Join(dir, "rewritten.pdf")
	writeTextPDF(t, base, 5, false, 1)
	writeTextPDF(t, same, 5, false, 2)
	writeTextPDF(t, rewritten, 5, true, 2)

	// Unrelated text should score near 0 with MinHash, and text hashing off
	// should still catch the page through worst-page scoring.
	for _, tc := range []struct {
		name string
		th   TextHashType
	}{{"minhash", TextMinHash}, {"off", TextHashOff}} {
		t.Run(tc.name, func(t *testing.T) {
			cmp, err := New(WithDPI(72), WithWorkers(2), WithTextHash(tc.th))
			if err != nil {
				t.Fatal(err)
			}
			defer cmp.Close()
			ctx := context.Background()

			res, err := cmp.Compare(ctx, base, rewritten)
			if err != nil {
				t.Fatal(err)
			}
			if res.IsSimilar(DefaultSimilarityThreshold) {
				t.Errorf("rewritten page passed: similarity %.4f", res.SimilarityScore)
			}
			page3 := slices.IndexFunc(res.PageResults, func(pr PageResult) bool { return pr.Page == 3 })
			if page3 < 0 {
				t.Fatalf("page 3 was not compared; pages: %d", len(res.PageResults))
			}
			pr := res.PageResults[page3]
			if tc.th == TextHashOff {
				if pr.TextSimilarity != nil {
					t.Errorf("text hash off: page 3 text similarity = %s, want nil", fmtSim(pr.TextSimilarity))
				}
			} else {
				if pr.TextSimilarity == nil || *pr.TextSimilarity > 0.1 {
					t.Errorf("page 3 text similarity = %s, want at most 0.10", fmtSim(pr.TextSimilarity))
				}
				if pr.TextSimilarity != nil && pr.Similarity > *pr.TextSimilarity {
					t.Errorf("page 3 similarity %.4f is above its text similarity %.4f", pr.Similarity, *pr.TextSimilarity)
				}
			}

			res, err = cmp.Compare(ctx, base, same)
			if err != nil {
				t.Fatal(err)
			}
			if res.ByteComparison.Identical {
				t.Fatal("fixture bug: same-content files are byte-identical")
			}
			if !res.IsSimilar(DefaultSimilarityThreshold) {
				t.Errorf("same content called different: similarity %.4f", res.SimilarityScore)
			}
		})
	}
}

// longText returns n distinct words, enough shingles for an estimate.
func longText(n int) []string {
	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("word%d", i)
	}
	return words
}

func TestTextSimilarityOnlyWhenTextDiffers(t *testing.T) {
	words := longText(100)
	a := render.PageInfo{TextHash: 1, Text: strings.Join(words, " ")}
	words[50] = "changed"
	b := render.PageInfo{TextHash: 2, Text: strings.Join(words, " ")}
	none := render.PageInfo{}
	shortA := render.PageInfo{TextHash: 3, Text: "Annual Report 2025"}
	shortB := render.PageInfo{TextHash: 4, Text: "Annual Report 2026"}

	c := &comparer{opts: DefaultOptions()}
	if c.textSimilarity(a, a) != nil {
		t.Error("identical text hashes: want nil")
	}
	if c.textSimilarity(a, none) != nil {
		t.Error("page without text: want nil")
	}
	if c.textSimilarity(shortA, shortB) != nil {
		t.Error("three-word pages: want nil, too short to estimate")
	}
	if s := c.textSimilarity(a, b); s == nil || *s <= 0.8 || *s >= 1 {
		t.Errorf("one word of 100 changed: got %s, want between 0.8 and 1", fmtSim(s))
	}

	c.opts.TextHash = TextHashOff
	if c.textSimilarity(a, b) != nil {
		t.Error("TextHashOff: want nil")
	}
}

func TestWorstSimilarity(t *testing.T) {
	pages := []PageResult{{Similarity: 1}, {Similarity: 0.4}, {Similarity: 0.9}}
	if got := worstSimilarity(pages); got != 0.4 {
		t.Errorf("worstSimilarity = %v, want 0.4", got)
	}
	if got := worstSimilarity(nil); got != 1 {
		t.Errorf("worstSimilarity(nil) = %v, want 1", got)
	}
}

func fmtSim(s *float64) string {
	if s == nil {
		return "nil"
	}
	return fmt.Sprintf("%.4f", *s)
}

func TestStrategicSample(t *testing.T) {
	for n := 1; n <= 5; n++ {
		if got := strategicSample(n); len(got) != n {
			t.Errorf("strategicSample(%d) = %v, want every page", n, got)
		}
	}
	for n := 6; n <= 500; n++ {
		got := strategicSample(n)
		maxSize := int(math.Sqrt(float64(n))) + 2
		if len(got) < 3 || len(got) > maxSize {
			t.Fatalf("strategicSample(%d) has %d pages, want 3 to %d", n, len(got), maxSize)
		}
		for i, p := range got {
			if p < 0 || p >= n || (i > 0 && p <= got[i-1]) {
				t.Fatalf("strategicSample(%d) = %v: not sorted, unique and in range", n, got)
			}
		}
		for _, must := range []int{0, n / 2, n - 1} {
			if !slices.Contains(got, must) {
				t.Fatalf("strategicSample(%d) = %v, missing page %d", n, got, must)
			}
		}
	}
	if got := strategicSample(100); len(got) != 12 {
		t.Errorf("strategicSample(100) has %d pages, want sqrt(100)+2 = 12", len(got))
	}
}
