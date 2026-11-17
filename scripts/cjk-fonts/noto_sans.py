# SPDX-License-Identifier: AGPL-3.0-only
"""Splits Noto Sans into the web fonts of the web client's Latin, Greek and Cyrillic text (ADR-0009 §5).

`scripts/task.sh cjk:generate` downloads the pinned variable Noto Sans of google/fonts and Noto Sans CJK, checks their
digests, and runs this through uv (uv.lock) after cjk_fonts.py:

    uv run --frozen --project scripts/cjk-fonts python -I scripts/cjk-fonts/noto_sans.py --release <commit> \
      --font NotoSans-VF.ttf --cjk NotoSansCJKjp-VF.otf --cjk NotoSansCJKsc-VF.otf ... \
      --out web/design-system/src/fonts/noto-sans --css web/design-system/src/noto-sans.css

It pins the width axis at its default (100), keeps the weight axis, and cuts the font into the WOFF2 slices of Google
Fonts' subsets (SUBSETS), each over the code points of its unicode-range that the font has. The code points that Noto
Sans CJK draws full-width in a region (… ※ × “ in Chinese) go into a slice of their own, "wide", and out of the others.

It writes the @font-face rules of two families into --css: "my Noto Sans", of every slice, for a text in no CJK
language; and "my Noto Sans beside CJK", of every slice but "wide", for a text in a CJK language, whose punctuation
Noto Sans CJK draws at its own width. The slices are the same files, which the browser fetches once.

Each slice closes over the GSUB and GPOS features of horizontal text (LAYOUT_FEATURES), `locl` included, which draws
the Bulgarian and Serbian forms of Cyrillic. The outputs depend only on the inputs: running it twice writes the same
bytes.
"""

from __future__ import annotations

import argparse
import io
import os
import sys
from pathlib import Path

from fontTools import subset
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer

FAMILY = "my Noto Sans"
FAMILY_BESIDE_CJK = "my Noto Sans beside CJK"

LAYOUT_FEATURES = ["calt", "ccmp", "clig", "kern", "liga", "locl", "mark", "mkmk", "rlig", "rvrn"]

# The inline box of a character, as Noto Sans CJK's and the emoji font's (ADR-0091 §10): one em, 88% above the
# baseline, so that the baselines of the fonts on a line agree and the line keeps its height when the font swaps in
# (font-display: swap).
ASCENT_OVERRIDE = "88%"
DESCENT_OVERRIDE = "12%"


def ranges(spec: str) -> list[range]:
    """A unicode-range value as Google Fonts writes it (U+0000-00FF, U+0131) as ranges of code points."""
    out = []
    for item in spec.split(","):
        a, _, b = item.strip().removeprefix("U+").partition("-")
        out.append(range(int(a, 16), int(b or a, 16) + 1))
    return out


# The subsets of Google Fonts' CSS for Noto Sans, in the order of its CSS: the browser checks the later rule first, so
# a code point in two subsets (a combining mark) is drawn from the later one.
SUBSETS = [
    ("cyrillic-ext", "U+0460-052F, U+1C80-1C8A, U+20B4, U+2DE0-2DFF, U+A640-A69F, U+FE2E-FE2F"),
    ("cyrillic", "U+0301, U+0400-045F, U+0490-0491, U+04B0-04B1, U+2116"),
    ("greek-ext", "U+1F00-1FFF"),
    ("greek", "U+0370-0377, U+037A-037F, U+0384-038A, U+038C, U+038E-03A1, U+03A3-03FF"),
    (
        "vietnamese",
        "U+0102-0103, U+0110-0111, U+0128-0129, U+0168-0169, U+01A0-01A1, U+01AF-01B0, U+0300-0301, U+0303-0304,"
        " U+0308-0309, U+0323, U+0329, U+1EA0-1EF9, U+20AB",
    ),
    (
        "latin-ext",
        "U+0100-02BA, U+02BD-02C5, U+02C7-02CC, U+02CE-02D7, U+02DD-02FF, U+0304, U+0308, U+0329, U+1D00-1DBF,"
        " U+1E00-1E9F, U+1EF2-1EFF, U+2020, U+20A0-20AB, U+20AD-20C0, U+2113, U+2C60-2C7F, U+A720-A7FF",
    ),
    (
        "latin",
        "U+0000-00FF, U+0131, U+0152-0153, U+02BB-02BC, U+02C6, U+02DA, U+02DC, U+0304, U+0308, U+0329, U+2000-206F,"
        " U+20AC, U+2122, U+2191, U+2193, U+2212, U+2215, U+FEFF, U+FFFD",
    ),
]


def format_runs(cps: list[int]) -> str:
    """Formats sorted code points as a unicode-range value, merging runs into ranges."""
    parts = []
    i = 0
    while i < len(cps):
        j = i
        while j + 1 < len(cps) and cps[j + 1] == cps[j] + 1:
            j += 1
        start, end = cps[i], cps[j]
        parts.append(f"U+{start:X}" if start == end else f"U+{start:X}-{end:X}")
        i = j + 1
    return ", ".join(parts)


def wide_in_cjk(paths: list[Path], cmap: set[int]) -> set[int]:
    """The code points of cmap that Noto Sans CJK draws full-width (one em) in any region."""
    wide = set()
    for path in paths:
        font = TTFont(str(path), lazy=True)
        upm = font["head"].unitsPerEm
        cjk = font.getBestCmap()
        hmtx = font["hmtx"]
        wide |= {cp for cp in cmap if cp in cjk and hmtx[cjk[cp]][0] >= upm}
    return wide


def subset_woff2(font_bytes: bytes, code_points: list[int]) -> bytes:
    options = subset.Options()
    options.layout_features = LAYOUT_FEATURES
    options.recalc_timestamp = False
    options.flavor = "woff2"
    font = TTFont(io.BytesIO(font_bytes), recalcTimestamp=False)
    subsetter = subset.Subsetter(options)
    subsetter.populate(unicodes=code_points)
    subsetter.subset(font)
    out = io.BytesIO()
    font.save(out, reorderTables=True)
    return out.getvalue()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--release", required=True, help="the commit of google/fonts, recorded in the CSS")
    parser.add_argument("--font", type=Path, required=True, help="NotoSans[wdth,wght].ttf of google/fonts")
    parser.add_argument("--cjk", type=Path, action="append", required=True, help="a variable Noto Sans CJK font")
    parser.add_argument("--out", type=Path, required=True, help="the directory of the WOFF2 files")
    parser.add_argument("--css", type=Path, required=True, help="the stylesheet of the @font-face rules to write")
    args = parser.parse_args()

    font = TTFont(str(args.font), recalcTimestamp=False)
    if "fvar" not in font or sorted(a.axisTag for a in font["fvar"].axes) != ["wdth", "wght"]:
        raise SystemExit(f"noto-sans: {args.font} is not a variable font of the wdth and wght axes")
    version = str(font["name"].getDebugName(5)).split(";")[0].removeprefix("Version ")
    font = instancer.instantiateVariableFont(font, {"wdth": 100})
    plain = io.BytesIO()
    font.save(plain, reorderTables=True)
    cmap = set(font.getBestCmap())

    # The letters the CJK fonts lack, which must draw from Noto Sans (ADR-0009 §5).
    for cp in (0x0105, 0x0142, 0x017C, 0x010D, 0x0159, 0x015F, 0x011F, 0x0456, 0x0457, 0x1EBF):
        if cp not in cmap:
            raise SystemExit(f"noto-sans: {args.font} lacks U+{cp:04X}")

    wide = wide_in_cjk(args.cjk, cmap)
    slices: list[tuple[str, list[int]]] = []
    served: set[int] = set()
    for name, spec in SUBSETS:
        cps = sorted(cp for r in ranges(spec) for cp in r if cp in cmap and cp not in wide)
        served |= set(cps)
        slices.append((name, cps))
    wide_cps = sorted(cp for name, spec in SUBSETS for r in ranges(spec) for cp in r if cp in cmap and cp in wide)
    slices.insert(0, ("wide", sorted(set(wide_cps))))

    args.out.mkdir(parents=True, exist_ok=True)
    wanted = set()
    total = 0
    for name, cps in slices:
        blob = subset_woff2(plain.getvalue(), cps)
        file = f"noto-sans-{name}.woff2"
        wanted.add(file)
        (args.out / file).write_bytes(blob)
        total += len(blob)
    for stale in args.out.glob("noto-sans-*.woff2"):
        if stale.name not in wanted:
            stale.unlink()

    rel = os.path.relpath(args.out.resolve(), args.css.resolve().parent).replace(os.sep, "/")
    if not rel.startswith("."):
        rel = "./" + rel
    rules = []
    for family in (FAMILY, FAMILY_BESIDE_CJK):
        for name, cps in slices:
            if family == FAMILY_BESIDE_CJK and name == "wide":
                continue
            rules.append(
                f"/* {family} {name} */\n"
                "@font-face {\n"
                f'  font-family: "{family}";\n'
                "  font-style: normal;\n"
                "  font-weight: 100 900;\n"
                "  font-display: swap;\n"
                f"  ascent-override: {ASCENT_OVERRIDE};\n"
                f"  descent-override: {DESCENT_OVERRIDE};\n"
                "  line-gap-override: 0%;\n"
                f'  src: url("{rel}/noto-sans-{name}.woff2") format("woff2");\n'
                f"  unicode-range: {format_runs(cps)};\n"
                "}\n"
            )
    header = (
        "/* SPDX-License-Identifier: AGPL-3.0-only */\n"
        "/* Generated by scripts/cjk-fonts (`scripts/task.sh cjk:generate`); do not edit.\n"
        f" * Noto Sans {version} of google/fonts at {args.release},\n"
        " * variable in weight at the default width, under the SIL Open Font License 1.1 (fonts/noto-sans/OFL-NotoSans.txt).\n"
        ' * "my Noto Sans beside CJK" leaves the code points that Noto Sans CJK draws full-width (the slice wide) to it.\n'
        " * The browser loads a slice only when the page draws a character of its unicode-range; for a character in two\n"
        " * ranges it checks the later rule first. */\n"
    )
    args.css.write_text(header + "\n" + "\n".join(rules), encoding="utf-8")
    print(f"noto-sans: {total / 1_000_000:.2f} MB in {len(slices)} slices ({len(served)} code points, {len(wide_cps)} wide)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
