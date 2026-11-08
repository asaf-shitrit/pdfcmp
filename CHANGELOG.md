# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- `--timeout` and Ctrl-C now stop a comparison while it reads page info, not only while it renders. For long documents whose text decides every page the timeout did not bound the run time, and an expired deadline exited `0` or `2` at random. An expired deadline now always exits `2`.

## [0.4.0] - 2026-11-08

### Fixed
- Rendered pages are now freed. PDFium's WebAssembly build renders into its own memory and every page stayed allocated, so memory grew with page count: comparing two 600-page documents peaked at 3.4 GB. Renders are copied out and released, and pages waiting for the other document's render of the same page are capped at the worker count plus one. The same comparison now peaks at 1.5 GB with the defaults (7 workers at 150 DPI on A4 under `--memory 512`) and 0.8 GB with `--workers 4`, 1.6 GB at 1500 pages, and runs in 6.5 s instead of 20 s on a 10-core M-series Mac.
- Comparisons no longer fail with "Timeout waiting for idle object" when the PDFium pool is busy. A document's render stream renders on the document's own instance and borrows more only while they are free, and comparisons run at once wait for a free pair of instances (`render.PoolSize` / 2) instead of opening their first document and waiting 30 s for the second. Opening a document waits on the comparison's context, so `--timeout` and Ctrl-C also stop a comparison waiting for the pool; a second Ctrl-C kills one slow to stop.
- `--workers` and `--memory` now limit rendering, not only scoring. `--memory` counts 8 renders per worker, which is what the pipeline holds at most.
- A data race between PDFium instances, reported by `go test -race` inside wazero.
- `pdfcmp quick` exits with `1` when the files differ; it exited `0` either way.
- The text summary's status uses `--threshold`, not the 0.95 default, so it agrees with the exit code.
- Comparing a file with itself reports its size and an identical byte comparison instead of "0 pages, 0 B" and "Byte Comparison: DIFFERENT".
- `pdfcmp version` printed `v0.1.0`; it prints the build's version, set by `make build` from `git describe`, or the module version `go install` records.
- `render`'s `RenderPages` stops and waits for its stream on the first page error, instead of leaving it rendering after it returns.
- An open error names the file once: `failed to open PDF a.pdf: 3: incorrect format`.
- Annotations and form fields are rendered, as viewers draw them: comments, stamps, filled-in and comb fields were invisible to the comparison, and a form flattened by another tool differed from the original in every field.
- A page's content hash covers images inside form objects, where LaTeX and other writers put included figures, and the page's annotations. A replaced figure or an added stamp annotation left the hash alone, so the page was taken as unchanged without being rendered.
- A document written again by another tool is no longer different because its text runs come out in another order, with spaces in other places, or with glyphs mapped to other symbols (Ghostscript turns bullets into "ï" or "¥" and broken encodings into control characters): two pages carry the same text when the letters and digits on them are the same (`render.PageInfo.TextBag`, `content.Bag`). Text that moved, and a change confined to punctuation, symbols or Latin-1 letters, are the render's to judge.
- A page printed to an image, or a scan given an OCR layer, against the same page with text is judged by its render: a page with text against one without is no longer a text change.
- A text change no render can show, such as hidden text or text turned into outlines, is no longer a difference: a page whose text differs but whose renders are the same picture (`visual.ChangedPixels` finds no pixel off by 64 levels or more) has the same text.
- Born-digital pages are checked for ink that appeared or disappeared (`visual.InkDiff`) instead of by tile means, which a sub-pixel move of a line of text, a hairline drawn heavier or a grey form-field border moved past the tolerance. A changed tile mean of more than 30 grey levels still makes a page different, for edits inside photos, where nothing is flat. When the tiles find no change, a hash score down to 0.85 passes the page, as it already did for scans.

### Changed
- Exit code `2`, not `1`, means the files could not be compared: errors are told apart from differences, as with `cmp` and `diff`. Usage is printed only for usage errors.
- `--format` accepts only `text` and `json`.
- Dependencies: `golang.org/x/text` v0.43.0 fixes GO-2026-5970, an infinite loop on invalid input that `content.Normalize` reached with a PDF's text; `go-pdfium` v1.21.4 and `wazero` v1.12.0. The module needs Go 1.26.

### Added
- `bench/realworld`, a benchmark built from real documents: IRS forms and arXiv papers with their real revisions, two long manuals, and web PDFs and scans from Hugging Face `pixparse/pdfa-eng-wds` and `pixparse/idl-wds`, each re-saved, rasterised, re-scanned or edited (a digit, a word, two swapped digits, a pen tick, a stamp, a photo, a page). It runs pdfcmp, red6 PDFCompare, pdf-visual-diff, diff-pdf, diff-pdf-visually, pdf-diff, `pdftotext` with `diff` and ImageMagick `compare` on every pair. On 1,007 held-out pairs pdfcmp scores 93.0%, the next tool (PDFCompare) 84.1%; see its README.
- `visual.InkDiff`, `InkScore`, `ChangedPixels` and their constants.
- `--timeout` for `compare`, and `pdfcmp --version`.
- `render.Options.Workers`, the most pages `RenderPagesStream` renders at once, and `render.PoolSize`.
- `render.ContextOpener`, implemented by the PDFium renderer's `OpenContext`.

## [0.3.0] - 2026-11-06

v0.2.0, tagged earlier the same day, has everything below except page alignment, text normalisation and the rescan tile check (`AlignedTileDiff`).

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
