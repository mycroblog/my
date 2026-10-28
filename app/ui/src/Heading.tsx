// SPDX-License-Identifier: AGPL-3.0-only
// A heading whose level (the page outline) and look (the type role) are separate props, so that no level is skipped for
// its look (web-client/16-look-states-and-wording.md G5). The look follows the level unless `size` says otherwise.
import type { ComponentPropsWithRef } from "react";
import { tv } from "tailwind-variants";
import { Spark, type SparkMark } from "./Spark";

const heading = tv({
  variants: {
    size: {
      page: "text-2xl font-bold",
      section: "text-lg font-bold",
      subsection: "font-bold",
      label: "text-sm font-bold",
      medium: "font-bold",
      meta: "text-sm font-normal",
    },
    tone: { fg: "text-fg", muted: "text-fg-muted" },
    wrap: { anywhere: "wrap-anywhere", truncate: "truncate", pretty: "text-pretty" },
    mono: { true: "font-mono" },
    shrink: { true: "min-w-0" },
    focusable: { true: "outline-none" },
  },
});

export type HeadingProps = Omit<ComponentPropsWithRef<"h2">, "className" | "style" | "tabIndex"> & {
  /** The level in the page outline: `1` is the page title (one per page, `PageHeading`), then one below the region it heads. */
  level: 1 | 2 | 3 | 4 | 5 | 6;
  /**
   * The type role: `page` (`text-2xl`), `section` (`text-lg`, the region title of G5), `subsection` (body size, a title
   * inside a region or a row), `label` (`text-sm font-medium`, a small group title), `meta` (`text-sm`, a heading that names a dated item beside its details). By default level 1 is `page`, level 2
   * `section`, and the others `subsection`.
   */
  size?: "page" | "section" | "subsection" | "label" | "medium" | "meta";
  /** The color token; inherited when unset. `fg` restores the body color inside a muted region. */
  tone?: "fg" | "muted";
  /** `anywhere` for a heading that is a name, a domain or a code; `truncate` for one line; `pretty` to balance a short title. */
  wrap?: "anywhere" | "truncate" | "pretty";
  /** The monospaced family, for a heading that is a code or an identifier. */
  mono?: boolean;
  /** Lets the heading shrink below its content's width in a row (`min-w-0`). */
  shrink?: boolean;
  /**
   * Receives focus from script (`tabIndex={-1}`, no outline), for the heading that focus moves to after an action
   * removes the control that had it. It never enters the tab order.
   */
  focusable?: boolean;
  /** A decorative mark (`✳` or `✧`, mango) after the text, hidden from assistive technology. The screen decides whether to use it. */
  spark?: SparkMark;
};

const defaultSize = {
  1: "page",
  2: "section",
  3: "subsection",
  4: "subsection",
  5: "subsection",
  6: "subsection",
} as const;

/** An `h1`–`h6` in a type role. */
export function Heading({
  level,
  size,
  tone,
  wrap,
  mono,
  shrink,
  focusable = false,
  spark,
  children,
  ...props
}: HeadingProps) {
  const Element = `h${String(level)}` as `h${typeof level}`;
  return (
    <Element
      {...props}
      tabIndex={focusable ? -1 : undefined}
      className={heading({ size: size ?? defaultSize[level], tone, wrap, mono, shrink, focusable })}
    >
      {children}
      {spark !== undefined && <Spark mark={spark} />}
    </Element>
  );
}
