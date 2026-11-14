// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline screen as the app shows it, at /app/timeline on the catalog's dev server: the router and the shell of
// @my/app, its home timeline from @my/app-features, signed in to a fake server instead of a live one. The fake answers
// the home timeline from the sample posts, a page of `pageSize` at a time, each page after the first a little late so
// that loading more shows. With `?fail=first` the first page fails while the page opens (a fetch that React's strict
// mode cancels and starts again included), so that the column's failed first load shows, and its retry, later,
// succeeds late. The router keeps its own history, so the catalog's address stays /app/timeline.
//
// The catalog's own control, over the app, stands in for the stream (#2890): it posts new samples on the fake server
// and hands them to the timeline as the stream will, or reports a reconnection, so that the timeline's policy of new
// posts and its anchoring show.

import { AppRoot, createAppRouter } from "@my/app";
import { createAppData, receiveHomeGap, receiveHomeStatuses } from "@my/app-data";
import { Button } from "@my/design-system";
import { createClient } from "@mycroblog/mykit";
import { posts, statusOf } from "./fixtures";

const pageSize = 6;
const lateMs = 2000;
// How long after the page opens a first page still fails with ?fail=first.
const failingMs = 1000;
const failFirst = new URLSearchParams(location.search).get("fail") === "first";

// What the fake server holds, newest first: the posts the control added, then the samples.
const served = posts.map((post) => statusOf(post, post.id));
let added = 0;

/** The fake server: the home timeline from what it holds, a page after the post of the cursor's ID. */
async function serve(request: Request): Promise<Response> {
  const url = new URL(request.url);
  if (url.pathname !== "/api/v1/timelines/home") return new Response(null, { status: 404 });
  // The cursor is the ID of the last post of the page before, so that posts added since do not shift the pages.
  const cursor = url.searchParams.get("cursor");
  const from = cursor === null ? 0 : served.findIndex((s) => s.id === cursor) + 1;
  if (failFirst && from === 0 && performance.now() < failingMs) {
    const problem = { type: "about:blank", title: "Internal Server Error", status: 500 };
    return new Response(JSON.stringify(problem), {
      status: 500,
      headers: { "Content-Type": "application/problem+json" },
    });
  }
  // The page as it is when asked, though posts are added while it comes.
  const items = served.slice(from, from + pageSize);
  const last = items.at(-1);
  const page = { items, ...(last && from + pageSize < served.length ? { next_cursor: last.id } : {}) };
  // A later page, or the first one retried after a failure, comes late, so that its loading shows.
  if (from > 0 || failFirst) await new Promise((resolve) => setTimeout(resolve, lateMs));
  return new Response(JSON.stringify(page), { headers: { "Content-Type": "application/json" } });
}

const server = "https://my.example";
const data = createAppData(createClient({ server, retry: { disabled: true }, fetch: serve }));

const router = createAppRouter({
  data,
  config: { apiOrigin: server, instanceName: "my.example", defaultLocale: "en", clientId: "my-web" },
  at: "/",
});

/**
 * Posts `count` new samples on the fake server and hands them to the timeline as the stream will, read back from the
 * server as the API's statuses.
 */
async function post(count: number) {
  const fresh = Array.from({ length: count }, () => {
    added += 1;
    const sample = posts[(added - 1) % posts.length];
    if (!sample) throw new Error("the catalog has no sample posts");
    return statusOf({ ...sample, ago: 0 }, `new-${added}`);
  }).reverse();
  served.unshift(...fresh);
  const { data: answer } = await data.api.GET("/timelines/home", { params: { query: {} } });
  receiveHomeStatuses(data, answer?.items.slice(0, count) ?? []);
}

// Down the end of the page, halfway, away from the new posts' button at the top and the overlay at the bottom.
const control = {
  position: "fixed",
  insetBlockStart: "40%",
  insetInlineEnd: "0.5rem",
  zIndex: 10,
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
} as const;

/** The catalog's control of the new posts, over the app: not a part of it. */
function NewPostsControl() {
  return (
    <div role="group" aria-label="Catalog: new posts" style={control}>
      <Button
        size="timeline"
        onPress={() => {
          void post(1);
        }}
      >
        Add a post
      </Button>
      <Button
        size="timeline"
        onPress={() => {
          void post(3);
        }}
      >
        Add 3
      </Button>
      <Button
        size="timeline"
        onPress={() => {
          receiveHomeGap(data);
        }}
      >
        Reconnect
      </Button>
    </div>
  );
}

/** The app's home screen, under the catalog's I18nProvider, and the catalog's control of new posts. */
export function AppPreview() {
  return (
    <>
      <AppRoot router={router} />
      <NewPostsControl />
    </>
  );
}
