"""Build labelled variants of real PDFs. Writes cases.json."""
import json, os, random, subprocess, sys, io
import pymupdf, pypdf

ROOT = os.path.dirname(os.path.abspath(__file__))
SRC, OUT = os.path.join(ROOT, "src"), os.path.join(ROOT, "cases")
os.makedirs(OUT, exist_ok=True)
rng = random.Random(7)

BASES = ["f1040_2024", "fw9_2024", "attn_v7", "bert_v2", "resnet_v1", "nist80063", "p17_2023", "pdfref"]
SCANNABLE = {"f1040_2024", "fw9_2024", "attn_v7", "bert_v2", "resnet_v1"}
REVISIONS = [("f1040_2023", "f1040_2024"), ("fw9_2018", "fw9_2024"), ("attn_v1", "attn_v7"), ("bert_v1", "bert_v2")]

def p(name): return os.path.join(OUT, name + ".pdf")

def resave(src, dst):
    subprocess.run(["gs", "-q", "-dNOPAUSE", "-dBATCH", "-sDEVICE=pdfwrite", "-dCompatibilityLevel=1.7", "-dAutoRotatePages=/None",
                    f"-sOutputFile={dst}", src], check=True)

def meta(src, dst):
    r = pypdf.PdfReader(src); w = pypdf.PdfWriter(clone_from=r)
    w.add_metadata({"/Title": "Re-exported copy", "/Producer": "Some Other Producer 9.1", "/ModDate": "D:20260101000000Z"})
    w.write(dst)

def restructure(src, dst):
    d = pymupdf.open(src); d.save(dst, garbage=4, deflate=True, clean=True)

def raster(src, dst, dpi=200):
    d = pymupdf.open(src); o = pymupdf.open()
    for pg in d:
        pix = pg.get_pixmap(dpi=dpi)
        np_ = o.new_page(width=pg.rect.width, height=pg.rect.height)
        np_.insert_image(np_.rect, stream=pix.tobytes("png"))
    o.save(dst, deflate=True)

def scan(src, dst, seed):
    """Rescan: 150 dpi grey, slight skew, noise, JPEG."""
    d = pymupdf.open(src); o = pymupdf.open(); r = random.Random(seed)
    for pg in d:
        png = pg.get_pixmap(dpi=150, colorspace=pymupdf.csGRAY).tobytes("png")
        angle = r.uniform(-0.4, 0.4)
        jpg = subprocess.run(["magick", "png:-", "-background", "white", "-rotate", f"{angle:.3f}",
                              "-gravity", "center", "-extent", "%dx%d" % (round(pg.rect.width*150/72), round(pg.rect.height*150/72)),
                              "-attenuate", "0.25", "+noise", "Gaussian", "-brightness-contrast", "%d" % r.randint(-4, 4),
                              "-quality", "65", "jpg:-"], input=png, capture_output=True, check=True).stdout
        np_ = o.new_page(width=pg.rect.width, height=pg.rect.height)
        np_.insert_image(np_.rect, stream=jpg)
    o.save(dst, deflate=True)

def pick_word(pg):
    words = [w for w in pg.get_text("words") if any(c.isdigit() for c in w[4]) and len(w[4]) >= 2]
    if not words:
        words = [w for w in pg.get_text("words") if w[4].isalpha() and len(w[4]) >= 5]
    return rng.choice(words) if words else None

def mid(d): return len(d) // 2

def edit_digit(src, dst):
    """Change one digit of one number on a middle page, keeping the font size."""
    d = pymupdf.open(src); pg = d[mid(d)]; w = pick_word(pg)
    rect = pymupdf.Rect(w[:4]); old = w[4]
    i = max(i for i, c in enumerate(old) if c.isdigit()) if any(c.isdigit() for c in old) else len(old) - 1
    c = old[i]; new = old[:i] + (str((int(c) + 1) % 10) if c.isdigit() else ("x" if c != "x" else "y")) + old[i+1:]
    pg.add_redact_annot(rect, fill=(1, 1, 1)); pg.apply_redactions(images=pymupdf.PDF_REDACT_IMAGE_NONE)
    pg.insert_text((rect.x0, rect.y1 - rect.height * 0.22), new, fontsize=rect.height * 0.8, fontname="helv")
    d.save(dst); return f"page {mid(d)+1}: {old!r} -> {new!r}"

def remove_word(src, dst):
    d = pymupdf.open(src); pg = d[mid(d)]
    words = [w for w in pg.get_text("words") if w[4].isalpha() and len(w[4]) >= 4]
    w = rng.choice(words)
    pg.add_redact_annot(pymupdf.Rect(w[:4]), fill=(1, 1, 1)); pg.apply_redactions(images=pymupdf.PDF_REDACT_IMAGE_NONE)
    d.save(dst); return f"page {mid(d)+1}: removed {w[4]!r}"

def stamp(src, dst):
    """Small initials in the margin, as an approver adds."""
    d = pymupdf.open(src); pg = d[mid(d)]; r = pg.rect
    pg.insert_text((r.width - 60, r.height - 24), "OK-JD", fontsize=7, fontname="helv", color=(0.1, 0.1, 0.6))
    d.save(dst); return f"page {mid(d)+1}: 7pt initials in the bottom margin"

def del_page(src, dst):
    d = pymupdf.open(src); n = mid(d); d.delete_page(n); d.save(dst); return f"deleted page {n+1}"

def swap(src, dst):
    d = pymupdf.open(src); n = mid(d); order = list(range(len(d))); order[n], order[n-1] = order[n-1], order[n]
    d.select(order); d.save(dst); return f"swapped pages {n} and {n+1}"

def main():
  global cases
  cases = []
  _main()

cases = []
def add(a, b, label, kind, note=""):
    cases.append({"a": a, "b": b, "label": label, "kind": kind, "note": note})

def _main():
    for base in BASES:
        src = os.path.join(SRC, base + ".pdf")
        add(src, src, "same", "identical")
        for kind, fn in [("resave", resave), ("meta", meta), ("restructure", restructure)]:
            fn(src, p(f"{base}.{kind}")); add(src, p(f"{base}.{kind}"), "same", kind)
        for kind, fn in [("digit", edit_digit), ("word", remove_word), ("stamp", stamp), ("delpage", del_page), ("swap", swap)]:
            note = fn(src, p(f"{base}.{kind}")); add(src, p(f"{base}.{kind}"), "diff", kind, note)
        if base in SCANNABLE:
            raster(src, p(f"{base}.raster")); add(src, p(f"{base}.raster"), "same", "raster")
            scan(src, p(f"{base}.scanA"), 1); scan(src, p(f"{base}.scanB"), 2)
            add(p(f"{base}.scanA"), p(f"{base}.scanB"), "same", "rescan")
            scan(p(f"{base}.digit"), p(f"{base}.scan_digit"), 3)
            add(p(f"{base}.scanA"), p(f"{base}.scan_digit"), "diff", "rescan+digit")
        print("built", base, file=sys.stderr)

    for a, b in REVISIONS:
        add(os.path.join(SRC, a + ".pdf"), os.path.join(SRC, b + ".pdf"), "diff", "revision", f"{a} vs {b}")

    json.dump(cases, open(os.path.join(ROOT, "cases.json"), "w"), indent=1)
    print(len(cases), "cases", file=sys.stderr)


if __name__ == "__main__":
    _main()
