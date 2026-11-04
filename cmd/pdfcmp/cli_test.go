package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/asafshitrit/pdfcmp/internal/fixtures"
	"github.com/asafshitrit/pdfcmp/pkg/compare"
)

// withFlags sets the compare command's flags to their defaults for one test,
// lets set override some of them, and restores the previous values after.
func withFlags(t *testing.T, set func()) {
	t.Helper()
	saved := []any{verbose, quiet, format, mode, dpi, workers, threshold, hashType, textHash, sampling, maxMemory}
	t.Cleanup(func() {
		verbose, quiet, format = saved[0].(bool), saved[1].(bool), saved[2].(string)
		mode, dpi, workers = saved[3].(string), saved[4].(int), saved[5].(int)
		threshold, hashType, textHash = saved[6].(float64), saved[7].(string), saved[8].(string)
		sampling, maxMemory = saved[9].(string), saved[10].(int)
	})
	verbose, quiet, format = false, true, "text"
	mode, dpi, workers, threshold = "auto", 72, runtime.NumCPU(), 0.95
	hashType, textHash, sampling, maxMemory = "both", "minhash", "auto", 512
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
	if len(opts) != 8 {
		t.Errorf("got %d options, want 8", len(opts))
	}

	withFlags(t, func() { verbose, quiet = true, false })
	if opts, _ := compareOptions(); len(opts) != 9 {
		t.Errorf("verbose: got %d options, want 9 with a progress callback", len(opts))
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
