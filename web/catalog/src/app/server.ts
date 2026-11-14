// SPDX-License-Identifier: AGPL-3.0-only

// The fake server the app's screens read on the catalog, in place of a live one: the client API's answers for the
// sample home timeline (timeline/fixtures.ts) and the samples of the post detail, the notifications and the profiles
// (samples.ts), with the writes those screens make (follow, block, mute, the answers of requests, the read marker) kept
// in memory until the page reloads. A later page of a list comes a little late, so that loading more shows; with
// `?fail=first` the home timeline's first page fails while the page opens, and its retry, later, succeeds late.

import type { PostAccount, PostStatus } from "@my/app-components";
import { serverHtml } from "../serverHtml";
import { accountOf, posts, statusOf, type TimelinePost } from "../timeline/fixtures";
import { held, lone, me, notices, profiles, thread, type Notice, type ProfileSample, type Who } from "./samples";

const pageSize = 6;
const lateMs = 2000;
// How long after the page opens a first page still fails with ?fail=first.
const failingMs = 1000;
const failFirst = new URLSearchParams(location.search).get("fail") === "first";

type Json = Record<string, unknown>;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": status >= 400 ? "application/problem+json" : "application/json" },
  });
const problem = (status: number, title: string) => json({ type: "about:blank", title, status }, status);
const late = () => new Promise((resolve) => setTimeout(resolve, lateMs));

/** An ID as the server's, a UUIDv7: in its canonical form, a later one sorts after an earlier one. */
function idAt(ms: number, n: number): string {
  const time = Math.max(0, Math.round(ms)).toString(16).padStart(12, "0");
  return `${time.slice(0, 8)}-${time.slice(8)}-7000-8000-${n.toString(16).padStart(12, "0")}`;
}

/** The home timeline, newest first: the posts the control added, then the samples. */
const home: PostStatus[] = posts.map((post) => statusOf(post, post.id));

// ---- Accounts ------------------------------------------------------------------------------------------------------

type Relationship = {
  id: string;
  following: boolean;
  requested: boolean;
  followed_by: boolean;
  requested_by: boolean;
  blocking: boolean;
  muting: boolean;
  muting_notifications: boolean;
  notify: boolean;
  show_boosts: boolean;
  note: string;
};

const accounts = new Map<string, Json>();
const relationships = new Map<string, Relationship>();
const statusesOf = new Map<string, { posts: PostStatus[]; pinned: PostStatus[]; featured: Json[] }>();

function relationshipOf(id: string): Relationship {
  let r = relationships.get(id);
  if (r === undefined) {
    r = {
      id,
      following: false,
      requested: false,
      followed_by: false,
      requested_by: false,
      blocking: false,
      muting: false,
      muting_notifications: false,
      notify: false,
      show_boosts: true,
      note: "",
    };
    relationships.set(id, r);
  }
  return r;
}

/** The account the API would send for a person of the samples, with its profile when it has one. */
function account(who: Who, profile?: ProfileSample): Json {
  const base: PostAccount = accountOf(who);
  const known = accounts.get(base.id);
  if (known !== undefined && profile === undefined) return known;
  const avatar = who.avatar === undefined ? undefined : { small: { url: who.avatar }, medium: { url: who.avatar } };
  const full: Json = {
    ...base,
    uri: `https://${base.domain}/users/${base.username}`,
    url: `https://${base.domain}/@${base.username}`,
    note: profile === undefined ? "" : serverHtml(profile.bio, base.domain),
    locked: profile?.locked ?? false,
    bot: profile?.bot ?? false,
    created_at: (profile?.joined ?? new Date("2025-01-01")).toISOString(),
    fields: (profile?.fields ?? []).map(([name, value]) => ({ name, value: serverHtml(value, base.domain) })),
    badges: (profile?.badges ?? []).map((label, i) => ({ role_id: idAt(0, i + 1), label, color: "neutral" })),
    avatar,
    header:
      profile?.banner === undefined ? undefined : { small: { url: profile.banner }, medium: { url: profile.banner } },
    followers_count: profile?.followers ?? 0,
    following_count: profile?.following ?? 0,
  };
  accounts.set(base.id, full);
  return full;
}

for (const profile of Object.values(profiles)) {
  const a = account(profile.who, profile);
  const id = a["id"] as string;
  const r = relationshipOf(id);
  r.following = profile.rel.following ?? false;
  r.requested = profile.rel.requested ?? false;
  r.followed_by = profile.rel.followedBy ?? false;
  r.blocking = profile.rel.blocking ?? false;
  r.muting = profile.rel.muting ?? false;
  const status = (p: TimelinePost) => statusOf(p, p.id);
  statusesOf.set(id, {
    posts: profile.posts.map(status),
    pinned: profile.pinned.map(status),
    featured: profile.featured.map(([name, count]) => ({
      name,
      url: `https://my.example/tags/${name}`,
      statuses_count: count,
    })),
  });
}

const viewer = account(me, profiles.own);

// ---- The thread ----------------------------------------------------------------------------------------------------

const statuses = new Map<string, PostStatus>();
const contexts = new Map<string, { ancestors: PostStatus[]; descendants: PostStatus[] }>();

const keep = (s: PostStatus & { in_reply_to_id?: string | undefined }) => {
  statuses.set(s.id, s);
  return s;
};

const filler = (n: number, who: Who, ago: number, lang: "en" | "ja", text: string): TimelinePost => ({
  id: `t-filler-${n}`,
  lang,
  author: who,
  ago,
  visibility: "public",
  text,
  counts: { replies: 1, boosts: 0, favourites: n % 4 },
});

{
  const root = keep(statusOf(thread.root, thread.root.id));
  // The ancestors the screen folds, between the first post and the two it shows.
  const folded = Array.from({ length: thread.folded }, (_, i) =>
    keep(
      statusOf(
        filler(
          i + 1,
          thread.parents[i % 2]?.author ?? me,
          5 * 3_600_000 - i * 600_000,
          "en",
          `Step ${i + 1} of the talk.`,
        ),
        `t-a${i + 2}`,
      ),
    ),
  );
  const parents = thread.parents.map((p) => keep(statusOf(p, p.id)));
  const ancestors = [root, ...folded, ...parents].map((s, i, all) =>
    i === 0 ? s : keep({ ...s, in_reply_to_id: all[i - 1]?.id }),
  );
  const focused = keep({ ...statusOf(thread.focused, thread.focused.id), in_reply_to_id: ancestors.at(-1)?.id });
  // The replies, depth first, each replying to the last one a step shallower; then more replies to the focused post,
  // beyond those the screen shows at first.
  const chain: string[] = [focused.id];
  const replies = thread.replies.map(({ post, depth }) => {
    const s = keep({ ...statusOf(post, post.id), in_reply_to_id: chain[depth - 1] });
    chain[depth] = s.id;
    chain.length = depth + 1;
    return s;
  });
  const more = Array.from({ length: thread.later + 8 }, (_, i) =>
    keep({
      ...statusOf(
        filler(
          100 + i,
          [me, thread.replies[0]?.post.author ?? me][i % 2] ?? me,
          40 * 60_000 - i * 60_000,
          "en",
          `A later reply, number ${i + 1}.`,
        ),
        `t-later-${i + 1}`,
      ),
      in_reply_to_id: focused.id,
    }),
  );
  contexts.set(focused.id, { ancestors, descendants: [...replies, ...more] });
  keep(statusOf(lone, lone.id));
}

/** The address of the samples' focused post, and of the post with no thread. */
export const threadPaths = {
  thread: `/@${accountOf(thread.focused.author).acct}/status/${thread.focused.id}`,
  lone: `/@${accountOf(lone.author).acct}/status/${lone.id}`,
};

// ---- The notifications ---------------------------------------------------------------------------------------------

const yours = (text: string): PostStatus =>
  statusOf({ lang: "en", author: me, ago: 2 * 3_600_000, visibility: "public", text }, "your-post");

type Interaction = { id: string; kind: "quote" | "reply" | "announce"; state: string; account: Json; status: Json };

const interactions: Interaction[] = [];

/** The notification the API would send for a sample. */
function notificationOf(n: Notice, i: number): Json {
  const at = Date.now() - n.ago;
  const base = { id: idAt(at, i), type: n.type, created_at: new Date(at).toISOString() };
  if (n.kind === "post") return { ...base, account: account(n.who), status: statusOf(n.post, n.post.id) };
  if (n.kind === "server") {
    switch (n.type) {
      case "poll_closed":
        return { ...base, status: yours(n.body ?? "") };
      case "announcement":
        return {
          ...base,
          announcement: {
            id: idAt(at, 900),
            title: n.title ?? "",
            content: "",
            published_at: base.created_at,
            revised_at: base.created_at,
            read: false,
          },
        };
      default:
        return base;
    }
  }
  const who = account(n.who);
  const about = n.about === undefined ? {} : { status: yours(n.about) };
  switch (n.type) {
    case "reaction":
      return { ...base, account: who, ...about, reaction: { name: n.emoji ?? "👍" } };
    case "account_moved":
      return {
        ...base,
        account: who,
        account_move: { target: n.to === undefined ? undefined : account(n.to), outcome: "followed" },
      };
    case "follow_request":
      relationshipOf(who["id"] as string).requested_by = true;
      return { ...base, account: who };
    case "quote_request":
      interactions.push({
        id: idAt(at, 800 + i),
        kind: "quote",
        state: "pending",
        account: who,
        status: about.status ?? yours(""),
      });
      return { ...base, account: who, ...about };
    default:
      return { ...base, account: who, ...about };
  }
}

/** The notifications, newest first, and the newest the reader read. */
const notifications: Json[] = notices.map(notificationOf);
let lastRead = notifications[notices.findIndex((n) => !n.unread)]?.["id"] as string | undefined;
let added = 0;

const unreadCount = () => notifications.filter((n) => lastRead === undefined || (n["id"] as string) > lastRead).length;

/** A new notification, as the stream will send it: someone favourited the reader's post. */
export function newNotification(): Json {
  added += 1;
  const who = [profiles.local.who, profiles.remote.who, profiles.muted.who][added % 3] ?? me;
  const n = notificationOf(
    {
      id: `n-new-${added}`,
      ago: 0,
      unread: true,
      kind: "act",
      type: "favourite",
      who,
      about: "Trying out the new home timeline on my.example. Hello, fediverse!",
    },
    1000 + added,
  );
  notifications.unshift(n);
  return n;
}

// ---- The answers ---------------------------------------------------------------------------------------------------

/** The page after the item of the cursor, as the server pages a list. */
function page<T extends { id: string }>(items: readonly T[], cursor: string | null, size = pageSize) {
  const from = cursor === null ? 0 : items.findIndex((s) => s.id === cursor) + 1;
  const slice = items.slice(from, from + size);
  const last = slice.at(-1);
  return { from, body: { items: slice, ...(last && from + size < items.length ? { next_cursor: last.id } : {}) } };
}

function change(id: string, path: string, method: string): Relationship | undefined {
  const a = accounts.get(id);
  if (a === undefined) return undefined;
  const r = relationshipOf(id);
  const on = method === "POST";
  if (path.endsWith("/follow")) {
    if (!on) Object.assign(r, { following: false, requested: false });
    else if (a["locked"]) r.requested = true;
    else r.following = true;
  }
  if (path.endsWith("/block"))
    Object.assign(r, { blocking: on, ...(on ? { following: false, followed_by: false } : {}) });
  if (path.endsWith("/mute")) r.muting = on;
  return r;
}

/** Answers a request of the client API as the server would, from the samples. */
export async function serve(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const path = url.pathname.replace(/^\/api\/v1/, "");
  const q = url.searchParams;
  const m = (pattern: RegExp) => pattern.exec(path);
  let match: RegExpExecArray | null;

  if (path === "/timelines/home") {
    const { from, body } = page(home, q.get("cursor"));
    if (failFirst && from === 0 && performance.now() < failingMs) return problem(500, "Internal Server Error");
    if (from > 0 || failFirst) await late();
    return json(body);
  }
  if (path === "/accounts/verify_credentials") return json(viewer);
  if (path === "/accounts/lookup") {
    const acct = (q.get("acct") ?? "").replace(/@my\.example$/, "");
    const found = [...accounts.values()].find((a) => a["acct"] === acct);
    return found === undefined ? problem(404, "Not Found") : json(found);
  }
  if (path === "/accounts/relationships") return json({ items: q.getAll("id").map(relationshipOf) });
  if ((match = m(/^\/accounts\/([^/]+)\/statuses$/))) {
    const id = decodeURIComponent(match[1] ?? "");
    const own = statusesOf.get(id) ?? { posts: [], pinned: [], featured: [] };
    if (q.get("pinned") === "true") return json({ items: own.pinned });
    const shown =
      q.get("exclude_replies") === "true" ? own.posts.filter((s) => s.in_reply_to_account_id === undefined) : own.posts;
    const { from, body } = page(shown, q.get("cursor"));
    if (from > 0) await late();
    return json(body);
  }
  if ((match = m(/^\/accounts\/([^/]+)\/featured_tags$/)))
    return json({ items: statusesOf.get(decodeURIComponent(match[1] ?? ""))?.featured ?? [] });
  if ((match = m(/^\/accounts\/([^/]+)\/(follow|block|mute)$/))) {
    const r = change(decodeURIComponent(match[1] ?? ""), path, request.method);
    return r === undefined ? problem(404, "Not Found") : json(r);
  }
  if ((match = m(/^\/statuses\/([^/]+)$/))) {
    const s = statuses.get(decodeURIComponent(match[1] ?? ""));
    return s === undefined ? problem(404, "Not Found") : json(s);
  }
  if ((match = m(/^\/statuses\/([^/]+)\/context$/)))
    return json(contexts.get(decodeURIComponent(match[1] ?? "")) ?? { ancestors: [], descendants: [] });
  if (path === "/notifications") {
    const types = q.getAll("types");
    const shown = types.length === 0 ? notifications : notifications.filter((n) => types.includes(n["type"] as string));
    const { from, body } = page(shown as { id: string }[], q.get("cursor"), 10);
    if (from > 0) await late();
    return json(body);
  }
  if (path === "/notifications/marker") {
    if (request.method === "PUT") {
      const { last_read_id: id } = (await request.json()) as { last_read_id: string };
      if (lastRead === undefined || id > lastRead) lastRead = id;
    }
    return json({ ...(lastRead === undefined ? {} : { last_read_id: lastRead }), unread_count: unreadCount() });
  }
  if (path === "/notifications/requests/summary")
    return json({ accounts_count: held.people, notifications_count: held.notifications });
  if (path === "/interaction_requests") return json({ items: interactions });
  if ((match = m(/^\/interaction_requests\/([^/]+)\/(accept|reject)$/))) {
    const r = interactions.find((i) => i.id === match?.[1]);
    if (r === undefined) return problem(404, "Not Found");
    r.state = match[2] === "accept" ? "accepted" : "rejected";
    return json(r);
  }
  if ((match = m(/^\/follow_requests\/([^/]+)\/(accept|reject)$/))) {
    const r = relationshipOf(decodeURIComponent(match[1] ?? ""));
    r.requested_by = false;
    if (match[2] === "accept") r.followed_by = true;
    return json(r);
  }
  return problem(404, "Not Found");
}

/** Posts `count` new samples on the home timeline, newest first, as the stream will send them. */
export function newPosts(count: number): PostStatus[] {
  const fresh = Array.from({ length: count }, () => {
    added += 1;
    const sample = posts[(added - 1) % posts.length];
    if (!sample) throw new Error("the catalog has no sample posts");
    return statusOf({ ...sample, ago: 0 }, `new-${added}`);
  }).reverse();
  home.unshift(...fresh);
  return fresh;
}
