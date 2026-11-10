// SPDX-License-Identifier: AGPL-3.0-only
// Every page has a way back (web-client/21-experience-principles.md): the installed app has no browser back button,
// so a page that is not a place of the navigation (routes/-layout/Navigation.tsx) declares `staticData.back`, and
// PageHeading puts a back button before its h1. The pages under a section with a navigation of its own (settings,
// administration, moderation, about) go back through it.
import { createRouter, type StaticDataRouteOption } from "@tanstack/react-router";
import { describe, expect, it } from "vitest";
import { navigationSections } from "./routes/-layout/Navigation";
import { routeTree } from "./routeTree.gen";

/** Sections whose layout lists their pages, which is their way back. */
const sectionsWithOwnNavigation = ["/settings", "/admin", "/moderation", "/about"];

/** Pages that are no place to come back from, by route id. */
const withoutWayBack: Readonly<Record<string, string>> = {
  "/": "the start page, where a way back leads: the landing page signed out, the home timeline signed in",
  "/login": "the sign-in page, an item of the navigation signed out (its href carries the page to return to)",
  "/welcome": "the first-run step after signing up, which goes on to the home timeline",
  "/login_/callback": "the end of a sign-in: it goes on by itself",
};

/** What the test reads of a route. */
type Page = { id: string; fullPath: string; children?: unknown; options: { staticData?: StaticDataRouteOption } };

/** Every route that renders a page: the routes without children (creating the router gives each its id and path). */
function pages(): Page[] {
  // The context is read only by loaders, which never run here.
  const router = createRouter({ routeTree, context: undefined as never });
  return (Object.values(router.routesById) as Page[]).filter(
    (route) => route.children === undefined || Object.keys(route.children as object).length === 0,
  );
}

const isUnder = (path: string, section: string) => path === section || path.startsWith(`${section}/`);

describe("the route tree", () => {
  const all = pages();

  it("lists the pages", () => {
    expect(all.length).toBeGreaterThan(50);
  });

  it.each(all.map((route) => [route.id, route] as const))("gives %s a way back", (_id, route) => {
    const path = route.fullPath.replace(/\/$/, "") || "/";
    if (route.id in withoutWayBack) return;
    if ((navigationSections as readonly string[]).includes(path)) return;
    if (sectionsWithOwnNavigation.some((section) => isUnder(path, section))) return;
    expect(route.options.staticData?.back, `${route.id} declares staticData.back`).toBe(true);
  });

  it("names only pages of the tree in its list of pages without a way back", () => {
    const ids = new Set(all.map((route) => route.id));
    expect(Object.keys(withoutWayBack).filter((id) => !ids.has(id))).toEqual([]);
  });
});
