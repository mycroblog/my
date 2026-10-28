// SPDX-License-Identifier: AGPL-3.0-only
// The relays screen in every width × locale project (ADR-0061, ADR-0027): an administrator registers and joins a
// relay, sees it listed awaiting its answer, leaves it, joins it again, leaves it and deletes it after its dialog,
// each step passing axe without scrolling sideways. `vite preview` has no API, so each test plays the server of a
// signed-in administrator with page.route; names change with the locale, so the structure is checked by role, by
// the names in the project's locale catalog and by what the server sends.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];
/** The client message `key` in the project's locale, with `{name}` replaced by `name`. */
function message(key: string, name = ""): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text.replace("{name}", name);
}

/** The signed-in administrator's account, as GET /accounts/verify_credentials answers, with manage_settings. */
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
  permissions: ["manage_settings"],
};

const relayUrl = "https://relay.example/inbox";
/** The relay in sentences: the host name of its URL, isolated (useRelays.relayName). */
const name = "⁦relay.example⁩";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the API of a signed-in administrator with no relay at first. Returns what was sent. */
async function administratorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const sent: string[] = [];
  let relays: Schemas["AdminRelay"][] = [];
  const withState = (state: Schemas["RelayState"]) => {
    relays = relays.map((relay) => ({ ...relay, state, state_changed_at: "2026-10-20T01:00:00Z" }));
    return relays[0];
  };
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
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance")
      return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "approval" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500 },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, administrator);
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
    if (key === "GET /admin/counts")
      return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    if (key === "GET /admin/relays") return json(route, { items: relays });
    if (key === "POST /admin/relays") {
      const body = request.postDataJSON() as Schemas["CreateRelayRequest"];
      sent.push(`create ${body.url} ${body.protocol ?? "auto"} ${String(body.receive)}`);
      const relay: Schemas["AdminRelay"] = {
        id: "0192f0c4-7a8e-7000-8000-0000000000d1",
        protocol: body.protocol ?? "mastodon",
        url: body.url,
        inbox_url: body.url,
        receive: body.receive ?? true,
        state: "pending",
        state_changed_at: "2026-10-20T00:00:00Z",
        taken_last_day: 0,
        dropped_last_day: 0,
        domain_suspended: false,
        created_at: "2026-10-20T00:00:00Z",
      };
      relays = [relay];
      return json(route, relay);
    }
    const membership = /^POST \/admin\/relays\/([^/]+)\/(follow|unfollow)$/.exec(key);
    if (membership) {
      sent.push(membership[2] ?? "");
      return json(route, withState(membership[2] === "follow" ? "pending" : "left"));
    }
    if (/^DELETE \/admin\/relays\/[^/]+$/.test(key)) {
      sent.push("delete");
      relays = [];
      return route.fulfill({ status: 204 });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("relays", () => {
  test("joins a relay, leaves it, joins it again and deletes it", async ({ page, makeAxeBuilder }) => {
    const sent = await administratorServer(page);
    await page.goto("/admin/relays");
    const main = page.getByRole("main");
    const form = main.getByRole("region", { name: message("admin.relays.new.title"), exact: true });
    await expect(form).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await form.getByRole("textbox", { name: message("admin.relays.url"), exact: true }).fill(relayUrl);
    await expect(form.getByRole("checkbox", { name: message("admin.relays.receive"), exact: true })).toBeChecked();
    await form.getByRole("button", { name: message("admin.relays.submit"), exact: true }).click();

    const list = main.getByRole("region", { name: message("admin.relays.list.title"), exact: true });
    await expect(list.getByRole("rowheader", { name: /^relay\.example/u })).toBeVisible();
    await expect(list.getByText(message("admin.relays.state.pending"), { exact: true })).toBeVisible();
    expect(sent).toEqual([`create ${relayUrl} auto true`]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await list.getByRole("button", { name: message("admin.relays.unfollow_relay", name), exact: true }).click();
    await expect(
      list.getByRole("button", { name: message("admin.relays.follow_relay", name), exact: true }),
    ).toBeVisible();
    await list.getByRole("button", { name: message("admin.relays.follow_relay", name), exact: true }).click();
    await expect(
      list.getByRole("button", { name: message("admin.relays.unfollow_relay", name), exact: true }),
    ).toBeVisible();
    await list.getByRole("button", { name: message("admin.relays.unfollow_relay", name), exact: true }).click();

    await list.getByRole("button", { name: message("admin.relays.delete_relay", name), exact: true }).click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("admin.relays.delete"), exact: true }).click();
    await expect(
      list.getByRole("heading", { level: 3, name: message("admin.relays.empty"), exact: true }),
    ).toBeVisible();
    expect(sent).toEqual([`create ${relayUrl} auto true`, "unfollow", "follow", "unfollow", "delete"]);
  });
});
