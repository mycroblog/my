// SPDX-License-Identifier: AGPL-3.0-only
// A page of a few numbered sections read in order (the landing page's "what kind of place, what we agree to, how to
// join"), with a progress line above them: a numbered link to each section, the one being read marked
// `aria-current="location"` and the ones before it drawn as passed. It is a static page, not a wizard: every section is
// there at once, and the line only links to them and shows where the reader is.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { goToSection, useCurrentSection } from "./currentSection";

const sectionProgress = tv({
  slots: {
    base: "flex min-w-0 flex-col gap-8 [&_h2]:scroll-mt-6 [&_h3]:scroll-mt-6",
    list: "flex items-start",
    item: [
      "relative flex min-w-0 flex-1 flex-col items-center",
      // The line from the step before to this one, behind the numbers; in the accent once the reader has reached it.
      "not-first:before:absolute not-first:before:top-4 not-first:before:-inset-s-1/2 not-first:before:h-0.5 not-first:before:w-full not-first:before:bg-border",
      "data-reached:not-first:before:bg-accent forced-colors:data-reached:not-first:before:bg-[Highlight]",
      "motion-safe:before:transition-colors motion-safe:before:duration-normal motion-safe:before:ease-standard",
    ],
    link: "group relative z-10 flex max-w-full flex-col items-center gap-1 rounded-md px-1 text-center text-sm text-fg-muted outline-none hover:text-fg focus-visible:focus-ring aria-[current=location]:font-bold aria-[current=location]:text-fg",
    number: [
      "flex size-8 items-center justify-center rounded-full border-2 border-border bg-surface font-bold tabular-nums",
      // A part before the one being read: passed, in the accent; the one being read is filled.
      "group-data-passed:border-accent group-data-passed:text-accent",
      "group-aria-[current=location]:bg-accent group-aria-[current=location]:text-accent-fg",
      "forced-colors:group-aria-[current=location]:border-[Highlight] forced-colors:group-aria-[current=location]:bg-[Highlight] forced-colors:group-aria-[current=location]:text-[HighlightText]",
      "motion-safe:transition-colors motion-safe:duration-normal motion-safe:ease-standard",
    ],
    title: "max-w-full wrap-anywhere text-pretty",
  },
});

/** One section of the page. */
export type SectionProgressPart = {
  /** The `id` of the section's heading, which the link points at. */
  id: string;
  /** The words of the link: the section's title. */
  title: string;
};

export type SectionProgressProps = {
  /** The accessible name of the progress line ("Parts of this page"). */
  label: string;
  /** The sections in their order; their headings, with these ids, are among `children`. */
  parts: readonly SectionProgressPart[];
  /** The sections (`Section` with `headingId`), in the order of `parts`. */
  children: ReactNode;
};

/**
 * The progress line and the sections under it. Choosing a part scrolls to its heading (smoothly only unless the reader
 * prefers reduced motion) and moves focus there; the line follows the section being read as the page scrolls.
 */
export function SectionProgress({ label, parts, children }: SectionProgressProps) {
  const slots = sectionProgress();
  const [currentId, setCurrentId] = useCurrentSection(parts.map((part) => part.id));
  const currentIndex = parts.findIndex((part) => part.id === currentId);
  return (
    <div className={slots.base()}>
      <nav aria-label={label}>
        <ol className={slots.list()}>
          {parts.map((part, index) => {
            const reached = currentIndex >= index;
            return (
              <li key={part.id} data-reached={reached || undefined} className={slots.item()}>
                <a
                  href={`#${part.id}`}
                  aria-current={part.id === currentId ? "location" : undefined}
                  data-passed={currentIndex > index || undefined}
                  className={slots.link()}
                  onClick={(event) => {
                    if (goToSection(event, part.id)) setCurrentId(part.id);
                  }}
                >
                  {/* The number only shows the order, which the list already gives. */}
                  <span aria-hidden="true" className={slots.number()}>
                    {index + 1}
                  </span>
                  <span className={slots.title()}>{part.title}</span>
                </a>
              </li>
            );
          })}
        </ol>
      </nav>
      {children}
    </div>
  );
}
