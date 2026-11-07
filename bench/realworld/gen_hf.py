"""Labelled variants of PDFs sampled from Hugging Face pixparse/pdfa-eng-wds (born-digital web PDFs)
and pixparse/idl-wds (scanned industry documents). Writes cases_hf.json."""
import glob, json, os, random, sys
import pymupdf
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import gen

ROOT = os.path.dirname(os.path.abspath(__file__))
HF = os.environ.get("HF_DIR", "hf")
OUT = os.path.join(ROOT, os.environ.get("CASES_DIR", "cases_hf")); os.makedirs(OUT, exist_ok=True)
CASES_FILE = os.environ.get("CASES_FILE", "cases_hf.json")
gen.OUT = OUT
SEED = int(os.environ.get("SEED", "11"))
rng = random.Random(SEED); gen.rng = random.Random(SEED + 1)

def usable(path, need_text):
    try:
        d = pymupdf.open(path)
        if d.needs_pass or not (1 <= len(d) <= 60): return False
        pg = d[len(d) // 2]
        words = [w for w in pg.get_text("words") if w[4].isalpha() and len(w[4]) >= 4]
        return (len(words) >= 20) if need_text else True
    except Exception:
        return False

def sample(dir_, n, need_text):
    files = sorted(glob.glob(os.path.join(ROOT, HF, dir_, "*.pdf"))); rng.shuffle(files)
    return [f for f in files if usable(f, need_text)][:n]

cases = []
def add(a, b, label, kind, src, note=""):
    cases.append({"a": a, "b": b, "label": label, "kind": kind, "source": src, "note": note})

def variants(src, tag, kinds_same, kinds_diff):
    base = f"{tag}-{os.path.basename(src)[:-4]}"
    out = lambda k: os.path.join(OUT, f"{base}.{k}.pdf")
    n = len(pymupdf.open(src))
    for kind in kinds_same:
        try:
            {"resave": gen.resave, "meta": gen.meta, "restructure": gen.restructure, "raster": gen.raster}[kind](src, out(kind))
            add(src, out(kind), "same", kind, tag)
        except Exception as e:
            print("skip", base, kind, e, file=sys.stderr)
    for kind in kinds_diff:
        if kind in ("delpage", "swap") and n < 2: continue
        try:
            note = {"digit": gen.edit_digit, "word": gen.remove_word, "stamp": gen.stamp,
                    "delpage": gen.del_page, "swap": gen.swap}[kind](src, out(kind))
            add(src, out(kind), "diff", kind, tag, note)
        except Exception as e:
            print("skip", base, kind, e, file=sys.stderr)
    return base, out

for i, src in enumerate(sample("pdfa", 60, True)):
    base, out = variants(src, "pdfa", ["resave", "meta", "restructure"] + (["raster"] if i % 3 == 0 else []),
                         ["digit", "word", "stamp", "delpage", "swap"])
    add(src, src, "same", "identical", "pdfa")
    if i % 4 == 0 and len(pymupdf.open(src)) <= 10:
        gen.scan(src, out("scanA"), 1); gen.scan(src, out("scanB"), 2); gen.scan(out("digit"), out("scan_digit"), 3)
        add(out("scanA"), out("scanB"), "same", "rescan", "pdfa")
        add(out("scanA"), out("scan_digit"), "diff", "rescan+digit", "pdfa")
    print("pdfa", i, file=sys.stderr)

for i, src in enumerate(sample("idl", 40, False)):
    base, out = variants(src, "idl", ["resave", "restructure"], ["stamp", "delpage", "swap"])
    add(src, src, "same", "identical", "idl")
    if len(pymupdf.open(src)) <= 10:
        gen.scan(src, out("scanA"), 1); gen.scan(src, out("scanB"), 2); gen.scan(out("stamp"), out("scan_stamp"), 3)
        add(out("scanA"), out("scanB"), "same", "rescan", "idl")
        add(out("scanA"), out("scan_stamp"), "diff", "rescan+stamp", "idl")
    print("idl", i, file=sys.stderr)

json.dump(cases, open(os.path.join(ROOT, CASES_FILE), "w"), indent=1)
print(len(cases), "cases", file=sys.stderr)
