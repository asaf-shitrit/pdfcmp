# pdfcmp

`pdfcmp` is a lightning-fast PDF comparison engine and Go library. It utilizes a layered pipeline—combining byte-level hashing, text similarity, and perceptual visual comparison—to detect document similarities with maximum efficiency and accuracy.

[![Go Reference](https://pkg.go.dev/badge/github.com/asafshitrit/pdfcmp.svg)](https://pkg.go.dev/github.com/asafshitrit/pdfcmp)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## How it works

The tool follows a layered approach to avoid heavy rendering whenever possible. It includes new **Smart Heuristics** to speed up decision making:

1.  **Metadata**: Checks page counts and basic file attributes.
2.  **Byte-wise**: Runs an xxHash64 check to catch identical files instantly (~9 GB/s).
3.  **Visual Thumbnails**:
    *   **Histogram Pre-check**: Calculates RGB histograms to reject major color differences instantly (2x faster than hashing).
    *   **Low-Res Render**: Renders first and last pages at 72 DPI for perceptual analysis.
4.  **Text Fingerprinting**: Extracts each page's text and compares its hash. Identical text content with similar layout boosts confidence significantly.
5.  **Text Similarity**: When the text differs, a MinHash signature of its word 3-shingles estimates how much of it the two pages share, and that caps the page's score. Perceptual hashes see where text sits, not what it says, so without this a page rewritten in the same layout still scores 81-89% visually similar.
6.  **Strategic Sampling**: For long documents, it analyzes a subset of pages ($\sqrt{n} + 2$) to estimate similarity.
7.  **Full Analysis**: If needed, it performs a complete page-by-page comparison using perceptual hashing (dHash and pHash).

A document is only as similar as its least similar page: the similarity score is the lowest page score, so one changed page makes a 100-page document different. The JSON output keeps the mean as `visual_comparison.overall_score`.

Rendering is powered by **PDFium** via WebAssembly, and the analysis is fully concurrent to make the most of multi-core CPUs.

## Installation

### CLI
```bash
go install github.com/asafshitrit/pdfcmp/cmd/pdfcmp@latest
```

### From source
```bash
git clone https://github.com/asaf-shitrit/pdfcmp.git
cd pdfcmp
make build
```

## Usage

### CLI
The simplest way to compare two files:
```bash
pdfcmp compare file1.pdf file2.pdf
```
*Exit code is `0` if similar, `1` if different.*

**Common flags:**
- `--threshold 0.98`: Adjust similarity sensitivity (default is 0.95).
- `--sampling all`: Scan every page instead of using the sampling heuristic.
- `--text-hash off`: Compare page text by exact hash only, as v0.1.0 did.
- `--format json`: Get detailed results in JSON format.
- `-v`: Show progress and stage information.

To only check if files are bit-to-bit identical:
```bash
pdfcmp quick file1.pdf file2.pdf
```

### Library
```go
import (
    "context"
    "fmt"
    "github.com/asafshitrit/pdfcmp/pkg/compare"
)

func main() {
    cmp, _ := compare.New(compare.WithThreshold(0.98))
    defer cmp.Close()

    result, _ := cmp.Compare(context.Background(), "a.pdf", "b.pdf")
    if result.Similar {
        fmt.Printf("Match! Score: %.2f%%\n", result.SimilarityScore * 100)
    }
}
```

## Performance

Benchmarks run on Apple M2 Pro with various PDF types (single-page, small 4-page, medium 10-page, large 30-page, and extra-large documents):

### Fast-Path Optimizations
| Scenario | Time | Notes |
| :--- | :--- | :--- |
| **Identical files (self-compare)** | 3.9μs | Text-first optimization bypasses rendering |
| **Small identical (4 pages)** | 4.6μs | Early exit after metadata check |
| **Byte-only mode** | 8.0μs | xxHash64 at ~8 GB/s |

### Full Visual Comparison
| Document Size | Pages | Time | Throughput |
| :--- | :--- | :--- | :--- |
| **Small** | 4 | 129ms | 31 pages/sec |
| **Medium** | 10 | 511ms | 20 pages/sec |
| **Large** | 30 | 1.54s | **74 pages/sec** |

### Sampling Strategies (10-page document)
| Strategy | Time | Speedup |
| :--- | :--- | :--- |
| **Thumbnail only** (72 DPI) | 15ms | 22x faster |
| **Strategic** (√n + 2 pages) | 85ms | 4x faster |
| **Full** (all pages) | 338ms | baseline |

### Hash Algorithm Performance
| Algorithm | Time (medium doc) | Use Case |
| :--- | :--- | :--- |
| **Histogram** | <1ms | Color pre-filter (instant rejection) |
| **DHash** | 89ms | Structural comparison |
| **PHash** | 79ms | Frequency-based (DCT) |
| **Both** | 113ms | Combined approach (best accuracy) |

### DPI Impact (single page render + hash)
| DPI | Time | Quality |
| :--- | :--- | :--- |
| **72** | 4.2ms | Thumbnail (good for hashing) |
| **150** | 16.5ms | Standard (4x slower) |
| **300** | 46.8ms | High-res (11x slower) |

## Accuracy & Robustness

The tool combines **dHash** (structure), **pHash** (frequency), and **histogram** analysis to remain robust against various degradations:

| Scenario | DHash | PHash | Combined | Histogram | Result |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Identical** | 100% | 100% | 100% | 100% | ✅ Pass |
| **Shift (1px)** | 100% | 96.9% | 98.4% | 98.6% | ✅ Pass |
| **Shift (5px)** | 98.4% | 85.9% | 92.2% | 98.7% | ✅ Pass |
| **Resize (90%)** | 100% | 96.9% | 98.4% | 94.7% | ✅ Pass |
| **Noise (5%)** | 82.8% | 93.8% | 88.3% | 98.7% | ✅ Pass |
| **Brightness (+10%)** | 100% | 96.9% | 98.4% | 95.8% | ✅ Pass |
| **Crop (center)** | 98.4% | 87.5% | 93.0% | 98.5% | ✅ Pass |

**Key findings:**
- **DHash** excels at detecting structural changes (shifts, brightness)
- **PHash** better handles noise and compression artifacts
- **Histogram** is excellent for quick color-based rejection
- **Combined approach** provides best overall accuracy

### Text changes

Perceptual hashes are built to survive the edits above, which is also why they miss a change of words. `bench/content_matrix_test.go` builds text PDFs of 1, 5, 10, 30 and 100 pages, 25 lines a page, changes only the middle page, and compares at 72, 150 and 300 DPI. The table counts how many of those 15 comparisons each version calls different at the default 0.95 threshold:

| Middle page change | v0.1.0 | text hash off | MinHash (default) | SimHash (not shipped) |
| :--- | :---: | :---: | :---: | :---: |
| None (rebuilt, new bytes) | 0/15 | 0/15 | 0/15 | 0/15 |
| Every line rewritten | 3/15 | 15/15 | 15/15 | 15/15 |
| 12 of 25 lines rewritten | | 15/15 | 15/15 | 15/15 |
| One line rewritten | | 11/15 | 15/15 | 13/15 |
| One digit of a total | 2/15 | 3/15 | 3/15 | 3/15 |

v0.1.0 averaged page scores, so one rewritten page of five or more passed. Scoring by the worst page fixes that on its own, but only by margin: a rewritten page still scores 0.81-0.89 on its perceptual hashes, so a threshold of 0.80 would pass it again. MinHash scores it below 0.01. A single changed number moves 3 of a page's ~230 shingles, an exact Jaccard similarity of 0.975, and both text hashes put it at 0.95 or above, so it passes the default threshold in every mode.

pdfcmp uses MinHash, not SimHash, because SimHash counts repeated wording where MinHash sees a set. On a templated page, where every line repeats a label and only the values differ, a shingle repeated ten times outvotes the ten changed values. Five such pages with all ten values changed have an exact Jaccard similarity of 0.36-0.38; MinHash reads 0.31-0.41 and SimHash 0.96-0.99. SimHash's 8-byte fingerprint would only pay for itself if pdfcmp stored signatures, which it does not. `bench/simhash_test.go` keeps it for the comparison above.

Run the matrix with:

```bash
PDFCMP_MATRIX=1 go test ./bench -run 'TestContentMatrix|TestTemplatedPage' -v
```

### Test Coverage

Benchmarks include diverse PDF types to ensure robustness:
- **Single-page**: Minimal overhead testing
- **Small (4 pages)**: Quick documents with text and graphics
- **Medium (10 pages)**: Standard reports and documents
- **Large (30 pages)**: Complex multi-page documents
- **Extra-large**: Stress testing with high page counts
- **Graphic-heavy**: Image-rich presentations
- **Mixed content**: Combined text, images, and vector graphics
- **Scanned documents**: OCR'd documents with image-based pages

## Development

- **Tests**: `make test`. Tests write the PDFs they need from `internal/fixtures`.
- **Benchmarks**: `make fixtures`, then `make bench`.
- **Linting**: `make lint`
- **CRAP gate**: `make crap` fails when a function's CRAP score (complexity² × (1 − coverage)³ + complexity) is above 30. `make hooks` runs it before every commit.

`go test -race` reports a data race inside wazero when two documents render at once ([wazero/wazero#2520](https://github.com/wazero/wazero/issues/2520)): instances that share a compiled module memoize function-type keys without synchronization.

## License
MIT - Copyright (c) 2026 Asaf Shitrit.
