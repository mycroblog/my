# SPDX-License-Identifier: AGPL-3.0-only
"""Splits Noto Color Emoji into the web fonts of the web client (ADR-0091 §9, §10).

`scripts/task.sh emoji:generate` downloads emoji-test.txt (EMOJI_VERSION) and the two pinned Noto Color Emoji files,
checks their digests, and runs this through uv (uv.lock) after scripts/emoji-table:

    uv run --frozen --project scripts/emoji-fonts python -I scripts/emoji-fonts/emoji_fonts.py \
      --emoji-test emoji-test.txt --version 18.0.0 --colrv1 Noto-COLRv1.ttf --svg NotoColorEmoji-Regular.ttf \
      --out app/ui/src/fonts/emoji --css app/ui/src/emoji.css

It writes, for each split, a COLRv1 and an OpenType-SVG WOFF2 into --out, and the @font-face rules of the family
"my Noto Color Emoji" into --css. The splits follow the groups of emoji-test.txt, with Flags split into the country
flags (pairs of regional indicators) and the other flags; every sequence goes whole into the split of its group, and a
split's unicode-range is every code point of its sequences but those of TEXT_CODE_POINTS.

It fails unless both fonts draw every fully-qualified and component sequence of emoji-test.txt as one glyph, in the
whole font and in the split of the sequence, shaped by HarfBuzz. The outputs depend only on the inputs: running it
twice writes the same bytes.
"""

from __future__ import annotations

import argparse
import io
import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path

import uharfbuzz as hb
from fontTools import subset
from fontTools.ttLib import TTFont

FAMILY = "my Noto Color Emoji"

# Code points that body text uses as ordinary characters, left out of every unicode-range so that the text fonts draw
# them (ADR-0091 §10): the keycap bases (# * 0-9) and the symbols whose default presentation in emoji-test.txt is text and
# that appear in running text. Their emoji presentation (#️⃣, ©️) falls back to the OS emoji font.
TEXT_CODE_POINTS: frozenset[int] = frozenset(
    [
        0x0023,  # # number sign
        0x002A,  # * asterisk
        *range(0x0030, 0x003A),  # 0-9
        0x00A9,  # © copyright
        0x00AE,  # ® registered
        0x203C,  # ‼ double exclamation mark
        0x2049,  # ⁉ exclamation question mark
        0x2122,  # ™ trade mark
        0x2139,  # ℹ information
        *range(0x2194, 0x219A),  # ↔ ↕ ↖ ↗ ↘ ↙ arrows
        0x21A9,  # ↩ right arrow curving left
        0x21AA,  # ↪ left arrow curving right
        0x25AA,  # ▪ black small square
        0x25AB,  # ▫ white small square
        0x25B6,  # ▶ play button
        0x25C0,  # ◀ reverse button
        0x25FB,  # ◻ white medium square
        0x25FC,  # ◼ black medium square
        0x2733,  # ✳ eight-spoked asterisk (the brand's spark, @my/ui Spark)
        0x2734,  # ✴ eight-pointed star
        0x2747,  # ❇ sparkle
        0x2934,  # ⤴ right arrow curving up
        0x2935,  # ⤵ right arrow curving down
        0x3030,  # 〰 wavy dash
        0x303D,  # 〽 part alternation mark
    ]
)

REGIONAL_INDICATORS = range(0x1F1E6, 0x1F200)

# The layout of the --css file's metrics: the inline box of an emoji is one em tall (88% above the baseline, 12% below),
# so that the line box keeps its height when the font swaps in for the OS emoji (font-display: swap); the glyph itself
# still draws at its own size.
ASCENT_OVERRIDE = "88%"
DESCENT_OVERRIDE = "12%"

LINE = re.compile(r"^([0-9A-F ]+?)\s*;\s*(fully-qualified|minimally-qualified|unqualified|component)\s*#")


@dataclass
class Split:
    """One split of the font: the sequences of a group of emoji-test.txt."""

    name: str
    sequences: list[tuple[int, ...]] = field(default_factory=list)

    def code_points(self) -> set[int]:
        return {cp for seq in self.sequences for cp in seq}

    def unicode_range(self) -> list[int]:
        return sorted(self.code_points() - TEXT_CODE_POINTS)


def slug(group: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", group.lower()).strip("-")


def read_splits(path: Path) -> list[Split]:
    """Reads the fully-qualified and component sequences of emoji-test.txt into splits, in the file's order."""
    splits: dict[str, Split] = {}
    group = ""
    for raw in path.read_text(encoding="utf-8").splitlines():
        if raw.startswith("# group:"):
            group = raw.removeprefix("# group:").strip()
            continue
        m = LINE.match(raw)
        if m is None or m.group(2) not in ("fully-qualified", "component"):
            continue
        if group == "":
            raise SystemExit(f"emoji-fonts: a sequence before the first group in {path}: {raw}")
        seq = tuple(int(cp, 16) for cp in m.group(1).split())
        name = slug(group)
        if name == "flags" and len(seq) == 2 and all(cp in REGIONAL_INDICATORS for cp in seq):
            name = "country-flags"
        splits.setdefault(name, Split(name)).sequences.append(seq)
    if not splits:
        raise SystemExit(f"emoji-fonts: no sequences in {path}")
    return list(splits.values())


def glyphs_of(face_blob: bytes, sequences: list[tuple[int, ...]]) -> list[str]:
    """Shapes each sequence with HarfBuzz and returns a line for each that is not exactly one glyph other than .notdef."""
    font = hb.Font(hb.Face(hb.Blob(face_blob)))
    bad = []
    for seq in sequences:
        buf = hb.Buffer()
        buf.add_codepoints(list(seq))
        buf.guess_segment_properties()
        hb.shape(font, buf)
        gids = [info.codepoint for info in buf.glyph_infos]
        if len(gids) != 1 or gids[0] == 0:
            bad.append(f"{' '.join(f'{cp:04X}' for cp in seq)} ({''.join(map(chr, seq))}): glyphs {gids}")
    return bad


def subset_font(path: Path, split: Split, drop_tables: list[str]) -> tuple[bytes, bytes]:
    """Returns the split of the font at path as an uncompressed font (for shaping) and as WOFF2."""
    options = subset.Options()
    options.layout_features = ["*"]
    options.drop_tables += drop_tables
    options.recalc_timestamp = False
    font = subset.load_font(str(path), options)
    subsetter = subset.Subsetter(options)
    subsetter.populate(unicodes=sorted(split.code_points()))
    subsetter.subset(font)
    plain = io.BytesIO()
    font.flavor = None
    font.save(plain, reorderTables=True)
    woff2 = io.BytesIO()
    font.flavor = "woff2"
    font.save(woff2, reorderTables=True)
    return plain.getvalue(), woff2.getvalue()


def version_of(path: Path) -> str:
    font = TTFont(str(path), lazy=True)
    return str(font["name"].getDebugName(5)).split(";")[0].removeprefix("Version ")


def format_range(code_points: list[int]) -> str:
    """Formats sorted code points as a unicode-range descriptor value, merging runs into ranges."""
    parts = []
    i = 0
    while i < len(code_points):
        j = i
        while j + 1 < len(code_points) and code_points[j + 1] == code_points[j] + 1:
            j += 1
        start, end = code_points[i], code_points[j]
        parts.append(f"U+{start:X}" if start == end else f"U+{start:X}-{end:X}")
        i = j + 1
    return ", ".join(parts)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--emoji-test", type=Path, required=True, help="the emoji-test.txt of --version")
    parser.add_argument("--version", required=True, help="the Unicode emoji version, recorded in the CSS")
    parser.add_argument("--colrv1", type=Path, required=True, help="Noto-COLRv1.ttf of googlefonts/noto-emoji")
    parser.add_argument("--svg", type=Path, required=True, help="NotoColorEmoji-Regular.ttf of google/fonts")
    parser.add_argument("--out", type=Path, required=True, help="the directory of the WOFF2 files")
    parser.add_argument("--css", type=Path, required=True, help="the stylesheet of the @font-face rules to write")
    args = parser.parse_args()

    splits = read_splits(args.emoji_test)
    every = [seq for split in splits for seq in split.sequences]
    formats = [
        # (the file name's part, the font, the tables it drops, the tech() of the src)
        ("colrv1", args.colrv1, ["SVG "], "color-COLRv1"),
        ("svg", args.svg, ["COLR", "CPAL"], "color-SVG"),
    ]

    failures = []
    for _, path, _, _ in formats:
        for line in glyphs_of(path.read_bytes(), every):
            failures.append(f"{path.name}: {line}")
    if failures:
        print(f"emoji-fonts: the fonts do not draw every emoji of {args.emoji_test} as one glyph:", file=sys.stderr)
        print("\n".join(failures), file=sys.stderr)
        return 1

    args.out.mkdir(parents=True, exist_ok=True)
    wanted = set()
    sizes: dict[str, dict[str, int]] = {}
    for kind, path, drop, _ in formats:
        for split in splits:
            plain, woff2 = subset_font(path, split, drop)
            for line in glyphs_of(plain, split.sequences):
                failures.append(f"{path.name} split {split.name}: {line}")
            name = f"noto-emoji-{kind}-{split.name}.woff2"
            wanted.add(name)
            (args.out / name).write_bytes(woff2)
            sizes.setdefault(kind, {})[split.name] = len(woff2)
    if failures:
        print("emoji-fonts: a split does not draw its emoji as one glyph:", file=sys.stderr)
        print("\n".join(failures), file=sys.stderr)
        return 1
    for stale in args.out.glob("noto-emoji-*.woff2"):
        if stale.name not in wanted:
            stale.unlink()

    rel = os.path.relpath(args.out.resolve(), args.css.resolve().parent).replace(os.sep, "/")
    if not rel.startswith("."):
        rel = "./" + rel
    rules = []
    for split in splits:
        srcs = ",\n    ".join(
            f'url("{rel}/noto-emoji-{kind}-{split.name}.woff2") format("woff2") tech({tech})'
            for kind, _, _, tech in formats
        )
        rules.append(
            "/* " + split.name + " */\n"
            "@font-face {\n"
            f'  font-family: "{FAMILY}";\n'
            "  font-style: normal;\n"
            "  font-weight: 100 900;\n"
            "  font-display: swap;\n"
            f"  ascent-override: {ASCENT_OVERRIDE};\n"
            f"  descent-override: {DESCENT_OVERRIDE};\n"
            "  line-gap-override: 0%;\n"
            f"  src:\n    {srcs};\n"
            f"  unicode-range: {format_range(split.unicode_range())};\n"
            "}\n"
        )
    header = (
        "/* SPDX-License-Identifier: AGPL-3.0-only */\n"
        "/* Generated by scripts/emoji-fonts (`scripts/task.sh emoji:generate`); do not edit.\n"
        f" * Noto Color Emoji for Unicode emoji {args.version} (ADR-0091): COLRv1 {version_of(args.colrv1)}"
        f" and OpenType-SVG {version_of(args.svg)},\n"
        " * one split per group of emoji-test.txt, under the SIL Open Font License 1.1 (fonts/emoji/OFL.txt).\n"
        " * The browser loads a split only when the page draws one of the code points of its unicode-range, and picks\n"
        " * the first src whose tech() it supports: COLRv1 in Chromium and Firefox, OpenType-SVG in WebKit. */\n"
    )
    args.css.write_text(header + "\n" + "\n".join(rules), encoding="utf-8")

    for kind, by_split in sizes.items():
        total = sum(by_split.values())
        detail = ", ".join(f"{name} {size / 1000:.0f} kB" for name, size in by_split.items())
        print(f"emoji-fonts: {kind} {total / 1_000_000:.2f} MB in {len(by_split)} splits ({detail})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
