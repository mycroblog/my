// SPDX-License-Identifier: AGPL-3.0-only

// An event for the organizer on the real server (#3238; FR-940, FR-942, FR-943, FR-948, FR-950): an organizer creates
// an event from the form with a date poll, a voice chat room and a place in person; a second person answers the poll;
// the organizer confirms a date, the second person answers going, and their place in person stays hidden until the
// organizer approves them, after which they see it; and the organizer cancels the event. What the screens show in each
// state is the stories' and the route tests' on the fake (web/app, events.stories.tsx, events-organize.dom.test.tsx).

import type { Page } from "@playwright/test";
import type { Person } from "../src/scenario";
import { EventForm, EventPage, Frame, SignIn } from "./screens";
import { expect, test } from "./test";

/** Signs in as `p` on `page` and waits for the home screen (a fresh person follows no one, so it has no feed). */
async function signInOn(page: Page, p: Person): Promise<void> {
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(p.email, p.password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
}

test("an organizer creates a polled event with a voice room, confirms a date, approves an attendee and cancels it", async ({
  page,
  browser,
  baseURL,
  fresh,
}) => {
  // The organizer may host voice chat rooms (the server's default lets the roles with `start_voice_rooms`, the admin's).
  const organizer = await fresh.person({ admin: true });
  const attendee = await fresh.person();
  const title = `Night walk ${attendee.username.slice(-6)}`;
  const place = `Gate ${attendee.username.slice(-6)}`;

  await signInOn(page, organizer);
  const form = new EventForm(page);
  await form.openFromList();
  await form.title.fill(title);
  await form.choosePoll();
  await form.voice.click();
  await expect(form.voice).toBeChecked();
  await form.inPerson.click();
  await expect(form.inPerson).toBeChecked();
  // The poll's two dates, the evenings three and four days ahead.
  const [first, second] = fresh.evenings([3, 4]);
  if (first === undefined || second === undefined) throw new Error("e2e: no two evenings");
  await form.pollDatesAt([first, second]);
  await form.placeName.fill(place);
  await form.address.fill("By the old clock tower");
  await form.submit.click();
  // The event's page takes the form's place.
  await expect(page).toHaveURL(/\/events\/(?!new$)[^/]+$/);
  const id = new URL(page.url()).pathname.split("/").at(-1) ?? "";
  const organizing = new EventPage(page, id);
  await expect(organizing.title(title)).toBeVisible();
  await expect(organizing.poll).toBeVisible();

  // The second person, in a browser of their own, answers the poll's first date.
  const other = await browser.newContext({
    ...(baseURL === undefined ? {} : { baseURL }),
    extraHTTPHeaders: { "X-Forwarded-For": "198.19.250.3" },
  });
  const attendeePage = await other.newPage();
  await signInOn(attendeePage, attendee);
  const attending = new EventPage(attendeePage, id);
  await attending.open();
  await attending.answerDate(first, "Can go");

  // The organizer confirms that date, asked first: the event now has its date, and answers to it.
  await organizing.open();
  await organizing.confirmDate(first);
  await expect(organizing.answers).toBeVisible();
  // The voice chat room the form chose is the event's place, to open at its start.
  await expect(page.getByRole("main").getByText(/^Opens when the event starts$/)).toBeVisible();

  // The second person answers going; the place in person is hidden from them until an organizer approves them.
  await attending.open();
  await attending.answer("Going");
  // The page says the place is held back, so that its absence is the server's and not a page still drawing.
  await expect(
    attendeePage.getByRole("main").getByText(/^In person: the place is shown to the attendees the organizers approve$/),
  ).toBeVisible();
  await expect(attendeePage.getByRole("main").getByText(place, { exact: true })).toHaveCount(0);

  await organizing.open();
  await organizing.approve(attendee.displayName);

  await attending.open();
  await expect(attendeePage.getByRole("main").getByText(place, { exact: true })).toBeVisible();

  // The organizer cancels the event, asked first; it stays, marked cancelled.
  await organizing.open();
  await organizing.cancel("The clock tower is closed.");
  await expect(
    page.getByRole("main").getByText("This event was cancelled by its organizer.", { exact: true }),
  ).toBeVisible();
  await attending.open();
  await expect(attendeePage.getByRole("main").getByText("The clock tower is closed.", { exact: true })).toBeVisible();
  await other.close();
});
