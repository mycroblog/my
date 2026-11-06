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

export function docsSidebar(): SidebarGroup[] {
  return readIndex().map((section) => ({
    label: section.heading,
    items: section.items.map((item) => ({
      label: shortLabel(item.label),
      slug: slugOf(item.path),
    })),
  }));
}
