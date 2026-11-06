package compare

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/asaf-shitrit/pdfcmp/pkg/visual"
)

// Result represents the complete comparison result
type Result struct {
	// Identical is true if PDFs are byte-for-byte identical
	Identical bool `json:"identical"`

	// SimilarityScore is the similarity of the least similar compared page
	// (0.0 to 1.0). One changed page makes the document different, however
	// many unchanged pages surround it.
	SimilarityScore float64 `json:"similarity_score"`

	// ByteComparison contains byte-wise comparison results
	ByteComparison ByteResult `json:"byte_comparison"`

	// VisualComparison contains visual comparison results
	VisualComparison VisualResult `json:"visual_comparison"`

	// PageResults contains per-page comparison results
	PageResults []PageResult `json:"page_results,omitempty"`

	// DeletedPages lists the pages of the first document that have no page
	// in the second, in the first document's numbering. Set only when the
	// documents' page counts differ.
	DeletedPages []int `json:"deleted_pages,omitempty"`

	// InsertedPages lists the pages of the second document that have no page
	// in the first, in the second document's numbering. Set only when the
	// documents' page counts differ.
	InsertedPages []int `json:"inserted_pages,omitempty"`

	// Metadata contains information about the compared files
	Metadata Metadata `json:"metadata"`

	// Duration is the total time taken for comparison
	Duration time.Duration `json:"duration_ns"`

	// ComparisonLayers indicates which comparison layers were executed
	ComparisonLayers []string `json:"comparison_layers"`

	// TextChanged is true when the extracted text of any compared page
	// differs. By default it makes IsSimilar false at any threshold.
	TextChanged bool `json:"text_changed"`

	// TileChanged is true when the worst tile of any compared page changed
	// by more than visual.TileTolerance. With tiles on it makes IsSimilar
	// false at any threshold.
	TileChanged bool `json:"tile_changed,omitempty"`

	// tiles is the comparer's Tiles option, which IsSimilar applies
	tiles bool

	// textMode is the comparer's TextChangeMode, which IsSimilar applies
	textMode TextChangeMode
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
	// OverallScore is the average similarity across all compared pages.
	// It dilutes a single changed page; SimilarityScore does not.
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
	// Page is the 1-indexed page number in the first document
	Page int `json:"page"`

	// Page2 is the page's 1-indexed number in the second document. It is
	// set when page counts differ and the pages were aligned; otherwise 0,
	// and the page has the number Page in both documents.
	Page2 int `json:"page2,omitempty"`

	// Similarity is the page's similarity score (0.0 to 1.0): the lower of
	// its visual and text similarity when both pages have text
	Similarity float64 `json:"similarity"`

	// TextSimilarity is the estimated similarity of the two pages' text,
	// set only when both pages have text and it differs
	TextSimilarity *float64 `json:"text_similarity,omitempty"`

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

	// TileSimilarity is the page's worst tile score: 1 - d/visual.TileScale
	// for the largest difference d between two tiles' mean grey levels, so
	// 0.95 is the tolerance (visual.TileTolerance). It is a diagnostic, set
	// when the page was checked on tiles; it does not enter Similarity.
	TileSimilarity *float64 `json:"tile_similarity,omitempty"`

	// WorstTile is the tile that scored TileSimilarity
	WorstTile *visual.Tile `json:"worst_tile,omitempty"`

	// TileChanged is true when the worst tile changed by more than
	// visual.TileTolerance. It makes the page different, whatever its
	// Similarity.
	TileChanged bool `json:"tile_changed,omitempty"`

	// IsDifferent is true when the page scores below the threshold or,
	// unless the comparison used TextChangeScored, its text changed, or when
	// its worst tile changed
	IsDifferent bool `json:"is_different"`

	// TextChanged is true when the two pages' extracted text differs,
	// including when only one page has text
	TextChanged bool `json:"text_changed,omitempty"`
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

// DifferentPages returns the pages that differ, in the first document's
// numbering: the changed pages, and the pages deleted from the first
// document. Pages inserted in the second document have no number there; see
// InsertedPages.
func (r *Result) DifferentPages() []int {
	pages := append(r.changedPages(), r.DeletedPages...)
	slices.Sort(pages)
	return pages
}

// changedPages returns the compared pages that differ, in the first
// document's numbering.
func (r *Result) changedPages() []int {
	var pages []int
	for _, pr := range r.PageResults {
		if pr.IsDifferent {
			pages = append(pages, pr.Page)
		}
	}
	return pages
}

// pagesAdded reports whether pages were deleted or inserted.
func (r *Result) pagesAdded() bool {
	return len(r.DeletedPages) > 0 || len(r.InsertedPages) > 0
}

// IsSimilar reports whether the documents are similar: the similarity score
// reaches the threshold and, unless the comparison used TextChangeScored, no
// page's text changed and, unless it turned tiles off, no page's tile changed.
func (r *Result) IsSimilar(threshold float64) bool {
	if r.pagesAdded() {
		return false
	}
	if r.TextChanged && r.textMode == TextChangeFails {
		return false
	}
	if r.TileChanged && r.tiles {
		return false
	}
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
	} else if r.IsSimilar(DefaultSimilarityThreshold) {
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

	summary += r.pageLines()

	summary += fmt.Sprintf("\nTime elapsed: %v", r.Duration.Round(time.Millisecond))

	return summary
}

// pageLines lists the modified, deleted and inserted pages, one line each,
// each line starting with a newline.
func (r *Result) pageLines() string {
	var lines string
	if pages := r.changedPages(); len(pages) > 0 {
		lines += fmt.Sprintf("\nModified pages: %v", pages)
		if r.pagesAdded() {
			lines += " (file 1 numbering)"
		}
	}
	if len(r.DeletedPages) > 0 {
		lines += fmt.Sprintf("\nDeleted pages (in file 1): %v", r.DeletedPages)
	}
	if len(r.InsertedPages) > 0 {
		lines += fmt.Sprintf("\nInserted pages (in file 2): %v", r.InsertedPages)
	}
	return lines
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
