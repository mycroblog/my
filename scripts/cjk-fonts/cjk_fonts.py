# SPDX-License-Identifier: AGPL-3.0-only
"""Splits Noto Sans CJK into the web fonts of the web client's text, one family per region (ADR-0009 §5).

`scripts/task.sh cjk:generate` downloads the pinned variable fonts of notofonts/noto-cjk, checks their digests, and
runs this through uv (uv.lock):

    uv run --frozen --project scripts/cjk-fonts python -I scripts/cjk-fonts/cjk_fonts.py --release Sans2.004 \
      --font jp=NotoSansCJKjp-VF.otf --font sc=NotoSansCJKsc-VF.otf --font tc=NotoSansCJKtc-VF.otf \
      --font kr=NotoSansCJKkr-VF.otf --out web/design-system/src/fonts/cjk --css web/design-system/src/cjk.css

It writes the WOFF2 slices of each region into --out and their @font-face rules, of the families "my Noto Sans CJK JP",
"... SC", "... TC" and "... KR", into --css. The browser loads a slice only when the page draws a character of its
unicode-range, so the slices follow how often characters are used, as Google Fonts' do:

- the characters outside Han and Hangul by script: Latin, Greek and Cyrillic, the other symbols, kana with the CJK
  punctuation, and the other CJK symbols;
- the Han characters a region uses most (COMMON_HAN, from the first level of its national character set, which Python's
  codecs hold) in slices of COMMON_SLICE characters, and the common Hangul syllables (KS X 1001) likewise;
- every other Han character and Hangul syllable in slices of RARE_SLICE characters by code point.

A slice of Han or Hangul has the span of its code points for its unicode-range, which keeps the CSS short; a common
slice's span also holds rare characters. The rare slices come first in the CSS: the browser, which checks the last rule
first, looks a character up in the common slice of its span and falls back to the rare slice when the common one lacks
it.

Each slice closes over the GSUB features of horizontal text (LAYOUT_FEATURES), without `locl` (the region is the
family) and without the vertical tables (the client sets no vertical text). The outputs depend only on the inputs:
running it twice writes the same bytes.
"""

from __future__ import annotations

import argparse
import io
import os
import sys
from concurrent.futures import ProcessPoolExecutor
from dataclasses import dataclass
from pathlib import Path

from fontTools import subset
from fontTools.ttLib import TTFont

REGIONS = {"jp": "JP", "sc": "SC", "tc": "TC", "kr": "KR"}

# The GSUB and GPOS features a browser applies to horizontal text by default, which each slice closes over.
LAYOUT_FEATURES = ["calt", "ccmp", "clig", "kern", "liga", "mark", "mkmk", "rlig", "rvrn"]
DROP_TABLES = ["vhea", "vmtx", "VORG", "VVAR"]

# How many characters a slice of common characters and a slice of the others hold.
COMMON_SLICE = 500
RARE_SLICE = 1000

# The inline box of a character, as the emoji font's (ADR-0091 §10): one em, 88% above the baseline, so that the line
# keeps its height when the font swaps in for the system's (font-display: swap); Noto Sans CJK's own em box is the same.
ASCENT_OVERRIDE = "88%"
DESCENT_OVERRIDE = "12%"

HANGUL = range(0xAC00, 0xD7A4)
# The blocks of Han characters, each sliced on its own so that a span never crosses into another script.
HAN_BLOCKS = [
    range(0x3400, 0x4DC0),  # Extension A
    range(0x4E00, 0xA000),  # Unified Ideographs
    range(0xF900, 0xFB00),  # Compatibility Ideographs
    range(0x20000, 0x2A6E0),  # Extension B
    range(0x2A700, 0x2EBF0),  # Extensions C to F
    range(0x2F800, 0x2FA20),  # Compatibility Supplement
    range(0x30000, 0x323B0),  # Extensions G and H
]


def decode_rows(codec: str, lead: range, trail: range) -> list[int]:
    """The characters of the double-byte codes lead × trail of codec, in the order of the character set."""
    out = []
    for a in lead:
        for b in trail:
            try:
                s = bytes([a, b]).decode(codec)
            except UnicodeDecodeError:
                continue
            if len(s) == 1:
                out.append(ord(s))
    return out


# The Han characters each region uses most: JIS X 0208 level 1, GB 2312 level 1, Big5 level 1, and the hanja of
# KS X 1001; and the Hangul syllables of KS X 1001, which every region's slices share.
COMMON_HAN = {
    "jp": decode_rows("euc_jp", range(0xB0, 0xD0), range(0xA1, 0xFF)),
    "sc": decode_rows("gb2312", range(0xB0, 0xD8), range(0xA1, 0xFF)),
    "tc": decode_rows("big5", range(0xA4, 0xC7), [*range(0x40, 0x7F), *range(0xA1, 0xFF)]),
    "kr": decode_rows("euc_kr", range(0xCA, 0xFE), range(0xA1, 0xFF)),
}
COMMON_HANGUL = decode_rows("euc_kr", range(0xB0, 0xC9), range(0xA1, 0xFF))

# The slices of the characters outside Han and Hangul, by script: (name, test of a code point).
LATIN = [range(0x0000, 0x0370), range(0x1E00, 0x1F00), range(0x2000, 0x2070), range(0x20A0, 0x20D0)]
GREEK_CYRILLIC = [range(0x0370, 0x0530), range(0x1C80, 0x1C90), range(0x1D00, 0x1DC0), range(0x1F00, 0x2000)]
# The punctuation of CJK text, kana, and the full-width and half-width forms; the other CJK symbols (radicals, enclosed
# and squared forms, Bopomofo, Hangul jamo) go into a slice of their own.
KANA = [range(0x3000, 0x3100), range(0xFF00, 0xFFF0)]


@dataclass
class Slice:
    name: str
    code_points: list[int]
    # The unicode-range: the runs of the code points, or for a slice of Han or Hangul the span from the first to the
    # last, which keeps the CSS short.
    span: bool = False

    def unicode_range(self) -> str:
        cps = sorted(self.code_points)
        if self.span and cps[0] != cps[-1]:
            return f"U+{cps[0]:X}-{cps[-1]:X}"
        return format_runs(cps)


def in_any(cp: int, ranges: list[range]) -> bool:
    return any(cp in r for r in ranges)


def chunks(cps: list[int], size: int) -> list[list[int]]:
    return [cps[i : i + size] for i in range(0, len(cps), size)]


def slices_of(region: str, cmap: set[int]) -> list[Slice]:
    """The slices of a region's font, in the order of the CSS: the rare slices before the common ones."""
    han = {cp for cp in cmap if in_any(cp, HAN_BLOCKS)}
    hangul = {cp for cp in cmap if cp in HANGUL}
    rest = cmap - han - hangul
    latin = sorted(cp for cp in rest if in_any(cp, LATIN))
    greek = sorted(cp for cp in rest if in_any(cp, GREEK_CYRILLIC))
    symbols = sorted(cp for cp in rest if cp < 0x2E80 and cp not in latin and cp not in greek)
    kana = sorted(cp for cp in rest if in_any(cp, KANA))
    cjk = sorted(cp for cp in rest if cp >= 0x2E80 and cp not in kana)
    out = [
        Slice("latin", latin),
        Slice("greek-cyrillic", greek),
        Slice("symbols", symbols),
        Slice("kana", kana),
        Slice("cjk-symbols", cjk),
    ]

    rare: list[Slice] = []
    common: list[Slice] = []
    for kind, chars, wanted, blocks in (
        ("hangul", hangul, COMMON_HANGUL, [HANGUL]),
        ("han", han, COMMON_HAN[region], HAN_BLOCKS),
    ):
        often = sorted(set(wanted) & chars)
        others = sorted(chars - set(often))
        # Each slice holds the next characters by code point within one block, so that the spans of the rare slices
        # never overlap one another, nor those of the common ones.
        n = 0
        for block in blocks:
            for part in chunks([cp for cp in others if cp in block], RARE_SLICE):
                rare.append(Slice(f"{kind}-{n:02d}", part, span=True))
                n += 1
        n = 0
        for block in blocks:
            for part in chunks([cp for cp in often if cp in block], COMMON_SLICE):
                common.append(Slice(f"{kind}-common-{n:02d}", part, span=True))
                n += 1
    return [s for s in out if s.code_points] + rare + common


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


def subset_woff2(path: str, code_points: list[int]) -> bytes:
    options = subset.Options()
    options.layout_features = LAYOUT_FEATURES
    options.drop_tables += DROP_TABLES
    options.recalc_timestamp = False
    options.flavor = "woff2"
    font = subset.load_font(path, options)
    subsetter = subset.Subsetter(options)
    subsetter.populate(unicodes=code_points)
    subsetter.subset(font)
    out = io.BytesIO()
    font.save(out, reorderTables=True)
    return out.getvalue()


def job(args: tuple[str, str, list[int]]) -> bytes:
    return subset_woff2(*args)


def version_of(path: Path) -> str:
    font = TTFont(str(path), lazy=True)
    return str(font["name"].getDebugName(5)).split(";")[0].removeprefix("Version ")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--release", required=True, help="the release of notofonts/noto-cjk, recorded in the CSS")
    parser.add_argument("--font", action="append", required=True, help="region=path of its variable Noto Sans CJK")
    parser.add_argument("--out", type=Path, required=True, help="the directory of the WOFF2 files")
    parser.add_argument("--css", type=Path, required=True, help="the stylesheet of the @font-face rules to write")
    args = parser.parse_args()

    fonts: dict[str, Path] = {}
    for spec in args.font:
        region, _, path = spec.partition("=")
        if region not in REGIONS:
            raise SystemExit(f"cjk-fonts: unknown region {region!r}; want one of {', '.join(REGIONS)}")
        fonts[region] = Path(path)
    if set(fonts) != set(REGIONS):
        raise SystemExit(f"cjk-fonts: want a --font for each of {', '.join(REGIONS)}")

    plan: list[tuple[str, Slice]] = []
    for region in REGIONS:
        font = TTFont(str(fonts[region]), lazy=True)
        if "fvar" not in font or [a.axisTag for a in font["fvar"].axes] != ["wght"]:
            raise SystemExit(f"cjk-fonts: {fonts[region]} is not a variable font of the wght axis alone")
        cmap = set(font.getBestCmap())
        missing = [cp for cp in COMMON_HAN[region] + COMMON_HANGUL if cp not in cmap]
        if missing:
            raise SystemExit(f"cjk-fonts: {fonts[region]} lacks {len(missing)} common characters, such as U+{missing[0]:X}")
        plan.extend((region, s) for s in slices_of(region, cmap))

    # Each process holds a whole font of about 1 GB while it cuts a slice.
    with ProcessPoolExecutor(max_workers=min(8, os.cpu_count() or 1)) as pool:
        blobs = list(pool.map(job, [(str(fonts[r]), s.code_points) for r, s in plan]))

    args.out.mkdir(parents=True, exist_ok=True)
    wanted = set()
    sizes: dict[str, int] = {}
    for (region, s), blob in zip(plan, blobs):
        name = f"noto-sans-cjk-{region}-{s.name}.woff2"
        wanted.add(name)
        (args.out / name).write_bytes(blob)
        sizes[region] = sizes.get(region, 0) + len(blob)
    for stale in args.out.glob("noto-sans-cjk-*.woff2"):
        if stale.name not in wanted:
            stale.unlink()

    rel = os.path.relpath(args.out.resolve(), args.css.resolve().parent).replace(os.sep, "/")
    if not rel.startswith("."):
        rel = "./" + rel
    rules = []
    for region, s in plan:
        rules.append(
            f"/* {region} {s.name} */\n"
            "@font-face {\n"
            f'  font-family: "my Noto Sans CJK {REGIONS[region]}";\n'
            "  font-style: normal;\n"
            "  font-weight: 100 900;\n"
            "  font-display: swap;\n"
            f"  ascent-override: {ASCENT_OVERRIDE};\n"
            f"  descent-override: {DESCENT_OVERRIDE};\n"
            "  line-gap-override: 0%;\n"
            f'  src: url("{rel}/noto-sans-cjk-{region}-{s.name}.woff2") format("woff2");\n'
            f"  unicode-range: {s.unicode_range()};\n"
            "}\n"
        )
    versions = ", ".join(f"{REGIONS[r]} {version_of(p)}" for r, p in fonts.items())
    header = (
        "/* SPDX-License-Identifier: AGPL-3.0-only */\n"
        "/* Generated by scripts/cjk-fonts (`scripts/task.sh cjk:generate`); do not edit.\n"
        f" * Noto Sans CJK of notofonts/noto-cjk {args.release} ({versions}),\n"
        " * variable in weight, under the SIL Open Font License 1.1 (fonts/cjk/OFL-NotoSansCJK.txt). The browser loads a\n"
        " * slice only when the page draws a character of its unicode-range; for a character in two ranges it checks the\n"
        " * later rule first. */\n"
    )
    args.css.write_text(header + "\n" + "\n".join(rules), encoding="utf-8")

    for region, size in sizes.items():
        count = sum(1 for r, _ in plan if r == region)
        print(f"cjk-fonts: {REGIONS[region]} {size / 1_000_000:.2f} MB in {count} slices")
    print(f"cjk-fonts: {sum(sizes.values()) / 1_000_000:.2f} MB in all")
    return 0


if __name__ == "__main__":
    sys.exit(main())
