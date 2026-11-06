// SPDX-License-Identifier: AGPL-3.0-only
// Starlight's route middleware (astro.config.ts): links the Markdown form of a page (src/page-markdown.ts) from its head.
import { defineRouteMiddleware } from "@astrojs/starlight/route-data";
import { markdownURL } from "./page-markdown";

export const onRequest = defineRouteMiddleware((context) => {
  const route = context.locals.starlightRoute;
  const href = markdownURL(route.id);
  if (href !== undefined) {
    route.head.push({ tag: "link", attrs: { rel: "alternate", type: "text/markdown", href } });
  }
});
