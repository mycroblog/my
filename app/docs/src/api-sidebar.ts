// SPDX-License-Identifier: AGPL-3.0-only
// Starlight's route middleware that bounds the sidebar of the client API reference (src/api-reference.ts adds it after
// starlight-openapi's own, which fills the group with one nested group per tag of the spec holding every operation).
// Every page carries the whole sidebar in its HTML, so listing every operation on every page would grow the site with
// the square of the API. The group keeps its overview and lists each tag as a link to the tag's page; only the tag of
// the current page (its tag page or one of its operations) stays a group of its operations. The pagination that
// starlight-openapi computed from the full group is left as it is, so the previous and next links still walk the
// operations one by one. Whether the group is listed at all is decided after, by the section of the current page
// (src/section-sidebar.ts).
import { defineRouteMiddleware, type StarlightRouteData } from "@astrojs/starlight/route-data";

/** The label of the sidebar group of the reference, which this middleware finds it by. */
export const apiReferenceLabel = "Client API reference";

type SidebarEntry = StarlightRouteData["sidebar"][number];
type SidebarGroup = Extract<SidebarEntry, { type: "group" }>;

function hasCurrent(entries: SidebarEntry[]): boolean {
  return entries.some((entry) => (entry.type === "group" ? hasCurrent(entry.entries) : entry.isCurrent));
}

/**
 * A tag group of the reference as one link to its tag page, the group's first entry ("Overview"), unless it holds the
 * current page. A tag without a tag page (no description in the spec) keeps its group.
 */
function trimTag(entry: SidebarEntry): SidebarEntry {
  if (entry.type !== "group" || hasCurrent(entry.entries)) {
    return entry;
  }
  const overview = entry.entries[0];
  if (overview?.type !== "link") {
    return entry;
  }
  return { ...overview, label: entry.label, isCurrent: false };
}

/** The sidebar `entries` with the reference's group, wherever it is nested, listing its tags as links. */
function trimAPIReference(entries: SidebarEntry[]): SidebarEntry[] {
  return entries.map((entry) => {
    if (entry.type !== "group") {
      return entry;
    }
    if (entry.label === apiReferenceLabel) {
      return { ...entry, entries: entry.entries.map(trimTag) } satisfies SidebarGroup;
    }
    return { ...entry, entries: trimAPIReference(entry.entries) } satisfies SidebarGroup;
  });
}

export const onRequest = defineRouteMiddleware((context) => {
  const route = context.locals.starlightRoute;
  route.sidebar = trimAPIReference(route.sidebar);
});
