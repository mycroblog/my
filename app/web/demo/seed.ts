// SPDX-License-Identifier: AGPL-3.0-only
// Applies the demo scenario (demo/scenario.ts, ADR-0117 §1) to a running `my serve` of the test build: the people
// and the server's settings with `bin/my`, everything else with the client and admin APIs through
// @mycroblog/mykit, signed in as each person through the web client's sign-in (the authorization code flow with
// PKCE). It writes no SQL: timelines, notifications, and the other projections come from the actions. Every creating
// request carries `Idempotency-Key: demo:<item>`, and an item the server already has is found again (people by
// username, posts with images by the alt text, the rest by its text or name) and left alone, so running the seed
// twice against one server adds nothing. It reaches the server and nothing else.
//
//   bun --conditions=@mycroblog/source demo/seed.ts --server http://localhost:8787 [--census]
//
// `bin/my` (DEMO_MY_BIN overrides it) runs in this process's environment, which must name the server's database
// (e2e/server.ts passes it). --census prints what the server holds instead (census below), for the test that a
// second run adds nothing.
import { execFileSync } from "node:child_process";
import { join, resolve } from "node:path";
import { parseArgs } from "node:util";
import {
  all,
  codeChallenge,
  createClient,
  exchange,
  newCodeVerifier,
  staticToken,
  uploadMedia,
  type Api,
  type components,
} from "@mycroblog/mykit";
import { drawPNG, type Drawing } from "./assets/draw.ts";
import {
  admin,
  announcements,
  costItems,
  follows,
  instanceSettings,
  instanceTexts,
  interactions,
  invites,
  lists,
  moderationActions,
  personas,
  posts,
  reports,
  roles,
  rules,
  staffBadges,
  supporters,
  type Persona,
} from "./scenario.ts";

type Status = components["schemas"]["Status"];
type Account = components["schemas"]["Account"];
type CredentialAccount = components["schemas"]["CredentialAccount"];

/** Runs `bin/my` with `args` and `input` on its standard input against the server's database; returns its output. */
export type RunMy = (args: readonly string[], input?: string) => string;

export type SeedOptions = {
  /** The origin of the server, such as http://localhost:8787. */
  server: string;
  runMy: RunMy;
  /** Prints the progress; nothing by default. */
  log?: (line: string) => void;
};

/** The header value of the scenario item `key`. */
const idempotencyKey = (key: string) => `demo:${key}`;

/** The data of an answer, which mykit sets whenever the call resolves. */
function body<T>(data: T | undefined): T {
  if (data === undefined) throw new Error("demo: an answer without a body");
  return data;
}

/** The query of a page of a collection: the cursor of the page, none for the first one. */
const at = (cursor: string | undefined) => (cursor === undefined ? {} : { cursor });

const sleep = (ms: number) => new Promise((done) => setTimeout(done, ms));

/** Polls `check` until it returns a value, for at most `timeoutMs`. */
async function until<T>(what: string, check: () => Promise<T | undefined>, timeoutMs = 60_000): Promise<T> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value !== undefined) return value;
    if (Date.now() > deadline) throw new Error(`demo: timed out waiting for ${what}`);
    await sleep(250);
  }
}

/** A minimal cookie jar and fetch without redirects: what a browser does on the server's sign-in pages. */
class Browser {
  readonly #cookies = new Map<string, string>();

  async request(url: string, form?: Record<string, string>): Promise<Response> {
    const headers = new Headers();
    if (this.#cookies.size > 0) {
      headers.set("Cookie", [...this.#cookies].map(([name, value]) => `${name}=${value}`).join("; "));
    }
    let payload: string | undefined;
    if (form !== undefined) {
      headers.set("Content-Type", "application/x-www-form-urlencoded");
      headers.set("Sec-Fetch-Site", "same-origin");
      payload = new URLSearchParams(form).toString();
    }
    const res = await fetch(url, {
      method: form === undefined ? "GET" : "POST",
      headers,
      redirect: "manual",
      ...(payload === undefined ? {} : { body: payload }),
    });
    for (const cookie of res.headers.getSetCookie()) {
      const [pair = ""] = cookie.split(";");
      const at = pair.indexOf("=");
      if (at > 0) this.#cookies.set(pair.slice(0, at).trim(), pair.slice(at + 1).trim());
    }
    return res;
  }
}

/** The Location of a redirect, resolved against the request's URL. */
function location(res: Response, base: string): string {
  const to = res.headers.get("Location");
  if (to === null || res.status < 300 || res.status >= 400) {
    throw new Error(`demo: expected a redirect from ${base}, got ${String(res.status)}`);
  }
  return new URL(to, base).toString();
}

/** Signs in as `p` through the web client's authorization code flow with PKCE and returns a client of the API. */
async function signIn(server: string, p: Persona): Promise<Api> {
  const browser = new Browser();
  const redirectUri = `${server}/login/callback`;
  const verifier = newCodeVerifier();
  const authorize = `${server}/oauth/authorize?${new URLSearchParams({
    response_type: "code",
    client_id: "my-web",
    redirect_uri: redirectUri,
    state: "demo",
    code_challenge: await codeChallenge(verifier),
    code_challenge_method: "S256",
  }).toString()}`;
  const signInPage = location(await browser.request(authorize), authorize);
  const page = await (await browser.request(signInPage)).text();
  const csrf = /name="csrf_token" value="([^"]+)"/.exec(page)?.[1];
  if (csrf === undefined) throw new Error("demo: the sign-in page has no csrf_token");
  const signedIn = await browser.request(`${server}/auth/sign_in`, {
    csrf_token: csrf,
    email: p.email,
    password: p.password,
    return_to: new URL(signInPage).searchParams.get("return_to") ?? "",
  });
  if (signedIn.status !== 303) throw new Error(`demo: signing in as ${p.username} answered ${String(signedIn.status)}`);
  const back = new URL(location(await browser.request(authorize), authorize));
  const code = back.searchParams.get("code");
  if (code === null) throw new Error(`demo: the authorization of ${p.username} returned no code`);
  const token = await exchange({ server, clientId: "my-web", redirectUri }, code, verifier);
  return createClient({ server, tokenSource: staticToken(token.accessToken), acceptLanguage: p.locale });
}

function personaOf(username: string): Persona {
  const p = personas.find((x) => x.username === username);
  if (p === undefined) throw new Error(`demo: no persona ${username}`);
  return p;
}

/** The scenario's state while the seed runs: the people's clients and accounts, and the posts by key. */
class Seeder {
  readonly #o: SeedOptions;
  readonly #apis = new Map<string, Api>();
  readonly #accounts = new Map<string, Account>();
  readonly #posts = new Map<string, Status>();

  constructor(o: SeedOptions) {
    this.#o = o;
  }

  log(line: string): void {
    this.#o.log?.(line);
  }

  async api(username: string): Promise<Api> {
    let api = this.#apis.get(username);
    if (api === undefined) {
      api = await signIn(this.#o.server, personaOf(username));
      this.#apis.set(username, api);
    }
    return api;
  }

  async account(username: string): Promise<Account> {
    let account = this.#accounts.get(username);
    if (account === undefined) {
      const api = await this.api(admin);
      account = body((await api.GET("/accounts/lookup", { params: { query: { acct: username } } })).data);
      this.#accounts.set(username, account);
    }
    return account;
  }

  post(key: string): Status {
    const status = this.#posts.get(key);
    if (status === undefined) throw new Error(`demo: no post ${key} before it is used`);
    return status;
  }

  /** `bin/my` with a setting that it reads back first, so that an unchanged one adds no revision. */
  setInstance(key: string, value: string): void {
    let current = "";
    try {
      current = this.#o.runMy(["instance", "get", key]).replace(/\n$/, "");
    } catch (err) {
      // A text that was never set.
      if (!String((err as { stderr?: unknown }).stderr).includes("is not set")) throw err;
    }
    if (current !== value) this.#o.runMy(["instance", "set", key, "-"], value);
  }

  /** Creates the people with `my admin create` and gives them their staff roles with `my admin role`. */
  createPeople(): void {
    for (const p of personas) {
      try {
        this.#o.runMy(["admin", "create", "-username", p.username, "-email", p.email], `${p.password}\n`);
        this.log(`created @${p.username}`);
      } catch (err) {
        if (!String((err as { stderr?: unknown }).stderr).includes("already")) throw err;
      }
      // `my admin create` is the command that makes a local user; it grants the admin role, which only the
      // administrator keeps (revoking a role that the user does not hold changes nothing).
      if (p.staff === "moderator") this.#o.runMy(["admin", "role", "grant", p.username, "moderator"]);
      if (p.staff !== "admin") this.#o.runMy(["admin", "role", "revoke", p.username, "admin"]);
    }
  }

  configureInstance(): void {
    for (const [key, value] of Object.entries(instanceTexts)) this.setInstance(key, value);
    for (const [key, value] of Object.entries(instanceSettings)) this.setInstance(key, value);
  }

  async addRules(): Promise<void> {
    const api = await this.api(admin);
    const existing = body((await api.GET("/admin/rules")).data).items.map((r) => r.text);
    for (const rule of rules) {
      if (existing.includes(rule.text)) continue;
      this.#o.runMy(["instance", "rule", "add", ...(rule.hint === undefined ? [] : ["-hint", rule.hint]), rule.text]);
    }
  }

  async setUpProfiles(): Promise<void> {
    for (const p of personas) {
      const api = await this.api(p.username);
      const me = body((await api.GET("/accounts/verify_credentials")).data);
      if (p.newcomer !== true) await this.setUpProfile(api, p, me);
      const prefs = body((await api.GET("/preferences")).data);
      if (prefs.locale !== p.locale || prefs.default_post_language !== p.locale) {
        await api.PATCH("/preferences", { body: { locale: p.locale, default_post_language: p.locale } });
      }
      this.#accounts.set(p.username, body((await api.GET("/accounts/verify_credentials")).data));
    }
  }

  /** Sets the display name, profile text, lock, and avatar of `p`, whose account is `me` now. */
  async setUpProfile(api: Api, p: Persona, me: CredentialAccount): Promise<void> {
    let avatar: string | undefined;
    if (me.avatar === undefined) {
      avatar = (await this.upload(api, `${p.username}-avatar.png`, avatarOf(p), "", "avatar")).id;
    }
    const changed = me.display_name !== p.displayName || me.source.note !== p.note || me.locked !== (p.locked ?? false);
    if (changed || avatar !== undefined) {
      await api.PATCH("/accounts/update_credentials", {
        body: {
          display_name: p.displayName,
          note: p.note,
          locked: p.locked ?? false,
          ...(avatar === undefined ? {} : { avatar_media_id: avatar }),
        },
      });
    }
  }

  /** Uploads an image drawn by code and waits until the server has processed it. */
  async upload(
    api: Api,
    name: string,
    drawing: Drawing,
    description: string,
    purpose: "attachment" | "avatar" = "attachment",
  ): Promise<components["schemas"]["MediaAttachment"]> {
    const file = new Blob([new Uint8Array(drawPNG(drawing))], { type: "image/png" });
    const media = await uploadMedia(api, {
      file,
      fileName: name,
      purpose,
      ...(description === "" ? {} : { description }),
    });
    return until(`the processing of ${name}`, async () => {
      const current = body((await api.GET("/media/{id}", { params: { path: { id: media.id } } })).data);
      if (current.state === "failed") throw new Error(`demo: the server could not process ${name}`);
      return current.state === "ready" ? current : undefined;
    });
  }

  /** The badges of the staff roles and the roles that administrators made, granted and shown by their holders. */
  async setUpRoles(): Promise<void> {
    const api = await this.api(admin);
    const existing = body((await api.GET("/admin/roles")).data).items;
    for (const { role, badge } of staffBadges) {
      const current = existing.find((r) => r.key === role)?.badge;
      if (current?.label !== badge.label || current.color !== badge.color) {
        await api.PATCH("/admin/roles/{role}", { params: { path: { role } }, body: { badge } });
      }
    }
    for (const role of roles) {
      if (!existing.some((r) => r.key === role.key)) {
        await api.POST("/admin/roles", {
          body: { key: role.key, position: role.position, permissions: role.permissions },
          headers: { "Idempotency-Key": idempotencyKey(`role-${role.key}`) },
        });
        await api.PATCH("/admin/roles/{role}", { params: { path: { role: role.key } }, body: { badge: role.badge } });
      }
      for (const holder of role.holders) {
        const account = await this.account(holder);
        await api.PUT("/admin/accounts/{id}/roles/{role}", { params: { path: { id: account.id, role: role.key } } });
      }
    }
    for (const p of personas) await this.showBadges(p.username);
  }

  /** Shows every badge of the person's roles on their profile. */
  async showBadges(username: string): Promise<void> {
    const api = await this.api(username);
    for (const choice of body((await api.GET("/preferences/badges")).data).items) {
      if (!choice.displayed) {
        await api.PUT("/preferences/badges/{role_id}", {
          params: { path: { role_id: choice.role_id } },
          body: { displayed: true },
        });
      }
    }
  }

  async follow(): Promise<void> {
    for (const f of follows) {
      const api = await this.api(f.follower);
      const followee = await this.account(f.followee);
      await api.POST("/accounts/{id}/follow", { params: { path: { id: followee.id } } });
      if (f.accepted === true) {
        const follower = await this.account(f.follower);
        await (
          await this.api(f.followee)
        ).POST("/follow_requests/{id}/authorize", { params: { path: { id: follower.id } } });
      }
    }
  }

  /** The people's lists and their members; a list is found again by its owner's lists and its name. */
  async makeLists(): Promise<void> {
    for (const list of lists) {
      const api = await this.api(list.owner);
      const existing = body((await api.GET("/lists")).data).items.find((l) => l.name === list.name);
      const made =
        existing ??
        body(
          (
            await api.POST("/lists", {
              body: { name: list.name },
              headers: { "Idempotency-Key": idempotencyKey(list.key) },
            })
          ).data,
        );
      for (const username of list.members) {
        const member = await this.account(username);
        // Adding a member again changes nothing.
        await api.POST("/lists/{id}/accounts/{account_id}", {
          params: { path: { id: made.id, account_id: member.id } },
        });
      }
    }
  }

  /** The person's posts that carry an image with `description`, found again on a second run. */
  async findByImage(username: string, description: string): Promise<Status | undefined> {
    const api = await this.api(username);
    const account = await this.account(username);
    for await (const status of all(async (cursor) =>
      body(
        (await api.GET("/accounts/{id}/statuses", { params: { path: { id: account.id }, query: at(cursor) } })).data,
      ),
    )) {
      if (status.media_attachments.some((m) => m.description === description)) return status;
    }
    return undefined;
  }

  async writePosts(): Promise<void> {
    for (const post of posts) {
      const api = await this.api(post.author);
      let media: { id: string }[] | undefined;
      if (post.image !== undefined) {
        const found = await this.findByImage(post.author, post.image.description);
        if (found !== undefined) {
          this.#posts.set(post.key, found);
          continue;
        }
        const { name, drawing, description } = post.image;
        media = [{ id: (await this.upload(api, name, drawing, description)).id }];
      }
      const status = body(
        (
          await api.POST("/statuses", {
            params: { header: { "Idempotency-Key": idempotencyKey(post.key) } },
            body: {
              text: post.text,
              visibility: post.visibility,
              language: post.language,
              ...(post.spoiler === undefined ? {} : { spoiler_text: post.spoiler }),
              ...(post.replyTo === undefined ? {} : { in_reply_to_id: this.post(post.replyTo).id }),
              ...(post.quote === undefined ? {} : { quoted_status_id: this.post(post.quote).id }),
              ...(media === undefined ? {} : { media }),
              ...(post.poll === undefined
                ? {}
                : { poll: { options: post.poll.options, expires_in: post.poll.expiresIn } }),
            },
          })
        ).data,
      );
      this.#posts.set(post.key, status);
    }
  }

  async interact(): Promise<void> {
    for (const i of interactions) {
      const api = await this.api(i.by);
      const id = this.post(i.post).id;
      switch (i.kind) {
        case "favourite":
          await api.POST("/statuses/{id}/favourite", { params: { path: { id } } });
          break;
        case "boost":
          await api.POST("/statuses/{id}/boost", { params: { path: { id } } });
          break;
        case "reaction":
          await api.PUT("/statuses/{id}/reaction", { params: { path: { id } }, body: { emoji: i.emoji } });
          break;
      }
    }
  }

  async report(): Promise<void> {
    const ruleIDs = body((await (await this.api(admin)).GET("/admin/rules")).data).items;
    for (const r of reports) {
      const api = await this.api(r.reporter);
      const target = await this.account(r.target);
      const filed = body((await api.GET("/reports")).data).items;
      let report = filed.find((x) => x.target_account.id === target.id && x.comment === r.comment);
      if (report === undefined) {
        report = body(
          (
            await api.POST("/reports", {
              body: {
                account_id: target.id,
                category: r.category,
                comment: r.comment,
                forward: false,
                status_ids: r.posts.map((key) => this.post(key).id),
                ...(r.rules === undefined
                  ? {}
                  : { rule_ids: r.rules.map((n) => ruleIDs.find((x) => x.text === rules[n]?.text)?.id ?? "") }),
              },
              headers: { "Idempotency-Key": idempotencyKey(r.key) },
            })
          ).data,
        );
      }
      if (r.resolve !== undefined) {
        await (
          await this.api(r.resolve.by)
        ).POST("/admin/reports/{id}/resolve", {
          params: { path: { id: report.id } },
          body: { outcome: r.resolve.outcome },
        });
      }
    }
  }

  async moderate(): Promise<void> {
    for (const a of moderationActions) {
      const api = await this.api(a.moderator);
      const target = await this.account(a.target);
      const taken = body(
        (await api.GET("/admin/accounts/{id}/actions", { params: { path: { id: target.id } } })).data,
      ).items;
      if (!taken.some((x) => x.public_reason === a.publicReason)) {
        await api.POST("/admin/accounts/{id}/actions", {
          params: { path: { id: target.id } },
          body: {
            kind: a.kind,
            reason_category: a.reasonCategory,
            public_reason: a.publicReason,
            notify_target: true,
          },
          headers: { "Idempotency-Key": idempotencyKey(a.key) },
        });
      }
      if (a.appeal === undefined) continue;
      const own = await this.api(a.target);
      const action = body((await own.GET("/moderation/actions")).data).items.find(
        (x) => x.public_reason === a.publicReason,
      );
      if (action === undefined) throw new Error(`demo: ${a.target} does not see the action ${a.key}`);
      // One appeal per action: an earlier run may have made it.
      if (action.appeal !== undefined) continue;
      await own.POST("/moderation/actions/{id}/appeal", {
        params: { path: { id: action.id } },
        body: { text: a.appeal },
        headers: { "Idempotency-Key": idempotencyKey(`${a.key}-appeal`) },
      });
    }
  }

  async setUpSupporters(): Promise<void> {
    const api = await this.api(admin);
    await api.PUT("/admin/supporter_program", { body: { enabled: true } });
    const { role, plan, subscriber } = supporters;
    if (!body((await api.GET("/admin/roles")).data).items.some((r) => r.key === role.key)) {
      await api.POST("/admin/roles", {
        body: { key: role.key, position: role.position, permissions: role.permissions },
        headers: { "Idempotency-Key": idempotencyKey(`role-${role.key}`) },
      });
      await api.PATCH("/admin/roles/{role}", { params: { path: { role: role.key } }, body: { badge: role.badge } });
    }
    const plans = async () => body((await api.GET("/admin/supporter_plans")).data).items;
    if (!(await plans()).some((p) => p.name === plan.name)) {
      await api.POST("/admin/supporter_plans", {
        body: {
          role: role.key,
          name: plan.name,
          description: plan.description,
          monthly_price: plan.monthlyPrice,
          annual_discount_percent: plan.annualDiscountPercent,
          emoji_limit: plan.emojiLimit,
          rank: plan.rank,
          visibility: "public",
        },
        headers: { "Idempotency-Key": idempotencyKey(plan.key) },
      });
    }
    const ready = await until("the prices of the supporter plan", async () =>
      (await plans()).find((p) => p.name === plan.name && p.prices_ready),
    );

    const own = await this.api(subscriber.username);
    const current = body((await own.GET("/supporter/subscription")).data).subscription;
    if (current?.state !== "active") {
      const checkout = body(
        (
          await own.POST("/supporter/checkout", {
            body: { plan_id: ready.id, interval: subscriber.interval },
            headers: { "Idempotency-Key": idempotencyKey(`${plan.key}-${subscriber.username}`) },
          })
        ).data,
      );
      // The fake provider's checkout page (a test build only): pay, as the person would press the button.
      const browser = new Browser();
      const page = await browser.request(checkout.redirect_url);
      if (page.status !== 200) throw new Error(`demo: the fake checkout answered ${String(page.status)}`);
      location(await browser.request(checkout.redirect_url, { action: "pay" }), checkout.redirect_url);
      await own.POST("/supporter/subscription/sync");
      await until("the supporter subscription", async () => {
        const s = body((await own.GET("/supporter/subscription")).data).subscription;
        return s?.state === "active" ? s : undefined;
      });
    }
    await own.PUT("/preferences/supporter_listing", { body: { listed: subscriber.listed } });
    await this.showBadges(subscriber.username);
  }

  async enterCosts(): Promise<void> {
    const api = await this.api(admin);
    const existing = body((await api.GET("/admin/cost_items")).data).items.map((c) => c.name);
    for (const c of costItems) {
      if (existing.includes(c.name)) continue;
      await api.POST("/admin/cost_items", {
        body: {
          schedule: c.schedule,
          interval: c.interval,
          starts_on: c.startsOn,
          name: c.name,
          provider: c.provider,
          category: c.category,
          price: { amount: c.amount, currency: c.currency },
          ...(c.memo === undefined ? {} : { memo: c.memo }),
        },
        headers: { "Idempotency-Key": idempotencyKey(c.key) },
      });
    }
  }

  async createInvites(): Promise<void> {
    const api = await this.api(admin);
    const staff = body((await api.GET("/admin/invites")).data).items.filter((i) => i.creator_kind === "staff");
    for (const invite of invites.staff.slice(staff.length)) {
      this.#o.runMy(["invite", "create", "-max-uses", String(invite.maxUses), "-expires-in", invite.expiresIn]);
    }
    for (const invite of invites.members) {
      const own = await this.api(invite.by);
      const mine = body((await own.GET("/invites")).data).items;
      if (mine.some((i) => i.max_uses === invite.maxUses)) continue;
      await own.POST("/invites", {
        body: { max_uses: invite.maxUses },
        headers: { "Idempotency-Key": idempotencyKey(invite.key) },
      });
    }
  }

  async announce(): Promise<void> {
    const api = await this.api(admin);
    const existing = body((await api.GET("/admin/announcements")).data).items.map((a) => a.text);
    for (const a of announcements) {
      if (existing.includes(a.text)) continue;
      await api.POST("/admin/announcements", {
        body: { title: a.title, text: a.text, publish: true },
        headers: { "Idempotency-Key": idempotencyKey(a.key) },
      });
    }
  }
}

/** The image of a person's avatar: a disc of their color on a lighter gradient. */
function avatarOf(p: Persona): Drawing {
  const [r, g, b] = p.color;
  const light = (c: number) => Math.round(c + (255 - c) * 0.6);
  return {
    width: 256,
    height: 256,
    top: [light(r), light(g), light(b)],
    bottom: p.color,
    shapes: [
      { kind: "circle", x: 128, y: 104, r: 52, color: [255, 255, 255] },
      { kind: "circle", x: 128, y: 250, r: 96, color: [255, 255, 255] },
    ],
  };
}

/** Applies the scenario to the server, in the order of demo/scenario.ts. */
export async function seed(o: SeedOptions): Promise<void> {
  const s = new Seeder(o);
  s.createPeople();
  s.configureInstance();
  await s.addRules();
  await s.setUpProfiles();
  await s.setUpRoles();
  await s.follow();
  await s.makeLists();
  await s.writePosts();
  await s.interact();
  await s.report();
  await s.moderate();
  await s.setUpSupporters();
  await s.enterCosts();
  await s.createInvites();
  await s.announce();
}

/**
 * What the server holds of the scenario, as counts: each person's posts, notifications, follows, and reports, and the
 * administrators' records, the audit log included, which every change of the admin API adds to. A second run of the
 * seed leaves it as it is.
 */
export async function census(o: SeedOptions): Promise<Record<string, number>> {
  const s = new Seeder(o);
  const count = async (
    fetch: (cursor: string | undefined) => Promise<{ items: readonly unknown[]; next_cursor?: string | null }>,
  ) => {
    let n = 0;
    for await (const _ of all(fetch)) n++;
    return n;
  };
  const out: Record<string, number> = {};
  for (const p of personas) {
    const api = await s.api(p.username);
    const me = body((await api.GET("/accounts/verify_credentials")).data);
    out[`${p.username}.statuses`] = await count(async (cursor) =>
      body((await api.GET("/accounts/{id}/statuses", { params: { path: { id: me.id }, query: at(cursor) } })).data),
    );
    out[`${p.username}.notifications`] = await count(async (cursor) =>
      body((await api.GET("/notifications", { params: { query: at(cursor) } })).data),
    );
    out[`${p.username}.followers`] = me.followers_count ?? 0;
    out[`${p.username}.following`] = me.following_count ?? 0;
    out[`${p.username}.reports`] = body((await api.GET("/reports")).data).items.length;
    out[`${p.username}.invites`] = body((await api.GET("/invites")).data).items.length;
    out[`${p.username}.lists`] = body((await api.GET("/lists")).data).items.length;
  }
  const api = await s.api(admin);
  out["audit_log"] = await count(async (cursor) =>
    body((await api.GET("/admin/audit_log", { params: { query: at(cursor) } })).data),
  );
  out["reports"] = await count(async (cursor) =>
    body((await api.GET("/admin/reports", { params: { query: at(cursor) } })).data),
  );
  out["rules"] = body((await api.GET("/admin/rules")).data).items.length;
  out["roles"] = body((await api.GET("/admin/roles")).data).items.length;
  out["announcements"] = body((await api.GET("/admin/announcements")).data).items.length;
  out["cost_items"] = body((await api.GET("/admin/cost_items")).data).items.length;
  out["supporter_plans"] = body((await api.GET("/admin/supporter_plans")).data).items.length;
  out["invites"] = body((await api.GET("/admin/invites")).data).items.length;
  return out;
}

/** Runs `bin/my` in this process's environment. */
function runMyHere(args: readonly string[], input = ""): string {
  const bin = process.env["DEMO_MY_BIN"] ?? join(resolve(import.meta.dirname, "../../.."), "bin/my");
  return execFileSync(bin, args, { input, encoding: "utf8", stdio: ["pipe", "pipe", "pipe"] });
}

if (import.meta.main) {
  const { values } = parseArgs({
    options: { server: { type: "string" }, census: { type: "boolean", default: false } },
  });
  if (values.server === undefined) throw new Error("demo/seed.ts: --server <origin> is required");
  const options: SeedOptions = {
    server: values.server,
    runMy: runMyHere,
    log: (line) => {
      console.log(`demo: ${line}`);
    },
  };
  if (values.census) {
    console.log(JSON.stringify(await census(options)));
  } else {
    await seed(options);
    console.log("demo: seeded");
  }
}
