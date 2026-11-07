"""Run every tool over a case file. usage: run.py cases.json out.json [tool ...]"""
import json, os, re, subprocess, sys, tempfile, time, glob, shutil
from concurrent.futures import ThreadPoolExecutor

PDFCMP = os.environ.get("PDFCMP", os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "pdfcmp"))
TIMEOUT = 600

def sh(cmd, **kw):
    return subprocess.run(cmd, capture_output=True, timeout=TIMEOUT, **kw)

def pdfcmp(a, b, *extra):
    r = sh([PDFCMP, "compare", "-q", *extra, a, b])
    return {0: "same", 1: "diff"}.get(r.returncode, "error")

def diffpdf(a, b, *extra):
    r = sh(["diff-pdf", *extra, a, b])
    return {0: "same", 1: "diff"}.get(r.returncode, "error")

def pages(path):
    r = sh(["pdfinfo", path]); m = re.search(rb"Pages:\s+(\d+)", r.stdout)
    return int(m.group(1)) if m else None

def text(path):
    r = sh(["pdftotext", "-q", path, "-"])
    if r.returncode != 0: raise RuntimeError("pdftotext")
    return " ".join(r.stdout.decode("utf8", "replace").split())

def textdiff(a, b):
    """pdftotext both, whitespace-normalised equality: what `diff <(pdftotext a -) <(pdftotext b -)` gives."""
    return "same" if text(a) == text(b) else "diff"

def render(path, d, dpi):
    r = sh(["pdftoppm", "-r", str(dpi), "-gray", "-png", path, os.path.join(d, "p")])
    if r.returncode != 0 and not glob.glob(os.path.join(d, "p*.png")): raise RuntimeError("pdftoppm")
    return sorted(glob.glob(os.path.join(d, "p*.png")))

def imagemagick(a, b, fuzz="10%", frac=0.001, dpi=72):
    """Render with poppler, then `magick compare -metric AE -fuzz` per page; different when a page's
    changed-pixel fraction exceeds frac or the page counts differ."""
    with tempfile.TemporaryDirectory() as t1, tempfile.TemporaryDirectory() as t2:
        pa, pb = render(a, t1, dpi), render(b, t2, dpi)
        if len(pa) != len(pb): return "diff"
        for x, y in zip(pa, pb):
            r = sh(["magick", "compare", "-metric", "AE", "-fuzz", fuzz, x, y, "null:"])
            m = re.match(rb"([\d.e+]+)", r.stderr.strip())
            if r.returncode == 2 or not m: return "diff"  # size mismatch
            w, h = map(int, sh(["magick", "identify", "-format", "%w %h", x]).stdout.split())
            if float(m.group(1)) / (w * h) > frac: return "diff"
        return "same"

def visualdiff(a, b, tol="0.0"):
    """pdf-visual-diff (npm): snapshot a, then compare b against it."""
    with tempfile.TemporaryDirectory() as t:
        r = sh(["node", os.path.join(os.path.dirname(__file__), "pvd.mjs"), a, b, t, tol],
               cwd=os.path.dirname(__file__))
        out = r.stdout.decode().strip()
        return out if out in ("same", "diff") else "error"

VENV_BIN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "venv", "bin")

def diff_pdf_visually(a, b):
    """diff-pdf-visually (pip): 50 DPI renders, a page differs below 100 dB PSNR. Exit 0 same, 2 different."""
    r = sh([os.path.join(VENV_BIN, "diff-pdf-visually"), "-q", a, b])
    return {0: "same", 2: "diff"}.get(r.returncode, "error")

def pdf_diff(a, b):
    """pdf-diff (JoshData, pip): compares the words and their order in the two PDFs' text layers."""
    r = sh([os.path.join(VENV_BIN, "pdf-diff"), a, b])
    if r.returncode == 0: return "diff"
    if b"There are no text differences" in r.stderr: return "same"
    return "error"

PDFCOMPARE_JAR = os.environ.get("PDFCOMPARE_JAR", os.path.join(os.path.dirname(os.path.abspath(__file__)), "tools", "jar", "pdfcompare-full.jar"))

def pdfcompare(a, b):
    """red6 PDFCompare 1.2.3 (Java), default settings: 300 DPI renders compared pixel by pixel.
    Exit 0 equal, 1 different, 2 error."""
    with tempfile.TemporaryDirectory() as t:
        r = sh(["java", "-Djava.awt.headless=true", "-jar", PDFCOMPARE_JAR, a, b, "-o", os.path.join(t, "out")])
    return {0: "same", 1: "diff"}.get(r.returncode, "error")

TOOLS = {
    "pdfcompare": pdfcompare,
    "diff-pdf-visually": diff_pdf_visually,
    "pdf-diff": pdf_diff,
    "pdfcmp": lambda a, b: pdfcmp(a, b),
    "diff-pdf": lambda a, b: diffpdf(a, b),
    "diff-pdf-tuned": lambda a, b: diffpdf(a, b, "--dpi=100", "--channel-tolerance=48", "--per-page-pixel-tolerance=300"),
    "pdftotext-diff": textdiff,
    "imagemagick": lambda a, b: imagemagick(a, b),
    "pdf-visual-diff": lambda a, b: visualdiff(a, b, "0.01"),
}

def one(tool, c):
    t0 = time.perf_counter()
    try: v = TOOLS[tool](c["a"], c["b"])
    except subprocess.TimeoutExpired: v = "timeout"
    except Exception as e: v = "error"
    return {"tool": tool, "a": c["a"], "b": c["b"], "verdict": v, "secs": time.perf_counter() - t0}

def main():
    cases = json.load(open(sys.argv[1])); out = sys.argv[2]
    tools = sys.argv[3:] or list(TOOLS)
    done = {}
    if os.path.exists(out):
        for r in json.load(open(out)): done[(r["tool"], r["a"], r["b"])] = r
    jobs = [(t, c) for t in tools for c in cases if (t, c["a"], c["b"]) not in done]
    workers = int(os.environ.get("J", "4"))
    with ThreadPoolExecutor(workers) as ex:
        for i, r in enumerate(ex.map(lambda j: one(*j), jobs)):
            done[(r["tool"], r["a"], r["b"])] = r
            if i % 50 == 0:
                print(f"{i}/{len(jobs)}", file=sys.stderr, flush=True)
                json.dump(list(done.values()), open(out, "w"))
    json.dump(list(done.values()), open(out, "w"))

if __name__ == "__main__":
    main()
