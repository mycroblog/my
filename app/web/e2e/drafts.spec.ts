// SPDX-License-Identifier: AGPL-3.0-only
// The drafts saved on the server and the scheduled posts (web-client/screens/drafts.md, screens/compose.md) in every
// width × locale project (ADR-0110): a draft saved in the new-post form is listed on the drafts page, which opens it
// again in a tab that kept nothing of it (as another device would); the page and the schedule popover are axe-clean and do not scroll sideways.
// `vite preview` has no API, so the spec answers the token refresh and the client API with `page.route`, keeping the
// drafts in memory as the server would. Names come from the project's catalog, so the spec finds the same elements in
// every locale.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, newPostButton, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { preferencesOf } from "./preferences.ts";

type Draft = components["schemas"]["Draft"];
type DraftRequest = components["schemas"]["DraftRequest"];

/** The client message `key` in the project's locale, with its `{name}` arguments replaced. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text.replaceAll(/\{(\w+)\}/g, (_, name: string) => values[name] ?? `{${name}}`);
}

const alice = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
};

/** A draft of `content`, as the server answers it. */
function draftOf(id: string, content: DraftRequest, scheduledAt?: string): Draft {
  const now = new Date().toISOString();
  return {
    id,
    created_at: now,
    updated_at: now,
    text: content.text ?? "",
    spoiler_text: content.spoiler_text ?? "",
    sensitive: content.sensitive ?? false,
    visibility: content.visibility,
    media: content.media ?? [],
    ...(content.language === undefined ? {} : { language: content.language }),
    ...(scheduledAt === undefined ? {} : { scheduled_at: scheduledAt }),
  };
}

/** Answers the client API as Alice's session, with the drafts of `initial` kept in memory; returns them. */
async function serveApi(page: Page, initial: Draft[] = []): Promise<Map<string, Draft>> {
  const drafts = new Map(initial.map((d) => [d.id, d]));
  let next = 0;
  const json = (route: Route, body: unknown, statusCode = 200, headers: Record<string, string> = {}) =>
    route.fulfill({
      status: statusCode,
      contentType: statusCode < 400 ? "application/json" : "application/problem+json",
      headers,
      body: JSON.stringify(body),
    });
  const draftAnswer = (route: Route, draft: Draft, statusCode = 200) =>
    json(route, draft, statusCode, { ETag: `"${draft.updated_at}"` });
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace("/api/v1", "");
    const method = request.method();
    if (path === "/accounts/verify_credentials") return json(route, { ...alice, statuses_count: 3, permissions: [] });
    if (path === "/instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0-e2e",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        full_text_search: { available: false, visibilities: [] },
        translation: { browser: false, providers: [] },
        limits: {
          max_post_characters: 500,
          require_media_description: false,
          drafts: {
            max_drafts: 100,
            max_scheduled_posts: 300,
            max_scheduled_posts_per_day: 25,
            scheduled_post_min_lead_minutes: 5,
            scheduled_post_max_lead_days: 366,
          },
        },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (path === "/preferences") {
      // The user's locale is the project's: the app displays itself in the signed-in user's locale.
      return json(
        route,
        preferencesOf({ locale: test.info().project.use.locale ?? "en", default_post_language: "en" }),
      );
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [] });
    if (path === "/custom_emojis") return json(route, { items: [] });
    if (path === "/drafts" && method === "GET") {
      const scheduled = url.searchParams.get("scheduled") === "true";
      return json(route, { items: [...drafts.values()].filter((d) => (d.scheduled_at !== undefined) === scheduled) });
    }
    if (path === "/drafts" && method === "POST") {
      next += 1;
      const draft = draftOf(
        `0192f0c4-7a8e-7000-8000-0000000004${String(next).padStart(2, "0")}`,
        request.postDataJSON() as DraftRequest,
      );
      drafts.set(draft.id, draft);
      return draftAnswer(route, draft, 201);
    }
    const one = /^\/drafts\/([^/]+)$/.exec(path);
    const found = one === null ? undefined : drafts.get(one[1] ?? "");
    if (one !== null && found !== undefined) {
      if (method === "GET") return draftAnswer(route, found);
      if (method === "PUT") {
        const draft = draftOf(found.id, request.postDataJSON() as DraftRequest, found.scheduled_at);
        drafts.set(draft.id, draft);
        return draftAnswer(route, draft);
      }
      if (method === "DELETE") {
        drafts.delete(found.id);
        return route.fulfill({ status: 204 });
      }
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return drafts;
}

/** Opens the new-post form from the navigation's button and returns the form. */
async function openForm(page: Page, hasTouch: boolean) {
  const open = newPostButton(page, message("compose.open"));
  if (hasTouch) await open.tap();
  else await open.click();
  const form = page.getByRole("form", { name: message("compose.title") });
  await expect(form.getByRole("textbox", { name: message("compose.text.label") })).toBeVisible();
  return form;
}

test.describe("the drafts", () => {
  test("a draft saved in the form opens from the drafts page with nothing left in the tab", async ({
    page,
    hasTouch,
  }) => {
    const drafts = await serveApi(page);
    await page.goto("/home");
    const form = await openForm(page, hasTouch);
    await form.getByRole("textbox", { name: message("compose.text.label") }).fill("Notes for the meetup");
    await form.getByRole("button", { name: message("drafts.save") }).click();
    await expect(form.getByRole("textbox", { name: message("compose.text.label") })).toHaveValue("");
    expect([...drafts.values()].map((d) => d.text)).toEqual(["Notes for the meetup"]);

    // As on another device: the tab keeps nothing of the form, and the draft comes from the server alone.
    await page.evaluate(() => {
      sessionStorage.clear();
    });
    await page.goto("/drafts");
    await expect(page.getByRole("heading", { level: 1, name: message("drafts.title") })).toBeVisible();
    await page
      .getByRole("list", { name: message("drafts.unscheduled.heading") })
      .getByRole("button", {
        name: message("drafts.action", { action: message("drafts.edit"), excerpt: "Notes for the meetup" }),
      })
      .click();
    const sheet = page.getByRole("form", { name: message("compose.title") });
    await expect(sheet.getByRole("textbox", { name: message("compose.text.label") })).toHaveValue(
      "Notes for the meetup",
    );
    await expect(sheet.getByText(message("drafts.editing"))).toBeVisible();
  });

  test("the drafts page and the schedule popover are axe-clean and do not scroll sideways", async ({
    page,
    makeAxeBuilder,
  }) => {
    const later = new Date(Date.now() + 2 * 24 * 60 * 60_000).toISOString();
    await serveApi(page, [
      draftOf(
        "0192f0c4-7a8e-7000-8000-0000000004a1",
        { text: "The market opens at nine.", visibility: "public" },
        later,
      ),
      draftOf("0192f0c4-7a8e-7000-8000-0000000004a2", {
        text: "A long thought about https://example.com/a/long/path/that/wraps/at/320/px",
        visibility: "public",
      }),
    ]);
    await page.goto("/drafts");
    await expect(page.getByRole("heading", { level: 1, name: message("drafts.title") })).toBeVisible();
    await expect(
      page.getByRole("list", { name: message("drafts.scheduled.heading") }).getByRole("listitem"),
    ).toHaveCount(1);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await page
      .getByRole("button", {
        name: message("drafts.action", { action: message("drafts.reschedule"), excerpt: "The market opens at nine." }),
      })
      .click();
    await expect(page.getByRole("dialog", { name: message("drafts.schedule.label") })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
