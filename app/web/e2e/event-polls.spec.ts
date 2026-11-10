// SPDX-License-Identifier: AGPL-3.0-only
// The date polls and voice chat rooms of events in every width project (ADR-0161 §7, §10; web-client/screens/event.md,
// event-editor.md): an event whose date is not set shows its candidates as a table with row and column headers, takes
// an answer in a row, and lists who answered; its organizer sets the date from a candidate, which ends the poll; an
// event whose voice chat room opened at its start leads into the room; and the editor creates an event with candidate
// dates and a voice chat room. Every step is axe-clean and without horizontal scroll (the table scrolls inside its
// own group). `vite preview` has no API, so the spec answers the token refresh and the client API with `page.route`,
// as a signed-in user's server would. Names come from the project's catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { preferencesOf } from "./preferences.ts";

type Schemas = components["schemas"];

/** The client message `key` in the project's locale, its arguments left as they are. */
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
const bob: Schemas["Account"] = {
  ...alice,
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  display_name: "Bob",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000002",
  url: "https://my.example/@bob",
};

const day = 24 * 60 * 60_000;
const hour = 60 * 60_000;
const pollId = "0192f0c4-7a8e-7000-8000-0000000000e3";
const ownPollId = "0192f0c4-7a8e-7000-8000-0000000000e4";
const voiceId = "0192f0c4-7a8e-7000-8000-0000000000e5";
const roomId = "0192f0c4-7a8e-7000-8000-0000000000f1";

/** 12:00 UTC `days` from now, and two hours later. */
function at(days: number): { starts_at: string; ends_at: string } {
  const now = new Date(Date.now() + days * day);
  const start = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate(), 12);
  return { starts_at: new Date(start).toISOString(), ends_at: new Date(start + 2 * hour).toISOString() };
}

function eventOf(overrides: Partial<Schemas["Event"]>): Schemas["Event"] {
  return {
    id: pollId,
    organizer: bob,
    coorganizers: [],
    title: "Board games",
    description: "",
    visibility: "public",
    time_zone: "UTC",
    place_disclosure: "approved",
    capacity: 0,
    places: [],
    hidden_places: [],
    ...at(30),
    state: "unscheduled",
    going_count: 0,
    created_at: "2026-11-01T00:00:00Z",
    revised_at: "2026-11-01T00:00:00Z",
    revision_count: 1,
    viewer: { organizer: false, coorganizer: false, invited: false, approved: false },
    ...overrides,
  };
}

/** The two candidates of a poll: 30 and 31 days ahead. */
function candidatesOf(prefix: string): Schemas["EventCandidate"][] {
  return [
    { id: `${prefix}1`, ...at(30), yes_count: 2, maybe_count: 1, no_count: 0, confirmed: false },
    { id: `${prefix}2`, ...at(31), yes_count: 0, maybe_count: 0, no_count: 3, confirmed: false },
  ];
}

/**
 * Answers the client API as Alice's session in UTC with voice chat on: Bob polls the date of his board games, Alice
 * polls hers, and Bob's live show has started with its voice chat room open.
 */
async function serveApi(page: Page): Promise<void> {
  let events: Schemas["Event"][] = [
    eventOf({}),
    eventOf({ id: ownPollId, title: "Picnic", organizer: alice, viewer: { ...eventOf({}).viewer, organizer: true } }),
    eventOf({
      id: voiceId,
      title: "Live show",
      state: "scheduled",
      starts_at: new Date(Date.now() - 5 * 60_000).toISOString(),
      ends_at: new Date(Date.now() + hour).toISOString(),
      places: [{ kind: "voice", speakers: "followers", room_id: roomId }],
    }),
  ];
  const candidates = new Map<string, Schemas["EventCandidate"][]>([
    [pollId, candidatesOf("0192f0c4-7a8e-7000-8000-0000000000c")],
    [ownPollId, candidatesOf("0192f0c4-7a8e-7000-8000-0000000000d")],
  ]);
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({
      status: statusCode,
      contentType: statusCode < 400 ? "application/json" : "application/problem+json",
      body: JSON.stringify(body),
    });
  const notFound = (route: Route) => json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace("/api/v1", "");
    const method = request.method();
    if (path === "/accounts/verify_credentials") return json(route, { ...alice, permissions: [] });
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
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        voice_enabled: true,
        voice_room_starters: "everyone",
        rules: [],
      });
    }
    if (path === "/preferences") {
      return json(
        route,
        preferencesOf({
          locale: test.info().project.use.locale ?? "en",
          default_post_language: "en",
          time_zone: "UTC",
        }),
      );
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [] });
    if (path === "/voice/rooms") return json(route, { items: [] });
    if (path === "/events" && method === "GET") return json(route, { items: events } satisfies Schemas["EventPage"]);
    if (path === "/events" && method === "POST") {
      const body = request.postDataJSON() as Schemas["EventCreate"];
      const created = eventOf({
        id: "0192f0c4-7a8e-7000-8000-0000000000e6",
        title: body.title,
        organizer: alice,
        places: body.places ?? [],
        viewer: { organizer: true, coorganizer: false, invited: false, approved: false },
      });
      candidates.set(
        created.id,
        (body.candidates ?? []).map((candidate, index) => ({
          id: `0192f0c4-7a8e-7000-8000-0000000000b${String(index)}`,
          ...candidate,
          yes_count: 0,
          maybe_count: 0,
          no_count: 0,
          confirmed: false,
        })),
      );
      events = [...events, created];
      return json(route, created, 201);
    }
    if (/^\/events\/[^/]+\/(?:attendees|invitations)$/u.test(path)) return json(route, { items: [] });
    const votes = /^\/events\/([^/]+)\/candidates\/([^/]+)\/votes$/u.exec(path);
    if (votes !== null) {
      return json(route, { items: [{ account: bob, vote: "yes" }] } satisfies Schemas["EventBallotPage"]);
    }
    const vote = /^\/events\/([^/]+)\/candidates\/([^/]+)\/vote$/u.exec(path);
    if (vote !== null && method === "PUT") {
      const { vote: answer } = request.postDataJSON() as Schemas["EventVoteUpdate"];
      const list = (candidates.get(vote[1] ?? "") ?? []).map((candidate) =>
        candidate.id === vote[2]
          ? { ...candidate, vote: answer, yes_count: candidate.yes_count + (answer === "yes" ? 1 : 0) }
          : candidate,
      );
      candidates.set(vote[1] ?? "", list);
      const answered = list.find((candidate) => candidate.id === vote[2]);
      return answered === undefined ? notFound(route) : json(route, answered);
    }
    const confirm = /^\/events\/([^/]+)\/candidates\/([^/]+)\/confirm$/u.exec(path);
    if (confirm !== null && method === "POST") {
      const candidate = (candidates.get(confirm[1] ?? "") ?? []).find((c) => c.id === confirm[2]);
      if (candidate === undefined) return notFound(route);
      events = events.map((event) =>
        event.id === confirm[1]
          ? { ...event, state: "scheduled", starts_at: candidate.starts_at, ends_at: candidate.ends_at }
          : event,
      );
      return json(
        route,
        events.find((event) => event.id === confirm[1]),
      );
    }
    const list = /^\/events\/([^/]+)\/candidates$/u.exec(path);
    if (list !== null && method === "GET") {
      return json(route, { items: candidates.get(list[1] ?? "") ?? [] } satisfies Schemas["EventCandidateList"]);
    }
    const one = /^\/events\/([^/]+)$/u.exec(path);
    if (one !== null) {
      const event = events.find((item) => item.id === one[1]);
      return event === undefined ? notFound(route) : json(route, event);
    }
    return notFound(route);
  });
}

test.describe("the date polls and voice chat rooms of events", () => {
  test("a poll is a table that takes an answer and lists who answered", async ({ page, makeAxeBuilder }) => {
    await serveApi(page);
    await page.goto(`/events/${pollId}`);
    await expect(page.getByRole("heading", { level: 1, name: "Board games" })).toBeVisible();
    const main = page.getByRole("main");
    const poll = main.getByRole("region", { name: message("events.poll.title") });
    const table = poll.getByRole("table", { name: message("events.poll.caption") });
    await expect(
      table.getByRole("columnheader", { name: message("events.poll.count.yes"), exact: true }),
    ).toBeVisible();
    await expect(table.getByRole("rowheader")).toHaveCount(2);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The first candidate's row: its counts, then the user's answer, pressed by its visible label.
    const first = table.getByRole("row").nth(1);
    await expect(first.getByRole("cell").first()).toHaveText("2");
    const yes = first.getByRole("radio", { name: message("events.poll.vote.yes"), exact: true });
    await first.getByText(message("events.poll.vote.yes"), { exact: true }).click();
    await expect(yes).toBeChecked();
    await expect(first.getByRole("cell").first()).toHaveText("3");

    await first.getByRole("button", { name: message("events.poll.ballots"), exact: false }).click();
    const dialog = page.getByRole("dialog");
    const people = dialog.getByRole("list", { name: message("events.poll.ballots") });
    await expect(people.getByText("Bob", { exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("events.poll.close"), exact: true }).click();
    await expect(dialog).toBeHidden();
  });

  test("the organizer sets the date from a candidate", async ({ page, makeAxeBuilder }) => {
    await serveApi(page);
    await page.goto(`/events/${ownPollId}`);
    await expect(page.getByRole("heading", { level: 1, name: "Picnic" })).toBeVisible();
    const main = page.getByRole("main");
    const poll = main.getByRole("region", { name: message("events.poll.title") });
    await expect(poll.getByRole("group", { name: message("events.poll.add_legend") })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    const first = poll.getByRole("table").getByRole("row").nth(1);
    await first.getByRole("button", { name: message("events.poll.confirm"), exact: false }).click();
    const dialog = page.getByRole("alertdialog");
    await dialog.getByRole("button", { name: message("events.poll.confirm.confirm"), exact: true }).click();
    await expect(poll).toBeHidden();
    await expect(main.getByText(message("events.state.unscheduled"), { exact: true })).toBeHidden();
    // The date is set: the user's answer to the event opens.
    const answer = main.getByRole("radiogroup", { name: message("events.answer.title") });
    await expect(answer.getByRole("radio", { name: message("events.answer.going"), exact: true })).toBeEnabled();
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("an event whose room opened at its start leads into the room", async ({ page, makeAxeBuilder }) => {
    await serveApi(page);
    await page.goto(`/events/${voiceId}`);
    await expect(page.getByRole("heading", { level: 1, name: "Live show" })).toBeVisible();
    const where = page.getByRole("main").getByRole("region", { name: message("events.place.title") });
    const join = where.getByRole("link", { name: message("events.place.voice.join"), exact: true });
    await expect(join).toHaveAttribute("href", `/voice/${roomId}`);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await join.click();
    await expect(page).toHaveURL(new RegExp(`/voice/${roomId}$`, "u"));
  });

  test("the editor creates an event with candidate dates and a voice chat room", async ({ page, makeAxeBuilder }) => {
    await serveApi(page);
    await page.goto("/events/new");
    await expect(page.getByRole("heading", { level: 1, name: message("events.form.new_title") })).toBeVisible();
    const main = page.getByRole("main");
    await main.getByRole("textbox", { name: message("events.form.title"), exact: false }).fill("Karaoke");
    await main.getByText(message("events.form.date_mode.poll"), { exact: true }).click();
    await expect(main.getByRole("radio", { name: message("events.form.date_mode.poll"), exact: true })).toBeChecked();
    for (const [index, days] of [10, 11].entries()) {
      const date = new Date(Date.now() + days * day).toISOString().slice(0, 10);
      const group = main.getByRole("group", {
        name: message("events.form.candidate").replace("{number}", String(index + 1)),
      });
      await group.getByLabel(message("events.form.starts_at"), { exact: false }).fill(`${date}T18:00`);
      await group.getByLabel(message("events.form.ends_at"), { exact: false }).fill(`${date}T21:00`);
    }
    await main.getByText(message("events.form.voice"), { exact: true }).click();
    await expect(
      main.getByRole("radiogroup", { name: message("events.form.voice.speakers"), exact: true }),
    ).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await main.getByRole("button", { name: message("events.form.submit_new"), exact: true }).click();
    await expect(page.getByRole("heading", { level: 1, name: "Karaoke" })).toBeVisible();
    const table = main.getByRole("region", { name: message("events.poll.title") }).getByRole("table");
    await expect(table.getByRole("rowheader")).toHaveCount(2);
    await expectNoAxeViolations(makeAxeBuilder);
  });
});
