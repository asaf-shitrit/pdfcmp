package compare

import (
	"encoding/json"
	"fmt"
	"time"
)

// Result represents the complete comparison result
type Result struct {
	// Identical is true if PDFs are byte-for-byte identical
	Identical bool `json:"identical"`

	// SimilarityScore is the overall visual similarity (0.0 to 1.0)
	SimilarityScore float64 `json:"similarity_score"`

	// ByteComparison contains byte-wise comparison results
	ByteComparison ByteResult `json:"byte_comparison"`

	// VisualComparison contains visual comparison results
	VisualComparison VisualResult `json:"visual_comparison"`

	// PageResults contains per-page comparison results
	PageResults []PageResult `json:"page_results,omitempty"`

	// Metadata contains information about the compared files
	Metadata Metadata `json:"metadata"`

	// Duration is the total time taken for comparison
	Duration time.Duration `json:"duration_ns"`

	// ComparisonLayers indicates which comparison layers were executed
	ComparisonLayers []string `json:"comparison_layers"`
}

// ByteResult contains byte-wise comparison results
type ByteResult struct {
	// Identical is true if files have the same hash
	Identical bool `json:"identical"`

	// Hash1 is the xxHash64 of the first file
	Hash1 uint64 `json:"hash1"`

	// Hash2 is the xxHash64 of the second file
	Hash2 uint64 `json:"hash2"`

	// Size1 is the size of the first file in bytes
	Size1 int64 `json:"size1"`

	// Size2 is the size of the second file in bytes
	Size2 int64 `json:"size2"`

	// SizesMatch is true if file sizes are equal
	SizesMatch bool `json:"sizes_match"`
}

// VisualResult contains visual comparison results
type VisualResult struct {
	// OverallScore is the average similarity across all compared pages
	OverallScore float64 `json:"overall_score"`

	// HashType indicates which hash algorithm(s) were used
	HashType string `json:"hash_type"`

	// PagesCompared is the number of pages that were visually compared
	PagesCompared int `json:"pages_compared"`

	// SamplingStrategy used for page selection
	SamplingStrategy string `json:"sampling_strategy"`

	// DPI used for rendering
	DPI int `json:"dpi"`
}

// PageResult contains comparison results for a single page
type PageResult struct {
	// Page is the 1-indexed page number
	Page int `json:"page"`

	// Similarity is the visual similarity score (0.0 to 1.0)
	Similarity float64 `json:"similarity"`

	// DHashDistance is the Hamming distance for dHash
	DHashDistance int `json:"dhash_distance"`

	// PHashDistance is the Hamming distance for pHash
	PHashDistance int `json:"phash_distance"`

	// DHash1 is the dHash of page in first PDF
	DHash1 uint64 `json:"dhash1,omitempty"`

	// DHash2 is the dHash of page in second PDF
	DHash2 uint64 `json:"dhash2,omitempty"`

	// PHash1 is the pHash of page in first PDF
	PHash1 uint64 `json:"phash1,omitempty"`

	// PHash2 is the pHash of page in second PDF
	PHash2 uint64 `json:"phash2,omitempty"`

	// IsDifferent indicates if this page differs significantly
	IsDifferent bool `json:"is_different"`
}

// Metadata contains information about the compared files
type Metadata struct {
	// File1 is the path to the first PDF
	File1 string `json:"file1"`

	// File2 is the path to the second PDF
	File2 string `json:"file2"`

	// Pages1 is the page count of the first PDF
	Pages1 int `json:"pages1"`

	// Pages2 is the page count of the second PDF
	Pages2 int `json:"pages2"`

	// Size1 is the file size of the first PDF in bytes
	Size1 int64 `json:"size1"`

	// Size2 is the file size of the second PDF in bytes
	Size2 int64 `json:"size2"`

	// DurationMS is the comparison duration in milliseconds
	DurationMS int64 `json:"duration_ms"`
}

// DifferentPages returns a list of page numbers that differ significantly
func (r *Result) DifferentPages() []int {
	var pages []int
	for _, pr := range r.PageResults {
		if pr.IsDifferent {
			pages = append(pages, pr.Page)
		}
	}
	return pages
}

// IsSimilar returns true if the overall similarity exceeds the given threshold
func (r *Result) IsSimilar(threshold float64) bool {
	return r.SimilarityScore >= threshold
}

// JSON returns the result as a JSON string
func (r *Result) JSON() (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Summary returns a human-readable summary of the comparison
func (r *Result) Summary() string {
	status := "DIFFERENT"
	if r.Identical {
		status = "IDENTICAL"
	} else if r.SimilarityScore >= 0.95 {
		status = "SIMILAR"
	}

	summary := fmt.Sprintf(`PDF Comparison Result
=====================
File 1: %s (%d pages, %s)
File 2: %s (%d pages, %s)

Status: %s
Overall Similarity: %.1f%%

Byte Comparison: %s
Visual Comparison: %.1f%% similar (%d pages compared)
`,
		r.Metadata.File1, r.Metadata.Pages1, formatBytes(r.Metadata.Size1),
		r.Metadata.File2, r.Metadata.Pages2, formatBytes(r.Metadata.Size2),
		status,
		r.SimilarityScore*100,
		byteStatus(r.ByteComparison.Identical),
		r.VisualComparison.OverallScore*100,
		r.VisualComparison.PagesCompared,
	)

	// Add modified pages if any
	diffPages := r.DifferentPages()
	if len(diffPages) > 0 {
		summary += fmt.Sprintf("\nModified pages: %v", diffPages)
	}

	summary += fmt.Sprintf("\nTime elapsed: %v", r.Duration.Round(time.Millisecond))

	return summary
}

func byteStatus(identical bool) string {
	if identical {
		return "IDENTICAL"
	}
	return "DIFFERENT"
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
