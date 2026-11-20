// SPDX-License-Identifier: AGPL-3.0-only

// What a screenshot shows beyond the seed's scenario, made on the demo server by the scenario's people through the
// client and admin APIs, as the seed makes the scenario (src/seed/seed.ts): the reports in the moderators' queue, the
// supporter plans and their listed members, and a live voice room. Every name is fixed, so that each run shows the
// same; the demo server is new on every run, so nothing is made twice.

import type { Browser } from "@playwright/test";
import type { Api, components } from "@mycroblog/mykit";
import { personOf } from "../src/scenario";
import { signIn } from "../src/seed/sign-in";

type Status = components["schemas"]["Status"];

function body<T>(data: T | undefined, what: string): T {
  if (data === undefined) throw new Error(`shots: ${what} was refused`);
  return data;
}

/** Signs the scenario's people in through the API, each once, from an address of the shots' own. */
export class Stage {
  readonly #server: string;
  readonly #runMy: (args: readonly string[], input?: string) => string;
  readonly #apis = new Map<string, Promise<Api>>();

  /** The demo server at `server`, and `bin/my` against its database. */
  constructor(server: string, runMy: (args: readonly string[], input?: string) => string) {
    this.#server = server;
    this.#runMy = runMy;
  }

  api(username: string): Promise<Api> {
    let api = this.#apis.get(username);
    if (api === undefined) {
      api = signIn(this.#server, personOf(username), undefined, undefined, "198.18.255.1");
      this.#apis.set(username, api);
    }
    return api;
  }

  async accountId(username: string): Promise<string> {
    const { data } = await (
      await this.api(username)
    ).GET("/accounts/lookup", { params: { query: { acct: username } } });
    return body(data, `the lookup of ${username}`).id;
  }

  /** The post of `author` whose text holds `text`, among their newest. */
  async post(author: string, text: string): Promise<Status> {
    const api = await this.api(author);
    const id = await this.accountId(author);
    const { data } = await api.GET("/accounts/{id}/statuses", { params: { path: { id }, query: { limit: 40 } } });
    const found = body(data, `the posts of ${author}`).items.find((s) => s.content.includes(text));
    if (found === undefined) throw new Error(`shots: no post of ${author} holds ${text}`);
    return found;
  }

  /**
   * Waits until the server's job queue holds no job that is due: the notifications, the counts and the timelines that
   * the jobs make after a write are all there, so that a screen shows the same on every run.
   */
  async settled(): Promise<void> {
    const admin = await this.api("hana");
    let quiet = 0;
    for (let attempt = 0; attempt < 300 && quiet < 3; attempt++) {
      // The pending jobs come the earliest first: the first one is due, or none is.
      const { data } = await admin.GET("/admin/jobs", { params: { query: { limit: 1 } } });
      const [next] = body(data, "the job queue").items;
      const due = next !== undefined && (next.run_at === undefined || Date.parse(next.run_at) <= Date.now());
      quiet = due ? 0 : quiet + 1;
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
    if (quiet < 3) throw new Error("shots: the job queue never went quiet");
  }

  /** Open reports from the scenario's people, of QuickDeals' advertisement, one of them taken by Marco. */
  async reports(): Promise<void> {
    const spam = await this.post("quickdeals", "Huge discounts today only");
    const target = await this.accountId("quickdeals");
    const filed = [
      { by: "sam", category: "spam", comment: "The same link under every post of mine." },
      { by: "layla", category: "spam", comment: "Advertising, and nothing else." },
      { by: "mio", category: "other", comment: "It follows and unfollows me every day." },
      { by: "ren", category: "spam", comment: "" },
    ] as const;
    const ids: string[] = [];
    for (const r of filed) {
      const { data } = await (
        await this.api(r.by)
      ).POST("/reports", {
        body: {
          account_id: target,
          status_ids: [spam.id],
          category: r.category,
          forward: false,
          ...(r.comment === "" ? {} : { comment: r.comment }),
        },
      });
      ids.push(body(data, `the report of ${r.by}`).id);
    }
    const first = ids[0];
    if (first === undefined) return;
    const { error } = await (
      await this.api("marco")
    ).POST("/admin/reports/{id}/assign_to_self", { params: { path: { id: first } } });
    if (error !== undefined) throw new Error("shots: Marco could not take the report");
  }

  /**
   * The supporter program with two plans on roles of their own, the supporter list shown to everyone, and Sam, Marco
   * and Layla subscribed through the fake provider's checkout in `browser` and listed: people of English, since the
   * checkout speaks the member's language and its Pay is pressed by its English name.
   */
  async supporters(browser: Browser): Promise<void> {
    const admin = await this.api("hana");
    const plans = [
      {
        role: "coffee",
        name: "Coffee",
        amount: "500",
        rank: 1,
        description: "A cup of coffee for the server, every month.",
      },
      {
        role: "loaf",
        name: "A whole loaf",
        amount: "1500",
        rank: 2,
        description: "Bread for the whole server, every month.",
      },
    ] as const;
    for (const plan of plans) {
      await admin.POST("/admin/roles", { body: { key: plan.role, position: plan.rank, permissions: [] } });
      const { error } = await admin.POST("/admin/supporter_plans", {
        body: {
          role: plan.role,
          name: plan.name,
          description: plan.description,
          monthly_price: { amount: plan.amount, currency: "JPY" },
          annual_discount_percent: 10,
          emoji_limit: 0,
          rank: plan.rank,
          visibility: "public",
        },
      });
      if (error !== undefined) throw new Error(`shots: the plan ${plan.name} was refused`);
    }
    if ((await admin.PUT("/admin/supporter_program", { body: { enabled: true } })).error !== undefined)
      throw new Error("shots: the supporter program was not enabled");
    this.#runMy(["instance", "set", "supporter_list_visibility", "-"], "everyone");
    let ids: Record<string, string> = {};
    for (let attempt = 0; attempt < 100 && Object.keys(ids).length < plans.length; attempt++) {
      const { data } = await admin.GET("/instance/supporter_plans");
      ids = Object.fromEntries((data?.items ?? []).map((p) => [p.name, p.id]));
      if (Object.keys(ids).length < plans.length) await new Promise((resolve) => setTimeout(resolve, 100));
    }
    const members = [
      { by: "sam", plan: "A whole loaf" },
      { by: "marco", plan: "Coffee" },
      { by: "layla", plan: "Coffee" },
    ] as const;
    for (const m of members) {
      const api = await this.api(m.by);
      const planId = ids[m.plan];
      if (planId === undefined) throw new Error(`shots: the plan ${m.plan} never took subscriptions`);
      const { data } = await api.POST("/supporter/checkout", { body: { plan_id: planId, interval: "month" } });
      const checkout = body(data, `the checkout of ${m.by}`);
      const page = await browser.newPage();
      try {
        await page.goto(checkout.redirect_url);
        await page.getByRole("button", { name: "Pay", exact: true }).click();
        await page.waitForURL((url) => !url.pathname.startsWith("/payments/"));
      } finally {
        await page.close();
      }
      await api.POST("/supporter/subscription/sync");
      for (let attempt = 0; attempt < 100; attempt++) {
        const { data: own } = await api.GET("/supporter/subscription");
        if (own?.subscription?.state === "active") break;
        await new Promise((resolve) => setTimeout(resolve, 100));
      }
      if ((await api.PUT("/preferences/supporter_listing", { body: { listed: true } })).error !== undefined)
        throw new Error(`shots: ${m.by} could not be listed`);
    }
  }

  /**
   * A live voice room that Hana hosts, with Sam as a co-host, Yuki and Layla speaking, and Marco, Ren and Mio
   * listening, Marco with a raised hand; each joins with a session of the API. Its ID.
   */
  async voiceRoom(): Promise<string> {
    const host = await this.api("hana");
    const { data } = await host.POST("/voice/rooms", {
      body: {
        name: "Saturday meetup: bread and tea",
        icon: "🍞",
        description: "Planning the meetup at the station. Bring a loaf, or a thermos.",
        hashtags: ["baking"],
        speakers: "manual",
      },
    });
    const room = body(data, "the room").room.id;
    const path = { id: room };
    for (const who of ["sam", "yuki", "layla", "marco", "ren", "mio"]) {
      const { error } = await (await this.api(who)).POST("/voice/rooms/{id}/participation", { params: { path } });
      if (error !== undefined) throw new Error(`shots: ${who} could not join the room`);
    }
    const cohost = await host.POST("/voice/rooms/{id}/participants/{account_id}/cohost", {
      params: { path: { id: room, account_id: await this.accountId("sam") } },
    });
    if (cohost.error !== undefined) throw new Error("shots: Sam could not become a co-host");
    for (const who of ["sam", "yuki", "layla"]) {
      if (who !== "sam") {
        const granted = await host.POST("/voice/rooms/{id}/participants/{account_id}/speaking_permission", {
          params: { path: { id: room, account_id: await this.accountId(who) } },
        });
        if (granted.error !== undefined) throw new Error(`shots: ${who} was not let speak`);
      }
      const { error } = await (await this.api(who)).POST("/voice/rooms/{id}/speaker", { params: { path } });
      if (error !== undefined) throw new Error(`shots: ${who} could not speak`);
    }
    const { error } = await (await this.api("marco")).POST("/voice/rooms/{id}/hand", { params: { path } });
    if (error !== undefined) throw new Error("shots: Marco could not raise a hand");
    return room;
  }
}
