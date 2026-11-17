// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages, as the client treats the server's answers (FR-212): a read that fails, a read answered late, a
// hide answered late, a conversation that is not there. The scenario's reader is Yuki, whose conversation with Sam is
// unread and comes after the one with Hana and Layla.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** The list of the reader's conversations. */
const list = () => main().findByRole("feed", { name: "Direct messages" });
/** The conversation with Sam in the list. */
const withSam = async () => within(await list()).findByRole("article", { name: "Sam Rivera" });
/** The messages of the conversation with Sam, on its screen; the names in its name are isolated from the words around. */
const messages = () => main().findByRole("feed", { name: /^Direct messages with ⁨?Sam Rivera⁩?$/ });
/** The navigation's link to the direct messages, named `name`: the place, then the count of the unread conversations. */
const place = (name: "Direct messages" | "Direct messages 1 unread") => screen.findByRole("link", { name });
/** What the fake's `newMessage` writes, after its mention of the reader. */
const said = (n: number) => `One more thing, number ${String(n)}.`;

/** Opens the conversation with Sam from its time, the link to it, and returns its messages. */
async function open({ user }: TestApp) {
  await user.click(within(await withSam()).getByRole("time"));
  return messages();
}

test("a read that fails is sent once per latest post: again for a newer post, and again on reopening", async () => {
  const on = fakeApp();
  const reads = on.fake.script("POST /conversations/{id}/read", { fail: 500 });
  const app = await renderApp("/conversations", { on });

  await open(app);
  await waitFor(() => {
    expect(reads.seen).toHaveLength(1);
  });
  // The read failed: the conversation is unread again.
  expect(await place("Direct messages 1 unread")).toBeDefined();

  // Sam's conversation is the older of the two.
  app.fake.newMessage(1);
  expect(await within(await messages()).findByText(said(1), { exact: false })).toBeDefined();
  await waitFor(() => {
    expect(reads.seen).toHaveLength(2);
  });

  await app.user.click(main().getByRole("button", { name: "Direct messages" }));
  expect(await within(await withSam()).findByRole("img", { name: "Unread" })).toBeDefined();
  expect(await within(await open(app)).findByText(said(1), { exact: false })).toBeDefined();
  // One read for each: opening, the newer post, reopening; a retry would count past it.
  await waitFor(() => {
    expect(reads.seen).toHaveLength(3);
  });
  expect(await place("Direct messages 1 unread")).toBeDefined();
  expect(reads.seen).toHaveLength(3);
});

test("a read answered for the post before a newer one keeps the newer one unread, then reads it", async () => {
  const on = fakeApp();
  const read = on.fake.script("POST /conversations/{id}/read", { times: 1, hold: "response" });
  const app = await renderApp("/conversations", { on });
  const feed = await open(app);
  const { url } = await read.arrived();
  const id = url.pathname.split("/").at(-2) ?? "";
  // The server read it up to the first post; a newer one arrives before the client hears so.
  await waitFor(async () => {
    const { data } = await app.data.api.GET("/conversations/{id}", { params: { path: { id } } });
    expect(data?.unread).toBe(false);
  });
  const newer = app.fake.newMessage(1);
  expect(await within(feed).findByText(said(1), { exact: false })).toBeDefined();
  read.release();

  // The answer for the first post leaves the newer one, which the client reads in turn.
  await waitFor(() => {
    expect(read.seen).toHaveLength(2);
  });
  expect(within(feed).getByText(said(1), { exact: false })).toBeDefined();
  expect(await place("Direct messages")).toBeDefined();

  const posted = app.fake.script("POST /statuses");
  const form = main().getByRole("form", { name: /^Your message to ⁨?Sam Rivera⁩?$/ });
  const box = await within(form).findByRole("textbox", { name: "Message" });
  // Pasted: the editor takes no typed key on a simulation.
  await app.user.click(box);
  await app.user.paste("Bus it is.");
  await app.user.click(within(form).getByRole("button", { name: /^Send to ⁨?Sam Rivera⁩?$/ }));
  expect((await posted.arrived()).body.in_reply_to_id).toBe(newer.id);
});

test("a newer post arriving while the hide is in flight keeps the conversation in the list", async () => {
  const on = fakeApp();
  const hide = on.fake.script("POST /conversations/{id}/hide", { hold: "response" });
  const app = await renderApp("/conversations", { on });

  await app.user.click(within(await withSam()).getByRole("button", { name: "More" }));
  await app.user.click(await screen.findByRole("menuitem", { name: "Hide from the list" }));
  await hide.arrived();
  await waitFor(async () => {
    expect(within(await list()).queryByRole("article", { name: "Sam Rivera" })).toBeNull();
  });
  // The server hid it up to its latest post; a newer one arrives before the client hears so.
  await waitFor(async () => {
    const { data } = await app.data.api.GET("/conversations");
    expect(data?.items).toHaveLength(1);
  });

  app.fake.newMessage(1);
  expect(await within(await withSam()).findByText(said(1), { exact: false })).toBeDefined();
  hide.release();
  // The screen says it hid the conversation once the server answered, and leaves the newer post's in the list, unread.
  expect(
    await main().findByText(/^You hid your direct messages with \u2068?Sam Rivera\u2069? from the list\./),
  ).toBeDefined();
  expect(within(await withSam()).getByText(said(1), { exact: false })).toBeDefined();
  expect(within(await withSam()).getByRole("img", { name: "Unread" })).toBeDefined();
});

test("the address of a conversation that is not the reader's opens the screen of the conversation, which says so", async () => {
  // An ID of the server's form that no conversation has.
  await renderApp("/conversations/01890a5d-ac96-774b-bcce-b302099a8057");

  expect(
    await main().findByText("These direct messages were not found. Check the link, or go back to the list."),
  ).toBeDefined();
});
