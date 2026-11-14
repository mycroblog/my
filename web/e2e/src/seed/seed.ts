// SPDX-License-Identifier: AGPL-3.0-only

// Applies the scenario (scenario.ts, ADR-0015 §5) to a running `my serve`: the people, the instance settings and the
// custom emoji with `bin/my`, everything else with the client API through @mycroblog/mykit, signed in as each person
// (sign-in.ts). It writes no SQL: the timelines and notifications come from the actions. Every creating request carries
// `Idempotency-Key: demo:<key>`, and what the server already has is found again (people by username, posts with images
// by their alt text, apps by name, emoji by shortcode, announcements and events by title) and left alone, so a second
// run against one server adds nothing. What the server decides and a test needs (the TOTP secrets, the passkeys' keys, the apps' client IDs) is the
// seed's state, which it reads before it runs and returns after.

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { all, uploadMedia, type Api, type components } from "@mycroblog/mykit";
import type { Drawing } from "../drawing";
import {
  admin,
  announcements,
  apps,
  emojis,
  eventStart,
  events,
  follows,
  instance,
  interactions,
  people,
  personOf,
  posts,
  restrictions,
  type Person,
} from "../scenario";
import { createPasskey, type PasskeyCredential } from "./passkey";
import { drawPNG } from "./png";
import { signIn } from "./sign-in";
import { totp, totpStep } from "./totp";

type Status = components["schemas"]["Status"];
type Account = components["schemas"]["Account"];

/** What the server decided for the scenario and the E2E tests need, by username or app key. */
export type SeedState = {
  totp: Record<string, string>;
  /** The TOTP step whose code confirmed each person's enrollment, which a later code avoids (totpCode). */
  totpSteps: Record<string, number>;
  passkeys: Record<string, PasskeyCredential>;
  apps: Record<string, { clientId: string }>;
};

const emptyState = (): SeedState => ({ totp: {}, totpSteps: {}, passkeys: {}, apps: {} });

/** Runs `bin/my` with `args` and `input` on its standard input against the server's database; returns its output. */
type RunMy = (args: readonly string[], input?: string) => string;

export type SeedOptions = {
  /** The origin of the server. */
  server: string;
  runMy: RunMy;
  /** The state of an earlier run against the same server. */
  state?: SeedState;
  log?: (line: string) => void;
};

/** The name of the passkey the seed registers. */
const passkeyName = "Demo passkey";

const key = (item: string) => ({ "Idempotency-Key": `demo:${item}` });

function body<T>(data: T | undefined, what: string): T {
  if (data === undefined) throw new Error(`seed: ${what} answered without a body`);
  return data;
}

/** The error output of a failed `bin/my`, to tell an expected refusal from a failure. */
const stderrOf = (err: unknown) => String((err as { stderr?: unknown }).stderr ?? err);

async function until<T>(what: string, check: () => Promise<T | undefined>, timeoutMs = 60_000): Promise<T> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value !== undefined) return value;
    if (Date.now() > deadline) throw new Error(`seed: timed out waiting for ${what}`);
    await new Promise((done) => setTimeout(done, 250));
  }
}

type Purpose = "attachment" | "avatar" | "header";

/**
 * Uploads a drawing as a PNG with `api` and waits until the server has processed it: the scenario's images, and those
 * of the E2E tests' own posts (fresh.ts).
 */
export async function uploadDrawing(api: Api, name: string, drawing: Drawing, description: string, purpose: Purpose) {
  const file = new Blob([new Uint8Array(drawPNG(drawing))], { type: "image/png" });
  const media = await uploadMedia(api, { file, fileName: name, purpose, description });
  return until(`the processing of ${name}`, async () => {
    const { data } = await api.GET("/media/{id}", { params: { path: { id: media.id } } });
    const current = body(data, `the media ${name}`);
    if (current.state === "failed") throw new Error(`seed: the server could not process ${name}`);
    return current.state === "ready" ? current : undefined;
  });
}

/** Enables two-factor authentication for the person of `api`: the secret, and the step whose code confirmed it. */
export async function enrollTOTP(api: Api): Promise<{ secret: string; step: number }> {
  const setup = body((await api.POST("/two_factor/totp/setup")).data, "the TOTP setup");
  const at = Date.now();
  const { error } = await api.POST("/two_factor/totp", {
    body: { setup_token: setup.setup_token, code: totp(setup.secret, at) },
  });
  if (error !== undefined) throw new Error("seed: the server refused the code of its own TOTP setup");
  return { secret: setup.secret, step: totpStep(at) };
}

class Seeder {
  readonly #o: SeedOptions;
  readonly state: SeedState;
  readonly #apis = new Map<string, Api>();
  readonly #accounts = new Map<string, Account>();
  readonly #posts = new Map<string, Status>();

  constructor(o: SeedOptions) {
    this.#o = o;
    this.state = { ...emptyState(), ...structuredClone(o.state ?? emptyState()) };
  }

  log(line: string) {
    this.#o.log?.(line);
  }

  async api(username: string): Promise<Api> {
    let api = this.#apis.get(username);
    if (api === undefined) {
      api = await signIn(this.#o.server, personOf(username), this.state.totp[username], this.state.totpSteps[username]);
      this.#apis.set(username, api);
    }
    return api;
  }

  async account(username: string): Promise<Account> {
    let account = this.#accounts.get(username);
    if (account === undefined) {
      const { data } = await (await this.api(admin)).GET("/accounts/lookup", { params: { query: { acct: username } } });
      account = body(data, `the lookup of ${username}`);
      this.#accounts.set(username, account);
    }
    return account;
  }

  post(key: string): Status {
    const status = this.#posts.get(key);
    if (status === undefined) throw new Error(`seed: the post ${key} is used before it is written`);
    return status;
  }

  /** Creates the people with `my admin create` and gives them their staff roles with `my admin role`. */
  createPeople() {
    for (const p of people) {
      try {
        this.#o.runMy(["admin", "create", "-username", p.username, "-email", p.email], `${p.password}\n`);
        this.log(`created @${p.username}`);
      } catch (err) {
        if (!stderrOf(err).includes("already")) throw err;
      }
      // `my admin create` makes an administrator; only the scenario's administrator keeps the role.
      if (p.staff === "moderator") this.#o.runMy(["admin", "role", "grant", p.username, "moderator"]);
      if (p.staff !== "admin") this.#o.runMy(["admin", "role", "revoke", p.username, "admin"]);
    }
  }

  /** The instance settings, each read back first, so that an unchanged one adds no revision. */
  configureInstance() {
    for (const [k, value] of Object.entries(instance)) {
      let current = "";
      try {
        current = this.#o.runMy(["instance", "get", k]).replace(/\n$/, "");
      } catch (err) {
        if (!stderrOf(err).includes("not set")) throw err;
      }
      if (current !== value) this.#o.runMy(["instance", "set", k, "-"], value);
    }
  }

  /** The custom emoji, each from a PNG file drawn into a temporary directory. */
  async addEmojis() {
    const { data } = await (await this.api(admin)).GET("/custom_emojis");
    const have = new Set(body(data, "the custom emoji").items.map((e) => e.shortcode));
    const dir = mkdtempSync(join(tmpdir(), "my-seed-"));
    try {
      for (const e of emojis) {
        if (have.has(e.shortcode)) continue;
        const file = join(dir, `${e.shortcode}.png`);
        writeFileSync(file, drawPNG(e.drawing));
        this.#o.runMy(["admin", "emoji", "create", "-shortcode", e.shortcode, file]);
      }
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  }

  /** Uploads a drawing as a PNG and waits until the server has processed it. */
  upload(api: Api, name: string, drawing: Drawing, description: string, purpose: Purpose) {
    return uploadDrawing(api, name, drawing, description, purpose);
  }

  async setUpProfiles() {
    for (const p of people) {
      const api = await this.api(p.username);
      const me = body((await api.GET("/accounts/verify_credentials")).data, "verify_credentials");
      const avatar =
        me.avatar === undefined
          ? (await this.upload(api, `${p.username}-avatar.png`, p.avatar, "", "avatar")).id
          : undefined;
      const header =
        me.header === undefined && p.header !== undefined
          ? (await this.upload(api, `${p.username}-header.png`, p.header, "", "header")).id
          : undefined;
      const fields = p.fields.map(([name, value]) => ({ name, value }));
      const same =
        me.display_name === p.displayName &&
        me.source.note === p.note &&
        me.locked === (p.locked ?? false) &&
        JSON.stringify(me.source.fields.map(({ name, value }) => ({ name, value }))) === JSON.stringify(fields);
      // Each change of a profile is a revision, sent to the followers: an unchanged one is left alone.
      if (!same || avatar !== undefined || header !== undefined)
        await api.PATCH("/accounts/update_credentials", {
          body: {
            display_name: p.displayName,
            note: p.note,
            locked: p.locked ?? false,
            fields,
            ...(avatar === undefined ? {} : { avatar_media_id: avatar }),
            ...(header === undefined ? {} : { header_media_id: header }),
          },
        });
      const prefs = body((await api.GET("/preferences")).data, "the preferences");
      if (prefs.locale !== p.locale || prefs.default_post_language !== p.locale)
        await api.PATCH("/preferences", { body: { locale: p.locale, default_post_language: p.locale } });
      this.#accounts.set(p.username, body((await api.GET("/accounts/verify_credentials")).data, "verify_credentials"));
    }
  }

  async follow() {
    for (const f of follows) {
      const api = await this.api(f.follower);
      const followee = await this.account(f.followee);
      const { data } = await api.GET("/accounts/relationships", { params: { query: { id: [followee.id] } } });
      const r = body(data, "the relationships").items[0];
      if (r?.following !== true && r?.requested !== true)
        await api.POST("/accounts/{id}/follow", { params: { path: { id: followee.id } } });
      if (f.accepted === true && r?.following !== true) {
        const follower = await this.account(f.follower);
        await (
          await this.api(f.followee)
        ).POST("/follow_requests/{id}/accept", {
          params: { path: { id: follower.id } },
        });
      }
    }
  }

  /** The author's post that carries an image with `description`, from an earlier run. */
  async findByImage(username: string, description: string): Promise<Status | undefined> {
    const api = await this.api(username);
    const account = await this.account(username);
    for await (const status of all(async (cursor) =>
      body(
        (
          await api.GET("/accounts/{id}/statuses", {
            params: { path: { id: account.id }, query: cursor === undefined ? {} : { cursor } },
          })
        ).data,
        "the statuses",
      ),
    ))
      if (status.media_attachments.some((m) => m.description === description)) return status;
    return undefined;
  }

  async writePosts() {
    for (const post of posts) {
      const api = await this.api(post.author);
      const first = post.images?.[0];
      const found = first === undefined ? undefined : await this.findByImage(post.author, first.description);
      let status = found;
      if (status === undefined) {
        const media = [];
        for (const image of post.images ?? [])
          media.push({
            id: (await this.upload(api, image.name, image.drawing, image.description, "attachment")).id,
            // A post keeps its own copy of each description.
            description: image.description,
          });
        const { data } = await api.POST("/statuses", {
          params: { header: key(post.key) },
          body: {
            text: post.text,
            visibility: post.visibility,
            language: post.language,
            ...(post.spoiler === undefined ? {} : { spoiler_text: post.spoiler }),
            ...(post.replyTo === undefined ? {} : { in_reply_to_id: this.post(post.replyTo).id }),
            ...(post.quote === undefined ? {} : { quoted_status_id: this.post(post.quote).id }),
            ...(media.length === 0 ? {} : { media }),
          },
        });
        status = body(data, `the post ${post.key}`);
      }
      this.#posts.set(post.key, status);
      if (post.pinned === true && status.pinned !== true)
        await api.POST("/statuses/{id}/pin", { params: { path: { id: status.id } } });
    }
  }

  async interact() {
    for (const i of interactions) {
      const api = await this.api(i.by);
      const path = { id: this.post(i.post).id };
      switch (i.kind) {
        case "favourite":
          await api.POST("/statuses/{id}/favourite", { params: { path } });
          break;
        case "boost":
          await api.POST("/statuses/{id}/boost", { params: { path } });
          break;
        case "bookmark":
          await api.POST("/statuses/{id}/bookmark", { params: { path } });
          break;
        case "reaction":
          await api.PUT("/statuses/{id}/reaction", { params: { path }, body: { emoji: i.emoji } });
          break;
      }
    }
  }

  async restrict() {
    for (const r of restrictions) {
      const api = await this.api(r.by);
      const path = { id: (await this.account(r.target)).id };
      if (r.kind === "block") await api.POST("/accounts/{id}/block", { params: { path } });
      else await api.POST("/accounts/{id}/mute", { params: { path } });
    }
  }

  async registerApps() {
    for (const app of apps) {
      const api = await this.api(app.owner);
      const existing = body((await api.GET("/oauth_apps")).data, "the OAuth apps").items.find(
        (a) => a.name === app.name,
      );
      const made =
        existing ??
        body(
          (
            await api.POST("/oauth_apps", {
              headers: key(app.key),
              body: {
                name: app.name,
                website: app.website,
                redirect_uris: [...app.redirectUris],
                scope: app.scope,
                client_type: "public",
              },
            })
          ).data,
          `the app ${app.key}`,
        ).app;
      this.state.apps[app.key] = { clientId: made.client_id };
    }
  }

  /**
   * The announcements, published by the administrator in order, each found again by its title; then marked read by
   * whom the scenario says.
   */
  async publishAnnouncements() {
    const api = await this.api(admin);
    const have = new Set<string>();
    for await (const a of all(async (cursor) =>
      body(
        (await api.GET("/admin/announcements", { params: { query: cursor === undefined ? {} : { cursor } } })).data,
        "the announcements",
      ),
    ))
      have.add(a.title);
    for (const a of announcements)
      if (!have.has(a.title))
        await api.POST("/admin/announcements", {
          headers: key(`announcement-${a.key}`),
          body: { title: a.title, text: a.text, publish: true },
        });
    for (const a of announcements)
      for (const reader of a.readBy ?? []) {
        const readerApi = await this.api(reader);
        const shown = body((await readerApi.GET("/announcements")).data, "the announcements").items;
        const found = shown.find((x) => x.title === a.title);
        if (found !== undefined && !found.read)
          await readerApi.POST("/announcements/{id}/read", { params: { path: { id: found.id } } });
      }
  }

  /** The upcoming events, each created by its organizer unless they organize one of its title already. */
  async createEvents() {
    for (const e of events) {
      const api = await this.api(e.organizer);
      const organizer = await this.account(e.organizer);
      const { data } = await api.GET("/events", { params: { query: { organizer_id: organizer.id } } });
      if (body(data, "the events").items.some((x) => x.title === e.title)) continue;
      const startsAt = eventStart(e, Date.now());
      await api.POST("/events", {
        headers: key(`event-${e.key}`),
        body: {
          title: e.title,
          description: e.description,
          time_zone: e.timeZone,
          starts_at: startsAt.toISOString(),
          ends_at: new Date(startsAt.getTime() + e.hours * 3_600_000).toISOString(),
        },
      });
    }
  }

  async registerPasskeys() {
    for (const p of people.filter((x) => x.passkey === true)) {
      const api = await this.api(p.username);
      const listed = body((await api.GET("/passkeys")).data, "the passkeys").items.some((k) => k.name === passkeyName);
      if (listed && this.state.passkeys[p.username] !== undefined) continue;
      if (listed) throw new Error(`seed: ${p.username} has a passkey whose key the seed's state lacks`);
      const begun = body(
        (await api.POST("/passkeys/registration", { body: { name: passkeyName, password: p.password } })).data,
        "the passkey registration",
      );
      const { credential, response } = createPasskey(begun.options, this.#o.server);
      await api.POST("/passkeys", {
        body: { registration_token: begun.registration_token, credential: response },
      });
      this.state.passkeys[p.username] = credential;
    }
  }

  /** Two-factor authentication last: each later sign-in of the person needs a code. */
  async enableTwoFactor() {
    for (const p of people.filter((x) => x.twoFactor === true)) {
      const api = await this.api(p.username);
      const enabled = body((await api.GET("/two_factor")).data, "the two-factor status").enabled;
      if (enabled && this.state.totp[p.username] !== undefined) continue;
      if (enabled) throw new Error(`seed: ${p.username} has two-factor authentication whose secret the seed lacks`);
      const { secret, step } = await enrollTOTP(api);
      this.state.totp[p.username] = secret;
      this.state.totpSteps[p.username] = step;
    }
  }
}

/** Applies the scenario to the server, in the order of scenario.ts, and returns the seed's state. */
export async function seed(o: SeedOptions): Promise<SeedState> {
  const s = new Seeder(o);
  s.createPeople();
  s.configureInstance();
  await s.addEmojis();
  await s.setUpProfiles();
  await s.follow();
  await s.writePosts();
  await s.interact();
  await s.restrict();
  await s.registerApps();
  await s.publishAnnouncements();
  await s.createEvents();
  await s.registerPasskeys();
  await s.enableTwoFactor();
  return s.state;
}

/** The person of the scenario who signs in as `username`, for the callers' messages. */
export const loginOf = (p: Person) => `${p.username.padEnd(12)} ${p.email.padEnd(22)} ${p.password}`;
