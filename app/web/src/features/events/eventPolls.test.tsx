// SPDX-License-Identifier: AGPL-3.0-only
// The date polls and voice chat rooms of events through the whole app (web-client/README.md T4;
// web-client/screens/event.md, event-editor.md): the real router and QueryClient, with MSW as the server. The poll is a
// table whose cells a screen reader reads with their date and column; an answer saves at once; everyone reads who
// answered; the organizer sets the date from a candidate. The voice place leads into its room once it opened. The
// editor creates an event with candidate dates and a voice chat room.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import {
  instanceFixture,
  otherAccountFixture,
  preferencesFixture,
  type Schemas,
  viewerFixture,
} from "../../test/fixtures";
import { apiBase, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { roomRefetchDelay } from "./hooks/useEventPage";
import { eventFixture, eventId } from "./test/handlers";

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-11-10T00:00:00Z") });
});

afterEach(() => {
  vi.useRealTimers();
});

const roomId = "0192f0c4-7a8e-7000-8000-0000000000f1";

function candidateFixture(overrides: Partial<Schemas["EventCandidate"]> = {}): Schemas["EventCandidate"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000000c1",
    starts_at: "2026-12-05T12:00:00Z",
    ends_at: "2026-12-05T14:00:00Z",
    yes_count: 2,
    maybe_count: 1,
    no_count: 0,
    confirmed: false,
    ...overrides,
  };
}

const saturday = candidateFixture();
const sunday = candidateFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000000c2",
  starts_at: "2026-12-06T12:00:00Z",
  ends_at: "2026-12-06T14:00:00Z",
  yes_count: 0,
  maybe_count: 0,
  no_count: 3,
});

const polling = eventFixture({
  state: "unscheduled",
  starts_at: saturday.starts_at,
  ends_at: sunday.ends_at,
});

/** The row of the poll's table whose row header names `day` (such as "Dec 5"). */
function rowOf(table: HTMLElement, day: RegExp): HTMLElement {
  const row = within(table)
    .getAllByRole("row")
    .find((candidate) => within(candidate).queryByRole("rowheader", { name: day }) !== null);
  if (row === undefined) throw new Error("no row");
  return row;
}

describe("the date poll of an event", () => {
  test("is a table of the candidates with headers, takes an answer, and lists who answered", async () => {
    let candidates = [saturday, sunday];
    let voted: unknown;
    server.use(
      resumableSession(),
      http.get(`${apiBase}/events/:id`, () => HttpResponse.json(polling)),
      http.get(`${apiBase}/events/:id/candidates`, () => HttpResponse.json({ items: candidates })),
      http.put(`${apiBase}/events/:id/candidates/:candidateId/vote`, async ({ request }) => {
        voted = await request.json();
        candidates = [{ ...saturday, yes_count: 3, vote: "yes" }, sunday];
        return HttpResponse.json(candidates[0]);
      }),
      http.get(`${apiBase}/events/:id/candidates/:candidateId/votes`, () =>
        HttpResponse.json({
          items: [{ account: otherAccountFixture, vote: "yes" }],
        } satisfies Schemas["EventBallotPage"]),
      ),
    );
    const { user } = await renderApp({ path: `/events/${eventId}` });
    expect(await screen.findByRole("heading", { level: 1, name: "Night picnic" })).toBeInTheDocument();
    expect(screen.getByText("Date to be decided")).toBeInTheDocument();
    const poll = screen.getByRole("region", { name: "Date poll" });
    const table = await within(poll).findByRole("table", { name: "Candidate dates and their answers" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((header) => header.textContent),
    ).toEqual(["Date", "Can go", "Maybe", "Can't go", "Your answer", "Actions"]);
    const first = rowOf(table, /Dec 5, 2026/u);
    expect(
      within(first)
        .getAllByRole("cell")
        .map((cell) => cell.textContent)
        .slice(0, 3),
    ).toEqual(["2", "1", "0"]);
    expect(within(rowOf(table, /Dec 6, 2026/u)).getAllByRole("cell")[2]).toHaveTextContent("3");
    // Nobody may answer going or not before a date is set.
    expect(screen.getByRole("radiogroup", { name: "Your answer" })).toHaveAttribute("aria-disabled", "true");

    const answer = within(first).getByRole("radiogroup", { name: /^Your answer for Sat, Dec 5, 2026/u });
    await user.click(within(answer).getByRole("radio", { name: "Can go" }));
    await waitFor(() => {
      expect(voted).toEqual({ vote: "yes" });
    });
    await waitFor(() => {
      expect(within(rowOf(table, /Dec 5, 2026/u)).getAllByRole("cell")[0]).toHaveTextContent("3");
    });
    expect(within(answer).getByRole("radio", { name: "Can go" })).toBeChecked();

    await user.click(within(first).getByRole("button", { name: /^Who answered Sat, Dec 5, 2026/u }));
    const dialog = await screen.findByRole("dialog", { name: /^Who answered Sat, Dec 5, 2026/u });
    const list = await within(dialog).findByRole("list", { name: "Who answered" });
    expect(within(list).getByText("Bob")).toBeInTheDocument();
    expect(within(list).getByText("Can go")).toBeInTheDocument();
    // A guest neither sets the date nor adds a candidate.
    expect(within(first).queryByRole("button", { name: /^Set this date/u })).not.toBeInTheDocument();
  });

  test("lets the organizer set the date from a candidate, which ends the poll", async () => {
    const organizing = { ...polling, organizer: viewerFixture, viewer: { ...polling.viewer, organizer: true } };
    let confirmed = false;
    let added: unknown;
    server.use(
      resumableSession(),
      http.post(`${apiBase}/events/:id/candidates`, async ({ request }) => {
        added = await request.json();
        return HttpResponse.json(
          candidateFixture({
            id: "0192f0c4-7a8e-7000-8000-0000000000c3",
            starts_at: "2026-12-07T12:00:00Z",
            ends_at: "2026-12-07T14:00:00Z",
          }),
          { status: 201 },
        );
      }),
      http.get(`${apiBase}/events/:id`, () =>
        HttpResponse.json(
          confirmed
            ? { ...organizing, state: "scheduled", starts_at: saturday.starts_at, ends_at: saturday.ends_at }
            : organizing,
        ),
      ),
      http.get(`${apiBase}/events/:id/candidates`, () =>
        HttpResponse.json({ items: [{ ...saturday, confirmed }, sunday] }),
      ),
      http.post(`${apiBase}/events/:id/candidates/:candidateId/confirm`, ({ params }) => {
        confirmed = params["candidateId"] === saturday.id;
        return HttpResponse.json({
          ...organizing,
          state: "scheduled",
          starts_at: saturday.starts_at,
          ends_at: saturday.ends_at,
        });
      }),
    );
    const { user } = await renderApp({ path: `/events/${eventId}` });
    const table = await screen.findByRole("table", { name: "Candidate dates and their answers" });
    // A candidate is typed as the wall clock of the event's time zone (Tokyo's), as the editor's dates are.
    const add = screen.getByRole("group", { name: "Add a candidate date" });
    await user.type(within(add).getByLabelText(/^Starts/u), "2026-12-07T21:00");
    await user.type(within(add).getByLabelText(/^Ends/u), "2026-12-07T23:00");
    await user.click(within(add).getByRole("button", { name: "Add date" }));
    expect(await screen.findByText(/^Added Mon, Dec 7, 2026/u)).toBeInTheDocument();
    expect(added).toEqual({ starts_at: "2026-12-07T12:00:00.000Z", ends_at: "2026-12-07T14:00:00.000Z" });
    expect(within(add).getByLabelText(/^Starts/u)).toHaveValue("");

    await user.click(within(rowOf(table, /Dec 5, 2026/u)).getByRole("button", { name: /^Set this date: Sat, Dec 5/u }));
    const dialog = await screen.findByRole("alertdialog", { name: /^Set the date to Sat, Dec 5, 2026/u });
    await user.click(within(dialog).getByRole("button", { name: "Set date" }));
    expect(await screen.findByText(/^The date is set to Sat, Dec 5, 2026/u)).toBeInTheDocument();
    expect(confirmed).toBe(true);
    await waitFor(() => {
      expect(screen.queryByRole("region", { name: "Date poll" })).not.toBeInTheDocument();
    });
    expect(screen.queryByText("Date to be decided")).not.toBeInTheDocument();
  });
});

describe("the voice chat place of an event", () => {
  const started = { starts_at: "2026-11-09T23:55:00Z", ends_at: "2026-11-10T02:00:00Z" };

  test("leads into the room once it opened at the start", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/events/:id`, () =>
        HttpResponse.json(
          eventFixture({
            ...started,
            places: [{ kind: "voice", speakers: "following", room_id: roomId }],
            hidden_places: [],
          }),
        ),
      ),
    );
    await renderApp({ path: `/events/${eventId}` });
    const where = await screen.findByRole("region", { name: "Where" });
    expect(within(where).getByText("The voice chat room is open.")).toBeInTheDocument();
    expect(within(where).getByRole("link", { name: "Join the voice chat room" })).toHaveAttribute(
      "href",
      `/voice/${roomId}`,
    );
    expect(within(where).getByText("Who can speak without asking: People the host follows")).toBeInTheDocument();
  });

  test("says that the room is opening after the start, and why it could not open", async () => {
    let place: Schemas["EventPlace"] = { kind: "voice" };
    server.use(
      resumableSession(),
      http.get(`${apiBase}/events/:id`, () =>
        HttpResponse.json(eventFixture({ ...started, places: [place], hidden_places: [] })),
      ),
    );
    const { unmount } = await renderApp({ path: `/events/${eventId}` });
    const where = await screen.findByRole("region", { name: "Where" });
    expect(within(where).getByText("The voice chat room is opening…")).toBeInTheDocument();
    expect(within(where).queryByRole("link", { name: "Join the voice chat room" })).not.toBeInTheDocument();
    unmount();

    place = { kind: "voice", room_unavailable: "hosting_elsewhere" };
    await renderApp({ path: `/events/${eventId}` });
    expect(
      await screen.findByText("The voice chat room could not open because the organizer was hosting another room."),
    ).toBeInTheDocument();
  });

  test("is asked for again at the start, then every few seconds until the room opened", () => {
    const voice = eventFixture({ places: [{ kind: "voice" }] });
    const start = Date.parse(voice.starts_at);
    expect(roomRefetchDelay(voice, start - 5_000)).toBe(6_000);
    expect(roomRefetchDelay(voice, start - 10 * 24 * 60 * 60_000)).toBe(60 * 60_000);
    expect(roomRefetchDelay(voice, start + 1_000)).toBe(10_000);
    expect(roomRefetchDelay(voice, start + 31 * 60_000)).toBe(false);
    expect(roomRefetchDelay({ ...voice, places: [{ kind: "voice", room_id: roomId }] }, start + 1_000)).toBe(false);
    expect(roomRefetchDelay(eventFixture(), start + 1_000)).toBe(false);
    expect(roomRefetchDelay({ ...voice, state: "cancelled" }, start + 1_000)).toBe(false);
  });
});

describe("the editor of an event", () => {
  test("creates an event with candidate dates and a voice chat room", async () => {
    let created: Schemas["EventCreate"] | undefined;
    server.use(
      resumableSession(),
      // The user's time zone is Tokyo's, whatever the browser's: the candidates are typed as its wall clock.
      http.get(`${apiBase}/preferences`, () =>
        HttpResponse.json({ ...preferencesFixture, time_zone: "Asia/Tokyo" } satisfies Schemas["Preferences"]),
      ),
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({ ...instanceFixture, voice_enabled: true, voice_room_starters: "everyone" }),
      ),
      http.post(`${apiBase}/events`, async ({ request }) => {
        created = (await request.json()) as Schemas["EventCreate"];
        return HttpResponse.json(polling, { status: 201 });
      }),
    );
    const { user } = await renderApp({ path: "/events/new" });
    expect(await screen.findByRole("heading", { level: 1, name: "New event" })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: /^Title/u }), "Night picnic");
    await user.click(screen.getByRole("radio", { name: "Ask people to answer candidate dates" }));
    // The start and the end of the event give way to those of each candidate, two to begin with.
    expect(screen.getAllByLabelText(/^Starts/u)).toHaveLength(2);

    const first = screen.getByRole("group", { name: "Candidate 1" });
    await user.type(within(first).getByLabelText(/^Starts/u), "2026-12-05T21:00");
    await user.type(within(first).getByLabelText(/^Ends/u), "2026-12-05T23:00");
    // The second candidate repeats the first, which is refused before anything is sent.
    const second = screen.getByRole("group", { name: "Candidate 2" });
    await user.type(within(second).getByLabelText(/^Starts/u), "2026-12-05T21:00");
    await user.type(within(second).getByLabelText(/^Ends/u), "2026-12-05T23:00");

    await user.click(await screen.findByRole("checkbox", { name: /^Voice chat room/u }));
    await user.click(
      within(screen.getByRole("radiogroup", { name: "Who can speak without asking" })).getByRole("radio", {
        name: "People the host follows",
      }),
    );
    await user.click(screen.getByRole("button", { name: "Create event" }));
    expect(await within(second).findByText("This date is already a candidate.")).toBeInTheDocument();
    expect(created).toBeUndefined();

    await user.clear(within(second).getByLabelText(/^Starts/u));
    await user.type(within(second).getByLabelText(/^Starts/u), "2026-12-06T21:00");
    await user.clear(within(second).getByLabelText(/^Ends/u));
    await user.type(within(second).getByLabelText(/^Ends/u), "2026-12-06T23:00");
    await user.click(screen.getByRole("button", { name: "Create event" }));
    await waitFor(() => {
      expect(created).toBeDefined();
    });
    expect(created).toEqual({
      title: "Night picnic",
      description: "",
      visibility: "public",
      time_zone: "Asia/Tokyo",
      place_disclosure: "approved",
      capacity: 0,
      places: [{ kind: "voice", speakers: "following" }],
      candidates: [
        { starts_at: "2026-12-05T12:00:00.000Z", ends_at: "2026-12-05T14:00:00.000Z" },
        { starts_at: "2026-12-06T12:00:00.000Z", ends_at: "2026-12-06T14:00:00.000Z" },
      ],
    });
  });
});
