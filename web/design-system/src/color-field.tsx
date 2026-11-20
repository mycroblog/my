// SPDX-License-Identifier: AGPL-3.0-only

// The field of a color token in the server's theme (ADR-0059): its name, the line about what it colors, and its color
// in each scheme, light then dark, side by side where the field has the room for both pairs' lines on one line each
// (wider than the app's main column), one under the other elsewhere, each at the top of its column. A scheme's color is a swatch, which opens the platform's own picker, and the color as
// #rrggbb, typed or pasted; under them, each pair the token makes with another (text on a surface) as the pair looks,
// its contrast ratio and whether it meets WCAG AA's minimum for the pair. A pair under its minimum is the field's
// error: the glyph of an error and the text's color, never a hue, and the field of that scheme is invalid, as the
// server refuses the theme then too (ADR-0059 §2, which counts the same ratios). The ratio is cut, never rounded up,
// so that a pair just under its minimum never reads as meeting it. While what was typed is no color, the swatch and
// the pairs' pictures are dimmed and their verdicts blank: they would speak of a color the field no longer holds.
//
// For assistive technology: the field is a group named by the token and described by what it colors, each scheme a
// group named by the scheme, its color's field named by the token and the scheme (Accent text, Light) and described by
// the pairs' verdicts, and its picker named as such with them (Choose a color, Accent text, Light), so that a list of a
// screen's controls tells each token's apart. What the pair looks like is shown
// alone: its ratio and verdict say it.

import { contrast } from "@my/design-tokens";
import {
  useEffect,
  useEffectEvent,
  useId,
  useState,
  type HTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";
import { tv } from "tailwind-variants/lite";
import { FieldError } from "./form-page";
import { Icon } from "./icons";
import { Row, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";

/** A color token's value in each scheme, as #rrggbb. */
export type SchemeColors = { light: string; dark: string };

/** A pair the token makes with another token, which the server holds to a minimum contrast (contrastPairs). */
export type ColorPair = {
  /** The pair, from the token's side, in the app's words (On Surface, Under Text). */
  label: string;
  /** The other token's value in each scheme. */
  other: SchemeColors;
  /** The token's place in the pair: the text over the other, or the background under it. */
  role: "text" | "background";
  /** The contrast ratio the pair needs (4.5 for text, 3 for a mark). */
  minimum: number;
};

export type ColorFieldProps = {
  /**
   * The ids of its colors' fields, `<id>-light` and `<id>-dark`, by which what holds it gives the focus to the scheme
   * whose pair is under its minimum (`focusRefusedField`).
   */
  id?: string | undefined;
  /** The token's name (Accent text). */
  label: string;
  /** What it colors, in a line under its name. */
  about?: string | undefined;
  value: SchemeColors;
  /** The token's new value, once a scheme's color is a whole #rrggbb. */
  onChange: (value: SchemeColors) => void;
  pairs: readonly ColorPair[];
  /**
   * Hears whether what a scheme's field holds is a whole #rrggbb, as that comes and goes: while it is not, the token's
   * value is the last whole color, which the field no longer shows, and what holds it saves nothing.
   */
  onValidity?: ((scheme: "light" | "dark", whole: boolean) => void) | undefined;
};

/** The schemes, in their order. */
const SCHEMES = ["light", "dark"] as const;
type Scheme = (typeof SCHEMES)[number];

/** A color as the field takes it: # and six hex digits, the # optional; lowercased with its #, or none. */
const hexOf = (text: string): string | undefined => {
  const m = /^#?([0-9a-f]{6})$/i.exec(text.trim());
  return m?.[1] === undefined ? undefined : `#${m[1].toLowerCase()}`;
};

/** A ratio cut to two decimals, so that a pair just under its minimum never reads as meeting it. */
const cut = (ratio: number) => Math.floor(ratio * 100) / 100;

/**
 * The schemes of a field, which know the field's own width: side by side where both have the room of a few pairs'
 * lines, one under the other where they have not.
 */
function SchemeGrid({ className, ...rest }: HTMLAttributes<HTMLDivElement> & { className: string }) {
  return (
    <div className="@container">
      <div
        {...rest}
        className={`${className} grid grid-cols-[minmax(0,1fr)] gap-screen-row-gap @min-[40rem]:grid-cols-2 @min-[40rem]:gap-x-shell-edge`}
      />
    </div>
  );
}

/** The field of a color as #rrggbb: wide enough for its seven characters. */
function HexFrame({ className, ...rest }: InputHTMLAttributes<HTMLInputElement> & { className: string }) {
  return <input {...rest} className={`${className} w-[7.5rem] min-w-0`} />;
}

const field = tv({
  slots: {
    root: "text-body text-text",
    legend: "font-label [word-break:auto-phrase]",
    about: "text-caption text-muted-text [word-break:auto-phrase]",
    scheme: "text-caption text-muted-text",
    // The swatch: the platform's picker, drawn as the color on a control's corners, inside a line that keeps a light
    // color seen on a light page (3:1, WCAG 1.4.11).
    swatch: [
      "box-border size-control-form cursor-pointer appearance-none rounded-control border-solid border-1",
      "border-muted-text bg-transparent p-0 outline-hidden focus-visible:shadow-focus",
      "[&::-webkit-color-swatch-wrapper]:p-0 [&::-webkit-color-swatch]:rounded-[calc(var(--radius-control)-1px)]",
      "[&::-webkit-color-swatch]:border-0 [&::-moz-color-swatch]:rounded-[calc(var(--radius-control)-1px)]",
      "[&::-moz-color-swatch]:border-0",
    ],
    // What speaks of the last whole color while the field holds none.
    stale: "opacity-disabled",
    hex: [
      "box-border h-control-form rounded-control border-0 bg-surface-raised px-control-timeline-inline",
      "font-mono text-body text-text outline-hidden focus-visible:shadow-focus",
      "[[data-surface-raised]_&]:bg-surface",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    pairs: "list-none p-0 text-caption",
    // What the pair looks like: a picture of text on its background, in the type's face.
    sample: "font-sans font-strong",
    pass: "text-muted-text",
    fail: "text-text",
    ratio: "tabular-nums [word-break:auto-phrase]",
  },
});

/**
 * What a pair looks like, a picture of text in the one color on the other: a picture, so that a pair under its
 * minimum is shown as it is and not read as text of the page that fails its own contrast.
 */
function PairSample({ fg, bg, className }: { fg: string; bg: string; className: string }) {
  return (
    <svg aria-hidden="true" focusable="false" viewBox="0 0 28 20" width="2.25em" height="1.6em" className={className}>
      <rect width="28" height="20" rx="4" fill={bg} />
      <text x="14" y="14.5" textAnchor="middle" fontSize="12" fill={fg}>
        Aa
      </text>
    </svg>
  );
}

/** A scheme's color: its swatch and its #rrggbb, and the verdicts of its pairs. */
function SchemeColor({
  id,
  scheme,
  nameId,
  value,
  pairs,
  onChange,
  onValidity,
}: {
  /** The id of its color's field; one of its own without it. */
  id: string | undefined;
  scheme: Scheme;
  /** The id of the token's name, which names its controls with the scheme's. */
  nameId: string;
  value: SchemeColors;
  pairs: readonly ColorPair[];
  onChange: (color: string) => void;
  onValidity: ((whole: boolean) => void) | undefined;
}) {
  const s = field();
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  const titleId = useId();
  const ownId = useId();
  const fieldId = id ?? ownId;
  const pairsId = useId();
  const formatId = useId();
  const pickId = useId();
  const color = value[scheme];
  // What the reader typed, as they typed it; the token's value where that changes to another color than the text says
  // (from the swatch, or from outside), never rewriting the text under the caret.
  const [typed, setTyped] = useState(color);
  const [given, setGiven] = useState(color);
  if (given !== color) {
    setGiven(color);
    if (hexOf(typed) !== color) setTyped(color);
  }
  const [left, setLeft] = useState(false);
  const whole = hexOf(typed) !== undefined;
  // Said as it changes, not at every render of what holds the field.
  const tell = useEffectEvent((now: boolean) => {
    onValidity?.(now);
  });
  useEffect(() => {
    tell(whole);
  }, [whole]);
  // A color that is not whole is said once the reader leaves the field, not at each key.
  const misformed = !whole && left;
  const verdicts = pairs.map((pair) => {
    const other = pair.other[scheme];
    const ratio = cut(contrast(color, other));
    return { pair, ratio, meets: ratio >= pair.minimum, other };
  });
  const fails = verdicts.some((v) => !v.meets);
  const format = (n: number) => new Intl.NumberFormat(lang, { maximumFractionDigits: 2 }).format(n);
  return (
    <Stack gap="entry-line" pack="start" role="group" aria-labelledby={titleId}>
      <span id={titleId} className={s.scheme()}>
        {t(scheme === "light" ? "color.light" : "color.dark")}
      </span>
      <Row align="center" gap="entry-caption">
        <span id={pickId} hidden>
          {t("color.pick")}
        </span>
        <input
          type="color"
          value={color}
          aria-labelledby={`${pickId} ${nameId} ${titleId}`}
          className={whole ? s.swatch() : `${s.swatch()} ${s.stale()}`}
          onChange={(e) => {
            setTyped(e.currentTarget.value);
            setLeft(false);
            onChange(e.currentTarget.value);
          }}
        />
        <HexFrame
          id={fieldId}
          type="text"
          value={typed}
          spellCheck={false}
          autoComplete="off"
          dir="ltr"
          aria-labelledby={`${nameId} ${titleId}`}
          aria-invalid={(whole && fails) || misformed ? true : undefined}
          aria-describedby={misformed ? formatId : whole ? pairsId : undefined}
          className={s.hex()}
          onChange={(e) => {
            const next = e.currentTarget.value;
            setTyped(next);
            const hex = hexOf(next);
            if (hex !== undefined) {
              setLeft(false);
              onChange(hex);
            }
          }}
          onBlur={() => {
            setLeft(true);
          }}
        />
      </Row>
      {misformed && <FieldError id={formatId}>{t("color.format")}</FieldError>}
      <Stack as="ul" id={pairsId} gap="entry-line" className={s.pairs()}>
        {verdicts.map(({ pair, ratio, meets, other }) => {
          const [fg, bg] = pair.role === "text" ? [color, other] : [other, color];
          return (
            <Row as="li" key={pair.label} align="center" gap="entry-caption">
              <Row as="span" inline rigid>
                <PairSample fg={fg} bg={bg} className={whole ? s.sample() : `${s.sample()} ${s.stale()}`} />
              </Row>
              <Row as="span" grow align="center" justify="between" gap="entry-caption">
                <span>{pair.label}</span>
                {whole && (
                  <Row
                    as="span"
                    inline
                    rigid
                    align="center"
                    gap="entry-caption"
                    className={meets ? s.pass() : s.fail()}
                  >
                    <Row as="span" inline rigid>
                      <Icon name={meets ? "chosen" : "error"} place="caption" />
                    </Row>
                    <span className={s.ratio()}>
                      <span className="sr-only">{t(meets ? "color.meets" : "color.below")} </span>
                      {t(meets ? "color.ratio" : "color.short", {
                        ratio: format(ratio),
                        minimum: format(pair.minimum),
                      })}
                    </span>
                  </Row>
                )}
              </Row>
            </Row>
          );
        })}
      </Stack>
    </Stack>
  );
}

/**
 * A color token of the server's theme: its name and what it colors, then its color in light and in dark, each with
 * the contrast of its pairs against WCAG AA; a pair under its minimum makes that scheme's field invalid.
 */
export function ColorField({ id, label, about, value, onChange, pairs, onValidity }: ColorFieldProps): ReactNode {
  const s = field();
  const nameId = useId();
  const aboutId = useId();
  const needsId = useId();
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  const format = (n: number) => new Intl.NumberFormat(lang, { maximumFractionDigits: 2 }).format(n);
  // The minimum each pair needs, said once for the field: one for them all, or text's and a mark's.
  const minima = pairs.map((pair) => pair.minimum);
  const high = minima.length === 0 ? undefined : Math.max(...minima);
  const low = minima.length === 0 ? undefined : Math.min(...minima);
  return (
    <Stack
      role="group"
      aria-labelledby={nameId}
      aria-describedby={[about === undefined ? undefined : aboutId, high === undefined ? undefined : needsId]
        .filter((by) => by !== undefined)
        .join(" ")}
      gap="screen-row-gap"
      className={s.root()}
    >
      <Stack gap="hair">
        <span id={nameId} className={s.legend()}>
          {label}
        </span>
        {about !== undefined && (
          <p id={aboutId} className={s.about()}>
            {about}
          </p>
        )}
        {high !== undefined && low !== undefined && (
          <p id={needsId} className={s.about()}>
            {t("color.needs", { kinds: high === low ? "one" : "two", high: format(high), low: format(low) })}
          </p>
        )}
      </Stack>
      <SchemeGrid className="">
        {SCHEMES.map((scheme) => (
          <SchemeColor
            key={scheme}
            id={id === undefined ? undefined : `${id}-${scheme}`}
            scheme={scheme}
            nameId={nameId}
            value={value}
            pairs={pairs}
            onChange={(color) => {
              onChange({ ...value, [scheme]: color });
            }}
            onValidity={
              onValidity === undefined
                ? undefined
                : (whole) => {
                    onValidity(scheme, whole);
                  }
            }
          />
        ))}
      </SchemeGrid>
    </Stack>
  );
}
