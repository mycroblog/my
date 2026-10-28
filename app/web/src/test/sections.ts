// SPDX-License-Identifier: AGPL-3.0-only
// The order of a screen's sections (web-client/21-experience-principles.md §21.4, L3): the names of the level-two
// headings of the main region, which a test compares with the order in the screen's requirements
// (docs/architecture/web-client/screens/).
import { screen, within } from "@testing-library/react";

/** The names of the main region's level-two headings, in the order of the page. */
export function sectionHeadings(): string[] {
  return within(screen.getByRole("main"))
    .getAllByRole("heading", { level: 2 })
    .map((heading) => heading.textContent);
}
