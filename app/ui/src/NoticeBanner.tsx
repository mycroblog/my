// SPDX-License-Identifier: AGPL-3.0-only
// A notice that the server keeps at the top of every screen and that the user cannot close (the service closure's
// announcement, FR-808): a named region with a bold headline, the details below it, and the links that lead to
// what the notice asks of the user. It is static content, not a live region: it is there when the page loads and
// is read in its place, so it does not interrupt (web-client/README.md Y7). It scrolls away with the page, as the
// offline bar does on a short screen, because what stays in place must not take the height of a small screen
// (WCAG 1.4.10). It pads for the left and right safe-area insets and, being first in the page, for the top one.
import type { ComponentPropsWithRef, ReactNode } from "react";
import { tv } from "tailwind-variants";
import { WarningIcon } from "./Icons";

const noticeBanner = tv({
  slots: {
    region:
      "flex flex-col gap-1 bg-warning-bg px-[max(1rem,env(safe-area-inset-left),env(safe-area-inset-right))] pt-[calc(0.5rem+env(safe-area-inset-top))] pb-2 text-sm text-fg",
    headline: "flex items-center gap-2 font-bold",
    icon: "shrink-0 text-warning [&>svg]:size-4.5",
    // A message of the operator is free text with long words; it wraps rather than scroll the page.
    details: "flex min-w-0 flex-col gap-1 wrap-anywhere",
    // The accent of a link has too little contrast on the warning tint, so a link takes the text color and is
    // underlined, so that color is not its only cue (WCAG 1.4.1).
    actions: "flex flex-wrap gap-x-4 gap-y-1 [&_a]:text-fg [&_a]:underline [&_a[data-hovered]]:text-fg",
  },
});

export type NoticeBannerProps = Omit<ComponentPropsWithRef<"section">, "children" | "className" | "style" | "aria-label"> & {
  /** The region's name, for the landmark list ("Notice of the server's closure"). */
  label: string;
  /** The one line that says what the notice is about. */
  headline: ReactNode;
  /** The details below the headline: paragraphs. */
  children?: ReactNode;
  /** The links to what the notice asks the user to do, after the details. */
  actions?: ReactNode;
};

export function NoticeBanner({ label, headline, children, actions, ...props }: NoticeBannerProps) {
  const slots = noticeBanner();
  return (
    <section {...props} aria-label={label} className={slots.region()}>
      <p className={slots.headline()}>
        <span className={slots.icon()}>
          <WarningIcon filled />
        </span>
        <span>{headline}</span>
      </p>
      {children !== undefined && <div className={slots.details()}>{children}</div>}
      {actions !== undefined && <div className={slots.actions()}>{actions}</div>}
    </section>
  );
}
