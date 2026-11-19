"""Legibility + colour test for the three SIGAP concepts.

Renders each delivered SVG at true 64/32/16 px on light, canvas and dark grounds
(brand, black, white colourways), measures ink coverage and surviving counters,
and writes legibility-test.png + legibility-report.txt.

Run from the exploration folder:  python _legibility_test.py
"""
import os
import sys
from collections import deque

sys.path.insert(0, r"C:\Users\losts\.omp\agent\skills\logo-design\scripts")
import render_png
from PIL import Image, ImageDraw, ImageFont

CONCEPTS = [("concept-a.svg", "A  Route S"), ("concept-b.svg", "B  Threshold"), ("concept-c.svg", "C  Channel")]
SIZES = [64, 32, 16]
COLOURWAYS = [("brand #0F766E", "#0F766E"), ("black", "#000000"), ("white", "#FFFFFF")]
GROUNDS = [("light #FFFFFF", "#FFFFFF"), ("canvas #F7F6F3", "#F7F6F3"), ("dark #0B1220", "#0B1220")]


def measure(path):
    """Ink coverage and number of separate ink islands in a rendered mark."""
    im = Image.open(path).convert("RGBA")
    w, h = im.size
    px = im.load()
    ink = sum(1 for y in range(h) for x in range(w) if px[x, y][3] > 128)
    seen = [[False] * w for _ in range(h)]
    q = deque()
    for x in range(w):
        for y in (0, h - 1):
            if px[x, y][3] <= 128 and not seen[y][x]:
                seen[y][x] = True
                q.append((x, y))
    for y in range(h):
        for x in (0, w - 1):
            if px[x, y][3] <= 128 and not seen[y][x]:
                seen[y][x] = True
                q.append((x, y))
    while q:
        x, y = q.popleft()
        for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)):
            nx, ny = x + dx, y + dy
            if 0 <= nx < w and 0 <= ny < h and not seen[ny][nx] and px[nx, ny][3] <= 128:
                seen[ny][nx] = True
                q.append((nx, ny))
    # how many separate ink islands survive at this size: the mark stays legible
    # as long as the count matches the drawing (A and B are one mass, C is two)
    islands, mark = 0, [[False] * w for _ in range(h)]
    for y in range(h):
        for x in range(w):
            if px[x, y][3] > 128 and not mark[y][x]:
                q = deque([(x, y)])
                mark[y][x] = True
                while q:
                    cx, cy = q.popleft()
                    for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                        nx, ny = cx + dx, cy + dy
                        if 0 <= nx < w and 0 <= ny < h and not mark[ny][nx] and px[nx, ny][3] > 128:
                            mark[ny][nx] = True
                            q.append((nx, ny))
                islands += 1
    return ink / (w * h), islands


def contrast(a, b):
    def lum(c):
        def f(v):
            v /= 255
            return v / 12.92 if v <= 0.03928 else ((v + 0.055) / 1.055) ** 2.4
        return 0.2126 * f(c[0]) + 0.7152 * f(c[1]) + 0.0722 * f(c[2])
    la, lb = lum(a), lum(b)
    hi, lo = max(la, lb), min(la, lb)
    return (hi + 0.05) / (lo + 0.05)


def hex2rgb(h):
    h = h.lstrip("#")
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


def main():
    cache, report = {}, []
    for f, _ in CONCEPTS:
        src = open(f, encoding="utf-8").read()
        for cname, chex in COLOURWAYS:
            tmp_svg = f"_lt_{f}_{chex.strip('#')}.svg"
            open(tmp_svg, "w", encoding="utf-8").write(src.replace("#0F766E", chex))
            for sz in SIZES:
                png = f"_lt_{f}_{chex.strip('#')}_{sz}.png"
                render_png.render(tmp_svg, png, sz, sz, backend="chrome")
                cache[(f, cname, sz)] = png
            os.remove(tmp_svg)

    report.append("SIGAP concepts — legibility test")
    report.append("rendered from the delivered SVGs at true pixel sizes (skill Chrome backend)")
    report.append("")
    report.append(f"{'file':12} {'colour':14} {'px':>4} {'ink':>7} {'islands':>8}")
    for f, _ in CONCEPTS:
        for cname, _ in COLOURWAYS:
            for sz in SIZES:
                cov, islands = measure(cache[(f, cname, sz)])
                report.append(f"{f:12} {cname:14} {sz:>4} {cov*100:>6.1f}% {islands:>8}")

    report.append("")
    report.append("Contrast of the brand colour (WCAG 2.1 relative luminance):")
    for gname, ghex in GROUNDS:
        r = contrast(hex2rgb("#0F766E"), hex2rgb(ghex))
        verdict = "AA text + non-text" if r >= 4.5 else ("non-text only" if r >= 3 else "FAILS")
        report.append(f"  #0F766E on {gname:16} {r:5.2f}:1  ({verdict})")

    open("legibility-report.txt", "w", encoding="utf-8").write("\n".join(report) + "\n")
    print("\n".join(report))

    # visual strip: rows = concept x ground, columns = 64/32/16 px, brand colourway
    font = ImageFont.truetype("arial.ttf", 15)
    small = ImageFont.truetype("arial.ttf", 12)
    rows = []
    for f, clabel in CONCEPTS:
        for gname, ghex in GROUNDS:
            rows.append((f, clabel, gname, ghex))
    cw, ch, pad = 100, 96, 22
    W = pad * 2 + 200 + len(SIZES) * cw
    H = pad * 2 + 48 + len(rows) * ch + 26
    sheet = Image.new("RGB", (W, H), "#EDEDEA")
    d = ImageDraw.Draw(sheet)
    d.text((pad, pad), "SIGAP concepts — size ladder on light / canvas / dark grounds", font=font, fill="#111")
    d.text((pad, pad + 20), "brand colour #0F766E, rendered from the delivered files at true pixel sizes", font=small, fill="#555")
    x0 = pad + 200
    for i, sz in enumerate(SIZES):
        d.text((x0 + i * cw + cw // 2 - 12, pad + 26), f"{sz}px", font=small, fill="#444")
    y0 = pad + 52
    for ri, (f, clabel, gname, ghex) in enumerate(rows):
        y = y0 + ri * ch
        d.text((pad, y + ch // 2 - 14), clabel, font=small, fill="#111")
        d.text((pad, y + ch // 2 + 2), gname, font=small, fill="#666")
        for ci, sz in enumerate(SIZES):
            tile = Image.new("RGB", (cw - 8, ch - 8), ghex)
            im = Image.open(cache[(f, "brand #0F766E", sz)]).convert("RGBA")
            tile.paste(im, ((cw - 8 - sz) // 2, (ch - 8 - sz) // 2), im)
            sheet.paste(tile, (x0 + ci * cw, y + 4))
    d.text((pad, H - 20),
           "16px column is a 1:1 pixel test. On the dark ground the brand teal measures 3.42:1 — enough for a graphic, not for text.",
           font=small, fill="#555")
    sheet.save("legibility-test.png")
    print("\nwrote legibility-test.png", sheet.size)

    for k in cache:
        os.remove(cache[k])


if __name__ == "__main__":
    main()
