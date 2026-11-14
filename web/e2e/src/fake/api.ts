// SPDX-License-Identifier: AGPL-3.0-only

// A fake of the client API for the screens' stories, where no server runs: a `fetch`-compatible `serve(request)` that
// answers as the server would after the seed applied the scenario (scenario.ts), as one person of it reads it. The
// statuses, the home timeline, a thread, a profile, the relationships and the notifications are derived from the
// scenario's people, follows, posts, interactions and restrictions; the IDs and times are the fake's own (the server
// decides them for real), newest last in the scenario's order. The writes of those screens (follow, block, mute, the
// answers of follow requests, the read marker) are kept in memory. A later page of a list comes a little late, so that
// loading more shows; with `failFirst` the home timeline's first page fails during the first second, and its retry
// succeeds late.

import { drawingURL } from "../drawing";
import {
  emojis,
  follows,
  interactions,
  people,
  personOf,
  posts,
  restrictions,
  type Person,
  type Post,
} from "../scenario";
import { serverHtml } from "./html";

/** The host the fake answers as. */
export const fakeHost = "demo.test";
/** The origin the fake answers as. */
export const fakeOrigin = `https://${fakeHost}`;

export type FakeOptions = {
  /** The username of the person who reads; yuki by default. */
  viewer?: string;
  /** Fails the home timeline's first page during the first second. */
  failFirst?: boolean;
  /** How late a later page comes, in ms; 2 s by default. */
  lateMs?: number;
};

type Json = Record<string, unknown>;

export type FakeApi = {
  /** Answers a request of the client API. */
  serve: (request: Request) => Promise<Response>;
  /** Adds `count` posts to the home timeline as the stream would send them, newest first, and returns them. */
  newPosts: (count: number) => Json[];
  /** Adds a notification as the stream would send it (a favourite of the reader's post), and returns it. */
  newNotification: () => Json;
  /** The app's address of the post with `key`. */
  postPath: (key: string) => string;
};

const pageSize = 6;
const minute = 60_000;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": status >= 400 ? "application/problem+json" : "application/json" },
  });
const problem = (status: number, title: string) => json({ type: "about:blank", title, status }, status);

/** An ID as the server's, a UUIDv7: in its canonical form, a later one sorts after an earlier one. */
function idAt(ms: number, n: number): string {
  const time = Math.max(0, Math.round(ms)).toString(16).padStart(12, "0");
  return `${time.slice(0, 8)}-${time.slice(8)}-7000-8000-${n.toString(16).padStart(12, "0")}`;
}

/** The usernames a text mentions, among the people. */
const mentioned = (text: string) =>
  [...text.matchAll(/@(\w+)/g)].map((m) => m[1] ?? "").filter((u) => people.some((p) => p.username === u));

const customEmojis = emojis.map((e) => {
  const url = drawingURL(e.drawing);
  return { shortcode: e.shortcode, url, static_url: url };
});

/** Creates a fake of the client API as `viewer` reads it. */
export function createFakeApi(o: FakeOptions = {}): FakeApi {
  const viewer = personOf(o.viewer ?? "yuki").username;
  const lateMs = o.lateMs ?? 2000;
  const started = Date.now();
  const late = () => new Promise((done) => setTimeout(done, lateMs));

  // The scenario's steps in order, follows, then posts, then interactions, a few minutes apart up to now.
  const steps = follows.length + posts.length + interactions.length;
  const timeOf = (step: number) => started - (steps - step) * 6 * minute;

  // ---- People and relationships -------------------------------------------------------------------------------------

  const accountId = new Map(people.map((p, i) => [p.username, idAt(0, i + 1)]));
  const usernameOf = new Map([...accountId].map(([u, id]) => [id, u]));
  const idOf = (username: string) => accountId.get(username) ?? "";
  const following = new Set<string>();
  const requested = new Set<string>();
  for (const f of follows) {
    const edge = `${f.follower}>${f.followee}`;
    if (personOf(f.followee).locked === true && f.accepted !== true) requested.add(edge);
    else following.add(edge);
  }
  const blocking = new Set(restrictions.filter((r) => r.kind === "block").map((r) => `${r.by}>${r.target}`));
  const muting = new Set(restrictions.filter((r) => r.kind === "mute").map((r) => `${r.by}>${r.target}`));
  const follows_ = (a: string, b: string) => following.has(`${a}>${b}`);
  const hidden = (author: string) => blocking.has(`${viewer}>${author}`) || muting.has(`${viewer}>${author}`);

  const accountOf = (username: string): Json => {
    const p = personOf(username);
    const avatar = drawingURL(p.avatar);
    return {
      id: idOf(username),
      username,
      acct: username,
      domain: fakeHost,
      display_name: p.displayName,
      emojis: [],
      avatar: { small: { url: avatar }, medium: { url: avatar } },
    };
  };

  const fullAccount = (p: Person): Json => {
    const header = p.header === undefined ? undefined : drawingURL(p.header);
    return {
      ...accountOf(p.username),
      uri: `${fakeOrigin}/users/${p.username}`,
      url: `${fakeOrigin}/@${p.username}`,
      note: serverHtml(p.note, fakeHost),
      locked: p.locked ?? false,
      bot: false,
      created_at: new Date(started - 30 * 24 * 60 * minute).toISOString(),
      fields: p.fields.map(([name, value]) => ({ name, value: serverHtml(value, fakeHost) })),
      badges: [],
      ...(header === undefined ? {} : { header: { small: { url: header }, medium: { url: header } } }),
      followers_count: people.filter((x) => follows_(x.username, p.username)).length,
      following_count: people.filter((x) => follows_(p.username, x.username)).length,
      statuses_count: posts.filter((x) => x.author === p.username).length,
    };
  };

  const relationshipOf = (id: string): Json => {
    const other = usernameOf.get(id) ?? "";
    return {
      id,
      following: follows_(viewer, other),
      requested: requested.has(`${viewer}>${other}`),
      followed_by: follows_(other, viewer),
      requested_by: requested.has(`${other}>${viewer}`),
      blocking: blocking.has(`${viewer}>${other}`),
      muting: muting.has(`${viewer}>${other}`),
      muting_notifications: muting.has(`${viewer}>${other}`),
      notify: false,
      show_boosts: true,
      note: "",
    };
  };

  // ---- Posts -----------------------------------------------------------------------------------------------------

  const postStep = (key: string) => follows.length + posts.findIndex((p) => p.key === key);
  const statusId = new Map(posts.map((p) => [p.key, idAt(timeOf(postStep(p.key)), postStep(p.key))]));
  const keyOf = new Map([...statusId].map(([k, id]) => [id, k]));
  const postOf = (key: string): Post => {
    const p = posts.find((x) => x.key === key);
    if (p === undefined) throw new Error(`fake: no post ${key}`);
    return p;
  };
  const on = (key: string, kind: string) => interactions.filter((i) => i.post === key && i.kind === kind);

  const visible = (p: Post) =>
    p.author === viewer ||
    p.visibility === "public" ||
    p.visibility === "unlisted" ||
    p.visibility === "local" ||
    (p.visibility === "private" && follows_(viewer, p.author)) ||
    (p.visibility === "direct" && mentioned(p.text).includes(viewer));

  function statusOf(p: Post, depth = 0): Json {
    const id = statusId.get(p.key) ?? "";
    const parent = p.replyTo === undefined ? undefined : postOf(p.replyTo);
    const reactions = new Map<string, { count: number; me: boolean }>();
    for (const r of interactions)
      if (r.kind === "reaction" && r.post === p.key) {
        const now = reactions.get(r.emoji) ?? { count: 0, me: false };
        reactions.set(r.emoji, { count: now.count + 1, me: now.me || r.by === viewer });
      }
    return {
      id,
      uri: `${fakeOrigin}/users/${p.author}/statuses/${id}`,
      url: `${fakeOrigin}/@${p.author}/${id}`,
      account: accountOf(p.author),
      content: serverHtml(p.text, fakeHost),
      spoiler_text: p.spoiler ?? "",
      sensitive: p.spoiler !== undefined,
      visibility: p.visibility,
      language: p.language,
      created_at: new Date(timeOf(postStep(p.key))).toISOString(),
      ...(parent === undefined
        ? {}
        : { in_reply_to_id: statusId.get(parent.key), in_reply_to_account_id: idOf(parent.author) }),
      media_attachments: (p.images ?? []).map((image, i) => {
        const url = drawingURL(image.drawing);
        return { id: `${id}-${i}`, type: "image", description: image.description, url, preview_url: url };
      }),
      mentions: mentioned(p.text).map((u) => ({ id: idOf(u), username: u, acct: u })),
      emojis: customEmojis,
      replies_count: posts.filter((x) => x.replyTo === p.key).length,
      boosts_count: on(p.key, "boost").length,
      favourites_count: on(p.key, "favourite").length,
      reactions: [...reactions].map(([emoji, r]) => {
        const custom = customEmojis.find((e) => `:${e.shortcode}:` === emoji);
        return custom === undefined
          ? { name: emoji, count: r.count, me: r.me }
          : { name: custom.shortcode, shortcode: custom.shortcode, emoji: custom, count: r.count, me: r.me };
      }),
      favourited: on(p.key, "favourite").some((i) => i.by === viewer),
      boosted: on(p.key, "boost").some((i) => i.by === viewer),
      bookmarked: on(p.key, "bookmark").some((i) => i.by === viewer),
      pinned: p.pinned === true && p.author === viewer,
      ...(p.quote === undefined || depth > 0
        ? {}
        : { quote: { state: "accepted", quoted_status: statusOf(postOf(p.quote), depth + 1) } }),
    };
  }

  /** A boost of `p` by `by`, at the interaction's step. */
  const boostOf = (p: Post, by: string, step: number): Json => {
    const shown = statusOf(p);
    return {
      ...shown,
      id: idAt(timeOf(step), step),
      created_at: new Date(timeOf(step)).toISOString(),
      account: accountOf(by),
      boost: shown,
    };
  };

  // The home timeline, newest first: the reader's posts and those of whom they follow (a reply only to one of
  // them), with their boosts; what the reader blocks or mutes is left out.
  const inHome = (author: string) => author === viewer || follows_(viewer, author);
  const homeEntries: { at: number; status: Json }[] = [];
  for (const p of posts) {
    const parent = p.replyTo === undefined ? undefined : postOf(p.replyTo);
    if (!inHome(p.author) || hidden(p.author) || !visible(p)) continue;
    if (parent !== undefined && !inHome(parent.author)) continue;
    homeEntries.push({ at: postStep(p.key), status: statusOf(p) });
  }
  interactions.forEach((i, n) => {
    if (i.kind !== "boost" || !inHome(i.by) || i.by === viewer) return;
    const p = postOf(i.post);
    if (hidden(p.author) || !visible(p) || p.visibility === "private" || p.visibility === "direct") return;
    const step = follows.length + posts.length + n;
    homeEntries.push({ at: step, status: boostOf(p, i.by, step) });
  });
  const home: Json[] = homeEntries.toSorted((a, b) => b.at - a.at).map((e) => e.status);

  /** A person's posts on their profile, newest first, and the pinned ones. */
  const profilePosts = (username: string) =>
    posts
      .filter((p) => p.author === username && visible(p))
      .toReversed()
      .map((p) => statusOf(p));

  /** The ancestors and descendants of a post, depth first, without what the reader blocks or mutes. */
  function contextOf(key: string) {
    const ancestors: Json[] = [];
    for (let p = postOf(key).replyTo; p !== undefined; p = postOf(p).replyTo) {
      const parent = postOf(p);
      if (visible(parent) && !hidden(parent.author)) ancestors.unshift(statusOf(parent));
    }
    const descendants: Json[] = [];
    const walk = (k: string) => {
      for (const child of posts.filter((x) => x.replyTo === k && visible(x) && !hidden(x.author))) {
        descendants.push(statusOf(child));
        walk(child.key);
      }
    };
    walk(key);
    return { ancestors, descendants };
  }

  // ---- Notifications ---------------------------------------------------------------------------------------------

  const notifications: Json[] = [];
  const notify = (step: number, type: string, from: string, more: Json = {}) => {
    if (from === viewer || hidden(from)) return;
    const at = timeOf(step);
    notifications.unshift({
      id: idAt(at, 10_000 + step),
      type,
      created_at: new Date(at).toISOString(),
      account: accountOf(from),
      ...more,
    });
  };
  follows.forEach((f, step) => {
    if (f.followee !== viewer) return;
    notify(step, requested.has(`${f.follower}>${f.followee}`) ? "follow_request" : "follow", f.follower);
  });
  posts.forEach((p, n) => {
    const step = follows.length + n;
    if (mentioned(p.text).includes(viewer)) notify(step, "mention", p.author, { status: statusOf(p) });
    if (p.quote !== undefined && postOf(p.quote).author === viewer)
      notify(step, "quote", p.author, { status: statusOf(p) });
  });
  interactions.forEach((i, n) => {
    const p = postOf(i.post);
    if (p.author !== viewer || i.kind === "bookmark") return;
    const step = follows.length + posts.length + n;
    const reaction = i.kind === "reaction" ? { reaction: { name: i.emoji.replace(/^:|:$/g, "") } } : {};
    notify(step, i.kind, i.by, { status: statusOf(p), ...reaction });
  });
  // The reader read all but the three newest.
  let lastRead = notifications[3]?.["id"] as string | undefined;
  const unreadCount = () =>
    notifications.filter((n) => lastRead === undefined || (n["id"] as string) > lastRead).length;

  // ---- Answers ---------------------------------------------------------------------------------------------------

  function page<T extends Json>(items: readonly T[], cursor: string | null, size = pageSize) {
    const from = cursor === null ? 0 : items.findIndex((s) => s["id"] === cursor) + 1;
    const slice = items.slice(from, from + size);
    const last = slice.at(-1);
    return { from, body: { items: slice, ...(last && from + size < items.length ? { next_cursor: last["id"] } : {}) } };
  }

  function change(id: string, what: string, add: boolean): Json | undefined {
    const other = usernameOf.get(id);
    if (other === undefined) return undefined;
    const edge = `${viewer}>${other}`;
    if (what === "follow") {
      following.delete(edge);
      requested.delete(edge);
      if (add) (personOf(other).locked === true ? requested : following).add(edge);
    }
    if (what === "block") {
      if (add) {
        blocking.add(edge);
        following.delete(edge);
        following.delete(`${other}>${viewer}`);
      } else blocking.delete(edge);
    }
    if (what === "mute" && add) muting.add(edge);
    if (what === "mute" && !add) muting.delete(edge);
    return relationshipOf(id);
  }

  async function serve(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const q = url.searchParams;
    const m = (pattern: RegExp) => pattern.exec(path);
    let match: RegExpExecArray | null;

    if (path === "/timelines/home") {
      const { from, body } = page(home, q.get("cursor"));
      if (o.failFirst === true && from === 0 && Date.now() - started < 1000)
        return problem(500, "Internal Server Error");
      if (from > 0 || o.failFirst === true) await late();
      return json(body);
    }
    if (path === "/accounts/verify_credentials") return json(fullAccount(personOf(viewer)));
    if (path === "/accounts/lookup") {
      const acct = (q.get("acct") ?? "").replace(new RegExp(`@${fakeHost}$`), "");
      const found = people.find((p) => p.username === acct);
      return found === undefined ? problem(404, "Not Found") : json(fullAccount(found));
    }
    if (path === "/accounts/relationships") return json({ items: q.getAll("id").map(relationshipOf) });
    if ((match = m(/^\/accounts\/([^/]+)$/))) {
      const username = usernameOf.get(decodeURIComponent(match[1] ?? ""));
      return username === undefined ? problem(404, "Not Found") : json(fullAccount(personOf(username)));
    }
    if ((match = m(/^\/accounts\/([^/]+)\/statuses$/))) {
      const username = usernameOf.get(decodeURIComponent(match[1] ?? "")) ?? "";
      const all = profilePosts(username);
      if (q.get("pinned") === "true")
        return json({ items: all.filter((s) => postOf(keyOf.get(s["id"] as string) ?? "").pinned) });
      const shown = q.get("exclude_replies") === "true" ? all.filter((s) => s["in_reply_to_id"] === undefined) : all;
      const { from, body } = page(shown, q.get("cursor"));
      if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/accounts\/([^/]+)\/featured_tags$/))) return json({ items: [] });
    if ((match = m(/^\/accounts\/([^/]+)\/(follow|unfollow|block|unblock|mute|unmute)$/))) {
      const verb = match[2] ?? "";
      const add = request.method === "POST" && !verb.startsWith("un");
      const r = change(decodeURIComponent(match[1] ?? ""), verb.replace(/^un/, ""), add);
      return r === undefined ? problem(404, "Not Found") : json(r);
    }
    if ((match = m(/^\/statuses\/([^/]+)$/))) {
      const key = keyOf.get(decodeURIComponent(match[1] ?? ""));
      return key === undefined || !visible(postOf(key)) ? problem(404, "Not Found") : json(statusOf(postOf(key)));
    }
    if ((match = m(/^\/statuses\/([^/]+)\/context$/))) {
      const key = keyOf.get(decodeURIComponent(match[1] ?? ""));
      return key === undefined ? problem(404, "Not Found") : json(contextOf(key));
    }
    if (path === "/notifications") {
      const types = q.getAll("types");
      const shown =
        types.length === 0 ? notifications : notifications.filter((n) => types.includes(n["type"] as string));
      const { from, body } = page(shown, q.get("cursor"), 10);
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
    if (path === "/notifications/requests/summary") return json({ accounts_count: 0, notifications_count: 0 });
    if (path === "/interaction_requests") return json({ items: [] });
    if ((match = m(/^\/follow_requests\/([^/]+)\/(accept|reject)$/))) {
      const other = usernameOf.get(decodeURIComponent(match[1] ?? ""));
      if (other === undefined || !requested.delete(`${other}>${viewer}`)) return problem(404, "Not Found");
      if (match[2] === "accept") following.add(`${other}>${viewer}`);
      return json(relationshipOf(idOf(other)));
    }
    return problem(404, "Not Found");
  }

  let added = 0;

  function newPosts(count: number): Json[] {
    const pool = posts.filter((p) => p.author !== viewer && inHome(p.author) && !hidden(p.author) && visible(p));
    const fresh = Array.from({ length: count }, () => {
      added += 1;
      const p = pool[(added - 1) % pool.length];
      if (p === undefined) throw new Error("fake: the reader follows no one who posts");
      const at = Date.now();
      return { ...statusOf(p), id: idAt(at, 900_000 + added), created_at: new Date(at).toISOString() };
    }).toReversed();
    home.unshift(...fresh);
    return fresh;
  }

  function newNotification(): Json {
    added += 1;
    const fans = people.filter((p) => follows_(p.username, viewer) && !hidden(p.username));
    const from = fans[added % Math.max(1, fans.length)]?.username ?? viewer;
    const own = posts.find((p) => p.author === viewer);
    const at = Date.now();
    const n: Json = {
      id: idAt(at, 900_000 + added),
      type: "favourite",
      created_at: new Date(at).toISOString(),
      account: accountOf(from),
      ...(own === undefined ? {} : { status: statusOf(own) }),
    };
    notifications.unshift(n);
    return n;
  }

  const postPath = (key: string) => `/@${postOf(key).author}/status/${statusId.get(key) ?? ""}`;

  return { serve, newPosts, newNotification, postPath };
}
