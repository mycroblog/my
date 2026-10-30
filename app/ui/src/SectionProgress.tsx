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
    base: "@container flex min-w-0 flex-col gap-10",
    // Under the top bar, opaque over the sections that scroll under it. Its own stacking context, above the sections'
    // (`sections`) and below the top bar's (z-10), so that nothing of it draws over the bar or its menu.
    nav: "sticky top-under-top-bar z-1 bg-surface py-2 short:static",
    list: "flex items-start gap-4 @md:gap-6",
    // The sections in a stacking context of their own, so that what they lift (a link card's button) stays under the line.
    sections:
      "isolate flex min-w-0 flex-col gap-16 @3xl:gap-24 [&_h2]:scroll-mt-under-section-progress [&_h3]:scroll-mt-under-section-progress",
    item: "flex min-w-0 flex-1",
    link: [
      "group relative flex min-h-11 w-full min-w-11 flex-col items-start justify-center gap-1 rounded-md pt-3 pb-1 text-start text-sm text-fg-muted outline-none hover:text-fg focus-visible:focus-ring aria-[current=location]:text-fg",
      // The line of this part, above its number: in the accent once the reader has reached it.
      "before:absolute before:inset-x-0 before:top-0 before:h-0.5 before:rounded-full before:bg-border",
      "data-reached:before:bg-accent forced-colors:data-reached:before:bg-[Highlight]",
      "motion-safe:before:transition-colors motion-safe:before:duration-normal motion-safe:before:ease-standard",
    ],
    // The number only shows the order, which the list already gives.
    number: "font-semibold tabular-nums group-data-reached:text-accent group-aria-[current=location]:text-accent",
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
              <li key={part.id} className={slots.item()}>
                <a
                  href={`#${part.id}`}
                  aria-current={part.id === currentId ? "location" : undefined}
                  data-reached={reached || undefined}
                  data-passed={currentIndex > index || undefined}
                  className={slots.link()}
                  onClick={(event) => {
                    if (goToSection(event, part.id)) setCurrentId(part.id);
                  }}
                >
                  {/* The number only shows the order, which the list already gives. */}
                  <span aria-hidden="true" className={slots.number()}>
                    {String(index + 1).padStart(2, "0")}
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
