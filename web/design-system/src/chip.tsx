// SPDX-License-Identifier: AGPL-3.0-only

// The chip, for an emoji reaction: a toggle on the raised surface holding a glyph and a count, a caption's size. On
// (the reader's own), it is knocked out: the text's color as its fill and the surface's as its text, with no hue, so
// that on differs from off by more than a tone. The count reads in the reader's locale and is isolated left-to-right
// (<bdi dir="ltr">), so that it keeps its place beside the glyph in either direction; the label carries what the chip
// says to assistive technology. Chips sit in a row that wraps (`ChipRow`).

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { ButtonBase, type ButtonProps } from "./button";
import { compactCount } from "./icon-button";
import { useReader } from "./locale";

const chip = tv({
  slots: {
    row: "flex flex-wrap gap-chip-spacing",
    glyph: "inline-flex text-[length:var(--chip-glyph)] leading-none",
    count: "tabular-nums",
  },
});

export type ChipProps = Omit<ButtonProps, "kind" | "size" | "busy" | "children" | "aria-label"> & {
  /** What the chip says to assistive technology: its glyph, its count and whether it is on. */
  label: string;
  /** The glyph (an emoji). */
  glyph: ReactNode;
  /** How many there are. */
  count: number;
  /** On: the reader's own. */
  pressed: boolean;
  /** The reader's locale, for the count; the I18nProvider's by default. */
  locale?: string | undefined;
};

export function Chip({ label, glyph, count, pressed, locale, ...rest }: ChipProps) {
  const { lang, direction } = useReader(locale);
  const s = chip();
  return (
    <ButtonBase {...rest} dir={direction} kind="chip" size="chip" pressed={pressed} aria-label={label}>
      <span aria-hidden="true" className={s.glyph()}>
        {glyph}
      </span>
      <bdi aria-hidden="true" dir="ltr" lang={lang} className={s.count()}>
        {compactCount(count, lang)}
      </bdi>
    </ButtonBase>
  );
}

/** A row of chips that wraps, named for assistive technology as a group. */
export function ChipRow({ label, lang, children }: { label: string; lang?: string | undefined; children: ReactNode }) {
  return (
    <div role="group" aria-label={label} lang={lang} className={chip().row()}>
      {children}
    </div>
  );
}
