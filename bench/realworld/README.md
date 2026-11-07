# Real-world benchmark

Each pair below is two PDFs and a label: **same** when a reader would see the
same document, **diff** when something visible changed. Every tool is asked
the same question, and run with its default settings unless the tool name says
otherwise.

## Data

Three sets, built the same way:

- **Dev set (111 pairs), used to tune pdfcmp.** IRS forms 1040 and W-9, the
  arXiv papers *Attention Is All You Need*, BERT and ResNet, NIST SP 800-63-3
  (76 pages), IRS Publication 17 (142 pages) and the 1993 PDF Reference (230
  pages). Includes four real revisions: 1040 2023 against 2024, W-9 2018
  against 2024, and two arXiv versions of each paper.
- **Tuning set (982 pairs).** 60 born-digital web PDFs sampled from the first
  shard of Hugging Face
  [`pixparse/pdfa-eng-wds`](https://huggingface.co/datasets/pixparse/pdfa-eng-wds)
  and 40 scanned documents from the first shard of
  [`pixparse/idl-wds`](https://huggingface.co/datasets/pixparse/idl-wds), the
  UCSF Industry Documents Library. pdfcmp was changed after seeing its misses
  here, so its score on this set is not a held-out figure.
- **Held-out sets (1,006 and 1,007 pairs).** Another 60 web PDFs and 40
  scans each, from the second and then the third shard of each dataset, never
  opened before. Each time the pdfcmp binary was frozen (its SHA-256
  recorded) and every label validated before any tool ran, and nothing was
  changed after that run was scored. The third set scores the current build
  and is the headline result; the second scored the build before the last
  fix (letters and digits only in the text bag), which was made on the tuning
  set alone.

Many of these PDFs are malformed, as web PDFs are.

Each source document becomes these pairs (`gen.py`, `gen_hf.py`,
`add_marks.py`, `add_imgedit.py`):

| Kind | Label | What changed |
| :--- | :---: | :--- |
| identical | same | nothing |
| meta | same | metadata rewritten (pypdf) |
| resave | same | re-distilled by Ghostscript `pdfwrite` |
| restructure | same | rewritten by MuPDF with garbage collection and clean |
| raster | same | every page printed to a 200 DPI image |
| rescan | same | two simulated scans of the same paper: 150 DPI grey, up to 0.4° lean, noise, exposure, JPEG 65 |
| digit | diff | one digit of one number changed |
| digitswap | diff | two adjacent digits of a number swapped (the characters on the page stay the same) |
| word | diff | one word removed |
| stamp | diff | 7pt initials in the bottom margin |
| mark | diff | a 6pt pen tick, drawn as vector strokes, with no text change |
| imgedit | diff | a 25% patch of the largest photo inverted |
| delpage / swap | diff | a page deleted / two pages swapped |
| rescan+digit / rescan+stamp | diff | a scan of the edited document against a scan of the original |
| revision | diff | a real later version of the document |

A tool that rewrites a PDF can also change how it looks: Ghostscript
substitutes missing fonts, and MuPDF's clean can drop content from a
malformed file. `validate.py` renders every same-labelled re-encoding and its
source with Poppler, an independent renderer, and drops the pair when more
than 1% of a page differs after a blur that hides anti-aliasing. 8 of 309
pairs were dropped, for every tool.

## Results

Third held-out set, 1,007 pairs, current build. Recall: different pairs
called different. FPR: same pairs called different.

| Tool | Accuracy | Recall | FPR | Median time |
| :--- | :---: | :---: | :---: | :---: |
| **pdfcmp** | **93.0%** | 97.1% | 12.5% | 0.23 s |
| red6 PDFCompare 1.2.3 (300 DPI, pixel by pixel) | 84.1% | 99.8% | 37.4% | 4.07 s |
| pdf-visual-diff 0.16 (tolerance 0.01) | 83.8% | 96.9% | 34.1% | 3.61 s |
| diff-pdf 0.5.3 | 82.1% | 100.0% | 42.4% | 0.20 s |
| diff-pdf-visually 1.8.1 | 81.2% | 100.0% | 44.5% | 0.52 s |
| pdf-diff 0.9.3 (JoshData) | 75.2% | 68.4% | 15.5% | 0.32 s |
| `pdftotext` + `diff` | 75.2% | 68.4% | 15.5% | 0.04 s |
| diff-pdf, 100 DPI, channel tolerance 48, 300 px per page | 61.9% | 52.4% | 25.2% | 0.17 s |
| `pdftoppm` + ImageMagick `compare -fuzz 10%` | 56.2% | 41.1% | 23.1% | 0.79 s |

Times here are per pair with six pairs running at once; see below for a run
one pair at a time.

The other sets agree. Second held-out set (1,006 pairs, previous build):
pdfcmp 92.5%, PDFCompare 83.7%, pdf-visual-diff 83.0%, diff-pdf 82.7%,
diff-pdf-visually 81.8%, pdf-diff 76.5%. Tuning set: pdfcmp 93.7% (v0.3.0:
90.2%), pdf-visual-diff 83.7%, PDFCompare 83.1%, diff-pdf 82.1%,
diff-pdf-visually 81.3%, pdf-diff 74.7%. Dev set: pdfcmp 93.7% (v0.3.0:
84.7%), diff-pdf, diff-pdf-visually and pdf-visual-diff 76.6%, PDFCompare
74.8%, pdf-diff and `pdftotext` + `diff` 69.4%.

Correct verdicts by kind, third held-out set:

| Kind | pdfcmp | PDFCompare | pdf-visual-diff | diff-pdf | diff-pdf-visually | pdf-diff |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| resave (same) | 78/99 | 29/99 | 42/99 | 30/99 | 8/99 | 61/99 |
| restructure (same) | 98/100 | 78/100 | 79/100 | 56/100 | 69/100 | 90/100 |
| raster (same) | 15/18 | 0/18 | 0/18 | 0/18 | 0/18 | 0/18 |
| rescan (same) | 22/49 | 0/49 | 0/49 | 0/49 | 0/49 | 49/49 |
| identical, meta (same) | 159/159 | 159/159 | 159/159 | 159/159 | 159/159 | 159/159 |
| digit | 60/60 | 60/60 | 60/60 | 60/60 | 60/60 | 60/60 |
| digitswap | 55/58 | 58/58 | 58/58 | 58/58 | 58/58 | 58/58 |
| word | 59/60 | 60/60 | 58/60 | 60/60 | 60/60 | 60/60 |
| stamp | 99/100 | 100/100 | 100/100 | 100/100 | 100/100 | 100/100 |
| mark | 53/60 | 60/60 | 60/60 | 60/60 | 60/60 | 0/60 |
| imgedit | 72/73 | 73/73 | 73/73 | 73/73 | 73/73 | 0/73 |
| delpage, swap | 121/122 | 121/122 | 106/122 | 122/122 | 122/122 | 120/122 |
| rescan+digit, rescan+stamp | 46/49 | 49/49 | 49/49 | 49/49 | 49/49 | 0/49 |

The pixel tools (PDFCompare, pdf-visual-diff, diff-pdf, diff-pdf-visually)
catch nearly every edit by calling 34-45% of the unchanged
documents different. The text tools (pdf-diff, `pdftotext` + `diff`) call
every pair of scans the same, since neither has text, and miss every pen mark
and photo edit. pdfcmp misses pen ticks drawn over text, swapped digits in
small type, and scanned edits inside a rescan's noise. Its false alarms are
its weakness: more than half of the second scans of the same paper, worst on
logo-heavy pages (no setting of its scan check trades fewer of them for no
more missed edits); and 21 of 99 Ghostscript re-saves, mostly files PDFium
renders differently from the original where Poppler and MuPDF do not.

Not compared: commercial tools (Adobe Acrobat Compare, Draftable, i-net
PDFC), which have no free command line.

Speed, 38 pairs run one at a time on a 10-core M2 Pro (30 from the tuning set
and the 8 dev resaves, up to 230 pages):

| Tool | Median | Mean | 230-page manual |
| :--- | :---: | :---: | :---: |
| pdfcmp | 0.19 s | 0.54 s | 0.3 s (stops at the first text change, and is wrong: a re-save that maps glyphs to other characters); about 3 s to compare all 230 pages |
| diff-pdf | 0.16 s | 0.25 s | 0.1 s (stops at the first differing pixel, and is wrong) |
| `pdftotext` + `diff` | 0.05 s | 0.10 s | 0.5 s |
| pdf-visual-diff | 5.93 s | 20.68 s | 225 s |

## Running it

Needs Ghostscript, Poppler, ImageMagick 7, diff-pdf, Node.js and Python 3.
PDFCompare needs Java and its jar in `tools/jar/pdfcompare-full.jar` (from
Maven Central, `de.redsix:pdfcompare:1.2.3`, classifier `full`), or
`PDFCOMPARE_JAR`. diff-pdf-visually 1.8.1 misreads ImageMagick 7.1.2's PSNR output and calls
every pair different; `patch_dpv.py` fixes the installed copy.

```bash
make build                                   # ../../pdfcmp
cd bench/realworld
./fetch.sh                                   # documents and two Hugging Face shards
python3 -m venv venv && ./venv/bin/pip install pymupdf pypdf diff-pdf-visually==1.8.1 pdf-diff==0.9.3
./venv/bin/python patch_dpv.py
npm install pdf-visual-diff@0.16.0
./venv/bin/python gen.py && ./venv/bin/python gen_hf.py
./venv/bin/python add_marks.py && ./venv/bin/python add_imgedit.py   # all sets by default
python3 validate.py cases.json && python3 validate.py cases_hf.json   # and cases_ho.json
# held-out set, from the second shard of each dataset:
# DEST=hf2 PDFA_SHARD=0001 IDL_SHARD=00001 ./fetch.sh
# HF_DIR=hf2 CASES_DIR=cases_ho CASES_FILE=cases_ho.json SEED=101 ./venv/bin/python gen_hf.py
# ./venv/bin/python add_marks.py cases_ho.json:cases_ho; ./venv/bin/python add_imgedit.py cases_ho.json:cases_ho
J=6 ./venv/bin/python run.py cases_hf.json results.json   # all tools; pdf-visual-diff takes about an hour
python3 score.py cases_hf.json results.json --by kind
```

`run.py cases.json out.json pdfcmp` runs one tool; `PDFCMP=path` picks the
binary; `score.py ... --miss pdfcmp` lists a tool's wrong verdicts. Results
depend on the sample drawn from the shards, which the scripts seed.
