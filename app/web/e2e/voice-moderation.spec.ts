// SPDX-License-Identifier: AGPL-3.0-only
// Moderating voice chat in every width × locale project (ADR-0100 §2, §11, §13, ADR-0027): reporting a room from its
// page, the voice chat section of the server settings, and a report's page of a room with its records and the
// confirmation that ends it, each axe-clean and without horizontal scroll. `vite preview` has no API, so the spec
// answers the token refresh and the client API with `page.route`, as a server with voice chat on would, for Alice,
// who holds manage_settings and manage_reports. Names come from the project's catalog, so the spec finds the same
// elements in every locale.
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
const carol: Schemas["Account"] = { ...alice, id: "0192f0c4-7a8e-7000-8000-000000000003", username: "carol", acct: "carol", display_name: "Carol", url: "https://my.example/@carol" };

/** Alice's own account, with the permissions of her role. */
const credentials = {
  ...alice,
  discoverable: true,
  indexable: true,
  ai_opt_out: false,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: ["manage_settings", "manage_reports"],
};

const roomId = "0192f0c4-7a8e-7000-8000-0000000000f1";
const roomName = "Friday night radio with a name long enough to wrap on a phone";
const room: Schemas["VoiceRoom"] = {
  id: roomId,
  name: roomName,
  description: "Talking about the week's music.",
  hashtags: ["music"],
  visibility: "local",
  speakers: "manual",
  host: bob,
  opened_at: "2026-10-26T12:00:00Z",
  participants_count: 1,
  participants: [{ account: bob, role: "speaker", host: true, cohost: false, hand_raised: false, blocked: false }],
};

const reportId = "0192f0c4-7a8e-7000-8000-0000000c0009";

/** The report of Bob's room as the moderators' API answers it; ended once a moderator ends the room. */
function roomReport(ended: boolean): Schemas["AdminReportDetail"] {
  const revision: Schemas["AdminVoiceRoomRevision"] = {
    id: "0192f0c4-7a8e-7000-8000-0000000a0001",
    name: roomName,
    description: "Talking about the week's music, and a very long description that goes on to wrap on a narrow screen.",
    hashtags: ["music"],
    visibility: "local",
    speakers: "manual",
    revised_by_account_id: bob.id,
    revised_at: "2026-10-26T12:00:00Z",
  };
  return {
    id: reportId,
    category: "spam",
    comment: "Advertising all evening.",
    forwarded: false,
    filed_at: "2026-10-26T12:30:00Z",
    state: "open",
    state_changed_at: "2026-10-26T12:30:00Z",
    reporter_account_id: carol.id,
    reporter_account: carol,
    target_account_id: bob.id,
    target_account: bob,
    status_ids: [],
    rule_ids: [],
    statuses: [],
    notes: [],
    target_history: { reports_count: 1, actions_count: 0 },
    voice_room_id: roomId,
    voice_room: {
      id: roomId,
      reported_revision_id: revision.id,
      opened_at: "2026-10-26T12:00:00Z",
      ...(ended ? { ended_at: "2026-10-26T13:00:00Z", end_reason: "moderator", ended_by_account_id: alice.id } : {}),
      revisions: [revision],
      participations: [
        { account_id: bob.id, joined_at: "2026-10-26T12:00:00Z", ...(ended ? { left_at: "2026-10-26T13:00:00Z", leave_reason: "room_ended" } : {}) },
        { account_id: carol.id, joined_at: "2026-10-26T12:10:00Z", left_at: "2026-10-26T12:20:00Z", leave_reason: "removed" },
      ],
      bans: [{ account_id: carol.id, banned_by_account_id: bob.id, banned_at: "2026-10-26T12:20:00Z" }],
    },
  };
}

/** The voice chat settings at their defaults, as GET /admin/settings lists them. */
const voiceSettings = {
  items: [
    { key: "voice_room_starters", kind: "enum", default: "permitted", choices: ["everyone", "permitted"], value: "permitted", stored: false },
    { key: "voice_public_summary", kind: "bool", default: "false", value: "false", stored: false },
    { key: "retention_voice_participants", kind: "int", default: "90", minimum: 30, maximum: 3650, value: "90", stored: false },
  ],
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: status < 400 ? "application/json" : "application/problem+json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/** Answers the client API as Alice's session on a server with voice chat on; returns the bodies of the reports and the room ends. */
async function serveApi(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const sent = { reports: [] as unknown[], ends: [] as unknown[] };
  let ended = false;
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow admin" }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0-e2e",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
        voice_enabled: true,
        voice_room_starters: "permitted",
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, credentials);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "en", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (key === "GET /voice/rooms") return json(route, { items: ended ? [] : [room] });
    if (key === `GET /voice/rooms/${roomId}`) return json(route, room);
    if (key === `GET /accounts/${bob.id}`) return json(route, bob);
    if (key === `GET /accounts/${carol.id}`) return json(route, carol);
    if (key === `GET /accounts/${alice.id}`) return json(route, alice);
    if (key === "POST /reports") {
      const body = request.postDataJSON() as Schemas["CreateReportRequest"];
      sent.reports.push(body);
      return json(route, { id: reportId, category: body.category, comment: body.comment ?? "", forwarded: false, filed_at: "2026-10-26T12:30:00Z", target_account: bob, status_ids: [], rule_ids: [], voice_room_id: roomId, state: "open" });
    }
    if (key === `GET /admin/reports/${reportId}`) return json(route, roomReport(ended));
    if (key === `POST /admin/voice_rooms/${roomId}/end`) {
      sent.ends.push(request.postDataJSON());
      ended = true;
      return route.fulfill({ status: 204 });
    }
    if (key === "GET /admin/instance_texts") return json(route, { items: [{ key: "name", body: "my example", revised_at: "2026-10-16T00:00:00Z" }] });
    if (key === "GET /admin/settings") return json(route, voiceSettings);
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("voice chat moderation", () => {
  test("reports a room from its page, the dialog axe-clean and without horizontal scroll", async ({ page, makeAxeBuilder }) => {
    const sent = await serveApi(page);
    await page.goto(`/voice/${roomId}`);
    await expect(page.getByRole("heading", { level: 1, name: roomName })).toBeVisible();
    await page.getByRole("button", { name: message("voice.room.report") }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("heading", { level: 3, name: message("report.room.category.heading") })).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await dialog.getByRole("radio", { name: message("report.category.spam") }).check();
    await dialog.getByRole("button", { name: message("report.next") }).click();
    await expect(dialog.getByRole("heading", { level: 3, name: message("report.comment.heading") })).toBeFocused();
    await dialog.getByRole("button", { name: message("report.submit") }).click();
    await expect(dialog.getByRole("heading", { level: 3, name: message("report.sent.heading") })).toBeFocused();
    expect(sent.reports).toEqual([{ account_id: bob.id, category: "spam", voice_room_id: roomId, forward: false }]);
  });

  test("shows the voice chat settings, axe-clean and without horizontal scroll", async ({ page, makeAxeBuilder }) => {
    await serveApi(page);
    await page.goto("/admin/settings");
    const section = page.getByRole("region", { name: message("admin.settings.voice.title") });
    await expect(section.getByRole("radiogroup", { name: message("admin.settings.voice.starters") })).toBeVisible();
    await expect(section.getByRole("textbox", { name: message("admin.settings.voice.retention") })).toHaveValue("90");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("shows a room's report with its records and ends the room after its confirmation", async ({ page, makeAxeBuilder }) => {
    const sent = await serveApi(page);
    await page.goto(`/moderation/reports/${reportId}`);
    const section = page.getByRole("region", { name: message("admin.report.voice.title") });
    await expect(section.getByRole("region", { name: message("admin.report.voice.participations") })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await section.getByRole("button", { name: message("admin.report.voice.end") }).click();
    const confirmation = page.getByRole("alertdialog", { name: message("admin.report.voice.end.confirm.title") });
    await expect(confirmation.getByRole("textbox", { name: message("admin.report.voice.end.note_field") })).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await confirmation.getByRole("textbox", { name: message("admin.report.voice.end.note_field") }).fill("Advertising");
    await confirmation.getByRole("button", { name: message("admin.report.voice.end") }).click();

    await expect(section.getByText(message("admin.report.voice.ended"))).toBeVisible();
    await expect(section.getByRole("heading", { level: 2, name: message("admin.report.voice.title") })).toBeFocused();
    await expect(section.getByRole("button", { name: message("admin.report.voice.end") })).toHaveCount(0);
    expect(sent.ends).toEqual([{ reason: "Advertising" }]);
  });
});
