// SPDX-License-Identifier: AGPL-3.0-only
// The top of an editorial page (the landing page): a kicker with the mark and the address, the page's title at the
// display size, a lede, a line of facts and the actions, on generous space, at the measure of running text. It carries
// no surface of its own: the type and the space make the hierarchy (app/ui/README.md "Spacing, radius, shadow").
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";

const masthead = tv({
  slots: {
    // The container whose width the title's and the lede's sizes follow (@3xl).
    base: "@container",
    inner: "flex flex-col gap-6 py-16 @3xl:py-24",
    kicker: "flex min-w-0 items-center gap-3 text-sm text-fg-muted",
    mark: "flex shrink-0",
    // The administrator's summary: larger than body text, in the body color, at the measure of running text. Its
    // paragraph (`dir="auto"`) lines up with the page's start in either direction (G9).
    lede: "max-w-prose text-xl text-pretty text-fg @3xl:text-2xl *:align-match-parent",
    facts: [
      "flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-fg-muted",
      // A middle dot between the facts, drawn by CSS so that it is not read out.
      "[&>li]:flex [&>li]:items-center [&>li]:gap-2 [&>li:not(:first-child)]:before:content-['·'] [&>li:not(:first-child)]:before:text-fg-muted",
    ],
    actions: "flex flex-wrap items-center gap-x-6 gap-y-3 pt-2",
    image: "pt-4",
  },
});

/** One fact of the masthead's line: the number of members, since when, who runs the place. */
export type MastheadFact = {
  key: string;
  content: ReactNode;
};

export type MastheadProps = {
  /** The page's title (`PageHeading` with `display`): the `h1`. */
  heading: ReactNode;
  /** A small decorative mark before the kicker (the server's icon or the wordmark), hidden from assistive technology by its caller. */
  mark?: ReactNode;
  /** The words above the title (the server's domain). */
  kicker?: ReactNode;
  /** The lede under the title (the server's summary): a paragraph, which the caller gives its direction and language. */
  lede?: ReactNode;
  /** The facts on one line under the lede, separated by middle dots. */
  facts?: readonly MastheadFact[];
  /** The actions: the primary one first. */
  actions?: ReactNode;
  /** A picture under the actions (the server's thumbnail, `BannerImage`). */
  image?: ReactNode;
};

/** No facts: one array for every render, so the default does not change between them. */
const noFacts: readonly MastheadFact[] = [];

/** The masthead of an editorial page. */
export function Masthead({ heading, mark, kicker, lede, facts = noFacts, actions, image }: MastheadProps) {
  const slots = masthead();
  return (
    <div className={slots.base()}>
      <div className={slots.inner()}>
        {(mark !== undefined || kicker !== undefined) && (
          <p className={slots.kicker()}>
            {mark !== undefined && <span className={slots.mark()}>{mark}</span>}
            {kicker}
          </p>
        )}
        {heading}
        {lede !== undefined && <div className={slots.lede()}>{lede}</div>}
        {facts.length > 0 && (
          <ul className={slots.facts()}>
            {facts.map((fact) => (
              <li key={fact.key}>{fact.content}</li>
            ))}
          </ul>
        )}
        {actions !== undefined && <div className={slots.actions()}>{actions}</div>}
        {image !== undefined && <div className={slots.image()}>{image}</div>}
      </div>
    </div>
  );
}
