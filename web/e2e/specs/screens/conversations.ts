// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages: the list of the reader's conversations, each an article named by its other people ("Hana 花 and
// ليلى Layla") with an Unread mark while unread, its time a link to the conversation, and a More menu that hides it; and
// a conversation's screen, headed Messages: its messages in a feed named "Direct messages with {names}", oldest first,
// the reader's marked "You", then the reply in place, a text box named Message and the button that sends it.

import type { Locator, Page } from "@playwright/test";
import { answered } from "./write";

const escaped = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/**
 * Settles once the server has answered the client's read of a conversation; ask before what makes the client mark it
 * read (opening it, a post arriving in it), and await it before reloading: the screen shows it read before the answer.
 */
export function conversationRead(page: Page): Promise<void> {
  const read = page.waitForResponse(
    (r) => r.request().method() === "POST" && /\/conversations\/[^/]+\/read$/.test(new URL(r.url()).pathname),
  );
  return read.then(() => undefined);
}

export class Conversations {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Direct messages", exact: true });
  }

  /** The reader's conversations, the one with the newest post first. */
  get feed(): Locator {
    return this.page.getByRole("feed", { name: "Direct messages", exact: true });
  }

  /** The conversation with `names`, the other people's display names as the list says them ("A and B"). */
  conversation(names: string): Locator {
    return this.feed.getByRole("article", { name: names, exact: true });
  }

  /** The mark of the conversation with `names` while it is unread. */
  unread(names: string): Locator {
    return this.conversation(names).getByRole("img", { name: "Unread", exact: true });
  }

  /** Opens the conversation with `names` from its time, the link to it. */
  async open(names: string): Promise<Conversation> {
    await this.conversation(names).getByRole("time").click();
    return new Conversation(this.page, names);
  }

  /** Hides the conversation with `names` from its More menu, and returns once the server has answered. */
  async hide(names: string): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/conversations\/[^/]+\/hide$/ }, async () => {
      await this.conversation(names).getByRole("button", { name: "More", exact: true }).click();
      await this.page.getByRole("menuitem", { name: "Hide from the list", exact: true }).click();
    });
  }

  /** The line that says the conversation with `names` (isolated from the words around) was hidden and comes back. */
  hidden(names: string): Locator {
    return this.page
      .getByRole("main")
      .getByText(
        new RegExp(
          `^You hid your direct messages with \\u2068?${escaped(names)}\\u2069? from the list\\. They come back when a new message arrives\\.$`,
        ),
      );
  }
}

export class Conversation {
  readonly page: Page;
  /** The other people's display names, as the screen says them. */
  readonly names: string;

  constructor(page: Page, names: string) {
    this.page = page;
    this.names = names;
  }

  /** The screen's heading. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Messages", exact: true });
  }

  /** The conversation's messages, oldest first; the names in its name are isolated from the words around them. */
  get feed(): Locator {
    return this.page.getByRole("feed", {
      name: new RegExp(`^Direct messages with \\u2068?${escaped(this.names)}\\u2069?$`),
    });
  }

  /** The posts of the conversation, oldest first. */
  get posts(): Locator {
    return this.feed.getByRole("article");
  }

  /** The post of `author` (a display name) that holds `text`. */
  post(author: string, text: string): Locator {
    return this.feed.getByRole("article", { name: author, exact: true }).filter({ hasText: text });
  }

  /** The reply in place, a form named for the people it goes to ("Your message to {names}"). */
  get replyForm(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("form", { name: new RegExp(`^Your message to \\u2068?${escaped(this.names)}\\u2069?$`) });
  }

  /** The text box of the reply in place. */
  get replyBox(): Locator {
    return this.replyForm.getByRole("textbox", { name: "Message", exact: true });
  }

  /** The button that sends the reply in place ("Send to {names}"). */
  get replyButton(): Locator {
    return this.replyForm.getByRole("button", {
      name: new RegExp(`^Send to \\u2068?${escaped(this.names)}\\u2069?$`),
    });
  }

  /** Writes `text` in the reply in place and sends it, and returns once the server has answered. */
  async reply(text: string): Promise<void> {
    await this.replyBox.fill(text);
    await answered(this.page, { methods: ["POST"], path: /\/statuses$/ }, async () => {
      await this.replyButton.click();
    });
  }

  /** Goes back to the list of the direct messages. */
  async back(): Promise<Conversations> {
    await this.page.getByRole("main").getByRole("button", { name: "Direct messages", exact: true }).click();
    return new Conversations(this.page);
  }
}
