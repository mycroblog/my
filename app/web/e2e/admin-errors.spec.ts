// SPDX-License-Identifier: AGPL-3.0-only
// The errors screens in every width × locale project (ADR-0103 §8, ADR-0027): an administrator with manage_errors sees
// the open error groups, opens one by keyboard, sees its latest occurrence with its stack and hourly counts, and
// resolves it by keyboard, after which focus is on the state's heading; each step passes axe without scrolling
// sideways. `vite preview` has no API, so the test plays the server of a signed-in administrator with page.route;
// names change with the locale, so the structure is checked by role, by the names in the project's locale catalog
// and by what the server sends.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** The signed-in administrator's account, as GET /accounts/verify_credentials answers, with manage_errors. */
const administrator = {
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
  permissions: ["manage_errors"],
};

const groupId = "0192f0c4-7a8e-7000-8000-0000000000e1";

/** A group with a long type, place and message, which must wrap rather than scroll the page sideways. */
function group(state: Schemas["ErrorGroupState"]): Schemas["ErrorGroup"] {
  return {
    id: groupId,
    fingerprint: "0123456789abcdef0123456789abcdef",
    state,
    state_since: "2026-10-20T00:00:00Z",
    ...(state === "resolved" ? { resolved_version: "0.3.1" } : {}),
    source: "server",
    kind: "panic",
    where: "client_api GET /api/v1/accounts/{id}/statuses/{status_id}/favourited_by",
    type: "*github.com/mycroblog/my/internal/adapter/db/postgres.constraintViolationError",
    message: "runtime error: invalid memory address or nil pointer dereference in a very long message that goes on",
    count_24h: 3,
    count_7d: 12,
    first_seen_at: "2026-10-20T00:00:00Z",
    last_seen_at: "2026-10-21T05:30:00Z",
    first_version: "0.3.0",
    last_version: "0.3.1",
  };
}

function detail(state: Schemas["ErrorGroupState"]): Schemas["ErrorGroupDetail"] {
  return {
    ...group(state),
    sample: {
      type: "*fmt.wrapError > *github.com/mycroblog/my/internal/adapter/db/postgres.constraintViolationError",
      message: "runtime error: invalid memory address or nil pointer dereference",
      stack:
        "goroutine 1 [running]:\ngithub.com/mycroblog/my/internal/app/status.(*Service).FavouritedByWithAVeryLongFunctionName(...)\n\tgithub.com/mycroblog/my/internal/app/status/favourited_by.go:42 +0x32",
      request_id: "01JB0000000000000000000000",
      trace_id: "4bf92f3577b34da6a3ce929d0e0e4736",
      version: "0.3.1",
      role: "all",
      occurred_at: "2026-10-21T05:30:00Z",
    },
    hourly_counts: [
      { hour: "2026-10-20T00:00:00Z", count: 9, last_version: "0.3.0" },
      { hour: "2026-10-21T05:00:00Z", count: 3, last_version: "0.3.1" },
    ],
    events: state === "resolved" ? [{ kind: "resolved", actor: "user", version: "0.3.1", occurred_at: "2026-10-21T06:00:00Z" }] : [],
  };
}

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/** Plays the API of a signed-in administrator with one open error group. Returns what was sent. */
async function administratorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const sent: string[] = [];
  let state: Schemas["ErrorGroupState"] = "open";
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow admin" }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance") return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "approval" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, administrator);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "und", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /admin/counts") return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/error_groups") {
      const wanted = url.searchParams.get("state");
      return json(route, { items: wanted === null || wanted === state ? [group(state)] : [] });
    }
    if (key === `GET /admin/error_groups/${groupId}`) return json(route, detail(state));
    if (key === `POST /admin/error_groups/${groupId}/resolve`) {
      sent.push("resolve");
      state = "resolved";
      return json(route, group(state));
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("errors", () => {
  test("lists the open groups, opens one by keyboard, and resolves it", async ({ page, makeAxeBuilder }) => {
    const sent = await administratorServer(page);
    await page.goto("/admin/errors");
    const main = page.getByRole("main");
    const table = main.getByRole("table", { name: message("admin.errors.list"), exact: true });
    await expect(table).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    const link = table.getByRole("link", { name: group("open").type, exact: true });
    await link.focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/admin/errors/${groupId}$`, "u"));
    const state = main.getByRole("region", { name: message("admin.errors.state"), exact: true });
    await expect(state).toBeVisible();
    const sample = main.getByRole("region", { name: message("admin.errors.sample"), exact: true });
    await expect(sample.getByText("4bf92f3577b34da6a3ce929d0e0e4736", { exact: true })).toBeVisible();
    await expect(main.getByRole("table", { name: message("admin.errors.hourly"), exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await state.getByRole("button", { name: message("admin.errors.resolve"), exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(state.getByRole("status")).toHaveText(message("admin.errors.resolved"));
    await expect(state.getByRole("heading", { name: message("admin.errors.state"), exact: true })).toBeFocused();
    await expect(state.getByRole("button", { name: message("admin.errors.resolve"), exact: true })).toHaveCount(0);
    expect(sent).toEqual(["resolve"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
