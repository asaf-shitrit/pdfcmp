# pdfcmp

`pdfcmp` is a lightning-fast PDF comparison engine and Go library. It utilizes a layered pipeline—combining byte-level hashing, text similarity, and perceptual visual comparison—to detect document similarities with maximum efficiency and accuracy.

[![Go Reference](https://pkg.go.dev/badge/github.com/asaf-shitrit/pdfcmp.svg)](https://pkg.go.dev/github.com/asaf-shitrit/pdfcmp)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## How it works

The tool follows a layered approach to avoid heavy rendering whenever possible. It includes new **Smart Heuristics** to speed up decision making:

1.  **Metadata**: Checks page counts and basic file attributes. When the page counts differ, the pages are aligned by their text (pages without text by their object count and content hash): `deleted_pages` lists the first file's pages that have no counterpart, `inserted_pages` the second file's, and each aligned pair is compared as usual, with its number in the second file in `page2`. The documents are different either way.
2.  **Byte-wise**: Runs an xxHash64 check to catch identical files instantly (~9 GB/s).
3.  **Visual Thumbnails**:
    *   **Histogram Pre-check**: Calculates RGB histograms to reject major color differences instantly (2x faster than hashing).
    *   **Low-Res Render**: Renders first and last pages at 72 DPI for perceptual analysis.
4.  **Text Fingerprinting**: Extracts each page's text and compares its hash. Identical text content with similar layout boosts confidence significantly.
5.  **Text Similarity**: When the text differs, a MinHash signature of its word 3-shingles estimates how much of it the two pages share, and that caps the page's score. Perceptual hashes see where text sits, not what it says, so without this a page rewritten in the same layout still scores 81-89% visually similar. When both pages have fewer than 60 distinct shingles, about six lines, they keep their visual score: on short text a single changed word would move the estimate by more than 10%.
6.  **Strategic Sampling**: For long documents, it analyzes a subset of pages ($\sqrt{n} + 2$) to estimate similarity.
7.  **Full Analysis**: If needed, it performs a complete page-by-page comparison using perceptual hashing (dHash and pHash).
8.  **Tiles**: Every rendered page is also cut into a 64x90 grid of tiles (about 1.5% of the page wide) and the tiles' mean grey levels are compared. A whole-page hash reduces the page to 9x8 or 32x32 samples, so a 120x60 px stamp on a scanned page left the score at 0.992; the stamp moves one tile by tens of grey levels. If the most changed tile moves by more than 6 grey levels, the page is different (`tile_changed` in the JSON), however high its score: like a text change, a tile change decides the verdict and leaves the score alone. A tile averages thousands of pixels, so a rescan at another JPEG quality moves none by more than about 2 levels. `tile_similarity` is `1 - d/120` for the worst tile's difference `d`, 0.95 at the tolerance, and `worst_tile` says where. A page whose text changed is left to the text rule. Scan pages (no text on either side) are compared after undoing the shift, lean and exposure of a rescan, so a second scan of the same paper passes while a 6 px dot or a 2 px line 30 px long still fails. `--no-tiles` turns this off.

A document is only as similar as its least similar page: the similarity score is the lowest page score, so one changed page makes a 100-page document different. The JSON output keeps the mean as `visual_comparison.overall_score`.

Rendering is powered by **PDFium** via WebAssembly, and the analysis is fully concurrent to make the most of multi-core CPUs.

## Installation

### CLI
```bash
go install github.com/asaf-shitrit/pdfcmp/cmd/pdfcmp@latest
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
*Exit code is `0` if similar, `1` if different and `2` if the files could not be compared (a missing file, a file that is not a PDF, a bad flag, a timeout), as with `cmp` and `diff`. Any change to a page's extracted text counts as different, even a single digit; the similarity score still shows how much changed.*

**Common flags:**
- `--threshold 0.98`: Adjust similarity sensitivity (default is 0.95).
- `--sampling all`: Scan every page instead of using the sampling heuristic.
- `--no-tiles`: Judge pages by their perceptual hashes and text alone, without the worst-tile check. In Go: `compare.WithTiles(false)`.
- `--text-tolerant`: Let the threshold decide when a page's text changed, instead of calling the files different. In Go: `compare.WithTextChange(compare.TextChangeScored)`.
- `--text-hash off`: Compare page text by exact hash only, as v0.1.0 did.
- `--format json`: Get detailed results in JSON format. `status` (`IDENTICAL`, `SIMILAR`, `DIFFERENT`) and `similar` give the verdict the exit code follows; `threshold`, `text_change_mode` (`fails` or `scored`) and `tiles` give the settings it was made with.
- `--timeout 2m`: Give up, with exit code `2`, if the comparison takes longer.
- `--memory 512`: Memory budget in MB for page renders (default 512). Fewer pages render at once to stay within it; the PDF engine's own memory comes on top.
- `-v`: Show progress and stage information.
- `pdfcmp --version`: Print the version.

To only check if files are bit-to-bit identical (exit code `0` if they are, `1` if not):
```bash
pdfcmp quick file1.pdf file2.pdf
```

### Library
```go
import (
    "context"
    "fmt"
    "github.com/asaf-shitrit/pdfcmp/pkg/compare"
)

func main() {
    cmp, _ := compare.New(compare.WithThreshold(0.98))
    defer cmp.Close()

    result, _ := cmp.Compare(context.Background(), "a.pdf", "b.pdf")
    if result.IsSimilar(0.98) {
        fmt.Printf("Match! Score: %.2f%%\n", result.SimilarityScore*100)
    } else {
        fmt.Println("Pages that changed:", result.DifferentPages())
    }
}
```

A `Comparer` is safe for concurrent use, and so are comparers used side by side: they share one pool of PDFium instances, sized to the CPU count (4 to 16), and comparisons wait for a free pair of instances instead of failing. Memory use depends on the worker count and DPI, not on page count.

## Performance

Measured on an Apple M2 Pro (10 cores, Go 1.27 on darwin/arm64) with:

```bash
make bench    # generates bench/fixtures/*.pdf, then go test -bench=. -benchmem ./bench/...
```

The fixtures come from `internal/fixtures`: small is 5 pages, medium 20, large 50, extra-large 100. The machine was shared with other jobs (1-minute load 4 to 7 on 10 cores, with spikes during some runs), so treat the numbers as rough. Medium and large varied by about 10% between runs, and the extra-large run by more.

### Without rendering
These pairs never reach the renderer. `bench/` names them `ByteIdentical`, `ByteOnly` and `PlanOnly`.

| Scenario | Time | What runs |
| :--- | :--- | :--- |
| **Same file twice** (small or extra-large) | 6μs | Byte hash only |
| **Byte-only mode** (5 pages, same text, different bytes) | 39μs | xxHash64 of both files |
| **Same text, different bytes** (5 pages) | 11ms | Page info and content hash for each page; every page scores 1.0 |
| **Same text, different bytes** (20 pages) | 32ms | Same |
| **Same text, different bytes** (50 pages) | 62ms | Same |

### Full visual comparison
Every page differs between the two files (`*_a` against `*_changed`), with `SamplingAll`, so every page is rendered at 150 DPI and hashed. The benchmark fails if any page scores 1.0 without a render.

| Document Size | Pages | Time | Throughput |
| :--- | :--- | :--- | :--- |
| **Small** | 5 | 72ms | 70 pages/sec |
| **Medium** | 20 | 216ms | 93 pages/sec |
| **Large** | 50 | 455ms | 110 pages/sec |
| **Extra-large** | 100 | 0.9 to 1.2s | 80 to 110 pages/sec |

### Sampling Strategies (20-page document)
`medium_a` against `medium_changed`, where every page differs.

| Strategy | Time | Speedup |
| :--- | :--- | :--- |
| **Thumbnail only** (72 DPI) | 17ms | 14x faster |
| **Strategic** (√n + 2 pages) | 71ms | 3.4x faster |
| **Full** (all pages) | 240ms | baseline |

### Hash Algorithm Performance
Hashing one rendered medium-size page image:

| Algorithm | Time | Use Case |
| :--- | :--- | :--- |
| **Histogram** | 0.69ms | Color pre-filter (instant rejection) |
| **DHash** | 0.82ms | Structural comparison |
| **PHash** | 0.84ms | Frequency-based (DCT) |
| **Both** | 1.7ms | Combined approach (best accuracy) |

### DPI Impact (single page render)
| DPI | Time | Quality |
| :--- | :--- | :--- |
| **72** | 0.8ms | Thumbnail (good for hashing) |
| **150** | 2.2ms | Standard (2.8x slower) |
| **300** | 12.7ms | High-res (16x slower) |

## Accuracy & Robustness

### Real documents

`bench/realworld` labels pairs built from real PDFs: IRS forms and arXiv
papers with their real revisions, long manuals, and web PDFs and scans from the
Hugging Face datasets `pixparse/pdfa-eng-wds` and `pixparse/idl-wds`. Each is
re-saved, rasterised or re-scanned (same), or edited by a digit, a word, a pen
tick, a stamp, a photo patch or a page (different). On 1,007 held-out pairs,
from shards never used while tuning pdfcmp, with the binary frozen before the
run:

| Tool | Accuracy | Changes caught | Unchanged called different |
| :--- | :---: | :---: | :---: |
| **pdfcmp** | **93.0%** | 97.1% | **12.5%** |
| red6 PDFCompare | 84.1% | 99.8% | 37.4% |
| pdf-visual-diff | 83.8% | 96.9% | 34.1% |
| diff-pdf | 82.1% | 100.0% | 42.4% |
| diff-pdf-visually | 81.2% | 100.0% | 44.5% |
| pdf-diff | 75.2% | 68.4% | 15.5% |
| `pdftotext` + `diff` | 75.2% | 68.4% | 15.5% |
| ImageMagick `compare` | 56.2% | 41.1% | 23.1% |

Pixel tools catch nearly every edit by calling 34-45% of the unchanged
documents different; text tools miss every change to a scan, a photo or a
drawing. Known misses and false alarms, such as two scans of the same paper
called different, are tracked as
[issues](https://github.com/asaf-shitrit/pdfcmp/issues?q=is%3Aissue+label%3Abug).
The comparison covers free, open-source tools;
commercial ones (Adobe Acrobat Compare, Draftable, i-net PDFC) were not
run. See [bench/realworld/README.md](bench/realworld/README.md)
for the data, results by kind of change, timings and how to run it.

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

v0.1.0 averaged page scores, so one rewritten page of five or more passed. Scoring by the worst page fixes that on its own, but only by margin: a rewritten page still scores 0.81-0.89 on its perceptual hashes, so a threshold of 0.80 would pass it again. MinHash scores it below 0.01. A single changed number moves 3 of a page's ~230 shingles, an exact Jaccard similarity of 0.975, and both text hashes put it at 0.95 or above, so it passes the default threshold in every mode. The verdict still catches it: by default a change to the characters of a page's extracted text makes the page different (`text_changed`), and `--text-tolerant` leaves it to the threshold. The text is normalised first (NFKC, soft hyphens and zero-width characters dropped, whitespace runs collapsed), and the same letters and digits in another order are the same text, so a re-export that reorders text runs or moves spaces is not a change (a separator between two digits or an accented letter exchanged for another, as in "1,500" for "1.500" or "Müller" for "Möller", is one); text that moved on the page is the render's to judge. A page without text, such as a scan, against the same page with text is judged by its render, as is a text change that changes no pixel, such as hidden text.

pdfcmp uses MinHash, not SimHash, because SimHash counts repeated wording where MinHash sees a set. On a templated page, where every line repeats a label and only the values differ, a shingle repeated ten times outvotes the ten changed values. Five such pages with all ten values changed have an exact Jaccard similarity of 0.36-0.38; MinHash reads 0.31-0.41 and SimHash 0.96-0.99. SimHash's 8-byte fingerprint would only pay for itself if pdfcmp stored signatures, which it does not. `bench/simhash_test.go` keeps it for the comparison above.

Run the matrix with:

```bash
PDFCMP_MATRIX=1 go test ./bench -run 'TestContentMatrix|TestTemplatedPage' -v
```

### Test Coverage

Benchmarks include diverse PDF types to ensure robustness:
- **Single-page**: Minimal overhead testing
- **Small (5 pages)**: Quick documents with text and graphics
- **Medium (20 pages)**: Standard reports and documents
- **Large (50 pages)**: Complex multi-page documents
- **Extra-large (100 pages)**: Stress testing with high page counts
- **Graphic-heavy**: Image-rich presentations
- **Mixed content**: Combined text, images, and vector graphics
- **Scanned documents**: OCR'd documents with image-based pages

## Development

- **Tests**: `make test`. Tests write the PDFs they need from `internal/fixtures`.
- **Benchmarks**: `make bench` generates the fixtures first. A PDF benchmark fails when its fixture is missing; set `PDFCMP_BENCH_SKIP_MISSING=1` to skip instead.
- **Linting**: `make lint`
- **CRAP gate**: `make crap` fails when a function's CRAP score (complexity² × (1 − coverage)³ + complexity) is above 30. `make hooks` runs it before every commit.

## License
MIT - Copyright (c) 2026 Asaf Shitrit.
