// SPDX-License-Identifier: AGPL-3.0-only
// The voice chat pages before the call in every width × locale project (ADR-0100 §8, §9, ADR-0027): the list of the
// open rooms opened from the navigation (the rail from md, the menu's sheet below it), the dialog that starts a room,
// and a room's page with its participants and the notice before joining, all axe-clean and without horizontal scroll;
// and the room's page of a visitor who is not signed in. `vite preview` has no API, so the spec answers the token
// refresh and the client API with `page.route`, as a server with voice chat on would. Names come from the project's
// catalog, so the spec finds the same elements in every locale.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, openFromNavigation, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

const alice: Schemas["Account"] = {
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
const bob: Schemas["Account"] = { ...alice, id: "0192f0c4-7a8e-7000-8000-000000000002", username: "bob", acct: "bob", display_name: "Bob", url: "https://my.example/@bob" };

/** Alice as the server knows her: a member who may open rooms. */
const credentials: Schemas["CredentialAccount"] = {
  ...alice,
  discoverable: true,
  indexable: true,
  ai_opt_out: false,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: ["start_voice_rooms"],
};

/** The instance with voice chat on, where only the users with start_voice_rooms open rooms. */
const instance: Schemas["Instance"] = {
  domain: "my.example",
  name: "my example",
  version: "0.0.0-e2e",
  source_url: "https://github.com/mycroblog/my",
  registrations: { mode: "open" },
  limits: { max_post_characters: 500, require_media_description: false },
  timelines: { local: "everyone", federated: "everyone" },
  federation: { mode: "both" },
  sends_email: false,
  search_indexing: "deny",
  ai_usage: "deny",
  finance_visibility: "private",
  income_visibility: "private",
  supporters_enabled: false,
  supporter_list_visibility: "off",
  voice_enabled: true,
  voice_room_starters: "permitted",
  rules: [],
};

const roomId = "0192f0c4-7a8e-7000-8000-0000000000f1";
const room: Schemas["VoiceRoom"] = {
  id: roomId,
  name: "Friday night radio with a name long enough to wrap on a phone",
  description: "Talking about the week's music.\nRequests welcome.",
  hashtags: ["music", "radio"],
  visibility: "local",
  speakers: "manual",
  host: bob,
  opened_at: "2026-10-26T12:00:00Z",
  participants_count: 2,
  participants: [
    { account: bob, role: "speaker", host: true, cohost: false, hand_raised: false, blocked: false },
    { account: { ...alice, id: "0192f0c4-7a8e-7000-8000-000000000003", username: "carol", acct: "carol", display_name: "Carol" }, role: "listener", host: false, cohost: false, hand_raised: false, blocked: false },
  ],
};

const notFound: Schemas["Problem"] = { type: "about:blank", title: "Not Found", status: 404 };

/** Answers the client API as Alice's session on a server with voice chat on and one open room; signed out without `signedIn`. */
async function serveApi(page: Page, { signedIn }: { signedIn: boolean }): Promise<void> {
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({ status: statusCode, contentType: statusCode < 400 ? "application/json" : "application/problem+json", body: JSON.stringify(body) });
  await page.route("**/oauth/token/refresh", (route) =>
    signedIn ? json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }) : json(route, { error: "invalid_grant" }, 400),
  );
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname.replace("/api/v1", "");
    if (path === "/instance") return json(route, instance);
    if (path === `/voice/rooms/${roomId}/summary`) return json(route, { id: roomId, state: "open", name: room.name, description: room.description, host: bob });
    if (!signedIn) return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    if (path === "/accounts/verify_credentials") return json(route, credentials);
    if (path === "/preferences") {
      // The user's locale is the project's: the app displays itself in the signed-in user's locale.
      return json(route, { locale: test.info().project.use.locale ?? "en", time_zone: "UTC", default_visibility: "public", default_post_language: "en", require_media_description: false, timeline_languages: [] });
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [] });
    if (path === "/voice/rooms") return json(route, { items: [room] });
    if (path === `/voice/rooms/${roomId}`) return json(route, room);
    return json(route, notFound, 404);
  });
}

test.describe("the voice chat pages", () => {
  test.beforeEach(async ({ page, hasTouch }) => {
    await serveApi(page, { signedIn: true });
    await page.goto("/home");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await openFromNavigation(page, "/voice", hasTouch);
    await expect(page.getByRole("heading", { level: 1, name: message("voice.title") })).toBeFocused();
  });

  test("lists the open rooms, axe-clean and without horizontal scroll", async ({ page, makeAxeBuilder }) => {
    const list = page.getByRole("list", { name: message("voice.list.label") });
    await expect(list.getByRole("heading", { level: 2, name: room.name })).toBeVisible();
    await expect(list.getByText(room.hashtags.map((tag) => `#${tag}`)[0] ?? "")).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("opens the dialog that starts a room, axe-clean and without horizontal scroll", async ({ page, makeAxeBuilder }) => {
    await page.getByRole("button", { name: message("voice.start") }).click();
    const dialog = page.getByRole("dialog", { name: message("voice.form.new_title") });
    await expect(dialog.getByRole("textbox", { name: message("voice.form.name") })).toBeFocused();
    await expect(dialog.getByRole("switch", { name: message("voice.form.post") })).toBeChecked();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("opens a room's page with its participants and the notice before joining", async ({ page, makeAxeBuilder }) => {
    await page.getByRole("link", { name: room.name }).first().click();
    await expect(page.getByRole("heading", { level: 1, name: room.name })).toBeFocused();
    await expect(page.getByText(message("voice.room.listed_notice"))).toBeVisible();
    await expect(page.getByRole("button", { name: message("voice.room.join") })).toBeVisible();
    await expect(page.getByRole("region", { name: message("voice.room.participants") }).getByRole("listitem")).toHaveCount(2);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("a room's page signed out", () => {
  test("says that only this server's users join, with the way to sign in", async ({ page, makeAxeBuilder }) => {
    await serveApi(page, { signedIn: false });
    await page.goto(`/voice/${roomId}`);
    await expect(page.getByRole("heading", { level: 1, name: room.name })).toBeVisible();
    await expect(page.getByText(message("voice.room.local_only"))).toBeVisible();
    await expect(page.getByRole("main").getByRole("link", { name: message("voice.room.sign_in") })).toBeVisible();
    await expect(page.getByRole("button", { name: message("voice.room.join") })).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
