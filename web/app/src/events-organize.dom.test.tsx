// SPDX-License-Identifier: AGPL-3.0-only

// The events for the organizer (ADR-0161 §3–§8; FR-940, FR-943, FR-944, FR-948), on the fake API: what a story cannot
// reach, as it follows a press. Creating an event with a fixed date and with a poll opens its page; a title left empty
// is marked and takes the focus; an attendee approved, a poll's date confirmed, the event cancelled with a note, a
// person invited and a co-organizer removed are each said, and the server holds them. And who is offered Create an
// event, and the failed read of it; the editor's faults of a fixed date, the poll's dates (a day after the last, 20 at
// most), the dates on the event's clock and what a change sends of them; the voice room by the organizer's right and
// the event's place; the places' fields and links; the server's refusals by field; Cancel for the organizer alone; an
// approval removed; a date withdrawn, the last kept, and a refused confirmation read again; and the invitations' and
// co-organizers' order, focus and limit; and no approval asked of an attendee who organizes, the event open, cancelled
// or ended, and an approval refused as they organize read again. What each state looks like is the stories'
// (events.stories.tsx).

import { scenario, type FakeEventPlan, type FakeOptions } from "@my/e2e/fake";
import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words } from "./test/app";
import { fakeApp } from "./test/server";

/** Waits until the page says `sentence` in its main or in one of its live regions. */
const says = (sentence: string) =>
  waitFor(() => {
    const said = [words(screen.getByRole("main")), ...screen.queryAllByRole("status").map((s) => words(s))];
    if (!said.some((text) => text.includes(sentence))) throw new Error(`test: the page says ${JSON.stringify(said)}`);
  });

/** Renders the app at `path` under the fake's event of `key` (its page for ""), once its heading shows. */
async function atEvent(key: string, path = "", heading?: string, options: FakeOptions = {}) {
  const on = fakeApp(options);
  const id = on.fake.events.id(key);
  const app = await renderApp(`/events/${id}${path}`, { on });
  await main().findByRole("heading", { name: heading ?? on.fake.events.shown(key)?.title ?? key, level: 2 });
  return { app, id, on };
}

test("the list offers Create an event, and the form creates an event with a fixed date whose page takes its place", async () => {
  const on = fakeApp();
  const app = await renderApp("/events", { on });
  await app.user.click(await screen.findByRole("link", { name: "Create an event" }));
  await app.user.type(await main().findByRole("textbox", { name: "Title" }), "Tea in the park");
  await app.user.click(main().getByRole("button", { name: "Create the event" }));
  await main().findByRole("heading", { name: "Tea in the park", level: 2 });
  expect(app.path()).toBe(`/events/${on.fake.events.id("Tea in the park")}`);
  expect(on.fake.events.shown("Tea in the park")?.state).toBe("scheduled");
});

test("a new event can poll its date: its page shows the poll's table with the dates offered", async () => {
  const on = fakeApp();
  const app = await renderApp("/events/new", { on });
  await app.user.type(await main().findByRole("textbox", { name: "Title" }), "Board games");
  await app.user.click(main().getByRole("radio", { name: /Poll for a date/ }));
  await app.user.click(main().getByRole("button", { name: "Create the event" }));
  await main().findByRole("heading", { name: "Board games", level: 2 });
  const table = await main().findByRole("table", { name: "Date poll" });
  // The form offers two dates to start from.
  await waitFor(() => {
    expect(within(table).getAllByRole("rowheader")).toHaveLength(2);
  });
  expect(on.fake.events.shown("Board games")?.state).toBe("unscheduled");
});

test("a title left empty is marked under its field, which takes the focus, and nothing is sent", async () => {
  const on = fakeApp();
  const app = await renderApp("/events/new", { on });
  const title = await main().findByRole("textbox", { name: "Title" });
  await app.user.click(main().getByRole("button", { name: "Create the event" }));
  await says("Give the event a title.");
  expect(title.getAttribute("aria-invalid")).toBe("true");
  expect(document.activeElement).toBe(title);
  expect(app.path()).toBe("/events/new");
});

test("an attendee waiting for approval says so, and its Approve approves them, the focus on what takes its place", async () => {
  const { app } = await atEvent("bread-class");
  const approve = await main().findByRole("button", { name: "Approve ليلى Layla" });
  expect(main().getAllByText("Waiting for approval")).toHaveLength(2);
  await app.user.click(approve);
  await says("You approved ليلى Layla.");
  expect(main().getAllByText("Approved")).toHaveLength(2);
  // Approve gave way to the menu that takes the approval back.
  await waitFor(() => {
    expect(document.activeElement).toBe(main().getByRole("button", { name: "Actions for ليلى Layla" }));
  });
});

test("an organizer confirms a date of the poll, asked first: the date is fixed and said", async () => {
  const { app, on } = await atEvent("photo-walk");
  const table = await main().findByRole("table", { name: "Date poll" });
  const [first] = within(table).getAllByRole("button", { name: /^Options for / });
  if (first === undefined) throw new Error("test: no date's menu");
  await app.user.click(first);
  await app.user.click(await screen.findByRole("menuitem", { name: "Confirm this date…" }));
  const dialog = await screen.findByRole("alertdialog");
  await app.user.click(within(dialog).getByRole("button", { name: "Confirm" }));
  await says("The event is on");
  expect(on.fake.events.shown("photo-walk")?.state).toBe("scheduled");
});

test("a poll whose dates could not be read says so, with Try again", async () => {
  const on = fakeApp();
  on.fake.script("GET /events/{id}/candidates", { fail: 500 });
  const id = on.fake.events.id("photo-walk");
  await renderApp(`/events/${id}`, { on });
  await says("The dates could not be loaded.");
});

test("the organizer cancels the event with a note from its menu, asked first", async () => {
  const { app, on } = await atEvent("photo-walk");
  await app.user.click(main().getByRole("button", { name: "Manage the event" }));
  await app.user.click(await screen.findByRole("menuitem", { name: "Cancel the event…" }));
  const dialog = await screen.findByRole("dialog", { name: "Cancel the event?" });
  await app.user.type(within(dialog).getByRole("textbox"), "The ridge is closed.");
  await app.user.click(within(dialog).getByRole("button", { name: "Cancel the event" }));
  await says("You cancelled the event.");
  expect(on.fake.events.shown("photo-walk")?.cancellation?.note).toBe("The ridge is closed.");
});

test("a cancellation the server says was made already closes the dialog, says so and reads the event again", async () => {
  const { app } = await atEvent("photo-walk");
  app.fake.script("POST /events/{id}/cancel", {
    times: 1,
    fail: { type: "/api/v1/problems/event-closed", title: "The event is closed", status: 409 },
  });
  const read = app.fake.script("GET /events/{id}");
  await app.user.click(main().getByRole("button", { name: "Manage the event" }));
  await app.user.click(await screen.findByRole("menuitem", { name: "Cancel the event…" }));
  const dialog = await screen.findByRole("dialog", { name: "Cancel the event?" });
  await app.user.click(within(dialog).getByRole("button", { name: "Cancel the event" }));
  await says("The event was already cancelled.");
  await read.arrived();
  await waitFor(() => {
    expect(screen.queryByRole("dialog", { name: "Cancel the event?" })).toBeNull();
  });
});

test("a note the server refuses is said under the note, which takes the focus", async () => {
  const { app } = await atEvent("photo-walk");
  app.fake.script("POST /events/{id}/cancel", {
    times: 1,
    fail: { type: "about:blank", title: "This value is not valid", status: 422, field: "note" },
  });
  await app.user.click(main().getByRole("button", { name: "Manage the event" }));
  await app.user.click(await screen.findByRole("menuitem", { name: "Cancel the event…" }));
  const dialog = await screen.findByRole("dialog", { name: "Cancel the event?" });
  const note = within(dialog).getByRole("textbox");
  await app.user.type(note, "Closed.");
  await app.user.click(within(dialog).getByRole("button", { name: "Cancel the event" }));
  await waitFor(() => {
    expect(note.getAttribute("aria-invalid")).toBe("true");
  });
  expect(document.activeElement).toBe(note);
});

test("an organizer invites someone found by name, which the screen says, the focus back on the search", async () => {
  const { app } = await atEvent("bread-class", "/invitations", "Invitations");
  const search = main().getByRole("searchbox", { name: "Name or handle" });
  await app.user.type(search, "ren");
  const found = await main().findByRole("list", { name: "People found" });
  await app.user.click(await within(found).findByRole("button", { name: "Invite" }));
  await says("You invited Ren 蓮.");
  const invited = main().getByRole("list", { name: "Invited" });
  await within(invited).findByText("Ren 蓮");
  await waitFor(() => {
    expect(document.activeElement).toBe(search);
  });
});

test("an invitation is revoked from its menu, asked first, the focus on the list", async () => {
  const { app } = await atEvent("bread-class", "/invitations", "Invitations");
  await app.user.click(await main().findByRole("button", { name: "Actions for Mio 澪" }));
  await app.user.click(await screen.findByRole("menuitem", { name: "Revoke the invitation…" }));
  const dialog = await screen.findByRole("alertdialog");
  await app.user.click(within(dialog).getByRole("button", { name: "Revoke" }));
  await says("You revoked the invitation of Mio 澪.");
  await waitFor(() => {
    expect(document.activeElement).toBe(main().getByRole("group", { name: "Invited" }));
  });
});

test("the organizer removes a co-organizer from their menu, asked first, which the screen says", async () => {
  const { app, on } = await atEvent("photo-walk", "/coorganizers", "Co-organizers");
  await app.user.click(await main().findByRole("button", { name: "Actions for Hana 花" }));
  await app.user.click(await screen.findByRole("menuitem", { name: "Remove as co-organizer…" }));
  const dialog = await screen.findByRole("alertdialog");
  await app.user.click(within(dialog).getByRole("button", { name: "Remove" }));
  await says("Hana 花 is no longer a co-organizer.");
  expect(on.fake.events.shown("photo-walk")?.coorganizers).toHaveLength(0);
  // The focus goes to the list the row left.
  await waitFor(() => {
    expect(focusedGroup()).toMatch(/^group: Co-organizers/);
  });
});

test("the voice room is offered to the organizer who may host rooms", async () => {
  await atEvent("photo-walk", "/edit", "Change the event");
  expect(main().queryByRole("checkbox", { name: /voice chat room/ })).not.toBeNull();
});

test("a co-organizer is not offered a voice room the event does not have: the server checks the organizer's right", async () => {
  await atEvent("bread-class", "/edit", "Change the event");
  expect(main().queryByRole("checkbox", { name: /voice chat room/ })).toBeNull();
});

test("a refusal of the places marks them as a group, which takes the focus", async () => {
  const on = fakeApp();
  on.fake.script("POST /events", {
    times: 1,
    fail: { type: "about:blank", title: "This value is not valid", status: 422, field: "places" },
  });
  const app = await renderApp("/events/new", { on });
  await app.user.type(await main().findByRole("textbox", { name: "Title" }), "Tea");
  await app.user.click(main().getByRole("button", { name: "Create the event" }));
  const places = await main().findByRole("group", { name: "Where" });
  await waitFor(() => {
    expect(document.activeElement).toBe(places);
  });
  expect(places.getAttribute("aria-describedby")).toMatch(/-error$/);
});

test("changing only the time zone of an event sends no date, its moments kept", async () => {
  const { app } = await atEvent("bread-class", "/edit", "Change the event");
  const patch = app.fake.script("PATCH /events/{id}");
  const zone = main().getByRole("combobox", { name: "Time zone" });
  await app.user.selectOptions(zone, "UTC");
  await app.user.click(main().getByRole("button", { name: "Save" }));
  const sent = await patch.arrived();
  expect(sent.body.time_zone).toBe("UTC");
  expect(sent.body.starts_at).toBeUndefined();
  expect(sent.body.ends_at).toBeUndefined();
});

test("each date a poll adds starts a day after the last, so a new poll of three dates is created", async () => {
  const on = fakeApp();
  const app = await renderApp("/events/new", { on });
  await app.user.type(await main().findByRole("textbox", { name: "Title" }), "Three days");
  await app.user.click(main().getByRole("radio", { name: /Poll for a date/ }));
  await app.user.click(main().getByRole("button", { name: "Add a date" }));
  await app.user.click(main().getByRole("button", { name: "Create the event" }));
  await main().findByRole("heading", { name: "Three days", level: 2 });
  const table = await main().findByRole("table", { name: "Date poll" });
  await waitFor(() => {
    expect(within(table).getAllByRole("rowheader")).toHaveLength(3);
  });
});

// ---- Who may create events, the editor's dates, places and refusals, and the organizers' screens (#3238) ----------

const hour = 3_600_000;
const day = 24 * hour;

/** A whole hour about `days` days from now. */
const ahead = (days: number) => Math.ceil((Date.now() + days * day) / hour) * hour;

/** How a field of a date and a time writes the instant `ms` on the clock of `zone`: "2027-01-10T09:00". */
function local(ms: number, zone: string): string {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(ms));
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((p) => p.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}-${part("day")}T${part("hour")}:${part("minute")}`;
}

/** The server with the events of `plans` beside the scenario's. */
const withEvents = (...plans: FakeEventPlan[]): FakeOptions => ({
  seed: { ...scenario, events: [...scenario.events, ...plans] },
});

/** A poll the reader organizes that has one date left. */
const oneDate: FakeEventPlan = {
  key: "one-date",
  organizer: "yuki",
  title: "Tea, one date",
  description: "",
  timeZone: "Asia/Tokyo",
  inDays: 5,
  hour: 9,
  hours: 1,
  candidates: [{ inDays: 5, hour: 9, hours: 1 }],
};

/** An event in a voice chat room, which sam organizes and the reader co-organizes. */
const voiceCoorganized: FakeEventPlan = {
  key: "voice-co",
  organizer: "sam",
  coorganizers: ["yuki"],
  title: "Voice night",
  description: "",
  timeZone: "Asia/Tokyo",
  inDays: 6,
  hour: 20,
  hours: 1,
  places: [{ kind: "voice", speakers: "manual" }],
};

/** An event the reader organizes with as many co-organizers as an event has. */
const fullyCoorganized: FakeEventPlan = {
  key: "three-co",
  organizer: "yuki",
  coorganizers: ["hana", "sam", "mio"],
  title: "Three helpers",
  description: "",
  timeZone: "Asia/Tokyo",
  inDays: 9,
  hour: 10,
  hours: 2,
};

/** The field of a date and a time named `name` in `scope` (it has no role). */
const dateField = (name: "Starts" | "Ends", scope = main()) =>
  scope.getByLabelText<HTMLInputElement>(name, { exact: true });

/** Sets the field of a date and a time `field` to `value`, as the browser's picker does. */
const setDate = (field: HTMLElement, value: string) => {
  fireEvent.change(field, { target: { value } });
};

/** Whether `element` is marked as wrong. */
const marked = (element: Element) => element.getAttribute("aria-invalid") === "true";

/** What says that `element` is wrong: the texts of what describes it whose id is an error's. */
const errorOf = (element: Element) =>
  (element.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id.endsWith("-error"))
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ")
    .trim();

/** What has the focus, as "<role>: <name>" of a group, or the element's tag and label. */
function focusedGroup(): string {
  const el = document.activeElement;
  if (el === null || el === document.body) return "nothing";
  const role = el.getAttribute("role") ?? el.tagName.toLowerCase();
  return `${role}: ${el.getAttribute("aria-label") ?? words(el)}`;
}

describe("who may create events", () => {
  test("on a server where only some may, Create an event is offered to a reader whose roles have create_events", async () => {
    const on = fakeApp({ eventCreators: "permitted" });
    on.fake.script("GET /accounts/verify_credentials", {
      change: (account) => ({ ...account, permissions: [...account.permissions, "create_events" as const] }),
    });
    await renderApp("/events", { on });
    expect(await screen.findByRole("link", { name: "Create an event" })).not.toBeNull();
  });

  test("on a server where only some may, a reader without the right is not offered Create an event", async () => {
    const on = fakeApp({ eventCreators: "permitted" });
    await renderApp("/events", { on });
    await main().findAllByRole("link", { name: /.+/ });
    await waitFor(() => {
      expect(on.data.queryClient.isFetching()).toBe(0);
    });
    expect(screen.queryAllByRole("link", { name: "Create an event" })).toHaveLength(0);
  });

  test("a failed read of whether the reader may create events offers Try again, which brings the form", async () => {
    const on = fakeApp();
    const failing = on.fake.script("GET /instance", { fail: 500 });
    const app = await renderApp("/events/new", { on });
    const alert = await main().findByRole("alert");
    expect(main().queryAllByRole("textbox")).toHaveLength(0);
    failing.end();
    await app.user.click(within(alert).getByRole("button", { name: "Try again" }));
    expect(await main().findByRole("textbox", { name: "Title" })).not.toBeNull();
  });
});

describe("the editor's fixed date", () => {
  const cases: readonly (readonly [string, boolean, () => readonly [number, number], readonly string[]])[] = [
    ["a title left empty and an end before the start", false, () => [ahead(3), ahead(3) - hour], ["Title", "Ends"]],
    [
      "a title left empty and a date longer than 7 days",
      false,
      () => [ahead(3), ahead(3) + 8 * day],
      ["Title", "Ends"],
    ],
    ["a start more than a year ahead", true, () => [ahead(400), ahead(400) + hour], ["Starts"]],
  ];
  for (const [what, titled, dates, faults] of cases)
    test(`${what}: every fault is marked at once, the focus on the first, and nothing is sent`, async () => {
      const on = fakeApp();
      const create = on.fake.script("POST /events");
      const app = await renderApp("/events/new", { on });
      const title = await main().findByRole("textbox", { name: "Title" });
      if (titled) await app.user.type(title, "Tea");
      await app.user.selectOptions(main().getByRole("combobox", { name: "Time zone" }), "UTC");
      const [start, end] = dates();
      setDate(dateField("Starts"), local(start, "UTC"));
      setDate(dateField("Ends"), local(end, "UTC"));
      await app.user.click(main().getByRole("button", { name: "Create the event" }));
      const fields = { Title: title, Starts: dateField("Starts"), Ends: dateField("Ends") };
      await waitFor(() => {
        expect(Object.entries(fields).flatMap(([name, field]) => (marked(field) ? [name] : []))).toEqual([...faults]);
      });
      for (const name of faults) expect(errorOf(fields[name as keyof typeof fields])).not.toBe("");
      expect(document.activeElement === fields[faults[0] as keyof typeof fields]).toBe(true);
      expect(create.seen).toHaveLength(0);
    });
});

describe("the poll's dates in the editor", () => {
  test("Add a date offers the last date a day later, as it is now", async () => {
    const app = await renderApp("/events/new");
    await app.user.click(await main().findByRole("radio", { name: /^Poll for a date/ }));
    await app.user.selectOptions(main().getByRole("combobox", { name: "Time zone" }), "UTC");
    const second = within(main().getByRole("group", { name: "Date 2" }));
    const start = ahead(6) + 30 * 60_000;
    setDate(dateField("Starts", second), local(start, "UTC"));
    setDate(dateField("Ends", second), local(start + 3 * hour, "UTC"));
    await app.user.click(main().getByRole("button", { name: "Add a date" }));
    const third = within(await main().findByRole("group", { name: "Date 3" }));
    expect(dateField("Starts", third).value).toBe(local(start + day, "UTC"));
    expect(dateField("Ends", third).value).toBe(local(start + 3 * hour + day, "UTC"));
  });

  test("a poll offers up to 20 dates: at 20, Add a date is no longer offered", async () => {
    const app = await renderApp("/events/new");
    await app.user.click(await main().findByRole("radio", { name: /^Poll for a date/ }));
    for (let n = 3; n <= 20; n += 1) await app.user.click(main().getByRole("button", { name: "Add a date" }));
    expect(main().getByRole("group", { name: "Date 20" })).not.toBeNull();
    const add = main().queryAllByRole("button", { name: "Add a date" });
    expect(add.every((b) => b.hasAttribute("disabled") || b.getAttribute("aria-disabled") === "true")).toBe(true);
  });

  test("the page of a poll offers to add the last date a day later, in the event's zone", async () => {
    const { on } = await atEvent("one-date", "", undefined, withEvents(oneDate));
    const shown = on.fake.events.shown("one-date");
    if (shown === undefined) throw new Error("test: the fake has no one-date event");
    const add = within(main().getByRole("group", { name: "Add a date" }));
    expect(dateField("Starts", add).value).toBe(local(Date.parse(shown.starts_at) + day, "Asia/Tokyo"));
    expect(dateField("Ends", add).value).toBe(local(Date.parse(shown.ends_at) + day, "Asia/Tokyo"));
  });
});

describe("dates in the event's zone, for a reader in another", () => {
  let zone: string | undefined;
  beforeAll(() => {
    zone = process.env["TZ"];
    process.env["TZ"] = "Europe/London";
  });
  afterAll(() => {
    if (zone === undefined) delete process.env["TZ"];
    else process.env["TZ"] = zone;
  });
  const reader: FakeOptions = { preferences: { time_zone: "America/New_York", time_zone_follows_browser: false } };

  test("the editor writes an event's dates on the clock of its zone", async () => {
    const { on } = await atEvent("bread-class", "/edit", "Change the event", reader);
    const shown = on.fake.events.shown("bread-class");
    if (shown === undefined) throw new Error("test: the fake has no bread-class event");
    expect(dateField("Starts").value).toBe(local(Date.parse(shown.starts_at), "Asia/Tokyo"));
    expect(dateField("Ends").value).toBe(local(Date.parse(shown.ends_at), "Asia/Tokyo"));
  });

  test("a fixed date entered in the form is sent as the moment it is in the event's zone", async () => {
    const on = fakeApp(reader);
    const create = on.fake.script("POST /events");
    const app = await renderApp("/events/new", { on });
    await app.user.type(await main().findByRole("textbox", { name: "Title" }), "Tea in Tokyo");
    await app.user.selectOptions(main().getByRole("combobox", { name: "Time zone" }), "Asia/Tokyo");
    const start = ahead(30);
    setDate(dateField("Starts"), local(start, "Asia/Tokyo"));
    setDate(dateField("Ends"), local(start + 2 * hour, "Asia/Tokyo"));
    await app.user.click(main().getByRole("button", { name: "Create the event" }));
    const sent = await create.arrived();
    expect(sent.body.time_zone).toBe("Asia/Tokyo");
    expect(Date.parse(sent.body.starts_at ?? "")).toBe(start);
    expect(Date.parse(sent.body.ends_at ?? "")).toBe(start + 2 * hour);
  });
});

/** The next night whose New York clock reads 1:30 twice, at its second 1:30 (6:30 UTC): November's first Sunday. */
function fallBackNight(): number {
  const year = new Date().getUTCFullYear();
  for (const y of [year, year + 1]) {
    const first = new Date(Date.UTC(y, 10, 1)).getUTCDay();
    const at = Date.UTC(y, 10, 1 + ((7 - first) % 7), 6, 30);
    if (at > Date.now() + day) return at;
  }
  throw new Error("test: no fall-back night within a year");
}

describe("what a change of an event sends of its dates", () => {
  test("a change of the title alone sends no date", async () => {
    const { app } = await atEvent("bread-class", "/edit", "Change the event");
    const patch = app.fake.script("PATCH /events/{id}");
    const title = main().getByRole("textbox", { name: "Title" });
    await app.user.type(title, "!");
    await app.user.click(main().getByRole("button", { name: "Save" }));
    const sent = await patch.arrived();
    expect(sent.body.title).toBe("Sourdough class!");
    expect(sent.body.starts_at).toBeUndefined();
    expect(sent.body.ends_at).toBeUndefined();
  });

  test("a change of the end alone keeps the start's moment: the second 1:30 of a fall-back night stays put", async () => {
    const at = fallBackNight();
    const on = fakeApp();
    const id = on.fake.events.id("bread-class");
    on.fake.script("GET /events/{id}", {
      change: (event) => ({
        ...event,
        time_zone: "America/New_York",
        starts_at: new Date(at).toISOString(),
        ends_at: new Date(at + 90 * 60_000).toISOString(),
      }),
    });
    const app = await renderApp(`/events/${id}/edit`, { on });
    await main().findByRole("heading", { name: "Change the event", level: 2 });
    // The clock reads 1:30 for 5:30 and for 6:30 UTC; the event is at the second.
    expect(dateField("Starts").value).toBe(local(at, "America/New_York"));
    expect(dateField("Starts").value.endsWith("T01:30")).toBe(true);
    const patch = app.fake.script("PATCH /events/{id}");
    setDate(dateField("Ends"), local(at + 150 * 60_000, "America/New_York"));
    await app.user.click(main().getByRole("button", { name: "Save" }));
    const sent = await patch.arrived();
    expect(Date.parse(sent.body.starts_at ?? "")).toBe(at);
    expect(Date.parse(sent.body.ends_at ?? "")).toBe(at + 150 * 60_000);
  });
});

describe("the voice chat room as a place", () => {
  const voice = /^In a voice chat room/;

  test("is not offered to an organizer who may not host rooms", async () => {
    const app = await renderApp("/events/new", { voiceStarters: "permitted" });
    await main().findByRole("checkbox", { name: "In person" });
    expect(main().queryAllByRole("checkbox", { name: voice })).toHaveLength(0);
    expect(app.path()).toBe("/events/new");
  });

  test("is offered, chosen, to a co-organizer of an event that has one, whatever their own right", async () => {
    await atEvent("voice-co", "/edit", "Change the event", {
      ...withEvents(voiceCoorganized),
      voiceStarters: "permitted",
    });
    const room = main().getByRole("checkbox", { name: voice });
    expect((room as HTMLInputElement).checked).toBe(true);
  });
});

describe("the places in person and online", () => {
  test("each shows its fields under its checkbox once it is chosen, and takes them away when it is not", async () => {
    const app = await renderApp("/events/new");
    const inPerson = await main().findByRole("checkbox", { name: "In person" });
    expect(main().queryAllByRole("textbox", { name: "Address" })).toHaveLength(0);
    await app.user.click(inPerson);
    expect(main().getByRole("textbox", { name: "Name" })).not.toBeNull();
    expect(main().getByRole("textbox", { name: "Address" })).not.toBeNull();
    expect(main().getByRole("textbox", { name: /^Map link/ })).not.toBeNull();
    await app.user.click(inPerson);
    expect(main().queryAllByRole("textbox", { name: "Address" })).toHaveLength(0);
    await app.user.click(main().getByRole("checkbox", { name: "Online" }));
    expect(main().getByRole("textbox", { name: "Name" })).not.toBeNull();
    expect(main().getByRole("textbox", { name: "Link" })).not.toBeNull();
  });

  test("an online place's link that is not http or https is marked under it, which takes the focus, and nothing is sent", async () => {
    const on = fakeApp();
    const create = on.fake.script("POST /events");
    const app = await renderApp("/events/new", { on });
    await app.user.type(await main().findByRole("textbox", { name: "Title" }), "Tea");
    await app.user.click(main().getByRole("checkbox", { name: "Online" }));
    await app.user.type(main().getByRole("textbox", { name: "Name" }), "The stream");
    const link = main().getByRole("textbox", { name: "Link" });
    await app.user.type(link, "ftp://live.example/tea");
    await app.user.click(main().getByRole("button", { name: "Create the event" }));
    await waitFor(() => {
      expect(marked(link)).toBe(true);
    });
    expect(errorOf(link)).not.toBe("");
    expect(document.activeElement === link).toBe(true);
    expect(create.seen).toHaveLength(0);
  });
});

describe("a refusal of the server that names a field", () => {
  const cases: readonly (readonly [
    string,
    (app: Awaited<ReturnType<typeof renderApp>>) => Promise<void>,
    () => HTMLElement,
  ])[] = [
    ["title", () => Promise.resolve(), () => main().getByRole("textbox", { name: "Title" })],
    ["time_zone", () => Promise.resolve(), () => main().getByRole("combobox", { name: "Time zone" })],
    ["visibility", () => Promise.resolve(), () => main().getByRole("radiogroup", { name: "Who can see the event" })],
    [
      "candidates",
      async (app) => {
        await app.user.click(main().getByRole("radio", { name: /^Poll for a date/ }));
      },
      () => main().getByRole("group", { name: "When" }),
    ],
    [
      "place_disclosure",
      async (app) => {
        await app.user.click(main().getByRole("checkbox", { name: "In person" }));
        await app.user.type(main().getByRole("textbox", { name: "Name" }), "The park");
        await app.user.type(main().getByRole("textbox", { name: "Address" }), "By the pond");
      },
      () => main().getByRole("radiogroup", { name: /^Who sees the places/ }),
    ],
  ];
  for (const [field, before, part] of cases)
    test(`of ${field} is said under its part, which takes the focus`, async () => {
      const on = fakeApp();
      on.fake.script("POST /events", {
        times: 1,
        fail: { type: "about:blank", title: "This value is not valid", status: 422, field },
      });
      const app = await renderApp("/events/new", { on });
      await app.user.type(await main().findByRole("textbox", { name: "Title" }), "Tea");
      await before(app);
      await app.user.click(main().getByRole("button", { name: "Create the event" }));
      await waitFor(() => {
        expect(errorOf(part())).not.toBe("");
      });
      await waitFor(() => {
        expect(document.activeElement === part()).toBe(true);
      });
      expect(app.path()).toBe("/events/new");
    });
});

test("a co-organizer's menu of the event offers no Cancel: only the organizer cancels", async () => {
  const { app } = await atEvent("bread-class");
  await app.user.click(main().getByRole("button", { name: "Manage the event" }));
  await screen.findByRole("menuitem", { name: "Change the event" });
  expect(screen.queryAllByRole("menuitem", { name: /^Cancel the event/ })).toHaveLength(0);
});

test("an approved attendee's menu removes the approval: the row waits for approval again, with Approve", async () => {
  const { app } = await atEvent("bread-class");
  await app.user.click(await main().findByRole("button", { name: "Actions for Hana 花" }));
  await app.user.click(await screen.findByRole("menuitem", { name: "Remove the approval" }));
  expect(await main().findByRole("button", { name: "Approve Hana 花" })).not.toBeNull();
  expect(main().getAllByText("Waiting for approval")).toHaveLength(3);
});

describe("the poll's dates on the event's page", () => {
  test("a date is withdrawn from its menu, asked first, and leaves the table", async () => {
    const { app } = await atEvent("photo-walk");
    const table = await main().findByRole("table", { name: "Date poll" });
    await waitFor(() => {
      expect(within(table).getAllByRole("rowheader")).toHaveLength(3);
    });
    const [first] = within(table).getAllByRole("button", { name: /^Options for / });
    if (first === undefined) throw new Error("test: no date's menu");
    await app.user.click(first);
    await app.user.click(await screen.findByRole("menuitem", { name: "Withdraw this date…" }));
    const dialog = await screen.findByRole("alertdialog");
    await app.user.click(within(dialog).getByRole("button", { name: "Withdraw" }));
    await waitFor(() => {
      expect(within(main().getByRole("table", { name: "Date poll" })).getAllByRole("rowheader")).toHaveLength(2);
    });
  });

  test("the last date cannot be withdrawn, and its menu says why", async () => {
    const { app } = await atEvent("one-date", "", undefined, withEvents(oneDate));
    await app.user.click(await main().findByRole("button", { name: /^Options for / }));
    const withdraw = await screen.findByRole("menuitem", { name: /^Withdraw this date…/ });
    expect(withdraw.getAttribute("aria-disabled")).toBe("true");
    expect(words(withdraw)).toContain("A poll keeps at least one date.");
  });

  test("a confirmation the server refuses, as another organizer confirmed meanwhile, reads the event again", async () => {
    const { app } = await atEvent("photo-walk");
    const table = await main().findByRole("table", { name: "Date poll" });
    const confirm = app.fake.script("POST /events/{id}/candidates/{candidate_id}/confirm", {
      times: 1,
      hold: "response",
      fail: { type: "/api/v1/problems/event-scheduled", title: "The date of the event is confirmed", status: 409 },
    });
    const [first] = within(table).getAllByRole("button", { name: /^Options for / });
    if (first === undefined) throw new Error("test: no date's menu");
    await app.user.click(first);
    await app.user.click(await screen.findByRole("menuitem", { name: "Confirm this date…" }));
    const dialog = await screen.findByRole("alertdialog");
    await app.user.click(within(dialog).getByRole("button", { name: "Confirm" }));
    await confirm.served();
    const read = app.fake.script("GET /events/{id}");
    confirm.release();
    await read.arrived();
    // The event the server holds now has its date: the reader answers it.
    expect(await main().findByRole("radiogroup", { name: "Your answer" })).not.toBeNull();
  });
});

describe("the invitations and the co-organizers", () => {
  for (const [path, heading, list] of [
    ["/invitations", "Invitations", "Invited"],
    ["/coorganizers", "Co-organizers", /^Co-organizers \(\d of 3\)$/],
  ] as const)
    test(`${heading}: the list comes first, then the search`, async () => {
      await atEvent(path === "/invitations" ? "bread-class" : "photo-walk", path, heading);
      const group = main().getByRole("group", { name: list });
      const search = main().getByRole("searchbox", { name: "Name or handle" });
      expect(group.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

  test("a revocation asks first, and says the person cannot be invited again", async () => {
    const { app } = await atEvent("bread-class", "/invitations", "Invitations");
    await app.user.click(await main().findByRole("button", { name: "Actions for Mio 澪" }));
    await app.user.click(await screen.findByRole("menuitem", { name: "Revoke the invitation…" }));
    const dialog = await screen.findByRole("alertdialog", { name: /Mio 澪/ });
    const described = (dialog.getAttribute("aria-describedby") ?? "")
      .split(" ")
      .map((id) => words(document.getElementById(id) ?? document.createElement("p")))
      .join(" ");
    expect(described).toMatch(/Mio 澪 cannot be invited to it again/);
    // The head's glyph bears the same name as the button with words.
    const keep = within(dialog)
      .getAllByRole("button", { name: "Keep the invitation" })
      .find((b) => words(b) === "Keep the invitation");
    if (keep === undefined) throw new Error("test: no Keep the invitation with words");
    await app.user.click(keep);
    await waitFor(() => {
      expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
    });
    expect(main().getByRole("button", { name: "Actions for Mio 澪" })).not.toBeNull();
  });

  test("the organizer names a co-organizer found by name, the focus back on the search", async () => {
    const { app, on } = await atEvent("photo-walk", "/coorganizers", "Co-organizers");
    const search = main().getByRole("searchbox", { name: "Name or handle" });
    await app.user.type(search, "ren");
    const found = await main().findByRole("list", { name: "People found" });
    await app.user.click(await within(found).findByRole("button", { name: "Name co-organizer" }));
    await waitFor(() => {
      expect(on.fake.events.shown("photo-walk")?.coorganizers.map((a) => a.username)).toEqual(["hana", "ren"]);
    });
    await within(main().getByRole("group", { name: "Co-organizers (2 of 3)" })).findByText("Ren 蓮");
    await waitFor(() => {
      expect(document.activeElement === search).toBe(true);
    });
  });

  test("an event with three co-organizers offers no search: it has as many as it can", async () => {
    await atEvent("three-co", "/coorganizers", "Co-organizers", withEvents(fullyCoorganized));
    expect(main().getByRole("group", { name: "Co-organizers (3 of 3)" })).not.toBeNull();
    expect(main().queryAllByRole("searchbox")).toHaveLength(0);
  });
});

describe("an attendee who organizes the event (ADR-0161 §6, #3359)", () => {
  /** The rows of those going and maybe, each by the handle it shows. */
  const attendeeRow = (handle: string): HTMLElement => {
    const rows = main()
      .getAllByRole("list", { name: /^(Going|Maybe)/ })
      .flatMap((list) => within(list).getAllByRole("listitem"))
      .filter((row) => words(row).includes(`@${handle}@`));
    const [row] = rows;
    if (row === undefined || rows.length !== 1) throw new Error(`test: ${rows.length} rows of @${handle}`);
    return row;
  };

  /** What the end of the row of `handle` carries of the approval: its words and its buttons. */
  const approvalOf = (handle: string) => {
    const row = attendeeRow(handle);
    return {
      waiting: words(row).includes("Waiting for approval"),
      buttons: within(row)
        .queryAllByRole("button")
        .map((b) => b.getAttribute("aria-label") ?? words(b))
        .filter((name) => name.startsWith("Approve") || name.startsWith("Actions for")),
    };
  };

  const none = { waiting: false, buttons: [] };

  /** The class the reader co-organizes, with its organizer going and the reader maybe (the story OrganizersAttending). */
  const organizersAttending: FakeOptions = {
    seed: {
      ...scenario,
      events: scenario.events.map((plan) =>
        plan.key === "bread-class"
          ? {
              ...plan,
              attendees: [
                { by: "sam", answer: "going" as const },
                ...(plan.attendees ?? []),
                { by: "yuki", answer: "maybe" as const },
              ],
            }
          : plan,
      ),
    },
  };

  test("while the event is open, neither the organizer nor the co-organizer waits, nor carries Approve or its menu", async () => {
    await atEvent("bread-class", "", undefined, organizersAttending);
    await waitFor(() => {
      expect(attendeeRow("sam")).toBeDefined();
    });
    expect(approvalOf("sam")).toEqual(none);
    expect(approvalOf("yuki")).toEqual(none);
    // The others keep theirs: the approved one its mark and menu, the one waiting Approve.
    expect(approvalOf("hana")).toEqual({ waiting: false, buttons: ["Actions for Hana 花"] });
    expect(approvalOf("layla")).toEqual({ waiting: true, buttons: ["Approve ليلى Layla"] });
  });

  /** An event the reader organizes and sam co-organizes, both attending, beside one attendee approved and one not. */
  const organized = (key: string, title: string, more: Partial<FakeEventPlan>): FakeEventPlan => ({
    key,
    organizer: "yuki",
    coorganizers: ["sam"],
    title,
    description: "",
    timeZone: "UTC",
    inDays: 3,
    hour: 12,
    hours: 1,
    attendees: [
      { by: "yuki", answer: "going" },
      { by: "sam", answer: "maybe" },
      { by: "hana", answer: "going", approved: true },
      { by: "layla", answer: "going" },
    ],
    ...more,
  });

  for (const [what, plan] of [
    ["cancelled", organized("organized-cancelled", "Kite day", { cancelled: { reason: "organizer" } })],
    ["ended", organized("organized-ended", "Picnic done", { inDays: 0, hour: 0, startsInMinutes: -2 * 24 * 60 })],
  ] as const)
    test(`once the event is ${what}, neither the organizer nor the co-organizer waits, nor carries Approve or its menu`, async () => {
      await atEvent(plan.key, "", plan.title, withEvents(plan));
      await waitFor(() => {
        expect(attendeeRow("yuki")).toBeDefined();
      });
      expect(approvalOf("yuki")).toEqual(none);
      expect(approvalOf("sam")).toEqual(none);
      // The approved one keeps their mark.
      expect(words(attendeeRow("hana"))).toContain("Approved");
    });

  test("an approval refused as the attendee now organizes reads the attendees again and says it did not finish", async () => {
    const { app } = await atEvent("bread-class");
    const approve = await main().findByRole("button", { name: "Approve ليلى Layla" });
    // Meanwhile the organizer named layla a co-organizer: the server refuses, and its attendees say she organizes.
    app.fake.script("PUT /events/{id}/attendees/{account_id}/approval", {
      times: 1,
      fail: {
        status: 422,
        type: "/api/v1/problems/event-attendee-organizes",
        title: "The attendee organizes the event",
      },
    });
    const reads = app.fake.script("GET /events/{id}/attendees", {
      change: (page) => ({
        ...page,
        items: page.items.map((a) => (a.account.username === "layla" ? { ...a, organizes: true, approved: true } : a)),
      }),
    });
    await app.user.click(approve);
    await reads.arrived();
    await says("Approving ليلى Layla did not finish.");
    await waitFor(() => {
      expect(approvalOf("layla")).toEqual(none);
    });
  });
});
