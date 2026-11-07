"""Add 'imgedit' cases: invert a 25% x 25% patch inside the largest raster image of the document."""
import json, os, sys
import pymupdf
ROOT = os.path.dirname(os.path.abspath(__file__))

def imgedit(src, dst):
    d = pymupdf.open(src); best = None
    for pno in range(len(d)):
        for img in d[pno].get_images(full=True):
            xref = img[0]
            for r in d[pno].get_image_rects(xref):
                if r.width * r.height > 2500 and (best is None or r.width * r.height > best[0]):
                    best = (r.width * r.height, pno, xref)
    if not best: raise ValueError("no image")
    _, pno, xref = best
    pix = pymupdf.Pixmap(d, xref)
    if pix.n - pix.alpha not in (1, 3): pix = pymupdf.Pixmap(pymupdf.csRGB, pix)
    w, h = pix.width, pix.height
    pix.invert_irect(pymupdf.IRect(int(w * .4), int(h * .4), int(w * .65), int(h * .65)))
    d[pno].replace_image(xref, pixmap=pix)
    d.save(dst); return f"page {pno+1}: inverted a patch of image xref {xref}"

PAIRS = [tuple(a.split(":")) for a in sys.argv[1:]] or [("cases.json", "cases"), ("cases_hf.json", "cases_hf")]
for cf, outdir in PAIRS:
    cases = [c for c in json.load(open(cf)) if c["kind"] != "imgedit"]
    for c in [c for c in cases if c["kind"] == "identical"]:
        src = c["a"]; dst = os.path.join(ROOT, outdir, f"{c.get('source','dev')}-{os.path.basename(src)[:-4]}.imgedit.pdf")
        try: note = imgedit(src, dst)
        except Exception as e: continue
        cases.append({"a": src, "b": dst, "label": "diff", "kind": "imgedit", "source": c.get("source", "dev"), "note": note})
    json.dump(cases, open(cf, "w"), indent=1)
    print(cf, sum(c["kind"] == "imgedit" for c in cases))
