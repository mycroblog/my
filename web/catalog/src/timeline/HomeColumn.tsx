// SPDX-License-Identifier: AGPL-3.0-only

// The column of /timeline, connected as a feature connects it (ADR-0025 §4): the Column of @my/app-components fed by
// useHomeTimeline() of @my/app-data, whose client talks to a fake server in this page instead of a live one. The fake
// answers the home timeline from the sample posts, a page of `pageSize` at a time, each page after the first a little
// late so that loading more shows. With `?fail=first` the first page fails while the page opens (a fetch that React's
// strict mode cancels and starts again included), so that the column's failed first load shows, and its retry, later,
// succeeds late; a post's action is written into the cache as the server's answer would be
// (ADR-0028 §6), and whether a post shows translated is the reader's, kept here.

import { Column, type PostAction, type PostStatus } from "@my/app-components";
import { AppDataProvider, createAppData, updateCachedStatuses, useHomeTimeline, type Status } from "@my/app-data";
import { createClient } from "@mycroblog/mykit";
import { useState } from "react";
import { answered } from "./answers";
import { posts, statusOf, translationOf } from "./fixtures";

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

const data = createAppData(createClient({ server: "https://my.example", retry: { disabled: true }, fetch: serve }));

const byId = new Map(posts.map((post) => [post.id, post]));

function Connected({ lang, label }: { lang: "en" | "ja"; label: string }) {
  const timeline = useHomeTimeline();
  const [translated, setTranslated] = useState(
    () => new Set(posts.filter((post) => post.translated === true).map((post) => post.id)),
  );
  const onAction = (action: PostAction) => {
    if (action.type === "translate" || action.type === "original") {
      const next = new Set(translated);
      if (action.type === "translate") next.add(action.id);
      else next.delete(action.id);
      setTranslated(next);
      return;
    }
    updateCachedStatuses(data.queryClient, action.id, (s: Status) => answered(s, action));
  };
  const postProps = (status: PostStatus) => {
    const shown = status.boost ?? status;
    const post = byId.get(shown.id);
    const translation = post && translationOf(post, lang);
    return {
      own: post?.own,
      translatable: translation !== undefined,
      translation: translated.has(shown.id) ? translation : undefined,
    };
  };
  return <Column label={label} {...timeline} onAction={onAction} postProps={postProps} />;
}

/** The home timeline's column, on its own data. */
export function HomeColumn(props: { lang: "en" | "ja"; label: string }) {
  return (
    <AppDataProvider data={data}>
      <Connected {...props} />
    </AppDataProvider>
  );
}
