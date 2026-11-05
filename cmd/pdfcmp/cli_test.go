package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/asafshitrit/pdfcmp/internal/fixtures"
	"github.com/asafshitrit/pdfcmp/pkg/compare"
	"github.com/jung-kurt/gofpdf"
)

// withFlags sets the compare command's flags to their defaults for one test,
// lets set override some of them, and restores the previous values after.
func withFlags(t *testing.T, set func()) {
	t.Helper()
	saved := []any{verbose, quiet, format, mode, dpi, workers, threshold, hashType, textHash, sampling, maxMemory, textTolerant}
	t.Cleanup(func() {
		verbose, quiet, format = saved[0].(bool), saved[1].(bool), saved[2].(string)
		mode, dpi, workers = saved[3].(string), saved[4].(int), saved[5].(int)
		threshold, hashType, textHash = saved[6].(float64), saved[7].(string), saved[8].(string)
		sampling, maxMemory = saved[9].(string), saved[10].(int)
		textTolerant = saved[11].(bool)
	})
	verbose, quiet, format = false, true, "text"
	mode, dpi, workers, threshold = "auto", 72, runtime.NumCPU(), 0.95
	hashType, textHash, sampling, maxMemory = "both", "minhash", "auto", 512
	textTolerant = false
	if set != nil {
		set()
	}
}

func fixture(t *testing.T, dir, name string) string {
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

func TestCompareOptions(t *testing.T) {
	withFlags(t, nil)
	opts, err := compareOptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 9 {
		t.Errorf("got %d options, want 8", len(opts))
	}

	withFlags(t, func() { verbose, quiet = true, false })
	if opts, _ := compareOptions(); len(opts) != 10 {
		t.Errorf("verbose: got %d options, want 10 with a progress callback", len(opts))
	}
}

func TestCompareOptionsRejectsUnknownValues(t *testing.T) {
	for _, tc := range []struct {
		set  func()
		want string
	}{
		{func() { mode = "fast" }, "invalid mode: fast"},
		{func() { hashType = "ahash" }, "invalid hash type: ahash"},
		{func() { textHash = "simhash" }, "invalid text hash: simhash"},
		{func() { sampling = "some" }, "invalid sampling strategy: some"},
	} {
		withFlags(t, tc.set)
		if _, err := compareOptions(); err == nil || err.Error() != tc.want {
			t.Errorf("err = %v, want %q", err, tc.want)
		}
	}
}

func TestCompareFiles(t *testing.T) {
	dir := t.TempDir()
	a := fixture(t, dir, "medium_a.pdf")
	b := fixture(t, dir, "medium_b.pdf")
	modified := fixture(t, dir, "medium_modified.pdf")
	ctx := context.Background()

	withFlags(t, func() { verbose, quiet = true, false })
	res, err := compareFiles(ctx, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsSimilar(threshold) {
		t.Errorf("same content: similarity %.4f", res.SimilarityScore)
	}

	res, err = compareFiles(ctx, a, modified)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsSimilar(threshold) {
		t.Errorf("modified page passed: similarity %.4f", res.SimilarityScore)
	}

	if _, err := compareFiles(ctx, a, filepath.Join(dir, "missing.pdf")); err == nil {
		t.Error("missing file: want an error")
	}

	withFlags(t, func() { mode = "fast" })
	if _, err := compareFiles(ctx, a, b); err == nil {
		t.Error("invalid mode: want an error")
	}

	withFlags(t, func() { dpi = 1 })
	if _, err := compareFiles(ctx, a, b); err == nil || !strings.Contains(err.Error(), "failed to create comparer") {
		t.Errorf("invalid DPI: err = %v", err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	withFlags(t, nil)
	if _, err := compareFiles(cancelled, a, modified); err == nil || !strings.Contains(err.Error(), "comparison failed") {
		t.Errorf("cancelled: err = %v", err)
	}
}

// invoiceLine is a line of ten words that no other line of the invoice shares
// a three-word run with.
func invoiceLine(i int) string {
	words := strings.Fields("amber basil cedar delta ember fjord grove harbor island jungle kettle lantern meadow nectar orchard pebble quartz ridge summit thistle upland velvet willow yonder zephyr")
	line := make([]string, 10)
	for j := range line {
		line[j] = words[(i*j*7+i*3+j*11)%len(words)] + fmt.Sprint(i)
	}
	return strings.Join(line, " ")
}

// invoice writes a one-page document of 25 lines whose line 13 is a total.
func invoice(t *testing.T, path, total string) string {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "", 11)
	for i := 1; i <= 25; i++ {
		line := invoiceLine(i)
		if i == 13 {
			line = "Total due: " + total
		}
		pdf.Cell(0, 8, line)
		pdf.Ln(9)
	}
	if err := pdf.OutputFileAndClose(path); err != nil {
		t.Fatal(err)
	}
	return path
}

// A pair that differs by one digit of text is different by default, which is
// what the exit code follows, and similar when the text change is left to the
// threshold.
func TestTextChangeVerdict(t *testing.T) {
	dir := t.TempDir()
	a := invoice(t, filepath.Join(dir, "a.pdf"), "1200.00")
	b := invoice(t, filepath.Join(dir, "b.pdf"), "1300.00")

	withFlags(t, nil)
	res, err := compareFiles(context.Background(), a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !res.TextChanged || res.IsSimilar(threshold) {
		t.Errorf("default: text_changed %v, similar %v (score %.4f), want changed and different",
			res.TextChanged, res.IsSimilar(threshold), res.SimilarityScore)
	}

	withFlags(t, func() { textTolerant = true })
	tolerant, err := compareFiles(context.Background(), a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !tolerant.TextChanged || !tolerant.IsSimilar(threshold) {
		t.Errorf("--text-tolerant: text_changed %v, similar %v (score %.4f), want changed and similar",
			tolerant.TextChanged, tolerant.IsSimilar(threshold), tolerant.SimilarityScore)
	}
	if res.SimilarityScore != tolerant.SimilarityScore {
		t.Errorf("score differs between modes: %v vs %v", res.SimilarityScore, tolerant.SimilarityScore)
	}
}

func TestWriteResult(t *testing.T) {
	res := &compare.Result{SimilarityScore: 0.5}

	withFlags(t, func() { format = "json" })
	var buf bytes.Buffer
	if err := writeResult(&buf, res); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON output does not parse: %v\n%s", err, buf.String())
	}
	if decoded["similarity_score"] != 0.5 {
		t.Errorf("similarity_score = %v, want 0.5", decoded["similarity_score"])
	}

	withFlags(t, nil)
	buf.Reset()
	if err := writeResult(&buf, res); err != nil {
		t.Fatal(err)
	}
	if buf.String() != res.Summary() {
		t.Errorf("text output is not the summary:\n%s", buf.String())
	}
}

func TestQuickCompare(t *testing.T) {
	dir := t.TempDir()
	a := fixture(t, dir, "single_a.pdf")
	diff := fixture(t, dir, "small_diff_a.pdf")

	withFlags(t, nil)
	var buf bytes.Buffer
	if err := quickCompare(&buf, a, a); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "IDENTICAL") {
		t.Errorf("same file: %q", buf.String())
	}

	buf.Reset()
	if err := quickCompare(&buf, a, diff); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "DIFFERENT") {
		t.Errorf("different files: %q", buf.String())
	}

	withFlags(t, func() { format = "json" })
	buf.Reset()
	if err := quickCompare(&buf, a, diff); err != nil {
		t.Fatal(err)
	}
	var decoded struct{ Identical bool }
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil || decoded.Identical {
		t.Errorf("JSON: %v, %+v", err, decoded)
	}

	if err := quickCompare(&buf, a, filepath.Join(dir, "missing.pdf")); err == nil {
		t.Error("missing file: want an error")
	}
}
