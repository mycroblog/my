// SPDX-License-Identifier: AGPL-3.0-only
// A page of a few numbered sections read in order (the landing page's "what kind of place, what we agree to, how to
// join"), with a progress line above them: a numbered link to each section, the one being read marked
// `aria-current="location"` and the ones before it drawn as passed. It is a static page, not a wizard: every section is
// there at once, and the line only links to them and shows where the reader is. The line sticks under the top bar while
// the sections scroll by (not on a short screen, where it would take too much of it), so it always shows where the
// reader is; on a narrow page (below @md) it shows only the numbers, the titles staying the links' names.
import { type ReactNode, useRef } from "react";
import { tv } from "tailwind-variants";
import { useBlockSizeProperty } from "./blockSizeProperty";
import { goToSection, useCurrentSection } from "./currentSection";

const sectionProgress = tv({
  slots: {
    base: "@container flex min-w-0 flex-col gap-8",
    // Under the top bar, opaque over the sections that scroll under it. Its own stacking context, above the sections'
    // (`sections`) and below the top bar's (z-10), so that nothing of it draws over the bar or its menu.
    nav: "sticky top-under-top-bar z-1 border-b border-border bg-surface py-1 short:static @md:py-2",
    list: "flex items-start",
    // The sections in a stacking context of their own, so that what they lift (a link card's button) stays under the line.
    sections:
      "isolate flex min-w-0 flex-col gap-8 [&_h2]:scroll-mt-under-section-progress [&_h3]:scroll-mt-under-section-progress",
    item: [
      "relative flex min-w-0 flex-1 flex-col items-center",
      // The line from the step before to this one, behind the numbers (through their middle: the number is centred
      // in its 44 px link on a narrow page, at the top of the link from @md); in the accent once the reader has reached it.
      "not-first:before:absolute not-first:before:top-5 @md:not-first:before:top-4 not-first:before:-inset-s-1/2 not-first:before:h-0.5 not-first:before:w-full not-first:before:bg-border",
      "data-reached:not-first:before:bg-accent forced-colors:data-reached:not-first:before:bg-[Highlight]",
      "motion-safe:before:transition-colors motion-safe:before:duration-normal motion-safe:before:ease-standard",
    ],
    link: "group relative z-1 flex min-h-11 max-w-full min-w-11 flex-col items-center justify-center gap-1 rounded-md px-1 text-center text-sm text-fg-muted outline-none hover:text-fg focus-visible:focus-ring aria-[current=location]:font-bold aria-[current=location]:text-fg",
    number: [
      "flex size-8 items-center justify-center rounded-full border-2 border-border bg-surface font-bold tabular-nums",
      // A part before the one being read: passed, in the accent; the one being read is filled.
      "group-data-passed:border-accent group-data-passed:text-accent",
      "group-aria-[current=location]:bg-accent group-aria-[current=location]:text-accent-fg",
      "forced-colors:group-aria-[current=location]:border-[Highlight] forced-colors:group-aria-[current=location]:bg-[Highlight] forced-colors:group-aria-[current=location]:text-[HighlightText]",
      "motion-safe:transition-colors motion-safe:duration-normal motion-safe:ease-standard",
    ],
    // On a narrow page only the numbers show, so that the line stays one short row on a phone; the title names the link.
    title: "max-w-full wrap-anywhere text-pretty @max-md:sr-only",
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
 * prefers reduced motion), below the line, and moves focus there; the line sticks under the top bar and follows the
 * section being read as the page scrolls.
 */
export function SectionProgress({ label, parts, children }: SectionProgressProps) {
  const slots = sectionProgress();
  const [currentId, setCurrentId] = useCurrentSection(parts.map((part) => part.id));
  const currentIndex = parts.findIndex((part) => part.id === currentId);
  const navRef = useRef<HTMLElement>(null);
  // The line's height, below which a heading stops when it is scrolled to (`scroll-mt-under-section-progress`).
  useBlockSizeProperty(navRef, "--section-progress-block-size");
  return (
    <div className={slots.base()}>
      <nav ref={navRef} aria-label={label} className={slots.nav()}>
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
      <div className={slots.sections()}>{children}</div>
    </div>
  );
}
