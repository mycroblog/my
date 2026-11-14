// SPDX-License-Identifier: AGPL-3.0-only

// A fake of the client API for the screens' stories, where no server runs: a `fetch`-compatible `serve(request)` that
// answers as the server would after the seed applied the scenario (scenario.ts), as one person of it reads it. The
// statuses, the home timeline, a thread, a profile, the relationships and the notifications are derived from the
// scenario's people, follows, posts, interactions and restrictions; the IDs and times are the fake's own (the server
// decides them for real), newest last in the scenario's order. The writes of those screens (follow, block, mute, the
// answers of follow requests, the read marker, the announcements read) are kept in memory. A list pages as the server's
// do: `limit` items a page, 20 by default, with a cursor while more follow, so that a list read to its end ends; a later
// page comes late only with `lateMs`. With `failFirst` the home timeline's first page fails during the first second,
// and its retry succeeds late. What the shell reads (the instance, the reader's rights, the announcements, the
// voice rooms, the events) answers in the state the options give the server: its timelines, one person's, its closure,
// its maintenance, down, or newer than the app. The voice rooms are the fake's own, since a room lives only while
// people are in it and the seed opens none.

import { drawingURL } from "../drawing";
import {
  admin,
  announcements as scenarioAnnouncements,
  emojis,
  eventStart,
  events as scenarioEvents,
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

/** Who may read a public timeline (the API's InstanceAudience). */
export type Audience = "off" | "signed_in" | "everyone";

export type FakeOptions = {
  /** The username of the person who reads; yuki by default. Their roles (the scenario's `staff`) set their rights. */
  viewer?: string;
  /** The reader has no session: what needs one answers 401. */
  visitor?: boolean;
  /** Fails the home timeline's first page during the first second. */
  failFirst?: boolean;
  /**
   * How late a later page comes, in ms, so that loading more shows; none by default, as a server answers at once. The
   * retry of `failFirst` comes 2 s late unless this says otherwise.
   */
  lateMs?: number;
  /** Who may read the local and the federated timelines; everyone by default. */
  timelines?: { local: Audience; federated: Audience };
  /** A one-person server, of the scenario's administrator: no public timelines. */
  singleUser?: boolean;
  /** The instance has a logo of its own. */
  logo?: boolean;
  /** The instance has a thumbnail, the image that represents it. */
  thumbnail?: boolean;
  /** The registration mode; open by default, closed on a one-person server. */
  registrations?: "open" | "approval" | "invite" | "closed";
  /** The instance's summary runs long. */
  longSummary?: boolean;
  /** The closure of the service: announced, or the server closed. */
  closure?: "announced" | "closed";
  /** The server is under maintenance: every operation answers its problem. */
  maintenance?: boolean;
  /**
   * The server stops answering, all but the first request of `/instance` (which the shell read before): the reader's
   * device has no network (`offline`, every request fails), or the server is down (`outage`, it answers 503 and its
   * health `unavailable`).
   */
  down?: "offline" | "outage";
  /** The server's web client epoch is above the app's, which then needs an update. */
  newerEpoch?: boolean;
  /** What is around the reader beside the timeline: everything by default, or nothing. */
  around?: "all" | "none";
  /** The voice chat rooms are off: the instance says so and `/voice/...` answers 404. */
  voiceOff?: boolean;
  /** The reader's latest export of their data: none by default, one being built, or one ready to download. */
  dataExport?: "none" | "building" | "ready";
};

/** The address of the fake's status page. */
export const fakeStatusPage = "https://status.demo.test";

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
  /** The app's address of the scenario's announcement with `key`, or of the fake's one that ended (`ended`). */
  announcementPath: (key: string) => string;
};

/** The items of a page when the request names no `limit`, and the most it may name: the server's (Limit). */
const pageSize = 20;
const maxPageSize = 40;
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
  const started = Date.now();
  const wait = (ms: number) => new Promise((done) => setTimeout(done, ms));
  /** A later page's delay: none unless `lateMs` asks for one. */
  const late = () => wait(o.lateMs ?? 0);

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
    // A reaction's emoji as the API gives it: a custom one of this server with its shortcode and image, else Unicode.
    const custom = i.kind === "reaction" ? customEmojis.find((e) => `:${e.shortcode}:` === i.emoji) : undefined;
    const reaction =
      i.kind !== "reaction"
        ? {}
        : custom === undefined
          ? { reaction: { name: i.emoji } }
          : { reaction: { name: custom.shortcode, shortcode: custom.shortcode, emoji: custom } };
    notify(step, i.kind, i.by, { status: statusOf(p), ...reaction });
  });
  // The reader read all but the three newest.
  let lastRead = notifications[3]?.["id"] as string | undefined;
  const unreadCount = () =>
    notifications.filter((n) => lastRead === undefined || (n["id"] as string) > lastRead).length;

  // ---- The shell: the instance, the reader's rights, what is around them ------------------------------------------

  const day = 24 * 60 * minute;
  const permissionsOf = (username: string): string[] => {
    const staff = personOf(username).staff;
    const moderation = ["manage_reports", "manage_accounts", "manage_domains", "manage_invites", "view_audit"];
    if (staff === "moderator") return moderation;
    if (staff !== "admin") return [];
    return [
      ...moderation,
      ...["manage_settings", "manage_rules", "manage_roles", "manage_jobs", "manage_announcements"],
      ...["manage_custom_emojis", "manage_webhooks", "manage_finance", "view_finance", "manage_service_closure"],
    ];
  };
  const brandImage = (svg: string) => ({
    revision: "1",
    svg_url: `data:image/svg+xml,${encodeURIComponent(svg)}`,
    width: 64,
    height: 64,
    renditions: [],
  });
  const logoSvg = (fill: string) =>
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="14" fill="${fill}"/><circle cx="32" cy="32" r="14" fill="#fff"/></svg>`;
  // A landscape of hills under a sun, at the 3:1 of a banner.
  const thumbnailSvg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 400"><rect width="1200" height="400" fill="#d6e6df"/>` +
    `<circle cx="960" cy="110" r="46" fill="#f6efe2"/><path d="M0 290 Q300 200 620 270 T1200 230 V400 H0Z" fill="#86b8a2"/>` +
    `<path d="M0 340 Q420 280 800 330 T1200 310 V400 H0Z" fill="#2f6f5b"/></svg>`;
  const summary =
    o.longSummary === true
      ? "A small community to try mycroblog with, run by volunteers who meet once a month. We post about the river " +
        "park, the town's events, lost and found, and photos of each season; newcomers are welcome to ask anything, " +
        "and the moderators answer reports within a day. 小さなコミュニティで mycroblog を試せます。"
      : "A small community to try mycroblog with.";
  const timelines =
    o.singleUser === true
      ? { local: "off", federated: "off" }
      : (o.timelines ?? { local: "everyone", federated: "everyone" });
  const around = o.around ?? "all";
  const instanceBody: Json = {
    domain: fakeHost,
    name: "mycroblog demo",
    short_description: summary,
    version: "0.0.0-demo",
    source_url: "https://example.invalid/mycroblog/my",
    status_page_url: fakeStatusPage,
    terms_url: `${fakeOrigin}/about/terms`,
    privacy_policy_url: `${fakeOrigin}/about/privacy`,
    // A one-person server and a closure close the registration whatever the mode (GET /instance shows the effective one).
    registrations: {
      mode: o.singleUser === true || o.closure !== undefined ? "closed" : (o.registrations ?? "open"),
    },
    login_providers_only: false,
    limits: { max_post_characters: 500, require_media_description: false },
    timelines,
    federation: { mode: "off" },
    sends_email: false,
    rules: [],
    search_indexing: "allow",
    ai_usage: "allow",
    link_cards: false,
    link_cards_ignore_robots_txt: false,
    finance_visibility: "private",
    income_visibility: "private",
    supporters_enabled: false,
    tips_enabled: false,
    supporter_list_visibility: "off",
    voice_enabled: o.voiceOff !== true,
    voice_room_starters: "everyone",
    full_text_search: { enabled: true },
    translation: { enabled: false },
    single_user: o.singleUser === true,
    ...(o.singleUser === true ? { single_user_account: fullAccount(personOf(admin)) } : {}),
    ...(o.logo === true ? { logo: brandImage(logoSvg("#2f6f8f")), logo_dark: brandImage(logoSvg("#8fc7e0")) } : {}),
    ...(o.thumbnail === true
      ? {
          thumbnail: {
            url: `data:image/svg+xml,${encodeURIComponent(thumbnailSvg)}`,
            width: 1200,
            height: 400,
            alt: "Green hills under a pale sun.",
          },
        }
      : {}),
    ...(o.closure === undefined
      ? {}
      : {
          closure: {
            state: o.closure,
            closes_at: new Date(started + (o.closure === "announced" ? 60 : -5) * day).toISOString(),
            export_until: new Date(started + (o.closure === "announced" ? 90 : 25) * day).toISOString(),
            message: "Thank you for the good years together. — the admins",
          },
        }),
    ...(around === "all"
      ? {
          funding_meter: {
            visibility: "percent",
            label: new Date(started).toISOString().slice(0, 7),
            starts_on: `${new Date(started).toISOString().slice(0, 7)}-01`,
            ends_on: `${new Date(started).toISOString().slice(0, 7)}-28`,
            percent: 72,
            covered: false,
            // A usage charge of the month is not closed yet, so the share may still change.
            estimated: true,
            rail: true,
          },
        }
      : {}),
  };

  // The announcements, newest first, each read by whom the scenario says; reads are kept.
  const announcementRead = new Set(
    scenarioAnnouncements.filter((a) => a.readBy?.includes(viewer) === true).map((a) => a.key),
  );
  const announcementId = (key: string) =>
    idAt(
      started - (scenarioAnnouncements.length - scenarioAnnouncements.findIndex((a) => a.key === key)) * day,
      20_000,
    );
  const announcementOf = (key: string): Json => {
    const a = scenarioAnnouncements.find((x) => x.key === key);
    const id = announcementId(key);
    const at = new Date(
      started - (scenarioAnnouncements.length - scenarioAnnouncements.findIndex((x) => x.key === key)) * day,
    );
    return {
      id,
      title: a?.title ?? "",
      content: serverHtml(a?.text ?? "", fakeHost),
      published_at: at.toISOString(),
      revised_at: at.toISOString(),
      read: announcementRead.has(key),
      ended: false,
    };
  };
  const shownAnnouncements = () =>
    around === "none" ? [] : scenarioAnnouncements.toReversed().map((a) => announcementOf(a.key));
  // One whose window ended a month ago, which the history lists after the ones showing now: the seed publishes none,
  // since a window that ended cannot be set on a new announcement.
  const endedAnnouncement: Json = {
    id: idAt(started - 40 * day, 20_001),
    title: "The server moved to a new data center",
    content: serverHtml("The move is done. Thank you for your patience during the short downtime.", fakeHost),
    published_at: new Date(started - 40 * day).toISOString(),
    ends_at: new Date(started - 30 * day).toISOString(),
    revised_at: new Date(started - 40 * day).toISOString(),
    read: false,
    ended: true,
  };
  const announcementHistory = () => (around === "none" ? [] : [...shownAnnouncements(), endedAnnouncement]);

  // The open voice rooms: rooms are live, so the scenario, which the seed applies, holds none; these stand in.
  const rooms = [
    { name: "Morning coffee chat", host: "sam", count: 12 },
    { name: "Release party for v0.2", host: "hana", count: 48 },
    { name: "Photo walk planning: which trail this weekend, and who brings the long lens", host: "yuki", count: 3 },
    { name: "Arabic and English, slowly", host: "layla", count: 5 },
  ].map((r, i) => ({
    id: idAt(started - (i + 1) * 30 * minute, 30_000 + i),
    name: r.name,
    description: "",
    hashtags: [],
    visibility: "public",
    speakers: "everyone",
    host: accountOf(r.host),
    opened_at: new Date(started - (i + 1) * 30 * minute).toISOString(),
    participants_count: r.count,
    participants: [],
  }));

  // The reader's exports of their data, newest first; a request adds one being built, which stays so.
  const dataExports: Json[] = [];
  if (o.dataExport === "building")
    dataExports.push({
      id: idAt(started - 10 * minute, 50_100),
      state: "pending",
      requested_at: new Date(started - 10 * minute).toISOString(),
    });
  if (o.dataExport === "ready")
    dataExports.push({
      id: idAt(started - day, 50_100),
      state: "ready",
      requested_at: new Date(started - day).toISOString(),
      completed_at: new Date(started - day + 20 * minute).toISOString(),
      expires_at: new Date(started + 6 * day).toISOString(),
      file_size: 4_812_345,
    });

  const eventOf = (e: (typeof scenarioEvents)[number], i: number): Json => {
    const startsAt = eventStart(e, started);
    return {
      id: idAt(startsAt.getTime(), 40_000 + i),
      organizer: accountOf(e.organizer),
      coorganizers: [],
      title: e.title,
      description: e.description,
      visibility: "public",
      time_zone: e.timeZone,
      place_disclosure: "approved",
      capacity: 0,
      places: [],
      hidden_places: [],
      starts_at: startsAt.toISOString(),
      ends_at: new Date(startsAt.getTime() + e.hours * 60 * minute).toISOString(),
      state: "scheduled",
      going_count: 4,
      created_at: new Date(started - day).toISOString(),
      revised_at: new Date(started - day).toISOString(),
      revision_count: 1,
      viewer: {},
    };
  };

  // ---- Answers ---------------------------------------------------------------------------------------------------

  /** The page of `items` that the request's `cursor` and `limit` ask for, as the server pages a list. */
  function page<T extends Json>(items: readonly T[], query: URLSearchParams) {
    const cursor = query.get("cursor");
    const limit = Number(query.get("limit") ?? pageSize);
    const size = Number.isInteger(limit) && limit > 0 ? Math.min(limit, maxPageSize) : pageSize;
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

  /** The operations a visitor without a session may call; every other answers 401. */
  const publicPath =
    /^\/(?:instance(?:\/health)?|accounts\/lookup|accounts\/[^/]+(?:\/statuses|\/featured_tags)?|statuses\/[^/]+(?:\/context)?|timelines\/(?:local|federated))$/;
  /** Whether `/instance` answered once, before the server stopped answering (`down`). */
  let instanceRead = false;

  /** The answer of the server in the state of the options: down, under maintenance, or as it is. */
  async function serve(request: Request): Promise<Response> {
    const path = new URL(request.url).pathname.replace(/^\/api\/v1/, "");
    const down = o.down !== undefined && (path !== "/instance" || instanceRead);
    if (path === "/instance") instanceRead = true;
    if (down && o.down === "offline") throw new TypeError("Failed to fetch");
    if (down && o.down === "outage")
      return path === "/instance/health"
        ? json({ status: "unavailable", checked_at: new Date().toISOString() }, 503)
        : problem(503, "Service Unavailable");
    if (o.maintenance === true)
      return path === "/instance/health"
        ? new Response(JSON.stringify({ status: "maintenance" }), {
            status: 503,
            headers: { "Content-Type": "application/json", "Retry-After": "3600" },
          })
        : new Response(
            JSON.stringify({
              type: "/api/v1/problems/maintenance",
              title: "Under maintenance",
              status: 503,
              message: "Moving the database to a larger disk. Thanks for your patience! — the admins",
              until: new Date(started + 2 * 60 * minute).toISOString(),
              status_page_url: fakeStatusPage,
            }),
            { status: 503, headers: { "Content-Type": "application/problem+json", "Retry-After": "3600" } },
          );
    if (o.visitor === true && !publicPath.test(path)) return problem(401, "Unauthorized");
    const response = await answer(request);
    if (o.newerEpoch === true) response.headers.set("X-Web-Client-Epoch", "999");
    return response;
  }

  async function answer(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const q = url.searchParams;
    const m = (pattern: RegExp) => pattern.exec(path);
    let match: RegExpExecArray | null;

    if (path === "/timelines/home") {
      const { from, body } = page(home, q);
      if (o.failFirst === true && from === 0 && Date.now() - started < 1000)
        return problem(500, "Internal Server Error");
      // The retry after the first page failed comes late, so that the screen shows it loading again.
      if (o.failFirst === true) await wait(o.lateMs ?? 2000);
      else if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/timelines\/(local|federated)$/))) {
      // The scenario's people are all this server's, so both public timelines hold their public posts (the local one
      // also those for this server only), newest first; a timeline the instance does not offer is a 404, one for
      // signed-in readers a 401 to a visitor.
      const kind = match[1] === "local" ? "local" : "federated";
      if (timelines[kind] === "off") return problem(404, "Not Found");
      if (timelines[kind] === "signed_in" && o.visitor === true) return problem(401, "Unauthorized");
      const shown = posts
        .filter((p) => p.visibility === "public" || (kind === "local" && p.visibility === "local"))
        .filter((p) => o.visitor === true || !hidden(p.author))
        .toReversed()
        .map((p) => statusOf(p));
      const { from, body } = page(shown, q);
      if (from > 0) await late();
      return json(body);
    }
    if (path === "/accounts/verify_credentials")
      return json({ ...fullAccount(personOf(viewer)), permissions: permissionsOf(viewer) });
    if (path === "/instance") return json(instanceBody);
    if (path === "/instance/health") return json({ status: "ok", checked_at: new Date().toISOString() });
    if (path === "/announcements") return json({ items: shownAnnouncements() });
    if (path === "/announcements/history") return json(page(announcementHistory(), q).body);
    if ((match = m(/^\/announcements\/([^/]+)$/)) && request.method === "GET") {
      const found = announcementHistory().find((a) => a["id"] === decodeURIComponent(match?.[1] ?? ""));
      return found === undefined ? problem(404, "Not Found") : json(found);
    }
    if ((match = m(/^\/announcements\/([^/]+)\/read$/)) && request.method === "POST") {
      const a = scenarioAnnouncements.find((x) => announcementId(x.key) === decodeURIComponent(match?.[1] ?? ""));
      if (a === undefined) return problem(404, "Not Found");
      announcementRead.add(a.key);
      return json(announcementOf(a.key));
    }
    if (o.voiceOff === true && path.startsWith("/voice/")) return problem(404, "Not Found");
    if (path === "/voice/rooms") {
      const shown = around === "none" ? [] : rooms;
      return json(page(shown, q).body);
    }
    if (path === "/events") {
      const shown = around === "none" ? [] : scenarioEvents.map(eventOf);
      return json(page(shown, q).body);
    }
    if (path === "/admin/counts") {
      const granted = permissionsOf(viewer);
      return json({
        ...(granted.includes("manage_reports") ? { open_reports: 3 } : {}),
        ...(granted.includes("manage_accounts") ? { pending_registrations: 2, pending_appeals: 1 } : {}),
      });
    }
    if (path === "/exports" && request.method === "POST") {
      if (dataExports.some((e) => e["state"] === "pending"))
        return json(
          { type: "/api/v1/problems/export-in-progress", title: "An export is in progress", status: 409 },
          409,
        );
      const made = {
        id: idAt(Date.now(), 50_000 + dataExports.length),
        state: "pending",
        requested_at: new Date().toISOString(),
      };
      dataExports.unshift(made);
      return json(made, 202);
    }
    if (path === "/exports") return json({ items: dataExports });
    if ((match = m(/^\/exports\/([^/]+)\/archive$/))) {
      const found = dataExports.find((e) => e["id"] === decodeURIComponent(match?.[1] ?? ""));
      if (found === undefined) return problem(404, "Not Found");
      if (found["state"] !== "ready")
        return json({ type: "/api/v1/problems/export-not-ready", title: "The export is not ready", status: 409 }, 409);
      return new Response(new Uint8Array([0x50, 0x4b, 0x05, 0x06, ...new Array<number>(18).fill(0)]), {
        headers: { "Content-Type": "application/zip", "Content-Disposition": 'attachment; filename="export.zip"' },
      });
    }
    if ((match = m(/^\/csv_exports\/([^/]+)$/))) {
      const kind = decodeURIComponent(match[1] ?? "");
      const rows = people
        .filter((p) => follows_(viewer, p.username))
        .map((p) => `${p.username}@${fakeHost},true,false,`);
      return new Response(["Account address,Show boosts,Notify on new posts,Languages", ...rows].join("\n"), {
        headers: {
          "Content-Type": "text/csv",
          "Content-Disposition": `attachment; filename="${kind === "following" ? "following_accounts" : kind}.csv"`,
        },
      });
    }
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
      const { from, body } = page(shown, q);
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
      const { from, body } = page(shown, q);
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
  const announcementPath = (key: string) =>
    `/announcements/${key === "ended" ? String(endedAnnouncement["id"]) : announcementId(key)}`;

  return { serve, newPosts, newNotification, postPath, announcementPath };
}
