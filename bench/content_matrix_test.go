package bench

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asaf-shitrit/pdfcmp/pkg/compare"
	"github.com/asaf-shitrit/pdfcmp/pkg/content"
	"github.com/jung-kurt/gofpdf"
)

// The content matrix measures what pdfcmp says about documents whose text
// changed while their layout did not. Every page carries 25 lines of text,
// one of which holds an invoice total. Variants change only the middle page:
//
//	same:    the document is rebuilt from the same seed (same content, new bytes)
//	replace: 24 of the middle page's 25 lines are different text in the same
//	         layout; line 13, the invoice total, is kept
//	half:    12 of the middle page's 25 lines are different text
//	line:    one line of the middle page is different text
//	number:  the invoice total on the middle page changes by one digit
//
// The jaccard column is the exact Jaccard similarity of the middle page's word
// 3-shingles, computed from the generated text, for the MinHash estimate in
// the textsim column to be read against. The simhash column is the SimHash
// estimate of the same pair (see simhash_test.go), and simverd is the verdict
// pdfcmp would reach with SimHash in MinHash's place.
//
// Run it with:
//
//	PDFCMP_MATRIX=1 go test ./bench -run TestContentMatrix -v -timeout 30m

const (
	matrixLinesPerPage = 25
	matrixTotalLine    = 12
)

var matrixWords = strings.Fields(`account amount annual approval balance billing
budget charge client contract cost credit customer date debit delivery deposit
discount due expense fee fiscal fund invoice item ledger license margin net
notice order owner paid partner payment period policy price quarter quote rate
receipt record refund renewal report revenue review salary schedule service
statement subtotal supplier tax term total transfer unit vendor warranty`)

type matrixVariant string

const (
	variantSame    matrixVariant = "same"
	variantReplace matrixVariant = "replace"
	variantHalf    matrixVariant = "half"
	variantLine    matrixVariant = "line"
	variantNumber  matrixVariant = "number"
)

// matrixLine returns a deterministic line of 9 or 10 words.
func matrixLine(r *rand.Rand) string {
	n := 9 + r.Intn(2)
	words := make([]string, n)
	for i := range words {
		words[i] = matrixWords[r.Intn(len(matrixWords))]
	}
	return strings.Join(words, " ")
}

// matrixPageLines returns the lines of page p of a document with the given
// page count. Only the middle page differs between variants.
func matrixPageLines(pages, p int, v matrixVariant) []string {
	middle := pages / 2
	orig := rand.New(rand.NewSource(int64(1000 + p)))
	alt := rand.New(rand.NewSource(int64(9000 + p)))

	total := 1250 + p
	if p == middle && v == variantNumber {
		total += 30 // 1,2X0 -> 1,2Y0: one digit
	}

	lines := make([]string, matrixLinesPerPage)
	for l := range lines {
		// Draw from both sources on every line so that a rewritten line
		// leaves the lines after it unchanged.
		a, b := matrixLine(orig), matrixLine(alt)
		lines[l] = a
		if p == middle {
			switch {
			case v == variantReplace,
				v == variantHalf && l < 13,
				v == variantLine && l == 5:
				lines[l] = b
			}
		}
		if l == matrixTotalLine {
			lines[l] = fmt.Sprintf("Invoice total due this period: %d,%03d.00 EUR", total/1000, total%1000)
		}
	}
	return lines
}

// exactJaccard is the Jaccard similarity of two texts' word 3-shingle sets.
func exactJaccard(a, b string) float64 {
	set := func(s string) map[uint64]bool {
		m := map[uint64]bool{}
		for _, h := range content.Shingles(s, content.DefaultShingleSize) {
			m[h] = true
		}
		return m
	}
	sa, sb := set(a), set(b)
	inter := 0
	for h := range sa {
		if sb[h] {
			inter++
		}
	}
	return float64(inter) / float64(len(sa)+len(sb)-inter)
}

// writeMatrixPDF writes a text PDF of the given page count. The middle page is
// altered according to the variant; every other page is identical across
// variants.
func writeMatrixPDF(path string, pages int, v matrixVariant, base bool) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(true)
	// The base and the variant get different creation dates, so even the
	// "same" variant has new bytes and has to pass through the full pipeline.
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !base {
		created = created.AddDate(0, 0, 1)
	}
	pdf.SetCreationDate(created)

	for p := 0; p < pages; p++ {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 11)
		for _, line := range matrixPageLines(pages, p, v) {
			pdf.CellFormat(0, 9, line, "", 1, "L", false, 0, "")
		}
	}
	return pdf.OutputFileAndClose(path)
}

type matrixCase struct {
	name string
	opts []compare.Option
}

// matrixHashModes lists the text-hash configurations the matrix runs. "off"
// runs first: its scores feed the SimHash verdict column of the MinHash rows.
var matrixHashModes = []matrixCase{
	{name: "off", opts: []compare.Option{compare.WithTextHash(compare.TextHashOff)}},
	{name: "minhash", opts: []compare.Option{compare.WithTextHash(compare.TextMinHash)}},
}

func TestContentMatrix(t *testing.T) {
	if os.Getenv("PDFCMP_MATRIX") == "" {
		t.Skip("set PDFCMP_MATRIX=1 to run the content matrix")
	}

	dir := t.TempDir()
	pageCounts := []int{1, 5, 10, 30, 100}
	variants := []matrixVariant{variantSame, variantReplace, variantHalf, variantLine, variantNumber}
	dpis := []int{72, 150, 300}
	const threshold = compare.DefaultSimilarityThreshold

	ctx := context.Background()
	offScores := map[string]float64{} // "pages/variant/dpi" -> score with text hashing off
	fmt.Printf("%-8s %-6s %-8s %-4s %8s %8s %8s %8s %8s %8s %-9s %-9s %6s  %s\n",
		"mode", "pages", "variant", "dpi", "score", "mean", "midpage", "textsim", "jaccard", "simhash", "verdict", "simverd", "ms", "layers")

	for _, hm := range matrixHashModes {
		for _, n := range pageCounts {
			base := filepath.Join(dir, fmt.Sprintf("base_%d.pdf", n))
			if err := writeMatrixPDF(base, n, variantSame, true); err != nil {
				t.Fatal(err)
			}
			mid := n / 2
			baseText := strings.Join(matrixPageLines(n, mid, variantSame), "\n")
			for _, v := range variants {
				varText := strings.Join(matrixPageLines(n, mid, v), "\n")
				jaccard := exactJaccard(baseText, varText)
				simSim := simHashSimilarity(baseText, varText)
				other := filepath.Join(dir, fmt.Sprintf("%s_%d.pdf", v, n))
				if err := writeMatrixPDF(other, n, v, false); err != nil {
					t.Fatal(err)
				}
				for _, dpi := range dpis {
					opts := append([]compare.Option{
						compare.WithDPI(dpi),
						compare.WithThreshold(threshold),
						compare.WithWorkers(4),
					}, hm.opts...)
					cmp, err := compare.New(opts...)
					if err != nil {
						t.Fatal(err)
					}
					res, err := cmp.Compare(ctx, base, other)
					cmp.Close()
					if err != nil {
						t.Fatalf("%s %d %s %d: %v", hm.name, n, v, dpi, err)
					}

					midSim, textSim := "-", "-"
					for _, pr := range res.PageResults {
						if pr.Page == mid+1 {
							midSim = fmt.Sprintf("%.4f", pr.Similarity)
							if pr.TextSimilarity != nil {
								textSim = fmt.Sprintf("%.4f", *pr.TextSimilarity)
							}
						}
					}
					verdict := "similar"
					if !res.Identical && res.SimilarityScore < threshold {
						verdict = "DIFFERENT"
					}

					// SimHash in MinHash's place. With text hashing off, each page
					// scores its visual similarity alone; the text cap only touches
					// the middle page, the one page whose text differs. The verdict
					// does not depend on the sampling path, since any page below the
					// threshold makes the documents different.
					key := fmt.Sprintf("%d/%s/%d", n, v, dpi)
					simVerdict := "-"
					if hm.name == "off" {
						offScores[key] = res.SimilarityScore
					} else if off, ok := offScores[key]; ok && !res.Identical {
						score := off
						if v != variantSame {
							score = math.Min(off, simSim)
						}
						simVerdict = "similar"
						if score < threshold {
							simVerdict = "DIFFERENT"
						}
					}

					fmt.Printf("%-8s %-6d %-8s %-4d %8.4f %8.4f %8s %8s %8.4f %8.4f %-9s %-9s %6d  %s\n",
						hm.name, n, v, dpi, res.SimilarityScore, res.VisualComparison.OverallScore,
						midSim, textSim, jaccard, simSim, verdict, simVerdict, res.Duration.Milliseconds(), strings.Join(res.ComparisonLayers, ","))
				}
			}
		}
	}
}
