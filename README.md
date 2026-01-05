# pdfcmp 📄🔍

[![Go Reference](https://pkg.go.dev/badge/github.com/asafshitrit/pdfcmp.svg)](https://pkg.go.dev/github.com/asafshitrit/pdfcmp)
[![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build Status](https://github.com/asaf-shitrit/pdfcmp/actions/workflows/go.yml/badge.svg)](https://github.com/asaf-shitrit/pdfcmp/actions)

**pdfcmp** is a high-performance, multi-layered PDF comparison tool and Go library. It efficiently detects whether two PDF files are identical or visually similar by using a fail-fast pipeline that combines lightning-fast byte hashing with sophisticated perceptual visual analysis.

---

## 🚀 Key Features

- **Blazing Fast**: Process **60+ pages per second** on modern hardware.
- **Fail-Fast Pipeline**:
    1. **Metadata**: Rejects files with different page counts or extreme size differences in $<1$ms.
    2. **Byte-wise**: Detects binary-identical files instantly using **xxHash64** (~9 GB/s).
    3. **Visual Thumbnails**: Renders key pages at low resolution to skip obviously different documents.
    4. **Smart Sampling**: For large docs, analyzes a strategic subset ($\sqrt{n} + 2$) to predict similarity.
    5. **Full Visual Analysis**: Performs deep analysis only when necessary.
- **Perceptual Hashing**: Uses **dHash** and **pHash** to detect visual similarity regardless of internal PDF optimization or compression.
- **Wasm-Powered Rendering**: Utilizes a high-performance **PDFium** engine compiled to WebAssembly for reliable, cross-platform rendering.
- **Concurrency**: Parallel architectures maximize CPU utilization by overlapping rendering and analysis tasks.

---

## 📦 Installation

### CLI Tool
```bash
# Install directly via Go
go install github.com/asafshitrit/pdfcmp/cmd/pdfcmp@latest
```

### From Source
```bash
git clone https://github.com/asaf-shitrit/pdfcmp.git
cd pdfcmp
make build
# (Optional) Move to your bin path
mv pdfcmp /usr/local/bin/
```

---

## 🛠️ CLI Usage

### Basic Comparison
Compare two PDF files for visual similarity:
```bash
pdfcmp compare file1.pdf file2.pdf
```
*Exits with code `0` if similar, `1` if different.*

### Advanced Options
```bash
# Set custom similarity threshold (0.0 to 1.0)
pdfcmp compare a.pdf b.pdf --threshold 0.99

# Force analysis of every single page (ignores sampling)
pdfcmp compare a.pdf b.pdf --sampling all

# Output detailed results as JSON
pdfcmp compare a.pdf b.pdf --format json

# Verbose mode with real-time progress
pdfcmp compare a.pdf b.pdf -v
```

### Quick Byte-wise Check
Only check if files are bit-for-bit identical:
```bash
pdfcmp quick file1.pdf file2.pdf
```

---

## 📚 Library Usage (Go)

You can easily integrate `pdfcmp` into your own Go projects.

```go
import (
    "context"
    "fmt"
    "github.com/asafshitrit/pdfcmp/pkg/compare"
)

func main() {
    ctx := context.Background()
    
    // Initialize comparer with options
    cmp, _ := compare.New(
        compare.WithThreshold(0.98),
        compare.WithDPI(150),
    )
    defer cmp.Close()

    // Run comparison
    result, err := cmp.Compare(ctx, "a.pdf", "b.pdf")
    if err != nil {
        panic(err)
    }

    if result.Similar {
        fmt.Printf("Visual Match! Score: %.2f%%\n", result.SimilarityScore * 100)
    } else {
        fmt.Println("Documents are different.")
    }
}
```

---

## ⚡ Benchmarks

*Tested on Apple M2 Pro (12 cores, 32GB RAM)*

| Scenario | Mode | Execution Time | Speed |
| :--- | :--- | :--- | :--- |
| **Identical Documents** | Bytewise | ~1.5ms | > 8 GB/s |
| **Medium PDF (10pgs)** | Visual | ~240ms | 41 pages/sec |
| **Large PDF (100pgs)** | Visual | ~1.6s | **62 pages/sec** |

---

## 🧠 Comparison Pipeline

1. **Layer 1 (Metadata)**: Instant rejection based on page count.
2. **Layer 2 (Bytewise)**: xxHash64 check for binary identity.
3. **Layer 3 (Thumbnails)**: Renders first and last page at 72 DPI.
4. **Layer 4 (Sampling)**: Stratified sampling of the entire document.
5. **Layer 5 (Deep Scan)**: Full document rendering and perceptual verification.

---

## 👨‍💻 Development

### Prerequisites
- Go 1.24+
- Make

### Useful Commands
```bash
make test       # Run unit tests
make bench      # Run performance benchmarks
make coverage   # Generate test coverage report
make lint       # Run golangci-lint
```

---

## 📄 License
MIT License - Copyright (c) 2026 Asaf Shitrit. See [LICENSE](LICENSE) for details.
