// SPDX-License-Identifier: AGPL-3.0-only

// The events for the attendee (ADR-0161 §7–§12, §15; FR-943, FR-946–948, FR-952), on the fake API: what a story cannot
// reach, as it follows a press. Answering an event saves at once and lists the reader among those going; a going the
// server refuses at capacity says so; the calendar file downloads and is said; a date of a poll answered saves with
// its counts; Join the room of an event joins it and opens the room; an event's notification leads to it; the
// reminders save at once and come back to the defaults; and a visitor is asked to sign in, back to the event. What
// each state looks like is the stories' (events.stories.tsx).

import { expect, test } from "bun:test";
import { fakeOrigin, scenario, type FakeEventPlan } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words } from "./test/app";
import { callsOnFakes } from "./test/call";
import { fakeApp } from "./test/server";

/** Renders the app at the page of the fake's event of `key`. */
async function atEvent(key: string) {
  const on = fakeApp();
  const id = on.fake.events.id(key);
  const app = await renderApp(`/events/${id}`, { on });
  await main().findByRole("heading", { name: on.fake.events.shown(key)?.title ?? key, level: 2 });
  return { app, id };
}

/** Waits until the page says `sentence` in its main or in one of its live regions. */
const says = (sentence: string) =>
  waitFor(() => {
    const said = [words(screen.getByRole("main")), ...screen.queryAllByRole("status").map((s) => words(s))];
    if (!said.some((text) => text.includes(sentence))) throw new Error(`test: the page says ${JSON.stringify(said)}`);
  });

/** Events of `organizer` in UTC at noon, `inDays` days after the seed runs, titled `${title} ${index}`. */
const plansOf = (organizer: string, title: string, days: readonly number[]): FakeEventPlan[] =>
  days.map((inDays, index) => ({
    key: `${title.toLowerCase()}-${index}`,
    organizer,
    title: `${title} ${index}`,
    description: "",
    timeZone: "UTC",
    inDays,
    hour: 12,
    hours: 1,
  }));

/** The IDs of the events the fake lists to the reader at `query`, all of them. */
async function listed(serve: (request: Request) => Promise<Response>, query = ""): Promise<string[]> {
  const answer = await serve(new Request(`${fakeOrigin}/api/v1/events?limit=40${query}`));
  const page = (await answer.json()) as { items: { id: string }[] };
  return page.items.map((e) => e.id);
}

test("the list shows the upcoming events in the server's order, each leading to its page", async () => {
  const on = fakeApp();
  const answer = await on.fake.serve(new Request(`${fakeOrigin}/api/v1/events`));
  const page = (await answer.json()) as { items: { id: string }[] };
  await renderApp("/events", { on });
  const links = within(await main().findByRole("list", { name: "Upcoming events" })).getAllByRole("link");
  expect(links.map((l) => l.getAttribute("href"))).toEqual(page.items.map((e) => `/events/${e.id}`));
});

test("the list shows the soonest events first, and Show more brings the later ones after them, each once", async () => {
  // Seeded the latest first, so that the soonest first is the server's order and not the seed's.
  const later = plansOf(
    "hana",
    "Later",
    [51, 50, 49, 48, 47, 46, 45, 44, 43, 42, 41, 40, 39, 38, 37, 36, 35, 34, 33, 32, 31, 30],
  );
  const on = fakeApp({ seed: { ...scenario, events: [...scenario.events, ...later] } });
  const all = await listed(on.fake.serve);
  expect(all.length).toBeGreaterThan(20);
  const app = await renderApp("/events", { on });
  const list = await main().findByRole("list", { name: "Upcoming events" });
  const hrefs = () =>
    within(list)
      .getAllByRole("link")
      .map((l) => l.getAttribute("href"));
  expect(hrefs()).toEqual(all.slice(0, hrefs().length).map((id) => `/events/${id}`));
  expect(hrefs().length).toBeLessThan(all.length);
  await app.user.click(main().getByRole("button", { name: "Show more" }));
  await waitFor(() => {
    if (hrefs().length !== all.length) throw new Error(`test: the list shows ${hrefs().length} of ${all.length}`);
  });
  expect(hrefs()).toEqual(all.map((id) => `/events/${id}`));
  // The later ones, by their titles: the nearest (the last seeded) first.
  const numbers = within(list)
    .getAllByRole("link")
    .map((l) => /Later (\d+)/.exec(words(l))?.[1])
    .filter((n) => n !== undefined)
    .map(Number);
  expect(numbers).toEqual(later.map((_, index) => later.length - 1 - index));
});

test("an event's page gives its own time and its zone's place where the reader's zone is another, its clock alike", async () => {
  // A Tokyo event to a reader in Seoul: the clocks read alike, the zones differ.
  const seoul = fakeApp({ preferences: { time_zone: "Asia/Seoul", time_zone_follows_browser: false } });
  await renderApp(`/events/${seoul.fake.events.id("cooking-night")}`, { on: seoul });
  await main().findByRole("heading", { name: "Cooking night", level: 2 });
  await waitFor(() => {
    if (!/(?:^|[^\d:])7:00 PM \(Tokyo\)/.test(words(screen.getByRole("main"))))
      throw new Error(`test: the page says ${words(screen.getByRole("main"))}`);
  });
});

test("an event's page in the reader's own zone gives no second time", async () => {
  const tokyo = fakeApp({ preferences: { time_zone: "Asia/Tokyo", time_zone_follows_browser: false } });
  await renderApp(`/events/${tokyo.fake.events.id("cooking-night")}`, { on: tokyo });
  await main().findByRole("heading", { name: "Cooking night", level: 2 });
  // The reader's own time is drawn, from 7 to 10 PM: the event's own, and no zone's place after it.
  await waitFor(() => {
    if (!/(?:^|[^\d:])7:00 – 10:00 PM/.test(words(screen.getByRole("main"))))
      throw new Error(`test: the page says ${words(screen.getByRole("main"))}`);
  });
  expect(words(screen.getByRole("main"))).not.toContain("(Tokyo)");
});

test("an event's own time carries its weekday where its day is another than the reader's", async () => {
  // Paris at 6 PM is 2 AM of the next day in Tokyo.
  const on = fakeApp({ preferences: { time_zone: "Asia/Tokyo", time_zone_follows_browser: false } });
  const id = on.fake.events.id("book-swap");
  const event = on.fake.events.shown("book-swap");
  if (event === undefined) throw new Error("test: the fake has no book swap the reader sees");
  const weekday = new Intl.DateTimeFormat("en", { weekday: "short", timeZone: "Europe/Paris" }).format(
    new Date(event.starts_at),
  );
  await renderApp(`/events/${id}`, { on });
  await main().findByRole("heading", { name: "Book swap", level: 2 });
  await waitFor(() => {
    if (!words(screen.getByRole("main")).includes(`${weekday} 6:00 PM (Paris)`))
      throw new Error(`test: the page says ${words(screen.getByRole("main"))}`);
  });
});

test("at capacity Going is refused, and Maybe and Not going still save", async () => {
  const { app } = await atEvent("tiny-concert");
  const answers = main().getByRole("radiogroup", { name: "Your answer" });
  expect(within(answers).getByRole("radio", { name: "Going" }).hasAttribute("disabled")).toBe(true);
  expect(within(answers).getByRole("radio", { name: "Maybe" }).hasAttribute("disabled")).toBe(false);
  expect(within(answers).getByRole("radio", { name: "Not going" }).hasAttribute("disabled")).toBe(false);
  await app.user.click(within(answers).getByRole("radio", { name: "Maybe" }));
  await says("Saved");
  expect(app.fake.events.shown("tiny-concert")?.viewer.answer).toBe("maybe");
  // Maybe gives the board: the answers are drawn again under the Details tab.
  const again = main().getByRole("radiogroup", { name: "Your answer" });
  await app.user.click(within(again).getByRole("radio", { name: "Not going" }));
  await waitFor(() => {
    if (app.fake.events.shown("tiny-concert")?.viewer.answer !== "declined")
      throw new Error("test: the server holds no not going");
  });
});

test("the page marks the event ended as it ends, with the page left open, and offers its calendar file no more", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("garden-day");
  const started = new Date(Date.now() - 60_000).toISOString();
  const ends = new Date(Date.now() + 1500).toISOString();
  on.fake.script("GET /events/{id}", { change: (event) => ({ ...event, starts_at: started, ends_at: ends }) });
  await renderApp(`/events/${id}`, { on });
  await main().findByRole("heading", { name: "Community garden day", level: 2 });
  expect(main().getAllByRole("button", { name: "Add to calendar" })).toHaveLength(1);
  expect(main().queryAllByText("Ended")).toHaveLength(0);
  await waitFor(
    () => {
      if (main().queryAllByText("Ended").length !== 1) throw new Error("test: the event is not marked ended");
    },
    { timeout: 4000 },
  );
  expect(main().queryAllByRole("button", { name: "Add to calendar" })).toHaveLength(0);
});

test("an event that started offers its calendar file", async () => {
  await atEvent("trail-talk");
  expect(main().getAllByRole("button", { name: "Add to calendar" })).toHaveLength(1);
});

test("answering going saves at once, and the reader is then among those going", async () => {
  const { app } = await atEvent("garden-day");
  const answers = main().getByRole("radiogroup", { name: "Your answer" });
  await app.user.click(within(answers).getByRole("radio", { name: "Going" }));
  await says("Saved");
  expect(app.fake.events.shown("garden-day")?.viewer.answer).toBe("going");
  const going = await main().findByRole("list", { name: "Going (2)" });
  expect(words(going)).toContain("Yuki");
});

test("going refused at capacity says so, and the event is read again: Going is then refused before a press", async () => {
  const { app } = await atEvent("garden-day");
  // Someone took the last place since the page was read: the server refuses, and now says the event is full.
  app.fake.script("PUT /events/{id}/rsvp", {
    times: 1,
    fail: { status: 409, type: "/api/v1/problems/event-full", title: "The event is full" },
  });
  app.fake.script("GET /events/{id}", { change: (event) => ({ ...event, capacity: event.going_count }) });
  const answers = main().getByRole("radiogroup", { name: "Your answer" });
  await app.user.click(within(answers).getByRole("radio", { name: "Going" }));
  await says("Not saved: the event is full.");
  await waitFor(() => {
    if (!within(answers).getByRole("radio", { name: "Going" }).hasAttribute("disabled"))
      throw new Error("test: Going can still be chosen");
  });
});

test("an answer refused once the event started is read again: the answers are then closed", async () => {
  const { app } = await atEvent("garden-day");
  app.fake.script("PUT /events/{id}/rsvp", {
    times: 1,
    fail: { status: 409, type: "/api/v1/problems/event-closed", title: "The event is closed" },
  });
  const started = new Date(Date.now() - 60_000).toISOString();
  app.fake.script("GET /events/{id}", { change: (event) => ({ ...event, starts_at: started }) });
  const answers = main().getByRole("radiogroup", { name: "Your answer" });
  await app.user.click(within(answers).getByRole("radio", { name: "Maybe" }));
  await waitFor(() => {
    const radios = within(answers).getAllByRole("radio");
    if (!radios.every((r) => r.hasAttribute("disabled"))) throw new Error("test: the answers are still open");
  });
});

test("the answers close as the event starts, with the page left open and no read of it again", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("garden-day");
  const starts = new Date(Date.now() + 1500).toISOString();
  on.fake.script("GET /events/{id}", { change: (event) => ({ ...event, starts_at: starts }) });
  await renderApp(`/events/${id}`, { on });
  const answers = await main().findByRole("radiogroup", { name: "Your answer" });
  expect(within(answers).getByRole("radio", { name: "Going" }).hasAttribute("disabled")).toBe(false);
  await waitFor(
    () => {
      if (!within(answers).getByRole("radio", { name: "Going" }).hasAttribute("disabled"))
        throw new Error("test: the answers are still open after the start");
    },
    { timeout: 4000 },
  );
  await says("Answers closed when the event started.");
});

test("a cancelled event offers its calendar file, which says it is cancelled", async () => {
  await atEvent("flea-market");
  expect(main().getByRole("button", { name: "Add to calendar" })).toBeDefined();
});

test("the column beside the timeline says of an event whose date is polled that it is, not a date", async () => {
  const on = fakeApp();
  const polled = on.fake.events.shown("photo-walk");
  if (polled === undefined) throw new Error("test: the fake has no poll the reader sees");
  const title = polled.title;
  on.fake.script("GET /events", { change: (page) => ({ ...page, items: [polled] }) });
  await renderApp("/lists", { on });
  // The lists' screen lists no event: a link of the event is the column's.
  const [link] = await screen.findAllByRole("link", { name: title });
  expect(words(link?.parentElement ?? document.body)).toContain("Date to be decided");
});

test("Add to calendar downloads the event's file and says so, the focus staying on the button", async () => {
  const { app, id } = await atEvent("book-swap");
  const button = main().getByRole("button", { name: "Add to calendar" });
  await app.user.click(button);
  await says(`Downloaded ${id}.ics.`);
  expect(app.focused()).toBe(button);
});

test("answering a date of a poll saves at once with its counts", async () => {
  const { app } = await atEvent("photo-walk");
  const table = await main().findByRole("table", { name: "Date poll" });
  const [first] = within(table).getAllByRole("radiogroup");
  if (first === undefined) throw new Error("test: the poll has no date");
  await app.user.click(within(first).getByRole("radio", { name: /Can go/ }));
  await says("Saved");
  const [candidate] = (
    (await (
      await app.fake.serve(new Request(`${fakeOrigin}/api/v1/events/${app.fake.events.id("photo-walk")}/candidates`))
    ).json()) as { items: { vote?: string }[] }
  ).items;
  expect(candidate?.vote).toBe("yes");
});

test("a date's answer that failed stays said beside a later one, and its Try again sends that date's answer", async () => {
  const { app, id } = await atEvent("photo-walk");
  const table = await main().findByRole("table", { name: "Date poll" });
  const [first, second] = within(table).getAllByRole("radiogroup");
  if (first === undefined || second === undefined) throw new Error("test: the poll has no two dates");
  app.fake.script("PUT /events/{id}/candidates/{candidate_id}/vote", { times: 1, fail: 500 });
  await app.user.click(within(first).getByRole("radio", { name: /Cannot go/ }));
  await says("did not finish.");
  await app.user.click(within(second).getByRole("radio", { name: /Can go/ }));
  await says("Saved");
  // The first date's failure is still said, with its own Try again.
  await says("did not finish.");
  const votes = app.fake.script("PUT /events/{id}/candidates/{candidate_id}/vote");
  await app.user.click(main().getByRole("button", { name: "Try again" }));
  await votes.served();
  const answer = await app.fake.serve(new Request(`${fakeOrigin}/api/v1/events/${id}/candidates`));
  const { items } = (await answer.json()) as { items: { vote?: string }[] };
  expect(items.map((c) => c.vote).slice(0, 2)).toEqual(["no", "yes"]);
});

test("who answered each date of a poll is folded, one Show a date, and unfolds into that date's people", async () => {
  const { app } = await atEvent("photo-walk");
  await main().findByRole("table", { name: "Date poll" });
  // The fake's poll has three dates: sam and hana can go to the first.
  const shows = main().getAllByRole("button", { name: "Show" });
  expect(shows).toHaveLength(3);
  expect(shows.every((b) => b.getAttribute("aria-expanded") === "false")).toBe(true);
  expect(main().queryAllByRole("list", { name: /^Can go/ })).toHaveLength(0);
  const [first] = shows;
  if (first === undefined) throw new Error("test: no date to unfold");
  await app.user.click(first);
  const people = await main().findByRole("list", { name: "Can go (2)" });
  expect(words(people)).toContain("Hana");
  expect(words(people)).toContain("Sam Rivera");
  expect(first.getAttribute("aria-expanded")).toBe("true");
  // The other dates stay folded.
  expect(shows.slice(1).map((b) => b.getAttribute("aria-expanded"))).toEqual(["false", "false"]);
});

test("Join the room joins the event's open room and opens its page", async () => {
  const calls = callsOnFakes();
  const { app } = await atEvent("coffee-voice");
  const room = app.fake.events.shown("coffee-voice")?.places.find((p) => p.kind === "voice")?.room_id;
  await app.user.click(await main().findByRole("button", { name: "Join the room" }));
  await calls.clock.advance(0);
  await app.arrived(`/voice/${room ?? ""}`);
  expect(app.fake.voice.joined()?.room).toBe(room);
});

test("an event's notification leads to the event", async () => {
  const on = fakeApp({ everyKind: "people" });
  const event = on.fake.events.id("meetup");
  await renderApp("/notifications", { on });
  const links = (await main().findAllByRole("link")).map((l) => l.getAttribute("href"));
  expect(links).toContain(`/events/${event}`);
});

test("the organizer's profile lists three of their upcoming events at most, the soonest", async () => {
  // sam organizes four that have not ended with this one: the profile lists the three the server lists first.
  const on = fakeApp({ seed: { ...scenario, events: [...scenario.events, ...plansOf("sam", "Soon", [1])] } });
  const theirs = await listed(on.fake.serve, `&organizer_id=${on.fake.accountId("sam")}`);
  expect(theirs.length).toBeGreaterThan(3);
  await renderApp("/@sam", { on });
  const list = await main().findByRole("list", { name: "Upcoming events" });
  expect(
    within(list)
      .getAllByRole("link")
      .map((l) => l.getAttribute("href")),
  ).toEqual(theirs.slice(0, 3).map((id) => `/events/${id}`));
});

test("the reminders are three at most, each none or from 5 minutes to 7 days before", async () => {
  await renderApp("/settings/events");
  await main().findByRole("combobox", { name: "Reminder 1" });
  expect(main().getAllByRole("combobox")).toHaveLength(3);
  expect(main().queryAllByRole("combobox", { name: "Reminder 4" })).toHaveLength(0);
  for (const name of ["Reminder 1", "Reminder 2", "Reminder 3"]) {
    const options = within(main().getByRole("combobox", { name })).getAllByRole("option");
    const said = options.map((o) => words(o));
    expect([name, said[0], said[1], said.at(-1)]).toEqual([name, "None", "7 days before", "5 minutes before"]);
    const minutes = options
      .map((o) => o.getAttribute("value"))
      .filter((v) => v !== "")
      .map(Number);
    expect([name, minutes.every((m) => m >= 5 && m <= 7 * 24 * 60)]).toEqual([name, true]);
  }
});

test("a reminder chosen saves at once, and the defaults come back by their button", async () => {
  const app = await renderApp("/settings/events");
  const second = await main().findByRole("combobox", { name: "Reminder 2" });
  await app.user.selectOptions(second, "None");
  await says("Saved");
  await app.user.click(main().getByRole("button", { name: "Restore the defaults" }));
  await says("Restored the defaults");
});

test("a reminder chosen stays in the place it was chosen in once saved, though the server keeps them the earliest first", async () => {
  const app = await renderApp("/settings/events");
  const first = await main().findByRole("combobox", { name: "Reminder 1" });
  // The defaults: one day, then one hour before. A third one, earlier than both, stays the third.
  await app.user.selectOptions(main().getByRole("combobox", { name: "Reminder 3" }), "2 days before");
  await says("Saved");
  const values = () =>
    ["Reminder 1", "Reminder 2", "Reminder 3"].map(
      (name) => main().getByRole<HTMLSelectElement>("combobox", { name }).value,
    );
  await waitFor(() => {
    if (values().join() !== "1440,60,2880") throw new Error(`test: the reminders show ${values().join()}`);
  });
  expect((first as HTMLSelectElement).value).toBe("1440");
});

test("a visitor is told only this server's people see events, and the one Sign in comes back to the event", async () => {
  const on = fakeApp({ visitor: true });
  const id = on.fake.events.id("book-swap");
  const app = await renderApp(`/events/${id}`, { on });
  expect(words(screen.getByRole("main"))).toContain("Only people with an account on this server can see its events.");
  expect(main().getAllByRole("button", { name: "Sign in" })).toHaveLength(1);
  await app.user.click(main().getByRole("button", { name: "Sign in" }));
  await waitFor(() => {
    if (!app.path().startsWith("/login")) throw new Error(`test: the app is at ${app.path()}`);
  });
  expect(new URLSearchParams(app.path().split("?")[1]).get("return_to")).toBe(`/events/${id}`);
});
