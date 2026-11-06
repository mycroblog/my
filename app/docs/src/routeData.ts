// SPDX-License-Identifier: AGPL-3.0-only
// Starlight's route middleware (astro.config.ts, routeMiddleware): the buttons of the landing page's hero name
// documents by their repository path (docs/operations/running.md), which become the URL of their page under the base
// path (siteLink), since the frontmatter cannot know the base path a version is built for. starlight-links-validator
// checks the links after this middleware has run.
import { defineRouteMiddleware } from "@astrojs/starlight/route-data";
import { siteLink } from "./published";

/** What the middleware changes of a route: the links of the hero's buttons in the page's frontmatter. */
type RouteContext = {
  locals: { starlightRoute: { entry: { data: { hero?: { actions?: { link: string }[] } } } } };
};

export const onRequest = defineRouteMiddleware((context: RouteContext) => {
  for (const action of context.locals.starlightRoute.entry.data.hero?.actions ?? []) {
    if (!/^[A-Za-z][A-Za-z0-9+.-]*:/.test(action.link)) {
      action.link = siteLink(action.link);
    }
  }
});
