// SPDX-License-Identifier: AGPL-3.0-only
// The sidebar of the site, built from the sections and order of docs/README.md so that the documents have one table
// of contents (ADR-0150 §4): a document added to docs/README.md appears here with no other change.
import { readIndex, slugOf } from "./published";

interface SidebarLink {
  label: string;
  slug: string;
}

interface SidebarGroup {
  label: string;
  items: SidebarLink[];
}

/** The text of a link in docs/README.md without its trailing explanation in parentheses, as a sidebar label. */
function shortLabel(label: string): string {
  return label.replace(/\s*\([^)]*\)\s*$/, "").replaceAll("`", "") || label;
}

/**
 * The groups of the sidebar. `after` places groups that do not come from docs/README.md (such as the API reference
 * that a plugin fills) right after the section with that heading, or at the end when docs/README.md has no such section.
 */
export function docsSidebar<T>(after: Record<string, T[]> = {}): (SidebarGroup | T)[] {
  const placed = new Set<string>();
  const groups = readIndex().flatMap((section): (SidebarGroup | T)[] => {
    const group = {
      label: section.heading,
      items: section.items.map((item) => ({ label: shortLabel(item.label), slug: slugOf(item.path) })),
    };
    placed.add(section.heading);
    return [group, ...(after[section.heading] ?? [])];
  });
  const rest = Object.entries(after).flatMap(([heading, extra]) => (placed.has(heading) ? [] : extra));
  return [...groups, ...rest];
}
