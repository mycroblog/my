// SPDX-License-Identifier: AGPL-3.0-only
// The section being read on a page of a few headed sections, and the move to one of them: shared by the table of
// contents of InfoPage and the progress line of SectionProgress, so that both mark and move the same way.
import { type MouseEvent, useEffect, useState } from "react";

/**
 * The id of the heading being read among `ids` (the ids of the sections' headings, in document order): the last one
 * that has passed the top fifth of the screen, else the first. Undefined without an IntersectionObserver (a test
 * environment), when no section is marked.
 */
export function useCurrentSection(ids: readonly string[]): [string | undefined, (id: string) => void] {
  const [currentId, setCurrentId] = useState<string>();
  // The ids as one string, so that a new array with the same ids does not observe again.
  const key = ids.join("\n");
  useEffect(() => {
    if (key === "" || typeof IntersectionObserver === "undefined") return;
    const headings = key.split("\n").flatMap((id) => document.getElementById(id) ?? []);
    // A heading crossing the line at the top fifth (the observer's root margin) is the moment to look again.
    const observer = new IntersectionObserver(
      () => {
        const above = headings.filter((heading) => heading.getBoundingClientRect().top <= window.innerHeight * 0.2);
        setCurrentId(above.at(-1)?.id ?? headings[0]?.id);
      },
      { rootMargin: "0px 0px -80% 0px", threshold: [0, 1] },
    );
    for (const heading of headings) observer.observe(heading);
    return () => {
      observer.disconnect();
    };
  }, [key]);
  return [currentId, setCurrentId];
}

/**
 * Follows a link to the heading `id` of the page: scrolls to it (smoothly only unless the reader prefers reduced
 * motion) and moves focus there, in place of the browser's jump. Returns whether the heading was found.
 */
export function goToSection(event: MouseEvent<HTMLAnchorElement>, id: string): boolean {
  const heading = document.getElementById(id);
  if (heading === null) return false;
  event.preventDefault();
  const smooth =
    typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: no-preference)").matches;
  heading.scrollIntoView({ behavior: smooth ? "smooth" : "auto", block: "start" });
  heading.tabIndex = -1;
  heading.focus({ preventScroll: true });
  return true;
}
