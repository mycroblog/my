// SPDX-License-Identifier: AGPL-3.0-only

// What an E2E test makes for itself on the demo server, so that it changes nothing another test or a second run of it
// reads (`--repeat-each`): a person, a post, a follow request. Each is made as the seed makes the scenario (seed.ts):
// the person with `bin/my admin create`, the rest with the client API through @mycroblog/mykit, signed in as each
// person, and no SQL. Each name holds a part of a random UUID, so that it is unique to the test and the run without
// reading the clock.

import { randomUUID } from "node:crypto";
import type { Api, components } from "@mycroblog/mykit";
import type { Drawing } from "./drawing";
import type { Image, Person, Visibility } from "./scenario";
import { enrollTOTP, uploadDrawing } from "./seed/seed";
import { signIn } from "./seed/sign-in";

type Status = components["schemas"]["Status"];
type Account = components["schemas"]["Account"];

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
};

export type FreshPostOptions = {
  /** The text; the post's own text ends with a word unique to it, so that a spec finds it alone. */
  text: string;
  visibility?: Visibility;
  image?: Image;
  /** The content warning. */
  spoiler?: string;
};

/** A post a test made: its ID, and its whole text, by which a spec finds it on the screen. */
export type FreshPost = { id: string; text: string };

/** A person's avatar: a plain square, which no screen of a test reads. */
const avatar: Drawing = { width: 64, height: 64, top: [120, 144, 168], bottom: [72, 96, 120], shapes: [] };

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
      api = await signIn(this.#o.server, p, secret, own?.step ?? this.#o.totp?.steps[p.username]);
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
    // `my admin create` makes an administrator; a person of a test is none.
    this.#o.runMy(["admin", "role", "revoke", p.username, "admin"]);
    const api = await this.#api(p);
    const { error } = await api.PATCH("/accounts/update_credentials", {
      body: { display_name: p.displayName, locked: o.locked === true },
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
    const media =
      o.image === undefined
        ? []
        : [
            {
              id: (await uploadDrawing(api, o.image.name, o.image.drawing, o.image.description, "attachment")).id,
              description: o.image.description,
            },
          ];
    const text = `${o.text} ${id.slice(0, 8)}`;
    const { data } = await api.POST("/statuses", {
      params: { header: { "Idempotency-Key": `fresh:${id}` } },
      body: {
        text,
        visibility: o.visibility ?? "public",
        language: author.locale,
        ...(o.spoiler === undefined ? {} : { spoiler_text: o.spoiler }),
        ...(media.length === 0 ? {} : { media }),
      },
    });
    const status: Status = body(data, "the post");
    return { id: status.id, text };
  }

  /** A follow request of `from` to the locked `to`, which waits for `to` to decide it. */
  async followRequest(from: Person, to: Person): Promise<void> {
    if (to.locked !== true) throw new Error(`fresh: ${to.username} is not locked, so a follow is no request`);
    await this.#follow(from, to, false);
  }
}
