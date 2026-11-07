"""Add 'mark' cases: a pen tick (two 0.8pt strokes, ~6pt) on a middle page, no text change."""
import json, os, random, sys
import pymupdf
ROOT = os.path.dirname(os.path.abspath(__file__))
rng = random.Random(5)

def mark(src, dst):
    d = pymupdf.open(src); n = len(d) // 2; pg = d[n]; r = pg.rect
    x = rng.uniform(r.width * 0.15, r.width * 0.85); y = rng.uniform(r.height * 0.15, r.height * 0.85)
    pg.draw_line((x, y), (x + 2, y + 3), color=(0, 0, 0), width=0.8)
    pg.draw_line((x + 2, y + 3), (x + 6, y - 3), color=(0, 0, 0), width=0.8)
    d.save(dst); return f"page {n+1}: tick at ({x:.0f},{y:.0f})"

def swap_digits(src, dst):
    d = pymupdf.open(src)
    for n in [len(d) // 2] + list(range(len(d))):
        pg = d[n]
        for w in pg.get_text("words"):
            t = w[4]
            for i in range(len(t) - 1):
                if t[i].isdigit() and t[i+1].isdigit() and t[i] != t[i+1]:
                    new = t[:i] + t[i+1] + t[i] + t[i+2:]
                    rect = pymupdf.Rect(w[:4])
                    pg.add_redact_annot(rect, fill=(1, 1, 1)); pg.apply_redactions(images=pymupdf.PDF_REDACT_IMAGE_NONE)
                    pg.insert_text((rect.x0, rect.y1 - rect.height * 0.22), new, fontsize=rect.height * 0.8, fontname="helv")
                    d.save(dst); return f"page {n+1}: {t!r} -> {new!r}"
    raise ValueError("no number")

PAIRS = [tuple(a.split(":")) for a in sys.argv[1:]] or [("cases.json", "cases"), ("cases_hf.json", "cases_hf")]
for cf, outdir in PAIRS:
    cases = json.load(open(cf)); cases = [c for c in cases if c["kind"] not in ("mark", "digitswap")]
    for c in [c for c in cases if c["kind"] == "identical" and c.get("source", "dev") != "idl"]:
        src = c["a"]; tag = os.path.basename(src)[:-4]
        dst = os.path.join(ROOT, outdir, f"{c.get('source','dev')}-{tag}.mark.pdf")
        try:
            note = mark(src, dst)
        except Exception as e:
            print("skip", src, e, file=sys.stderr); continue
        cases.append({"a": src, "b": dst, "label": "diff", "kind": "mark", "source": c.get("source", "dev"), "note": note})
        dst = dst.replace(".mark.pdf", ".digitswap.pdf")
        try:
            note = swap_digits(src, dst)
        except Exception as e:
            print("skip swap", src, e, file=sys.stderr); continue
        cases.append({"a": src, "b": dst, "label": "diff", "kind": "digitswap", "source": c.get("source", "dev"), "note": note})
    json.dump(cases, open(cf, "w"), indent=1); print(cf, len(cases))
