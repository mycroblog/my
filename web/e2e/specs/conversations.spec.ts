// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212): a reader opens the list from the navigation, which counts the unread conversations;
// opening one marks it read on the server; the reply in place goes to its people as a direct post and shows in the
// conversation; a conversation hidden leaves the list and comes back with a new post; and a new direct post arrives in
// the list by the stream. Each test is between fresh people, so the reads and hides of one run leave the next alone.

import {
  Conversation,
  conversationRead,
  Conversations,
  failingReads,
  Frame,
  heldWrite,
  nextReplyTarget,
  streamOpened,
} from "./screens";
import { expect, test } from "./test";

test("a reader opens an unread conversation, which marks it read, and replies in place as a direct post", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const friend = await fresh.person();
  const { text: asked } = await fresh.post(friend, {
    text: `@${reader.username} Lunch at the bakery on Friday?`,
    visibility: "direct",
  });

  await signIn(reader);
  const frame = new Frame(page);
  await expect(frame.link("Direct messages")).toHaveAccessibleName("Direct messages 1 unread");
  await frame.goTo("Direct messages");

  const list = new Conversations(page);
  await expect(list.heading).toBeVisible();
  await expect(list.unread(friend.displayName)).toBeVisible();

  const read = conversationRead(page);
  const conversation = await list.open(friend.displayName);
  await expect(conversation.heading).toBeVisible();
  await expect(conversation.post(friend.displayName, asked)).toBeVisible();
  // Opening it marked it read: the count leaves the navigation.
  await expect(frame.link("Direct messages")).toHaveAccessibleName("Direct messages");
  await read;

  await conversation.reply("Friday works. Noon?");
  // The reply is the conversation's latest post, the reader's, direct and to the friend.
  await expect(conversation.posts).toHaveCount(2);
  const reply = conversation.post(reader.displayName, "Friday works. Noon?");
  await expect(reply).toContainText("You");
  await expect(reply.getByRole("img", { name: "Direct", exact: true })).toBeVisible();
  await expect(reply.getByRole("link", { name: `@${friend.username}`, exact: true })).toBeVisible();

  // Read on the server: the list read again shows it read, with the reply as its latest post.
  const again = await conversation.back();
  await page.reload();
  // The reader's own latest post is said as theirs, its mention of the friend first.
  await expect(again.conversation(friend.displayName)).toContainText(`You: @${friend.username} Friday works. Noon?`);
  await expect(again.unread(friend.displayName)).toHaveCount(0);
  await expect(frame.link("Direct messages")).toHaveAccessibleName("Direct messages");
});

test("a new direct post arrives in the list by the stream, unread", async ({ page, signIn, fresh }) => {
  const reader = await fresh.person();
  const friend = await fresh.person();
  const stream = streamOpened(page);
  await signIn(reader);
  const frame = new Frame(page);
  await frame.goTo("Direct messages");
  const list = new Conversations(page);
  await expect(list.heading).toBeVisible();

  // Made once the stream is open: the stream brings it, not the list read as it opened.
  await stream;
  const { text } = await fresh.post(friend, { text: `@${reader.username} Are you still up?`, visibility: "direct" });
  await expect(list.conversation(friend.displayName)).toContainText(text);
  await expect(list.unread(friend.displayName)).toBeVisible();
  await expect(frame.link("Direct messages")).toHaveAccessibleName("Direct messages 1 unread");
});

test("a post arriving in the conversation the reader has open shows in it, read", async ({ page, signIn, fresh }) => {
  const reader = await fresh.person();
  const friend = await fresh.person();
  const first = await fresh.post(friend, { text: `@${reader.username} Is the oven fixed?`, visibility: "direct" });
  const stream = streamOpened(page);
  await signIn(reader);
  const frame = new Frame(page);
  await frame.goTo("Direct messages");
  const opened = conversationRead(page);
  const conversation = await new Conversations(page).open(friend.displayName);
  await expect(conversation.post(friend.displayName, first.text)).toBeVisible();
  await expect(frame.link("Direct messages")).toHaveAccessibleName("Direct messages");
  await opened;

  await stream;
  const read = conversationRead(page);
  const { text } = await fresh.post(friend, {
    text: `@${reader.username} The repairman comes at three.`,
    visibility: "direct",
    replyTo: first.id,
  });
  await expect(conversation.post(friend.displayName, text)).toBeVisible();
  await expect(conversation.posts).toHaveCount(2);
  await read;
  // Read while open, on the server too: the list read again shows it read.
  const again = await conversation.back();
  await page.reload();
  await expect(again.conversation(friend.displayName)).toContainText(text);
  await expect(again.unread(friend.displayName)).toHaveCount(0);
  await expect(frame.link("Direct messages")).toHaveAccessibleName("Direct messages");
});

test("a hidden conversation leaves the list and comes back when a new post arrives in it", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const friend = await fresh.person();
  const first = await fresh.post(friend, { text: `@${reader.username} Did you get the photos?`, visibility: "direct" });
  await signIn(reader);
  await new Frame(page).goTo("Direct messages");
  const list = new Conversations(page);
  await expect(list.conversation(friend.displayName)).toBeVisible();

  await list.hide(friend.displayName);
  await expect(list.conversation(friend.displayName)).toHaveCount(0);
  await expect(list.hidden(friend.displayName)).toBeVisible();
  // Hidden on the server: the list read again leaves it out.
  const stream = streamOpened(page);
  await page.reload();
  await expect(list.heading).toBeVisible();
  await expect(list.conversation(friend.displayName)).toHaveCount(0);

  await stream;
  const { text } = await fresh.post(friend, {
    text: `@${reader.username} I sent them again.`,
    visibility: "direct",
    replyTo: first.id,
  });
  await expect(list.conversation(friend.displayName)).toContainText(text);
  await expect(list.unread(friend.displayName)).toBeVisible();
});

test("a read that fails is sent once per latest post: again for a newer post, and again on reopening", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const friend = await fresh.person();
  const first = await fresh.post(friend, { text: `@${reader.username} Got a minute?`, visibility: "direct" });
  const reads = await failingReads(page);
  const stream = streamOpened(page);
  await signIn(reader);
  const frame = new Frame(page);
  await frame.goTo("Direct messages");
  const conversation = await new Conversations(page).open(friend.displayName);
  await expect(conversation.post(friend.displayName, first.text)).toBeVisible();
  await expect.poll(reads.sent).toBe(1);
  // The read failed: the conversation is unread again.
  await expect(frame.link("Direct messages")).toHaveAccessibleName("Direct messages 1 unread");

  await stream;
  const { text } = await fresh.post(friend, {
    text: `@${reader.username} Never mind, found it.`,
    visibility: "direct",
    replyTo: first.id,
  });
  await expect(conversation.post(friend.displayName, text)).toBeVisible();
  await expect.poll(reads.sent).toBe(2);

  const list = await conversation.back();
  await expect(list.unread(friend.displayName)).toBeVisible();
  const reopened = await list.open(friend.displayName);
  await expect(reopened.post(friend.displayName, text)).toBeVisible();
  // One read for each: opening, the newer post, reopening; a retry would count past it.
  await expect.poll(reads.sent).toBe(3);
  await expect(frame.link("Direct messages")).toHaveAccessibleName("Direct messages 1 unread");
  await expect.poll(reads.sent).toBe(3);
});

test("a read answered for the post before a newer one keeps the newer one unread, then reads it", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const friend = await fresh.person();
  const first = await fresh.post(friend, { text: `@${reader.username} Train or bus?`, visibility: "direct" });
  const read = await heldWrite(page, "read");
  const stream = streamOpened(page);
  await signIn(reader);
  const frame = new Frame(page);
  await frame.goTo("Direct messages");
  const conversation = await new Conversations(page).open(friend.displayName);
  await read.arrived;

  // The server read it up to the first post; a newer one arrives before the client hears so.
  await stream;
  const newer = await fresh.post(friend, {
    text: `@${reader.username} Bus is faster today.`,
    visibility: "direct",
    replyTo: first.id,
  });
  await expect(conversation.post(friend.displayName, newer.text)).toBeVisible();
  read.release();

  // The answer for the first post leaves the newer one, which the client reads in turn.
  await expect.poll(read.sent).toBe(2);
  await expect(conversation.post(friend.displayName, newer.text)).toBeVisible();
  await expect(frame.link("Direct messages")).toHaveAccessibleName("Direct messages");

  const target = nextReplyTarget(page);
  await conversation.reply("Bus it is.");
  await expect(target).resolves.toBe(newer.id);
});

test("a newer post arriving while the hide is in flight keeps the conversation in the list", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const friend = await fresh.person();
  const first = await fresh.post(friend, { text: `@${reader.username} Keys are under the mat.`, visibility: "direct" });
  const hide = await heldWrite(page, "hide");
  const stream = streamOpened(page);
  await signIn(reader);
  await new Frame(page).goTo("Direct messages");
  const list = new Conversations(page);
  await expect(list.conversation(friend.displayName)).toBeVisible();

  await list.startHide(friend.displayName);
  await hide.arrived;
  await expect(list.conversation(friend.displayName)).toHaveCount(0);

  await stream;
  const newer = await fresh.post(friend, {
    text: `@${reader.username} Actually, I left them with Sam.`,
    visibility: "direct",
    replyTo: first.id,
  });
  await expect(list.conversation(friend.displayName)).toContainText(newer.text);
  const answered = page.waitForResponse((r) => /\/conversations\/[^/]+\/hide$/.test(new URL(r.url()).pathname));
  hide.release();
  await answered;
  await expect(list.conversation(friend.displayName)).toContainText(newer.text);
  await expect(list.unread(friend.displayName)).toBeVisible();
  // On the server too: hidden up to the first post, so the newer one shows it.
  await page.reload();
  await expect(list.conversation(friend.displayName)).toContainText(newer.text);
});

test("a conversation that is not the reader's says so", async ({ page, signIn, fresh }) => {
  await signIn(await fresh.person());
  const conversation = new Conversation(page, "");
  // An ID of the server's form that no conversation has.
  await conversation.openAt("01890a5d-ac96-774b-bcce-b302099a8057");
  await expect(conversation.gone).toBeVisible();
});
