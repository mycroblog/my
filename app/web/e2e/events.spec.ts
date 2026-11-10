// SPDX-License-Identifier: AGPL-3.0-only
// The events in every width project (ADR-0161; web-client/screens/events.md, event.md, event-editor.md): the list
// opened from the navigation (the rail from md, the menu's sheet below it), an event's page with its time in the
// viewer's time zone and in Tokyo's, a place shown only to approved attendees named by its kind, an online link that
// says it leaves the site, the user's answer, and the editor that creates an event, axe-clean and without horizontal
// scroll at every step. `vite preview` has no API, so the spec answers the token refresh and the client API with
// `page.route`, as a signed-in user's server would. Names come from the project's catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import {
  escapeRegExp,
  expect,
  expectNoAxeViolations,
  expectNoHorizontalScroll,
  openFromNavigation,
  test,
} from "./fixtures";
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

/** 12:00 UTC (21:00 in Tokyo) a month from now, and two hours later. */
function eventTimes(): { starts_at: string; ends_at: string } {
  const now = new Date(Date.now() + 30 * day);
  const start = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate(), 12);
  return { starts_at: new Date(start).toISOString(), ends_at: new Date(start + 2 * 60 * 60_000).toISOString() };
}

/** Bob's picnic in Tokyo for every local user: an online link that Alice sees and a place in person that she does not. */
function picnic(): Schemas["Event"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000000e1",
    organizer: bob,
    coorganizers: [],
    title: "Night picnic",
    description: "Bring a blanket.",
    visibility: "public",
    time_zone: "Asia/Tokyo",
    place_disclosure: "approved",
    capacity: 0,
    places: [{ kind: "url", name: "Live stream", url: "https://stream.example/live" }],
    hidden_places: ["physical"],
    ...eventTimes(),
    state: "scheduled",
    going_count: 2,
    created_at: "2026-11-01T00:00:00Z",
    revised_at: "2026-11-01T00:00:00Z",
    revision_count: 1,
    viewer: { organizer: false, coorganizer: false, invited: false, approved: false },
  };
}

/** Answers the client API as Alice's session in UTC: Bob's picnic is coming up, and the events change with the requests. */
async function serveApi(page: Page): Promise<void> {
  let events: Schemas["Event"][] = [picnic()];
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
        rules: [],
      });
    }
    if (path === "/preferences") {
      // The user's locale is the project's, and the user's time zone UTC, which is not the event's.
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
    if (path === "/events" && method === "GET") return json(route, { items: events } satisfies Schemas["EventPage"]);
    if (path === "/events" && method === "POST") {
      const body = request.postDataJSON() as Schemas["EventCreate"];
      const created: Schemas["Event"] = {
        ...picnic(),
        id: "0192f0c4-7a8e-7000-8000-0000000000e2",
        organizer: alice,
        title: body.title,
        description: body.description ?? "",
        time_zone: body.time_zone ?? "UTC",
        places: body.places ?? [],
        hidden_places: [],
        starts_at: body.starts_at ?? "",
        ends_at: body.ends_at ?? "",
        going_count: 0,
        viewer: { organizer: true, coorganizer: false, invited: false, approved: false },
      };
      events = [...events, created];
      return json(route, created, 201);
    }
    const attendees = /^\/events\/([^/]+)\/attendees$/u.exec(path);
    if (attendees !== null) return json(route, { items: [] } satisfies Schemas["EventAttendeePage"]);
    const invitations = /^\/events\/([^/]+)\/invitations$/u.exec(path);
    if (invitations !== null) return json(route, { items: [] } satisfies Schemas["EventInvitationPage"]);
    const rsvp = /^\/events\/([^/]+)\/rsvp$/u.exec(path);
    if (rsvp !== null) {
      const { answer } = request.postDataJSON() as Schemas["EventRSVP"];
      events = events.map((event) =>
        event.id === rsvp[1]
          ? { ...event, viewer: { ...event.viewer, answer }, going_count: event.going_count + 1 }
          : event,
      );
      const answered = events.find((event) => event.id === rsvp[1]);
      return answered === undefined ? notFound(route) : json(route, answered);
    }
    const one = /^\/events\/([^/]+)$/u.exec(path);
    if (one !== null) {
      const event = events.find((item) => item.id === one[1]);
      return event === undefined ? notFound(route) : json(route, event);
    }
    return notFound(route);
  });
}

test.describe("the events", () => {
  test("are listed, read in two time zones, answered, and created", async ({ page, hasTouch, makeAxeBuilder }) => {
    await serveApi(page);
    await page.goto("/home");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await openFromNavigation(page, "/events", hasTouch);
    await expect(page.getByRole("heading", { level: 1, name: message("events.title") })).toBeFocused();
    const main = page.getByRole("main");
    const list = main.getByRole("list", { name: message("events.list.label") });
    await expect(list.getByRole("listitem")).toHaveCount(1);
    // The viewer is in UTC; the event's own time in Tokyo follows, named by the zone's place.
    await expect(list.getByText(/\(Tokyo\)/u)).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The event's page: the place in person is named by its kind only, and the online link says where it leaves to.
    await list.getByRole("link", { name: "Night picnic" }).click();
    await expect(page.getByRole("heading", { level: 1, name: "Night picnic" })).toBeFocused();
    const where = main.getByRole("region", { name: message("events.place.title") });
    await expect(where.getByText(message("events.place.hidden.physical"))).toBeVisible();
    const stream = where.getByRole("link", { name: new RegExp(`^${escapeRegExp("Live stream")}`, "u") });
    await expect(stream).toHaveAttribute("target", "_blank");
    await expect(stream).toHaveAttribute("rel", /nofollow/u);
    await expect(where.getByText("stream.example", { exact: false })).toBeVisible();
    await expect(main.getByText(/\(Tokyo\)/u)).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Answer: the visible label of "Going", which saves at once.
    const answer = main.getByRole("radiogroup", { name: message("events.answer.title") });
    const going = answer.getByRole("radio", { name: message("events.answer.going"), exact: true });
    await answer.getByText(message("events.answer.going"), { exact: true }).click();
    await expect(going).toBeChecked();

    // Create: back to the list, New event, the title and the dates, Create event.
    await page.goto("/events");
    await expect(page.getByRole("heading", { level: 1, name: message("events.title") })).toBeVisible();
    await main.getByRole("link", { name: message("events.new") }).click();
    await expect(page.getByRole("heading", { level: 1, name: message("events.form.new_title") })).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await main
      .getByRole("textbox", { name: new RegExp(`^${escapeRegExp(message("events.form.title"))}`, "u") })
      .fill("Board games");
    const start = new Date(Date.now() + 10 * day).toISOString().slice(0, 10);
    await main.getByLabel(new RegExp(`^${escapeRegExp(message("events.form.starts_at"))}`, "u")).fill(`${start}T18:00`);
    await main.getByLabel(new RegExp(`^${escapeRegExp(message("events.form.ends_at"))}`, "u")).fill(`${start}T21:00`);
    await main.getByRole("button", { name: message("events.form.submit_new"), exact: true }).click();
    await expect(page.getByRole("heading", { level: 1, name: "Board games" })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
