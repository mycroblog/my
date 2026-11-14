// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline screen as the app shows it, at /timeline on the catalog's dev server: the router and the shell of
// @my/app, its home timeline from @my/app-features, signed in to a fake server instead of a live one. The fake answers
// the home timeline from the sample posts, a page of `pageSize` at a time, each page after the first a little late so
// that loading more shows. With `?fail=first` the first page fails while the page opens (a fetch that React's strict
// mode cancels and starts again included), so that the column's failed first load shows, and its retry, later,
// succeeds late. The router keeps its own history, so the catalog's address stays /timeline.

import { AppRoot, createAppRouter } from "@my/app";
import { createAppData } from "@my/app-data";
import { createClient } from "@mycroblog/mykit";
import { posts, statusOf } from "./fixtures";

const pageSize = 6;
const lateMs = 2000;
// How long after the page opens a first page still fails with ?fail=first.
const failingMs = 1000;
const failFirst = new URLSearchParams(location.search).get("fail") === "first";

/** The fake server: the home timeline from the sample posts, by cursor (the index of a page's first post). */
async function serve(request: Request): Promise<Response> {
  const url = new URL(request.url);
  if (url.pathname !== "/api/v1/timelines/home") return new Response(null, { status: 404 });
  const from = Number(url.searchParams.get("cursor") ?? "0");
  if (failFirst && from === 0 && performance.now() < failingMs) {
    const problem = { type: "about:blank", title: "Internal Server Error", status: 500 };
    return new Response(JSON.stringify(problem), {
      status: 500,
      headers: { "Content-Type": "application/problem+json" },
    });
  }
  // A later page, or the first one retried after a failure, comes late, so that its loading shows.
  if (from > 0 || failFirst) await new Promise((resolve) => setTimeout(resolve, lateMs));
  const to = from + pageSize;
  const page = {
    items: posts.slice(from, to).map((post) => statusOf(post, post.id)),
    ...(to < posts.length ? { next_cursor: String(to) } : {}),
  };
  return new Response(JSON.stringify(page), { headers: { "Content-Type": "application/json" } });
}

const server = "https://my.example";

const router = createAppRouter({
  data: createAppData(createClient({ server, retry: { disabled: true }, fetch: serve })),
  config: { apiOrigin: server, instanceName: "my.example", defaultLocale: "en" },
  at: "/",
});

/** The app's home screen, under the catalog's I18nProvider. */
export function AppPreview() {
  return <AppRoot router={router} />;
}
