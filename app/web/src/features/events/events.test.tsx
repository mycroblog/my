// SPDX-License-Identifier: AGPL-3.0-only
// The events through the whole app (web-client/README.md T4; web-client/screens/events.md, event.md,
// event-editor.md): the real router and QueryClient, with MSW as the server. The list shows an event in the viewer's
// time zone with its own beside it; the page names a hidden place only by its kind, says where a link leaves to, and
// takes an answer; the organizer approves an attendee and cancels the event; the editor creates an event from the wall
// clock of its time zone.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { otherAccountFixture, preferencesFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { apiBase, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { eventFixture, eventId } from "./test/handlers";

const fsi = (text: string) => `⁨${text}⁩`;

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual. The fixture's event is three weeks ahead.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-11-10T00:00:00Z") });
});

afterEach(() => {
  vi.useRealTimers();
});

/** The user's time zone is Tokyo's, whatever the browser's. */
function inTokyo() {
  server.use(
    http.get(`${apiBase}/preferences`, () =>
      HttpResponse.json({ ...preferencesFixture, time_zone: "Asia/Tokyo" } satisfies Schemas["Preferences"]),
    ),
  );
}

describe("the list of events", () => {
  test("shows an event in the viewer's time zone, with the event's own time beside it", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/events`, () => HttpResponse.json({ items: [eventFixture({ going_count: 3 })] })),
    );
    await renderApp({ path: "/events" });
    expect(await screen.findByRole("heading", { level: 1, name: "Events" })).toBeInTheDocument();
    const list = await screen.findByRole("list", { name: "Upcoming events" });
    const item = within(list).getByRole("listitem");
    expect(within(item).getByRole("link", { name: "Night picnic" })).toHaveAttribute("href", `/events/${eventId}`);
    // The viewer is in UTC (the preferences of the fixtures); the event is in Tokyo, where it starts at 9 PM on Saturday.
    expect(within(item).getByText(/Dec 5, 2026/u)).toBeInTheDocument();
    expect(await within(item).findByText(/^Sat, 9:00\D+11:00\sPM \(Tokyo\)$/u)).toBeInTheDocument();
    expect(within(item).getByText("3 going")).toBeInTheDocument();
    expect(within(item).getByRole("link", { name: "Bob" })).toHaveAttribute("href", "/@bob");
  });

  test("shows only the viewer's time when the event is in the viewer's time zone", async () => {
    inTokyo();
    server.use(
      resumableSession(),
      http.get(`${apiBase}/events`, () => HttpResponse.json({ items: [eventFixture()] })),
    );
    await renderApp({ path: "/events" });
    const list = await screen.findByRole("list", { name: "Upcoming events" });
    expect(await within(list).findByText(/Sat, Dec 5, 2026, 9:00/u)).toBeInTheDocument();
    expect(within(list).queryByText(/\(Tokyo\)/u)).not.toBeInTheDocument();
  });

  test("says when no event is coming up", async () => {
    server.use(resumableSession());
    await renderApp({ path: "/events" });
    expect(await screen.findByRole("heading", { name: "No events coming up" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "New event" })).toHaveAttribute("href", "/events/new");
  });
});

describe("the page of an event", () => {
  test("names a hidden place by its kind only, says where a link leaves to, and takes an answer", async () => {
    let answered: unknown;
    server.use(
      resumableSession(),
      http.put(`${apiBase}/events/:id/rsvp`, async ({ request }) => {
        answered = await request.json();
        return HttpResponse.json(
          eventFixture({ going_count: 1, viewer: { ...eventFixture().viewer, answer: "going" } }),
        );
      }),
    );
    const { user } = await renderApp({ path: `/events/${eventId}` });
    expect(await screen.findByRole("heading", { level: 1, name: "Night picnic" })).toBeInTheDocument();
    expect(await screen.findByText(/\(Tokyo\)$/u)).toBeInTheDocument();

    const where = screen.getByRole("region", { name: "Where" });
    expect(
      within(where).getByText("The place to meet in person is shown to the attendees the organizers approve."),
    ).toBeInTheDocument();
    const stream = within(where).getByRole("link", { name: /^Live stream/u });
    expect(stream).toHaveAttribute("href", "https://stream.example/live");
    expect(stream).toHaveAttribute("target", "_blank");
    expect(stream).toHaveAttribute("rel", expect.stringContaining("nofollow"));
    expect(within(where).getByText("Opens stream.example, a site outside this server")).toBeInTheDocument();

    // The description's URL is a link that leaves the site too.
    const about = screen.getByRole("region", { name: "About" });
    expect(within(about).getByRole("link", { name: /^https:\/\/maps\.example\/park/u })).toHaveAttribute(
      "target",
      "_blank",
    );

    await user.click(screen.getByRole("radio", { name: "Going" }));
    await waitFor(() => {
      expect(answered).toEqual({ answer: "going" });
    });
    expect(await screen.findByText("Going", { selector: "span" })).toBeInTheDocument();
  });

  test("lets the organizer approve an attendee and cancel the event with a reason", async () => {
    const organizing = eventFixture({
      organizer: viewerFixture,
      places: [
        { kind: "url", name: "Live stream", url: "https://stream.example/live" },
        { kind: "physical", name: "Riverside park", address: "1-1 Kawabata" },
      ],
      hidden_places: [],
      going_count: 1,
      viewer: { organizer: true, coorganizer: false, invited: false, approved: false },
    });
    const attendee: Schemas["EventAttendee"] = { account: otherAccountFixture, answer: "going", approved: false };
    let approved = false;
    let cancelled: unknown;
    server.use(
      resumableSession(),
      http.get(`${apiBase}/events/:id`, () =>
        HttpResponse.json(
          cancelled === undefined
            ? organizing
            : {
                ...organizing,
                state: "cancelled",
                cancellation: { reason: "organizer", note: "Rain", cancelled_at: "2026-11-10T00:00:00Z" },
              },
        ),
      ),
      http.get(`${apiBase}/events/:id/attendees`, () => HttpResponse.json({ items: [{ ...attendee, approved }] })),
      http.put(`${apiBase}/events/:id/attendees/:accountId/approval`, () => {
        approved = true;
        return HttpResponse.json({ ...attendee, approved });
      }),
      http.post(`${apiBase}/events/:id/cancel`, async ({ request }) => {
        cancelled = await request.json();
        return HttpResponse.json({
          ...organizing,
          state: "cancelled",
          cancellation: { reason: "organizer", note: "Rain", cancelled_at: "2026-11-10T00:00:00Z" },
        });
      }),
    );
    const { user } = await renderApp({ path: `/events/${eventId}` });
    expect(await screen.findByRole("heading", { level: 1, name: "Night picnic" })).toBeInTheDocument();
    // The organizer sees every place, the address too.
    expect(screen.getByText("1-1 Kawabata")).toBeInTheDocument();

    const attendees = await screen.findByRole("list", { name: "Attendees" });
    await user.click(within(attendees).getByRole("button", { name: "Approve: Bob" }));
    expect(await screen.findByText(`Approved ${fsi("Bob")}.`)).toBeInTheDocument();
    expect(await within(attendees).findByText("Approved")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel event" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Cancel this event?" });
    await user.type(within(dialog).getByRole("textbox", { name: /^Reason/u }), "Rain");
    await user.click(within(dialog).getByRole("button", { name: "Cancel event" }));
    expect(await screen.findByText("The organizer cancelled this event.")).toBeInTheDocument();
    expect(screen.getByText("Reason: Rain")).toBeInTheDocument();
    expect(cancelled).toEqual({ note: "Rain" });
    // A cancelled event takes no answers.
    expect(screen.queryByRole("radiogroup", { name: "Your answer" })).not.toBeInTheDocument();
  });

  test("shows a moderator's cancellation with its note", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/events/:id`, () =>
        HttpResponse.json(
          eventFixture({
            state: "cancelled",
            cancellation: { reason: "moderator", note: "Against the rules", cancelled_at: "2026-11-09T00:00:00Z" },
          }),
        ),
      ),
    );
    await renderApp({ path: `/events/${eventId}` });
    expect(await screen.findByText("The moderators of this server cancelled this event.")).toBeInTheDocument();
    expect(screen.getByText("Reason: Against the rules")).toBeInTheDocument();
    expect(screen.getByText("Cancelled")).toBeInTheDocument();
  });

  test("says that an event the user does not see is not found", async () => {
    server.use(resumableSession());
    await renderApp({ path: "/events/0192f0c4-7a8e-7000-8000-0000000000ff" });
    expect(await screen.findByRole("heading", { level: 1, name: "Event not found" })).toBeInTheDocument();
  });
});

describe("the editor of an event", () => {
  test("creates an event from the wall clock of its time zone", async () => {
    inTokyo();
    let created: Schemas["EventCreate"] | undefined;
    server.use(
      resumableSession(),
      http.post(`${apiBase}/events`, async ({ request }) => {
        created = (await request.json()) as Schemas["EventCreate"];
        return HttpResponse.json(eventFixture({ title: created.title }), { status: 201 });
      }),
    );
    const { user } = await renderApp({ path: "/events/new" });
    expect(await screen.findByRole("heading", { level: 1, name: "New event" })).toBeInTheDocument();
    // The editor starts in the user's time zone setting (Tokyo), whatever the zone of the machine the test runs on.
    expect(await screen.findAllByText("In the time zone of Tokyo")).toHaveLength(2);
    await user.type(screen.getByRole("textbox", { name: /^Title/u }), "Night picnic");

    // A start without an end is refused before anything is sent.
    await user.click(screen.getByRole("button", { name: "Create event" }));
    expect(await screen.findByText("Enter the start.")).toBeInTheDocument();
    expect(created).toBeUndefined();

    await user.type(screen.getByLabelText(/^Starts/u), "2026-12-05T21:00");
    await user.type(screen.getByLabelText(/^Ends/u), "2026-12-05T23:00");
    await user.click(screen.getByRole("checkbox", { name: "Online link" }));
    await user.type(screen.getByRole("textbox", { name: /^Name of the link/u }), "Live stream");
    await user.type(screen.getByRole("textbox", { name: /^URL/u }), "https://stream.example/live");
    await user.click(screen.getByRole("button", { name: "Create event" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Night picnic" })).toBeInTheDocument();
    expect(created).toEqual({
      title: "Night picnic",
      description: "",
      visibility: "public",
      time_zone: "Asia/Tokyo",
      place_disclosure: "approved",
      capacity: 0,
      places: [{ kind: "url", name: "Live stream", url: "https://stream.example/live" }],
      starts_at: "2026-12-05T12:00:00.000Z",
      ends_at: "2026-12-05T14:00:00.000Z",
    });
  });
});
