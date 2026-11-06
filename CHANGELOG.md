# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Page alignment: when page counts differ, pages are aligned by their text (by object count and content hash when a page has none) instead of the comparison stopping at "page count mismatch". `Result.DeletedPages` and `InsertedPages` (`deleted_pages`, `inserted_pages`) name the pages with no counterpart, `PageResult.Page2` (`page2`) the aligned page in the second file, and every aligned pair is compared by the usual rules. `DifferentPages` includes deleted pages. `fixtures.Sequence` and `fixtures.Redrawn`.
- Tile check: each rendered page is cut into a 64x90 grid (swapped for landscape) and the tiles' mean grey levels are compared. A page whose worst tile moves by more than 6 grey levels (`visual.TileTolerance`) is different (`tile_changed`), so a small local edit such as a stamp on a scan is no longer lost in the whole-page hashes. Like a text change it decides the verdict and leaves `Similarity` alone. Pages whose text changed are left to the text rule. `visual.TileDiff`, `visual.TileSimilarity`, `WithTiles` option, `--no-tiles` flag, and `PageResult.TileSimilarity`, `WorstTile`, `TileChanged` and `Result.TileChanged` (`tile_similarity`, `worst_tile`, `tile_changed` in JSON).
- `visual.AlignedTileDiff`, the tile check for rescans: it fits the second page's exposure to the first's, finds the shift and lean between them, and matches each tile with the closest tile near where it should be, in both directions. A scan page (neither page has text) uses it instead of `TileDiff`, so a second scan of the same paper, shifted, rotated, re-exposed, blurred and noisier, is no longer called different in every tile. When its tiles agree, a hash score of at least 0.85 no longer fails the page. Pages with text are compared tile for tile as before.
- `fixtures.Scans`, `ScanImage`, `TextScanImage` and `StampImage`, which write scanned-page PDFs and stamp them; `fixtures.Rescan`, a seeded rescan simulator at three strengths, and `fixtures.MarkImage`, which draws thin lines, ticks and dots.
- Text similarity for pages whose text differs: 128-slot MinHash signatures of word 3-shingles, in the new `pkg/content` package. The estimate caps the page's similarity, so a page rewritten in the same layout is no longer scored by its perceptual hashes alone. When both pages have fewer than 60 distinct shingles (about six lines), the visual score decides: on short text one changed word moves the Jaccard similarity by more than 10%.
- `WithTextHash` option and `--text-hash minhash|off` flag.
- `PageResult.TextSimilarity` (`text_similarity` in JSON) and `render.PageInfo.Text`.
- `bench/content_matrix_test.go`, a text-change accuracy matrix, and `bench/simhash_test.go`, the MinHash-versus-SimHash comparison (`PDFCMP_MATRIX=1`).
- `internal/fixtures`, so tests write the PDFs they need instead of skipping without them, and `make fixtures` for the benchmark set.
- A CRAP gate: `make crap`, run before each commit by `.githooks/pre-commit` (`make hooks`).

### Changed
- Extracted text is normalised before it is hashed or shingled (`content.Normalize`): NFKC, soft hyphens and zero-width characters dropped, whitespace runs collapsed. A Ghostscript rewrite of a LibreOffice export, which adds a space before each line break, no longer counts as a text change. `golang.org/x/text` is a direct dependency.
- `Result.SimilarityScore` is now the lowest page similarity instead of the mean, so one changed page makes the documents different. `VisualComparison.OverallScore` keeps the mean.
- A sample with any page below the threshold no longer triggers the full comparison; the documents are already different.
- The thumbnail stage exits early when either thumbnail scores below 0.5, not when their mean does. The result then lists only the thumbnail pages.
- When one document's render stream fails outright, the other stops rendering instead of finishing every page.
- Page metadata is read once per page before rendering instead of again inside each hashing worker.

### Fixed
- Comparing PDFs with different page counts hung forever. They now score 0 with sampling strategy `page count mismatch`.
- `comparePages` could deadlock: a receiver sent a hash job while holding the lock the hashing workers need to take one. It hung whenever rendering outpaced hashing.
- A page that failed to render left `comparePages` waiting forever; render errors are now collected and returned. Their messages could also name the wrong page.
- Fixtures: `rand.Seed` has been a no-op since Go 1.24, so "identical" fixture pairs differed in every value; each page also spilled its footer onto a second page, doubling every page count; and `medium_modified.pdf` changed every page after page 10. Benchmark figures in this README predate these fixes.
- `make clean` deleted the fixture generator along with the fixtures.
- Fixtures take their creation date from the file name, so same-content pairs always differ in their bytes and their comparison never stops at the byte check.

### Removed
- The structural check stage. It compared the first page's text length and object count and only logged the result; no decision used it. `comparison_layers` no longer lists `structural`.

## [0.1.0] - 2026-01-08

### Added
- Multi-stage comparison pipeline (Byte-wise -> Structural -> Visual).
- Rendering powered by PDFium via WebAssembly (wazero).
- Perceptual hashing (dHash and pHash) for visual similarity.
- Strategic sampling for large documents ($\sqrt{n} + 2$ pages).
- Concurrent page processing and hashing.
- Color histogram pre-check for fast rejection.
- Text character count and graphical object count heuristics.
- CLI tool with `compare` and `quick` commands.
- JSON output support for machine-readable results.
- Comprehensive benchmark suite.
