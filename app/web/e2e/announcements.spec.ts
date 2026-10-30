// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements in every width × locale project (FR-726, ADR-0125, ADR-0027): below xl a user sees the
// published ones above the home timeline with the unread one marked in words, and marks it read; from xl they are in
// the third column by their titles instead; the list page has the ended ones and opens one from its notification,
// marking it read; an administrator writes an announcement with its title and withdraws it after its dialog. Each step passes axe without scrolling sideways. `vite preview`
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
  title: "Scheduled maintenance tonight",
  content: `<p>Scheduled maintenance of ${"the-database-and-the-media-storage-".repeat(3)}tonight. See <a href="https://status.example/">the status page</a>.</p>`,
  published_at: "2026-10-21T00:00:00Z",
  ends_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-21T00:00:00Z",
  read: false,
  ended: false,
};
/** An announcement whose window has ended, which only the list page has. */
const ended: Schemas["Announcement"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000f4",
  title: "Rules updated",
  content: "<p>We updated the rules.</p>",
  published_at: "2026-09-01T00:00:00Z",
  ends_at: "2026-09-08T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
  read: false,
  ended: true,
};
const read: Schemas["Announcement"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000f2",
  title: "مرحبا",
  content: "<p>مرحبا بكم في الخادم</p>",
  published_at: "2026-10-20T00:00:00Z",
  updated_at: "2026-10-20T00:00:00Z",
  read: true,
  ended: false,
};

/** Whether the project's viewport has the third column (xl, 80rem). */
function hasThirdColumn(): boolean {
  return (test.info().project.use.viewport?.width ?? 0) >= 1280;
}

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
    test.skip(hasThirdColumn(), "from xl the announcements are in the third column, not above the home timeline");
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
    // The last unread one read: the region goes, and focus moves to the page's heading.
    await expect(region).toBeHidden();
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("shows the titles in the third column from xl, and not above the home timeline", async ({
    page,
    makeAxeBuilder,
  }) => {
    test.skip(!hasThirdColumn(), "below xl there is no third column");
    await serveApi(page, [], (key, route) => {
      if (key === "GET /announcements") return json(route, { items: [unread, read] });
      if (key === "GET /announcements/history") return json(route, { items: [unread, read, ended] });
      if (key === `POST /announcements/${unread.id}/read`) return json(route, { ...unread, read: true });
      return null;
    });
    await page.goto("/home");
    const panel = page.getByRole("complementary", { name: new RegExp(`^${message("announcements.title")}`, "u") });
    const title = panel.getByRole("link", { name: new RegExp(`^${unread.title}`, "u") });
    await expect(title).toBeVisible();
    await expect(panel.getByRole("link", { name: read.title })).toBeVisible();
    await expect(page.getByRole("main").getByRole("region", { name: message("announcements.title") })).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);

    // The title opens the announcement on the list page, which marks it read; the panel is not on that page.
    await title.click();
    const heading = page.getByRole("main").getByRole("heading", { level: 2, name: unread.title, exact: true });
    await expect(heading).toBeFocused();
    await expect(panel).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("reads the ended announcements on the list page and opens one from its notification", async ({
    page,
    makeAxeBuilder,
  }) => {
    const reads: string[] = [];
    await serveApi(page, [], (key, route) => {
      if (key === "GET /announcements") return json(route, { items: [unread, read] });
      if (key === "GET /announcements/history") {
        return json(route, { items: [{ ...unread, read: reads.includes(unread.id) }, read, ended] });
      }
      if (key === `POST /announcements/${unread.id}/read`) {
        reads.push(unread.id);
        return json(route, { ...unread, read: true });
      }
      return null;
    });
    await page.goto("/announcements");
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 2, name: new RegExp(`^${ended.title}`, "u") })).toBeVisible();
    await expect(main.getByText(message("announcements.ended"), { exact: true })).toBeVisible();
    await expect(main.getByRole("article")).toHaveCount(3);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // From its notification (or its Web Push): the page focuses it and marks it read.
    await page.goto(`/announcements?id=${unread.id}`);
    await expect(main.getByRole("heading", { level: 2, name: unread.title, exact: true })).toBeFocused();
    expect(reads).toEqual([unread.id]);
    await expect(main.getByRole("button", { name: message("announcements.mark_read"), exact: true })).toHaveCount(0);
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
        sent.push(`create ${String(body.publish)} ${body.title}`);
        announcements = [
          {
            id: "0192f0c4-7a8e-7000-8000-0000000000f3",
            title: body.title,
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
      .getByRole("textbox", { name: message("admin.announcements.announcement_title"), exact: true })
      .fill("Maintenance tonight");
    await form
      .getByRole("textbox", { name: message("admin.announcements.text"), exact: true })
      .fill("The server is read-only from 02:00.");
    await form.getByRole("button", { name: message("admin.announcements.create_published"), exact: true }).click();
    const list = main.getByRole("region", { name: message("admin.announcements.list.title"), exact: true });
    const withdraw = list.getByRole("button", {
      name: message("admin.announcements.withdraw_one", { announcement: "Maintenance tonight" }),
      exact: true,
    });
    await expect(withdraw).toBeVisible();
    expect(sent).toEqual(["create true Maintenance tonight"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await withdraw.click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("admin.announcements.withdraw"), exact: true }).click();
    await expect(withdraw).toBeHidden();
    expect(sent).toEqual(["create true Maintenance tonight", "withdraw"]);
    await expectNoAxeViolations(makeAxeBuilder);
  });
});
