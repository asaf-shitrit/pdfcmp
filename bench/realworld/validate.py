"""Check same-labelled re-encodings with an independent renderer (poppler). A variant whose pages
differ visibly from the source (vfrac.sh: over 1% of a page's pixels differ by more than 20% in any
colour channel after a sigma-1.5 blur at 100 DPI, or a different page count) was altered by the
generator, not just re-encoded: mark it invalid."""
import json, sys, os, re, glob, subprocess, tempfile
from concurrent.futures import ThreadPoolExecutor
cf = sys.argv[1]; cases = json.load(open(cf))

def frac(a, b):
    out = subprocess.run([os.path.join(os.path.dirname(os.path.abspath(__file__)), "vfrac.sh"), a, b], capture_output=True, text=True).stdout.strip()
    try: return float(out)
    except ValueError: return 1.0

todo = [c for c in cases if c["label"] == "same" and c["kind"] in ("resave", "restructure", "meta", "raster")]
with ThreadPoolExecutor(4) as ex:
    for c, f in zip(todo, ex.map(lambda c: frac(c["a"], c["b"]), todo)):
        c["poppler_diff"] = f
        c["invalid"] = f > 0.01
json.dump(cases, open(cf, "w"), indent=1)
bad = [c for c in todo if c["invalid"]]
print(cf, "checked", len(todo), "invalid", len(bad))
for c in bad: print(" ", c["kind"], os.path.basename(c["b"]), round(c["poppler_diff"], 4))
