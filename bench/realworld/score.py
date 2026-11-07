"""usage: score.py cases.json results.json [--by kind|source] [--miss tool]"""
import json, sys, collections, statistics
cases = {(c["a"], c["b"]): c for c in json.load(open(sys.argv[1])) if not c.get("invalid")}
res = [r for r in json.load(open(sys.argv[2])) if (r["a"], r["b"]) in cases]
by = sys.argv[sys.argv.index("--by") + 1] if "--by" in sys.argv else None
miss = sys.argv[sys.argv.index("--miss") + 1] if "--miss" in sys.argv else None

tools = list(dict.fromkeys(r["tool"] for r in res))
print(f"{'tool':16} {'acc':>6} {'TP':>4} {'FN':>4} {'TN':>4} {'FP':>4} {'err':>4} {'recall':>7} {'FPR':>6} {'med s':>6} {'total s':>8}")
for t in tools:
    rs = [r for r in res if r["tool"] == t]
    k = collections.Counter()
    for r in rs:
        lab = cases[(r["a"], r["b"])]["label"]; v = r["verdict"]
        if v not in ("same", "diff"): k["err"] += 1; k["err_" + lab] += 1; continue
        k[{("diff", "diff"): "TP", ("diff", "same"): "FN", ("same", "same"): "TN", ("same", "diff"): "FP"}[(lab, v)]] += 1
    n = len(rs); pos = k["TP"] + k["FN"] + k["err_diff"]; neg = k["TN"] + k["FP"] + k["err_same"]
    secs = [r["secs"] for r in rs]
    print(f"{t:16} {(k['TP']+k['TN'])/n:6.1%} {k['TP']:4} {k['FN']:4} {k['TN']:4} {k['FP']:4} {k['err']:4} "
          f"{k['TP']/max(pos,1):7.1%} {k['FP']/max(neg,1):6.1%} {statistics.median(secs):6.2f} {sum(secs):8.0f}")

if by:
    groups = sorted({cases[(r['a'], r['b'])][by] for r in res})
    print(f"\ncorrect by {by}")
    print(f"{'':16}" + "".join(f"{g[:12]:>13}" for g in groups))
    for t in tools:
        row = f"{t:16}"
        for g in groups:
            rs = [r for r in res if r["tool"] == t and cases[(r["a"], r["b"])][by] == g]
            ok = sum(r["verdict"] == cases[(r["a"], r["b"])]["label"] for r in rs)
            row += f"{f'{ok}/{len(rs)}':>13}"
        print(row)

if miss:
    print(f"\n{miss} misses")
    for r in res:
        c = cases[(r["a"], r["b"])]
        if r["tool"] == miss and r["verdict"] != c["label"]:
            print(c["label"], r["verdict"], c["kind"], c["b"].split("/")[-1], c.get("note", ""))
