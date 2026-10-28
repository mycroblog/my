// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef, ReactNode } from "react";
import { tv } from "tailwind-variants";

const emptyState = tv({
  slots: {
    base: "flex flex-col items-center gap-3 px-4 py-10 text-center",
    illustration: "grid size-22 place-items-center rounded-2xl bg-surface-sunken text-accent [&>svg]:size-11",
    heading: "font-serif text-2xl font-medium tracking-tight text-fg",
    description: "max-w-[60ch] text-sm text-fg-muted",
    action: "mt-2 flex flex-wrap justify-center gap-2",
  },
});

/** Heading levels an empty state can take; the page title is the only h1 (web-client/README.md Y5). */
export type HeadingLevel = 2 | 3 | 4 | 5 | 6;

export type EmptyStateProps = Omit<ComponentPropsWithRef<"div">, "children" | "title"> & {
  /** The short message ("No posts yet"). */
  heading: ReactNode;
  /** The heading's level in the page outline: one below the heading of the region the empty state fills. */
  headingLevel?: HeadingLevel;
  /** What the user can do about it, or why the list is empty. */
  description?: ReactNode;
  /** A decorative picture (an icon or an SVG). It is hidden from assistive technology; the heading carries the meaning. */
  illustration?: ReactNode;
  /** The way out: a button or a link from @my/ui. */
  action?: ReactNode;
};

/** What a list, a timeline or a search shows when it has nothing to show. */
export function EmptyState({
  heading,
  headingLevel = 2,
  description,
  illustration,
  action,
  className,
  ...props
}: EmptyStateProps) {
  const slots = emptyState();
  const Heading = `h${String(headingLevel)}` as `h${HeadingLevel}`;
  return (
    <div {...props} className={slots.base({ className })}>
      {illustration !== undefined && (
        <div aria-hidden="true" className={slots.illustration()}>
          {illustration}
        </div>
      )}
      <Heading className={slots.heading()}>{heading}</Heading>
      {description !== undefined && <p className={slots.description()}>{description}</p>}
      {action !== undefined && <div className={slots.action()}>{action}</div>}
    </div>
  );
}
