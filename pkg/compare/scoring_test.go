package compare

import (
	"context"
	"math"
	"testing"

	"github.com/asaf-shitrit/pdfcmp/pkg/render"
)

func ptr(f float64) *float64 { return &f }

func TestTextAdjust(t *testing.T) {
	text := func(h1, h2 uint64) pageInfoPair {
		return pageInfoPair{info1: render.PageInfo{TextHash: h1}, info2: render.PageInfo{TextHash: h2}, ok: true}
	}
	for _, tc := range []struct {
		name    string
		visual  float64
		info    pageInfoPair
		textSim *float64
		want    float64
	}{
		{"no page info", 0.8, pageInfoPair{}, nil, 0.8},
		{"one page without text", 0.8, text(1, 0), nil, 0.8},
		{"same text lifts a close score", 0.92, text(1, 1), nil, 0.99},
		{"same text keeps a higher score", 0.995, text(1, 1), nil, 0.995},
		{"same text leaves a distant score", 0.85, text(1, 1), nil, 0.85},
		{"different text costs 5%", 1.0, text(1, 2), nil, 0.95},
		{"text similarity caps the score", 1.0, text(1, 2), ptr(0.3), 0.3},
		{"a higher text similarity does not lift it", 0.8, text(1, 2), ptr(0.9), 0.76},
	} {
		if got := textAdjust(tc.visual, tc.info, tc.textSim); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: textAdjust(%v) = %v, want %v", tc.name, tc.visual, got, tc.want)
		}
	}
}

func TestNeedsFullScan(t *testing.T) {
	pages := func(sims ...float64) []PageResult {
		r := make([]PageResult, len(sims))
		for i, s := range sims {
			r[i].Similarity = s
		}
		return r
	}
	auto := &comparer{opts: DefaultOptions()}
	strategic := &comparer{opts: DefaultOptions()}
	strategic.opts.Sampling = SamplingStrategic

	for _, tc := range []struct {
		name      string
		c         *comparer
		sample    []PageResult
		pageCount int
		want      bool
	}{
		{"similar sample of a longer document", auto, pages(1, 0.99, 0.97), 10, true},
		// The mean (0.9667) is above the threshold; the worst page is not.
		{"one sampled page below the threshold", auto, pages(1, 1, 0.9), 10, false},
		{"sample already covers every page", auto, pages(1, 1, 1), 3, false},
		{"strategic sampling never scans further", strategic, pages(1, 1, 1), 10, false},
	} {
		if got := tc.c.needsFullScan(tc.sample, tc.pageCount); got != tc.want {
			t.Errorf("%s: needsFullScan = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// One thumbnail scoring 0 ends the comparison at the thumbnail stage, even
// though the mean of both thumbnails (0.5) is not below the early-exit
// threshold.
func TestThumbnailEarlyExitOnWorstPage(t *testing.T) {
	c := &comparer{opts: DefaultOptions()}
	c.opts.Workers = 2
	doc1 := &fakeDoc{pages: 20}
	doc2 := &fakeDoc{pages: 20, blank: map[int]bool{0: true}}

	result := &Result{}
	done, err := c.thumbnailStage(context.Background(), result, doc1, doc2)
	if err != nil {
		t.Fatal(err)
	}
	if !done || result.VisualComparison.SamplingStrategy != "thumbnail (early exit)" {
		t.Fatalf("done = %v, strategy %q; want an early exit", done, result.VisualComparison.SamplingStrategy)
	}
	if result.SimilarityScore >= ThumbnailEarlyExitThresh || result.VisualComparison.OverallScore < ThumbnailEarlyExitThresh {
		t.Errorf("score %.3f, mean %.3f; want the worst page below %.1f and the mean not",
			result.SimilarityScore, result.VisualComparison.OverallScore, ThumbnailEarlyExitThresh)
	}
}
