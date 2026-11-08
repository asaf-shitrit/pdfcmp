package compare

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidateRejectsBadValues(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Options)
		want string
	}{
		{"NaN threshold", func(o *Options) { o.SimilarityThreshold = math.NaN() }, "similarity threshold"},
		{"+Inf threshold", func(o *Options) { o.SimilarityThreshold = math.Inf(1) }, "similarity threshold"},
		{"-Inf threshold", func(o *Options) { o.SimilarityThreshold = math.Inf(-1) }, "similarity threshold"},
		{"negative threshold", func(o *Options) { o.SimilarityThreshold = -0.1 }, "similarity threshold"},
		{"unknown mode", func(o *Options) { o.Mode = Mode(99) }, "unknown mode 99"},
		{"negative mode", func(o *Options) { o.Mode = Mode(-1) }, "unknown mode -1"},
		{"unknown hash type", func(o *Options) { o.HashType = HashType(7) }, "unknown hash type 7"},
		{"unknown sampling", func(o *Options) { o.Sampling = SamplingStrategy(9) }, "unknown sampling strategy 9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := DefaultOptions()
			tc.set(&o)
			err := o.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate() = %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestValidateAcceptsThresholdBounds(t *testing.T) {
	for _, v := range []float64{0, 0.5, 1} {
		o := DefaultOptions()
		o.SimilarityThreshold = v
		if err := o.Validate(); err != nil {
			t.Errorf("threshold %v: %v", v, err)
		}
	}
}

func TestWorkersForClampsToMaxWorkers(t *testing.T) {
	tests := []struct{ cpus, want int }{
		{0, MinWorkers},
		{1, 1},
		{8, 8},
		{MaxWorkers, MaxWorkers},
		{128, MaxWorkers},
	}
	for _, tc := range tests {
		if got := workersFor(tc.cpus); got != tc.want {
			t.Errorf("workersFor(%d) = %d, want %d", tc.cpus, got, tc.want)
		}
	}
	o := DefaultOptions()
	if err := o.Validate(); err != nil {
		t.Errorf("default options invalid: %v", err)
	}
}

func TestPageCountStageEmptyDocument(t *testing.T) {
	tests := []struct {
		name         string
		n1, n2       int
		wantDeleted  []int
		wantInserted []int
	}{
		{"first empty", 0, 3, nil, []int{1, 2, 3}},
		{"second empty", 2, 0, []int{1, 2}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &comparer{opts: DefaultOptions()}
			res := &Result{}
			if !c.pageCountStage(res, &fakeDoc{pages: tc.n1}, &fakeDoc{pages: tc.n2}) {
				t.Fatal("stage did not decide")
			}
			if !slices.Equal(res.DeletedPages, tc.wantDeleted) || !slices.Equal(res.InsertedPages, tc.wantInserted) {
				t.Errorf("deleted %v inserted %v, want %v and %v",
					res.DeletedPages, res.InsertedPages, tc.wantDeleted, tc.wantInserted)
			}
			for _, threshold := range []float64{0, 0.5, 1} {
				if res.IsSimilar(threshold) {
					t.Errorf("IsSimilar(%v) = true, want false", threshold)
				}
			}
		})
	}
}

func TestPageCountStageBothEmptyIsNotByteIdentical(t *testing.T) {
	c := &comparer{opts: DefaultOptions()}
	res := &Result{}
	if !c.pageCountStage(res, &fakeDoc{}, &fakeDoc{}) {
		t.Fatal("stage did not decide")
	}
	if res.Identical {
		t.Error("Identical = true for documents that are not byte-identical")
	}
	if !res.IsSimilar(1) {
		t.Error("two empty documents should be similar")
	}
	if strings.Contains(res.Summary(), "Status: IDENTICAL") {
		t.Errorf("summary says IDENTICAL:\n%s", res.Summary())
	}
}

func TestByteModeVerdictIgnoresThreshold(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.pdf")
	b := filepath.Join(dir, "b.pdf")
	same := filepath.Join(dir, "same.pdf")
	for path, data := range map[string]string{a: "aaa", b: "bbb", same: "aaa"} {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, threshold := range []float64{0, 0.5, 1} {
		cmp, err := New(WithMode(ModeByte), WithThreshold(threshold))
		if err != nil {
			t.Fatal(err)
		}
		res, err := cmp.Compare(context.Background(), a, b)
		if err != nil {
			t.Fatal(err)
		}
		if res.IsSimilar(threshold) {
			t.Errorf("threshold %v: differing bytes reported similar", threshold)
		}
		res, err = cmp.Compare(context.Background(), a, same)
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsSimilar(threshold) {
			t.Errorf("threshold %v: identical bytes reported different", threshold)
		}
		_ = cmp.Close()
	}
}
