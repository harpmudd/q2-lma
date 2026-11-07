#!/usr/bin/env python3
"""Draw the mini-guide's screen mockups: the app's pages in Q2 Pod's Minimal theme.

Mockups, not screenshots. The rows are the app's own, captured from it running on a computer
against archive.org (cmd/q2ctl), and already cut to fit by it. The page follows Q2 Pod's
page_row_detail (coverflow.c) and the Minimal theme's numbers (test/coverflow.py): a 48 px title,
64 px rows, text from x 36, a 20 px caption over a 16 px #AAAAAA detail at y 36, and the chevron
at x 322. Arial stands in for Liberation Sans, whose widths it shares. The selection
and the status bar are drawn to look like the theme, not taken from it.

    python docs/guide/mockups.py            # writes docs/guide/shots/*.png
"""
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

S = 2  # drawn at twice the Q2's 375x320, for print
W, H = 375, 320
BAR, TITLE, ROW, X = 30, 48, 64, 36
FONTS = Path("C:/Windows/Fonts")
OUT = Path(__file__).resolve().parent / "shots"


def font(name, px):
    return ImageFont.truetype(str(FONTS / name), px * S)


CAP, DET, TTL, BARF = font("arial.ttf", 20), font("arial.ttf", 16), font("arial.ttf", 20), font("arial.ttf", 15)
STAR = font("seguisym.ttf", 15)  # Arial has no ★
BG, SEL = "#161616", "#48484a"  # a softer black than the screen's, for print


def label(d, x, y, text, f, fill):
    """text from x, centred on y, its ★ from a symbol font."""
    for i, part in enumerate(text.split("★")):
        if i:
            d.text((x, y - 1.5 * S), "★", font=STAR, fill=fill, anchor="lm")
            x += d.textlength("★", font=STAR)
        d.text((x, y), part, font=f, fill=fill, anchor="lm")
        x += d.textlength(part, font=f)


def status_bar(d, playing):
    if playing:  # the play triangle, as the bar shows while music plays
        d.polygon([(16 * S, 9 * S), (16 * S, 21 * S), (26 * S, 15 * S)], fill="white")
    d.text((W / 2 * S, 15 * S), "8:30 PM", font=BARF, fill="white", anchor="mm")
    x, y = 312 * S, 22 * S  # Wi-Fi: three arcs and a dot
    for r in (11, 7.5, 4):
        d.arc([x - r * S, y - r * S, x + r * S, y + r * S], 225, 315, fill="white", width=int(2.2 * S))
    d.ellipse([x - 1.6 * S, y - 3 * S, x + 1.6 * S, y + 0.2 * S], fill="white")
    bx, by = 330 * S, 9 * S  # the battery, nearly full
    d.rounded_rectangle([bx, by, bx + 22 * S, by + 12 * S], radius=2.5 * S, outline="white", width=int(1.5 * S))
    d.rectangle([bx + 23 * S, by + 4 * S, bx + 25 * S, by + 8 * S], fill="white")
    d.rectangle([bx + 3 * S, by + 3 * S, bx + 18 * S, by + 9 * S], fill="white")


def page(name, title, rows, sel=0, playing=True):
    """rows: (caption, detail, chevron); detail None for a one-line row."""
    im = Image.new("RGB", (W * S, H * S), BG)
    d = ImageDraw.Draw(im)
    status_bar(d, playing)
    d.text((W / 2 * S, (BAR + TITLE / 2) * S), title, font=TTL, fill="white", anchor="mm")
    y0 = BAR + TITLE
    for i, (cap, det, into) in enumerate(rows):
        y = y0 + i * ROW
        if y >= H:
            break
        if i == sel:
            # Rounded Corners' pill: the row inset SEL_INSET (6) a side, its ends h / 2 (navigation.c)
            d.rounded_rectangle([6 * S, y * S, (W - 6) * S, (y + ROW) * S - 1], radius=ROW / 2 * S, fill=SEL)
        if det is None:
            label(d, X * S, (y + ROW / 2) * S, cap, CAP, "white")
        else:
            label(d, X * S, (y + 4 + 16) * S, cap, CAP, "white")
            label(d, X * S, (y + 36 + 12) * S, det, DET, "#aaaaaa")
        if into:  # the chevron, its glyph at x 322 to 332
            cy = (y + ROW / 2) * S
            d.line([(323 * S, cy - 6 * S), (330 * S, cy), (323 * S, cy + 6 * S)], fill="#8e8e93", width=int(2.2 * S))
    im.save(OUT / f"{name}.png", optimize=True)
    return im


SHOWS = [
    ("May 5 · Veterans' Memorial…", "New Haven, CT · SBD 4.8★", True),
    ("May 7 · Boston Garden", "Boston, MA · SBD 5.0★ · 9 versions", True),
    ("May 8 · Barton Hall", "Ithaca, NY · SBD 4.8★ · 23 versions", True),
    ("May 9 · Buffalo Memorial…", "Buffalo, NY · SBD 4.9★ · 7 versions", True),
]


def main():
    OUT.mkdir(exist_ok=True)
    page("menu", "Live Music Archive", [
        ("My Artists", "6 artists", True),
        ("All Artists", "A to Z", True),
        ("Top Artists", "Most played on the archive", True),
        ("Search Artists", "By name", False),
    ], playing=False)
    page("menu-playing", "Live Music Archive", [
        ("Grateful Dead", "May 8, 1977 · Barton Hall · 3 of 20", False),
        ("My Artists", "6 artists", True),
        ("All Artists", "A to Z", True),
        ("Top Artists", "Most played on the archive", True),
    ], sel=1)
    page("artist", "Grateful Dead", [
        ("Shuffle a Show", "From this artist", False),
        ("Remove from My Artists", "", False),
        ("1977", "421 recordings", True),
        ("1976", "301 recordings", True),
    ], sel=2, playing=False)
    page("year", "1977", SHOWS, sel=2, playing=False)
    page("show", "May 8 · Barton Hall", [
        ("Play Show", "SBD · 4.8★ · 20 songs · 3:00:02", False),
        ("More Versions", "22 other recordings", True),
        ("01 Minglewood Blues", "6:21", False),
        ("02 Loser", "8:52", False),
    ], playing=False)
    page("show-playing", "May 8 · Barton Hall", [
        ("El Paso", "3 of 20", False),
        ("Play Show", "SBD · 4.8★ · 20 songs · 3:00:02", False),
        ("More Versions", "22 other recordings", True),
        ("01 Minglewood Blues", "6:21 · On the Q2", False),
    ], sel=1)
    page("settings", "Settings", [
        ("Cache Size: 2.0 GB", "1.4 GB used", False),
        ("Prefer: Ogg", "Smaller files, so songs start sooner", False),
        ("Clear Cache", "Keeps the show playing", False),
        ("Card: 41.2 GB free", "", False),
    ], playing=False)


if __name__ == "__main__":
    main()
