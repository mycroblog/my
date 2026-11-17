// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212): a reader opens the list from the navigation, which counts the unread conversations;
// opening one marks it read on the server; the reply in place goes to its people as a direct post and shows in the
// conversation; a conversation hidden leaves the list and comes back with a new post; and a new direct post arrives in
// the list by the stream. Each test is between fresh people, so the reads and hides of one run leave the next alone.

import { conversationRead, Conversations, Frame, streamOpened } from "./screens";
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
