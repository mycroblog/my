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
    "data-pressed:translate-y-press data-pressed:bg-surface-raised-pressed",
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
  base: "rounded-control px-screen-name-gap text-caption leading-[1.5] font-label",
  variants: {
    // The tint and the text of a badge given a hue (a role's): one of the fixed pairs of the tokens, at 4.5:1 in either
    // scheme; without one, the raised surface, which on a raised surface (a settings section) takes the page's.
    hue: {
      none: "bg-surface-raised text-text [[data-surface-raised]_&]:bg-surface",
      neutral: "bg-badge-neutral text-on-badge-neutral",
      red: "bg-badge-red text-on-badge-red",
      orange: "bg-badge-orange text-on-badge-orange",
      yellow: "bg-badge-yellow text-on-badge-yellow",
      green: "bg-badge-green text-on-badge-green",
      teal: "bg-badge-teal text-on-badge-teal",
      blue: "bg-badge-blue text-on-badge-blue",
      purple: "bg-badge-purple text-on-badge-purple",
      pink: "bg-badge-pink text-on-badge-pink",
    },
  },
  defaultVariants: { hue: "none" },
});

/** The hues a badge may take, each a fixed tint and text. */
export type BadgeHue = "neutral" | "red" | "orange" | "yellow" | "green" | "teal" | "blue" | "purple" | "pink";

/**
 * A small label on the raised surface (follows you, a role, a bot), as the author wrote it, isolated; `hue`, one of
 * the badges' fixed colors (a role's badge), which only tells one badge from another: the label carries its meaning.
 */
export function Badge({ hue, children }: { hue?: BadgeHue | undefined; children: ReactNode }) {
  return (
    <bdi dir="auto" className={badge({ hue: hue ?? "none" })}>
      {children}
    </bdi>
  );
}
