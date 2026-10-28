// SPDX-License-Identifier: AGPL-3.0-only
// The reading layout of the information pages: one column of about 680px and a table
// of contents built from the column's own `h2` headings.
import { type MouseEvent, type ReactNode, useEffect, useRef, useState } from "react";
import { tv } from "tailwind-variants";
import { goToSection, useCurrentSection } from "./currentSection";

const infoPage = tv({
  slots: {
    base: "relative mx-auto w-full max-w-170 min-w-0",
    column: "flex min-w-0 flex-col gap-8 [&_h2]:scroll-mt-6 [&_h2]:outline-none",
    aside: "absolute inset-y-0 inset-s-full ms-8 w-50",
    sticky: "sticky top-6",
    nav: "flex flex-col gap-2",
    title: "text-xs font-extrabold tracking-widest text-fg-muted uppercase",
    list: "flex flex-col gap-0.5",
    link: "block rounded-full px-3 py-1.5 text-sm text-fg-muted hover:bg-surface-sunken aria-[current=location]:bg-soft aria-[current=location]:font-semibold aria-[current=location]:text-fg",
    summary: "cursor-pointer rounded-md py-1 text-sm font-semibold text-fg",
  },
});

type Section = { id: string; title: string };

export type InfoPageProps = {
  /** The page's `PageHeading`, above the folded contents. */
  heading: ReactNode;
  /** The page: sections each headed by an `h2` with an `id`; those headings make up the contents. */
  children: ReactNode;
  /** The contents' accessible name and visible title ("On this page"). */
  contentsLabel: string;
  /**
   * `beside` stands the contents sticky in the margin right of the column, which needs 13rem of free space there;
   * `inline` (default) folds them above the column. The app decides, since only it knows the room (the shell's columns).
   */
  placement?: "beside" | "inline";
};

/** Reads the `h2` headings that have an id, in document order. */
function readSections(column: HTMLElement): Section[] {
  return Array.from(column.querySelectorAll<HTMLElement>("h2[id]"))
    .map((heading) => ({ id: heading.id, title: heading.textContent.trim() }))
    .filter((section) => section.title !== "");
}

function sameSections(a: readonly Section[], b: readonly Section[]) {
  return (
    a.length === b.length &&
    a.every((section, index) => section.id === b[index]?.id && section.title === b[index].title)
  );
}

/**
 * A reading column with a table of contents. The contents are read from the headings in the DOM, as the sections of a
 * page load one by one, and follow them; the section being read is marked `aria-current="location"`. Choosing an entry
 * scrolls to its heading (smoothly only unless the reader prefers reduced motion) and moves focus there.
 */
export function InfoPage({ heading, children, contentsLabel, placement = "inline" }: InfoPageProps) {
  const slots = infoPage();
  const columnRef = useRef<HTMLDivElement>(null);
  const [sections, setSections] = useState<Section[]>([]);

  useEffect(() => {
    const column = columnRef.current;
    if (column === null) return;
    const update = () => {
      const next = readSections(column);
      setSections((previous) => (sameSections(previous, next) ? previous : next));
    };
    update();
    const observer = new MutationObserver(update);
    observer.observe(column, { childList: true, subtree: true, characterData: true });
    return () => {
      observer.disconnect();
    };
  }, []);

  const [currentId, setCurrentId] = useCurrentSection(sections.map((section) => section.id));

  const go = (event: MouseEvent<HTMLAnchorElement>, id: string) => {
    if (goToSection(event, id)) setCurrentId(id);
  };

  const list = (
    <ul className={slots.list()}>
      {sections.map((section) => (
        <li key={section.id}>
          <a
            href={`#${section.id}`}
            aria-current={section.id === currentId ? "location" : undefined}
            className={slots.link()}
            onClick={(event) => {
              go(event, section.id);
            }}
          >
            {section.title}
          </a>
        </li>
      ))}
    </ul>
  );

  return (
    <div className={slots.base()}>
      <div ref={columnRef} className={slots.column()}>
        {heading}
        {/* The folded contents come right after the heading on narrow screens. */}
        {placement === "inline" && sections.length > 1 && (
          <details>
            <summary className={slots.summary()}>{contentsLabel}</summary>
            <nav aria-label={contentsLabel} className="pt-2">
              {list}
            </nav>
          </details>
        )}
        {children}
      </div>
      {placement === "beside" && sections.length > 1 && (
        <div className={slots.aside()}>
          <div className={slots.sticky()}>
            <nav aria-label={contentsLabel} className={slots.nav()}>
              <p className={slots.title()} aria-hidden="true">
                {contentsLabel}
              </p>
              {list}
            </nav>
          </div>
        </div>
      )}
    </div>
  );
}
