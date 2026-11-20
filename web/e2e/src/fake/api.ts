// SPDX-License-Identifier: AGPL-3.0-only

// The fake of the client API (ADR-0171 §5), the one the component tests and the screens' stories share, where no
// server runs: a `fetch`-compatible `serve(request)` that answers as the server would after the seed applied the
// scenario (scenario.ts), as one person of it reads it. Each operation it implements is a handler of `handlers`, whose
// request and answer are typed by the API specification (spec.ts); a request of no handler fails by name. A fake holds
// its own state, so a test makes one of its own, from the scenario or from a seed it gives.
//
// The statuses, the home timeline, a thread, a profile with its lists of follows and followers (as far as their owner
// shows them), the relationships and the notifications are derived from the seed's people, follows, posts,
// interactions and restrictions; the IDs and times are the fake's own (the server decides them for real), newest last
// in the seed's order; in the scenario a person of another server is the fake's own, since the seed's server federates
// with none, one of them moved, and so is a person of this server who deactivated their account. The writes (a post, a
// boost, a follow, a block, a mute, a private note, being notified of a person's posts, a follower removed, a server
// muted as ADR-0013 §30 hides it, the answers of follow requests, the read marker, the announcements read, the
// conversations of the direct messages read and hidden, the reader's bookmarks, the threads they mute and followed
// hashtags, the reader's edits and deletions of their own posts, the reader's drafts and their schedules, a draft
// published, the reader's articles posted and edited, the images they upload, their preferences, their profile, and
// their personal settings of settings.ts) are kept in memory; a post's edit history holds the scenario's versions of
// it, then the reader's edits. An article's body comes only with the status read alone, as the server's does, and its
// preview renders as publishing would, unless the options make it fail. A list pages as the server's do: `limit` items
// a page, 20 by default, with a cursor while more follow, so that a list read to its end ends; a later page comes late
// only with `lateMs`. With `failFirst` the home timeline's first page fails during the first second, and its retry
// succeeds late. What the shell reads (the instance, the reader's rights and preferences, the announcements, the voice
// rooms, the events) answers in the state the options give the server: its timelines, one person's, its closure, its
// maintenance, down, or newer than the app (the preferences, as `preferences` starts them). The voice rooms are the
// fake's own, since a room lives only while people are in it and the seed opens none. A post is translated by the
// provider `translation` names, which answers or refuses as the options say. With `approvals`, posts of the fake's own
// approve replies, boosts and quotes by hand (ADR-0155): what waits shows to its author alone and counts nowhere, the
// reader's requests list and are answered, and a reply approved joins its thread.
//
// How the reader signs in, the browsers and apps of the account, their own OAuth apps and invites are access.ts's; the
// voice rooms, joining one, and its session's heartbeat and stream, voice.ts's; the events, events.ts's.
//
// The stream (GET /streaming) stays open and silent until `send` gives it an event, or `newPosts` and
// `newNotification` make one. A test scripts an operation with `script` (script.ts). The token revocation the app
// signs out through (POST /oauth/revoke) is no operation of the client API: the fake answers it itself, keeps it
// (`revocations`), or fails it (`revocation`).

import type { StreamEvent } from "@mycroblog/mykit";
import { drawingBlurhash, drawingURL } from "../drawing";
import {
  admin,
  announcements as scenarioAnnouncements,
  conversationReads as scenarioConversationReads,
  drafts as scenarioDrafts,
  draftSchedule,
  emojis as scenarioEmojis,
  events as scenarioEvents,
  follows as scenarioFollows,
  interactions as scenarioInteractions,
  lists as scenarioLists,
  people as scenarioPeople,
  portrait,
  posts as scenarioPosts,
  restrictions as scenarioRestrictions,
  type CustomEmoji,
  type DraftPlan,
  type Follow,
  type Image,
  type Interaction,
  type LinkCard,
  type Person,
  type PersonList,
  type Post,
  type Restriction,
  type ServerAnnouncement,
} from "../scenario";
import { day, idAt, minute, notFound, page, problem, variants, type PageQuery } from "./common";
import { articleHtml, serverHtml } from "./html";
import { createSettings, defaultSettings, type FakeSettings } from "./settings";
import { eventHandlers, fakeEventPlans, type FakeEventPlan, type FakeEvents } from "./events";
import { financeHandlers, type FakeFinanceOptions } from "./finance";
import { financeAdminHandlers, type FakeFinanceAdmin, type FakeFinanceAdminOptions } from "./finance-admin";
import { supporterHandlers, type FakeSupporterOptions, type FakeSupporters } from "./supporters";
import { supportersAdminHandlers, type FakeSupportersAdmin, type FakeSupportersAdminOptions } from "./supporters-admin";
import { voiceHandlers, type FakeVoice, type FakeVoiceMany, type FakeVoiceOpenRefusal } from "./voice";
import { adminHandlers, type FakeAdmin, type FakeAdminOptions } from "./admin-settings";
import { closureHandlers, type FakeClosureOptions } from "./closure";
import { themeHandlers, type FakeThemeOptions } from "./theme";
import { aboutHandlers, type FakeAboutOptions } from "./about";
import { fakeRules, reportHandlers, type FakeReportOptions, type FakeReports } from "./reports";
import { domainHandlers, type FakeDomainOptions, type FakeDomains } from "./domains";
import { permissionsOfRoles } from "./staff-roles";
import { refusalOf, staffServerHandlers, type FakeStaffServer, type FakeStaffServerOptions } from "./staff-server";
import { moderationHandlers, type FakeModeration, type FakeModerationOptions } from "./accounts";
import { operationsHandlers, type FakeOperations, type FakeOperationsOptions } from "./operations";
import { createScripts, type Script, type Scripted } from "./script";
import {
  accessHandlers,
  appIds,
  inviteIds,
  loginProviderIds,
  type AccessOptions,
  type FakeLoginProviderKind,
} from "./access";
import { createRouter, exact, type Answer, type Handlers, type Refusal, type Route, type Schema } from "./spec";

type Account = Schema<"Account">;
type Status = Schema<"Status">;
type Notification = Schema<"Notification">;
type Draft = Schema<"Draft">;
type MediaAttachment = Schema<"MediaAttachment">;

/** A person of a seed; one of another server has their whole handle, `user@domain`, as `username`. */
export type FakePerson = Person & {
  /** The handle of the account they moved to. */
  movedTo?: string;
};

/** What a fake server holds when it starts, in the scenario's terms (scenario.ts), each list oldest first. */
export type FakeSeed = {
  people: readonly FakePerson[];
  follows: readonly Follow[];
  posts: readonly Post[];
  interactions: readonly Interaction[];
  restrictions: readonly Restriction[];
  lists: readonly PersonList[];
  drafts: readonly DraftPlan[];
  announcements: readonly ServerAnnouncement[];
  events: readonly FakeEventPlan[];
  emojis: readonly CustomEmoji[];
  conversationReads: readonly { by: string; post: string }[];
};

/**
 * The people of other servers, the fake's own, since the seed's server federates with none. sora, on photos.example,
 * follows the reader, and sam boosts a post of sora's; sora's account on social.example moved to it, and keeps its
 * posts.
 */
const remotePeople: readonly FakePerson[] = [
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

/** What a fake holds unless it is given a seed: the scenario, then the people of other servers and what they did. */
export const scenario: FakeSeed = {
  people: [...scenarioPeople, ...remotePeople],
  follows: [...scenarioFollows, { follower: "sora@photos.example", followee: "yuki" }],
  posts: [...scenarioPosts, ...remotePosts],
  interactions: [...scenarioInteractions, { kind: "boost", by: "sam", post: "sora-film" }],
  // The reader blocks quickdeals and mutes kai (the scenario's), and blocks the account sora left behind when moving.
  restrictions: [...scenarioRestrictions, { kind: "block", by: "yuki", target: "sora@social.example" }],
  lists: scenarioLists,
  drafts: scenarioDrafts,
  announcements: scenarioAnnouncements,
  // The scenario's events, then the fake's own, whose places, polls, attendees and rooms the seed does not make.
  events: [...scenarioEvents, ...fakeEventPlans],
  emojis: scenarioEmojis,
  conversationReads: scenarioConversationReads,
};

const nothing: FakeSeed = {
  people: [],
  follows: [],
  posts: [],
  interactions: [],
  restrictions: [],
  lists: [],
  drafts: [],
  announcements: [],
  events: [],
  emojis: [],
  conversationReads: [],
};

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

/** The domain of a person's server: theirs after the `@` of their handle, else this server's. */
const domainOf = (username: string) => (username.includes("@") ? username.slice(username.indexOf("@") + 1) : undefined);

/** The host the fake answers as. */
export const fakeHost = "demo.test";
/** The origin the fake answers as. */
export const fakeOrigin = `https://${fakeHost}`;

/** The origin of a person's server. */
const originOf = (username: string) => `https://${domainOf(username) ?? fakeHost}`;

/** Who may read a public timeline (the API's InstanceAudience). */
export type Audience = Schema<"InstanceAudience">;

export type FakeOptions = {
  /**
   * What the server holds in place of the scenario: the lists it names, and nothing else. The options that name the
   * scenario's people (`approvals`, `held`, `everyKind`, `singleUser`) need them.
   */
  seed?: Partial<FakeSeed>;
  /** The username of the person who reads; yuki by default. Their roles (the seed's `staff`) set their rights. */
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
  /** The closure as its administrators run it (closure.ts): its date passed, a short notice, delivered, warnings. */
  closureAdmin?: FakeClosureOptions;
  /** The instance theme as its administrators edit it (theme.ts): nothing by default, a draft, or it published. */
  theme?: FakeThemeOptions;
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
  /**
   * The server shows the summaries of its rooms to visitors (the instance setting `voice_public_summary`): a room's
   * page then names it, and while it is open its description and host.
   */
  voiceSummaries?: boolean;
  /**
   * Who may open voice chat rooms (the instance setting `voice_room_starters`): every local account by default, or
   * only those whose roles have `start_voice_rooms` (`permitted`), which no person of the scenario has.
   */
  voiceStarters?: "everyone" | "permitted";
  /** Why the server refuses every opening of a room: the reader may not, too many are open, or they host another. */
  voiceOpen?: FakeVoiceOpenRefusal;
  /** More open rooms, rooms the reader hosted, and invitations to the reader's room than a page holds. */
  voiceMany?: FakeVoiceMany;
  /**
   * Who may create events (the instance setting `event_creators`): every local account by default, or only those
   * whose roles have `create_events` (`permitted`), as the admin's do.
   */
  eventCreators?: "everyone" | "permitted";
  /**
   * The reader's exports of their data: none by default; one being built (`building`), one ready to download
   * (`ready`), one being built over an earlier one that is ready (`rebuilding`), one that could not be built
   * (`failed`), or one whose archive is past the day the server keeps it until and not yet removed (`expired`).
   */
  dataExport?: "none" | "building" | "ready" | "rebuilding" | "failed" | "expired";
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
    /**
     * The server sends a report's text to its provider for the moderators (`translation_reports_to_server`): the
     * provider then receives private posts too, as a report's comment counts.
     */
    reports?: boolean;
  };
  /**
   * How many items a page of the staff's lists (under /admin) holds when the request names no `limit`: the server's 20
   * by default; fewer, so that a story or a test reaches the second page of a list the fake holds a few of.
   */
  staffPage?: number;
  /**
   * The moderators' queue of reports (reports.ts): the fake's own reports by default, or none; `crowded`, kai reported
   * 24 times more as well, so that the queue and kai's reports run past a page.
   */
  reports?: FakeReportOptions;
  /** The domain actions, the allowlist, the subscriptions, the servers and the audit log (domains.ts), or none. */
  domains?: FakeDomainOptions;
  /** The administration's relays, login providers and OAuth apps (admin-settings.ts): the fake's, or none. */
  admin?: FakeAdminOptions;
  /**
   * The moderation of accounts (accounts.ts): the fake's own actions, appeals, registrations, decisions of the hooks,
   * sign-up blocks and invites by default, or none of them; `crowded`, kai with 30 sign-ins and 24 accounts of the
   * fake's own that signed up from kai's address as well, so that the account search and kai's lists run past a page.
   */
  moderation?: FakeModerationOptions;
  /**
   * The operations (operations.ts): the fake's jobs, error groups and rules of the link card crawler by default, or
   * none, or `crowded` (more rules and an error group's hours than a screen shows at once); `uxMeasurement`, the
   * instance measures the web client (`ux_measurement`), whose samples the fake then holds.
   */
  operations?: FakeOperationsOptions;
  uxMeasurement?: boolean;
  /** The keys of the posts the reader bookmarked besides the seed's, the newest bookmark first. */
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
   * The follow requests to the reader: those of the seed (sam's to ren) and, after them, of the people named in
   * `from`, the last the newest; their list fails (`fails`, a 500) or never answers (`waits`).
   */
  followRequests?: { from?: readonly string[]; answer?: "fails" | "waits" };
  /** The reader's private notes on people at the start, by username. */
  notes?: Readonly<Record<string, string>>;
  /** The people whose new posts notify the reader at the start. */
  subscriptions?: readonly string[];
  /**
   * The reader's personal settings at the start, in place of `defaultSettings` for the members given (settings.ts):
   * their keyword filters, badges, featured hashtags, notification policy, the rows of their notification settings
   * (`memberNotificationRows` are a member's, `moderatorNotificationRows` what a moderator gets besides), auto-delete
   * policy and its maximum, imports,
   * aliases, why the account cannot be deactivated, and the moderators' actions with their appeals. The filters in
   * effect apply to what the reader reads, as on the server (ADR-0157 §6): in the contexts a filter names (the home and
   * list timelines and the stream's posts, the notifications, the public and hashtag timelines, a thread, a profile's
   * posts), a post of another that holds one of its words is left out by `hide`, or carries the filter in its
   * `filtered` by `warn`; the post read alone is folded by either.
   */
  settings?: Partial<FakeSettings>;
  /** The domains the reader mutes at the start. */
  domainMutes?: readonly string[];
  /** The hashtags the reader follows at the start, by name, the first followed first. */
  followedTags?: readonly string[];
  /**
   * The reader's preferences at the start, where they are not the server's defaults for a new account (the language
   * and the time zone follow the browser, new posts go into the timelines by themselves, large link cards): the ones
   * named here, as GET /preferences answers them. The reader's changes (PATCH /preferences) are kept.
   */
  preferences?: Partial<Schema<"Preferences">>;
  /**
   * The reader's preferences are not read (`read`: GET /preferences fails, a 500) or not saved (`save`: PATCH
   * /preferences fails, a 500, and changes nothing).
   */
  preferencesFail?: "read" | "save";
  /**
   * The server itself requires a description of every media before it is posted (its limit
   * `require_media_description`), whatever the reader's own preference; it does not by default.
   */
  requiresDescriptions?: boolean;
  /**
   * In place of the seed's notifications, one of each kind of a group the server sends: from people (`people`), the
   * server's notices to the reader (`notices`), or those to its moderators (`moderators`), each a page at most.
   */
  everyKind?: "people" | "notices" | "moderators";
  /**
   * The notifications the reader's notification policy holds back (ADR-0158 §7), by account: a boost and a favourite
   * of kiri@far.example, and a follow of kobo@bread.example, the newest first; accepting or dismissing them is kept.
   */
  held?: boolean;
  /** With `held`, a follow of each of these people held back too, older than the others: more than a page of accounts. */
  heldFrom?: readonly string[];
  /**
   * Web Push (FR-206): the server sends it (its application key), and the reader has no subscription until they
   * register one; none by default, as a server without a VAPID key.
   */
  push?: boolean;
  /** The server sends email (the instance's `sends_email`); it does not by default. */
  email?: boolean;
  /**
   * The verification email (POST /accounts/email/verification), which the fake queues (a 202) unless the server sends
   * no email (a 404): `limited`, the server sent as many as it sends for now and answers a 429 whose `Retry-After`
   * asks for 25 minutes.
   */
  verificationEmail?: "limited";
  /**
   * With `push`: the reader holds a subscription already (`subscribed`), or the read of it (`subscription-fails`) or of
   * their notification settings (`settings-fail`) fails with a 500.
   */
  pushState?: "subscribed" | "subscription-fails" | "settings-fail";
  /** How many of the reader's newest notifications they have not read yet; 3 by default. */
  unreadNotifications?: number;
  /** The first dismissal of a notification fails (a 500); the next is kept, as a try again finds it. */
  dismissFails?: boolean;
  /**
   * An upload is still processed when the server answers it, and ready when it is read again (GET /media/{id}), as on
   * a server that processes it meanwhile; ready at once by default. `refused`: every upload is refused as a
   * file the server cannot read as an image (a 422 of the type `media-corrupt`, the server's words its title).
   */
  uploads?: "processing" | "refused";
  /**
   * How the reader's account stands in the settings (access.ts): its password, passkeys, two-factor authentication,
   * linked accounts, signed-in browsers, authorized apps, own OAuth apps and invites; a typical account by default.
   */
  access?: AccessOptions;
  /**
   * The token revocation, which the app signs out through (POST /oauth/revoke, no operation of the client API), fails
   * (`fails`, a 503) and ends nothing; by default it answers 200 and the fake keeps it (`revocations`).
   */
  revocation?: "fails";
  /**
   * What the instance states of search engines and of AI (ADR-0057 §1), each `allow` by default: an account cannot
   * allow what the instance denies.
   */
  crawlers?: { search?: Schema<"SearchIndexing">; ai?: Schema<"AIUsage"> };
  /**
   * The reader's own profile: it starts with no name, text, fields or pictures (`start: "empty"`), and a change of it
   * (PATCH /accounts/update_credentials) is refused for the value of its display name, which the problem names
   * (`save: "name"`), refused by the server's review (`save: "refused"`), or never answered (`save: "waits"`).
   */
  profile?: { start?: "empty"; save?: "name" | "refused" | "waits" };
  /**
   * The supporter program (supporters.ts): off by default; given, on with two plans, and the reader's subscription,
   * the tips, the supporters list and the reader's place on it as it says.
   */
  supporters?: FakeSupporterOptions;
  /**
   * What the instance says on its about page (about.ts): its summary, rules, status page, terms and privacy policy by
   * default; everything the page shows, or nearly nothing; and who may read the domains it moderates.
   */
  about?: FakeAboutOptions;
  /**
   * The finance (finance.ts): what the instance publishes of its costs and income (nothing by default), estimates,
   * the funding meter, and the reader as a holder of `view_finance`.
   */
  finance?: FakeFinanceOptions;
  /**
   * The administration of roles, custom emoji, announcements and webhooks (staff-server.ts): the roles the reader
   * holds in place of the scenario's, and what each area starts with.
   */
  staffServer?: FakeStaffServerOptions;
  /** The administrators' finance (finance-admin.ts): the cost items, the totals, the income, the billing imports. */
  financeAdmin?: FakeFinanceAdminOptions;
  /** The administrators' supporter program (supporters-admin.ts): the program, the tips, the plans, the subscriptions. */
  supportersAdmin?: FakeSupportersAdminOptions;
};

/** Whom an interaction policy admits (the API's InteractionAudience). */
type PolicyAudience = Schema<"InteractionAudience">;

/** Who may reply to, boost and quote a post: at once, and by its author's approval (the API's InteractionRule). */
type PolicyRule = Schema<"InteractionRule">;

type Policy = Partial<Record<"reply" | "announce" | "quote", PolicyRule>>;

/** A post of `approvals`: its policy, and whether the author of the post it replies to declined it. */
type ApprovalPost = Post & {
  policy?: Policy;
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

/** An event of the stream as mykit reads it, which `send` writes as the server does. */
export type FakeStreamEvent = Exclude<StreamEvent, { type: "unknown" }>;

export type FakeApi = {
  /** Answers a request of the client API, as a `fetch`; one for an operation the fake does not implement throws. */
  serve: (request: Request) => Promise<Response>;
  /**
   * Scripts the operation of `route` (`POST /statuses`, `GET /statuses/{id}`) from now on: its requests are counted
   * and kept, and its answer held, failed or changed as `script` says.
   */
  script: <R extends Route>(route: R, script?: Script<R>) => Scripted<R>;
  /** Sends `event` on every open stream (GET /streaming). */
  send: (event: FakeStreamEvent) => void;
  /** The reader's call: the rooms' IDs, their session, and what its room stream says (voice.ts). */
  readonly voice: FakeVoice;
  /** The events' IDs by the seed's keys, and an event as the reader sees it now (events.ts). */
  readonly events: FakeEvents;
  /** The plans' and the tips' IDs, and the reader's subscription as the server holds it now (supporters.ts). */
  readonly supporters: FakeSupporters;
  /** The reports' IDs by the fake's keys, the open ones, and a report filed by a person of the fake (reports.ts). */
  readonly reports: FakeReports;
  /** The domain actions' and the subscriptions' IDs by the fake's keys, and the audit log (domains.ts). */
  readonly domains: FakeDomains;
  /** The IDs of the custom emoji, announcements and webhooks of the administration by their keys (staff-server.ts). */
  readonly staffServer: FakeStaffServer;
  /** The IDs of the cost items by the fake's keys (finance-admin.ts). */
  readonly financeAdmin: FakeFinanceAdmin;
  /** The IDs of the plans and the subscriptions by the fake's keys (supporters-admin.ts). */
  readonly supportersAdmin: FakeSupportersAdmin;
  /** The IDs of the rules, relays and login providers of the administration's fake (admin-settings.ts). */
  readonly admin: FakeAdmin;
  /** The IDs of the moderation's actions, appeals, registrations and the rest by the fake's keys (accounts.ts). */
  readonly moderation: FakeModeration;
  /** The IDs of the operations' jobs and error groups by the fake's keys (operations.ts). */
  readonly operations: FakeOperations;
  /** The requests for an operation the fake does not implement, each as `METHOD /path`, which `serve` refused. */
  readonly unimplemented: readonly string[];
  /** The requests of a visitor (`visitor`) for an operation that needs a session, each as `METHOD /path`, answered 401. */
  readonly refused: readonly string[];
  /**
   * The token revocations the server took (POST /oauth/revoke), each as the form it was sent with: the client
   * (`client_id`) and the token it names. One it took ended the reader's session.
   */
  readonly revocations: readonly { readonly clientId: string; readonly token: string }[];
  /**
   * Adds `count` posts to the home timeline, sends them on the stream as the reader's keyword filters leave them, and
   * returns them, newest first.
   */
  newPosts: (count: number) => Status[];
  /** Adds a notification (a favourite of the reader's post), sends it on the stream, and returns it. */
  newNotification: () => Notification;
  /**
   * Adds a direct post to the reader's conversation at `index` of their direct messages (newest first, the hidden ones
   * too), by its first other person in reply to its latest post, sends the conversation on the stream, and returns
   * the post.
   */
  newMessage: (index: number) => Status;
  /** The app's address of the post with `key`. */
  postPath: (key: string) => string;
  /** The app's address of the seed's announcement with `key`, or of the fake's one that ended (`ended`). */
  announcementPath: (key: string) => string;
  /** The app's address of the reader's list at `index`, oldest first. */
  listPath: (index: number) => string;
  /** The app's address of the editor of the reader's keyword filter at `index`, oldest first. */
  filterPath: (index: number) => string;
  /** The app's address of the moderators' action on the reader's account at `index`, newest first. */
  actionPath: (index: number) => string;
  /** The ID of the seed's post of `key`, or of the reader's draft of that key. */
  idOf: (key: string) => string;
  /** The ID of the account of the seed's person of `username`. */
  accountId: (username: string) => string;
  /** The app's address of the reader's conversation at `index` of their direct messages, newest first. */
  conversationPath: (index: number) => string;
  /** The app's address of the held-back notifications of the person of `username` (`held`). */
  heldPath: (username: string) => string;
  /** The app's address of the reader's newest notification of `type`: where a push or an email about it leads. */
  notificationPath: (type: string) => string;
  /** The ID of the fake's login provider of `kind`, as a provider's return names it in the address's fragment. */
  loginProviderId: (kind: FakeLoginProviderKind) => string;
  /**
   * The ID of the fake's app at `index` of the apps the reader authorized (`authorized`: the server's web client, an
   * app that registered itself, one known by its metadata document, one a user registered) or of the apps the reader
   * registered (`own`: a confidential, a public and a disabled one); an ID that names no app past them.
   */
  appId: (list: "authorized" | "own", index: number) => string;
  /**
   * The ID of the reader's invite at `index` of their list as it is by default (two that work, the second used once,
   * a revoked one, an expired one, one used up, one whose creator was not eligible), then of the twenty older ones of
   * `access.invites: "many"`; an ID that names none past them.
   */
  inviteId: (index: number) => string;
};

/** An answer that never comes. */
const never = () => new Promise<never>(() => undefined);

/** The IDs of the reader's uploads a body in Markdown shows as images (`![alt](media:ID)`), each once, in order. */
const bodyImages = (text: string) =>
  [...new Set([...text.matchAll(/!\[[^\]]*\]\(media:([^)\s]+)\)/g)].map((x) => x[1] ?? ""))].toSorted();

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

/** The hashtags of a text, as written. */
const hashtagsIn = (text: string) => [...text.matchAll(/#([\p{L}\p{M}\p{N}_]+)/gu)].map((x) => x[1] ?? "");

/** What an event of the stream is on the wire: its name, and its data. */
function wire(event: FakeStreamEvent): [string, unknown] {
  switch (event.type) {
    case "update":
      return ["update", event.status];
    case "list_update":
      return ["list_update", { list_id: event.listId, status: event.status }];
    case "notification":
      return ["notification", event.notification];
    case "conversation":
      return ["conversation", event.conversation];
    case "announcement":
      return ["announcement", event.announcement];
    case "notification_requests":
      return ["notification_requests", event.summary];
    default:
      return [event.type, { id: event.id }];
  }
}

/** Creates a fake of the client API as `viewer` reads it. */
export function createFakeApi(o: FakeOptions = {}): FakeApi {
  const seed: FakeSeed = o.seed === undefined ? scenario : { ...nothing, ...o.seed };
  const { people, follows, restrictions } = seed;
  const personOf = (username: string): FakePerson => {
    const found = people.find((p) => p.username === username);
    if (found === undefined) throw new Error(`fake: no person ${username}`);
    return found;
  };
  /** The handle of the account a person moved to, if they moved. */
  const movedOf = (username: string) => people.find((p) => p.username === username)?.movedTo;
  /** The usernames a text mentions, among the people. */
  const mentioned = (text: string) =>
    [...text.matchAll(/@(\w+)/g)].map((m) => m[1] ?? "").filter((u) => people.some((p) => p.username === u));

  const customEmojis: Schema<"CustomEmoji">[] = seed.emojis.map((e) => {
    const url = drawingURL(e.drawing);
    return { shortcode: e.shortcode, url, static_url: url };
  });
  /** The custom emoji as the picker lists them: those without a category first, then by category and shortcode. */
  const pickerEmojis: Schema<"PickerCustomEmoji">[] = seed.emojis
    .map((e) => {
      const url = drawingURL(e.drawing);
      return exact<Schema<"PickerCustomEmoji">>()({
        shortcode: e.shortcode,
        url,
        static_url: url,
        aliases: [...(e.aliases ?? [])],
        ...(e.category === undefined ? {} : { category: e.category }),
      });
    })
    .toSorted(
      (a, b) =>
        Number(a.category !== undefined) - Number(b.category !== undefined) ||
        (a.category ?? "").localeCompare(b.category ?? "") ||
        a.shortcode.localeCompare(b.shortcode),
    );
  /** A reaction's emoji as the seed writes it (`:shortcode:` for a custom one) from what a request names. */
  const seedEmoji = (name: string) => {
    const bare = name.replace(/^:|:$/g, "");
    return customEmojis.some((e) => e.shortcode === bare) ? `:${bare}:` : name;
  };

  const viewer = personOf(o.viewer ?? "yuki").username;
  /** The reader has no session: the server answers them what anyone may read, with nothing of a reader's own. */
  const visitor = o.visitor === true;
  const posts: ApprovalPost[] = [...seed.posts, ...(o.approvals === true ? approvalPosts : [])];
  const interactions = [...seed.interactions, ...(o.approvals === true ? approvalInteractions : [])];
  const started = Date.now();
  const wait = (ms: number) => new Promise((done) => setTimeout(done, ms));
  /** A later page's delay: none unless `lateMs` asks for one. */
  const late = () => wait(o.lateMs ?? 0);

  // The seed's steps in order, follows, then posts, then interactions, a few minutes apart up to now.
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
   * The follows a person's counts take in beyond the seed's people (with people of servers the seed's federates
   * with none), the fake's own: an account whose lists its owner keeps (FR-311) still shows how many they hold.
   */
  const beyond: Readonly<Record<string, { followers: number; following: number }>> = {
    mio: { followers: 48, following: 35 },
    kai: { followers: 127, following: 64 },
    "sora@photos.example": { followers: 1284, following: 312 },
    // Its followers moved with it; it still follows those it did.
    "sora@social.example": { followers: 3, following: 298 },
  };
  /**
   * Who may read a person's lists of follows and followers (FR-311), the fake's own, since the seed sets none: mio
   * shows them to mio's followers only, kai to nobody, everyone else to everyone.
   */
  const followsVisibility: Readonly<Record<string, Schema<"FollowsVisibility">>> = {
    mio: "followers",
    kai: "private",
  };
  const hidden = (author: string) =>
    !visitor && (blocking.has(`${viewer}>${author}`) || muting.has(`${viewer}>${author}`));

  // Every account the server answers is a whole one, as the client API's Account is: a post's author too, which the
  // client shows a profile from at once.
  const accountOf = (username: string): Account => fullAccount(personOf(username));

  /** The reader's personal settings (settings.ts), which start as `defaultSettings` with the options' members. */
  const personal = createSettings({
    started,
    origin: fakeOrigin,
    password: personOf(viewer).password,
    viewerId: visitor ? undefined : idOf(viewer),
    accountExists: (id) => usernameOf.has(id),
    accountId: (username) => accountId.get(username),
    ownStatuses: () => posts.filter((p) => p.author === viewer && visible(p)).map((p) => statusOf(p)),
    settings: { ...defaultSettings, ...o.settings },
  });
  const shownBadges = personal.shownBadges;
  /** The statuses of a read as the reader's keyword filters leave them in a context (settings.ts). */
  const judged = personal.judged;

  /** What the reader changed of their own profile (PATCH /accounts/update_credentials); the seed's person otherwise. */
  const edit: {
    displayName?: string;
    note?: string;
    fields?: [string, string][];
    avatar?: string | null;
    header?: string | null;
    locked?: boolean;
    bot?: boolean;
    discoverable?: boolean;
    indexable?: boolean;
    aiOptOut?: boolean;
    feedEnabled?: boolean;
    followsVisibility?: Schema<"FollowsVisibility">;
  } = o.profile?.start === "empty" ? { displayName: "", note: "", fields: [], avatar: null, header: null } : {};

  const fullAccount = (p: FakePerson): Account => {
    const remote = domainOf(p.username);
    const own = p.username === viewer && !visitor;
    const username = remote === undefined ? p.username : p.username.slice(0, p.username.indexOf("@"));
    // The reader's own account moves, and comes back, by the settings (settings.ts); another's by the seed.
    const moved = own ? usernameOf.get(personal.movedTo() ?? "") : movedOf(p.username);
    return exact<Account>()({
      id: idOf(p.username),
      username,
      acct: p.username,
      domain: remote ?? fakeHost,
      display_name: own ? (edit.displayName ?? p.displayName) : p.displayName,
      emojis: [],
      ...(own && edit.avatar !== undefined
        ? edit.avatar === null
          ? {}
          : { avatar: variants(edit.avatar) }
        : { avatar: variants(drawingURL(p.avatar)) }),
      uri: `${originOf(p.username)}/users/${username}`,
      url: `${originOf(p.username)}/@${username}`,
      note: serverHtml(own ? (edit.note ?? p.note) : p.note, remote ?? fakeHost),
      locked: own ? (edit.locked ?? p.locked ?? false) : (p.locked ?? false),
      bot: own ? (edit.bot ?? false) : false,
      created_at: new Date(started - 30 * day).toISOString(),
      fields: (own ? (edit.fields ?? p.fields) : p.fields).map(([name, value]) => ({
        name,
        value: serverHtml(value, fakeHost),
      })),
      badges: own ? shownBadges() : [],
      ...(own && edit.header !== undefined
        ? edit.header === null
          ? {}
          : { header: variants(edit.header) }
        : p.header === undefined
          ? {}
          : { header: variants(drawingURL(p.header)) }),
      // The account it moved to, without a move of its own: moves are not followed in chains.
      ...(moved === undefined ? {} : { moved: fullAccount(personOf(moved)) }),
      followers_count:
        people.filter((x) => follows_(x.username, p.username)).length + (beyond[p.username]?.followers ?? 0),
      following_count:
        people.filter((x) => follows_(p.username, x.username)).length + (beyond[p.username]?.following ?? 0),
    });
  };

  /** The reader's private notes on people, by username, and the people whose new posts notify them. */
  const notes = new Map<string, string>(Object.entries(o.notes ?? {}));
  const subscribed = new Set<string>(o.subscriptions ?? []);

  const relationshipOf = (id: string): Schema<"Relationship"> => {
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

  /** The domains the reader mutes, in the order they muted them, and whether a person's server is one of them. */
  const domainMutes: Schema<"DomainMute">[] = (o.domainMutes ?? []).map((domain) => ({
    domain,
    created_at: new Date(started).toISOString(),
  }));
  const mutedServer = (username: string) => {
    const domain = domainOf(username);
    return domain !== undefined && domainMutes.some((m) => domain === m.domain || domain.endsWith(`.${m.domain}`));
  };

  // ---- Posts --------------------------------------------------------------------------------------------------------

  const postStep = (key: string) => follows.length + posts.findIndex((p) => p.key === key);
  const statusId = new Map(posts.map((p) => [p.key, idAt(timeOf(postStep(p.key)), postStep(p.key))]));
  const keyOf = new Map([...statusId].map(([k, id]) => [id, k]));
  const postOf = (key: string): ApprovalPost => {
    const p = posts.find((x) => x.key === key);
    if (p === undefined) throw new Error(`fake: no post ${key}`);
    return p;
  };

  // ---- Approvals (ADR-0155) -----------------------------------------------------------------------------------------

  type Kind = Schema<"InteractionKind">;
  /** Whether `audience` of `p` admits `who`: its author always, the mentioned in any but nobody, as the server does. */
  const admits = (audience: PolicyAudience, p: Post, who: string) =>
    who === p.author ||
    audience === "everyone" ||
    (audience === "followers" && follows_(who, p.author)) ||
    (audience !== "nobody" && mentioned(p.text).includes(who));
  /** Whether `who` may do `kind` with the post `p` of `policy`: at once, once its author approves, or not. */
  const permitted = (policy: Policy, p: Post, kind: Kind, who: string): Schema<"InteractionPermission"> => {
    const rule = policy[kind] ?? { automatic: "everyone" };
    if (admits(rule.automatic, p, who)) return "allowed";
    return rule.manual !== undefined && admits(rule.manual, p, who) ? "pending" : "denied";
  };
  /** The same for a post: none for one of no policy. */
  const permissionOf = (p: ApprovalPost, kind: Kind, who: string) =>
    p.policy === undefined ? undefined : permitted(p.policy, p, kind, who);
  /**
   * A request of `by` to reply to (`via` the reply), boost or quote (`via` the quoting post) the post of `target`, at
   * the step of what asks; one the seed says the author declined is so from the start.
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

  /** The reader's boosts as they set them, by the key of the post, over the seed's. */
  const ownBoost = new Map<string, boolean>();
  /** The reader's boosts made here that wait for the approval of the post's author, by the key of the post. */
  const waitingBoosts = new Set<string>();
  /** The interactions of `kind` with the post of `key`; a boost only once it needs no approval or has it. */
  const on = (key: string, kind: string) =>
    interactions.filter((i) => i.post === key && i.kind === kind && !(i.kind === "boost" && boostWaits(i)));
  /** Who boosted the post of `key`, in order. */
  const boostersOf = (key: string) => [
    ...on(key, "boost").flatMap((i) => (i.by === viewer && ownBoost.has(key) ? [] : [i.by])),
    ...(ownBoost.get(key) === true ? [viewer] : []),
  ];

  // The reader's favourite and reaction as they set them, by the key of the post, over the seed's: one or the
  // other, a reaction's emoji as the seed writes it, or none (null).
  const ownFavourite = new Map<string, boolean>();
  const ownReaction = new Map<string, string | null>();
  const mineSet = (key: string) => ownFavourite.has(key) || ownReaction.has(key);
  /** The threads the reader muted (ADR-0158 §8), by the key of the post that starts each. */
  const mutedThreads = new Set<string>();
  const threadOf = (key: string): string => {
    const parent = postOf(key).replyTo;
    return parent === undefined || !posts.some((p) => p.key === parent) ? key : threadOf(parent);
  };
  /** The keys of the posts the reader bookmarked, the newest bookmark first: the options', then the seed's. */
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
  const lists: { id: string; name: string; members: string[]; created_at: string }[] = seed.lists
    .filter((l) => l.owner === viewer)
    .map((l, n) => ({
      id: idAt(0, 60_000 + n),
      name: l.name,
      members: [...l.members],
      created_at: new Date(started - (30 - n) * day).toISOString(),
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
    problem(422, "The list could not be saved", {
      type: "/api/v1/problems/validation",
      field: "name",
      detail: "Write a name of 1 to 50 characters on one line.",
    });
  const listBody = (l: (typeof lists)[number]): Schema<"List"> => ({
    id: l.id,
    name: l.name,
    members_count: l.members.length,
    created_at: l.created_at,
  });
  /** The hashtags the reader follows, by their names in lowercase. */
  const followedTags = new Set<string>(o.followedTags?.map((name) => name.toLowerCase()));
  /** The hashtags of a text, in lowercase, as the server reads them. */
  const hashtagsOf = (text: string) => hashtagsIn(text).map((name) => name.toLowerCase());
  /** A hashtag as first written in a post, else as asked, and whether the reader follows it. */
  const tagOf = (asked: string): Schema<"Hashtag"> => {
    const key = asked.toLowerCase();
    const written = posts.flatMap((p) => hashtagsIn(p.text)).find((name) => name.toLowerCase() === key);
    return {
      name: written ?? asked,
      url: `${fakeOrigin}/tags/${encodeURIComponent(key)}`,
      following: followedTags.has(key),
    };
  };
  /** The hashtags of a status, as its text and its article name them, each once. */
  const statusTags = (text: string, more: readonly string[] = []): Schema<"StatusTag">[] =>
    [...new Map([...hashtagsIn(text), ...more.map((t) => t.replace(/^#/, ""))].map((t) => [t.toLowerCase(), t]))].map(
      ([key, name]) => ({ name, url: `${fakeOrigin}/tags/${encodeURIComponent(key)}` }),
    );
  /** The people a status mentions, as the API's StatusMention. */
  const mentionsOf = (text: string): Schema<"StatusMention">[] =>
    mentioned(text).map((u) => ({ id: idOf(u), username: u, acct: u, url: `${fakeOrigin}/@${u}` }));
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
  const reactionEmojiOf = (emoji: string): Schema<"ReactionEmoji"> => {
    const custom = customEmojis.find((e) => `:${e.shortcode}:` === emoji);
    return custom === undefined
      ? { name: emoji }
      : { name: custom.shortcode, shortcode: custom.shortcode, emoji: custom };
  };

  /** The reader's votes, by the key of the post, as the seed has them and as the reader votes. */
  const ownVotes = new Map<string, readonly number[]>();
  for (const i of interactions) if (i.kind === "vote" && i.by === viewer) ownVotes.set(i.post, i.choices);

  /**
   * The poll of `p` as the reader sees it, open for the days the seed gives it from the start of the fake; a visitor's
   * says nothing of a vote of their own.
   */
  function pollOf(p: Post): Schema<"Poll"> | undefined {
    if (p.poll === undefined) return undefined;
    const own = ownVotes.get(p.key);
    const votes = [
      ...interactions.flatMap((i) => (i.kind === "vote" && i.post === p.key && i.by !== viewer ? [i.choices] : [])),
      ...(own === undefined ? [] : [own]),
    ];
    return exact<Schema<"Poll">>()({
      id: idAt(timeOf(postStep(p.key)), 60_000 + postStep(p.key)),
      expires_at: new Date(started + p.poll.days * day).toISOString(),
      expired: false,
      multiple: p.poll.multiple ?? false,
      votes_count: votes.reduce((n, c) => n + c.length, 0),
      voters_count: votes.length,
      options: p.poll.options.map((title, i) => ({ title, votes_count: votes.filter((c) => c.includes(i)).length })),
      ...(visitor ? {} : { voted: own !== undefined, own_votes: [...(own ?? [])] }),
    });
  }

  // The reader's own posts they deleted, by key, and the versions of each post: those of the seed (its `earlier`,
  // then its text), then the reader's edits.
  const deleted = new Set<string>();
  type Version = { text: string; spoiler: string; at: number; media?: readonly string[] };
  const edits = new Map<string, Version[]>();
  /** The versions of `p`, oldest first: what it was posted with first, the last what it says now. */
  function versionsOf(p: Post): readonly Version[] {
    const posted = timeOf(postStep(p.key));
    const first = [...(p.earlier ?? []), p.text].map((text, i) => ({
      text,
      spoiler: p.spoiler ?? "",
      at: posted + i * 2 * minute,
    }));
    return [...first, ...(edits.get(p.key) ?? [])];
  }

  const centered = { focus: { x: 0, y: 0 } };
  /** An image of the seed as an attachment of `id`: its drawing served whole and as its preview, with its blurhash. */
  const imageOf = (id: string, image: Image): MediaAttachment => {
    const url = drawingURL(image.drawing);
    const { width, height } = image.drawing;
    return {
      id,
      type: "image",
      state: "ready",
      description: image.description,
      url,
      preview_url: url,
      blurhash: drawingBlurhash(image.drawing),
      meta: { width, height, ...centered },
    };
  };

  /**
   * A post's attachments as a version of it holds them (all of them unless it names which), as the server gives them,
   * its images then the rest: a picture's drawing; a video's or a GIF's poster as its preview, with no file the fake
   * could play; a sound with its length and no file; and a file of another kind as a link to where it would be.
   */
  const mediaOf = (p: Post, id: string, version: Version): MediaAttachment[] => {
    const images = (p.images ?? []).map((image, i) => imageOf(`${id}-${i}`, image));
    const rest = (p.attachments ?? []).map((a, n): MediaAttachment => {
      const base = {
        id: `${id}-${images.length + n}`,
        state: "ready" as const,
        ...(a.description === "" ? {} : { description: a.description }),
      };
      const attachment = exact<MediaAttachment>();
      if (a.kind === "audio") return attachment({ ...base, type: "audio", meta: { duration: a.seconds, ...centered } });
      if (a.kind === "unknown")
        return attachment({ ...base, type: "unknown", url: `${fakeOrigin}/media/${a.name}`, meta: centered });
      const { width, height } = a.poster;
      return attachment({
        ...base,
        type: a.kind,
        preview_url: drawingURL(a.poster),
        blurhash: drawingBlurhash(a.poster),
        meta: exact<Schema<"MediaMeta">>()({
          width,
          height,
          ...(a.seconds === undefined ? {} : { duration: a.seconds }),
          ...centered,
        }),
      });
    });
    return [...images, ...rest].filter((m) => version.media === undefined || version.media.includes(m.id));
  };

  /** The card of a post's first link, as the server makes it from the page `text` links to. */
  const cardOf = (card: LinkCard, text: string): Schema<"LinkCard"> =>
    exact<Schema<"LinkCard">>()({
      url: /https?:\/\/\S+/.exec(text)?.[0] ?? fakeOrigin,
      title: card.title,
      description: card.description,
      site_name: card.site,
      ...(card.language === undefined ? {} : { language: card.language }),
      ...(card.image === undefined
        ? {}
        : {
            image: exact<Schema<"LinkCardImage">>()({
              url: drawingURL(card.image.drawing),
              width: card.image.drawing.width,
              height: card.image.drawing.height,
              blurhash: drawingBlurhash(card.image.drawing),
              ...(card.image.description === undefined ? {} : { description: card.image.description }),
            }),
          }),
      ...(card.author === undefined ? {} : { author: accountOf(card.author) }),
    });

  // A reply waiting for its approval, or refused it, shows to its author alone; a visitor sees what anyone may, which
  // leaves out a post for this server's people (`local`).
  const visible = (p: Post) => {
    const own = !visitor && p.author === viewer;
    return (
      !deleted.has(p.key) &&
      (own || replyApproval(p.key) === undefined) &&
      (own ||
        p.visibility === "public" ||
        p.visibility === "unlisted" ||
        (!visitor && p.visibility === "local") ||
        (!visitor && p.visibility === "private" && follows_(viewer, p.author)) ||
        (!visitor && p.visibility === "direct" && mentioned(p.text).includes(viewer)))
    );
  };

  /** How many replies the reader posted here to each status that count on it, by its ID: those that wait for no approval. */
  const repliesMade = new Map<string, number>();

  /** The title, summary and hashtags of the seed's articles as the reader edited them, by the key of each. */
  const articleEdits = new Map<string, { title: string; summary: string; tags: readonly string[] }>();

  /** An article's title, summary and cover, as the API's StatusArticle. */
  const articleOf = (p: Post, id: string): Schema<"StatusArticle"> | undefined => {
    const a = p.article;
    if (a === undefined) return undefined;
    const edited = articleEdits.get(p.key);
    return exact<Schema<"StatusArticle">>()({
      title: edited?.title ?? a.title,
      summary: edited?.summary ?? a.summary,
      ...(a.cover === undefined ? {} : { cover: imageOf(`${id}-cover`, a.cover) }),
    });
  };

  /**
   * A post as the API's Status; an article with its body only when `full`, as a status read alone gives it, and with
   * none in a list (ADR-0160 §8). A visitor's has none of a reader's own marks, as the server leaves them out of an
   * anonymous answer: whether they favourited, boosted, bookmarked, muted, pinned or reacted, their approvals, and
   * what they may do with it.
   */
  function statusOf(p: ApprovalPost, depth = 0, full = false): Status {
    const id = statusId.get(p.key) ?? "";
    const approval = !visitor && p.author === viewer ? replyApproval(p.key) : undefined;
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
    const boosters = boostersOf(p.key);
    const poll = pollOf(p);
    const local = p.author.split("@")[0] ?? "";
    return exact<Status>()({
      id,
      uri: `${originOf(p.author)}/users/${local}/statuses/${id}`,
      url: `${originOf(p.author)}/@${local}/${id}`,
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
        : { in_reply_to_id: statusId.get(parent.key) ?? "", in_reply_to_account_id: idOf(parent.author) }),
      media_attachments: mediaOf(p, id, now),
      // A card is of the text's first link: an edit that takes the link out takes the card with it.
      ...(p.card === undefined || !/https?:\/\/\S+/.test(now.text) ? {} : { card: cardOf(p.card, now.text) }),
      mentions: mentionsOf(now.text),
      tags: statusTags(now.text, articleEdits.get(p.key)?.tags),
      filtered: [],
      emojis: customEmojis,
      replies_count:
        posts.filter((x) => x.replyTo === p.key && replyApproval(x.key) === undefined).length +
        (repliesMade.get(id) ?? 0),
      boosts_count: boosters.length,
      favourites_count: favouriters.length,
      reactions: [...reactions]
        .toSorted(([, a], [, b]) => b.count - a.count)
        .map(([emoji, r]) =>
          exact<Schema<"StatusReaction">>()({
            ...reactionEmojiOf(emoji),
            count: r.count,
            ...(visitor ? {} : { me: r.me }),
          }),
        ),
      ...(visitor
        ? {}
        : {
            favourited: favouriters.includes(viewer),
            boosted: boosters.includes(viewer),
            bookmarked: bookmarks.includes(p.key),
            muted: mutedThreads.has(threadOf(p.key)),
          }),
      ...(!visitor && p.author === viewer ? { pinned: p.pinned === true } : {}),
      ...(poll === undefined ? {} : { poll }),
      ...(article === undefined ? {} : { article }),
      ...(p.quote === undefined || depth > 0
        ? {}
        : {
            quote: exact<Schema<"StatusQuote">>()({
              state: quoteState,
              ...(quoteState === "accepted" ? { quoted_status: statusOf(postOf(p.quote), depth + 1) } : {}),
            }),
          }),
      ...(approval === undefined ? {} : { approval }),
      ...(!visitor &&
      (waitingBoosts.has(p.key) ||
        interactions.some((i) => i.kind === "boost" && i.by === viewer && i.post === p.key && boostWaits(i)))
        ? { boost_approval: "pending" as const }
        : {}),
      ...(p.policy === undefined || visitor
        ? {}
        : {
            permissions: {
              reply: permitted(p.policy, p, "reply", viewer),
              announce: permitted(p.policy, p, "announce", viewer),
              quote: permitted(p.policy, p, "quote", viewer),
            },
            ...(p.author === viewer
              ? {
                  interaction_policy: {
                    reply: p.policy.reply ?? { automatic: "everyone" as const },
                    announce: p.policy.announce ?? { automatic: "everyone" as const },
                    quote: p.policy.quote ?? { automatic: "everyone" as const },
                  },
                }
              : {}),
          }),
    });
  }

  /** A boost of `p` by `by`, at the interaction's step. */
  const boostOf = (p: Post, by: string, step: number): Status => {
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
  const homeEntries: { at: number; status: Status }[] = [];
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
  const home: Status[] = homeEntries.toSorted((a, b) => b.at - a.at).map((e) => e.status);
  /** The handle of a status's author, or of a notification's sender; empty for a notification from no one. */
  const authorOf = (s: { account?: Account }) => s.account?.acct ?? "";
  /**
   * Whether a status of home stays there as the reader's blocks and mutes are now: none of whom they block or mute, and
   * of the servers they mute (ADR-0013 §30) a boost of a post leaves, and so does a post, unless the reader follows its
   * author.
   */
  const inHomeNow = (s: Status) => {
    const boosted = s.boost;
    if (hidden(authorOf(s)) || (boosted !== undefined && hidden(authorOf(boosted)))) return false;
    if (boosted !== undefined) return !mutedServer(authorOf(boosted));
    return !mutedServer(authorOf(s)) || follows_(viewer, authorOf(s));
  };

  /**
   * A status of a list as it is now: a post of the seed (or a boost of one) as edited since, none once deleted;
   * another (a post the stream brought, one the reader made here) as it was.
   */
  function current(s: Status): Status[] {
    const key = keyOf.get(s.id);
    if (key !== undefined) return deleted.has(key) ? [] : [statusOf(postOf(key))];
    const boostedKey = s.boost === undefined ? undefined : keyOf.get(s.boost.id);
    if (boostedKey === undefined) return [s];
    return deleted.has(boostedKey) ? [] : [{ ...s, boost: statusOf(postOf(boostedKey)) }];
  }
  /** The home timeline as it reads now. */
  const homeNow = () => home.flatMap(current).filter(inHomeNow);

  /** The seed's post of the status `id` that the reader may see, or none. */
  const shownPost = (id: string) => {
    const key = keyOf.get(id);
    const p = key === undefined ? undefined : postOf(key);
    return p === undefined || !visible(p) ? undefined : p;
  };

  /** A person's posts on their profile, newest first. */
  const profilePosts = (username: string) => posts.filter((p) => p.author === username && visible(p)).toReversed();

  /** The ancestors and descendants of a post, depth first, without what the reader blocks or mutes. */
  function contextOf(key: string): Schema<"StatusContext"> {
    const ancestors: Status[] = [];
    for (let p = postOf(key).replyTo; p !== undefined; p = postOf(p).replyTo) {
      const parent = postOf(p);
      if (visible(parent) && !hidden(parent.author)) ancestors.unshift(statusOf(parent));
    }
    const descendants: Status[] = [];
    const walk = (k: string) => {
      for (const child of posts.filter((x) => x.replyTo === k && visible(x) && !hidden(x.author))) {
        descendants.push(statusOf(child));
        walk(child.key);
      }
    };
    walk(key);
    return { ancestors, descendants };
  }

  // The translation provider of `translation`, with the visibilities a provider receives by default.
  const translationProvider: Schema<"InstanceTranslationProvider"> = {
    id: "fake",
    name: "Demo Translate",
    visibilities: ["public", "local", "unlisted", ...(o.translation?.reports === true ? ["private" as const] : [])],
  };

  /**
   * The answer to translating `p` into `target`, by the provider of `translation`: the post with each text behind the
   * language it goes into, or the refusal the provider stands for. A post whose visibility the provider may not receive
   * is refused as the server refuses it, whatever the provider.
   */
  function translationOf(p: Post, target: string): Answer<"POST /statuses/{id}/translation"> {
    const provider = o.translation?.provider;
    const refuse = (status: number, type: string, title: string, headers: Record<string, string> = {}): Refusal => ({
      ...problem(status, title, { type: `/api/v1/problems/${type}` }),
      headers,
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
    const body: Schema<"StatusTranslation"> = {
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
    };
    return { status: 200, body };
  }

  /** A request as the API's InteractionRequest: what the reply or the quoting post says, none for a boost. */
  const requestBody = (r: AskedApproval): Schema<"InteractionRequest"> =>
    exact<Schema<"InteractionRequest">>()({
      id: requestId(r),
      kind: r.kind,
      state: stateOf(r),
      created_at: new Date(timeOf(r.step)).toISOString(),
      account: accountOf(r.by),
      status: statusOf(postOf(r.target)),
      ...(r.via === undefined ? {} : { interacting_status: statusOf(postOf(r.via)) }),
    });

  // ---- Direct messages ----------------------------------------------------------------------------------------------

  // The reader's conversations: their direct posts by thread, each by its root's key, newest first. One is unread while
  // a post by another is newer than where the reader read it to (the seed's reads, then the reader's), as the
  // server says, whoever wrote the latest; one hidden stays out until a newer post.
  const rootOf = (p: Post): Post => (p.replyTo === undefined ? p : rootOf(postOf(p.replyTo)));
  const conversationIdOf = (root: Post) => idAt(0, 95_000 + posts.indexOf(root));
  const readTo = new Map<string, string>(
    seed.conversationReads
      .filter((r) => r.by === viewer)
      .map((r) => [conversationIdOf(rootOf(postOf(r.post))), statusId.get(r.post) ?? ""]),
  );
  const hiddenTo = new Map<string, string>();
  type HeldConversation = { at: string; hidden: boolean; body: Schema<"Conversation"> };
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
  const conversationOf = (id: string) => conversationsNow().find((c) => c.body.id === id);

  // ---- Notifications ------------------------------------------------------------------------------------------------

  type NotificationType = Schema<"NotificationType">;
  /** What a notification is about, beside who sent it. */
  type About = Omit<Notification, "id" | "type" | "created_at" | "account">;
  const newestFirst = (a: { id: string }, b: { id: string }) => (b.id > a.id ? 1 : -1);
  const notifications: Notification[] = [];
  const notify = (step: number, type: NotificationType, from: string, more: About = {}) => {
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
    notify(step, i.kind, i.by, {
      status: statusOf(p),
      ...(i.kind === "reaction" ? { reaction: reactionEmojiOf(i.emoji) } : {}),
    });
  });

  // ---- Drafts, scheduled posts and what the reader posts ------------------------------------------------------------

  // The reader's drafts, newest first, each saved a few hours before the next: the seed's, then those the reader
  // saves. One whose schedule `fails` failed yesterday, for the post it replied to that is gone, and the reader was
  // notified; one with a `schedule` waits for its time.
  const draftLimits: Schema<"InstanceDraftLimits"> = {
    max_drafts: 100,
    max_scheduled_posts: 300,
    max_scheduled_posts_per_day: 25,
    scheduled_post_min_lead_minutes: 5,
    scheduled_post_max_lead_days: 366,
  };
  /** The limits of an article's body, the server's defaults (ADR-0160 §5). */
  const articleLimits: Schema<"InstanceArticleLimits"> = { max_bytes: 100_000, max_images: 20 };
  const gonePost = idAt(started - 48 * 60 * minute, 80_999);
  const seedDrafts = seed.drafts.filter((d) => d.author === viewer);
  /** The ID of the seed's draft at `i`, and when it was saved. */
  const draftTime = (i: number) => started - (seedDrafts.length - i) * 5 * 60 * minute;
  const draftId = (i: number) => idAt(draftTime(i), 80_000 + i);
  const ownDrafts: Draft[] = seedDrafts
    .map((d, i): Draft => {
      const id = draftId(i);
      const saved = new Date(draftTime(i)).toISOString();
      const replyTo = d.fails !== undefined ? gonePost : d.replyTo === undefined ? undefined : statusId.get(d.replyTo);
      const failedAt = new Date(started - 40 * minute).toISOString();
      return exact<Draft>()({
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
                outcome: "failed" as const,
                reason: "reply_unavailable" as const,
                occurred_at: failedAt,
              },
            }),
      });
    })
    .toReversed();
  /** The images of the seed's drafts, by the ID of each as the drafts name it. */
  const draftImages = new Map<string, Image>(
    seedDrafts.flatMap((d, i) => (d.images ?? []).map((image, n) => [`${draftId(i)}-${String(n)}`, image] as const)),
  );
  let draftsMade = 0;
  const draftOf = (id: string) => ownDrafts.find((d) => d.id === id);
  const etagOf = (d: Draft) => `"${String(Date.parse(d.updated_at))}"`;
  /** A draft as a save leaves it: its content replaced by `body`, saved now (a millisecond after its last save at least). */
  const savedDraft = (
    d: Pick<Draft, "id" | "created_at" | "updated_at" | "scheduled_at" | "last_schedule">,
    body: Schema<"DraftRequest">,
  ): Draft => {
    const { article, poll } = body;
    return exact<Draft>()({
      id: d.id,
      created_at: d.created_at,
      updated_at: new Date(Math.max(Date.now(), Date.parse(d.updated_at) + 1)).toISOString(),
      text: body.text ?? "",
      spoiler_text: body.spoiler_text ?? "",
      sensitive: body.sensitive ?? false,
      ...(body.language === undefined ? {} : { language: body.language }),
      visibility: body.visibility,
      ...(body.in_reply_to_id === undefined ? {} : { in_reply_to_id: body.in_reply_to_id }),
      ...(body.quoted_status_id === undefined ? {} : { quoted_status_id: body.quoted_status_id }),
      ...(body.interaction_policy === undefined ? {} : { interaction_policy: body.interaction_policy }),
      media: body.media ?? [],
      ...(poll === undefined ? {} : { poll }),
      // An article's draft lists the uploads its body shows, as the server finds them at each save.
      ...(article === undefined
        ? {}
        : {
            article: {
              title: article.title ?? "",
              summary: article.summary ?? "",
              tags: article.tags ?? [],
              ...(article.cover === undefined ? {} : { cover: article.cover }),
              images: bodyImages(body.text ?? ""),
            },
          }),
      ...(d.scheduled_at === undefined ? {} : { scheduled_at: d.scheduled_at }),
      ...(d.last_schedule === undefined ? {} : { last_schedule: d.last_schedule }),
    });
  };
  const putDraft = (d: Draft) => {
    ownDrafts.splice(
      ownDrafts.findIndex((x) => x.id === d.id),
      1,
      d,
    );
  };
  const draftAnswer = <S extends 200 | 201>(status: S, d: Draft) => ({ status, body: d, headers: { ETag: etagOf(d) } });
  const unprocessable = (detail: string) => problem(422, "Unprocessable Content", { detail });
  /** An article the server's automated review refuses, as the server answers it. */
  const refusedArticle = () =>
    problem(422, "The post was rejected", {
      type: "/api/v1/problems/status-rejected",
      detail: "This server's review did not accept the article. Change it and try again.",
    });
  /** An article refused for the value of its summary, which the problem names in `field`, as the server answers it. */
  const refusedSummary = () =>
    problem(422, "This could not be done.", {
      detail: "This value is not valid. Check what you entered.",
      field: "summary",
    });
  /** The reader's uploads, by their IDs: the file each was, as the fake serves it once it is processed. */
  const uploads = new Map<string, MediaAttachment>();
  let uploaded = 0;
  /** The statuses the fake made (a post, a draft published, an article), by their IDs, each as it is read alone. */
  const made = new Map<string, Status>();
  /** The HTML of an article's body, its images of the reader's uploads as images, any other as a link of its alt text. */
  const bodyHtml = (text: string) => {
    // Each image of an upload stands as a mark the rendering leaves alone, then as its <img>.
    const images: string[] = [];
    const marked = text.replace(/!\[([^\]]*)\]\(media:([^)\s]+)\)/g, (whole, alt: string, id: string) => {
      const file = uploads.get(id);
      if (file === undefined) return whole;
      const { width = 0, height = 0 } = file.meta;
      images.push(
        `<img src="${file.url ?? ""}" alt="${alt.replace(/"/g, "&quot;")}" width="${String(width)}" height="${String(height)}" loading="lazy">`,
      );
      return `⟦${String(images.length - 1)}⟧`;
    });
    return articleHtml(marked, fakeHost).replace(/⟦(\d+)⟧/g, (_, n: string) => images[Number(n)] ?? "");
  };
  /** A file the reader names in what they write: an upload of theirs, or an image of a draft of the seed. */
  const fileOf = (item: Schema<"StatusMediaItem">): MediaAttachment | undefined => {
    const image = draftImages.get(item.id);
    const file = uploads.get(item.id) ?? (image === undefined ? undefined : imageOf(item.id, image));
    if (file === undefined) return undefined;
    return item.description === undefined ? file : { ...file, description: item.description };
  };
  /** What the reader wrote of an article: its title, and its summary, cover and hashtags when it has them. */
  type WrittenArticle = {
    title: string;
    summary?: string;
    tags?: readonly string[];
    cover?: Schema<"StatusMediaItem">;
  };
  /** What the reader posts: a post, a draft they publish, or an article. */
  type Written = {
    text: string;
    spoiler_text?: string;
    sensitive?: boolean;
    language?: string;
    visibility: Schema<"Visibility">;
    in_reply_to_id?: string;
    quoted_status_id?: string;
    media?: readonly Schema<"StatusMediaItem">[];
    poll?: Schema<"PollInput">;
    article?: WrittenArticle;
  };
  /** An article's part of a status, from what its author wrote: its title, its summary, its cover. */
  const madeArticle = (article: WrittenArticle, text: string): Schema<"StatusArticle"> => {
    const summary = (article.summary ?? "").trim();
    const cover = article.cover === undefined ? undefined : fileOf(article.cover);
    return exact<Schema<"StatusArticle">>()({
      title: article.title,
      summary: summary === "" ? summaryOf(text) : summary,
      ...(cover === undefined ? {} : { cover: { ...cover, description: article.cover?.description ?? "" } }),
    });
  };
  /** What the reader wrote of each article the fake made, by its ID: its body and its article's part. */
  const madeSources = new Map<string, { text: string; spoiler: string; article?: WrittenArticle }>();
  /** A status by its ID as it reads now: one the fake made, or a post of the seed the reader may see. */
  const statusById = (id: string): Status | undefined => {
    const p = shownPost(id);
    return made.get(id) ?? (p === undefined ? undefined : statusOf(p));
  };
  /**
   * A status the reader makes: a post, a draft published, or an article posted (with its body, as read alone), newest,
   * on top of their home timeline.
   */
  const publish = (w: Written): Status => {
    draftsMade += 1;
    const at = Date.now();
    const { article, text } = w;
    const id = idAt(at, 950_000 + draftsMade);
    const parent = w.in_reply_to_id === undefined ? undefined : statusById(w.in_reply_to_id);
    const quoted = w.quoted_status_id === undefined ? undefined : statusById(w.quoted_status_id);
    // A reply to a post whose author approves the reader's replies by hand waits, and counts nowhere (ADR-0155).
    const asked = w.in_reply_to_id === undefined ? undefined : shownPost(w.in_reply_to_id);
    const waits = asked !== undefined && permissionOf(asked, "reply", viewer) === "pending";
    const status = exact<Status>()({
      id,
      uri: `${fakeOrigin}/users/${viewer}/statuses/${id}`,
      url: `${fakeOrigin}/@${viewer}/${id}`,
      account: accountOf(viewer),
      content: article === undefined ? serverHtml(text, fakeHost) : bodyHtml(text),
      spoiler_text: w.spoiler_text ?? "",
      sensitive: w.sensitive ?? false,
      visibility: w.visibility,
      language: w.language ?? "en",
      created_at: new Date(at).toISOString(),
      ...(w.in_reply_to_id === undefined ? {} : { in_reply_to_id: w.in_reply_to_id }),
      ...(parent === undefined ? {} : { in_reply_to_account_id: parent.account.id }),
      media_attachments: (w.media ?? []).flatMap((item) => fileOf(item) ?? []),
      mentions: mentionsOf(text),
      tags: statusTags(article === undefined ? text : "", article?.tags),
      filtered: [],
      emojis: customEmojis,
      replies_count: 0,
      boosts_count: 0,
      favourites_count: 0,
      reactions: [],
      favourited: false,
      boosted: false,
      bookmarked: false,
      pinned: false,
      ...(w.poll === undefined
        ? {}
        : {
            poll: {
              id: idAt(at, 960_000 + draftsMade),
              expires_at: new Date(at + w.poll.expires_in * 1000).toISOString(),
              expired: false,
              multiple: w.poll.multiple ?? false,
              votes_count: 0,
              voters_count: 0,
              options: w.poll.options.map((title) => ({ title, votes_count: 0 })),
              voted: false,
              own_votes: [],
            },
          }),
      ...(quoted === undefined ? {} : { quote: { state: "accepted" as const, quoted_status: quoted } }),
      ...(article === undefined ? {} : { article: madeArticle(article, text) }),
      ...(waits ? { approval: "pending" as const } : {}),
    });
    made.set(id, status);
    if (parent !== undefined && !waits) countReply(parent.id, 1);
    madeSources.set(id, { text, spoiler: w.spoiler_text ?? "", ...(article === undefined ? {} : { article }) });
    // A list holds no article's body (ADR-0160 §8).
    home.unshift(article === undefined ? status : { ...status, content: "" });
    return status;
  };
  /** Counts `by` more replies on the status `id`, one of the seed or one the reader made here. */
  const countReply = (id: string, by: number) => {
    const own = made.get(id);
    if (own === undefined) {
      repliesMade.set(id, Math.max(0, (repliesMade.get(id) ?? 0) + by));
      return;
    }
    const counted = { ...own, replies_count: Math.max(0, own.replies_count + by) };
    made.set(id, counted);
    const at = home.findIndex((s) => s.id === id);
    if (at >= 0) home.splice(at, 1, counted.article === undefined ? counted : { ...counted, content: "" });
  };
  /** The statuses posted with an `Idempotency-Key`, by the key. */
  const posted = new Map<string, Status>();
  /** What `post` makes, or what the request's `Idempotency-Key` made before: a retry posts nothing again. */
  const once = (request: Request, post: () => Status): Status => {
    const key = request.headers.get("Idempotency-Key");
    const before = key === null ? undefined : posted.get(key);
    if (before !== undefined) return before;
    const status = post();
    if (key !== null) posted.set(key, status);
    return status;
  };
  /** Edits the article of `id` the fake made, as `PUT /articles/{id}` does: its body, title, summary, cover, hashtags. */
  const editMade = (id: string, edit: Schema<"EditArticleRequest">): Status | undefined => {
    const before = made.get(id);
    if (before === undefined) return undefined;
    const { text } = edit;
    const article: WrittenArticle = {
      title: edit.title,
      summary: edit.summary ?? "",
      tags: edit.tags ?? [],
      ...(edit.cover === undefined ? {} : { cover: edit.cover }),
    };
    const after: Status = {
      ...before,
      content: bodyHtml(text),
      sensitive: edit.sensitive ?? before.sensitive,
      language: edit.language ?? before.language,
      edited_at: new Date().toISOString(),
      tags: statusTags("", article.tags),
      article: madeArticle(article, text),
    };
    made.set(id, after);
    madeSources.set(id, { text, spoiler: "", article });
    const at = home.findIndex((s) => s.id === id);
    if (at >= 0) home.splice(at, 1, { ...after, content: "" });
    return after;
  };
  // The notification of each failed schedule, among the others by its time.
  for (const d of ownDrafts) {
    const failed = d.last_schedule;
    if (failed?.reason === undefined) continue;
    notifications.push({
      id: idAt(Date.parse(failed.occurred_at), 10_900),
      type: "scheduled_status_failed",
      created_at: failed.occurred_at,
      scheduled_status_failure: { draft_id: d.id, reason: failed.reason, publish_at: failed.publish_at },
    });
  }
  notifications.sort(newestFirst);

  // The reader read all but the newest, three unless the options say how many.
  const unreadAtStart = o.unreadNotifications ?? 3;
  let lastRead = notifications[unreadAtStart]?.id;
  const unreadCount = () => notifications.filter((n) => lastRead === undefined || n.id > lastRead).length;

  // ---- The shell: the instance, the reader's rights, what is around them --------------------------------------------

  const permissionsOf = (username: string): Schema<"Permission">[] => {
    const staff = personOf(username).staff;
    const moderation = ["manage_reports", "manage_accounts", "manage_domains", "manage_invites", "view_audit"] as const;
    // A holder of `view_finance` by a role of their own, as a plan's role may grant it.
    const finance = username === viewer && o.finance?.viewer !== undefined ? (["view_finance"] as const) : [];
    // The roles a story gives the reader in place of the scenario's, which grant what they grant.
    const roles = username === viewer ? o.staffServer?.roles?.viewer : undefined;
    if (roles !== undefined) return permissionsOfRoles(roles);
    if (staff === "moderator") return [...moderation, ...finance];
    if (staff !== "admin") return [...finance];
    return [
      ...moderation,
      ...([
        "manage_settings",
        "manage_rules",
        "manage_roles",
        "manage_jobs",
        "manage_errors",
        "manage_announcements",
      ] as const),
      ...(["manage_custom_emojis", "manage_webhooks", "manage_finance", "view_finance", "manage_supporters"] as const),
      ...(["manage_login_providers", "manage_oauth_apps"] as const),
      "manage_service_closure",
      "manage_login_recovery",
    ];
  };
  /** The reader's own account, with what only they read of it. */
  const credentialOf = (username: string): Schema<"CredentialAccount"> => {
    const p = personOf(username);
    return exact<Schema<"CredentialAccount">>()({
      ...fullAccount(p),
      discoverable: edit.discoverable ?? true,
      indexable: edit.indexable ?? true,
      ai_opt_out: edit.aiOptOut ?? false,
      // The feed follows `indexable` until the reader chooses about it (FR-213).
      feed_enabled: edit.feedEnabled ?? edit.indexable ?? true,
      feed_follows_indexable: edit.feedEnabled === undefined,
      follows_visibility: edit.followsVisibility ?? followsVisibility[username] ?? "public",
      source: {
        note: edit.note ?? p.note,
        fields: (edit.fields ?? p.fields).map(([name, value]) => ({ name, value })),
      },
      posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
      statuses_count: posts.filter((x) => x.author === username).length,
      follow_requests_count: [...requested].filter((edge) => edge.endsWith(`>${username}`)).length,
      permissions: permissionsOf(username),
    });
  };
  /**
   * The reader's preferences: the server's defaults for a person of their locale, with what the options name
   * (`preferences`) in their place, then as the reader changed them.
   */
  let preferences: Schema<"Preferences"> = {
    locale: personOf(viewer).locale,
    locale_follows_browser: true,
    time_zone: "UTC",
    time_zone_chosen: false,
    time_zone_follows_browser: true,
    default_visibility: "public",
    default_interaction_policy: {
      reply: { automatic: "everyone" },
      announce: { automatic: "everyone" },
      quote: { automatic: "everyone" },
    },
    default_post_language: "und",
    require_media_description: false,
    timeline_auto_insert: true,
    link_card_size: "large",
    timeline_languages: [],
    ...o.preferences,
  };
  /** A language tag as the server keeps it (`ja-jp` is `ja-JP`); undefined for one it refuses. */
  const languageTag = (tag: string): string | undefined => {
    try {
      return Intl.getCanonicalLocales(tag)[0];
    } catch {
      return undefined;
    }
  };
  /** Whether `zone` is a time zone the server knows, an IANA name. */
  const knownZone = (zone: string): boolean => {
    try {
      new Intl.DateTimeFormat("en", { timeZone: zone });
      return true;
    } catch {
      return false;
    }
  };
  /**
   * The reader's preferences changed as the server changes them (PATCH /preferences): what the request leaves out
   * stays, a language or a time zone named without whether it follows the browser stops following it, and the
   * timelines' languages are kept sorted, each once. A value the server refuses (a malformed language, an unknown time
   * zone, more than 64 languages) changes nothing and answers the problem that names its preference.
   */
  function changePreferences(change: Schema<"UpdatePreferencesRequest">): Answer<"PATCH /preferences"> {
    const refused = (field: string, detail = "This value is not valid. Check what you entered.") =>
      problem(422, "This could not be done.", { field, detail });
    const { default_interaction_policy: policy, timeline_languages: languages, ...plain } = change;
    const locale = change.locale === undefined ? undefined : languageTag(change.locale);
    if (change.locale !== undefined && (locale === undefined || locale === "und")) return refused("locale");
    if (change.time_zone !== undefined && !knownZone(change.time_zone)) return refused("time_zone");
    const postLanguage =
      change.default_post_language === undefined ? undefined : languageTag(change.default_post_language);
    if (change.default_post_language !== undefined && postLanguage === undefined)
      return refused("default_post_language");
    if (languages !== undefined && languages.length > 64) return refused("timeline_languages", "Use at most 64 items.");
    const tags = languages?.map(languageTag);
    if (tags?.includes(undefined) === true) return refused("timeline_languages");
    preferences = {
      ...preferences,
      ...plain,
      ...(locale === undefined ? {} : { locale, locale_follows_browser: change.locale_follows_browser ?? false }),
      ...(change.time_zone === undefined
        ? {}
        : { time_zone_chosen: true, time_zone_follows_browser: change.time_zone_follows_browser ?? false }),
      ...(postLanguage === undefined ? {} : { default_post_language: postLanguage }),
      default_interaction_policy: { ...preferences.default_interaction_policy, ...policy },
      ...(tags === undefined
        ? {}
        : { timeline_languages: [...new Set(tags.filter((tag) => tag !== undefined))].toSorted() }),
    };
    return { status: 200, body: preferences };
  }
  const brandImage = (svg: string): Schema<"InstanceBrandImage"> => ({
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
  const timelines: Schema<"InstanceTimelines"> =
    o.singleUser === true
      ? { local: "off", federated: "off" }
      : (o.timelines ?? { local: "everyone", federated: "everyone" });
  const around = o.around ?? "all";
  // An uncompressed P-256 point, as a server's VAPID key is: a key of no server, which no push service is asked for.
  const pushServerKey = "BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA_0QTpQtUbVlUls0VJXg7A8u-Ts1XbjhazAkj7I99e8QcYP7DkM";
  const month = new Date(started).toISOString().slice(0, 7);

  // The supporter program and the finance (supporters.ts, finance.ts), which the instance says are on or published.
  const supporting = supporterHandlers({
    options: o.supporters,
    started,
    visitor,
    closing: o.closure !== undefined,
    origin: fakeOrigin,
    known: (username) => people.some((p) => p.username === username) && username !== deactivated,
    visibleAccount: (username) =>
      people.some((p) => p.username === username) &&
      username !== deactivated &&
      (visitor || (!blocking.has(`${viewer}>${username}`) && !blocking.has(`${username}>${viewer}`)))
        ? accountOf(username)
        : undefined,
    viewerAccount: () => accountOf(viewer),
    problem,
    notFound,
    page,
  });
  const finance = financeHandlers({
    options: o.finance,
    started,
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    planCounts: () => supporting.supporters.planCounts(),
    tips: supporting.instance.tips_enabled,
    problem,
    notFound,
    accountId: idOf,
  });
  const fundingMeter = o.finance === undefined ? undefined : finance.meter(month);
  // What the administrators keep of the finance and the supporter program (finance-admin.ts, supporters-admin.ts).
  const financeAdmin = financeAdminHandlers({
    options: o.financeAdmin,
    started,
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    problem,
    notFound,
  });
  const supportersAdmin = supportersAdminHandlers({
    options: o.supportersAdmin,
    started,
    accountId: idOf,
    problem,
    notFound,
  });

  // What the about page shows of the instance, and its documents and moderated domains (about.ts).
  const about = aboutHandlers({
    options: o.about,
    started,
    origin: fakeOrigin,
    visitor,
    summary,
    rules: fakeRules(started).rules,
    statusPage: fakeStatusPage,
    contactAccount: () => fullAccount(personOf(admin)),
    problem,
    notFound,
  });

  const instanceBody = exact<Schema<"Instance">>()({
    domain: fakeHost,
    name: "mycroblog demo",
    version: "0.0.0-demo",
    source_url: "https://example.invalid/mycroblog/my",
    ...about.instance,
    // A one-person server and a closure close the registration whatever the mode (GET /instance shows the effective one).
    registrations: {
      mode: o.singleUser === true || o.closure !== undefined ? "closed" : (o.registrations ?? "open"),
    },
    login_providers_only: o.access?.providersOnly === true,
    limits: {
      max_post_characters: 500,
      require_media_description: o.requiresDescriptions === true,
      drafts: draftLimits,
      articles: articleLimits,
    },
    article_authoring: o.articles?.authoring ?? "everyone",
    timelines,
    federation: { mode: "off" },
    sends_email: o.email === true,
    ...(o.push === true ? { push_server_key: pushServerKey } : {}),
    search_indexing: o.crawlers?.search ?? "allow",
    ai_usage: o.crawlers?.ai ?? "allow",
    ux_measurement: o.uxMeasurement === true,
    ...finance.instance,
    ...supporting.instance,
    voice_enabled: o.voiceOff !== true,
    voice_room_starters: o.voiceStarters ?? "everyone",
    event_creators: o.eventCreators ?? "everyone",
    full_text_search:
      o.search?.posts === true
        ? { available: true, visibilities: ["public", "local"] }
        : { available: false, visibilities: [] },
    translation: {
      browser: o.translation?.browser === true,
      providers: o.translation?.provider === undefined ? [] : [translationProvider],
      ...(o.translation?.reports === true ? { reports_to_server: true } : {}),
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
    // With `finance`, the meter of its costs and income; else one of its own, unless nothing is around the reader.
    ...(o.finance !== undefined
      ? fundingMeter === undefined
        ? {}
        : { funding_meter: fundingMeter }
      : around === "all"
        ? {
            funding_meter: {
              visibility: "percent" as const,
              label: month,
              starts_on: `${month}-01`,
              ends_on: `${month}-28`,
              percent: 72,
              covered: false,
              // A usage charge of the month is not closed yet, so the share may still change.
              estimated: true,
              rail: true,
            },
          }
        : {}),
  });

  // The announcements, newest first, each read by whom the seed says; reads are kept.
  const announcementRead = new Set(
    seed.announcements.filter((a) => a.readBy?.includes(viewer) === true).map((a) => a.key),
  );
  const announcementTime = (key: string) =>
    started - (seed.announcements.length - seed.announcements.findIndex((a) => a.key === key)) * day;
  const announcementId = (key: string) => idAt(announcementTime(key), 20_000);
  const announcementOf = (key: string): Schema<"Announcement"> => {
    const a = seed.announcements.find((x) => x.key === key);
    const at = new Date(announcementTime(key)).toISOString();
    return {
      id: announcementId(key),
      title: a?.title ?? "",
      content: serverHtml(a?.text ?? "", fakeHost),
      published_at: at,
      revised_at: at,
      read: announcementRead.has(key),
      ended: false,
    };
  };
  // The ones published from the administration (staff-announcements.ts) first, as the newest.
  const shownAnnouncements = () =>
    around === "none"
      ? []
      : [
          ...staffServer.server.shownAnnouncements(),
          ...seed.announcements.toReversed().map((a) => announcementOf(a.key)),
        ];
  // One whose window ended a month ago, which the history lists after the ones showing now: the seed publishes none,
  // since a window that ended cannot be set on a new announcement.
  const endedAnnouncement: Schema<"Announcement"> = {
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

  // The open voice rooms: rooms are live, so the scenario, which the seed applies, holds none; these stand in, each
  // while the seed has its host, with those of its participants the seed has: on the stage the host first (`host`,
  // a speaker), the co-hosts (`co`) and the speakers (`speak`), and the listeners, a raised hand among them (`hand`).
  // A participant the reader blocks, or who blocks the reader, is marked so.
  type FakeRoom = {
    name: string;
    host: string;
    icon?: string;
    description?: string;
    hashtags?: string[];
    speakers?: Schema<"VoiceRoomSpeakers">;
    co?: string[];
    speak?: string[];
    listen?: string[];
    hand?: string[];
  };
  const fakeRooms: FakeRoom[] = [
    {
      name: "Morning coffee chat",
      host: "sam",
      icon: "☕",
      description: "Bread, bikes and whatever the week brought. Drop in, say hi, or just listen.",
      hashtags: ["coffee"],
      speakers: "manual",
      co: ["mio"],
      speak: ["hana"],
      listen: ["marco", "layla", "ren", "kai", "quickdeals", "kobo@bread.example"],
      hand: ["layla"],
    },
    { name: "Release party for v0.2", host: "hana", icon: "🎉", hashtags: ["mycroblog"], listen: ["marco", "sam"] },
    {
      name: "Photo walk planning: which trail this weekend, and who brings the long lens",
      host: "yuki",
      speakers: "manual",
      co: ["hana"],
      speak: ["mio"],
      listen: ["kai", "ren", "marco"],
      hand: ["ren"],
    },
    { name: "Arabic and English, slowly", host: "layla", hashtags: ["coffee"], listen: ["ren"] },
  ];
  const local = (username: string) => people.some((p) => p.username === username) && !username.includes("@");
  const blocksEither = (username: string) =>
    blocking.has(`${viewer}>${username}`) || blocking.has(`${username}>${viewer}`);
  const participant = (
    username: string,
    role: "speaker" | "listener",
    r: FakeRoom,
  ): Schema<"VoiceRoomParticipant"> => ({
    account: accountOf(username),
    role,
    hosting: username === r.host,
    cohosting: r.co?.includes(username) === true,
    hand_raised: r.hand?.includes(username) === true,
    blocked: blocksEither(username),
    // Their live session, which the room's stream names, and nothing connects to.
    session: `fake-peer-${username}`,
  });
  const rooms: Schema<"VoiceRoom">[] = fakeRooms.flatMap((r, i) => {
    if (!local(r.host)) return [];
    // The reader hosts a room only while in it: their room waits for them, as one opened for a scheduled time does.
    const stage = [...(r.host === viewer ? [] : [r.host]), ...(r.co ?? []), ...(r.speak ?? [])].filter(local);
    const participants = [
      ...stage.map((u) => participant(u, "speaker", r)),
      ...(r.listen ?? []).filter((u) => local(u) && u !== viewer).map((u) => participant(u, "listener", r)),
    ].filter((p) => p.account.username !== viewer);
    return [
      {
        id: idAt(started - (i + 1) * 30 * minute, 30_000 + i),
        name: r.name,
        ...(r.icon === undefined ? {} : { icon: { name: r.icon } }),
        description: r.description ?? "",
        hashtags: r.hashtags ?? [],
        visibility: "local",
        speakers: r.speakers ?? "everyone",
        host: accountOf(r.host),
        opened_at: new Date(started - (i + 1) * 30 * minute).toISOString(),
        participants_count: participants.length,
        participants,
        can_speak: false,
      },
    ];
  });

  let voiceIds = 0;
  const voiceRooms = voiceHandlers({
    rooms,
    viewer: () => accountOf(viewer),
    off: o.voiceOff === true,
    listed: () => (around === "none" ? [] : rooms),
    summaries: o.voiceSummaries === true,
    ended: {
      id: idAt(started - 3 * 60 * minute, 30_100),
      name: "Late night radio",
    },
    hidden: {
      id: idAt(started - 2 * 60 * minute, 30_101),
      name: "Book club, members only",
    },
    history: [
      {
        id: idAt(started - 26 * 60 * minute, 30_102),
        name: "Sunday photo review",
        opened_at: new Date(started - 26 * 60 * minute).toISOString(),
        ended_at: new Date(started - 24 * 60 * minute).toISOString(),
        end_reason: "host",
      },
      {
        id: idAt(started - 8 * 24 * 60 * minute, 30_103),
        name: "Trail report",
        opened_at: new Date(started - 8 * 24 * 60 * minute).toISOString(),
        ended_at: new Date(started - 8 * 24 * 60 * minute + 40 * minute).toISOString(),
        end_reason: "empty",
      },
    ],
    openRefusal: o.voiceOpen,
    many: o.voiceMany,
    accountByName: (username) => (local(username) ? accountOf(username) : undefined),
    newId: () => idAt(Date.now(), 31_000 + (voiceIds += 1)),
    accountById: (id) => {
      const username = usernameOf.get(id);
      return username === undefined ? undefined : accountOf(username);
    },
    follows: (follower, followee) => {
      const a = usernameOf.get(follower);
      const b = usernameOf.get(followee);
      return a !== undefined && b !== undefined && follows_(a, b);
    },
    blocked: (id) => {
      const username = usernameOf.get(id);
      return username !== undefined && blocksEither(username);
    },
    problem,
    notFound,
    page,
  });

  // The reader's exports of their data, newest first; a request adds one being built, which stays so.
  const dataExports: Schema<"DataExport">[] = [];
  if (o.dataExport === "building" || o.dataExport === "rebuilding")
    dataExports.push({
      id: idAt(started - 10 * minute, 50_100),
      state: "pending",
      requested_at: new Date(started - 10 * minute).toISOString(),
    });
  if (o.dataExport === "failed")
    dataExports.push({
      id: idAt(started - 2 * 60 * minute, 50_101),
      state: "failed",
      requested_at: new Date(started - 2 * 60 * minute).toISOString(),
      completed_at: new Date(started - 2 * 60 * minute + 5 * minute).toISOString(),
      expires_at: new Date(started + 7 * day).toISOString(),
    });
  if (o.dataExport === "ready" || o.dataExport === "rebuilding")
    dataExports.push({
      id: idAt(started - day, 50_100),
      state: "ready",
      requested_at: new Date(started - day).toISOString(),
      completed_at: new Date(started - day + 20 * minute).toISOString(),
      expires_at: new Date(started + 6 * day).toISOString(),
      file_size: 4_812_345,
    });
  if (o.dataExport === "expired")
    dataExports.push({
      id: idAt(started - 8 * day, 50_100),
      state: "ready",
      requested_at: new Date(started - 8 * day).toISOString(),
      completed_at: new Date(started - 8 * day + 20 * minute).toISOString(),
      expires_at: new Date(started - 60 * minute).toISOString(),
      file_size: 4_812_345,
    });

  // The events (events.ts), of the seed's plans whose organizer it has; a voice place's open room is the one its
  // organizer hosts among the rooms above.
  const events = eventHandlers({
    plans: seed.events,
    started,
    viewer,
    accountOf: (username) => (people.some((p) => p.username === username) ? accountOf(username) : undefined),
    usernameOf: (id) => usernameOf.get(id),
    active: (username) => local(username) && username !== deactivated,
    follows: follows_,
    blocks: (a, b) => blocking.has(`${a}>${b}`) || blocking.has(`${b}>${a}`),
    roomHostedBy: (username) => rooms.find((r) => r.host.username === username)?.id,
    voiceOff: o.voiceOff === true,
    mayHostRooms: () => o.voiceStarters !== "permitted" || permissionsOf(viewer).includes("start_voice_rooms"),
    mayCreate: () => o.eventCreators !== "permitted" || permissionsOf(viewer).includes("create_events"),
    unlisted: around === "none",
    origin: fakeOrigin,
    problem,
    notFound,
    page,
  });

  /** When the oldest open report was filed, as the counts say it: nothing while none is open. */
  const oldestOpen = (at: string | undefined) => (at === undefined ? {} : { oldest_open_report_filed_at: at });

  // The moderators' queue of reports (reports.ts), the fake's own, and the reports the reader files.
  const moderated = reportHandlers({
    started,
    options: o.reports,
    viewer,
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    accountOf,
    known: (username) => people.some((p) => p.username === username),
    knownPost: (key) => posts.some((p) => p.key === key),
    accountId: idOf,
    accountById: (id) => {
      const username = usernameOf.get(id);
      return username === undefined ? undefined : accountOf(username);
    },
    statusOf: (key) => statusOf(postOf(key)),
    statusById: (id) => {
      const key = keyOf.get(id);
      return key === undefined ? undefined : statusOf(postOf(key));
    },
    eventId: (key) => events.events.id(key),
    translates: o.translation?.reports === true && o.translation.provider === "answers",
    host: fakeHost,
  });

  // The domains, the remote servers and the audit log (domains.ts), the fake's own.
  const federated = domainHandlers({
    started,
    options: o.domains,
    viewer,
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    accountOf,
    known: (username) => people.some((p) => p.username === username),
    accountId: idOf,
    accountById: (id) => {
      const username = usernameOf.get(id);
      return username === undefined ? undefined : accountOf(username);
    },
    follows: () => following,
    host: fakeHost,
  });

  // The instance theme (theme.ts) and the closure of the service as its administrators run it (closure.ts), whose state
  // the instance says.
  const theming = themeHandlers({
    started,
    options: o.theme,
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    origin: fakeOrigin,
  });
  const closing = closureHandlers({
    started,
    state: o.closure,
    options: o.closureAdmin,
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    domain: fakeHost,
    message: instanceBody.closure?.message ?? "",
  });

  // The administration of roles, custom emoji, announcements and webhooks (staff-server.ts).
  const staffServer = staffServerHandlers({
    started,
    options: o.staffServer,
    viewer,
    visitor,
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    accountOf,
    known: (username) => people.some((p) => p.username === username),
    accountId: idOf,
    accountById: (id) => {
      const username = usernameOf.get(id);
      return username === undefined ? undefined : accountOf(username);
    },
    localEmojis: customEmojis,
    host: fakeHost,
    origin: fakeOrigin,
    idAt,
    problem: refusalOf,
    notFound,
  });

  // The administration's settings, texts, rules, relays, login providers and OAuth apps (admin-settings.ts), as the
  // instance says them.
  const administered = adminHandlers({
    started,
    options: o.admin,
    initial: {
      registration_mode: o.registrations ?? "open",
      single_user: String(o.singleUser === true),
      login_providers_only: String(o.access?.providersOnly === true),
      voice_room_starters: o.voiceStarters ?? "everyone",
      event_creators: o.eventCreators ?? "everyone",
      search_indexing: o.crawlers?.search ?? "allow",
      ai_usage: o.crawlers?.ai ?? "allow",
      link_cards: "false",
      contact_email: "admins@demo.example",
      // What the instance publishes of its finance and its supporters (finance.ts, supporters.ts), and the meter's target.
      finance_visibility: finance.instance.finance_visibility,
      income_visibility: finance.instance.income_visibility,
      funding_meter_visibility: fundingMeter?.visibility ?? "off",
      funding_meter_target_amount: "20000",
      funding_meter_target_currency: "JPY",
      funding_meter_rail: "true",
      supporter_list_visibility: supporting.instance.supporter_list_visibility,
      ...(o.thumbnail === true ? { thumbnail: idAt(started, 990_900) } : {}),
      ...(o.translation?.provider === undefined ? {} : { translation_providers: translationProvider.id }),
      ...(o.translation?.browser === true ? { translation_browser: "true" } : {}),
      ...(o.translation?.reports === true ? { translation_reports_to_server: "true" } : {}),
      ...(o.uxMeasurement === true ? { ux_measurement: "true" } : {}),
    },
    texts: {
      name: instanceBody.name,
      short_description: instanceBody.short_description ?? "",
      description:
        "We are a small server for the people of the river town. Post about the park, the events, lost and found, " +
        "and the seasons.",
      terms: "## Terms of service\n\nBe kind, follow the rules, and keep your password to yourself.",
      privacy_policy:
        "## Privacy policy\n\nWe keep what you post and the addresses you sign in from, for as long as the settings say.",
      ...(o.thumbnail === true ? { thumbnail_alt: "Green hills under a pale sun." } : {}),
    },
    brands:
      o.logo === true
        ? {
            logo: { ...brandImage(logoSvg("#2f6f8f")), revised_at: new Date(started).toISOString() },
            logo_dark: { ...brandImage(logoSvg("#8fc7e0")), revised_at: new Date(started).toISOString() },
          }
        : {},
    translationProviders: [
      { id: translationProvider.id, name: translationProvider.name },
      { id: "libretranslate", name: "LibreTranslate" },
    ],
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    accountOf,
    viewer,
    host: fakeHost,
  });

  // The operations (operations.ts): the job queue, the error groups, the UX measurements, the link card crawler's rules.
  const operated = operationsHandlers({
    started,
    options: o.operations,
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    setting: (key) => administered.admin.setting(key),
  });

  // The moderation of accounts (accounts.ts): actions, appeals, registrations, hooks' decisions, sign-up blocks, invites.
  const accounts = moderationHandlers({
    started,
    options: o.moderation,
    viewer,
    permissions: () => (visitor ? [] : permissionsOf(viewer)),
    people,
    accountOf,
    accountId: idOf,
    statusById: (id) => {
      const key = keyOf.get(id);
      return key === undefined ? undefined : statusOf(postOf(key));
    },
    reports: moderated.reports,
    singleUser: o.singleUser === true,
    registrationClosed: instanceBody.registrations.mode === "closed",
    host: fakeHost,
  });

  // With `everyKind`, a notification of each kind of its group in place of the seed's, an hour apart: from people
  // (about the reader's post), the server's notices, or those to its moderators, each with what its sentence and its
  // target read.
  if (o.everyKind !== undefined) {
    const own = posts.find((p) => p.author === viewer);
    const status: About = own === undefined ? {} : { status: statusOf(own) };
    // A post of `from`: one that mentions the reader, else any of theirs, else the reader's own.
    const postBy = (from: string): About => {
      const p =
        posts.find((x) => x.author === from && mentioned(x.text).includes(viewer)) ??
        posts.find((x) => x.author === from);
      return p === undefined ? status : { status: statusOf(p) };
    };
    const firstEvent = seed.events[0];
    const event = firstEvent === undefined ? undefined : events.events.shown(firstEvent.key);
    const about: About = event === undefined ? {} : { event_id: event.id, event };
    const room = rooms[0];
    type Kinds = [NotificationType, string | undefined, About][];
    const fromPeople: Kinds = [
      ["follow", "kobo@bread.example", {}],
      ["follow_request", "kiri@far.example", {}],
      ["mention", "sam", postBy("sam")],
      ["quote", "mio", postBy("mio")],
      ["favourite", "marco", status],
      ["boost", "hana", status],
      ["reaction", "layla", { ...status, reaction: { name: "🎉" } }],
      ["reply_request", "marco", status],
      ["boost_request", "mio", status],
      ["quote_request", "sam", status],
      ["status", "hana", postBy("hana")],
      ["follow_accepted", "hana", {}],
      ["poll_closed", undefined, status],
      ["interaction_accepted", "layla", status],
      ["interaction_rejected", "marco", status],
      ["account_moved", "sora@social.example", { account_move: { target: accountOf("sora@photos.example") } }],
      ["voice_room_started", "hana", room === undefined ? {} : { voice_room_id: room.id, voice_room: room }],
      ["voice_room_invited", "sam", room === undefined ? {} : { voice_room_id: room.id }],
      ["event_invited", "mio", about],
    ];
    const notices: Kinds = [
      ["event_schedule_confirmed", undefined, about],
      ["event_changed", undefined, about],
      ["event_cancelled", undefined, about],
      ["event_reminder", undefined, about],
      ["event_started", undefined, about],
      ["event_room_unavailable", undefined, about],
      ["announcement", undefined, { announcement: announcementOf(seed.announcements[0]?.key ?? "") }],
      ["moderation_notice", undefined, {}],
      ["report_resolved", undefined, {}],
      ["appeal_decided", undefined, {}],
      ["hook_appeal_decided", undefined, {}],
      ["relationships_severed", undefined, {}],
      ["custom_emoji_request_decided", undefined, {}],
      ["supporter_payment_failed", undefined, {}],
      ["supporter_subscription_ended", undefined, {}],
      ["supporter_plan_retired", undefined, {}],
      ["service_closure", undefined, { service_closure: { change: "announced" } }],
      ["supporter_service_closure", undefined, { service_closure: { change: "announced" } }],
      [
        "auto_delete_limit",
        undefined,
        { auto_delete_limit: { max_age_days: 365, enforce_from: new Date(started + 30 * day).toISOString() } },
      ],
    ];
    const moderators: Kinds = [
      ["admin_report", undefined, { report_id: idAt(started, 60_000) }],
      ["admin_sign_up", "kiri@far.example", {}],
      ["admin_appeal", undefined, { appeal_id: idAt(started, 60_001) }],
      ["admin_webhook_disabled", undefined, {}],
      ["admin_hook_failing", undefined, {}],
      ["admin_domain_list_failing", undefined, {}],
      ["admin_custom_emoji_request", "mio", {}],
      ["admin_account_moved", undefined, { account_move: { target: accountOf("sora@photos.example") } }],
      ["admin_service_closure_due", undefined, { service_closure: { change: "announced" } }],
      ["admin_error_group", undefined, { error_group: { change: "created" } }],
      ["admin_hook_appeal", undefined, { hook_appeal_id: idAt(started, 60_002) }],
    ];
    const kinds = { people: fromPeople, notices, moderators }[o.everyKind];
    notifications.length = 0;
    kinds.forEach(([type, from, more], i) => {
      const at = started - (i + 1) * 60 * minute;
      notifications.push({
        id: idAt(at, 11_000 + i),
        type,
        created_at: new Date(at).toISOString(),
        ...(from === undefined ? {} : { account: accountOf(from) }),
        ...more,
      });
    });
    notifications.sort(newestFirst);
    // The reader read all but the newest, as of the seed's.
    lastRead = notifications[unreadAtStart]?.id;
  }

  // With `dismissFails`, the dismissals of a notification still to fail before one is kept.
  let dismissFailures = o.dismissFails === true ? 1 : 0;

  // With `held`, the notifications the policy holds back, by the account that sent them, the newest account first.
  const held = new Map<string, Notification[]>();
  if (o.held === true) {
    const own = posts.find((p) => p.author === viewer);
    const about: About = own === undefined ? {} : { status: statusOf(own) };
    const hold = (from: string, type: NotificationType, minutesAgo: number, n: number, more: About = {}) => {
      const at = started - minutesAgo * minute;
      const list = held.get(from) ?? [];
      list.push({
        id: idAt(at, 12_000 + n),
        type,
        created_at: new Date(at).toISOString(),
        account: accountOf(from),
        ...more,
      });
      held.set(from, list);
    };
    hold("kiri@far.example", "favourite", 20, 0, about);
    hold("kiri@far.example", "boost", 45, 1, about);
    hold("kobo@bread.example", "follow", 90, 2);
    (o.heldFrom ?? []).forEach((from, i) => {
      hold(from, "follow", 120 + i * 10, 3 + i);
    });
  }
  const heldRequestOf = (username: string): Schema<"NotificationRequest"> => {
    const list = held.get(username) ?? [];
    return exact<Schema<"NotificationRequest">>()({
      id: idOf(username),
      account: accountOf(username),
      notifications_count: list.length,
      ...(list[0] === undefined ? {} : { last_notification: list[0] }),
    });
  };

  /** The reader's Web Push subscription, once they registered one (`push`). */
  let pushSubscription: Schema<"PushSubscription"> | undefined =
    o.pushState === "subscribed"
      ? { id: idAt(0, 70_000), endpoint: "https://push.example/s/held", alerts: ["mention"], server_key: pushServerKey }
      : undefined;

  // ---- The stream ---------------------------------------------------------------------------------------------------

  /** The open streams, which stay open until their requests are given up. */
  const streams = new Set<ReadableStreamDefaultController<Uint8Array>>();
  const encoder = new TextEncoder();
  function send(event: FakeStreamEvent) {
    const [name, data] = wire(event);
    const frame = encoder.encode(`event: ${name}\ndata: ${JSON.stringify(data)}\n\n`);
    for (const stream of streams) stream.enqueue(frame);
  }
  /** A stream that opens now and ends when `signal` aborts. */
  function openStream(signal: AbortSignal): ReadableStream<Uint8Array> {
    let own: ReadableStreamDefaultController<Uint8Array> | undefined;
    const close = () => {
      if (own === undefined || !streams.delete(own)) return;
      own.close();
    };
    return new ReadableStream<Uint8Array>({
      start(controller) {
        own = controller;
        streams.add(controller);
        signal.addEventListener("abort", close, { once: true });
      },
      cancel() {
        if (own !== undefined) streams.delete(own);
      },
    });
  }

  // ---- Answers ------------------------------------------------------------------------------------------------------

  function change(id: string, what: "follow" | "block" | "mute", add: boolean): Answer<"POST /accounts/{id}/follow"> {
    const other = usernameOf.get(id);
    if (other === undefined) return notFound();
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
    return { status: 200, body: relationshipOf(id) };
  }

  /** Does `act` to the seed's post of the status `id` the reader may see, and answers it as it reads then. */
  function acted(id: string, act: (p: ApprovalPost) => Refusal | undefined): Answer<"POST /statuses/{id}/bookmark"> {
    const p = shownPost(id);
    if (p === undefined) return notFound();
    return act(p) ?? { status: 200, body: statusOf(p) };
  }

  /** The seed's posts as a page of statuses, later pages late. */
  async function statusPage(shown: readonly Status[], query: PageQuery): Promise<Answer<"GET /timelines/home">> {
    const { from, body } = page(shown, query);
    if (from > 0) await late();
    return { status: 200, body };
  }

  /** A page of accounts, later pages late. */
  async function accountPage(
    shown: readonly Account[],
    query: PageQuery,
  ): Promise<Answer<"GET /accounts/{id}/followers">> {
    const { from, body } = page(shown, query);
    if (from > 0) await late();
    return { status: 200, body };
  }

  /** The reader's private note on the person of the account `id`, set to `note`. */
  function noted(id: string, note: string): Answer<"PUT /accounts/{id}/note"> {
    const other = usernameOf.get(id);
    if (other === undefined) return notFound();
    // The server counts code points.
    if (Array.from(note).length > 2000) return problem(422, "Unprocessable Entity");
    if (note === "") notes.delete(other);
    else notes.set(other, note);
    return { status: 200, body: relationshipOf(id) };
  }

  /** An account's list of whom it follows, or of its `followers`, newest follow first, as far as it shows them. */
  function followsOf(
    id: string,
    followers: boolean,
    query: PageQuery,
  ): Promise<Answer<"GET /accounts/{id}/followers">> {
    if (o.followsLists === "waits") return never();
    if (o.followsLists === "fails") return Promise.resolve(problem(500, "Internal Server Error"));
    const username = usernameOf.get(id);
    if (username === undefined) return Promise.resolve(notFound());
    const shown = followsVisibility[username] ?? "public";
    // A visitor is no one's follower, and owns no list.
    const reader = visitor ? undefined : viewer;
    if (
      username !== reader &&
      shown !== "public" &&
      !(shown === "followers" && reader !== undefined && follows_(reader, username))
    )
      return Promise.resolve(
        problem(403, "This account's follows are not shown", { type: "/api/v1/problems/follows-hidden" }),
      );
    const listed = [...following]
      .toReversed()
      .map((edge) => edge.split(">") as [string, string])
      .filter(([from, to]) => (followers ? to === username : from === username))
      .map(([from, to]) => accountOf(followers ? from : to));
    return accountPage(listed, query);
  }

  /**
   * The reader's notification as its list shows it now: with its post as it is, and as their keyword filters leave it
   * among the notifications; none once the post is deleted, or while a filter hides it.
   */
  const notificationNow = (n: Notification): Notification[] => {
    if (n.status === undefined) return [n];
    const [now] = judged(current(n.status), "notifications");
    return now === undefined ? [] : [{ ...n, status: now }];
  };

  /** The source of the seed's post `p` of the reader's own, as they wrote it. */
  const sourceOf = (p: Post): Schema<"StatusSource"> => {
    const id = statusId.get(p.key) ?? "";
    const now = versionsOf(p).at(-1);
    const a = p.article;
    const edited = articleEdits.get(p.key);
    return exact<Schema<"StatusSource">>()({
      id,
      text: now?.text ?? p.text,
      spoiler_text: now?.spoiler ?? "",
      language: p.language,
      ...(a === undefined
        ? {}
        : {
            article: exact<Schema<"ArticleSource">>()({
              title: edited?.title ?? a.title,
              summary: edited?.summary ?? a.summary,
              tags: [...(edited?.tags ?? [])],
              ...(a.cover === undefined ? {} : { cover: { id: `${id}-cover`, description: a.cover.description } }),
            }),
          }),
    });
  };

  /** The operations the fake implements, each typed by the specification. */
  const handlers: Handlers = {
    // ---- Timelines, hashtags, lists, bookmarks ----------------------------------------------------------------------

    "GET /timelines/home": async ({ query }) => {
      const { from, body } = page(judged(homeNow(), "home"), query);
      if (o.failFirst === true && from === 0 && Date.now() - started < 1000)
        return problem(500, "Internal Server Error");
      // The retry after the first page failed comes late, so that the screen shows it loading again.
      if (o.failFirst === true) await wait(o.lateMs ?? 2000);
      else if (from > 0) await late();
      return { status: 200, body };
    },
    "GET /timelines/tag/{hashtag}": ({ params, query }) => {
      // The public posts with the hashtag, newest first, as they read now, and its local ones for a reader signed in.
      const tag = params.hashtag.replace(/^#/, "").toLowerCase();
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
      return statusPage(judged(shown, "public"), query);
    },
    "GET /timelines/local": ({ query }) => publicTimeline("local", query),
    "GET /timelines/federated": ({ query }) => publicTimeline("federated", query),
    "GET /timelines/list/{id}": ({ params, query }) => {
      // The home timeline's entries by the list's members, never the reader's own (ADR-0013 §13).
      const found = lists.find((l) => l.id === params.id);
      if (found === undefined) return notFound();
      return statusPage(
        judged(
          homeNow().filter((s) => found.members.includes(s.account.username)),
          "home",
        ),
        query,
      );
    },
    "GET /bookmarks": ({ query }) =>
      statusPage(
        bookmarks
          .map(postOf)
          .filter((p) => visible(p))
          .map((p) => statusOf(p)),
        query,
      ),
    "GET /tags/{hashtag}": ({ params }) => ({ status: 200, body: tagOf(params.hashtag.replace(/^#/, "")) }),
    "POST /tags/{hashtag}/follow": ({ params }) => {
      // One more than the most the reader may follow is refused as the server refuses it.
      const asked = params.hashtag.replace(/^#/, "");
      const key = asked.toLowerCase();
      if (!followedTags.has(key)) {
        if (followedTags.size >= (o.followedTagLimit ?? 100))
          return problem(422, "Too many followed hashtags", {
            type: "/api/v1/problems/too-many-followed-tags",
            detail: "You follow as many hashtags as you can. Unfollow one to follow another.",
          });
        followedTags.add(key);
      }
      return { status: 200, body: tagOf(asked) };
    },
    "DELETE /tags/{hashtag}/follow": ({ params }) => {
      const asked = params.hashtag.replace(/^#/, "");
      followedTags.delete(asked.toLowerCase());
      return { status: 200, body: tagOf(asked) };
    },
    "GET /tags/search": ({ query }) => {
      // The hashtags of the public, local and unlisted posts that start with `q`: the one equal to it first, then the
      // most used.
      const q = (query.get("q") ?? "").replace(/^#/u, "").normalize("NFKC").toLowerCase();
      const limit = Number(query.get("limit") ?? "8");
      const uses = new Map<string, { name: string; count: number }>();
      for (const p of posts)
        if (!deleted.has(p.key) && p.visibility !== "private" && p.visibility !== "direct")
          for (const [, tag = ""] of p.text.matchAll(/(?<![\p{L}\p{N}_])#([\p{L}\p{M}\p{N}_]+)/gu)) {
            const key = tag.normalize("NFKC").toLowerCase();
            uses.set(key, { name: uses.get(key)?.name ?? tag, count: (uses.get(key)?.count ?? 0) + 1 });
          }
      const found = [...uses]
        .filter(([key]) => key.startsWith(q))
        .toSorted(([ka, a], [kb, b]) => Number(kb === q) - Number(ka === q) || b.count - a.count)
        .map(([, t]): Schema<"TagSuggestion"> => ({ name: t.name, statuses_count: t.count }));
      return { status: 200, body: { items: found.slice(0, limit) } };
    },
    "GET /lists": () => ({ status: 200, body: { items: lists.map(listBody) } }),
    "POST /lists": ({ body }) => {
      // A name of 1 to 50 characters on one line; at the limit of lists, the server's refusal.
      const name = body.name.trim();
      if (!listName(name)) return listNameRefused();
      if (lists.length >= listLimit)
        return problem(422, "You have as many lists as you may", { type: "/api/v1/problems/too-many-lists" });
      madeLists += 1;
      const list = {
        id: idAt(Date.now(), 61_000 + madeLists),
        name,
        members: [],
        created_at: new Date().toISOString(),
      };
      lists.push(list);
      return { status: 201, body: listBody(list) };
    },
    "GET /lists/{id}": ({ params }) => {
      const found = lists.find((l) => l.id === params.id);
      return found === undefined ? notFound() : { status: 200, body: listBody(found) };
    },
    "PATCH /lists/{id}": ({ params, body }) => {
      const found = lists.find((l) => l.id === params.id);
      if (found === undefined) return notFound();
      const name = body.name.trim();
      if (!listName(name)) return listNameRefused();
      found.name = name;
      return { status: 200, body: listBody(found) };
    },
    "DELETE /lists/{id}": ({ params }) => {
      const at = lists.findIndex((l) => l.id === params.id);
      if (at < 0) return notFound();
      lists.splice(at, 1);
      return { status: 204 };
    },
    "GET /lists/{id}/accounts": ({ params, query }) => {
      // The members in the order of their IDs.
      const found = lists.find((l) => l.id === params.id);
      if (found === undefined) return notFound();
      return accountPage(
        found.members.map(accountOf).toSorted((a, b) => a.id.localeCompare(b.id)),
        query,
      );
    },
    "POST /lists/{id}/accounts/{account_id}": ({ params }) => {
      const found = lists.find((l) => l.id === params.id);
      const username = usernameOf.get(params.account_id);
      if (found === undefined || username === undefined) return notFound();
      if (found.members.includes(username)) return { status: 204 };
      if (!follows_(viewer, username))
        return problem(422, "You can add only accounts you follow", { type: "/api/v1/problems/not-followed" });
      if (found.members.length >= memberLimit)
        return problem(422, "The list has as many members as it may", {
          type: "/api/v1/problems/too-many-list-members",
        });
      found.members.push(username);
      return { status: 204 };
    },
    "DELETE /lists/{id}/accounts/{account_id}": ({ params }) => {
      const found = lists.find((l) => l.id === params.id);
      const username = usernameOf.get(params.account_id);
      if (found === undefined || username === undefined) return notFound();
      found.members = found.members.filter((x) => x !== username);
      return { status: 204 };
    },
    "GET /accounts/{id}/lists": ({ params }) => {
      const username = usernameOf.get(params.id);
      return {
        status: 200,
        body: { items: lists.filter((l) => username !== undefined && l.members.includes(username)).map(listBody) },
      };
    },

    // ---- The shell --------------------------------------------------------------------------------------------------

    "GET /accounts/verify_credentials": () => ({ status: 200, body: credentialOf(viewer) }),
    "GET /preferences": () =>
      o.preferencesFail === "read" ? problem(500, "Internal Server Error") : { status: 200, body: preferences },
    "PATCH /preferences": ({ body }) =>
      o.preferencesFail === "save" ? problem(500, "Internal Server Error") : changePreferences(body),
    "GET /instance": () =>
      o.about?.instance === "waits"
        ? never()
        : o.about?.instance === "fails"
          ? problem(500, "Internal Server Error")
          : { status: 200, body: closing.instanceOf(instanceBody) },
    "GET /instance/health": () => {
      const health: Schema<"ServerHealth"> = { status: "ok", checked_at: new Date().toISOString() };
      return { status: 200, body: health };
    },
    "GET /announcements": () => ({ status: 200, body: { items: shownAnnouncements() } }),
    "GET /announcements/history": ({ query }) => ({ status: 200, body: page(announcementHistory(), query).body }),
    "GET /announcements/{id}": ({ params }) => {
      const found = announcementHistory().find((a) => a.id === params.id);
      return found === undefined ? notFound() : { status: 200, body: found };
    },
    "POST /announcements/{id}/read": ({ params }) => {
      const a = seed.announcements.find((x) => announcementId(x.key) === params.id);
      if (a === undefined) return notFound();
      announcementRead.add(a.key);
      return { status: 200, body: announcementOf(a.key) };
    },
    "GET /admin/counts": () => {
      const granted = permissionsOf(viewer);
      const counts = exact<Schema<"AdminCounts">>()({
        ...(granted.includes("manage_reports") ? { open_reports: moderated.reports.openCount() } : {}),
        ...(granted.includes("manage_reports") ? oldestOpen(moderated.reports.oldestOpen()) : {}),
        ...(granted.includes("manage_accounts")
          ? {
              ...(o.singleUser === true ? {} : { pending_registrations: accounts.moderation.pendingRegistrations() }),
              pending_appeals: accounts.moderation.pendingAppeals(),
            }
          : {}),
        ...(granted.includes("manage_custom_emojis")
          ? { pending_custom_emoji_requests: staffServer.server.pendingEmojiRequests() }
          : {}),
        ...(granted.includes("manage_errors") ? { open_error_groups: operated.operations.openErrorGroups() } : {}),
      });
      return { status: 200, body: counts };
    },
    "GET /custom_emojis": () => ({ status: 200, body: { items: pickerEmojis } }),
    "GET /streaming": ({ request }) => ({ status: 200, type: "text/event-stream", raw: openStream(request.signal) }),

    // ---- The reader's exports ---------------------------------------------------------------------------------------

    "GET /exports": () => ({ status: 200, body: { items: dataExports } }),
    "POST /exports": () => {
      if (dataExports.some((e) => e.state === "pending"))
        return problem(409, "An export is in progress", { type: "/api/v1/problems/export-in-progress" });
      const requested: Schema<"DataExport"> = {
        id: idAt(Date.now(), 50_000 + dataExports.length),
        state: "pending",
        requested_at: new Date().toISOString(),
      };
      dataExports.unshift(requested);
      return { status: 202, body: requested };
    },
    "GET /exports/{id}/archive": ({ params }) => {
      const found = dataExports.find((e) => e.id === params.id);
      if (found === undefined) return notFound();
      if (found.state !== "ready")
        return problem(409, "The export is not ready", { type: "/api/v1/problems/export-not-ready" });
      return {
        status: 200,
        type: "application/zip",
        raw: new Uint8Array([0x50, 0x4b, 0x05, 0x06, ...new Array<number>(18).fill(0)]),
        headers: { "Content-Disposition": 'attachment; filename="export.zip"' },
      };
    },
    "GET /csv_exports/{kind}": ({ params }) => {
      const rows = people
        .filter((p) => follows_(viewer, p.username))
        .map((p) => `${p.username}@${fakeHost},true,false,`);
      const name = params.kind === "following" ? "following_accounts" : params.kind;
      return {
        status: 200,
        type: "text/csv",
        raw: ["Account address,Show boosts,Notify on new posts,Languages", ...rows].join("\n"),
        headers: { "Content-Disposition": `attachment; filename="${name}.csv"` },
      };
    },

    // ---- Accounts ---------------------------------------------------------------------------------------------------

    "GET /accounts/search": ({ query }) => {
      // As the server finds them (ADR-0092 §1, FR-307): a username, `username@host` or a word of the display name that
      // starts with `q`, and an account of this server whose username or display name holds it; the blocked and muted
      // left out, and the person of another server it has not met; the exact username first, then the followed, then
      // the rest.
      const q = (query.get("q") ?? "").replace(/^@/u, "").normalize("NFKC").toLowerCase();
      const limit = Number(query.get("limit") ?? "8");
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
            ].some((w) => w.startsWith(q)) ||
            (!p.username.includes("@") && (p.username.includes(q) || name(p).includes(q))),
        )
        .toSorted(
          (a, b) =>
            Number(b.username === q) - Number(a.username === q) ||
            Number(follows_(viewer, b.username)) - Number(follows_(viewer, a.username)),
        );
      return { status: 200, body: { items: found.slice(0, limit).map(fullAccount) } };
    },
    "GET /accounts/lookup": ({ query }) => {
      // A handle, or the address of a profile (`https://host/@user`); the person of another server this one has not
      // met is found only by `resolve`, which fetches and keeps their account (FR-305).
      // Fetching an account from its server is a reader's: a visitor's resolve is refused, as the server refuses it.
      if (visitor && query.get("resolve") === "true") return problem(401, "Unauthorized");
      const asked = (query.get("acct") ?? "").replace(/^@/u, "");
      const profile = /^https?:\/\/([^/]+)\/@([^/@]+)\/?$/u.exec(asked);
      const handle = profile === null ? asked : `${profile[2] ?? ""}@${profile[1] ?? ""}`;
      const acct = handle.replace(new RegExp(`@${fakeHost}$`), "");
      if (acct === unmet && query.get("resolve") === "true") met.add(unmet);
      const found =
        acct === deactivated || (acct === unmet && !met.has(unmet))
          ? undefined
          : people.find((p) => p.username === acct);
      return found === undefined ? notFound() : { status: 200, body: fullAccount(found) };
    },
    "GET /accounts/suggestions": () => {
      // This server's accounts the reader does not follow, those that posted last first (FR-308); none on a
      // one-person server.
      if (o.singleUser === true)
        return problem(404, "This is a one-person server", { type: "/api/v1/problems/single-user-mode" });
      const lastPost = (username: string) => posts.findLastIndex((p) => p.author === username);
      const suggested = people
        .filter((p) => !p.username.includes("@") && p.username !== viewer && p.username !== deactivated)
        .filter((p) => !follows_(viewer, p.username) && !requested.has(`${viewer}>${p.username}`))
        .filter((p) => !blocking.has(`${viewer}>${p.username}`) && !muting.has(`${viewer}>${p.username}`))
        .toSorted((a, b) => lastPost(b.username) - lastPost(a.username))
        .map((p): Schema<"AccountSuggestion"> => ({ source: "recently_active", account: accountOf(p.username) }));
      return { status: 200, body: { items: suggested } };
    },
    "GET /accounts/relationships": ({ query }) => ({
      status: 200,
      body: { items: query.all("id").map(relationshipOf) },
    }),
    "GET /accounts/{id}": ({ params }) => {
      const username = usernameOf.get(params.id);
      if (username === undefined) return notFound();
      // Only the account's own read carries the room where it hosts or speaks (ADR-0100 §8).
      const entry = voiceRooms.voice.entryOf(params.id);
      return {
        status: 200,
        body: { ...fullAccount(personOf(username)), ...(entry === undefined ? {} : { voice_room: entry }) },
      };
    },
    "GET /accounts/{id}/statuses": ({ params, query }) => {
      const all = profilePosts(usernameOf.get(params.id) ?? "");
      if (query.get("pinned") === "true") {
        const pinned = all.filter((p) => p.pinned === true).map((p) => statusOf(p));
        return { status: 200, body: { items: judged(pinned, "account") } };
      }
      const shown =
        query.get("articles") === "true"
          ? all.filter((p) => p.article !== undefined)
          : query.get("exclude_replies") === "true"
            ? all.filter((p) => p.replyTo === undefined)
            : all;
      return statusPage(
        judged(
          shown.map((p) => statusOf(p)),
          "account",
        ),
        query,
      );
    },
    "GET /accounts/{id}/featured_tags": ({ params }) => ({
      status: 200,
      body: { items: params.id === idOf(viewer) ? personal.featured() : [] },
    }),
    "GET /followed_tags": () => ({
      status: 200,
      body: {
        // The latest followed first, as the server lists them.
        items: [...followedTags]
          .map((name, i) => ({
            name,
            url: `${fakeOrigin}/tags/${name}`,
            followed_at: new Date(started - (followedTags.size - i) * day).toISOString(),
          }))
          .reverse(),
      },
    }),
    "GET /blocks": async ({ query }) => {
      const blocked = [...blocking]
        .map((edge) => edge.split(">") as [string, string])
        .filter(([by]) => by === viewer)
        .map(([, target]) => accountOf(target));
      return accountPage(blocked, query);
    },
    "GET /mutes": async ({ query }) => {
      const muted = [...muting]
        .map((edge) => edge.split(">") as [string, string])
        .filter(([by]) => by === viewer)
        .map(([, target]): Schema<"MutedAccount"> => ({ account: accountOf(target), hide_notifications: true }));
      const { from, body } = page(muted, query, (m) => m.account.id);
      if (from > 0) await late();
      return { status: 200, body };
    },
    "POST /accounts/email/verification": () => {
      if (visitor) return problem(401, "Unauthorized");
      if (o.email !== true) return notFound();
      if (o.verificationEmail === "limited")
        return { ...problem(429, "Too Many Requests"), headers: { "Retry-After": String(25 * 60) } };
      return { status: 202 };
    },
    "PATCH /accounts/update_credentials": ({ body }) => {
      if (visitor) return problem(401, "Unauthorized");
      if (o.profile?.save === "waits") return new Promise<never>(() => undefined);
      if (o.profile?.save === "refused")
        return problem(422, "The profile was rejected", {
          type: "/api/v1/problems/profile-rejected",
          detail: "This server's automated review did not accept this change. Your profile stays as it was.",
        });
      if (o.profile?.save === "name" && body.display_name !== undefined)
        return problem(422, "This could not be done.", {
          detail: "This value is not valid. Check what you entered.",
          field: "display_name",
        });
      // What the server refuses of a text, which the problem names in `field` (a part of a field without the field's
      // index, as the server does): longer than its limit, or holding a control character it does not take there (a
      // line break or a tab is one too, but in the profile text and a field's value).
      const refusedText = (field: string, text: string | undefined, max: number, lines: boolean) => {
        if (text === undefined) return undefined;
        const controls = lines ? /[^\P{Cc}\n\r\t]/u : /\p{Cc}/u;
        const detail =
          Array.from(text).length > max
            ? `Use at most ${String(max)} characters.`
            : controls.test(text)
              ? "This value is not valid. Check what you entered."
              : undefined;
        return detail === undefined ? undefined : problem(422, "This could not be done.", { detail, field });
      };
      const refusedOf =
        refusedText("display_name", body.display_name, 30, false) ??
        refusedText("note", body.note, 500, true) ??
        (body.fields !== undefined && body.fields.length > 4
          ? problem(422, "This could not be done.", { detail: "You can have at most 4.", field: "fields" })
          : undefined) ??
        (body.fields ?? [])
          .flatMap((f) => [
            refusedText("fields[].name", f.name, 255, false),
            refusedText("fields[].value", f.value, 255, true),
          ])
          .find((refused) => refused !== undefined);
      if (refusedOf !== undefined) return refusedOf;
      // A picture is a processed upload of the reader's, or empty to remove it.
      const picture = (id: string | undefined): string | null | undefined | Refusal => {
        if (id === undefined) return undefined;
        if (id === "") return null;
        const url = uploads.get(id)?.url;
        return url === undefined ? problem(422, "The image is not there or is not processed") : url;
      };
      const [avatar, header] = [picture(body.avatar_media_id), picture(body.header_media_id)];
      if (typeof avatar === "object" && avatar !== null) return avatar;
      if (typeof header === "object" && header !== null) return header;
      if (body.display_name !== undefined) edit.displayName = body.display_name;
      if (body.note !== undefined) edit.note = body.note;
      if (body.fields !== undefined) edit.fields = body.fields.map((f) => [f.name, f.value]);
      if (avatar !== undefined) edit.avatar = avatar;
      if (header !== undefined) edit.header = header;
      if (body.locked !== undefined) edit.locked = body.locked;
      if (body.bot !== undefined) edit.bot = body.bot;
      if (body.discoverable !== undefined) edit.discoverable = body.discoverable;
      if (body.indexable !== undefined) edit.indexable = body.indexable;
      if (body.ai_opt_out !== undefined) edit.aiOptOut = body.ai_opt_out;
      if (body.feed_enabled !== undefined) edit.feedEnabled = body.feed_enabled;
      if (body.follows_visibility !== undefined) edit.followsVisibility = body.follows_visibility;
      return { status: 200, body: credentialOf(viewer) };
    },
    "GET /accounts/{id}/followers": ({ params, query }) => followsOf(params.id, true, query),
    "GET /accounts/{id}/following": ({ params, query }) => followsOf(params.id, false, query),
    "POST /accounts/{id}/follow": ({ params }) => change(params.id, "follow", true),
    "DELETE /accounts/{id}/follow": ({ params }) => change(params.id, "follow", false),
    "POST /accounts/{id}/block": ({ params }) => change(params.id, "block", true),
    "DELETE /accounts/{id}/block": ({ params }) => change(params.id, "block", false),
    "POST /accounts/{id}/mute": ({ params }) => change(params.id, "mute", true),
    "DELETE /accounts/{id}/mute": ({ params }) => change(params.id, "mute", false),
    "PUT /accounts/{id}/note": ({ params, body }) => noted(params.id, body.note),
    "DELETE /accounts/{id}/note": ({ params }) => noted(params.id, ""),
    "POST /accounts/{id}/subscribe": ({ params }) => {
      const other = usernameOf.get(params.id);
      if (other === undefined) return notFound();
      subscribed.add(other);
      return { status: 200, body: relationshipOf(params.id) };
    },
    "DELETE /accounts/{id}/subscribe": ({ params }) => {
      const other = usernameOf.get(params.id);
      if (other === undefined) return notFound();
      subscribed.delete(other);
      return { status: 200, body: relationshipOf(params.id) };
    },
    "DELETE /accounts/{id}/follower": ({ params }) => {
      const other = usernameOf.get(params.id);
      if (other === undefined) return notFound();
      following.delete(`${other}>${viewer}`);
      return { status: 200, body: relationshipOf(params.id) };
    },
    "GET /follow_requests": ({ query }) => {
      // The accounts that ask to follow the reader, newest first.
      if (o.followRequests?.answer === "waits") return never();
      if (o.followRequests?.answer === "fails") return problem(500, "Internal Server Error");
      const asking = [...requested]
        .toReversed()
        .map((edge) => edge.split(">") as [string, string])
        .filter(([, to]) => to === viewer)
        .map(([from]) => accountOf(from));
      return accountPage(asking, query);
    },
    "POST /follow_requests/{id}/accept": ({ params }) => {
      const other = usernameOf.get(params.id);
      if (other === undefined || !requested.delete(`${other}>${viewer}`)) return notFound();
      following.add(`${other}>${viewer}`);
      return { status: 200, body: relationshipOf(params.id) };
    },
    "POST /follow_requests/{id}/reject": ({ params }) => {
      const other = usernameOf.get(params.id);
      if (other === undefined || !requested.delete(`${other}>${viewer}`)) return notFound();
      return { status: 200, body: relationshipOf(params.id) };
    },
    "GET /domain_mutes": ({ query }) => ({ status: 200, body: page(domainMutes, query, (m) => m.domain).body }),
    "POST /domain_mutes": ({ body }) => {
      // A remote domain, as the server normalizes it; this server's own is refused.
      const domain = body.domain.toLowerCase().replace(/\.$/, "");
      if (domain === fakeHost || !domain.includes(".")) return problem(422, "Unprocessable Entity");
      let mute = domainMutes.find((d) => d.domain === domain);
      if (mute === undefined) {
        mute = { domain, created_at: new Date().toISOString() };
        domainMutes.push(mute);
      }
      return { status: 200, body: mute };
    },
    "DELETE /domain_mutes/{domain}": ({ params }) => {
      const at = domainMutes.findIndex((d) => d.domain === params.domain.toLowerCase());
      if (at >= 0) domainMutes.splice(at, 1);
      return { status: 204 };
    },

    // ---- Statuses ---------------------------------------------------------------------------------------------------

    "POST /statuses": ({ body, request }) => ({ status: 201, body: once(request, () => publish(body)) }),
    "GET /statuses/search": ({ query }) => {
      // The public and local posts that hold every word, newest first, where the server has a search engine (ADR-0149).
      if (o.search?.posts !== true)
        return problem(404, "Posts cannot be searched on this server", {
          type: "/api/v1/problems/search-unavailable",
        });
      const words = (query.get("q") ?? "")
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
      return statusPage(shown, query);
    },
    "GET /statuses/{id}": ({ params }) => {
      // A status the reader made here is read alone with its body; a post of the seed, an article with its body.
      const own = made.get(params.id);
      if (own !== undefined) return { status: 200, body: own };
      const p = shownPost(params.id);
      // The post the reader opened is never left out: a filter that hides it in a thread folds it (ADR-0157 §6).
      const [opened] = p === undefined ? [] : judged([statusOf(p, 0, true)], "thread", true);
      return opened === undefined ? notFound() : { status: 200, body: opened };
    },
    "PUT /statuses/{id}": ({ params, body }) => {
      const p = shownPost(params.id);
      if (p === undefined) return notFound();
      if (p.author !== viewer) return problem(403, "Forbidden");
      // A post with a poll is never edited: its votes belong to the poll as posted. An article is edited with
      // PUT /articles/{id}.
      if (p.poll !== undefined || p.article !== undefined) return problem(422, "Unprocessable Entity");
      edits.set(p.key, [
        ...(edits.get(p.key) ?? []),
        {
          text: body.text,
          spoiler: body.spoiler_text ?? "",
          at: Date.now(),
          media: (body.media ?? []).map((x) => x.id),
        },
      ]);
      return { status: 200, body: statusOf(p) };
    },
    "DELETE /statuses/{id}": ({ params }) => {
      const own = made.get(params.id);
      if (own !== undefined) {
        made.delete(params.id);
        if (own.in_reply_to_id !== undefined && own.approval === undefined) countReply(own.in_reply_to_id, -1);
        const at = home.findIndex((s) => s.id === params.id);
        if (at >= 0) home.splice(at, 1);
        return { status: 204 };
      }
      const p = shownPost(params.id);
      if (p === undefined) return notFound();
      if (p.author !== viewer) return problem(403, "Forbidden");
      deleted.add(p.key);
      return { status: 204 };
    },
    "GET /statuses/{id}/source": ({ params }) => {
      // The source of a status the reader made here is what they wrote; that of another's post is a 404, as the
      // server's is.
      const own = made.get(params.id);
      if (own !== undefined) {
        const source = madeSources.get(params.id);
        const article = source?.article;
        const body = exact<Schema<"StatusSource">>()({
          id: params.id,
          text: source?.text ?? "",
          spoiler_text: source?.spoiler ?? "",
          language: own.language,
          ...(article === undefined
            ? {}
            : {
                article: exact<Schema<"ArticleSource">>()({
                  title: article.title,
                  summary: article.summary ?? "",
                  tags: [...(article.tags ?? [])],
                  ...(article.cover === undefined ? {} : { cover: article.cover }),
                }),
              }),
        });
        return { status: 200, body };
      }
      const p = shownPost(params.id);
      return p === undefined || p.author !== viewer ? notFound() : { status: 200, body: sourceOf(p) };
    },
    "GET /statuses/{id}/history": ({ params }) => {
      if (made.has(params.id)) return { status: 200, body: { items: [] } };
      const p = shownPost(params.id);
      if (p === undefined) return notFound();
      const id = statusId.get(p.key) ?? "";
      const items = versionsOf(p).map((v): Schema<"StatusEdit"> => ({
        content: serverHtml(v.text, fakeHost),
        spoiler_text: v.spoiler,
        sensitive: v.spoiler !== "",
        created_at: new Date(v.at).toISOString(),
        media_attachments: mediaOf(p, id, v),
        emojis: customEmojis,
      }));
      return { status: 200, body: { items } };
    },
    "GET /statuses/{id}/context": ({ params }) => {
      if (made.has(params.id)) return { status: 200, body: { ancestors: [], descendants: [] } };
      // The thread of a post the reader may not see is not there either.
      const key = shownPost(params.id)?.key;
      if (key === undefined) return notFound();
      const { ancestors, descendants } = contextOf(key);
      return {
        status: 200,
        body: { ancestors: judged(ancestors, "thread"), descendants: judged(descendants, "thread") },
      };
    },
    "POST /statuses/{id}/mute": ({ params }) =>
      acted(params.id, (p) => {
        mutedThreads.add(threadOf(p.key));
        return undefined;
      }),
    "DELETE /statuses/{id}/mute": ({ params }) =>
      acted(params.id, (p) => {
        mutedThreads.delete(threadOf(p.key));
        return undefined;
      }),
    "POST /statuses/{id}/bookmark": ({ params }) =>
      acted(params.id, (p) => {
        const at = bookmarks.indexOf(p.key);
        if (at >= 0) bookmarks.splice(at, 1);
        bookmarks.unshift(p.key);
        return undefined;
      }),
    "DELETE /statuses/{id}/bookmark": ({ params }) =>
      acted(params.id, (p) => {
        const at = bookmarks.indexOf(p.key);
        if (at >= 0) bookmarks.splice(at, 1);
        return undefined;
      }),
    "POST /statuses/{id}/boost": ({ params }) =>
      acted(params.id, (p) => {
        // A boost its author must approve waits (ADR-0155); one the policy denies is refused.
        const permission = permissionOf(p, "announce", viewer);
        if (permission === "denied") return problem(403, "Forbidden");
        if (permission === "pending") waitingBoosts.add(p.key);
        else ownBoost.set(p.key, true);
        return undefined;
      }),
    "DELETE /statuses/{id}/boost": ({ params }) =>
      acted(params.id, (p) => {
        waitingBoosts.delete(p.key);
        ownBoost.set(p.key, false);
        return undefined;
      }),
    "POST /statuses/{id}/favourite": ({ params }) =>
      acted(params.id, (p) => {
        keepOwn(p.key);
        ownFavourite.set(p.key, true);
        ownReaction.set(p.key, null);
        return undefined;
      }),
    "DELETE /statuses/{id}/favourite": ({ params }) =>
      acted(params.id, (p) => {
        keepOwn(p.key);
        ownFavourite.set(p.key, false);
        return undefined;
      }),
    "PUT /statuses/{id}/reaction": ({ params, body }) =>
      acted(params.id, (p) => {
        const named = seedEmoji(body.emoji);
        // A custom emoji of another server, or a shortcode this server has not, is refused as the server does.
        if (named.includes("@") || /^\w+$/u.test(named)) return problem(422, "Unprocessable Entity");
        keepOwn(p.key);
        ownReaction.set(p.key, named);
        ownFavourite.set(p.key, false);
        return undefined;
      }),
    "DELETE /statuses/{id}/reaction": ({ params }) =>
      acted(params.id, (p) => {
        keepOwn(p.key);
        ownReaction.set(p.key, null);
        return undefined;
      }),
    "POST /statuses/{id}/poll/votes": ({ params, body }) =>
      acted(params.id, (p) => {
        const poll = pollOf(p);
        if (poll !== undefined && poll.id !== body.poll_id) return problem(409, "Conflict");
        const { choices } = body;
        const taken =
          poll !== undefined &&
          poll.voted !== true &&
          choices.length > 0 &&
          (p.poll?.multiple === true || choices.length === 1) &&
          new Set(choices).size === choices.length &&
          choices.every((c) => Number.isInteger(c) && c >= 0 && c < (p.poll?.options.length ?? 0));
        if (!taken) return problem(422, "Unprocessable Entity");
        ownVotes.set(p.key, choices);
        return undefined;
      }),
    "GET /statuses/{id}/reactions": async ({ params, query }) => {
      const p = shownPost(params.id);
      if (p === undefined) return notFound();
      const emoji = query.get("emoji");
      // Who reacted, newest first; the cursor of a page is where its last reaction stands among them.
      const shown = reactionsOf(p.key)
        .filter((r) => emoji === null || reactionEmojiOf(r.emoji).name === emoji)
        .toReversed()
        .map((r) => ({
          at: idAt(timeOf(r.step), 70_000 + r.step),
          reactor: {
            account: accountOf(r.by),
            emoji: reactionEmojiOf(r.emoji),
            created_at: new Date(timeOf(r.step)).toISOString(),
          } satisfies Schema<"StatusReactor">,
        }));
      const { from, body } = page(shown, query, (r) => r.at);
      if (from > 0) await late();
      return { status: 200, body: { ...body, items: body.items.map((r) => r.reactor) } };
    },
    "POST /statuses/{id}/translation": ({ params, body }) => {
      const p = shownPost(params.id);
      return p === undefined ? notFound() : translationOf(p, body.target_language);
    },

    // ---- Notifications ----------------------------------------------------------------------------------------------

    "GET /notifications": async ({ query }) => {
      const types = query.all("types");
      // None from whom the reader blocks or mutes now, or from a server they mute; a notification of a post shows it
      // as it is now, and goes with it once it is deleted.
      const shown = notifications
        .filter((n) => types.length === 0 || types.includes(n.type))
        .filter((n) => !hidden(authorOf(n)) && !mutedServer(authorOf(n)))
        .flatMap(notificationNow);
      const { from, body } = page(shown, query);
      if (from > 0) await late();
      return { status: 200, body };
    },
    "GET /notifications/{id}": ({ params }) => {
      // One of the reader's notifications, as the list shows it; none of another or a dismissed one (a 404).
      const found = notifications.find((n) => n.id === params.id);
      if (found === undefined || hidden(authorOf(found)) || mutedServer(authorOf(found))) return notFound();
      const now = notificationNow(found)[0];
      return now === undefined ? notFound() : { status: 200, body: now };
    },
    "POST /notifications/{id}/dismiss": ({ params }) => {
      if (dismissFailures > 0) {
        dismissFailures -= 1;
        return problem(500, "Internal Server Error");
      }
      const at = notifications.findIndex((n) => n.id === params.id);
      if (at < 0) return notFound();
      notifications.splice(at, 1);
      return { status: 204 };
    },
    "GET /notifications/marker": () => ({ status: 200, body: markerNow() }),
    "PUT /notifications/marker": ({ body }) => {
      if (lastRead === undefined || body.last_read_id > lastRead) lastRead = body.last_read_id;
      return { status: 200, body: markerNow() };
    },
    "GET /notifications/requests/summary": () => {
      const summary: Schema<"NotificationRequestSummary"> = {
        accounts_count: held.size,
        notifications_count: [...held.values()].reduce((sum, list) => sum + list.length, 0),
      };
      return { status: 200, body: summary };
    },
    "GET /notifications/requests": async ({ query }) => {
      const { from, body } = page([...held.keys()].map(heldRequestOf), query);
      if (from > 0) await late();
      return { status: 200, body };
    },
    "GET /notifications/requests/{id}": ({ params }) => {
      const username = heldFrom(params.id);
      return username === undefined ? notFound() : { status: 200, body: heldRequestOf(username) };
    },
    "GET /notifications/requests/{id}/notifications": ({ params, query }) => {
      const username = heldFrom(params.id);
      if (username === undefined) return notFound();
      return { status: 200, body: page(held.get(username) ?? [], query).body };
    },
    "POST /notifications/requests/{id}/accept": ({ params }) => {
      const username = heldFrom(params.id);
      if (username === undefined) return notFound();
      notifications.push(...(held.get(username) ?? []));
      notifications.sort(newestFirst);
      held.delete(username);
      return { status: 204 };
    },
    "POST /notifications/requests/{id}/dismiss": ({ params }) => {
      const username = heldFrom(params.id);
      if (username === undefined) return notFound();
      held.delete(username);
      return { status: 204 };
    },
    "GET /push/subscription": () => {
      if (o.push !== true) return notFound();
      if (o.pushState === "subscription-fails") return problem(500, "Internal Server Error");
      return pushSubscription === undefined ? notFound() : { status: 200, body: pushSubscription };
    },
    "PUT /push/subscription": ({ body }) => {
      if (o.push !== true) return notFound();
      pushSubscription = {
        id: idAt(Date.now(), 70_000),
        endpoint: body.endpoint,
        alerts: body.alerts,
        server_key: pushServerKey,
      };
      return { status: 200, body: pushSubscription };
    },
    "DELETE /push/subscription": () => {
      if (o.push !== true) return notFound();
      pushSubscription = undefined;
      return { status: 204 };
    },
    ...personal.handlers,
    "GET /preferences/notifications": (call) => {
      if (o.pushState === "settings-fail") return problem(500, "Internal Server Error");
      return personal.handlers["GET /preferences/notifications"]?.(call) ?? notFound();
    },

    // ---- Direct messages --------------------------------------------------------------------------------------------

    "GET /conversations": async ({ query }) => {
      const { from, body } = page(
        conversationsNow()
          .filter((c) => !c.hidden)
          .map((c) => c.body),
        query,
      );
      if (from > 0) await late();
      return { status: 200, body };
    },
    "GET /conversations/{id}": ({ params }) => {
      const found = conversationOf(params.id);
      return found === undefined ? notFound() : { status: 200, body: found.body };
    },
    "POST /conversations/{id}/read": ({ params }) => {
      const found = conversationOf(params.id);
      if (found === undefined) return notFound();
      readTo.set(found.body.id, found.at);
      return { status: 200, body: { ...found.body, unread: false } };
    },
    "POST /conversations/{id}/hide": ({ params }) => {
      const found = conversationOf(params.id);
      if (found === undefined) return notFound();
      hiddenTo.set(found.body.id, found.at);
      return { status: 204 };
    },

    // ---- Approvals --------------------------------------------------------------------------------------------------

    "GET /interaction_requests": async ({ query }) => {
      // Those of the reader's posts that wait, newest first, of one kind when it is asked.
      const kind = query.get("kind");
      const waiting = requests
        .filter(
          (r) => postOf(r.target).author === viewer && stateOf(r) === "pending" && (kind === null || r.kind === kind),
        )
        .toSorted((a, b) => b.step - a.step)
        .map(requestBody);
      const { from, body } = page(waiting, query);
      if (from > 0) await late();
      return { status: 200, body };
    },
    "POST /interaction_requests/{id}/accept": ({ params }) => answered(params.id, "accepted"),
    "POST /interaction_requests/{id}/reject": ({ params }) => answered(params.id, "rejected"),

    // ---- Articles, uploads, drafts ----------------------------------------------------------------------------------

    "POST /articles/preview": ({ body }) => {
      // The body as the server would render it, with its size and images; one that fails, as the options say.
      if (o.articles?.preview === "fails") return problem(500, "Internal Server Error");
      const { text, summary } = body;
      const content = bodyHtml(text);
      const blank = summary === undefined || summary.trim() === "";
      const preview: Schema<"ArticlePreview"> = {
        content,
        summary: blank ? summaryOf(text) : summary.trim(),
        summary_from_body: blank,
        bytes: new TextEncoder().encode(content).length,
        max_bytes: articleLimits.max_bytes,
        images: bodyImages(text).filter((id) => uploads.has(id)).length,
        max_images: articleLimits.max_images,
      };
      return { status: 200, body: preview };
    },
    "POST /articles": ({ body, request }) => {
      if (o.articles?.authoring === "permission" && !permissionsOf(viewer).includes("create_articles"))
        return problem(403, "Forbidden");
      if (o.articles?.publish === "refused") return refusedArticle();
      if (o.articles?.publish === "summary") return refusedSummary();
      const { title, summary, tags, cover, text, visibility, language, sensitive } = body;
      const article: WrittenArticle = {
        title,
        ...(summary === undefined ? {} : { summary }),
        ...(tags === undefined ? {} : { tags }),
        ...(cover === undefined ? {} : { cover }),
      };
      return {
        status: 201,
        body: once(request, () =>
          publish({
            text,
            visibility,
            article,
            ...(language === undefined ? {} : { language }),
            ...(sensitive === undefined ? {} : { sensitive }),
          }),
        ),
      };
    },
    "PUT /articles/{id}": ({ params, body }) => {
      if (o.articles?.publish === "refused") return refusedArticle();
      if (o.articles?.publish === "summary") return refusedSummary();
      const after = editMade(params.id, body);
      if (after !== undefined) return { status: 200, body: after };
      const p = shownPost(params.id);
      if (p === undefined) return notFound();
      if (p.author !== viewer) return problem(403, "Forbidden");
      if (p.article === undefined) return problem(422, "Unprocessable Entity");
      articleEdits.set(p.key, { title: body.title, summary: body.summary ?? "", tags: body.tags ?? [] });
      edits.set(p.key, [...(edits.get(p.key) ?? []), { text: body.text, spoiler: "", at: Date.now() }]);
      return { status: 200, body: statusOf(p, 0, true) };
    },
    "POST /media": async ({ request }) => {
      // An image the reader uploads, served from the reader's own copy of the file: ready at once, or still processed
      // until it is read again (`uploads`); or refused, as the server refuses a file it cannot read as an image: the
      // problem of that type, whose title says it, with no detail (internal/interface/clientapi/media.go).
      if (o.uploads === "refused")
        return problem(422, "The file could not be read as an image. Choose another file.", {
          type: "/api/v1/problems/media-corrupt",
        });
      const form = await request.formData();
      const file = form.get("file");
      const description = form.get("description");
      uploaded += 1;
      const id = idAt(Date.now(), 990_000 + uploaded);
      const url = file instanceof Blob ? URL.createObjectURL(file) : "";
      const media = exact<MediaAttachment>()({
        id,
        type: "image",
        state: "ready",
        ...(typeof description === "string" && description !== "" ? { description } : {}),
        url,
        preview_url: url,
        meta: { width: 1200, height: 800, ...centered },
      });
      uploads.set(id, media);
      const waiting: MediaAttachment = { id, type: "image", state: "processing", meta: centered };
      return { status: 202, body: o.uploads === "processing" ? waiting : media };
    },
    "GET /media/{id}": ({ params }) => {
      // An upload of the reader's, processed by now, or an image of a draft of the seed.
      const image = draftImages.get(params.id);
      const media = uploads.get(params.id) ?? (image === undefined ? undefined : imageOf(params.id, image));
      return media === undefined ? notFound() : { status: 200, body: media };
    },
    "GET /drafts": ({ query }) => {
      const scheduled = query.get("scheduled");
      const shown = ownDrafts.filter(
        (d) => scheduled === null || (d.scheduled_at !== undefined) === (scheduled === "true"),
      );
      return { status: 200, body: page(shown, query).body };
    },
    "POST /drafts": ({ body }) => {
      draftsMade += 1;
      const now = new Date().toISOString();
      const draft = savedDraft({ id: idAt(Date.now(), 85_000 + draftsMade), created_at: now, updated_at: now }, body);
      ownDrafts.unshift(draft);
      return draftAnswer(201, draft);
    },
    "GET /drafts/{id}": ({ params }) => {
      const d = draftOf(params.id);
      return d === undefined ? notFound() : draftAnswer(200, d);
    },
    "PUT /drafts/{id}": ({ params, body, request }) => {
      const d = draftOf(params.id);
      if (d === undefined) return notFound();
      // A save over a version another device replaced since is refused, as the server does (ADR-0156 §9).
      if (request.headers.get("If-Match") !== etagOf(d)) return problem(412, "Precondition Failed");
      const saved = savedDraft(d, body);
      putDraft(saved);
      return draftAnswer(200, saved);
    },
    "DELETE /drafts/{id}": ({ params }) => {
      const d = draftOf(params.id);
      if (d === undefined) return notFound();
      ownDrafts.splice(ownDrafts.indexOf(d), 1);
      return { status: 204 };
    },
    "PUT /drafts/{id}/schedule": ({ params, body }) => {
      const d = draftOf(params.id);
      if (d === undefined) return notFound();
      const lead = Date.parse(body.publish_at) - Date.now();
      if (lead < draftLimits.scheduled_post_min_lead_minutes * minute)
        return unprocessable(
          `A post can be scheduled no sooner than ${String(draftLimits.scheduled_post_min_lead_minutes)} minutes from now.`,
        );
      if (lead > draftLimits.scheduled_post_max_lead_days * day)
        return unprocessable(
          `A post can be scheduled no later than ${String(draftLimits.scheduled_post_max_lead_days)} days from now.`,
        );
      const scheduled: Draft = { ...d, scheduled_at: new Date(Date.parse(body.publish_at)).toISOString() };
      putDraft(scheduled);
      return draftAnswer(200, scheduled);
    },
    "DELETE /drafts/{id}/schedule": ({ params }) => {
      const d = draftOf(params.id);
      if (d === undefined) return notFound();
      if (d.scheduled_at === undefined) return problem(409, "Conflict");
      const { scheduled_at: at, ...rest } = d;
      const unscheduled: Draft = {
        ...rest,
        last_schedule: { publish_at: at, outcome: "cancelled", occurred_at: new Date().toISOString() },
      };
      putDraft(unscheduled);
      return draftAnswer(200, unscheduled);
    },
    "POST /drafts/{id}/publish": ({ params }) => {
      const d = draftOf(params.id);
      if (d === undefined) return notFound();
      // The review refuses an article published from its draft as it refuses one posted at once; the draft stays.
      if (d.article !== undefined && o.articles?.publish === "refused") return refusedArticle();
      ownDrafts.splice(ownDrafts.indexOf(d), 1);
      return { status: 201, body: publish(d) };
    },

    // ---- The voice chat rooms -----------------------------------------------------------------------------------------

    ...voiceRooms.handlers,

    // ---- The events ---------------------------------------------------------------------------------------------------

    ...events.handlers,

    // ---- The supporters and the finance -------------------------------------------------------------------------------

    ...about.handlers,
    ...supporting.handlers,
    ...finance.handlers,
    ...financeAdmin.handlers,
    ...supportersAdmin.handlers,

    // ---- The moderators' reports ------------------------------------------------------------------------------------

    ...moderated.handlers,
    ...accounts.handlers,

    // ---- The domains, the remote servers and the audit log ------------------------------------------------------------

    ...federated.handlers,

    // ---- The administration of roles, custom emoji, announcements and webhooks ----------------------------------------

    ...staffServer.handlers,
    // The roles a story of the supporters' administration names its plans on (supporters-admin.ts), where it asks
    // for that administration; the administration of roles' own (staff-roles.ts) everywhere else.
    ...supportersAdmin.roles,
    // ---- The administration's settings, texts, rules, relays, login providers and OAuth apps -------------------------

    ...administered.handlers,

    // ---- The operations: jobs, error groups, UX measurements, the link card crawler's rules ------------------------

    ...operated.handlers,

    // ---- The instance theme and the closure of the service --------------------------------------------------------

    ...theming.handlers,
    ...closing.handlers,

    // ---- How the reader signs in, the browsers and apps of the account, their own apps and invites ----------------------

    ...accessHandlers({
      options: o.access ?? {},
      viewer: { username: viewer, password: personOf(viewer).password },
      started,
      origin: fakeOrigin,
      singleUser: o.singleUser === true,
      registrationClosed: instanceBody.registrations.mode === "closed",
      problem,
      notFound,
      idAt,
      page,
    }),
  };

  /**
   * Keeps what the reader has now of the post of `key` as theirs, before a write changes it: one favourite or one
   * reaction per post (FR-116), so setting one takes back the other, and taking back one leaves the other as it was.
   */
  function keepOwn(key: string) {
    const favourited = favouritersOf(key).includes(viewer);
    const reacted = reactionsOf(key).find((r) => r.by === viewer)?.emoji ?? null;
    ownFavourite.set(key, favourited);
    ownReaction.set(key, reacted);
  }

  // The seed's people are all this server's, so both public timelines hold their public posts (the local one also
  // those for this server only), newest first; a timeline the instance does not offer is a 404, one for signed-in
  // readers a 401 to a visitor.
  function publicTimeline(kind: "local" | "federated", query: PageQuery): Promise<Answer<"GET /timelines/local">> {
    if (timelines[kind] === "off") return Promise.resolve(notFound());
    if (timelines[kind] === "signed_in" && o.visitor === true) return Promise.resolve(problem(401, "Unauthorized"));
    const shown = posts
      .filter((p) => !deleted.has(p.key))
      .filter((p) => p.visibility === "public" || (kind === "local" && p.visibility === "local"))
      .filter((p) => o.visitor === true || (!hidden(p.author) && !mutedServer(p.author)))
      .toReversed()
      .map((p) => statusOf(p));
    return statusPage(judged(shown, "public"), query);
  }

  const markerNow = (): Schema<"NotificationMarker"> =>
    exact<Schema<"NotificationMarker">>()({
      ...(lastRead === undefined ? {} : { last_read_id: lastRead }),
      unread_count: unreadCount(),
    });

  /** The username of the account `id` whose notifications are held back, or none. */
  const heldFrom = (id: string) => {
    const username = usernameOf.get(id);
    return username === undefined || !held.has(username) ? undefined : username;
  };

  /** The reader's answer to the request `id` of one of their posts. */
  function answered(id: string, answer: "accepted" | "rejected"): Answer<"POST /interaction_requests/{id}/accept"> {
    const r = requests.find((x) => requestId(x) === id);
    if (r === undefined || postOf(r.target).author !== viewer) return notFound();
    if (stateOf(r) !== "pending") return problem(409, "Conflict");
    answers.set(r.key, answer);
    return { status: 200, body: requestBody(r) };
  }

  /**
   * The operations a visitor without a session may call, as the specification opens them (`security: {}`); every other
   * answers 401: the reader's own account, their relationships, the searches and the suggestions among them, whose
   * paths read like an account's, a status's or a hashtag's.
   */
  const publicPath =
    /^\/(?:instance(?:\/health|\/supporter_plans|\/supporters|\/tip_amounts|\/cost_totals|\/income_totals|\/documents\/[^/]+|\/domain_actions)?|custom_emojis|accounts\/lookup|accounts\/(?!(?:verify_credentials|search|relationships|suggestions)$)[^/]+(?:\/statuses|\/featured_tags|\/followers|\/following)?|statuses\/(?!search$)[^/]+(?:\/context|\/history|\/reactions)?|timelines\/(?:local|federated|tag\/[^/]+)|tags\/(?!search$)[^/]+|voice\/rooms\/[^/]+\/summary)$/;
  /** Whether `/instance` answered once, before the server stopped answering (`down`). */
  let instanceRead = false;

  const json = (body: unknown, status: number, headers: Record<string, string> = {}) =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": status >= 400 ? "application/problem+json" : "application/json", ...headers },
    });
  const health = (body: Schema<"ServerHealth">, headers: Record<string, string> = {}) =>
    json(body, 503, { "Content-Type": "application/json", ...headers });

  const unimplemented: string[] = [];
  const refused: string[] = [];
  const route = createRouter(
    handlers,
    (error) => {
      unimplemented.push(error.operation);
    },
    o.staffPage,
  );

  const revocations: { clientId: string; token: string }[] = [];

  /** The answer of the revocation endpoint (RFC 7009), outside the client API: 200 with no body, the form kept. */
  async function revoked(request: Request): Promise<Response> {
    if (o.revocation === "fails") return json(problem(503, "Service Unavailable").problem, 503);
    const form = new URLSearchParams(await request.text());
    revocations.push({ clientId: form.get("client_id") ?? "", token: form.get("token") ?? "" });
    return new Response(null, { status: 200, headers: { "Cache-Control": "no-store" } });
  }

  /** The answer of the server in the state of the options: down, under maintenance, or as it is. */
  async function served(request: Request): Promise<Response> {
    const { pathname } = new URL(request.url);
    const path = pathname.replace(/^\/api\/v1/, "");
    const down = o.down !== undefined && (path !== "/instance" || instanceRead);
    if (path === "/instance") instanceRead = true;
    if (down && o.down === "offline") throw new TypeError("Failed to fetch");
    if (!down && o.maintenance !== true && request.method === "POST" && pathname === "/oauth/revoke")
      return revoked(request);
    if (down && o.down === "outage")
      return path === "/instance/health"
        ? health({ status: "unavailable", checked_at: new Date().toISOString() })
        : json(problem(503, "Service Unavailable").problem, 503);
    // The maintenance problem carries the operator's message, its end and the status page beside a problem's members.
    if (o.maintenance === true)
      return path === "/instance/health"
        ? health({ status: "maintenance" }, { "Retry-After": "3600" })
        : json(
            {
              ...problem(503, "Under maintenance", { type: "/api/v1/problems/maintenance" }).problem,
              message: "Moving the database to a larger disk. Thanks for your patience! — the admins",
              until: new Date(started + 2 * 60 * minute).toISOString(),
              status_page_url: fakeStatusPage,
            },
            503,
            { "Retry-After": "3600" },
          );
    if (o.visitor === true && !publicPath.test(path)) {
      refused.push(`${request.method} ${path}`);
      return json(problem(401, "Unauthorized").problem, 401);
    }
    const response = await route(request);
    if (o.newerEpoch === true) response.headers.set("X-Web-Client-Epoch", "999");
    return response;
  }

  const scripts = createScripts(Object.keys(handlers) as Route[], served);

  let added = 0;

  function newPosts(count: number): Status[] {
    const pool = posts.filter((p) => p.author !== viewer && inHome(p.author) && !hidden(p.author) && visible(p));
    const fresh = Array.from({ length: count }, () => {
      added += 1;
      const p = pool[(added - 1) % pool.length];
      if (p === undefined) throw new Error("fake: the reader follows no one who posts");
      const at = Date.now();
      return { ...statusOf(p), id: idAt(at, 900_000 + added), created_at: new Date(at).toISOString() };
    }).toReversed();
    home.unshift(...fresh);
    // The stream sends each as the home timeline shows it: folded by a filter, or not at all (ADR-0157 §7).
    for (const status of judged(fresh.toReversed(), "home")) send({ type: "update", status });
    return fresh;
  }

  function newNotification(): Notification {
    added += 1;
    const fans = people.filter((p) => follows_(p.username, viewer) && !hidden(p.username));
    const from = fans[added % Math.max(1, fans.length)]?.username ?? viewer;
    const own = posts.find((p) => p.author === viewer);
    const at = Date.now();
    const notification = exact<Notification>()({
      id: idAt(at, 900_000 + added),
      type: "favourite",
      created_at: new Date(at).toISOString(),
      account: accountOf(from),
      ...(own === undefined ? {} : { status: statusOf(own) }),
    });
    notifications.unshift(notification);
    send({ type: "notification", notification });
    return notification;
  }

  function newMessage(index: number): Status {
    const held = conversationsNow()[index];
    const last = posts.find((p) => statusId.get(p.key) === held?.at);
    const from = usernameOf.get(held?.body.accounts[0]?.id ?? "");
    if (held === undefined || last === undefined || from === undefined)
      throw new Error(`fake: ${viewer} has no conversation ${String(index)} with another person`);
    added += 1;
    const message: Post = {
      key: `message-${String(added)}`,
      author: from,
      replyTo: last.key,
      text: `@${viewer} One more thing, number ${String(added)}.`,
      visibility: "direct",
      language: "en",
    };
    posts.push(message);
    const id = idAt(timeOf(postStep(message.key)), postStep(message.key));
    statusId.set(message.key, id);
    keyOf.set(id, message.key);
    const now = conversationOf(held.body.id);
    if (now !== undefined) send({ type: "conversation", conversation: now.body });
    return statusOf(message);
  }

  const postPath = (key: string) => `/@${postOf(key).author}/status/${statusId.get(key) ?? ""}`;
  const announcementPath = (key: string) =>
    `/announcements/${key === "ended" ? endedAnnouncement.id : announcementId(key)}`;

  const listPath = (index: number) => `/lists/${lists[index]?.id ?? "none"}`;
  const filterPath = (index: number) => `/settings/filters/${personal.filterId(index) ?? "none"}`;
  const actionPath = (index: number) => `/settings/account/status/${personal.actionId(index) ?? "none"}`;

  const keyedId = (key: string) => {
    const post = statusId.get(key);
    if (post !== undefined) return post;
    const at = seedDrafts.findIndex((d) => d.key === key);
    if (at < 0) throw new Error(`fake: no post or draft ${key} of ${viewer}`);
    return draftId(at);
  };

  const conversationPath = (index: number) => `/conversations/${conversationsNow()[index]?.body.id ?? "none"}`;

  const heldPath = (username: string) => `/notifications/requests/${idOf(username)}`;
  const notificationPath = (type: string) =>
    `/notifications/${notifications.find((n) => n.type === type)?.id ?? "none"}`;

  return {
    serve: scripts.serve,
    script: scripts.script,
    send,
    voice: voiceRooms.voice,
    events: events.events,
    supporters: supporting.supporters,
    reports: moderated.reports,
    domains: federated.domains,
    staffServer: staffServer.server,
    financeAdmin: financeAdmin.admin,
    supportersAdmin: supportersAdmin.admin,
    admin: administered.admin,
    moderation: accounts.moderation,
    operations: operated.operations,
    unimplemented,
    refused,
    revocations,
    newPosts,
    newNotification,
    newMessage,
    postPath,
    announcementPath,
    listPath,
    filterPath,
    actionPath,
    idOf: keyedId,
    accountId: (username) => idOf(personOf(username).username),
    conversationPath,
    heldPath,
    notificationPath,
    loginProviderId: (kind) => loginProviderIds(started, idAt)[kind],
    appId: (list, index) => appIds(started, idAt)[list][index] ?? idAt(started, 74_990),
    inviteId: (index) => inviteIds(started, idAt)[index] ?? idAt(started, 76_990),
  };
}
