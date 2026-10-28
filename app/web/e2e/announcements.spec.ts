// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements in every width × locale project (FR-726, ADR-0027): a user sees the published ones
// above the home timeline with the unread one marked in words, and marks it read; an administrator writes an
// announcement and withdraws it after its dialog. Each step passes axe without scrolling sideways. `vite preview`
// has no API, so each test plays the server with page.route; names change with the locale, so the elements are
// found by role and by the names in the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];
/** The client message `key` in the project's locale, with each `{name}` of `values` replaced. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return Object.entries(values).reduce((out, [name, value]) => out.replace(`{${name}}`, value), text);
}

/** The signed-in user's account, as GET /accounts/verify_credentials answers, with `permissions`. */
function account(permissions: Schemas["Permission"][]) {
  return {
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
    source: { note: "", fields: [] },
    posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
    permissions,
  };
}

/** An unread announcement with a long word and a link, and a read one in Arabic, which reads right to left. */
const unread: Schemas["Announcement"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000f1",
  content: `<p>Scheduled maintenance of ${"the-database-and-the-media-storage-".repeat(3)}tonight. See <a href="https://status.example/">the status page</a>.</p>`,
  published_at: "2026-10-21T00:00:00Z",
  ends_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-21T00:00:00Z",
  read: false,
};
const read: Schemas["Announcement"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000f2",
  content: "<p>مرحبا بكم في الخادم</p>",
  published_at: "2026-10-20T00:00:00Z",
  updated_at: "2026-10-20T00:00:00Z",
  read: true,
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the API of a signed-in user with `permissions`; `extra` answers the other operations, or null for a 404. */
async function serveApi(
  page: Page,
  permissions: Schemas["Permission"][],
  extra: (key: string, route: Route) => Promise<void> | null,
) {
  const locale = test.info().project.use.locale ?? "en";
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
    if (key === "GET /accounts/verify_credentials") return json(route, account(permissions));
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
    if (key === "GET /notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (key === "GET /timelines/home") return json(route, { items: [] });
    if (key === "GET /admin/counts")
      return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    const answered = extra(key, route);
    if (answered !== null) return answered;
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

test.describe("announcements", () => {
  test("shows the announcements above the home timeline and marks the unread one read", async ({
    page,
    makeAxeBuilder,
  }) => {
    let announcements = [unread, read];
    await serveApi(page, [], (key, route) => {
      if (key === "GET /announcements") return json(route, { items: announcements });
      if (key === `POST /announcements/${unread.id}/read`) {
        announcements = [{ ...unread, read: true }, read];
        return json(route, announcements[0]);
      }
      return null;
    });
    await page.goto("/home");
    const region = page.getByRole("main").getByRole("region", { name: message("announcements.title") });
    const markRead = region.getByRole("button", { name: message("announcements.mark_read"), exact: true });
    await expect(markRead).toBeVisible();
    await expect(region.getByRole("article")).toHaveCount(2);
    await expect(region.getByText(message("announcements.unread"), { exact: true })).toBeVisible();
    // The direction follows the text of the whole body (PostContent), the paragraph's parent.
    await expect(region.getByText("مرحبا بكم في الخادم").locator("xpath=..")).toHaveAttribute("dir", "auto");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await markRead.click();
    await expect(markRead).toBeHidden();
    await expect(region.getByRole("heading", { level: 3 }).first()).toBeFocused();
    await expect(region.getByText(message("announcements.unread"), { exact: true })).toBeHidden();
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("writes an announcement and withdraws it after its dialog", async ({ page, makeAxeBuilder }) => {
    const sent: string[] = [];
    let announcements: Schemas["AdminAnnouncement"][] = [];
    await serveApi(page, ["manage_announcements"], (key, route) => {
      if (key === "GET /announcements") return json(route, { items: [] });
      if (key === "GET /admin/announcements") return json(route, { items: announcements });
      if (key === "POST /admin/announcements") {
        const body = route.request().postDataJSON() as Schemas["AdminAnnouncementCreateRequest"];
        sent.push(`create ${String(body.publish)}`);
        announcements = [
          {
            id: "0192f0c4-7a8e-7000-8000-0000000000f3",
            text: body.text,
            content: `<p>${body.text}</p>`,
            state: body.publish ? "published" : "draft",
            ...(body.ends_at === undefined ? {} : { ends_at: body.ends_at }),
            ...(body.publish ? { published_at: "2026-10-21T00:00:00Z" } : {}),
            created_at: "2026-10-21T00:00:00Z",
            updated_at: "2026-10-21T00:00:00Z",
          },
        ];
        return json(route, announcements[0]);
      }
      if (/^POST \/admin\/announcements\/[^/]+\/withdraw$/.test(key)) {
        sent.push("withdraw");
        announcements = announcements.map((item) => ({ ...item, state: "withdrawn" }));
        return json(route, announcements[0]);
      }
      return null;
    });
    await page.goto("/admin/announcements");
    const main = page.getByRole("main");
    const form = main.getByRole("region", { name: message("admin.announcements.new.title"), exact: true });
    await expect(form).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await form
      .getByRole("textbox", { name: message("admin.announcements.text"), exact: true })
      .fill("Maintenance tonight");
    await form.getByRole("button", { name: message("admin.announcements.create_published"), exact: true }).click();
    const list = main.getByRole("region", { name: message("admin.announcements.list.title"), exact: true });
    const withdraw = list.getByRole("button", {
      name: message("admin.announcements.withdraw_one", { announcement: "Maintenance tonight" }),
      exact: true,
    });
    await expect(withdraw).toBeVisible();
    expect(sent).toEqual(["create true"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await withdraw.click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("admin.announcements.withdraw"), exact: true }).click();
    await expect(withdraw).toBeHidden();
    expect(sent).toEqual(["create true", "withdraw"]);
    await expectNoAxeViolations(makeAxeBuilder);
  });
});
