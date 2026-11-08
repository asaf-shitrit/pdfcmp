# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- The JSON output carries the run's verdict: `status` (`IDENTICAL`, `SIMILAR` or `DIFFERENT`, as the summary prints it), `similar` (what the exit code follows), `threshold`, `text_change_mode` (`fails` or `scored`) and `tiles`. The verdict depends on these settings, which no other field showed, so two runs could print the same JSON and exit differently. `Result.Verdict()` returns the same values; existing fields are unchanged.

### Fixed
- `bytewise.Compare` hashes the files it already opened and checked, not the paths again. It validated both files as regular, then reopened each path to hash it, so a path replaced by a FIFO in between could hang the call, the sizes and the hashes could come from different files, and a comparison cost four opens. `QuickCompare`, `CompareWithEarlyExit` and `HashFile` took only a `os.Stat` or a plain open, so `CompareWithEarlyExit("/dev/zero", "/dev/zero")` saw two empty files and read forever, and `HashFile` on a FIFO blocked. They now accept regular files only and return an error for pipes, devices and unreadable files. `CompareWithEarlyExit` also reads full chunks, so a short read on one file no longer reports equal files as different.
- Chinese, Japanese, Korean and Thai pages are compared by their characters, not as one word. `content.Tokens` split on whitespace alone, so a page written without spaces was a single token with one shingle, below the 60 that `content.Similarity` needs, and the word fallback compared one huge token and scored 0 even for near-identical pages. With the page counts different, a Chinese document with a deleted chapter and renumbered footers was paired by position and reported the wrong pages, where the same English document was aligned correctly; with `--text-tolerant` a fully rewritten page lost only 5%. Each character of Han, Hiragana, Katakana, Hangul, Thai, Lao, Khmer, Myanmar and Tibetan text is now its own token. Text without those characters tokenizes as before.
- Aligning documents of different lengths no longer prepares each page's text again for every pair of pages it is scored against. Each page is tokenized, shingled and hashed once, when first needed, and the pairs use that: 99 pages against 100 pages of 800 words went from 1.9 s to 0.11 s. The pairing is unchanged.
- `WithLogger(nil)` uses the no-op logger, and `Options.Validate` rejects a nil `Logger`. Both used to pass, and `Compare` then panicked on the first log call.
- A page whose text changed only in a separator between two digits or in an accented letter is different, not similar: "1,500" for "1.500", "12:30" for "12.30", "Müller" for "Möller" kept the page's letters and digits, so they scored as the same text (`content.Swapped`). It applies when the texts, compared position by position with whitespace removed, differ only in such marks (a digit-flanked separator, or an accented letter inside a word), so a space an extractor moved, a glyph remapped on its own, or reordered text never counts as an exchange. A glyph a re-save remapped beside a real exchange (a bullet turned "ï" or "¥" next to "1,500" for "1.500") no longer hides it: differences between characters the letter-and-digit comparison ignores are skipped, while a changed letter or digit still rules the exchange out. Insertions and deletions of such marks stay with the render.
- Pages are paired by their words when the page counts differ and the pages are short. Text with fewer than 60 shingles has no similarity estimate, so every pair scored 0 and the pages were paired by position: with a "Page k of N" footer, inserting one page at the start reported every later page as different and the last one as inserted. Short pages are now scored by the Jaccard similarity of their words, leaving out words with digits, so renumbered footers do not make unlike pages look alike. The inserted page is found and the others stay paired with their own counterparts.
- `--timeout` and Ctrl-C now stop a comparison while it reads page info, not only while it renders. For long documents whose text decides every page the timeout did not bound the run time, and an expired deadline exited `0` or `2` at random. An expired deadline now always exits `2`.
- Input files that are not regular files, or cannot be read, now exit with `2` instead of a wrong verdict or a hang. A pipe's size is the bytes buffered so far, so `pdfcmp quick <(cat a.pdf) a.pdf` reported DIFFERENT for identical content over the pipe buffer, and `pdfcmp quick /dev/zero /dev/zero` never ended. An unreadable file exited `1` when its size differed from the other file's and `2` when it matched. The command line rejects pipes, devices and sockets and opens every file before comparing; `bytewise.Compare` opens both files first and returns an error for anything but a regular file.
- A page's content hash covers the page's rotation, the box that is shown, and each object's clip path and text render mode, so a page rotated, cropped or clipped differently, or with text made invisible, is no longer scored identical without being rendered. It also covers how an object is drawn where its bounds cannot show it: the whole transform matrix of an image or form (a flipped image has the same bounds) and the mirror and quarter turn of text. The shown box is the CropBox (corners put in order, so an inverted `/CropBox [600 300 0 0]` is the box PDFium shows), or the MediaBox when there is none, cut to the MediaBox; a CropBox outside the MediaBox hashes as an empty box with its own marker. A path adds all its segments (type, close flag, points mapped through its matrix into page space; no sampling, so one moved point in a long chart is seen) and how it is painted (fill rule, stroked or not, and for a stroke its width, cap, join and dash array and phase), so a mirrored or rotated drawing, `re S` against `re B`, `f` against `f*` or an added dash hash differently while the same drawing under a scaling `cm` hashes the same. An annotation adds its flags (a printed annotation set hidden), and an image its colour space, bit depth and size, raw data and, up to 65,536 pixels, the pixels PDFium decodes from it (a `/Decode` array, a palette). What the hash cannot capture is not guessed: `render.ErrNotHashable` makes the planner render the page. That is any object, also inside a form, with an optional content mark (a layer switched on or off), a shading object, a large image outside DeviceGray, DeviceRGB and DeviceCMYK, and a page of more than 200,000 path segments. Known gaps, as go-pdfium cannot read them: an ExtGState blend mode (`/BM`), a `/Decode` array on a large image, and an image soft or stencil mask. Hashing the raw boxes and every matrix made a page with no CropBox differ from one whose CropBox equals the MediaBox, and a path drawn under a scaling `cm` differ from the same path drawn directly, as a re-save by another writer does. The planner then rendered every such page, which took a 100-page document from 87 ms to 2.1 s.
- The pHash filled 63 of its 64 bits (it skipped the DC term and used nothing in its place) while similarity divides by 64, so two opposite images scored 1/64 instead of 0 and every pHash score was slightly high. The hash now also uses the first horizontal AC term outside the 8x8 block, so it fills all 64 bits.
- `--memory` sizes renders from the largest page of either document among the pages to render, not the first page of the first document. A document with an A4 cover and A0 pages could peak at several GB under `--memory 512`; it now runs with fewer workers. With no page size readable it uses one worker. Uniform A4 documents are unaffected.
- `--threshold NaN` is rejected. Every comparison with NaN is false, so the range check let it through and `pdfcmp compare a.pdf a.pdf -t NaN` reported IDENTICAL yet exited 1. `Options.Validate` also rejects unknown mode, hash type and sampling values.
- A document with no pages compared with one that has pages is now different at any threshold, and its pages are listed as inserted or deleted. With `-t 0` it was reported SIMILAR with exit code 0, and the JSON listed no pages.
- `--mode byte` decides on bytes alone. Files whose bytes differed exited 0 with `-t 0`, because no similarity score is computed in that mode and the threshold was compared with 0.
- Two different files with no pages are no longer marked `identical`, which means byte-identical. The summary showed IDENTICAL next to "Byte Comparison: DIFFERENT"; it now shows SIMILAR.
- The default worker count is capped at 64, the most `Validate` accepts. On a host with more than 64 CPUs, `pdfcmp compare` with no flags and `compare.New()` with default options failed with "workers must be between 1 and 64".

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
