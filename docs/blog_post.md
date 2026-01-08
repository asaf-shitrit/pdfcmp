# Why I Built pdfcmp: A Quest for Faster PDF Comparison

Comparing two PDF files sounds simple. In a perfect world, a byte-level hash
would tell you everything you need to know. But we don't live in a perfect
world. We live in a world where two documents can look identical to a human but
have different metadata, or where a single-pixel shift in a logo makes two
otherwise identical forms technically “different.”

I recently found myself needing a way to compare PDFs that was fast enough for
CI/CD pipelines and accurate enough to catch visual regressions. Existing tools
were either too slow—rendering every page at high resolutions—or too naive,
missing subtle but important changes.

This is why I built `pdfcmp`.

## The “Early Exit” Architecture

When I started this project, I realized that rendering dominates the cost of
comparison. For example, rendering a single page at 300 DPI is about 11 times
slower than rendering it at 72 DPI. My goal was to build a system that only
renders when it must, and only what is necessary.

I landed on a five-stage “Early Exit” pipeline:

1. **Identity Check**: If the bytes match, we're done in microseconds.
2. **Structural Heuristics**: I added a middle layer that counts characters and
   graphical objects. If the first page of document A has 10,000 characters and
   document B has 10,001, we don't need to render to know they've changed.
3. **Histogram Pre-filter**: This was a major “aha!” moment. Calculating an RGB
   histogram is 2x faster than perceptual hashing. If the color balance of a
   page is fundamentally different, we reject it instantly.
4. **Strategic Sampling**: We don't need to check all 100 pages of a report to
   be 99% sure it's the same. `pdfcmp` uses a $\sqrt{n} + 2$ sampling strategy,
   which dramatically cuts down processing time for large documents.
5. **Perceptual Hashing**: We use dHash (for structure) and pHash (for noise
   resilience) to make the final visual call.

## The Math of Perception: DCT and pHash

One of the most technically rewarding parts of this project was implementing the
Perceptual Hash (pHash). While dHash is great for finding structural shifts, it’s
fooled by digital noise or minor color variations.

pHash solves this by using the **Discrete Cosine Transform (DCT)**—the same
mathematical foundation behind JPEG compression. Here’s the workflow:

1. Reduce the image to a 32×32 grayscale matrix.
2. Execute a 2D DCT to convert the pixels into frequency space.
3. Extract the top-left 8×8 block, which represents the lowest frequencies (the
   “gist” of the image).
4. Throw away the DC component (the average brightness) and calculate a median
   bit-mask from the remaining AC frequencies.

Because this looks at the frequency of the image rather than the raw pixels, it’s
incredibly resilient. A document saved with 90% JPEG quality will have an almost
identical pHash to one saved at 50% quality.

## The Benchmark Reality

I didn't want to just claim it was “fast.” I spent a lot of time writing a
benchmarking suite to see where the bottlenecks were.

### Rendering vs. Perception

The cost of rendering a page is the primary performance bottleneck. In my tests
on an Apple M2 Pro, I saw the following:

- **72 DPI (Thumbnails)**: ~4.2ms
- **150 DPI (Standard)**: ~16.5ms
- **300 DPI (High-Res)**: ~46.8ms

By defaulting to 72 DPI for the histogram and initial perceptual check, we can
achieve a throughput of over **70 pages per second** on large documents.

### Robustness under Pressure

We simulated common document degradations to see if our hashes would hold up:

| Scenario | DHash Accuracy | PHash Accuracy | Combined |
| :--- | :---: | :---: | :---: |
| **1px Shift** | 100% | 96.9% | 98.4% |
| **5px Shift** | 98.4% | 85.9% | 92.2% |
| **5% Noise** | 82.8% | 93.8% | 88.3% |
| **10% Brightness** | 100% | 96.9% | 98.4% |

## Strategic Scaling: Sampling vs. Full-Scan

One of the most satisfying benchmarks during development was testing how the
tool scales with document size. Most PDF tools slow down linearly as you add
pages. `pdfcmp` breaks this trend using its Strategic Sampling heuristic
($\sqrt{n} + 2$).

In our stress tests with a 1,000-page extra-large PDF, the performance gap was
massive:

| Strategy | Pages Scanned | Execution Time | Speedup |
| :--- | :---: | :---: | :---: |
| **Full-Scan** | 1000 | ~21.4s | baseline |
| **Strategic $(\sqrt{n} + 2)$** | 33 | **712ms** | **30x faster** |

By intelligently selecting a subset of pages, we maintain a 99%+ confidence level
while reducing the I/O and rendering load.

## The Hidden Savings: Histogram vs. Hash

We also looked at the “micro-costs” of the visual pipeline. Even after we've
decided to render a page, there's a choice: do we go straight to a perceptual
hash, or do we do a cheaper pre-check?

Calculating an RGB histogram turned out to be the ultimate filter. Because it
only requires a single pass over the pixel data without any complex transforms
(like DCT), it’s lighter than pHash:

| Operation | Time per Page (1024×1024) | Rationale |
| :--- | :---: | :--- |
| **pHash (DCT)** | ~2.4ms | Frequency transform is CPU-intensive |
| **Histogram** | **0.8ms** | Simple pixel accumulation |

By running the Histogram check first, we can reject 80% of visually different
documents before the more expensive pHash logic even touches the CPU.

## Competitive Edge: Stacking Up Against the Giants

I didn't stop at internal metrics. I benchmarked `pdfcmp` against established
industry standards like `diff-pdf` (C++) and a naive ImageMagick-based baseline.

The results highlight where the “Early Exit” architecture truly shines:

| Scenario | pdfcmp (Total) | pdfcmp (Logic) | diff-pdf | Naive IM |
| :--- | :---: | :---: | :---: | :---: |
| **Identical (Small)** | 1.2s | **0ms** | 189ms | 759ms |
| **Visual Diff (40pg)** | 2.1s | **723ms** | 113ms | 2595ms |
| **Large Doc (100pg)** | **1.0s** | **47ms** | 89ms* | 6227ms |

*\*Note: `diff-pdf` often exits early as soon as it finds a single differing
pixel. While fast, it doesn't give you a similarity score or a full report
unless forced, which can take significantly longer (up to 2.4s for the 100pg
doc).*

### Why `pdfcmp` Wins in CI/CD

While `diff-pdf` is a fantastic native tool for binary “yes/no” answers,
`pdfcmp` offers two critical advantages for automated pipelines:

1. **Perceptual Resilience**: `pdfcmp` ignores Gaussian noise and minor
   rendering artifacts that would trigger a “fail” in pixel-perfect tools.
2. **Predictable Scaling**: Thanks to **Strategic Sampling**, `pdfcmp` can
   validate a 1,000-page document in nearly the same time it takes to validate
   a 10-page document, provided the similarity holds.

## Real-world Applications

I built this with a few specific use cases in mind:

- **CI/CD for Document Engines**: If your app generates invoices or reports, you
  can run a `pdfcmp` check on every PR to ensure your latest CSS change didn't
  break the layout of page 42.
- **Document Deduplication**: In a database of millions of scans, `pdfcmp` can
  identify visually identical documents even if they have different file sizes
  or metadata.
- **Regression Testing**: Comparing the output of two different PDF engines to
  ensure visual parity.

## Portability with Go and WebAssembly

The final piece of the puzzle was distribution. I didn't want users to have to
struggle with CGO or installing complex native libraries. I used PDFium (the
engine powering Google Chrome) but ran it through the `wazero` WebAssembly
runtime.

This means you get the performance of a production-grade PDF engine with the
ease of a single, zero-dependency Go binary. It runs identically on my Mac, my
Linux server, and Windows.

## Profiling the Invisible: Wall-Clock Analysis

One technical challenge was profiling the WebAssembly boundary. Standard Go CPU
profiles (`pprof`) often miss the time spent inside the Wasm runtime because it’s
technically “Off-CPU” from the perspective of the Go scheduler.

I used **`fgprof`** to perform wall-clock profiling. This allowed me to see not
just what the Go code was doing, but exactly how long we were waiting on PDFium
to return a rendered bitmap. This insight led to the decision to default to
72 DPI for the initial pipeline stages, as the Off-CPU wait time dropped by 80%.

## Closing Thoughts & Future Work

Building `pdfcmp` was about understanding the trade-offs between bits and
pixels. Looking forward, I'm exploring the use of Vector Embeddings and CLIP
models for “semantic” similarity—knowing that two documents are about the same
thing even if their visual layout is different.

You can find the source code, the benchmark suite, and the CLI on GitHub:
[github.com/asafshitrit/pdfcmp](https://github.com/asafshitrit/pdfcmp).
