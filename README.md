# similar-pdf

> **A high-performance, multi-layer PDF comparison tool written in Go.**

`similar-pdf` is a CLI tool and Go library designed to efficiently detect whether two PDF files are identical or visually similar. It employs a smart multi-stage pipeline to strictly minimize the amount of work required for each comparison, making it suitable for high-throughput environments.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)
![Coverage](https://img.shields.io/badge/coverage-80%25-green.svg)

## 🚀 Features

- **Extreme Performance**: Capable of processing **60+ pages per second** on modern hardware.
- **Smart Pipeline**:
  1.  **Metadata Check**: Files with different sizes/page counts are rejected immediately ($<1ms$).
  2.  **Byte-wise Check**: Uses **xxHash64** (~9 GB/s) to detect byte-identical files instantly.
  3.  **Visual Check**: Renders pages using a high-performance **WebAssembly (PDFium)** engine.
  4.  **Strategic Sampling**: For large documents, it compares a strategic subset ($\sqrt{n} + 2$) of pages first.
- **Perceptual Hashing**: Uses robust **dHash** (Difference Hash) and **pHash** (Perceptual Hash) to detect visual similarity even if internal PDF structures differ.
- **Concurrency**: Fully pipelined architecture overlaps rendering and hashing for maximum throughput.

## 📦 Installation

```bash
# Install via Go
go install github.com/asafshitrit/similar-pdf/cmd/pdfcompare@latest
```

Or build from source:

```bash
git clone https://github.com/asafshitrit/similar-pdf.git
cd similar-pdf
make install
```

## 🛠️ Usage

### Basic Comparison
Compare two files to see if they are visually similar.

```bash
pdfcompare compare file1.pdf file2.pdf
```
*Returns exit code `0` if similar, `1` if different.*

### Options
```bash
# Set custom similarity threshold (default 0.95)
pdfcompare compare a.pdf b.pdf --threshold 0.99

# Force full visual comparison (scan every page)
pdfcompare compare a.pdf b.pdf --sampling all

# Output as JSON
pdfcompare compare a.pdf b.pdf --format json

# Verbose output (show progress)
pdfcompare compare a.pdf b.pdf -v
```

### Quick Mode
If you only care about byte-for-byte identity (fastest):
```bash
pdfcompare quick file1.pdf file2.pdf
```

## ⚡ Benchmarks

Benchmarks run on an Apple M2 Pro:

| Scenario | Time | Throughput |
| :--- | :--- | :--- |
| **Identical Files** | ~1.5ms | > 8 GB/s |
| **Visual Check (Sampled)** | ~1.0s | N/A |
| **Full Visual (Large Doc)** | ~2.0s | **63 pages/sec** |

To run benchmarks yourself:
```bash
./bench/run_cli_benchmarks.sh
```

## 🧠 How It Works

The tool uses a "fail-fast" pipeline strategy:

1.  **Layer 1 (Metadata)**: If file sizes differ by a huge margin (and byte mode is on), or page counts differ, exit immediately.
2.  **Layer 2 (Bytewise)**: Hashes files with xxHash64. If hashes match, they are 100% identical. Cost: Minimal.
3.  **Layer 3 (Thumbnail)**: Renders the first and last page at low DPI. If these look totally different, stop.
4.  **Layer 4 (Sampling)**: For large docs, checks a spread of pages ($\sqrt{n} + 2$). If these match, the document is likely the same.
5.  **Layer 5 (Full)**: Only if configured or if high-confidence is needed, render and hash every single page.

## 👨‍💻 Development

### Requirements
- Go 1.21+
- Make

### Commands
```bash
make build      # Build binary
make test       # Run unit tests
make test-race  # Run tests with race detector
make bench      # Run Go benchmarks
make coverage   # Generate coverage report
```

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.
