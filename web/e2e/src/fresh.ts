// SPDX-License-Identifier: AGPL-3.0-only

// What an E2E test makes for itself on the demo server, so that it changes nothing another test or a second run of it
// reads (`--repeat-each`): a person, a post, a follow request. Each is made as the seed makes the scenario (seed.ts):
// the person with `bin/my admin create`, the rest with the client API through @mycroblog/mykit, signed in as each
// person, and no SQL. Each name holds a part of a random UUID, so that it is unique to the test and the run without
// reading the clock.

import { randomBytes, randomUUID } from "node:crypto";
import { all, ProblemError, type Api, type components } from "@mycroblog/mykit";
import type { Drawing } from "./drawing";
import { silentMP3, tinyWebM } from "./av";
import {
  admin,
  approvalDomain,
  people,
  personOf,
  type Image,
  type Person,
  type Poll,
  type Visibility,
} from "./scenario";
import { enrollTOTP, uploadDrawing, uploadFile } from "./seed/seed";
import { signIn } from "./seed/sign-in";

type Status = components["schemas"]["Status"];
type Account = components["schemas"]["Account"];
type MediaAttachment = components["schemas"]["MediaAttachment"];

/** A person a test made: a Person, and with two-factor authentication its secret and the step its enrollment used. */
export type FreshPerson = Person & { totp?: { secret: string; step: number } };

export type FreshPersonOptions = {
  /** Whether a follow waits for the person's approval. */
  locked?: boolean;
  /** Enables two-factor authentication (TOTP) last, after the follows. */
  totp?: boolean;
  /** The people the person follows, accepted at once when one is locked. */
  follows?: readonly Person[];
  /** The people who follow the person, accepted at once when the person is locked. */
  followedBy?: readonly Person[];
  /** Keeps the administrator role that `my admin create` gives, for a test of the administrators' API. */
  admin?: boolean;
  /** Who may read the person's lists of follows and followers (FR-311); everyone by default. */
  followsVisibility?: components["schemas"]["FollowsVisibility"];
  /**
   * A role the person holds, made by the scenario's administrator the first time a test asks for its key (at place 1,
   * with `permissions`), and granted to the person: the same key with the same permissions in every test that names
   * it (`create_custom_emojis`: a person who asks for custom emoji).
   */
  role?: { key: string; permissions: readonly components["schemas"]["Permission"][] };
};

/**
 * A video or a sound to attach (av.ts): a two-second WebM with `poster` as its picture, or a silent MP3 of about three
 * seconds; `description` is its alt text, empty for none.
 */
type FreshFile = { kind: "video"; description: string; poster: Drawing } | { kind: "audio"; description: string };

export type FreshPostOptions = {
  /** The text; the post's own text ends with a word unique to it, so that a spec finds it alone. */
  text: string;
  visibility?: Visibility;
  image?: Image;
  /** More images, after `image`. */
  images?: readonly Image[];
  /** Videos and sounds, after the images. */
  files?: readonly FreshFile[];
  /** Its media are sensitive: covered until the reader shows them. */
  sensitive?: boolean;
  /** The content warning. */
  spoiler?: string;
  /** A poll, which ends `days` days after the post. */
  poll?: Poll;
  /** The language the post is written in (a BCP 47 tag); the author's locale by default. */
  language?: string;
  /** The ID of the post it replies to. */
  replyTo?: string;
  /**
   * Who may reply to, boost and quote it (ADR-0155): for each kind given, whom it admits at once (`automatic`) and by
   * the author's approval (`manual`); the author's defaults for the others.
   */
  policy?: components["schemas"]["InteractionPolicyChange"];
};

/**
 * A post a test made: its ID, its whole text, by which a spec finds it on the screen, and its media as the server
 * describes them, in order.
 */
export type FreshPost = { id: string; text: string; media: readonly MediaAttachment[] };

/** A draft of a test's: its ID, and its whole text, which finds it on the screen. */
export type FreshDraft = { id: string; text: string };

/** Someone the server does not know yet: what they sign up with on the registration screen. */
export type Newcomer = { username: string; email: string; password: string };

/**
 * Someone the login provider knows and the server does not yet: the subject the provider names them by, and the email
 * address, name and username it tells of them.
 */
export type ProviderIdentity = { subject: string; email: string; name: string; username: string };

/** An invite of the staff: its code, and the address of the registration screen with the code filled in. */
export type FreshInvite = { code: string; url: string };

/** A person's avatar: a plain square, which no screen of a test reads. */
const avatar: Drawing = { width: 64, height: 64, top: [120, 144, 168], bottom: [72, 96, 120], shapes: [] };

/**
 * An address of 198.18.0.0/15 (for tests) that a person's requests come from, as each person comes from their own:
 * the server limits sign-ins and tokens per client IP, and the tests' people would otherwise share one limit.
 */
function clientIP(): string {
  const [a = 0, b = 0, c = 0] = randomBytes(3);
  return `198.${String(18 + (a & 1))}.${String(b)}.${String(c)}`;
}

/** A part of a random UUID: unique to a test and a run, and short enough for a username. */
const unique = () => randomUUID().replaceAll("-", "").slice(0, 16);

function body<T>(data: T | undefined, what: string): T {
  if (data === undefined) throw new Error(`fresh: ${what} answered without a body`);
  return data;
}

export type FreshOptions = {
  /** The origin of the demo server. */
  server: string;
  /** Runs `bin/my` with `args` and `input` on its standard input against the server's database. */
  runMy: (args: readonly string[], input?: string) => string;
  /**
   * The TOTP secrets of the scenario's people who have two-factor authentication and the steps their enrollments used,
   * by username (the seed's state), to sign them in.
   */
  totp?: { secrets: Readonly<Record<string, string>>; steps: Readonly<Record<string, number>> };
};

/** Makes the subjects of one test. */
export class Fresh {
  readonly #o: FreshOptions;
  readonly #apis = new Map<string, Api>();

  constructor(o: FreshOptions) {
    this.#o = o;
  }

  async #api(p: Person | FreshPerson): Promise<Api> {
    let api = this.#apis.get(p.username);
    if (api === undefined) {
      const own = "totp" in p ? p.totp : undefined;
      const secret = own?.secret ?? this.#o.totp?.secrets[p.username];
      api = await signIn(this.#o.server, p, secret, own?.step ?? this.#o.totp?.steps[p.username], clientIP());
      this.#apis.set(p.username, api);
    }
    return api;
  }

  async #account(p: Person): Promise<Account> {
    const { data } = await (await this.#api(p)).GET("/accounts/lookup", { params: { query: { acct: p.username } } });
    return body(data, `the lookup of ${p.username}`);
  }

  /** `follower` follows `followee`, and `followee` accepts it when they are locked and `accept` is set. */
  async #follow(follower: Person, followee: Person, accept: boolean) {
    const target = await this.#account(followee);
    const { error } = await (
      await this.#api(follower)
    ).POST("/accounts/{id}/follow", { params: { path: { id: target.id } } });
    if (error !== undefined) throw new Error(`fresh: ${follower.username} could not follow ${followee.username}`);
    if (!accept || followee.locked !== true) return;
    const source = await this.#account(follower);
    const accepted = await (
      await this.#api(followee)
    ).POST("/follow_requests/{id}/accept", { params: { path: { id: source.id } } });
    if (accepted.error !== undefined)
      throw new Error(`fresh: ${followee.username} could not accept the follow of ${follower.username}`);
  }

  /** A new person with a password, a display name unique to them, and what `o` asks for. */
  async person(o: FreshPersonOptions = {}): Promise<FreshPerson> {
    const id = unique();
    const username = `fresh_${id}`;
    const p: FreshPerson = {
      username,
      email: `${username}@fresh.test`,
      password: `fresh-password-${id}`,
      displayName: `Fresh ${id.slice(0, 8)}`,
      note: "",
      fields: [],
      locale: "en",
      avatar,
      ...(o.locked === true ? { locked: true } : {}),
      ...(o.totp === true ? { twoFactor: true } : {}),
    };
    this.#o.runMy(["admin", "create", "-username", p.username, "-email", p.email], `${p.password}\n`);
    // `my admin create` makes an administrator; a person of a test is none unless it asks to be.
    if (o.admin === true) p.staff = "admin";
    else this.#o.runMy(["admin", "role", "revoke", p.username, "admin"]);
    const api = await this.#api(p);
    const { error } = await api.PATCH("/accounts/update_credentials", {
      body: {
        display_name: p.displayName,
        locked: o.locked === true,
        ...(o.followsVisibility === undefined ? {} : { follows_visibility: o.followsVisibility }),
      },
    });
    if (error !== undefined) throw new Error(`fresh: the profile of ${p.username} was refused`);
    for (const followee of o.follows ?? []) await this.#follow(p, followee, true);
    for (const follower of o.followedBy ?? []) await this.#follow(follower, p, true);
    if (o.role !== undefined) await this.#grant(p, o.role);
    if (o.totp === true) p.totp = await enrollTOTP(api);
    return p;
  }

  /** A new post of `author`, whose text ends with a word unique to it. */
  async post(author: Person, o: FreshPostOptions): Promise<FreshPost> {
    const id = unique();
    const api = await this.#api(author);
    const media: MediaAttachment[] = [];
    for (const image of [...(o.image === undefined ? [] : [o.image]), ...(o.images ?? [])])
      media.push(await uploadDrawing(api, image.name, image.drawing, image.description, "attachment"));
    for (const [i, file] of (o.files ?? []).entries()) {
      const name = `${file.kind}-${String(i)}.${file.kind === "video" ? "webm" : "mp3"}`;
      const [bytes, type] = file.kind === "video" ? [tinyWebM(), "video/webm"] : [silentMP3(3), "audio/mpeg"];
      const poster = file.kind === "video" ? file.poster : undefined;
      media.push(await uploadFile(api, name, new Blob([bytes], { type }), file.description, "attachment", poster));
    }
    const text = `${o.text} ${id.slice(0, 8)}`;
    const { data } = await api.POST("/statuses", {
      params: { header: { "Idempotency-Key": `fresh:${id}` } },
      body: {
        text,
        visibility: o.visibility ?? "public",
        language: o.language ?? author.locale,
        ...(o.replyTo === undefined ? {} : { in_reply_to_id: o.replyTo }),
        ...(o.policy === undefined ? {} : { interaction_policy: o.policy }),
        ...(o.spoiler === undefined ? {} : { spoiler_text: o.spoiler }),
        ...(o.sensitive === true ? { sensitive: true } : {}),
        ...(media.length === 0 ? {} : { media: media.map((m) => ({ id: m.id, description: m.description ?? "" })) }),
        ...(o.poll === undefined
          ? {}
          : {
              poll: {
                options: [...o.poll.options],
                expires_in: o.poll.days * 86_400,
                multiple: o.poll.multiple ?? false,
              },
            }),
      },
    });
    const status: Status = body(data, "the post");
    return { id: status.id, text, media: status.media_attachments };
  }

  /**
   * A new draft of `author` on the server, whose text ends with a word unique to it, scheduled `inSeconds` from now
   * when given (the server takes no time nearer than its `scheduled_post_min_lead_minutes`).
   */
  async draft(author: Person, o: { text: string; inSeconds?: number }): Promise<FreshDraft> {
    const api = await this.#api(author);
    const text = `${o.text} ${unique().slice(0, 8)}`;
    const { data } = await api.POST("/drafts", { body: { text, visibility: "public", language: author.locale } });
    const draft = body(data, "the draft");
    if (o.inSeconds !== undefined) {
      const { error } = await api.PUT("/drafts/{id}/schedule", {
        params: { path: { id: draft.id } },
        body: { publish_at: new Date(Date.now() + o.inSeconds * 1000).toISOString() },
      });
      if (error !== undefined) throw new Error(`fresh: the schedule of ${draft.id} was refused`);
    }
    return { id: draft.id, text };
  }

  /**
   * A new voice chat room that `host` opens, for every local user, where everyone may speak without asking; its ID.
   * The host joins it with a session of the API, which their browser's join takes over (one place per person).
   */
  async voiceRoom(host: Person, o: { name: string }): Promise<{ id: string; name: string }> {
    const name = `${o.name} ${unique().slice(0, 8)}`;
    const { data } = await (await this.#api(host)).POST("/voice/rooms", { body: { name, speakers: "everyone" } });
    return { id: body(data, "the room").room.id, name };
  }

  /**
   * A new event for every local user that `organizer` organizes, whose title is `title` and a word unique to it, two
   * hours long: `inDays` days from now (3 by default), or, with `candidates`, a date poll of a date that many days from
   * now each. Its ID, its title, and the start of each candidate, in their order.
   */
  async event(
    organizer: Person,
    o: { title: string; inDays?: number; candidates?: readonly number[] },
  ): Promise<{ id: string; title: string; candidates: readonly Date[] }> {
    const title = `${o.title} ${unique().slice(0, 8)}`;
    const now = Date.now();
    const startOf = (days: number) => new Date(now + days * 86_400_000);
    const span = (days: number) => ({
      starts_at: startOf(days).toISOString(),
      ends_at: new Date(startOf(days).getTime() + 2 * 3_600_000).toISOString(),
    });
    const { data } = await (
      await this.#api(organizer)
    ).POST("/events", {
      body: {
        title,
        time_zone: "UTC",
        ...(o.candidates === undefined ? span(o.inDays ?? 3) : { candidates: o.candidates.map(span) }),
      },
    });
    return { id: body(data, "the event").id, title, candidates: (o.candidates ?? []).map(startOf) };
  }

  /** The evenings (18:00 of the test's zone) `inDays` days from now, each, for a form a spec fills with dates. */
  evenings(inDays: readonly number[]): Date[] {
    const now = Date.now();
    return inDays.map((days) => {
      const evening = new Date(now + days * 86_400_000);
      evening.setHours(18, 0, 0, 0);
      return evening;
    });
  }

  /** `by` reacts to the post `post` with `emoji`: a Unicode emoji, or a custom one's shortcode without colons. */
  async reaction(by: Person, post: FreshPost, emoji: string): Promise<void> {
    const { error } = await (
      await this.#api(by)
    ).PUT("/statuses/{id}/reaction", { params: { path: { id: post.id } }, body: { emoji } });
    if (error !== undefined) throw new Error(`fresh: ${by.username} could not react to ${post.id}`);
  }

  /** The ID of `p`'s newest notification of `type`, as the server lists them. */
  async notificationOf(p: Person, type: "mention" | "favourite" | "follow"): Promise<string> {
    const { data } = await (await this.#api(p)).GET("/notifications", { params: { query: { types: [type] } } });
    const id = body(data, `the notifications of ${p.username}`).items[0]?.id;
    if (id === undefined) throw new Error(`fresh: ${p.username} has no ${type} notification`);
    return id;
  }

  /** `by` favourites the post `post`. */
  async favourite(by: Person, post: FreshPost): Promise<void> {
    const { error } = await (
      await this.#api(by)
    ).POST("/statuses/{id}/favourite", { params: { path: { id: post.id } } });
    if (error !== undefined) throw new Error(`fresh: ${by.username} could not favourite ${post.id}`);
  }

  /**
   * The notification policy of `p` (ADR-0158): what happens to the notifications of the sources that meet each
   * condition given; the others keep theirs.
   */
  async notificationPolicy(
    p: Person,
    policy: Partial<Record<"not_following" | "not_follower" | "new_account", "accept" | "filter" | "drop">>,
  ): Promise<void> {
    const { error } = await (await this.#api(p)).PATCH("/preferences/notification_policy", { body: policy });
    if (error !== undefined) throw new Error(`fresh: the notification policy of ${p.username} was refused`);
  }

  /** `by` boosts the post `post`: at once, or as a request where its author approves `by`'s boosts by hand. */
  async boost(by: Person, post: FreshPost): Promise<void> {
    const { error } = await (await this.#api(by)).POST("/statuses/{id}/boost", { params: { path: { id: post.id } } });
    if (error !== undefined) throw new Error(`fresh: ${by.username} could not boost ${post.id}`);
  }

  /** The text and the content warning of `author`'s post `id` as the server keeps them (its source). */
  async source(author: Person, id: string): Promise<{ text: string; spoiler: string }> {
    const { data } = await (await this.#api(author)).GET("/statuses/{id}/source", { params: { path: { id } } });
    const source = body(data, `the source of ${id}`);
    return { text: source.text, spoiler: source.spoiler_text };
  }

  /** `author` deletes their post `post`. */
  async deletion(author: Person, post: FreshPost): Promise<void> {
    const { error } = await (await this.#api(author)).DELETE("/statuses/{id}", { params: { path: { id: post.id } } });
    if (error !== undefined) throw new Error(`fresh: ${author.username} could not delete ${post.id}`);
  }

  /**
   * `from` moves their account to `to`, both of this server (ADR-0089 §3): `to` names `from` as an alias first, then
   * `from` moves with their password.
   */
  async move(from: FreshPerson, to: FreshPerson): Promise<void> {
    const aliased = await (await this.#api(to)).POST("/accounts/aliases", { body: { acct: from.username } });
    if (aliased.error !== undefined)
      throw new Error(
        `fresh: ${to.username} could not name ${from.username} as an alias: ${JSON.stringify(aliased.error)}`,
      );
    const target = await this.#account(to);
    const moved = await (
      await this.#api(from)
    ).POST("/accounts/move", { body: { target_account_id: target.id, password: from.password } });
    if (moved.error !== undefined)
      throw new Error(`fresh: ${from.username} could not move to ${to.username}: ${JSON.stringify(moved.error)}`);
  }

  /**
   * `staff`, a person who keeps the administrator's rights (`fresh.person({ admin: true })`), takes the moderation
   * action `kind` on `target`'s account, which is told of it: for the reason shown to the account, and for
   * `expiresIn` seconds, or until a moderator lifts it when none is given.
   */
  async moderate(
    staff: Person,
    target: Person,
    o: { kind: components["schemas"]["AdminAccountActionKind"]; publicReason: string; expiresIn?: number },
  ): Promise<void> {
    const account = await this.#account(target);
    const body: components["schemas"]["CreateAccountActionRequest"] = {
      kind: o.kind,
      reason_category: "other",
      public_reason: o.publicReason,
      notify_target: true,
    };
    if (o.expiresIn !== undefined) body.expires_in = o.expiresIn;
    const { error } = await (
      await this.#api(staff)
    ).POST("/admin/accounts/{id}/actions", { params: { path: { id: account.id } }, body });
    if (error !== undefined)
      throw new Error(`fresh: ${staff.username} could not act on ${target.username}: ${JSON.stringify(error)}`);
  }

  /**
   * What the audit log holds of the operations of `staff`, a person who keeps the administrator's rights, the newest
   * first: each as its kind and the kind of its target (`account_action_issued account`).
   */
  async audited(staff: Person): Promise<string[]> {
    const actor = await this.#account(staff);
    const { data } = await (
      await this.#api(staff)
    ).GET("/admin/audit_log", { params: { query: { actor_account_id: actor.id, limit: 40 } } });
    return body(data, `the audit log of ${staff.username}`).items.map(
      (entry) => `${entry.action} ${entry.target_kind}`,
    );
  }

  /**
   * A name, an address and a password that nobody has signed up with; with `approval`, an address of the domain whose
   * sign-ups wait for a moderator (the scenario's `approvalDomain`).
   */
  newcomer(o: { approval?: boolean } = {}): Newcomer {
    const id = unique();
    const domain = o.approval === true ? approvalDomain : "fresh.test";
    return { username: `fresh_${id}`, email: `fresh_${id}@${domain}`, password: `fresh-password-${id}` };
  }

  /** A person of the login provider whom nobody has signed in as, to name on the fake provider's page. */
  providerIdentity(): ProviderIdentity {
    const id = unique();
    return {
      subject: `fresh-subject-${id}`,
      email: `fresh_${id}@fresh.test`,
      name: `Fresh ${id.slice(0, 8)}`,
      username: `fresh_${id}`,
    };
  }

  /** Grants `p` the role of `role.key`, which the administrator makes first where the server has none of that key. */
  async #grant(p: Person, role: { key: string; permissions: readonly components["schemas"]["Permission"][] }) {
    const api = await this.#staff("admin");
    try {
      await api.POST("/admin/roles", { body: { key: role.key, position: 1, permissions: [...role.permissions] } });
    } catch (e) {
      // Another test, or another run of this one, made it already: the key is taken, the role is the same.
      if (!(e instanceof ProblemError && e.status === 422)) throw e;
    }
    const account = await this.#account(p);
    await api.PUT("/admin/accounts/{id}/roles/{role}", { params: { path: { id: account.id, role: role.key } } });
  }

  /** The scenario's administrator, and its moderator. */
  async #staff(role: "admin" | "moderator"): Promise<Api> {
    const moderator = people.find((p) => p.staff === "moderator");
    if (moderator === undefined) throw new Error("fresh: the scenario has no moderator");
    return this.#api(role === "admin" ? personOf(admin) : moderator);
  }

  /** The account of `username` as the staff see it, with whether its email address is confirmed. */
  async #adminAccount(username: string) {
    const api = await this.#staff("admin");
    const found = await api.GET("/admin/accounts", { params: { query: { username, origin: "local" } } });
    const id = body(found.data, `the search for ${username}`).items.find((a) => a.account.username === username)?.id;
    if (id === undefined) throw new Error(`fresh: the server has no account ${username}`);
    const { data } = await api.GET("/admin/accounts/{id}", { params: { path: { id } } });
    return body(data, `the account ${username}`);
  }

  /** A new invite of the administrator, which one person signs up with. */
  async invite(): Promise<FreshInvite> {
    const { data } = await (await this.#staff("admin")).POST("/admin/invites", { body: { max_uses: 1 } });
    const invite = body(data, "the invite");
    return { code: invite.code, url: invite.url };
  }

  /** The moderator approves the registration of `who`, which waits for it. */
  async approve(who: Newcomer): Promise<void> {
    const api = await this.#staff("moderator");
    for await (const registration of all(async (cursor) =>
      body(
        (await api.GET("/admin/registrations", { params: { query: cursor === undefined ? {} : { cursor } } })).data,
        "the registrations that wait",
      ),
    )) {
      if (registration.username !== who.username) continue;
      const { error } = await api.POST("/admin/registrations/{id}/approve", {
        params: { path: { id: registration.id } },
        body: {},
      });
      if (error !== undefined) throw new Error(`fresh: the registration of ${who.username} was not approved`);
      return;
    }
    throw new Error(`fresh: no registration of ${who.username} waits for approval`);
  }

  /** Whether the server holds the email address of `who` as confirmed. */
  async emailConfirmed(who: Newcomer): Promise<boolean> {
    return (await this.#adminAccount(who.username)).email_verified === true;
  }

  /**
   * The supporter program on the server's fake payment provider, with the tips (¥500, ¥1,000, ¥3,000) and the
   * supporters list shown to everyone: two plans, Supporter (¥500 a month) and Patron (¥1,500), each 10% off yearly,
   * made once for the run on roles of their own, and returned once both take subscriptions (their prices are made by a
   * job of the server). Every later call finds them. The program stays on for the run, as nothing reads it off.
   */
  async supporterProgram(): Promise<{ supporter: string; patron: string }> {
    const api = await this.#staff("admin");
    const listed = body((await api.GET("/admin/supporter_plans")).data, "the plans").items;
    const plans = [
      { role: "e2e_supporter", name: "Supporter", amount: "500", rank: 1 },
      { role: "e2e_patron", name: "Patron", amount: "1500", rank: 2 },
    ] as const;
    for (const plan of plans) {
      if (listed.some((p) => p.name === plan.name)) continue;
      await api.POST("/admin/roles", { body: { key: plan.role, position: plan.rank, permissions: [] } });
      const { error } = await api.POST("/admin/supporter_plans", {
        body: {
          role: plan.role,
          name: plan.name,
          description: `The ${plan.name} plan of the tests.`,
          monthly_price: { amount: plan.amount, currency: "JPY" },
          annual_discount_percent: 10,
          emoji_limit: 0,
          rank: plan.rank,
          visibility: "public",
        },
      });
      if (error !== undefined) throw new Error(`fresh: the plan ${plan.name} was refused`);
    }
    if ((await api.PUT("/admin/supporter_program", { body: { enabled: true } })).error !== undefined)
      throw new Error("fresh: the supporter program was not enabled");
    const amounts = ["500", "1000", "3000"].map((amount) => ({ amount, currency: "JPY" }));
    if ((await api.PUT("/admin/supporter_tips", { body: { enabled: true, amounts } })).error !== undefined)
      throw new Error("fresh: the tips were not enabled");
    // The plans take subscriptions once the server made their prices at the provider.
    for (let attempt = 0; attempt < 100; attempt++) {
      const { data } = await api.GET("/instance/supporter_plans");
      const ids = Object.fromEntries((data?.items ?? []).map((p) => [p.name, p.id]));
      if (ids["Supporter"] !== undefined && ids["Patron"] !== undefined)
        return { supporter: ids["Supporter"], patron: ids["Patron"] };
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    throw new Error("fresh: the plans never took subscriptions");
  }

  /**
   * A new role of `staff`, a fresh person made with `admin: true`, that a supporter plan can be on: no holders, no
   * permission, below the moderators. It returns the role's key, unique to the test and the run.
   */
  async planRole(staff: FreshPerson): Promise<string> {
    const key = `plan_${unique()}`;
    const api = await this.#api(staff);
    const { error } = await api.POST("/admin/roles", { body: { key, position: 10, permissions: [] } });
    if (error !== undefined) throw new Error(`fresh: the role ${key} was refused`);
    return key;
  }

  /**
   * Deletes the local custom emoji of `shortcode` a test made, as the scenario's administrator, so that no test's emoji
   * stays in every reader's picker; none of that shortcode is no failure.
   */
  async deleteEmoji(shortcode: string): Promise<void> {
    const api = await this.#staff("admin");
    const { data } = await api.GET("/admin/custom_emojis", { params: { query: { local: true, shortcode } } });
    for (const emoji of data?.items.filter((e) => e.shortcode === shortcode) ?? [])
      await api.DELETE("/admin/custom_emojis/{id}", { params: { path: { id: emoji.id } } });
  }

  /**
   * Withdraws every announcement titled `title` that is not withdrawn, as the scenario's administrator, so that no
   * test's announcement stays above every reader's home timeline; none of that title is no failure.
   */
  async withdrawAnnouncement(title: string): Promise<void> {
    const api = await this.#staff("admin");
    for (const state of ["draft", "scheduled", "showing"] as const) {
      const { data } = await api.GET("/admin/announcements", { params: { query: { state } } });
      for (const a of data?.items.filter((x) => x.title === title) ?? [])
        await api.POST("/admin/announcements/{id}/withdraw", { params: { path: { id: a.id } } });
    }
  }

  /** A follow request of `from` to the locked `to`, which waits for `to` to decide it. */
  async followRequest(from: Person, to: Person): Promise<void> {
    if (to.locked !== true) throw new Error(`fresh: ${to.username} is not locked, so a follow is no request`);
    await this.#follow(from, to, false);
  }
}
