// SPDX-License-Identifier: AGPL-3.0-only

// An event's board on the real server (#3354, ADR-0161 §17): a reader going to a fresh event is told of the
// organizer's post on its board, follows the notification to the Board tab, writes a post there that the server keeps,
// and answers not going, which takes the board and its tabs away. What the board shows in each state, the composer it
// opens, the refusals and the focus are the route tests' on the fake (web/app, events-board.dom.test.tsx).

import { Composer, EventPage, Notifications } from "./screens";
import { expect, test } from "./test";

/** The marks that isolate a name in a sentence (U+2068, U+2069), which the screen may put around one. */
const [open, close] = [String.fromCodePoint(0x2068), String.fromCodePoint(0x2069)];

/** A name in a pattern, with its isolating marks or without. */
const named = (name: string) => `${open}?${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}${close}?`;

test("a reader follows the notification of a post to the board, writes on it, and answering not going takes the board away", async ({
  page,
  signIn,
  fresh,
}) => {
  const organizer = await fresh.person();
  const reader = await fresh.person();
  const event = await fresh.event(organizer, { title: "Picnic", boardWriters: "readers" });
  await fresh.answer(reader, event.id, "going");
  const posted = await fresh.boardPost(organizer, event.id, { text: "Meet at the north gate." });
  await signIn(reader);

  // The notification is the server's, made by a job once the post was written: the list read again shows it.
  const notifications = new Notifications(page);
  const line = notifications.feed.getByRole("article", {
    name: new RegExp(`^${named(organizer.displayName)} wrote on the board of ${named(event.title)}$`),
  });
  await expect(async () => {
    await page.goto("/notifications");
    await expect(line).toBeVisible();
  }).toPass();
  await expect(line.getByText(posted.text, { exact: true })).toBeVisible();
  await line.getByRole("link", { name: /^.* wrote on the board of .*$/ }).click();

  const screen = new EventPage(page, event.id);
  await expect(page).toHaveURL(new RegExp(`/events/${event.id}\\?tab=board$`));
  await expect(screen.tab(event.title, "Board")).toHaveAttribute("aria-selected", "true");
  await expect(screen.boardPost(posted.text)).toBeVisible();

  const dialog = await screen.writeOnTheBoard();
  await expect(dialog.getByText(`On the board of ${event.title}`, { exact: true })).toBeVisible();
  const composer = new Composer(dialog);
  const text = "I bring the blanket.";
  await composer.write(text);
  await composer.holds(text);
  await screen.postOnTheBoard(dialog);
  await expect(screen.boardPost(text)).toBeVisible();

  // The post is the server's, on the board alone: the page read anew shows it there.
  await page.goto(`/events/${event.id}?tab=board`);
  await expect(screen.boardPost(text)).toBeVisible();
  await expect(screen.boardPost(posted.text)).toBeVisible();

  await screen.tab(event.title, "Details").click();
  await screen.answer("Not going");
  await expect(screen.tabs(event.title)).toHaveCount(0);
  await expect(screen.board).toHaveCount(0);

  // The server no longer lets the reader read the board: the Board tab's address opens the event without it.
  await page.goto(`/events/${event.id}?tab=board`);
  await expect(screen.title(event.title)).toBeVisible();
  await expect(screen.tabs(event.title)).toHaveCount(0);
  await expect(screen.board).toHaveCount(0);
});
