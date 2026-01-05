# pdfcmp

`pdfcmp` is a high-performance tool and Go library for comparing PDF files. It uses a multi-stage pipeline to detect whether two documents are byte-identical or visually similar, optimizing for speed by failing as early as possible.

[![Go Reference](https://pkg.go.dev/badge/github.com/asafshitrit/pdfcmp.svg)](https://pkg.go.dev/github.com/asafshitrit/pdfcmp)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## How it works

The tool follows a layered approach to avoid heavy rendering whenever possible:

1.  **Metadata**: Checks page counts and basic file attributes.
2.  **Byte-wise**: Runs an xxHash64 check to catch identical files instantly (~9 GB/s).
3.  **Visual Thumbnails**: Renders the first and last pages at low resolution to detect obvious mismatches.
4.  **Strategic Sampling**: For long documents, it analyzes a subset of pages ($\sqrt{n} + 2$) to estimate similarity without scanning the whole file.
5.  **Full Analysis**: If needed, it performs a complete page-by-page comparison using perceptual hashing (dHash and pHash).

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

Benchmarks run on an Apple M2 Pro:

| Scenario | Mode | Execution Time |
| :--- | :--- | :--- |
| **Identical Docs** | Bytewise | ~1.5ms |
| **10-page PDF** | Visual | ~240ms |
| **100-page PDF** | Visual | ~1.6s (~62 pgs/sec) |

## Development

- **Tests**: `make test`
- **Benchmarks**: `make bench`
- **Linting**: `make lint`

## License
MIT - Copyright (c) 2026 Asaf Shitrit.
