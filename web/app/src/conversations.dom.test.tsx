// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212): what the list and a conversation show, with the count of the unread conversations in
// the navigation (on the rail's place, and on the tab bar's Menu, which holds the place on a phone), and how the client
// treats the server's answers: a read that fails, a read answered late, a hide answered late, a conversation that is
// not there. The scenario's reader is Yuki, whose conversation with Sam is unread and comes after the one with Hana
// and Layla.

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
import { paste } from "./test/composer";
import { fakeApp } from "./test/server";
import { tabBar } from "./test/viewport";

/** The list of the reader's conversations. */
const list = () => main().findByRole("feed", { name: "Direct messages" });
/** The conversation with Sam in the list. */
const withSam = async () => within(await list()).findByRole("article", { name: "Sam Rivera" });
/** The messages of the conversation with Sam, on its screen; the names in its name are isolated from the words around. */
const messages = () => main().findByRole("feed", { name: /^Direct messages with ⁨?Sam Rivera⁩?$/ });
/** The navigation's link to the direct messages, named `name`: the place, then the count of the unread conversations. */
const place = (name: "Direct messages" | "Direct messages 1 unread") => screen.findByRole("link", { name });
/** The tab bar's Menu, named `name`: the word, then how many conversations of the place inside it are unread. */
const menu = (name: "Menu" | "Menu 1 unread in direct messages") => within(tabBar()).findByRole("button", { name });
/** The conversation with Hana and Layla in the list; the names in its name are isolated from the words around. */
const withGroup = async () =>
  within(await list()).findByRole("article", { name: /^⁨?Hana 花⁩?\s+and\s+⁨?ليلى Layla⁩?$/ });
/** The display names of the scenario's people who write direct messages. */
const people = ["Sam Rivera", "Yuki 雪", "Hana 花", "ليلى Layla"];
/** Who wrote each post of `feed`, in its order: a post is named by its author. */
const authorsOf = (feed: HTMLElement): (string | undefined)[] => {
  const by = people.map((name) => [name, within(feed).queryAllByRole("article", { name })] as const);
  return within(feed)
    .getAllByRole("article")
    .map((post) => by.find(([, posts]) => posts.includes(post))?.[0]);
};
/** What the fake's `newMessage` writes, after its mention of the reader. */
const said = (n: number) => `One more thing, number ${String(n)}.`;

/** Opens the conversation with Sam from its time, the link to it, and returns its messages. */
async function open({ user }: TestApp) {
  await user.click(within(await withSam()).getByRole("time"));
  return messages();
}

test("the list shows the reader's conversations under its heading, the newest first, and the navigation counts the unread one", async () => {
  await renderApp("/conversations");

  expect(main().getByRole("heading", { level: 1, name: "Direct messages" })).toBeDefined();
  const [group, sam] = [await withGroup(), await withSam()];
  expect(within(await list()).getAllByRole("article")).toHaveLength(2);
  expect(group.compareDocumentPosition(sam) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(within(sam).getAllByRole("img", { name: "Unread" })).toHaveLength(1);
  expect(within(group).queryAllByRole("img", { name: "Unread" })).toHaveLength(0);
  expect(await place("Direct messages 1 unread")).toBeDefined();
  expect(await menu("Menu 1 unread in direct messages")).toBeDefined();
});

test("a reader with no conversation is shown none, and the navigation counts none", async () => {
  await renderApp("/conversations", { seed: { people: scenario.people, posts: [] } });

  expect(await main().findByText(/^No direct messages yet\./)).toBeDefined();
  expect(main().queryAllByRole("article")).toHaveLength(0);
  expect(await place("Direct messages")).toBeDefined();
  expect(await menu("Menu")).toBeDefined();
});

test("a conversation of a group hidden from the list leaves the others, under a line that names its people", async () => {
  const app = await renderApp("/conversations");

  await app.user.click(within(await withGroup()).getByRole("button", { name: "More" }));
  await app.user.click(await screen.findByRole("menuitem", { name: "Hide from the list" }));

  expect(
    await main().findByText(
      /^You hid your direct messages with \u2068?Hana 花\u2069? and \u2068?ليلى Layla\u2069? from the list\./,
    ),
  ).toBeDefined();
  await waitFor(async () => {
    expect(within(await list()).getAllByRole("article")).toHaveLength(1);
  });
  expect(await withSam()).toBeDefined();
  // Sam's is the unread one: the count stays.
  expect(await place("Direct messages 1 unread")).toBeDefined();
});

test("a conversation shows its posts oldest first, with the reply in place", async () => {
  const on = fakeApp();
  // The conversation with Sam is the older of the two.
  await renderApp(on.fake.conversationPath(1), { on });

  expect(main().getByRole("heading", { level: 1, name: "Messages" })).toBeDefined();
  expect(authorsOf(await messages())).toEqual(["Sam Rivera", "Yuki 雪", "Sam Rivera"]);
  const form = main().getByRole("form", { name: /^Your message to ⁨?Sam Rivera⁩?$/ });
  expect(await within(form).findByRole("textbox", { name: "Message" })).toBeDefined();
});

// Defect: the list the shell reads for the count is asked before the read is sent and answered, unread, before the
// read is; the read's answer does not correct it, and the navigation counts 1 unread until the list is read again.
// Opened from the list, which is read by then, the count goes (the tests of the reads below).
test("a conversation opened by its address is read: the navigation counts none", async () => {
  const on = fakeApp();
  const reads = on.fake.script("POST /conversations/{id}/read");
  await renderApp(on.fake.conversationPath(1), { on });
  await messages();
  await reads.served();

  await waitFor(() => {
    const [link] = screen.getAllByRole("link", { name: /^Direct messages/ });
    if (/unread/.test(link?.textContent ?? "")) throw new Error("test: the navigation still counts an unread one");
  });
});

test("a conversation of a group shows the posts of its people oldest first, with the reply in place", async () => {
  const on = fakeApp();
  await renderApp(on.fake.conversationPath(0), { on });

  const feed = await main().findByRole("feed", {
    name: /^Direct messages with ⁨?Hana 花⁩?\s+and\s+⁨?ليلى Layla⁩?$/,
  });
  expect(main().getByRole("heading", { level: 1, name: "Messages" })).toBeDefined();
  expect(authorsOf(feed)).toEqual(["Hana 花", "ليلى Layla", "Yuki 雪"]);
  const form = main().getByRole("form", { name: /^Your message to ⁨?Hana 花⁩?\s+and\s+⁨?ليلى Layla⁩?$/ });
  expect(await within(form).findByRole("textbox", { name: "Message" })).toBeDefined();
});

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
  await paste(app, box, "Bus it is.");
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
