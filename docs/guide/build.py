#!/usr/bin/env python3
"""Build LMA-Mini-Guide.pdf from guide.html with headless Chrome, as Q2 Pod's own guide is built
(docs/manual/build.py there).

Two passes: the first finds which page each heading lands on (via hidden @@id@@ markers read
back with PyMuPDF), the second fills in the contents. Run mockups.py first if a screen changed.

    python docs/guide/build.py [OUT.pdf]      # default: target/LMA-Mini-Guide.pdf
"""
import re, shutil, subprocess, sys, tempfile
from pathlib import Path

import pymupdf

here = Path(__file__).resolve().parent
src = (here / "guide.html").read_text(encoding="utf-8")
candidates = ["chromium", "google-chrome-stable", "google-chrome", "chrome",
              "C:/Program Files/Google/Chrome/Application/chrome.exe",
              "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"]
chrome = next((c for c in candidates if shutil.which(c) or Path(c).is_file()), None)
if not chrome:
    sys.exit("Chrome, Chromium or Edge not found")


def render(html, out):
    tmp = here / ".build.html"  # here, so the relative image paths resolve
    tmp.write_text(html, encoding="utf-8")
    try:
        subprocess.run([shutil.which(chrome) or chrome, "--headless", "--disable-gpu", "--no-pdf-header-footer",
                        "--virtual-time-budget=10000", f"--print-to-pdf={out}", tmp.as_uri()],
                       check=True, capture_output=True)
    finally:
        tmp.unlink()


marked = re.sub(r'(<h[12][^>]*\bid="([^"]+)"[^>]*>)', r'\1<i class="m">@@\2@@</i>', src)
with tempfile.TemporaryDirectory() as d:
    first = Path(d) / "pass1.pdf"
    render(re.sub(r"\{\{p:[^}]+\}\}", "00", marked), first)
    with pymupdf.open(first) as doc:
        text = [p.get_text() for p in doc]
pages = {m: i + 1 for i, t in enumerate(text) for m in re.findall(r"@@([^@\s]+)@@", t)}
missing = set(re.findall(r"\{\{p:([^}]+)\}\}", src)) - pages.keys()
if missing:
    sys.exit(f"headings not found in PDF: {sorted(missing)}")
out = Path(sys.argv[1]) if len(sys.argv) > 1 else here.parent.parent / "target" / "LMA-Mini-Guide.pdf"
out.parent.mkdir(parents=True, exist_ok=True)
render(re.sub(r"\{\{p:([^}]+)\}\}", lambda m: str(pages[m[1]]), src), out.resolve())
print(f"wrote {out}")
