// SPDX-License-Identifier: AGPL-3.0-only
// Starlight's route middleware that bounds the sidebar to the section of the current page (src/api-reference.ts adds it
// after the API reference's, src/api-sidebar.ts). Every page carries the whole sidebar in its HTML, so listing every
// document of every section on every page, in every locale, would grow the site with sections x documents x locales
// (`collapsed: true` only closes a <details> whose entries are still in the HTML). The top-level group that holds the
// current page keeps its entries; every other top-level group is one link, labelled with the section's heading (in the
// locale of the page), to the first document of the section, so a reader can still move between sections. A page that
// no section lists (the landing page, a page the sidebar does not name) shows every section as such a link. The
// pagination that Starlight computed from the full sidebar is left as it is, so the previous and next links still walk
// the documents in order across the sections.
import { defineRouteMiddleware, type StarlightRouteData } from "@astrojs/starlight/route-data";
import { apiReferenceLabel } from "./api-sidebar";

type SidebarEntry = StarlightRouteData["sidebar"][number];
type SidebarLink = Extract<SidebarEntry, { type: "link" }>;

function hasCurrent(entries: SidebarEntry[]): boolean {
  return entries.some((entry) => (entry.type === "group" ? hasCurrent(entry.entries) : entry.isCurrent));
}

/** The first link of `entries`, depth first, in sidebar order. */
function firstLink(entries: SidebarEntry[]): SidebarLink | undefined {
  for (const entry of entries) {
    const link = entry.type === "group" ? firstLink(entry.entries) : entry;
    if (link !== undefined) {
      return link;
    }
  }
  return undefined;
}

/**
 * The sections of the sidebar: each top-level entry, with the group of the client API reference joined to the one
 * before it (the reference is placed after the section Developers but belongs to it).
 */
function sections(sidebar: SidebarEntry[]): SidebarEntry[][] {
  const out: SidebarEntry[][] = [];
  for (const entry of sidebar) {
    const previous = out.at(-1);
    if (entry.type === "group" && entry.label === apiReferenceLabel && previous !== undefined) {
      previous.push(entry);
    } else {
      out.push([entry]);
    }
  }
  return out;
}

/** The sidebar with every section that does not hold the current page as one link to its first document. */
export function currentSectionOnly(sidebar: SidebarEntry[]): SidebarEntry[] {
  return sections(sidebar).flatMap((section): SidebarEntry[] => {
    const [head] = section;
    if (head === undefined || head.type !== "group" || hasCurrent(section)) {
      return section;
    }
    const first = firstLink(head.entries);
    return first === undefined ? [] : [{ ...first, label: head.label, isCurrent: false }];
  });
}

export const onRequest = defineRouteMiddleware((context) => {
  const route = context.locals.starlightRoute;
  route.sidebar = currentSectionOnly(route.sidebar);
});
