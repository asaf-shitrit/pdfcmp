import subprocess
import time
import os
import re
from pathlib import Path

# Paths
FIXTURES_DIR = Path("bench/fixtures")
PDFCMP_BIN = next((p for p in ["./pdfcmp", "./bin/pdfcmp"] if os.path.exists(p)), None)

SCENARIOS = [
    {
        "name": "Identical (Small)",
        "file1": "small_a.pdf",
        "file2": "small_a.pdf", # true identical
        "pages": 1
    },
    {
        "name": "Visual Diff (Medium)",
        "file1": "medium_a.pdf",
        "file2": "medium_modified.pdf",
        "pages": 40
    },
    {
        "name": "Large Doc (Sampling)",
        "file1": "large_a.pdf",
        "file2": "large_b.pdf",
        "pages": 100,
        "pdfcmp_args": ["--sampling", "strategic"]
    },
    {
        "name": "Large Doc (Full Scan)",
        "file1": "large_a.pdf",
        "file2": "large_b.pdf",
        "pages": 100,
        "pdfcmp_args": ["--sampling", "all"]
    }
]

def run_cmd(cmd):
    start = time.perf_counter()
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=120, check=False)
        end = time.perf_counter()
        return (end - start) * 1000, result.stdout + result.stderr, result.returncode
    except Exception as e:
        return 0, str(e), -1

def extract_logic_time(output):
    # Try to find "Time elapsed: 554ms"
    match = re.search(r"Time elapsed: ([\d.]+)(ms|s)", output)
    if match:
        val = float(match.group(1))
        if match.group(2) == "s":
            val *= 1000
        return val
    return 0

def run_imagemagick_diff(f1, f2):
    start = time.perf_counter()
    try:
        os.makedirs("tmp/bench1", exist_ok=True)
        os.makedirs("tmp/bench2", exist_ok=True)
        # Render all pages for accuracy
        subprocess.run(["pdftocairo", "-png", "-r", "72", str(f1), "tmp/bench1/p"], check=True)
        subprocess.run(["pdftocairo", "-png", "-r", "72", str(f2), "tmp/bench2/p"], check=True)
        # Just compare the first differing image found or just one
        subprocess.run(["magick", "compare", "-metric", "AE", "tmp/bench1/p-1.png", "tmp/bench2/p-1.png", "null:"], capture_output=True)
        end = time.perf_counter()
        return (end - start) * 1000
    except Exception:
        return 0

def main():
    if not PDFCMP_BIN:
        print(f"Error: pdfcmp binary not found.")
        return

    print("| Scenario | Pages | pdfcmp (Total) | pdfcmp (Logic) | diff-pdf | Naive IM |")
    print("| :--- | :---: | :---: | :---: | :---: | :---: |")

    for s in SCENARIOS:
        f1 = FIXTURES_DIR / s["file1"]
        f2 = FIXTURES_DIR / s["file2"]

        if not f1.exists() or not f2.exists():
            continue

        # pdfcmp
        args = [PDFCMP_BIN, "compare", str(f1), str(f2)] + s.get("pdfcmp_args", [])
        ms_total, out, _ = run_cmd(args)
        ms_logic = extract_logic_time(out)

        # diff-pdf
        ms_diff, _, _ = run_cmd(["diff-pdf", "--null", str(f1), str(f2)])

        # IM (Naive)
        ms_im = run_imagemagick_diff(f1, f2)
        
        print(f"| {s['name']:<20} | {s['pages']:^5} | {ms_total:>8.0f}ms | {ms_logic:>8.0f}ms | {ms_diff:>8.0f}ms | {ms_im:>8.0f}ms |")

if __name__ == "__main__":
    main()
