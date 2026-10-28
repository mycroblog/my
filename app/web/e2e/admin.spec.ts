// SPDX-License-Identifier: AGPL-3.0-only
// The admin screens in every width × locale project (ADR-0027): the remote servers and their history, resuming
// deliveries, and the queue dashboard with a job's attempt history and running a failed job again, each passing axe
// without scrolling sideways. `vite preview` has
// no API, so each test plays the server of a signed-in operator with page.route; names change with the
// locale, so the structure is checked by role, the names in the project's locale catalog and the identifiers the
// server sends (domains, job types).
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** The signed-in operator's account, as GET /accounts/verify_credentials answers, with both admin permissions. */
const operator = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  discoverable: true,
  indexable: true,
  ai_opt_out: false,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: ["manage_domains", "manage_jobs"],
};

// A long domain and a long payload, to show that they wrap at 320 px.
const longDomain = "a-very-long-subdomain-of-a-remote-server-for-testing.social.example";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the admin API of a signed-in operator; returns the jobs run again and the servers marked available. */
async function operatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let paused: Record<string, unknown> = {
    domain: longDomain,
    state: "unavailable",
    since: "2026-10-10T08:00:00Z",
    failure_days: 7,
    last_failure_day: "2026-10-10",
  };
  let events = [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000e1",
      kind: "marked_unavailable",
      reason: "failure_days",
      occurred_at: "2026-10-10T08:00:00Z",
    },
  ];
  let dead = [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000d1",
      type: "federation.deliver",
      state: "dead",
      payload: JSON.stringify({
        delivery_id: "0192f0c4-7a8e-7000-8000-0000000000aa",
        inbox: `https://${longDomain}/users/someone-with-a-long-name/inbox`,
      }),
      idempotency_key: "deliver:0192f0c4-7a8e-7000-8000-0000000000aa",
      attempts: 9,
      last_error: "502 Bad Gateway",
      created_at: "2026-10-13T10:00:00Z",
      failed_at: "2026-10-17T10:00:00Z",
    },
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000d0",
      type: "kv.cleanup",
      state: "dead",
      payload: "null",
      attempts: 3,
      last_error: "db down",
      created_at: "2026-10-16T10:00:00Z",
      failed_at: "2026-10-16T11:00:00Z",
    },
  ];
  const pending = [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000c1",
      type: "federation.deliver",
      state: "retrying",
      payload: JSON.stringify({ delivery_id: "0192f0c4-7a8e-7000-8000-0000000000ab" }),
      ordering_key: `inbox:https://${longDomain}/users/someone-with-a-long-name/inbox`,
      attempts: 2,
      last_error: `Post "https://${longDomain}/users/someone-with-a-long-name/inbox": dial tcp: connection refused`,
      run_at: "2026-10-17T10:05:00Z",
      created_at: "2026-10-17T09:00:00Z",
    },
  ];
  const attempts = [
    {
      attempt: 2,
      type: "federation.deliver",
      outcome: "retry",
      error: `Post "https://${longDomain}/users/someone-with-a-long-name/inbox": dial tcp: connection refused`,
      worker: "worker-0123456789abcdef",
      started_at: "2026-10-17T10:00:00Z",
      finished_at: "2026-10-17T10:00:01Z",
    },
  ];
  const actions: string[] = [];
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, {
      access_token: "e2e-token",
      token_type: "Bearer",
      expires_in: 3600,
      scope: "read write follow admin",
    }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance")
      return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, operator);
    if (key === "GET /preferences") {
      return json(route, {
        locale,
        time_zone: "UTC",
        default_visibility: "public",
        default_post_language: "und",
        require_media_description: false,
        timeline_languages: [],
      });
    }
    if (key === "GET /admin/instances") {
      return json(route, {
        items: [
          paused,
          { domain: "flaky.example", state: "available", failure_days: 2, last_failure_day: "2026-10-16" },
        ],
      });
    }
    if (key === `GET /admin/instances/${longDomain}`) return json(route, paused);
    if (key === `GET /admin/instances/${longDomain}/availability_events`) return json(route, { items: events });
    if (key === `POST /admin/instances/${longDomain}/mark_available`) {
      actions.push(key);
      paused = { domain: longDomain, state: "available", since: "2026-10-17T00:00:00Z", failure_days: 0 };
      events = [
        {
          id: "0192f0c4-7a8e-7000-8000-0000000000e2",
          kind: "marked_available",
          reason: "operator",
          occurred_at: "2026-10-17T00:00:00Z",
        },
        ...events,
      ];
      return json(route, paused);
    }
    if (key === "GET /admin/queue") {
      return json(route, {
        adapter: "db",
        pending_listing: true,
        counts: [
          { state: "retrying", type: "federation.deliver", count: pending.length },
          ...dead.map((job) => ({ state: "dead", type: job.type, count: 1 })),
        ],
        oldest_ready_at: "2026-10-17T09:30:00Z",
      });
    }
    if (key === "GET /admin/jobs")
      return json(route, { items: new URL(request.url()).searchParams.get("state") === "dead" ? dead : pending });
    if (/^GET \/admin\/jobs\/[^/]+\/attempts$/.test(key)) return json(route, { items: attempts });
    const retry = /^POST \/admin\/jobs\/([^/]+)\/retry$/.exec(key);
    if (retry) {
      actions.push(key);
      dead = dead.filter((job) => job.id !== retry[1]);
      return route.fulfill({ status: 204, headers: { "Cache-Control": "no-store" } });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return actions;
}

const pages = [
  "/settings",
  "/moderation/instances",
  `/moderation/instances/${longDomain}`,
  "/admin/jobs",
  "/admin/jobs?state=dead",
] as const;

for (const path of pages) {
  test.describe(`page ${path}`, () => {
    test.beforeEach(async ({ page }) => {
      await operatorServer(page);
      await page.goto(path);
      await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
      // A list shows a table, a page of sections its headings.
      await expect(
        page.getByRole("main").getByRole("heading", { level: 2 }).or(page.getByRole("main").getByRole("table")).first(),
      ).toBeVisible();
    });

    test("has no axe violations", async ({ makeAxeBuilder }) => {
      await expectNoAxeViolations(makeAxeBuilder);
    });

    test("does not scroll horizontally", async ({ page }) => {
      await expectNoHorizontalScroll(page);
    });
  });
}

test.describe("remote servers", () => {
  test("lists the servers, opens one with its history, and resumes deliveries to it", async ({
    page,
    makeAxeBuilder,
  }) => {
    const actions = await operatorServer(page);
    await page.goto("/moderation/instances");
    const main = page.getByRole("main");

    const servers = main.getByRole("table");
    await expect(servers.getByRole("rowheader")).toHaveText([longDomain, "flaky.example"]);
    await expect(servers.getByRole("row").nth(1).getByRole("link", { name: longDomain })).toBeVisible();
    await expect(servers.getByRole("row").nth(2).getByRole("link", { name: "flaky.example" })).toBeVisible();
    // The first server's state change and the last failure day of each.
    await expect(servers.locator("time")).toHaveCount(3);

    await main.getByRole("link", { name: longDomain }).click();
    await expect(main.getByRole("heading", { level: 1, name: longDomain })).toBeFocused();
    const resume = main.getByRole("button", { name: message("admin.instance.mark_available"), exact: true });
    await resume.click();
    await expect(resume).toBeHidden();
    await expect(
      main.getByRole("table", { name: message("admin.instance.history.title"), exact: true }).getByRole("rowheader"),
    ).toHaveCount(2);
    expect(actions).toEqual([`POST /admin/instances/${longDomain}/mark_available`]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("queue dashboard", () => {
  test("opens a job's attempt history", async ({ page, makeAxeBuilder }) => {
    await operatorServer(page);
    await page.goto("/admin/jobs");
    const panel = page.getByRole("main").getByRole("tabpanel");
    const job = panel
      .getByRole("listitem")
      .filter({ has: page.getByRole("heading", { level: 3, name: "federation.deliver" }) });
    await expect(job).toBeVisible();

    const history = job.getByRole("button", {
      name: message("admin.jobs.history.toggle_job").replace("{type}", "federation.deliver"),
      exact: true,
    });
    await expect(history).toHaveAttribute("aria-expanded", "false");
    await history.click();
    await expect(history).toHaveAttribute("aria-expanded", "true");
    await expect(job.getByRole("heading", { level: 4 })).toHaveCount(1);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("runs a failed job again: it leaves the list and focus moves to the next job", async ({
    page,
    makeAxeBuilder,
  }) => {
    const actions = await operatorServer(page);
    await page.goto("/admin/jobs?state=dead");
    const panel = page.getByRole("main").getByRole("tabpanel");
    await expect(panel.getByRole("heading", { level: 3, name: "federation.deliver" })).toBeVisible();

    await expect(panel.getByRole("list").first()).toMatchAriaSnapshot(`
      - list:
        - listitem:
          - heading "federation.deliver" [level=3]
          - button
        - listitem:
          - heading "kv.cleanup" [level=3]
          - button
    `);

    const row = (type: string) =>
      panel.getByRole("listitem").filter({ has: page.getByRole("heading", { level: 3, name: type }) });
    await row("federation.deliver").getByRole("button").first().click();
    await expect(panel.getByRole("heading", { level: 3, name: "federation.deliver" })).toBeHidden();
    await expect(row("kv.cleanup").getByRole("button").first()).toBeFocused();
    expect(actions).toEqual(["POST /admin/jobs/0192f0c4-7a8e-7000-8000-0000000000d1/retry"]);
    await expectNoAxeViolations(makeAxeBuilder);
  });
});
