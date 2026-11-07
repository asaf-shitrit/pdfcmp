"""diff-pdf-visually 1.8.1 reads ImageMagick's -verbose PSNR as decibels, but
ImageMagick 7.1.2 prints it normalised to [0, 1] (dB / 120), so every pair,
identical ones included, scores 1 dB and is called different. This patches the
installed copy in the venv to convert back. usage: venv/bin/python patch_dpv.py"""
import glob, os, sys

[path] = glob.glob(os.path.join(sys.prefix, "lib", "python*", "site-packages", "diff_pdf_visually", "diff.py"))
s = open(path).read()
old = '''    all_num = INFINITY if (all_str == "0" or all_str == "1.#INF") else float(all_str)'''
new = old + '''
    # pdfcmp benchmark patch: ImageMagick 7.1.2 prints PSNR normalised to
    # [0, 1] (dB / 120) in -verbose output, 1 for identical images.
    if all_num != INFINITY and all_num <= 1:
        all_num = INFINITY if all_num >= 1 else all_num * 120'''
if "pdfcmp benchmark patch" not in s:
    assert old in s, "unexpected diff-pdf-visually version"
    open(path, "w").write(s.replace(old, new))
print("patched", path)
