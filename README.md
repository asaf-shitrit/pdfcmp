# pdfcmp

`pdfcmp` is a high-performance tool and Go library for comparing PDF files. It uses a multi-stage pipeline to detect whether two documents are byte-identical or visually similar, optimizing for speed by failing as early as possible.

[![Go Reference](https://pkg.go.dev/badge/github.com/asafshitrit/pdfcmp.svg)](https://pkg.go.dev/github.com/asafshitrit/pdfcmp)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## How it works

The tool follows a layered approach to avoid heavy rendering whenever possible. It includes new **Smart Heuristics** to speed up decision making:

1.  **Metadata**: Checks page counts and basic file attributes.
2.  **Structural Check**: Compares text character counts and graphical object counts per page to detect layout changes without rendering.
3.  **Byte-wise**: Runs an xxHash64 check to catch identical files instantly (~9 GB/s).
4.  **Visual Thumbnails**:
    *   **Histogram Pre-check**: Calculates RGB histograms to reject major color differences instantly (2x faster than hashing).
    *   **Low-Res Render**: Renders first and last pages at 72 DPI for perceptual analysis.
5.  **Text Fingerprinting**: Extracts text and compares its hash. Identical text content with similar layout boosts confidence significantly.
6.  **Strategic Sampling**: For long documents, it analyzes a subset of pages ($\sqrt{n} + 2$) to estimate similarity.
7.  **Full Analysis**: If needed, it performs a complete page-by-page comparison using perceptual hashing (dHash and pHash).

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

- **Tests**: `make test`
- **Benchmarks**: `make bench`
- **Linting**: `make lint`

## License
MIT - Copyright (c) 2026 Asaf Shitrit.
