// SPDX-License-Identifier: AGPL-3.0-only

// What an E2E test makes for itself on the demo server, so that it changes nothing another test or a second run of it
// reads (`--repeat-each`): a person, a post, a follow request. Each is made as the seed makes the scenario (seed.ts):
// the person with `bin/my admin create`, the rest with the client API through @mycroblog/mykit, signed in as each
// person, and no SQL. Each name holds a part of a random UUID, so that it is unique to the test and the run without
// reading the clock.

import { randomBytes, randomUUID } from "node:crypto";
import type { Api, components } from "@mycroblog/mykit";
import type { Drawing } from "./drawing";
import { silentMP3, tinyWebM } from "./av";
import type { Image, Person, Poll, Visibility } from "./scenario";
import { enrollTOTP, uploadDrawing, uploadFile } from "./seed/seed";
import { signIn } from "./seed/sign-in";

type Status = components["schemas"]["Status"];
type Account = components["schemas"]["Account"];
type MediaAttachment = components["schemas"]["MediaAttachment"];
type ThemeTokenValue = components["schemas"]["ThemeTokenValue"];

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

  /** `by` reacts to the post `post` with `emoji`: a Unicode emoji, or a custom one's shortcode without colons. */
  async reaction(by: Person, post: FreshPost, emoji: string): Promise<void> {
    const { error } = await (
      await this.#api(by)
    ).PUT("/statuses/{id}/reaction", { params: { path: { id: post.id } }, body: { emoji } });
    if (error !== undefined) throw new Error(`fresh: ${by.username} could not react to ${post.id}`);
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
   * The stylesheet of a draft of the instance theme with `tokens` overridden, which `admin` saves and the server would
   * serve once published: an administrator applies it over a page to preview the draft. Nothing is published, so no
   * other test sees the theme.
   */
  async themeDraftStylesheet(admin: FreshPerson, tokens: readonly ThemeTokenValue[]): Promise<string> {
    if (admin.staff !== "admin") throw new Error(`fresh: ${admin.username} is no administrator`);
    const api = await this.#api(admin);
    const saved = await api.PUT("/admin/theme/draft", { body: { tokens: [...tokens] } });
    if (saved.error !== undefined)
      throw new Error(`fresh: the theme draft was refused: ${JSON.stringify(saved.error)}`);
    const { data } = await api.GET("/admin/theme/draft/stylesheet", { parseAs: "text" });
    return body(data, "the stylesheet of the theme draft");
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

  /** A follow request of `from` to the locked `to`, which waits for `to` to decide it. */
  async followRequest(from: Person, to: Person): Promise<void> {
    if (to.locked !== true) throw new Error(`fresh: ${to.username} is not locked, so a follow is no request`);
    await this.#follow(from, to, false);
  }
}
