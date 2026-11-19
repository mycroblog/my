// SPDX-License-Identifier: AGPL-3.0-only

// An event for the attendee on the real server (#3237, FR-943, FR-946, FR-948): a fresh person answers a fresh event
// going, sees themselves among those going, downloads its calendar file, which holds the event, and answers the dates
// of another's poll, each answer kept by the server. What the screens show in each state is the stories' and the
// route tests' on the fake (web/app, events.stories.tsx, events-routes.dom.test.tsx).

import { readFile } from "node:fs/promises";
import { EventPage } from "./screens";
import { expect, test } from "./test";

test("a reader answers an event going, is among those going, downloads its calendar file, and answers a poll's dates", async ({
  page,
  signIn,
  fresh,
}) => {
  const organizer = await fresh.person();
  const reader = await fresh.person();
  const event = await fresh.event(organizer, { title: "Picnic" });
  const poll = await fresh.event(organizer, { title: "Book club", candidates: [4, 5] });
  await signIn(reader);

  const screen = new EventPage(page, event.id);
  await screen.open();
  await expect(screen.title(event.title)).toBeVisible();
  await screen.answer("Going");
  await expect(screen.answers.getByRole("radio", { name: "Going", exact: true })).toBeChecked();
  await expect(screen.going(1).getByText(reader.displayName)).toBeVisible();

  // The answer is the server's: the page read anew says it.
  await screen.open();
  await expect(screen.answers.getByRole("radio", { name: "Going", exact: true })).toBeChecked();

  const download = await screen.downloadCalendar();
  expect(download.suggestedFilename()).toBe(`${event.id}.ics`);
  const ics = await readFile(await download.path(), "utf8");
  expect(ics).toContain("BEGIN:VEVENT");
  expect(ics).toContain(`SUMMARY:${event.title}`);

  const polled = new EventPage(page, poll.id);
  await polled.open();
  await expect(polled.title(poll.title)).toBeVisible();
  // While its date is polled, the poll is what the reader answers: no answer to the event, no calendar file.
  await expect(polled.answers).toHaveCount(0);
  const [first, second] = poll.candidates;
  if (first === undefined || second === undefined) throw new Error("e2e: the poll has no two dates");
  await polled.answerDate(first, "Can go");
  await polled.answerDate(second, "Maybe");
  await polled.open();
  await expect(polled.dateAnswers(first).getByRole("radio", { name: "Can go", exact: true })).toBeChecked();
  await expect(polled.dateAnswers(second).getByRole("radio", { name: "Maybe", exact: true })).toBeChecked();
});
