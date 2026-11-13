// SPDX-License-Identifier: AGPL-3.0-only

// The rough column of the home timeline screen (#3005), on the catalog only while its issue (#2989) stays open: the
// posts of @my/app-components separated by space alone, with the keyboard moving between them. It stands in for
// @my/app-data too: it keeps each sample status and answers a post's actions on it as the server would (favourite,
// boost, bookmark, an emoji reaction, a translation), so that the screen behaves while nothing is fetched.

import { Post, type PostAction, type PostStatus, type PostTranslation } from "@my/app-components";
import { rootPx } from "@my/design-tokens";
import { useState, type KeyboardEvent } from "react";
import { statusOf, translationOf, type TimelinePost } from "./fixtures";

const rem = (px: number) => `${px / rootPx}rem`;

/** The status a toggle leaves: on or off, its count moved with it. */
const toggled = (on: boolean | undefined, count: number) => ({ on: !(on ?? false), count: count + (on ? -1 : 1) });

/** The status after the reader's reaction with `name`: taken back if it was theirs, else moved there. */
function reacted(status: PostStatus, name: string): PostStatus {
  const was = status.reactions.find((r) => r.me === true)?.name;
  const reactions = status.reactions
    .map((r) => {
      if (r.name === name) return { ...r, me: was !== name, count: r.count + (was === name ? -1 : 1) };
      if (r.name === was) return { ...r, me: false, count: r.count - 1 };
      return r;
    })
    .filter((r) => r.count > 0);
  return { ...status, reactions };
}

/** What an action changes on the status it acts on; the others change nothing here. */
function answered(status: PostStatus, action: PostAction): PostStatus {
  switch (action.type) {
    case "favourite": {
      const t = toggled(status.favourited, status.favourites_count);
      return { ...status, favourited: t.on, favourites_count: t.count };
    }
    case "boost": {
      const t = toggled(status.boosted, status.boosts_count);
      return { ...status, boosted: t.on, boosts_count: t.count };
    }
    case "bookmark":
      return { ...status, bookmarked: !(status.bookmarked ?? false) };
    case "reaction":
      return reacted(status, action.name);
    default:
      return status;
  }
}

/**
 * One sample post, with its status and whether it shows translated kept as the server and the reader leave them;
 * `translationAs` changes the translation the server answers (the catalog's notes of a translation it knows less of).
 */
export function SamplePost({
  post,
  lang,
  translationAs,
}: {
  post: TimelinePost;
  lang: "en" | "ja";
  translationAs?: ((t: PostTranslation) => PostTranslation) | undefined;
}) {
  const [status, setStatus] = useState(() => statusOf(post, post.id));
  const answer = translationOf(post, lang);
  const translation = answer && translationAs ? translationAs(answer) : answer;
  const [translated, setTranslated] = useState(post.translated === true);
  const onAction = (action: PostAction) => {
    if (action.type === "translate" || action.type === "original") {
      setTranslated(action.type === "translate");
      return;
    }
    setStatus((s) => (s.boost === undefined ? answered(s, action) : { ...s, boost: answered(s.boost, action) }));
  };
  return (
    <Post
      status={status}
      own={post.own}
      translatable={translation !== undefined}
      translation={translated ? translation : undefined}
      onAction={onAction}
    />
  );
}

// Keys that move between posts: j and k, or the arrows while a post itself has focus.
const next = new Set(["j", "ArrowDown"]);
const previous = new Set(["k", "ArrowUp"]);

/**
 * The column: the posts separated by space alone. j and k (or the arrows on a focused post) move focus to the next or
 * the previous post, from inside a post too; typing into a field is left alone.
 */
export function Column({ posts, lang, label }: { posts: TimelinePost[]; lang: "en" | "ja"; label: string }) {
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement;
    if (e.altKey || e.ctrlKey || e.metaKey || target.closest("input, textarea, [contenteditable], [role=menu]")) return;
    const arrow = e.key.startsWith("Arrow");
    if (!next.has(e.key) && !previous.has(e.key)) return;
    if (arrow && !target.matches("[data-entry]")) return;
    const all = Array.from(e.currentTarget.querySelectorAll<HTMLElement>("[data-entry]"));
    const current = target.closest<HTMLElement>("[data-entry]");
    const i = current ? all.indexOf(current) : -1;
    const to = all[next.has(e.key) ? i + 1 : Math.max(0, i - 1)];
    if (!to) return;
    e.preventDefault();
    to.focus();
    to.scrollIntoView({ block: "nearest" });
  };
  return (
    <div
      role="feed"
      aria-label={label}
      onKeyDown={onKeyDown}
      style={{ display: "grid", gap: rem(28), gridTemplateColumns: "minmax(0, 1fr)" }}
    >
      {posts.map((post) => (
        <SamplePost key={post.id} post={post} lang={lang} />
      ))}
    </div>
  );
}
