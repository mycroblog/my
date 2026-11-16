// SPDX-License-Identifier: AGPL-3.0-only

// A fake of the client API for the screens' stories, where no server runs: a `fetch`-compatible `serve(request)` that
// answers as the server would after the seed applied the scenario (scenario.ts), as one person of it reads it. The
// statuses, the home timeline, a thread, a profile with its lists of follows and followers (as far as their owner shows
// them), the relationships and the notifications are derived from the
// scenario's people, follows, posts, interactions and restrictions; the IDs and times are the fake's own (the server
// decides them for real), newest last in the scenario's order; a person of another server is the fake's own, since the
// seed's server federates with none, one of them moved, and so is a person of this server who deactivated their
// account. The writes of those screens (follow, block, mute, a private note, being notified
// of a person's posts, a follower removed, a server muted as ADR-0013 §30 hides it, the
// answers of follow requests, the read marker, the announcements read, the conversations of the direct messages read
// and hidden, the reader's bookmarks and followed hashtags, the reader's edits and deletions of their own posts, the
// reader's drafts and their schedules, a draft published, the reader's articles posted and edited and the images they
// upload) are kept in memory; a post's edit history holds the scenario's versions of it, then the reader's edits. An
// article's body comes only with the status read alone, as the server's does, and its preview renders as publishing
// would, unless the options make it fail. A list pages as
// the server's
// do: `limit` items a page, 20 by default, with a cursor while more follow, so that a list read to its end ends; a later
// page comes late only with `lateMs`. With `failFirst` the home timeline's first page fails during the first second,
// and its retry succeeds late. What the shell reads (the instance, the reader's rights, the announcements, the
// voice rooms, the events) answers in the state the options give the server: its timelines, one person's, its closure,
// its maintenance, down, or newer than the app. The voice rooms are the fake's own, since a room lives only while
// people are in it and the seed opens none. A post is translated by the provider `translation` names, which answers
// or refuses as the options say. With `approvals`, posts of the fake's own approve replies, boosts and quotes by hand
// (ADR-0155): what waits shows to its author alone and counts nowhere, the reader's requests list and are answered,
// and a reply approved joins its thread.

import { drawingBlurhash, drawingURL } from "../drawing";
import {
  admin,
  announcements as scenarioAnnouncements,
  conversationReads,
  drafts,
  draftSchedule,
  emojis,
  eventStart,
  events as scenarioEvents,
  follows as scenarioFollows,
  interactions as scenarioInteractions,
  lists as scenarioLists,
  people as scenarioPeople,
  personOf as scenarioPersonOf,
  portrait,
  posts as scenarioPosts,
  restrictions,
  type Follow,
  type Image,
  type Interaction,
  type LinkCard,
  type Person,
  type Post,
} from "../scenario";
import { articleHtml, serverHtml } from "./html";

/**
 * The people of other servers, the fake's own, since the seed's server federates with none: each `username` is their
 * whole handle, `user@domain`, and `movedTo` the handle of the account one moved to. sora, on photos.example, follows
 * the reader, and sam boosts a post of sora's; sora's account on social.example moved to it, and keeps its posts.
 */
const remotePeople: readonly (Person & { movedTo?: string })[] = [
  {
    username: "sora@photos.example",
    email: "",
    password: "",
    displayName: "Sora そら",
    note: "Film photographer on another server. 別のサーバーでフィルム写真を撮っています。",
    fields: [],
    locale: "en",
    avatar: portrait([60, 140, 170]),
  },
  {
    username: "sora@social.example",
    email: "",
    password: "",
    displayName: "Sora そら",
    note: "This account moved. 引っ越しました。",
    fields: [],
    locale: "en",
    avatar: portrait([60, 140, 170]),
    movedTo: "sora@photos.example",
  },
  {
    // A bakery's account, so that one word ("bak") finds a person, the scenario's #baking and its posts at once.
    username: "kobo@bread.example",
    email: "",
    password: "",
    displayName: "Bakery Kobo",
    note: "A small bakery by the station. 駅前の小さなパン屋です。",
    fields: [],
    locale: "en",
    avatar: portrait([200, 150, 90]),
  },
  {
    username: "kiri@far.example",
    email: "",
    password: "",
    displayName: "Kiri 霧",
    note: "Writes about mountain weather. 山の天気について書いています。",
    fields: [],
    locale: "en",
    avatar: portrait([150, 150, 160]),
  },
];

/**
 * A person of another server this one has not met yet: no search finds them until a lookup with `resolve` fetched
 * their account (FR-305), by `@kiri@far.example` or their profile's address.
 */
const unmet = "kiri@far.example";

/**
 * A person of this server who deactivated their account, whom the server answers as not there (ADR-0089 §15): a 404,
 * and in no list.
 */
const deactivated = "nao";

/** The handle of the account a person moved to, if they moved. */
const movedOf = (username: string) => remotePeople.find((p) => p.username === username)?.movedTo;

const remoteFollows: readonly Follow[] = [{ follower: "sora@photos.example", followee: "yuki" }];

const remotePosts: readonly Post[] = [
  {
    key: "sora-moving",
    author: "sora@social.example",
    text: "Moving to photos.example, where the darkroom is. See you there.",
    visibility: "public",
    language: "en",
  },
  {
    key: "sora-film",
    author: "sora@photos.example",
    text: "Developed the first roll from the coast. The grain came out just right.",
    visibility: "public",
    language: "en",
  },
];

const remoteInteractions: readonly Interaction[] = [{ kind: "boost", by: "sam", post: "sora-film" }];

/** Everyone the fake answers about: the scenario's people, then those of other servers. */
const people: readonly Person[] = [...scenarioPeople, ...remotePeople];
const personOf = (username: string): Person =>
  remotePeople.find((p) => p.username === username) ?? scenarioPersonOf(username);
const follows: readonly Follow[] = [...scenarioFollows, ...remoteFollows];

/** The domain of a person's server: theirs after the `@` of their handle, else this server's. */
const domainOf = (username: string) => (username.includes("@") ? username.slice(username.indexOf("@") + 1) : undefined);

/** The host the fake answers as. */
export const fakeHost = "demo.test";
/** The origin the fake answers as. */
export const fakeOrigin = `https://${fakeHost}`;

/** The origin of a person's server. */
const originOf = (username: string) => `https://${domainOf(username) ?? fakeHost}`;

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
  /**
   * How posts are translated: not at all by default. `provider` enables one provider of the server, for the posts of
   * every visibility but followers-only and direct: one that answers (`answers`, each text behind the language it
   * goes into, "[ja] …", as the server's fake provider does), or one that refuses every request, over the monthly limit
   * (`limit`), as no provider may receive the post (`unavailable`), for a language it does not translate (`language`),
   * or failing (`failed`). `browser` lets readers use the browser's own translator.
   */
  translation?: {
    provider?: "answers" | "limit" | "unavailable" | "language" | "failed";
    browser?: boolean;
  };
  /** The keys of the posts the reader bookmarked besides the scenario's, the newest bookmark first. */
  bookmarks?: readonly string[];
  /** The most hashtags the reader may follow, the server's 100 by default; 0 refuses every follow. */
  followedTagLimit?: number;
  /**
   * The articles (ADR-0160): who may write new ones, everyone by default or only the roles with `create_articles`
   * (`authoring: "permission"`, which no person of the scenario has); a preview of a body that fails (`preview:
   * "fails"`); a publication or an edit the server's review refuses (`publish: "refused"`), or refuses for the value
   * of its summary, which the problem names (`publish: "summary"`).
   */
  articles?: { authoring?: "everyone" | "permission"; preview?: "fails"; publish?: "refused" | "summary" };
  /**
   * Posts whose authors approve replies, boosts and quotes by hand (ADR-0155), after the scenario's (`approvalPosts`):
   * the reader's (yuki's), which marco replies to and boosts and mio quotes, each waiting for the reader's approval, and
   * layla's, to which the reader's reply waits, another was not approved, and the reader's boost waits. The requests
   * are answered as the server answers them.
   */
  approvals?: boolean;
  /** The lists of an account's follows and followers fail (`fails`, a 500) or never answer (`waits`). */
  followsLists?: "fails" | "waits";
  /**
   * The search (ADR-0149): `posts` gives the server a search engine, which the instance says and which finds the
   * public and local posts that hold every word; none by default, as a server without one.
   */
  search?: { posts?: boolean };
  /**
   * The most lists the reader may have (the server's 50 by default) and members a list may (1,000): a smaller one shows
   * the server's refusal at its limit (the reader has 3 lists, the first holds 2 accounts).
   */
  listLimits?: { lists?: number; members?: number };
  /**
   * The follow requests to the reader: those of the scenario (sam's to ren) and, after them, of the people named in
   * `from`, the last the newest; their list fails (`fails`, a 500) or never answers (`waits`).
   */
  followRequests?: { from?: readonly string[]; answer?: "fails" | "waits" };
  /** The reader's private notes on people at the start, by username. */
  notes?: Readonly<Record<string, string>>;
  /** The people whose new posts notify the reader at the start. */
  subscriptions?: readonly string[];
  /** The domains the reader mutes at the start. */
  domainMutes?: readonly string[];
};

/** Whom an interaction policy admits (the API's InteractionAudience). */
type PolicyAudience = "nobody" | "mentioned" | "followers" | "everyone";

/** Who may reply to, boost and quote a post: at once, and by its author's approval (the API's InteractionRule). */
type PolicyRule = { automatic: PolicyAudience; manual?: PolicyAudience };

/** A post of `approvals`: its policy, and whether the author of the post it replies to declined it. */
type ApprovalPost = Post & {
  policy?: Partial<Record<"reply" | "announce" | "quote", PolicyRule>>;
  declined?: boolean;
};

/** The posts of `approvals`, after the scenario's, oldest first. */
const approvalPosts: readonly ApprovalPost[] = [
  {
    key: "yuki-asks",
    author: "yuki",
    text: "週末の写真展に出す一枚を、まだ選べずにいます。",
    visibility: "public",
    language: "ja",
    policy: {
      reply: { automatic: "followers", manual: "everyone" },
      announce: { automatic: "followers", manual: "everyone" },
      quote: { automatic: "nobody", manual: "everyone" },
    },
  },
  {
    key: "layla-asks",
    author: "layla",
    text: "Translating a poem this week. Please ask before you share it.",
    visibility: "public",
    language: "en",
    policy: {
      reply: { automatic: "mentioned", manual: "everyone" },
      announce: { automatic: "mentioned", manual: "everyone" },
      quote: { automatic: "nobody" },
    },
  },
  {
    key: "marco-asks-reply",
    author: "marco",
    replyTo: "yuki-asks",
    text: "The ridge at dawn would be my pick.",
    visibility: "public",
    language: "en",
  },
  {
    key: "mio-asks-quote",
    author: "mio",
    quote: "yuki-asks",
    text: "写真展、楽しみ。",
    visibility: "public",
    language: "ja",
  },
  {
    key: "yuki-waits",
    author: "yuki",
    replyTo: "layla-asks",
    text: "どの詩ですか？ 読んでみたいです。",
    visibility: "public",
    language: "ja",
  },
  {
    key: "yuki-declined",
    author: "yuki",
    replyTo: "layla-asks",
    text: "翻訳ができたら教えてください。",
    visibility: "public",
    language: "ja",
    declined: true,
  },
];

/** The boosts of `approvals`, each waiting for the approval of the post's author. */
const approvalInteractions: readonly Interaction[] = [
  { kind: "boost", by: "marco", post: "yuki-asks" },
  { kind: "boost", by: "yuki", post: "layla-asks" },
];

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
  /** The app's address of the reader's list at `index`, oldest first. */
  listPath: (index: number) => string;
  /** The ID of the scenario's post of `key`, or of the reader's draft of that key. */
  idOf: (key: string) => string;
  /** The app's address of the reader's conversation at `index` of their direct messages, newest first. */
  conversationPath: (index: number) => string;
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

/** The custom emoji as the picker lists them: those without a category first, then by category and shortcode. */
const pickerEmojis = emojis
  .map((e, i) => ({
    ...customEmojis[i],
    aliases: e.aliases ?? [],
    ...(e.category === undefined ? {} : { category: e.category }),
  }))
  .toSorted(
    (a, b) =>
      Number(a.category !== undefined) - Number(b.category !== undefined) ||
      (a.category ?? "").localeCompare(b.category ?? "") ||
      (a.shortcode ?? "").localeCompare(b.shortcode ?? ""),
  );

/** A reaction's emoji as the scenario writes it (`:shortcode:` for a custom one) from what a request names. */
const scenarioEmoji = (name: string) => {
  const bare = name.replace(/^:|:$/g, "");
  return customEmojis.some((e) => e.shortcode === bare) ? `:${bare}:` : name;
};

/** Creates a fake of the client API as `viewer` reads it. */
export function createFakeApi(o: FakeOptions = {}): FakeApi {
  const viewer = personOf(o.viewer ?? "yuki").username;
  const posts: readonly ApprovalPost[] = [
    ...scenarioPosts,
    ...remotePosts,
    ...(o.approvals === true ? approvalPosts : []),
  ];
  const interactions = [
    ...scenarioInteractions,
    ...remoteInteractions,
    ...(o.approvals === true ? approvalInteractions : []),
  ];
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
  /** The people of other servers this one has met by a lookup with `resolve` (`unmet` until then). */
  const met = new Set<string>();
  for (const f of follows) {
    const edge = `${f.follower}>${f.followee}`;
    if (personOf(f.followee).locked === true && f.accepted !== true) requested.add(edge);
    else following.add(edge);
  }
  for (const from of o.followRequests?.from ?? []) requested.add(`${personOf(from).username}>${viewer}`);
  const blocking = new Set(restrictions.filter((r) => r.kind === "block").map((r) => `${r.by}>${r.target}`));
  const muting = new Set(restrictions.filter((r) => r.kind === "mute").map((r) => `${r.by}>${r.target}`));
  const follows_ = (a: string, b: string) => following.has(`${a}>${b}`);
  /**
   * Who may read a person's lists of follows and followers (FR-311), the fake's own, since the seed sets none: mio
   * shows them to mio's followers only, kai to nobody, everyone else to everyone.
   */
  /**
   * The follows a person's counts take in beyond the scenario's people (with people of servers the seed's federates
   * with none), the fake's own: an account whose lists its owner keeps (FR-311) still shows how many they hold.
   */
  const beyond: Readonly<Record<string, { followers: number; following: number }>> = {
    mio: { followers: 48, following: 35 },
    kai: { followers: 127, following: 64 },
    "sora@photos.example": { followers: 1284, following: 312 },
    // Its followers moved with it; it still follows those it did.
    "sora@social.example": { followers: 3, following: 298 },
  };
  const followsVisibility: Readonly<Record<string, "public" | "followers" | "private">> = {
    mio: "followers",
    kai: "private",
  };
  const hidden = (author: string) => blocking.has(`${viewer}>${author}`) || muting.has(`${viewer}>${author}`);

  // Every account the server answers is a whole one, as the client API's Account is: a post's author too, which the
  // client shows a profile from at once.
  const accountOf = (username: string): Json => fullAccount(personOf(username));

  const fullAccount = (p: Person): Json => {
    const header = p.header === undefined ? undefined : drawingURL(p.header);
    const avatar = drawingURL(p.avatar);
    const remote = domainOf(p.username);
    const username = remote === undefined ? p.username : p.username.slice(0, p.username.indexOf("@"));
    return {
      id: idOf(p.username),
      username,
      acct: p.username,
      domain: remote ?? fakeHost,
      display_name: p.displayName,
      emojis: [],
      avatar: { small: { url: avatar }, medium: { url: avatar } },
      uri: `${originOf(p.username)}/users/${username}`,
      url: `${originOf(p.username)}/@${username}`,
      note: serverHtml(p.note, remote ?? fakeHost),
      locked: p.locked ?? false,
      bot: false,
      created_at: new Date(started - 30 * 24 * 60 * minute).toISOString(),
      fields: p.fields.map(([name, value]) => ({ name, value: serverHtml(value, fakeHost) })),
      badges: [],
      ...(header === undefined ? {} : { header: { small: { url: header }, medium: { url: header } } }),
      // The account it moved to, without a move of its own: moves are not followed in chains.
      ...(movedOf(p.username) === undefined ? {} : { moved: fullAccount(personOf(movedOf(p.username) ?? "")) }),
      followers_count:
        people.filter((x) => follows_(x.username, p.username)).length + (beyond[p.username]?.followers ?? 0),
      following_count:
        people.filter((x) => follows_(p.username, x.username)).length + (beyond[p.username]?.following ?? 0),
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
      notify: subscribed.has(other),
      show_boosts: true,
      note: notes.get(other) ?? "",
    };
  };

  /** The reader's private notes on people, by username, and the people whose new posts notify them. */
  const notes = new Map<string, string>(Object.entries(o.notes ?? {}));
  const subscribed = new Set<string>(o.subscriptions ?? []);

  /** The domains the reader mutes, in the order they muted them, and whether a person's server is one of them. */
  const domainMutes: { domain: string; created_at: string }[] = (o.domainMutes ?? []).map((domain) => ({
    domain,
    created_at: new Date(started).toISOString(),
  }));
  const mutedServer = (username: string) => {
    const domain = domainOf(username);
    return domain !== undefined && domainMutes.some((m) => domain === m.domain || domain.endsWith(`.${m.domain}`));
  };

  // ---- Posts -----------------------------------------------------------------------------------------------------

  const postStep = (key: string) => follows.length + posts.findIndex((p) => p.key === key);
  const statusId = new Map(posts.map((p) => [p.key, idAt(timeOf(postStep(p.key)), postStep(p.key))]));
  const keyOf = new Map([...statusId].map(([k, id]) => [id, k]));
  const postOf = (key: string): ApprovalPost => {
    const p = posts.find((x) => x.key === key);
    if (p === undefined) throw new Error(`fake: no post ${key}`);
    return p;
  };

  // ---- Approvals (ADR-0155) -----------------------------------------------------------------------------------------

  type Kind = "reply" | "announce" | "quote";
  /** Whether `audience` of `p` admits `who`: its author always, the mentioned in any but nobody, as the server does. */
  const admits = (audience: PolicyAudience, p: Post, who: string) =>
    who === p.author ||
    audience === "everyone" ||
    (audience === "followers" && follows_(who, p.author)) ||
    (audience !== "nobody" && mentioned(p.text).includes(who));
  /** Whether `who` may do `kind` with `p`: at once, once its author approves, or not; none for a post of no policy. */
  const permissionOf = (p: ApprovalPost, kind: Kind, who: string): "allowed" | "pending" | "denied" | undefined => {
    if (p.policy === undefined) return undefined;
    const rule = p.policy[kind] ?? { automatic: "everyone" };
    if (admits(rule.automatic, p, who)) return "allowed";
    return rule.manual !== undefined && admits(rule.manual, p, who) ? "pending" : "denied";
  };
  /**
   * A request of `by` to reply to (`via` the reply), boost or quote (`via` the quoting post) the post of `target`, at
   * the step of what asks; one the scenario says the author declined is so from the start.
   */
  type AskedApproval = {
    key: string;
    kind: Kind;
    by: string;
    target: string;
    via?: string;
    step: number;
    declined: boolean;
  };
  const requests: AskedApproval[] = [];
  posts.forEach((p, n) => {
    const step = follows.length + n;
    for (const [kind, target] of [
      ["reply", p.replyTo],
      ["quote", p.quote],
    ] as const)
      if (target !== undefined && permissionOf(postOf(target), kind, p.author) === "pending")
        requests.push({
          key: `${kind}:${p.key}`,
          kind,
          by: p.author,
          target,
          via: p.key,
          step,
          declined: p.declined === true,
        });
  });
  interactions.forEach((i, n) => {
    if (i.kind === "boost" && permissionOf(postOf(i.post), "announce", i.by) === "pending")
      requests.push({
        key: `announce:${i.by}:${i.post}`,
        kind: "announce",
        by: i.by,
        target: i.post,
        step: follows.length + posts.length + n,
        declined: false,
      });
  });
  /** The answers the reader gave to the requests of their posts, by the request's key. */
  const answers = new Map<string, "accepted" | "rejected">();
  const stateOf = (r: AskedApproval) => answers.get(r.key) ?? (r.declined ? "rejected" : "pending");
  const requestId = (r: AskedApproval) => idAt(timeOf(r.step), 85_000 + r.step);
  /** The approval the reply of `key` waits for or was refused, while it is not given; none for a reply that needs none. */
  const replyApproval = (key: string) => {
    const r = requests.find((x) => x.kind === "reply" && x.via === key);
    const state = r === undefined ? "accepted" : stateOf(r);
    return state === "accepted" ? undefined : state;
  };
  /** Whether the boost `i` waits for the approval of its post's author, or was refused it. */
  const boostWaits = (i: { by: string; post: string }) => {
    const r = requests.find((x) => x.kind === "announce" && x.by === i.by && x.target === i.post);
    return r !== undefined && stateOf(r) !== "accepted";
  };
  /** The quote by the post of `key` while its author has not approved it: pending or rejected. */
  const quoteApproval = (key: string) => {
    const r = requests.find((x) => x.kind === "quote" && x.via === key);
    const state = r === undefined ? "accepted" : stateOf(r);
    return state === "accepted" ? undefined : state;
  };

  /** The interactions of `kind` with the post of `key`; a boost only once it needs no approval or has it. */
  const on = (key: string, kind: string) =>
    interactions.filter((i) => i.post === key && i.kind === kind && !(i.kind === "boost" && boostWaits(i)));

  // The reader's favourite and reaction as they set them, by the key of the post, over the scenario's: one or the
  // other, a reaction's emoji as the scenario writes it, or none (null).
  const ownFavourite = new Map<string, boolean>();
  const ownReaction = new Map<string, string | null>();
  const mineSet = (key: string) => ownFavourite.has(key) || ownReaction.has(key);
  /** The keys of the posts the reader bookmarked, the newest bookmark first: the options', then the scenario's. */
  const bookmarks: string[] = [
    ...new Set([
      ...(o.bookmarks ?? []),
      ...interactions
        .filter((i) => i.kind === "bookmark" && i.by === viewer)
        .map((i) => i.post)
        .toReversed(),
    ]),
  ];
  /**
   * The reader's lists of the people they follow, oldest first, with ids of the fake's own; the reader makes, renames
   * and deletes them and changes their members (ADR-0013 §11, §14).
   */
  const lists: { id: string; name: string; members: string[]; created_at: string }[] = scenarioLists
    .filter((l) => l.owner === viewer)
    .map((l, n) => ({
      id: idAt(0, 60_000 + n),
      name: l.name,
      members: [...l.members],
      created_at: new Date(started - (30 - n) * 24 * 60 * minute).toISOString(),
    }));
  /** How many lists the reader may have, and members a list. */
  const listLimit = o.listLimits?.lists ?? 50;
  const memberLimit = o.listLimits?.members ?? 1000;
  /** How many lists the reader made here, for the ids of new ones. */
  let madeLists = 0;
  /** Whether `name` is a list's name: 1 to 50 characters (grapheme clusters) without control characters. */
  const listName = (name: string) => {
    const length = [...new Intl.Segmenter().segment(name)].length;
    return length > 0 && length <= 50 && !/\p{Cc}/u.test(name);
  };
  /** The server's refusal of a list's name that is not one: blank, too long, or with control characters. */
  const listNameRefused = () =>
    json(
      {
        type: "/api/v1/problems/validation",
        title: "The list could not be saved",
        status: 422,
        field: "name",
        detail: "Write a name of 1 to 50 characters on one line.",
      },
      422,
    );
  const listBody = (l: (typeof lists)[number]): Json => ({
    id: l.id,
    name: l.name,
    members_count: l.members.length,
    created_at: l.created_at,
  });
  /** The hashtags the reader follows, by their names in lowercase. */
  const followedTags = new Set<string>();
  /** The hashtags of a text, in lowercase, as the server reads them. */
  const hashtagsOf = (text: string) =>
    [...text.matchAll(/#([\p{L}\p{M}\p{N}_]+)/gu)].map((x) => (x[1] ?? "").toLowerCase());
  /** Who favourited the post of `key`, in order. */
  const favouritersOf = (key: string) => [
    ...on(key, "favourite").flatMap((i) => (i.by === viewer && mineSet(key) ? [] : [i.by])),
    ...(ownFavourite.get(key) === true ? [viewer] : []),
  ];
  /** The reactions to the post of `key`, oldest first: who reacted, with which emoji, at which step. */
  const reactionsOf = (key: string) => {
    const list = interactions.flatMap((i, n) =>
      i.kind === "reaction" && i.post === key && !(i.by === viewer && mineSet(key))
        ? [{ by: i.by, emoji: i.emoji, step: follows.length + posts.length + n }]
        : [],
    );
    const mine = ownReaction.get(key);
    if (mine !== undefined && mine !== null) list.push({ by: viewer, emoji: mine, step: steps });
    return list;
  };
  /** A reaction's emoji as the API gives it (ReactionEmoji): a custom one with its image. */
  const reactionEmojiOf = (emoji: string) => {
    const custom = customEmojis.find((e) => `:${e.shortcode}:` === emoji);
    return custom === undefined
      ? { name: emoji }
      : { name: custom.shortcode, shortcode: custom.shortcode, emoji: custom };
  };

  /** The reader's votes, by the key of the post, as the scenario has them and as the reader votes. */
  const ownVotes = new Map<string, readonly number[]>();
  for (const i of interactions) if (i.kind === "vote" && i.by === viewer) ownVotes.set(i.post, i.choices);

  /** The poll of `p` as the reader sees it, open for the days the scenario gives it from the start of the fake. */
  function pollOf(p: Post): Json | undefined {
    if (p.poll === undefined) return undefined;
    const own = ownVotes.get(p.key);
    const votes = [
      ...interactions.flatMap((i) => (i.kind === "vote" && i.post === p.key && i.by !== viewer ? [i.choices] : [])),
      ...(own === undefined ? [] : [own]),
    ];
    return {
      id: idAt(timeOf(postStep(p.key)), 60_000 + postStep(p.key)),
      expires_at: new Date(started + p.poll.days * 24 * 60 * minute).toISOString(),
      expired: false,
      multiple: p.poll.multiple ?? false,
      votes_count: votes.reduce((n, c) => n + c.length, 0),
      voters_count: votes.length,
      options: p.poll.options.map((title, i) => ({ title, votes_count: votes.filter((c) => c.includes(i)).length })),
      voted: own !== undefined,
      own_votes: own ?? [],
    };
  }

  // The reader's own posts they deleted, by key, and the versions of each post: those of the scenario (its `earlier`,
  // then its text), then the reader's edits.
  const deleted = new Set<string>();
  type Version = { text: string; spoiler: string; at: number; media?: readonly string[] };
  const edits = new Map<string, Version[]>();
  /** The versions of `p`, oldest first: what it was posted with first, the last what it says now. */
  function versionsOf(p: Post): readonly Version[] {
    const posted = timeOf(postStep(p.key));
    const scenario = [...(p.earlier ?? []), p.text].map((text, i) => ({
      text,
      spoiler: p.spoiler ?? "",
      at: posted + i * 2 * minute,
    }));
    return [...scenario, ...(edits.get(p.key) ?? [])];
  }

  /**
   * A post's attachments as a version of it holds them (all of them unless it names which), as the server gives them,
   * its images then the rest: a picture's drawing served whole and as its preview, with its blurhash; a video's or a
   * GIF's poster as its preview, with no file the fake could play; a sound with its length and no file; and a file of
   * another kind as a link to where it would be.
   */
  const mediaOf = (p: Post, id: string, version: Version): Json[] =>
    [
      ...(p.images ?? []).map((image) => {
        const url = drawingURL(image.drawing);
        const { width, height } = image.drawing;
        return {
          type: "image",
          state: "ready",
          description: image.description,
          url,
          preview_url: url,
          blurhash: drawingBlurhash(image.drawing),
          meta: { width, height, focus: { x: 0, y: 0 } },
        };
      }),
      ...(p.attachments ?? []).map((a): Json => {
        const described = a.description === "" ? {} : { description: a.description };
        if (a.kind === "audio")
          return { type: "audio", state: "ready", ...described, meta: { duration: a.seconds, focus: { x: 0, y: 0 } } };
        if (a.kind === "unknown")
          return {
            type: "unknown",
            state: "ready",
            ...described,
            url: `${fakeOrigin}/media/${a.name}`,
            meta: { focus: { x: 0, y: 0 } },
          };
        const { width, height } = a.poster;
        return {
          type: a.kind,
          state: "ready",
          ...described,
          preview_url: drawingURL(a.poster),
          blurhash: drawingBlurhash(a.poster),
          meta: { width, height, ...(a.seconds === undefined ? {} : { duration: a.seconds }), focus: { x: 0, y: 0 } },
        };
      }),
    ]
      .map((m, i) => ({ id: `${id}-${i}`, ...m }))
      .filter((m) => version.media === undefined || version.media.includes(m.id));

  /** The card of a post's first link, as the server makes it from the page `text` links to. */
  const cardOf = (card: LinkCard, text: string): Json => ({
    url: /https?:\/\/\S+/.exec(text)?.[0] ?? fakeOrigin,
    title: card.title,
    description: card.description,
    site_name: card.site,
    ...(card.language === undefined ? {} : { language: card.language }),
    ...(card.image === undefined
      ? {}
      : {
          image: {
            url: drawingURL(card.image.drawing),
            width: card.image.drawing.width,
            height: card.image.drawing.height,
            blurhash: drawingBlurhash(card.image.drawing),
            ...(card.image.description === undefined ? {} : { description: card.image.description }),
          },
        }),
    ...(card.author === undefined ? {} : { author: accountOf(card.author) }),
  });

  // A reply waiting for its approval, or refused it, shows to its author alone.
  const visible = (p: Post) =>
    !deleted.has(p.key) &&
    (p.author === viewer || replyApproval(p.key) === undefined) &&
    (p.author === viewer ||
      p.visibility === "public" ||
      p.visibility === "unlisted" ||
      p.visibility === "local" ||
      (p.visibility === "private" && follows_(viewer, p.author)) ||
      (p.visibility === "direct" && mentioned(p.text).includes(viewer)));

  /** The title, summary and hashtags of the scenario's articles as the reader edited them, by the key of each. */
  const articleEdits = new Map<string, { title: string; summary: string; tags: readonly string[] }>();

  /** An article's title, summary and cover, as the API's StatusArticle. */
  const articleOf = (p: Post, id: string): Json | undefined => {
    const a = p.article;
    if (a === undefined) return undefined;
    const cover = a.cover;
    const edited = articleEdits.get(p.key);
    return {
      title: edited?.title ?? a.title,
      summary: edited?.summary ?? a.summary,
      ...(cover === undefined
        ? {}
        : {
            cover: {
              id: `${id}-cover`,
              type: "image",
              state: "ready",
              description: cover.description,
              url: drawingURL(cover.drawing),
              preview_url: drawingURL(cover.drawing),
              blurhash: drawingBlurhash(cover.drawing),
              meta: { width: cover.drawing.width, height: cover.drawing.height, focus: { x: 0, y: 0 } },
            },
          }),
    };
  };

  /**
   * A post as the API's Status; an article with its body only when `full`, as a status read alone gives it, and with
   * none in a list (ADR-0160 §8).
   */
  function statusOf(p: ApprovalPost, depth = 0, full = false): Json {
    const id = statusId.get(p.key) ?? "";
    const approval = p.author === viewer ? replyApproval(p.key) : undefined;
    const quoteState = quoteApproval(p.key) ?? "accepted";
    const article = articleOf(p, id);
    const versions = versionsOf(p);
    const now = versions.at(-1) ?? { text: p.text, spoiler: p.spoiler ?? "", at: 0 };
    const parent = p.replyTo === undefined ? undefined : postOf(p.replyTo);
    const reactions = new Map<string, { count: number; me: boolean }>();
    for (const r of reactionsOf(p.key)) {
      const now = reactions.get(r.emoji) ?? { count: 0, me: false };
      reactions.set(r.emoji, { count: now.count + 1, me: now.me || r.by === viewer });
    }
    const favouriters = favouritersOf(p.key);
    return {
      id,
      uri: `${originOf(p.author)}/users/${p.author.split("@")[0] ?? ""}/statuses/${id}`,
      url: `${originOf(p.author)}/@${p.author.split("@")[0] ?? ""}/${id}`,
      account: accountOf(p.author),
      content: article === undefined ? serverHtml(now.text, fakeHost) : full ? articleHtml(now.text, fakeHost) : "",
      spoiler_text: now.spoiler,
      sensitive: now.spoiler !== "" || p.sensitive === true,
      visibility: p.visibility,
      language: p.language,
      created_at: new Date(timeOf(postStep(p.key))).toISOString(),
      ...(versions.length > 1 ? { edited_at: new Date(now.at).toISOString() } : {}),
      ...(parent === undefined
        ? {}
        : { in_reply_to_id: statusId.get(parent.key), in_reply_to_account_id: idOf(parent.author) }),
      media_attachments: mediaOf(p, id, now),
      ...(p.card === undefined ? {} : { card: cardOf(p.card, now.text) }),
      mentions: mentioned(now.text).map((u) => ({ id: idOf(u), username: u, acct: u })),
      emojis: customEmojis,
      replies_count: posts.filter((x) => x.replyTo === p.key && replyApproval(x.key) === undefined).length,
      boosts_count: on(p.key, "boost").length,
      favourites_count: favouriters.length,
      reactions: [...reactions]
        .toSorted(([, a], [, b]) => b.count - a.count)
        .map(([emoji, r]) => ({ ...reactionEmojiOf(emoji), count: r.count, me: r.me })),
      favourited: favouriters.includes(viewer),
      boosted: on(p.key, "boost").some((i) => i.by === viewer),
      bookmarked: bookmarks.includes(p.key),
      pinned: p.pinned === true && p.author === viewer,
      ...(p.poll === undefined ? {} : { poll: pollOf(p) }),
      ...(article === undefined ? {} : { article }),
      ...(p.quote === undefined || depth > 0
        ? {}
        : {
            quote: {
              state: quoteState,
              ...(quoteState === "accepted" ? { quoted_status: statusOf(postOf(p.quote), depth + 1) } : {}),
            },
          }),
      ...(approval === undefined ? {} : { approval }),
      ...(interactions.some((i) => i.kind === "boost" && i.by === viewer && i.post === p.key && boostWaits(i))
        ? { boost_approval: "pending" }
        : {}),
      ...(p.policy === undefined
        ? {}
        : {
            permissions: {
              reply: permissionOf(p, "reply", viewer),
              announce: permissionOf(p, "announce", viewer),
              quote: permissionOf(p, "quote", viewer),
            },
            ...(p.author === viewer
              ? {
                  interaction_policy: {
                    reply: p.policy.reply ?? { automatic: "everyone" },
                    announce: p.policy.announce ?? { automatic: "everyone" },
                    quote: p.policy.quote ?? { automatic: "everyone" },
                  },
                }
              : {}),
          }),
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
    if (i.kind !== "boost" || !inHome(i.by) || i.by === viewer || boostWaits(i)) return;
    const p = postOf(i.post);
    if (hidden(p.author) || !visible(p) || p.visibility === "private" || p.visibility === "direct") return;
    const step = follows.length + posts.length + n;
    homeEntries.push({ at: step, status: boostOf(p, i.by, step) });
  });
  const home: Json[] = homeEntries.toSorted((a, b) => b.at - a.at).map((e) => e.status);
  /** The handle of a status's author, or of a notification's sender; empty for a notification from no one. */
  const authorOf = (s: Json) => ((s["account"] as Json | undefined)?.["acct"] as string | undefined) ?? "";
  /**
   * Whether a status of home stays there as the reader's blocks and mutes are now: none of whom they block or mute, and
   * of the servers they mute (ADR-0013 §30) a boost of a post leaves, and so does a post, unless the reader follows its
   * author.
   */
  const inHomeNow = (s: Json) => {
    const boosted = s["boost"] as Json | undefined;
    if (hidden(authorOf(s)) || (boosted !== undefined && hidden(authorOf(boosted)))) return false;
    if (boosted !== undefined) return !mutedServer(authorOf(boosted));
    return !mutedServer(authorOf(s)) || follows_(viewer, authorOf(s));
  };

  /**
   * A status of a list as it is now: a post of the scenario (or a boost of one) as edited since, none once deleted;
   * another (a post the stream brought) as it was.
   */
  function current(s: Json): Json[] {
    const key = keyOf.get(s["id"] as string);
    if (key !== undefined) return deleted.has(key) ? [] : [statusOf(postOf(key))];
    const boosted = s["boost"] as Json | undefined;
    const boostedKey = boosted === undefined ? undefined : keyOf.get(boosted["id"] as string);
    if (boostedKey === undefined) return [s];
    return deleted.has(boostedKey) ? [] : [{ ...s, boost: statusOf(postOf(boostedKey)) }];
  }

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

  /**
   * The answer to translating `p` into `target`, by the provider of `translation`: the post with each text behind the
   * language it goes into, or the refusal the provider stands for. A post whose visibility the provider may not receive
   * is refused as the server refuses it, whatever the provider.
   */
  function translationOf(p: Post, target: string): Response {
    const provider = o.translation?.provider;
    const refuse = (status: number, type: string, title: string, headers: Record<string, string> = {}) =>
      new Response(JSON.stringify({ type: `/api/v1/problems/${type}`, title, status }), {
        status,
        headers: { "Content-Type": "application/problem+json", ...headers },
      });
    if (
      provider === undefined ||
      provider === "unavailable" ||
      !translationProvider.visibilities.includes(p.visibility)
    )
      return refuse(422, "translation-unavailable", "No translation provider may receive this post");
    if (provider === "limit") {
      const now = new Date();
      const next = Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 1, 1);
      return refuse(429, "translation-limit", "The monthly translation limit is reached", {
        "Retry-After": String(Math.ceil((next - now.getTime()) / 1000)),
      });
    }
    if (provider === "language")
      return refuse(
        422,
        "translation-unsupported-language",
        "The translation providers do not translate this language",
      );
    if (provider === "failed") return refuse(503, "translation-failed", "The translation provider did not answer");
    const into = (text: string) => `[${target}] ${text}`;
    return json({
      language: target,
      source_language: p.language,
      content: serverHtml(into(p.text), fakeHost),
      spoiler_text: p.spoiler === undefined ? "" : into(p.spoiler),
      media_attachments: (p.images ?? []).map((image, i) => ({
        id: `${statusId.get(p.key) ?? ""}-${i}`,
        description: into(image.description),
      })),
      poll_options: (p.poll?.options ?? []).map(into),
      provider: { id: translationProvider.id, name: translationProvider.name },
    });
  }

  /** A request as the API's InteractionRequest: what the reply or the quoting post says, none for a boost. */
  const requestBody = (r: AskedApproval): Json => ({
    id: requestId(r),
    kind: r.kind,
    state: stateOf(r),
    created_at: new Date(timeOf(r.step)).toISOString(),
    account: accountOf(r.by),
    status: statusOf(postOf(r.target)),
    ...(r.via === undefined ? {} : { interacting_status: statusOf(postOf(r.via)) }),
  });

  // ---- Direct messages -------------------------------------------------------------------------------------------

  // The reader's conversations: their direct posts by thread, each by its root's key, newest first. One is unread while
  // a post by another is newer than where the reader read it to (the scenario's reads, then the reader's), as the
  // server says, whoever wrote the latest; one hidden stays out until a newer post.
  const rootOf = (p: Post): Post => (p.replyTo === undefined ? p : rootOf(postOf(p.replyTo)));
  const conversationIdOf = (root: Post) => idAt(0, 95_000 + posts.indexOf(root));
  const readTo = new Map<string, string>(
    conversationReads
      .filter((r) => r.by === viewer)
      .map((r) => [conversationIdOf(rootOf(postOf(r.post))), statusId.get(r.post) ?? ""]),
  );
  const hiddenTo = new Map<string, string>();
  type HeldConversation = { at: string; hidden: boolean; body: Json };
  const conversationsNow = (): HeldConversation[] => {
    const threads = new Map<Post, Post[]>();
    for (const p of posts)
      if (p.visibility === "direct" && visible(p) && !hidden(p.author))
        threads.set(rootOf(p), [...(threads.get(rootOf(p)) ?? []), p]);
    return [...threads]
      .flatMap(([root, thread]): HeldConversation[] => {
        const last = thread.at(-1);
        if (last === undefined) return [];
        const id = conversationIdOf(root);
        const lastId = statusId.get(last.key) ?? "";
        const others = [...new Set(thread.flatMap((p) => [p.author, ...mentioned(p.text)]))].filter(
          (u) => u !== viewer,
        );
        return [
          {
            at: lastId,
            hidden: hiddenTo.get(id) === lastId,
            body: {
              id,
              accounts: others.map(accountOf),
              last_status: statusOf(last),
              unread: thread.some((p) => p.author !== viewer && (readTo.get(id) ?? "") < (statusId.get(p.key) ?? "")),
            },
          },
        ];
      })
      .toSorted((a, b) => (b.at > a.at ? 1 : -1));
  };
  const conversationOf = (id: string) => conversationsNow().find((c) => c.body["id"] === id);

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
  // A reply or a quote that waits for an approval tells nothing but its request, and a boost too.
  posts.forEach((p, n) => {
    const step = follows.length + n;
    if (mentioned(p.text).includes(viewer) && replyApproval(p.key) === undefined)
      notify(step, "mention", p.author, { status: statusOf(p) });
    if (p.quote !== undefined && postOf(p.quote).author === viewer && quoteApproval(p.key) === undefined)
      notify(step, "quote", p.author, { status: statusOf(p) });
  });
  const requestType = { reply: "reply_request", announce: "boost_request", quote: "quote_request" } as const;
  for (const r of requests)
    if (postOf(r.target).author === viewer && stateOf(r) === "pending")
      notify(r.step, requestType[r.kind], r.by, { status: statusOf(postOf(r.target)) });
  interactions.forEach((i, n) => {
    const p = postOf(i.post);
    if (p.author !== viewer || i.kind === "bookmark" || i.kind === "vote") return;
    if (i.kind === "boost" && boostWaits(i)) return;
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
  // ---- Drafts and scheduled posts -----------------------------------------------------------------------------------

  // The reader's drafts, newest first, each saved a few hours before the next: the scenario's, then those the reader
  // saves. One whose schedule `fails` failed yesterday, for the post it replied to that is gone, and the reader was
  // notified; one with a `schedule` waits for its time.
  const draftLimits = {
    max_drafts: 100,
    max_scheduled_posts: 300,
    max_scheduled_posts_per_day: 25,
    scheduled_post_min_lead_minutes: 5,
    scheduled_post_max_lead_days: 366,
  };
  /** The limits of an article's body, the server's defaults (ADR-0160 §5). */
  const articleLimits = { max_bytes: 100_000, max_images: 20 };
  const gonePost = idAt(started - 48 * 60 * minute, 80_999);
  const ownDrafts: Json[] = drafts
    .filter((d) => d.author === viewer)
    .map((d, i, all) => {
      const id = idAt(started - (all.length - i) * 5 * 60 * minute, 80_000 + i);
      const saved = new Date(started - (all.length - i) * 5 * 60 * minute).toISOString();
      const replyTo = d.fails !== undefined ? gonePost : d.replyTo === undefined ? undefined : statusId.get(d.replyTo);
      const failedAt = new Date(started - 40 * minute).toISOString();
      return {
        id,
        created_at: saved,
        updated_at: saved,
        text: d.text,
        spoiler_text: d.spoiler ?? "",
        sensitive: false,
        language: d.language,
        visibility: "public",
        ...(replyTo === undefined ? {} : { in_reply_to_id: replyTo }),
        media: (d.images ?? []).map((image, n) => ({ id: `${id}-${String(n)}`, description: image.description })),
        ...(d.article === undefined
          ? {}
          : { article: { title: d.article.title, summary: d.article.summary, tags: [...d.article.tags], images: [] } }),
        ...(d.poll === undefined
          ? {}
          : {
              poll: {
                options: [...d.poll.options],
                expires_in: d.poll.days * 86_400,
                multiple: d.poll.multiple ?? false,
              },
            }),
        ...(d.schedule === undefined ? {} : { scheduled_at: draftSchedule(d.schedule, started).toISOString() }),
        ...(d.fails === undefined
          ? {}
          : {
              last_schedule: {
                publish_at: failedAt,
                outcome: "failed",
                reason: "reply_unavailable",
                occurred_at: failedAt,
              },
            }),
      };
    })
    .toReversed();
  /** The images of the scenario's drafts, by the ID of each as the drafts name it. */
  const draftImages = new Map<string, Image>(
    drafts
      .filter((d) => d.author === viewer)
      .flatMap((d, i, all) => {
        const id = idAt(started - (all.length - i) * 5 * 60 * minute, 80_000 + i);
        return (d.images ?? []).map((image, n) => [`${id}-${String(n)}`, image] as const);
      }),
  );
  let draftsMade = 0;
  const draftOf = (id: string) => ownDrafts.find((d) => d["id"] === id);
  const etagOf = (d: Json) => `"${String(Date.parse(d["updated_at"] as string))}"`;
  const draftAnswer = (d: Json, status = 200) =>
    new Response(JSON.stringify(d), { status, headers: { "Content-Type": "application/json", ETag: etagOf(d) } });
  /** The IDs of the reader's uploads a body in Markdown shows as images (`![alt](media:ID)`), each once, in order. */
  const bodyImages = (text: string) =>
    [...new Set([...text.matchAll(/!\[[^\]]*\]\(media:([^)\s]+)\)/g)].map((x) => x[1] ?? ""))].toSorted();
  /** A draft as a save leaves it: its content replaced by `body`, saved now (a millisecond after its last save at least). */
  const savedDraft = (d: Json, body: Json): Json => {
    const last = Date.parse(d["updated_at"] as string);
    const { scheduled_at: scheduled, last_schedule: outcome } = d;
    const article = body["article"] as Json | undefined;
    return {
      id: d["id"],
      created_at: d["created_at"],
      updated_at: new Date(Math.max(Date.now(), last + 1)).toISOString(),
      text: "",
      spoiler_text: "",
      sensitive: false,
      media: [],
      ...body,
      // An article's draft lists the uploads its body shows, as the server finds them at each save.
      ...(article === undefined
        ? {}
        : {
            article: {
              title: "",
              summary: "",
              tags: [],
              ...article,
              images: bodyImages(typeof body["text"] === "string" ? body["text"] : ""),
            },
          }),
      ...(scheduled === undefined ? {} : { scheduled_at: scheduled }),
      ...(outcome === undefined ? {} : { last_schedule: outcome }),
    };
  };
  const putDraft = (d: Json) => {
    ownDrafts.splice(
      ownDrafts.findIndex((x) => x["id"] === d["id"]),
      1,
      d,
    );
  };
  const unprocessable = (detail: string) =>
    json({ type: "about:blank", title: "Unprocessable Content", status: 422, detail }, 422);
  /** An article the server's automated review refuses, as the server answers it. */
  const refusedArticle = () =>
    json(
      {
        type: "/api/v1/problems/status-rejected",
        title: "The post was rejected",
        status: 422,
        detail: "This server's review did not accept the article. Change it and try again.",
      },
      422,
    );
  /** An article refused for the value of its summary, which the problem names in `field`, as the server answers it. */
  const refusedSummary = () =>
    json(
      {
        type: "about:blank",
        title: "This could not be done.",
        status: 422,
        detail: "This value is not valid. Check what you entered.",
        field: "summary",
      },
      422,
    );
  /** The reader's uploads, by their IDs: the file each was, as the fake serves it. */
  const uploads = new Map<string, Json>();
  /** The statuses the fake made (a draft published, an article posted), by their IDs, read alone as the server reads them. */
  const made = new Map<string, Json>();
  /**
   * The summary the server makes of an article's body: the text of its blocks (no headings, code or images) with one
   * space between blocks and lines, as the server joins them, cut at a word, with an ellipsis.
   */
  const summaryOf = (text: string) => {
    const plain = text
      .split("\n")
      .filter((l) => !l.startsWith("#") && !l.startsWith("```"))
      // A line's block marker (a list item's, a quote's) is no text of it.
      .map((l) => l.replace(/^\s*(?:[-*+]|\d+[.)]|>)\s+/, ""))
      .join(" ")
      .replace(/!\[[^\]]*\]\([^)]*\)|\*\*|`/g, "")
      .replace(/\s+/g, " ")
      .trim();
    if (plain.length <= 200) return plain;
    const cut = plain.slice(0, 200);
    return `${cut.slice(0, Math.max(cut.lastIndexOf(" "), 1))}…`;
  };
  /** The HTML of an article's body, its images of the reader's uploads as images, any other as a link of its alt text. */
  const bodyHtml = (text: string) => {
    // Each image of an upload stands as a mark the rendering leaves alone, then as its <img>.
    const images: string[] = [];
    const marked = text.replace(/!\[([^\]]*)\]\(media:([^)\s]+)\)/g, (whole, alt: string, id: string) => {
      const file = uploads.get(id) as { url?: string; meta?: { width?: number; height?: number } } | undefined;
      if (file === undefined) return whole;
      const { width = 0, height = 0 } = file.meta ?? {};
      images.push(
        `<img src="${file.url ?? ""}" alt="${alt.replace(/"/g, "&quot;")}" width="${String(width)}" height="${String(height)}" loading="lazy">`,
      );
      return `⟦${String(images.length - 1)}⟧`;
    });
    return articleHtml(marked, fakeHost).replace(/⟦(\d+)⟧/g, (_, n: string) => images[Number(n)] ?? "");
  };
  /** An article's part of a status, from what its author wrote: its title, its summary, its cover. */
  const madeArticle = (article: Json, text: string): Json => {
    const summary = typeof article["summary"] === "string" ? article["summary"].trim() : "";
    const cover = article["cover"] as { id: string; description?: string } | undefined;
    const file = cover === undefined ? undefined : uploads.get(cover.id);
    return {
      title: article["title"],
      summary: summary === "" ? summaryOf(text) : summary,
      ...(file === undefined ? {} : { cover: { ...file, description: cover?.description ?? "" } }),
    };
  };
  /**
   * A status the reader makes: a draft published, or an article posted (with its body, as read alone), newest, on top
   * of their home timeline.
   */
  const publish = (d: Json): Json => {
    draftsMade += 1;
    const at = Date.now();
    const article = d["article"] as Json | undefined;
    const text = d["text"] as string;
    const id = idAt(at, 950_000 + draftsMade);
    const status: Json = {
      id,
      uri: `${fakeOrigin}/users/${viewer}/statuses/${id}`,
      url: `${fakeOrigin}/@${viewer}/${id}`,
      account: accountOf(viewer),
      content: article === undefined ? serverHtml(text, fakeHost) : bodyHtml(text),
      spoiler_text: d["spoiler_text"] ?? "",
      sensitive: d["sensitive"] ?? false,
      visibility: d["visibility"],
      language: d["language"] ?? "en",
      created_at: new Date(at).toISOString(),
      ...(d["in_reply_to_id"] === undefined ? {} : { in_reply_to_id: d["in_reply_to_id"] }),
      media_attachments: [],
      mentions: [],
      tags: ((article?.["tags"] ?? []) as string[]).map((name) => ({
        name: name.replace(/^#/, ""),
        url: `${fakeOrigin}/tags/${encodeURIComponent(name.replace(/^#/, "").toLowerCase())}`,
      })),
      emojis: customEmojis,
      replies_count: 0,
      boosts_count: 0,
      favourites_count: 0,
      reactions: [],
      favourited: false,
      boosted: false,
      bookmarked: false,
      pinned: false,
      ...(article === undefined ? {} : { article: madeArticle(article, text) }),
    };
    made.set(id, status);
    if (article !== undefined) madeSources.set(id, { text, article });
    // A list holds no article's body (ADR-0160 §8).
    home.unshift(article === undefined ? status : { ...status, content: "" });
    return status;
  };
  /** What the reader wrote of each article the fake made, by its ID: its body and its article's part. */
  const madeSources = new Map<string, { text: string; article: Json }>();
  /** Edits the article of `id` the fake made, as `PUT /articles/{id}` does: its body, title, summary, cover, hashtags. */
  const editMade = (id: string, edit: Json): Json | undefined => {
    const before = made.get(id);
    if (before === undefined) return undefined;
    const text = edit["text"] as string;
    const article = {
      title: edit["title"],
      summary: edit["summary"] ?? "",
      tags: edit["tags"] ?? [],
      ...(edit["cover"] === undefined ? {} : { cover: edit["cover"] }),
    };
    const after: Json = {
      ...before,
      content: bodyHtml(text),
      sensitive: edit["sensitive"] ?? before["sensitive"],
      language: edit["language"] ?? before["language"],
      edited_at: new Date().toISOString(),
      article: madeArticle(article, text),
    };
    made.set(id, after);
    madeSources.set(id, { text, article });
    const at = home.findIndex((s) => s["id"] === id);
    if (at >= 0) home.splice(at, 1, { ...after, content: "" });
    return after;
  };
  // The notification of each failed schedule, among the others by its time.
  for (const d of ownDrafts) {
    const failed = d["last_schedule"] as { publish_at: string; reason: string; occurred_at: string } | undefined;
    if (failed === undefined) continue;
    notifications.push({
      id: idAt(Date.parse(failed.occurred_at), 10_900),
      type: "scheduled_status_failed",
      created_at: failed.occurred_at,
      scheduled_status_failure: { draft_id: d["id"], reason: failed.reason, publish_at: failed.publish_at },
    });
  }
  notifications.sort((a, b) => ((b["id"] as string) > (a["id"] as string) ? 1 : -1));

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
  // The translation provider of `translation`, with the visibilities a provider receives by default.
  const translationProvider = {
    id: "fake",
    name: "Demo Translate",
    visibilities: ["public", "local", "unlisted"],
  };
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
    limits: {
      max_post_characters: 500,
      require_media_description: false,
      drafts: draftLimits,
      articles: articleLimits,
    },
    article_authoring: o.articles?.authoring ?? "everyone",
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
    full_text_search:
      o.search?.posts === true
        ? { available: true, visibilities: ["public", "local"] }
        : { available: false, visibilities: [] },
    translation: {
      browser: o.translation?.browser === true,
      providers: o.translation?.provider === undefined ? [] : [translationProvider],
    },
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
    // An account the reader no longer follows leaves every list of theirs (ADR-0013 §11).
    if (!following.has(edge)) for (const l of lists) l.members = l.members.filter((x) => x !== other);
    return relationshipOf(id);
  }

  /** The operations a visitor without a session may call; every other answers 401. */
  const publicPath =
    /^\/(?:instance(?:\/health)?|custom_emojis|accounts\/lookup|accounts\/[^/]+(?:\/statuses|\/featured_tags)?|statuses\/[^/]+(?:\/context|\/history|\/reactions)?|timelines\/(?:local|federated|tag\/[^/]+)|tags\/[^/]+)$/;
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
      const { from, body } = page(home.flatMap(current).filter(inHomeNow), q);
      if (o.failFirst === true && from === 0 && Date.now() - started < 1000)
        return problem(500, "Internal Server Error");
      // The retry after the first page failed comes late, so that the screen shows it loading again.
      if (o.failFirst === true) await wait(o.lateMs ?? 2000);
      else if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/timelines\/tag\/([^/]+)$/))) {
      // The public posts with the hashtag, newest first, as they read now, and its local ones for a reader signed in.
      const tag = decodeURIComponent(match[1] ?? "")
        .replace(/^#/, "")
        .toLowerCase();
      const audience = o.visitor === true ? ["public"] : ["public", "local"];
      const shown = posts
        .filter(
          (p) =>
            visible(p) &&
            audience.includes(p.visibility) &&
            (o.visitor === true || (!hidden(p.author) && !mutedServer(p.author))),
        )
        .filter((p) => hashtagsOf(versionsOf(p).at(-1)?.text ?? p.text).includes(tag))
        .toReversed()
        .map((p) => statusOf(p));
      const { from, body } = page(shown, q);
      if (from > 0) await late();
      return json(body);
    }
    // `/tags/search` is the search of hashtags, below.
    if ((match = m(/^\/tags\/(?!search$)([^/]+)(\/follow)?$/))) {
      // A hashtag as first written in a post, else as asked; whether the reader follows it. One more than the most
      // the reader may follow is refused as the server refuses it.
      const asked = decodeURIComponent(match[1] ?? "").replace(/^#/, "");
      const key = asked.toLowerCase();
      if (match[2] !== undefined && request.method === "POST" && !followedTags.has(key)) {
        if (followedTags.size >= (o.followedTagLimit ?? 100))
          return json(
            {
              type: "/api/v1/problems/too-many-followed-tags",
              title: "Too many followed hashtags",
              detail: "You follow as many hashtags as you can. Unfollow one to follow another.",
              status: 422,
            },
            422,
          );
        followedTags.add(key);
      }
      if (match[2] !== undefined && request.method === "DELETE") followedTags.delete(key);
      const written = posts
        .flatMap((p) => [...p.text.matchAll(/#([\p{L}\p{M}\p{N}_]+)/gu)].map((x) => x[1] ?? ""))
        .find((name) => name.toLowerCase() === key);
      const name = written ?? asked;
      return json({ name, url: `${fakeOrigin}/tags/${encodeURIComponent(key)}`, following: followedTags.has(key) });
    }
    if (path === "/lists" && request.method === "POST") {
      // A name of 1 to 50 characters on one line; at the limit of lists, the server's refusal.
      const name = (((await request.json()) as { name?: string }).name ?? "").trim();
      if (!listName(name)) return listNameRefused();
      if (lists.length >= listLimit)
        return json(
          { type: "/api/v1/problems/too-many-lists", title: "You have as many lists as you may", status: 422 },
          422,
        );
      madeLists += 1;
      const made = {
        id: idAt(Date.now(), 61_000 + madeLists),
        name,
        members: [],
        created_at: new Date().toISOString(),
      };
      lists.push(made);
      return json(listBody(made), 201);
    }
    if (path === "/lists") return json({ items: lists.map(listBody) });
    if ((match = m(/^\/accounts\/([^/]+)\/lists$/))) {
      const username = usernameOf.get(decodeURIComponent(match[1] ?? ""));
      return json({ items: lists.filter((l) => username !== undefined && l.members.includes(username)).map(listBody) });
    }
    if ((match = m(/^\/lists\/([^/]+)\/accounts\/([^/]+)$/))) {
      const found = lists.find((l) => l.id === decodeURIComponent(match?.[1] ?? ""));
      const username = usernameOf.get(decodeURIComponent(match[2] ?? ""));
      if (found === undefined || username === undefined) return problem(404, "Not Found");
      if (request.method === "DELETE") {
        found.members = found.members.filter((x) => x !== username);
        return new Response(null, { status: 204 });
      }
      if (found.members.includes(username)) return new Response(null, { status: 204 });
      if (!follows_(viewer, username))
        return json(
          { type: "/api/v1/problems/not-followed", title: "You can add only accounts you follow", status: 422 },
          422,
        );
      if (found.members.length >= memberLimit)
        return json(
          {
            type: "/api/v1/problems/too-many-list-members",
            title: "The list has as many members as it may",
            status: 422,
          },
          422,
        );
      found.members.push(username);
      return new Response(null, { status: 204 });
    }
    if ((match = m(/^\/lists\/([^/]+)\/accounts$/))) {
      // The members in the order of their IDs.
      const found = lists.find((l) => l.id === decodeURIComponent(match?.[1] ?? ""));
      if (found === undefined) return problem(404, "Not Found");
      const members = found.members.map(accountOf).toSorted((a, b) => String(a["id"]).localeCompare(String(b["id"])));
      const { from, body } = page(members, q);
      if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/lists\/([^/]+)$/))) {
      const at = lists.findIndex((l) => l.id === decodeURIComponent(match?.[1] ?? ""));
      const found = lists[at];
      if (found === undefined) return problem(404, "Not Found");
      if (request.method === "DELETE") {
        lists.splice(at, 1);
        return new Response(null, { status: 204 });
      }
      if (request.method === "PATCH") {
        const name = (((await request.json()) as { name?: string }).name ?? "").trim();
        if (!listName(name)) return listNameRefused();
        found.name = name;
      }
      return json(listBody(found));
    }
    if ((match = m(/^\/timelines\/list\/([^/]+)$/))) {
      // The home timeline's entries by the list's members, never the reader's own (ADR-0013 §13).
      const found = lists.find((l) => l.id === decodeURIComponent(match?.[1] ?? ""));
      if (found === undefined) return problem(404, "Not Found");
      const shown = home
        .flatMap(current)
        .filter(inHomeNow)
        .filter((s) => found.members.includes((s["account"] as Json)["username"] as string));
      const { from, body } = page(shown, q);
      if (from > 0) await late();
      return json(body);
    }
    if (path === "/bookmarks") {
      const shown = bookmarks
        .map(postOf)
        .filter((p) => visible(p))
        .map((p) => statusOf(p));
      const { from, body } = page(shown, q);
      if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/statuses\/([^/]+)\/bookmark$/)) && request.method !== "GET") {
      const key = keyOf.get(decodeURIComponent(match[1] ?? ""));
      const p = key === undefined ? undefined : postOf(key);
      if (p === undefined || !visible(p)) return problem(404, "Not Found");
      const at = bookmarks.indexOf(p.key);
      if (at >= 0) bookmarks.splice(at, 1);
      if (request.method === "POST") bookmarks.unshift(p.key);
      return json(statusOf(p));
    }
    if ((match = m(/^\/timelines\/(local|federated)$/))) {
      // The scenario's people are all this server's, so both public timelines hold their public posts (the local one
      // also those for this server only), newest first; a timeline the instance does not offer is a 404, one for
      // signed-in readers a 401 to a visitor.
      const kind = match[1] === "local" ? "local" : "federated";
      if (timelines[kind] === "off") return problem(404, "Not Found");
      if (timelines[kind] === "signed_in" && o.visitor === true) return problem(401, "Unauthorized");
      const shown = posts
        .filter((p) => !deleted.has(p.key))
        .filter((p) => p.visibility === "public" || (kind === "local" && p.visibility === "local"))
        .filter((p) => o.visitor === true || (!hidden(p.author) && !mutedServer(p.author)))
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
    if (path === "/accounts/search") {
      // As the server finds them (ADR-0092 §1, FR-307): a username, `username@host` or a word of the display name that
      // starts with `q`, and an account of this server whose username or display name holds it; the blocked and muted
      // left out, and the person of another server it has not met; the exact username first, then the followed, then
      // the rest.
      const query = (q.get("q") ?? "").replace(/^@/u, "").normalize("NFKC").toLowerCase();
      const limit = Number(q.get("limit") ?? "8");
      const name = (p: Person) => p.displayName.normalize("NFKC").toLowerCase().replace(/:\w+:/gu, "");
      const found = people
        .filter((p) => !blocking.has(`${viewer}>${p.username}`) && !muting.has(`${viewer}>${p.username}`))
        .filter((p) => p.username !== unmet || met.has(unmet))
        .filter(
          (p) =>
            [
              p.username.includes("@") ? p.username : `${p.username}@${fakeHost}`,
              name(p),
              ...name(p).split(/\s+/u),
            ].some((w) => w.startsWith(query)) ||
            (!p.username.includes("@") && (p.username.includes(query) || name(p).includes(query))),
        )
        .toSorted(
          (a, b) =>
            Number(b.username === query) - Number(a.username === query) ||
            Number(follows_(viewer, b.username)) - Number(follows_(viewer, a.username)),
        );
      return json({ items: found.slice(0, limit).map(fullAccount) });
    }
    if (path === "/tags/search") {
      // The hashtags of the public, local and unlisted posts that start with `q`: the one equal to it first, then the
      // most used.
      const query = (q.get("q") ?? "").replace(/^#/u, "").normalize("NFKC").toLowerCase();
      const limit = Number(q.get("limit") ?? "8");
      const uses = new Map<string, { name: string; count: number }>();
      for (const p of posts)
        if (!deleted.has(p.key) && p.visibility !== "private" && p.visibility !== "direct")
          for (const [, tag = ""] of p.text.matchAll(/(?<![\p{L}\p{N}_])#([\p{L}\p{M}\p{N}_]+)/gu)) {
            const key = tag.normalize("NFKC").toLowerCase();
            uses.set(key, { name: uses.get(key)?.name ?? tag, count: (uses.get(key)?.count ?? 0) + 1 });
          }
      const found = [...uses]
        .filter(([key]) => key.startsWith(query))
        .toSorted(([ka, a], [kb, b]) => Number(kb === query) - Number(ka === query) || b.count - a.count)
        .map(([, t]) => ({ name: t.name, statuses_count: t.count }));
      return json({ items: found.slice(0, limit) });
    }
    if (path === "/accounts/lookup") {
      // A handle, or the address of a profile (`https://host/@user`); the person of another server this one has not
      // met is found only by `resolve`, which fetches and keeps their account (FR-305).
      const asked = (q.get("acct") ?? "").replace(/^@/u, "");
      const profile = /^https?:\/\/([^/]+)\/@([^/@]+)\/?$/u.exec(asked);
      const handle = profile === null ? asked : `${profile[2] ?? ""}@${profile[1] ?? ""}`;
      const acct = handle.replace(new RegExp(`@${fakeHost}$`), "");
      if (acct === unmet && q.get("resolve") === "true") met.add(unmet);
      const found =
        acct === deactivated || (acct === unmet && !met.has(unmet))
          ? undefined
          : people.find((p) => p.username === acct);
      return found === undefined ? problem(404, "Not Found") : json(fullAccount(found));
    }
    if (path === "/statuses/search") {
      // The public and local posts that hold every word, newest first, where the server has a search engine (ADR-0149).
      if (o.search?.posts !== true)
        return json(
          {
            type: "/api/v1/problems/search-unavailable",
            title: "Posts cannot be searched on this server",
            status: 404,
          },
          404,
        );
      const words = (q.get("q") ?? "")
        .normalize("NFKC")
        .toLowerCase()
        .split(/\s+/u)
        .filter((w) => w !== "");
      const shown = posts
        .filter((p) => !deleted.has(p.key) && (p.visibility === "public" || p.visibility === "local"))
        .filter((p) => !hidden(p.author) && visible(p))
        .filter((p) => words.every((w) => p.text.normalize("NFKC").toLowerCase().includes(w)))
        .toReversed()
        .map((p) => statusOf(p));
      const { from, body } = page(shown, q);
      if (from > 0) await late();
      return json(body);
    }
    if (path === "/accounts/suggestions") {
      // This server's accounts the reader does not follow, those that posted last first (FR-308); none on a
      // one-person server.
      if (o.singleUser === true)
        return json(
          { type: "/api/v1/problems/single-user-mode", title: "This is a one-person server", status: 404 },
          404,
        );
      const lastPost = (username: string) => posts.findLastIndex((p) => p.author === username);
      const suggested = people
        .filter((p) => !p.username.includes("@") && p.username !== viewer && p.username !== deactivated)
        .filter((p) => !follows_(viewer, p.username) && !requested.has(`${viewer}>${p.username}`))
        .filter((p) => !blocking.has(`${viewer}>${p.username}`) && !muting.has(`${viewer}>${p.username}`))
        .toSorted((a, b) => lastPost(b.username) - lastPost(a.username))
        .map((p) => ({ source: "recently_active", account: accountOf(p.username) }));
      return json({ items: suggested });
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
      const shown =
        q.get("articles") === "true"
          ? all.filter((s) => s["article"] !== undefined)
          : q.get("exclude_replies") === "true"
            ? all.filter((s) => s["in_reply_to_id"] === undefined)
            : all;
      const { from, body } = page(shown, q);
      if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/accounts\/([^/]+)\/featured_tags$/))) return json({ items: [] });
    if ((match = m(/^\/accounts\/([^/]+)\/(followers|following)$/))) {
      // The accounts of the list, newest follow first; a list the owner keeps from the reader is a 403.
      if (o.followsLists === "waits") return new Promise<Response>(() => undefined);
      if (o.followsLists === "fails") return problem(500, "Internal Server Error");
      const username = usernameOf.get(decodeURIComponent(match[1] ?? ""));
      if (username === undefined) return problem(404, "Not Found");
      const shown = followsVisibility[username] ?? "public";
      if (username !== viewer && shown !== "public" && !(shown === "followers" && follows_(viewer, username)))
        return json(
          {
            type: "/api/v1/problems/follows-hidden",
            title: "This account's follows are not shown",
            status: 403,
          },
          403,
        );
      const followers = match[2] === "followers";
      const listed = [...following]
        .toReversed()
        .map((edge) => edge.split(">") as [string, string])
        .filter(([from, to]) => (followers ? to === username : from === username))
        .map(([from, to]) => accountOf(followers ? from : to));
      const { from, body } = page(listed, q);
      if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/accounts\/([^/]+)\/(follow|unfollow|block|unblock|mute|unmute)$/))) {
      const verb = match[2] ?? "";
      const add = request.method === "POST" && !verb.startsWith("un");
      const r = change(decodeURIComponent(match[1] ?? ""), verb.replace(/^un/, ""), add);
      return r === undefined ? problem(404, "Not Found") : json(r);
    }
    if ((match = m(/^\/accounts\/([^/]+)\/(note|subscribe|follower)$/)) && request.method !== "GET") {
      // The reader's private note on a person, being notified of their new posts, and removing them as a follower.
      const id = decodeURIComponent(match[1] ?? "");
      const other = usernameOf.get(id);
      if (other === undefined) return problem(404, "Not Found");
      if (match[2] === "note") {
        const note = request.method === "PUT" ? ((await request.json()) as { note: string }).note : "";
        // The server counts code points.
        if (Array.from(note).length > 2000) return problem(422, "Unprocessable Entity");
        if (note === "") notes.delete(other);
        else notes.set(other, note);
      } else if (match[2] === "subscribe") {
        if (request.method === "POST") subscribed.add(other);
        else subscribed.delete(other);
      } else following.delete(`${other}>${viewer}`);
      return json(relationshipOf(id));
    }
    if (path === "/domain_mutes" && request.method === "GET")
      return json(
        page(
          domainMutes.map((m) => ({ ...m, id: m.domain })),
          q,
        ).body,
      );
    if (path === "/domain_mutes" && request.method === "POST") {
      // A remote domain, as the server normalizes it; this server's own is refused.
      const domain = ((await request.json()) as { domain: string }).domain.toLowerCase().replace(/\.$/, "");
      if (domain === fakeHost || !domain.includes(".")) return problem(422, "Unprocessable Entity");
      let mute = domainMutes.find((d) => d.domain === domain);
      if (mute === undefined) {
        mute = { domain, created_at: new Date().toISOString() };
        domainMutes.push(mute);
      }
      return json(mute);
    }
    if ((match = m(/^\/domain_mutes\/([^/]+)$/)) && request.method === "DELETE") {
      const domain = decodeURIComponent(match[1] ?? "").toLowerCase();
      const at = domainMutes.findIndex((d) => d.domain === domain);
      if (at >= 0) domainMutes.splice(at, 1);
      return new Response(null, { status: 204 });
    }
    if ((match = m(/^\/statuses\/([^/]+)(\/source|\/history)?$/)) && made.has(decodeURIComponent(match[1] ?? ""))) {
      // A status the reader made here: read alone with its body, its source what they wrote, deleted as theirs.
      const sid = decodeURIComponent(match[1] ?? "");
      const status = made.get(sid) ?? {};
      if (match[2] === "/source") {
        const source = madeSources.get(sid);
        return json({
          id: sid,
          text: source?.text ?? "",
          spoiler_text: "",
          language: status["language"],
          ...(source === undefined
            ? {}
            : {
                article: {
                  title: source.article["title"],
                  summary: source.article["summary"] ?? "",
                  tags: source.article["tags"] ?? [],
                  ...(source.article["cover"] === undefined ? {} : { cover: source.article["cover"] }),
                },
              }),
        });
      }
      if (match[2] === "/history") return json({ items: [] });
      if (request.method === "GET") return json(status);
      if (request.method === "DELETE") {
        made.delete(sid);
        const at = home.findIndex((s) => s["id"] === sid);
        if (at >= 0) home.splice(at, 1);
        return new Response(null, { status: 204 });
      }
      return problem(405, "Method Not Allowed");
    }
    if ((match = m(/^\/statuses\/([^/]+)\/context$/)) && made.has(decodeURIComponent(match[1] ?? "")))
      return json({ ancestors: [], descendants: [] });
    if ((match = m(/^\/statuses\/([^/]+)(\/source|\/history)?$/))) {
      const key = keyOf.get(decodeURIComponent(match[1] ?? ""));
      const p = key === undefined ? undefined : postOf(key);
      if (p === undefined || !visible(p)) return problem(404, "Not Found");
      const id = statusId.get(p.key) ?? "";
      const own = p.author === viewer;
      if (match[2] === "/history" && request.method === "GET")
        return json({
          items: versionsOf(p).map((v) => ({
            content: serverHtml(v.text, fakeHost),
            spoiler_text: v.spoiler,
            sensitive: v.spoiler !== "",
            created_at: new Date(v.at).toISOString(),
            media_attachments: mediaOf(p, id, v),
            emojis: customEmojis,
          })),
        });
      // The source of another's post is a 404, as the server's is.
      if (match[2] === "/source" && request.method === "GET") {
        const now = versionsOf(p).at(-1);
        const a = p.article;
        const edited = articleEdits.get(p.key);
        return own
          ? json({
              id,
              text: now?.text ?? p.text,
              spoiler_text: now?.spoiler ?? "",
              language: p.language,
              ...(a === undefined
                ? {}
                : {
                    article: {
                      title: edited?.title ?? a.title,
                      summary: edited?.summary ?? a.summary,
                      tags: [...(edited?.tags ?? [])],
                      ...(a.cover === undefined
                        ? {}
                        : { cover: { id: `${id}-cover`, description: a.cover.description } }),
                    },
                  }),
            })
          : problem(404, "Not Found");
      }
      if (match[2] !== undefined) return problem(404, "Not Found");
      if (request.method === "GET") return json(statusOf(p, 0, true));
      if (!own) return problem(403, "Forbidden");
      if (request.method === "DELETE") {
        deleted.add(p.key);
        return new Response(null, { status: 204 });
      }
      if (request.method === "PUT") {
        // A post with a poll is never edited: its votes belong to the poll as posted. An article is edited with
        // PUT /articles/{id}.
        if (p.poll !== undefined || p.article !== undefined) return problem(422, "Unprocessable Entity");
        const edit = (await request.json()) as { text: string; spoiler_text?: string; media?: { id: string }[] };
        edits.set(p.key, [
          ...(edits.get(p.key) ?? []),
          {
            text: edit.text,
            spoiler: edit.spoiler_text ?? "",
            at: Date.now(),
            media: (edit.media ?? []).map((x) => x.id),
          },
        ]);
        return json(statusOf(p));
      }
      return problem(405, "Method Not Allowed");
    }
    if ((match = m(/^\/statuses\/([^/]+)\/poll\/votes$/)) && request.method === "POST") {
      const key = keyOf.get(decodeURIComponent(match[1] ?? ""));
      const p = key === undefined ? undefined : postOf(key);
      if (p === undefined || !visible(p)) return problem(404, "Not Found");
      const poll = pollOf(p);
      const vote = (await request.json()) as { poll_id: string; choices: number[] };
      if (poll !== undefined && poll["id"] !== vote.poll_id) return problem(409, "Conflict");
      const { choices } = vote;
      const taken =
        poll !== undefined &&
        poll["voted"] !== true &&
        choices.length > 0 &&
        (p.poll?.multiple === true || choices.length === 1) &&
        new Set(choices).size === choices.length &&
        choices.every((c) => Number.isInteger(c) && c >= 0 && c < (p.poll?.options.length ?? 0));
      if (!taken) return problem(422, "Unprocessable Entity");
      ownVotes.set(p.key, choices);
      return json(statusOf(p));
    }
    if (path === "/custom_emojis") return json({ items: pickerEmojis });
    if ((match = m(/^\/statuses\/([^/]+)\/(reaction|favourite)$/)) && request.method !== "GET") {
      const key = keyOf.get(decodeURIComponent(match[1] ?? ""));
      const p = key === undefined ? undefined : postOf(key);
      if (p === undefined || !visible(p)) return problem(404, "Not Found");
      // One favourite or one reaction per post (FR-116): setting one takes back the other. What the reader has now is
      // kept as theirs first, so that taking back one leaves the other as it was.
      const favourited = favouritersOf(p.key).includes(viewer);
      const reacted = reactionsOf(p.key).find((r) => r.by === viewer)?.emoji ?? null;
      ownFavourite.set(p.key, favourited);
      ownReaction.set(p.key, reacted);
      if (match[2] === "favourite") {
        const add = request.method === "POST";
        ownFavourite.set(p.key, add);
        if (add) ownReaction.set(p.key, null);
      } else if (request.method === "PUT") {
        const { emoji } = (await request.json()) as { emoji: string };
        const named = scenarioEmoji(emoji);
        // A custom emoji of another server, or a shortcode this server has not, is refused as the server does.
        if (named.includes("@") || /^\w+$/u.test(named)) return problem(422, "Unprocessable Entity");
        ownReaction.set(p.key, named);
        ownFavourite.set(p.key, false);
      } else {
        ownReaction.set(p.key, null);
      }
      return json(statusOf(p));
    }
    if ((match = m(/^\/statuses\/([^/]+)\/reactions$/))) {
      const key = keyOf.get(decodeURIComponent(match[1] ?? ""));
      const p = key === undefined ? undefined : postOf(key);
      if (p === undefined || !visible(p)) return problem(404, "Not Found");
      const emoji = q.get("emoji");
      const shown = reactionsOf(p.key)
        .filter((r) => emoji === null || reactionEmojiOf(r.emoji).name === emoji)
        .toReversed()
        .map((r) => ({
          id: idAt(timeOf(r.step), 70_000 + r.step),
          account: accountOf(r.by),
          emoji: reactionEmojiOf(r.emoji),
          created_at: new Date(timeOf(r.step)).toISOString(),
        }));
      const { from, body } = page(shown, q);
      if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/statuses\/([^/]+)\/translation$/)) && request.method === "POST") {
      const key = keyOf.get(decodeURIComponent(match[1] ?? ""));
      const p = key === undefined ? undefined : postOf(key);
      if (p === undefined || !visible(p)) return problem(404, "Not Found");
      const { target_language: target } = (await request.json()) as { target_language: string };
      return translationOf(p, target);
    }
    if ((match = m(/^\/statuses\/([^/]+)\/context$/))) {
      const key = keyOf.get(decodeURIComponent(match[1] ?? ""));
      return key === undefined ? problem(404, "Not Found") : json(contextOf(key));
    }
    if (path === "/notifications") {
      const types = q.getAll("types");
      // None from whom the reader blocks or mutes now, or from a server they mute.
      const typed = notifications
        .filter((n) => types.length === 0 || types.includes(n["type"] as string))
        .filter((n) => !hidden(authorOf(n)) && !mutedServer(authorOf(n)));
      // A notification of a post shows it as it is now, and goes with it once it is deleted.
      const shown = typed.flatMap((n) => {
        const s = n["status"] as Json | undefined;
        if (s === undefined) return [n];
        const now = current(s)[0];
        return now === undefined ? [] : [{ ...n, status: now }];
      });
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
    if (path === "/conversations") {
      const { from, body } = page(
        conversationsNow()
          .filter((c) => !c.hidden)
          .map((c) => c.body),
        q,
      );
      if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/conversations\/([^/]+)(\/read|\/hide)?$/))) {
      const found = conversationOf(decodeURIComponent(match[1] ?? ""));
      if (found === undefined) return problem(404, "Not Found");
      const id = found.body["id"] as string;
      if (match[2] === undefined && request.method === "GET") return json(found.body);
      if (match[2] === "/read" && request.method === "POST") {
        readTo.set(id, found.at);
        return json({ ...found.body, unread: false });
      }
      if (match[2] === "/hide" && request.method === "POST") {
        hiddenTo.set(id, found.at);
        return new Response(null, { status: 204 });
      }
      return problem(405, "Method Not Allowed");
    }
    if (path === "/notifications/requests/summary") return json({ accounts_count: 0, notifications_count: 0 });
    if (path === "/interaction_requests" && request.method === "GET") {
      // Those of the reader's posts that wait, newest first, of one kind when it is asked.
      const kind = q.get("kind");
      const waiting = requests
        .filter(
          (r) => postOf(r.target).author === viewer && stateOf(r) === "pending" && (kind === null || r.kind === kind),
        )
        .toSorted((a, b) => b.step - a.step)
        .map(requestBody);
      const { from, body } = page(waiting, q);
      if (from > 0) await late();
      return json(body);
    }
    if ((match = m(/^\/interaction_requests\/([^/]+)\/(accept|reject)$/)) && request.method === "POST") {
      const r = requests.find((x) => requestId(x) === decodeURIComponent(match?.[1] ?? ""));
      if (r === undefined || postOf(r.target).author !== viewer) return problem(404, "Not Found");
      if (stateOf(r) !== "pending") return problem(409, "Conflict");
      answers.set(r.key, match[2] === "accept" ? "accepted" : "rejected");
      return json(requestBody(r));
    }
    if (path === "/articles/preview" && request.method === "POST") {
      // The body as the server would render it, with its size and images; one that fails, as the options say.
      if (o.articles?.preview === "fails") return problem(500, "Internal Server Error");
      const { text, summary } = (await request.json()) as { text: string; summary?: string };
      const content = bodyHtml(text);
      const blank = summary === undefined || summary.trim() === "";
      return json({
        content,
        summary: blank ? summaryOf(text) : summary.trim(),
        summary_from_body: blank,
        bytes: new TextEncoder().encode(content).length,
        max_bytes: articleLimits.max_bytes,
        images: bodyImages(text).filter((id) => uploads.has(id)).length,
        max_images: articleLimits.max_images,
      });
    }
    if (path === "/articles" && request.method === "POST") {
      if (o.articles?.authoring === "permission" && !permissionsOf(viewer).includes("create_articles"))
        return problem(403, "Forbidden");
      if (o.articles?.publish === "refused") return refusedArticle();
      if (o.articles?.publish === "summary") return refusedSummary();
      const body = (await request.json()) as Json;
      const { title, summary, tags, cover, ...rest } = body;
      return json(
        publish({ ...rest, article: { title, summary, tags, ...(cover === undefined ? {} : { cover }) } }),
        201,
      );
    }
    if ((match = m(/^\/articles\/([^/]+)$/)) && request.method === "PUT") {
      const sid = decodeURIComponent(match[1] ?? "");
      if (o.articles?.publish === "refused") return refusedArticle();
      if (o.articles?.publish === "summary") return refusedSummary();
      const edit = (await request.json()) as Json;
      const after = editMade(sid, edit);
      if (after !== undefined) return json(after);
      const key = keyOf.get(sid);
      const p = key === undefined ? undefined : postOf(key);
      if (p === undefined || !visible(p)) return problem(404, "Not Found");
      if (p.author !== viewer) return problem(403, "Forbidden");
      if (p.article === undefined) return problem(422, "Unprocessable Entity");
      articleEdits.set(p.key, {
        title: edit["title"] as string,
        summary: (edit["summary"] as string | undefined) ?? "",
        tags: (edit["tags"] as string[] | undefined) ?? [],
      });
      edits.set(p.key, [...(edits.get(p.key) ?? []), { text: edit["text"] as string, spoiler: "", at: Date.now() }]);
      return json(statusOf(p, 0, true));
    }
    if (path === "/media" && request.method === "POST") {
      // An image the reader uploads, ready at once, served from the browser's own copy of the file.
      const form = await request.formData();
      const file = form.get("file");
      const description = form.get("description");
      added += 1;
      const id = idAt(Date.now(), 990_000 + added);
      const url = file instanceof Blob ? URL.createObjectURL(file) : "";
      const media: Json = {
        id,
        type: "image",
        state: "ready",
        ...(typeof description === "string" && description !== "" ? { description } : {}),
        url,
        preview_url: url,
        meta: { width: 1200, height: 800, focus: { x: 0, y: 0 } },
      };
      uploads.set(id, media);
      return json(media, 201);
    }
    if (
      (match = m(/^\/media\/([^/]+)$/)) &&
      request.method === "GET" &&
      uploads.has(decodeURIComponent(match[1] ?? ""))
    )
      return json(uploads.get(decodeURIComponent(match[1] ?? "")));
    if (path === "/drafts" && request.method === "GET") {
      const scheduled = q.get("scheduled");
      const shown = ownDrafts.filter(
        (d) => scheduled === null || (d["scheduled_at"] !== undefined) === (scheduled === "true"),
      );
      return json(page(shown, q).body);
    }
    if (path === "/drafts" && request.method === "POST") {
      draftsMade += 1;
      const now = new Date().toISOString();
      const made = savedDraft(
        { id: idAt(Date.now(), 85_000 + draftsMade), created_at: now, updated_at: now },
        (await request.json()) as Json,
      );
      ownDrafts.unshift(made);
      return draftAnswer(made, 201);
    }
    if ((match = m(/^\/drafts\/([^/]+)(\/schedule|\/publish)?$/))) {
      const d = draftOf(decodeURIComponent(match[1] ?? ""));
      if (d === undefined) return problem(404, "Not Found");
      const what = `${request.method} ${match[2] ?? ""}`;
      if (what === "GET ") return draftAnswer(d);
      if (what === "PUT ") {
        // A save over a version another device replaced since is refused, as the server does (ADR-0156 §9).
        if (request.headers.get("If-Match") !== etagOf(d))
          return json({ type: "about:blank", title: "Precondition Failed", status: 412 }, 412);
        const saved = savedDraft(d, (await request.json()) as Json);
        putDraft(saved);
        return draftAnswer(saved);
      }
      if (what === "DELETE ") {
        ownDrafts.splice(ownDrafts.indexOf(d), 1);
        return new Response(null, { status: 204 });
      }
      if (what === "PUT /schedule") {
        const { publish_at: at } = (await request.json()) as { publish_at: string };
        const lead = Date.parse(at) - Date.now();
        if (lead < draftLimits.scheduled_post_min_lead_minutes * minute)
          return unprocessable(
            `A post can be scheduled no sooner than ${String(draftLimits.scheduled_post_min_lead_minutes)} minutes from now.`,
          );
        if (lead > draftLimits.scheduled_post_max_lead_days * 24 * 60 * minute)
          return unprocessable(
            `A post can be scheduled no later than ${String(draftLimits.scheduled_post_max_lead_days)} days from now.`,
          );
        const scheduled = { ...d, scheduled_at: new Date(Date.parse(at)).toISOString() };
        putDraft(scheduled);
        return draftAnswer(scheduled);
      }
      if (what === "DELETE /schedule") {
        if (d["scheduled_at"] === undefined) return problem(409, "Conflict");
        const now = new Date().toISOString();
        const { scheduled_at: at, ...rest } = d;
        const unscheduled = { ...rest, last_schedule: { publish_at: at, outcome: "cancelled", occurred_at: now } };
        putDraft(unscheduled);
        return draftAnswer(unscheduled);
      }
      if (what === "POST /publish") {
        // The review refuses an article published from its draft as it refuses one posted at once; the draft stays.
        if (d["article"] !== undefined && o.articles?.publish === "refused") return refusedArticle();
        ownDrafts.splice(ownDrafts.indexOf(d), 1);
        return json(publish(d), 201);
      }
      return problem(405, "Method Not Allowed");
    }
    if ((match = m(/^\/media\/([^/]+)$/)) && request.method === "GET") {
      const id = decodeURIComponent(match[1] ?? "");
      const image = draftImages.get(id);
      if (image === undefined) return problem(404, "Not Found");
      const url = drawingURL(image.drawing);
      const { width, height } = image.drawing;
      return json({
        id,
        type: "image",
        state: "ready",
        description: image.description,
        url,
        preview_url: url,
        blurhash: drawingBlurhash(image.drawing),
        meta: { width, height, focus: { x: 0, y: 0 } },
      });
    }
    if (path === "/follow_requests") {
      // The accounts that ask to follow the reader, newest first.
      if (o.followRequests?.answer === "waits") return new Promise<Response>(() => undefined);
      if (o.followRequests?.answer === "fails") return problem(500, "Internal Server Error");
      const asking = [...requested]
        .toReversed()
        .map((edge) => edge.split(">") as [string, string])
        .filter(([, to]) => to === viewer)
        .map(([from]) => accountOf(from));
      const { from, body } = page(asking, q);
      if (from > 0) await late();
      return json(body);
    }
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

  const listPath = (index: number) => `/lists/${lists[index]?.id ?? "none"}`;

  const keyedId = (key: string) => {
    const post = statusId.get(key);
    if (post !== undefined) return post;
    const at = drafts.filter((d) => d.author === viewer).findIndex((d) => d.key === key);
    if (at < 0) throw new Error(`fake: no post or draft ${key} of ${viewer}`);
    const all = drafts.filter((d) => d.author === viewer).length;
    return idAt(started - (all - at) * 5 * 60 * minute, 80_000 + at);
  };

  const conversationPath = (index: number) => {
    const id = conversationsNow()[index]?.body["id"];
    return `/conversations/${typeof id === "string" ? id : "none"}`;
  };

  return { serve, newPosts, newNotification, postPath, announcementPath, listPath, idOf: keyedId, conversationPath };
}
