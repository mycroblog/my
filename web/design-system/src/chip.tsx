// SPDX-License-Identifier: AGPL-3.0-only

// The chip, for an emoji reaction: a toggle on the raised surface holding a glyph and a count, a caption's size. On
// (the reader's own), it is knocked out: the text's color as its fill and the surface's as its text, with no hue, so
// that on differs from off by more than a tone. The count reads in the reader's locale and is isolated left-to-right
// (<bdi dir="ltr">), so that it keeps its place beside the glyph in either direction; the label carries what the chip
// says to assistive technology. Chips sit in a row that wraps (`ChipRow`). A chip may also lead to a place (`ChipLink`),
// and a badge is a label on the same surface that is none of these.

import type { ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { ButtonBase, type ButtonProps } from "./button";
import { compactCount } from "./icon-button";
import { Box, Row } from "./layout";
import { useReader } from "./locale";

const chip = tv({
  slots: {
    glyph: "text-[length:var(--chip-glyph)] leading-none",
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
      <Row as="span" inline aria-hidden="true" className={s.glyph()}>
        {glyph}
      </Row>
      <bdi aria-hidden="true" dir="ltr" lang={lang} className={s.count()}>
        {compactCount(count, lang)}
      </bdi>
    </ButtonBase>
  );
}

/** A row of chips that wraps, named for assistive technology as a group. */
export function ChipRow({ label, lang, children }: { label: string; lang?: string | undefined; children: ReactNode }) {
  return (
    <Row wrap gap="chip-spacing" role="group" aria-label={label} lang={lang}>
      {children}
    </Row>
  );
}

const link = tv({
  base: [
    "rounded-control bg-surface-raised px-chip-inline text-body leading-[var(--chip-height)] text-text no-underline",
    "outline-hidden data-hovered:bg-surface-raised-hover data-focus-visible:shadow-focus",
  ],
});

/**
 * A chip that leads to `href`: its words on the raised surface, then a `count` in the caption's size, muted. The
 * words stand in a box of their own, so that a word longer than the line breaks inside the chip whatever the caller
 * gives (bare words in a row would be a box the row cannot make narrower).
 */
export function ChipLink({ href, count, children }: { href: string; count?: string | undefined; children: ReactNode }) {
  return (
    <Row as={<Link href={href} className={link()} />} inline align="baseline" gap="screen-name-gap">
      <Box as="span">{children}</Box>
      {count !== undefined && (
        <>
          {" "}
          <Box as="span" text="caption" tone="muted">
            {count}
          </Box>
        </>
      )}
    </Row>
  );
}

const badge = tv({
  // On a raised surface (a section of a settings screen), it takes the page's surface.
  base: [
    "rounded-control bg-surface-raised px-screen-name-gap text-caption leading-[1.5] font-label text-text",
    "[[data-surface-raised]_&]:bg-surface",
  ],
});

/** A small label on the raised surface (follows you, a role, a bot), as the author wrote it, isolated. */
export function Badge({ children }: { children: ReactNode }) {
  return (
    <bdi dir="auto" className={badge()}>
      {children}
    </bdi>
  );
}
