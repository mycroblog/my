// SPDX-License-Identifier: AGPL-3.0-only

// A feed of posts (the home timeline, a profile's posts, a thread) and a post in it, with what a person does to it.
// A post is found by its author's display name, which names its article, and a part of its text.

import type { Locator, Page } from "@playwright/test";
import { Composer } from "./composer";
import { EmojiPicker } from "./emoji-picker";
import { answered } from "./write";

/** A count button's name: the action, then the count when there is one ("Favourite", "Favourite, 2"). */
const counted = (action: string) => new RegExp(`^${action}(, \\d+)?$`);

/** A vote in a post's poll. */
const votes = { methods: ["POST"], path: /\/statuses\/[^/]+\/poll\/votes$/ } as const;

/** A bookmark of a post, set or taken back. */
const bookmark = { methods: ["POST", "DELETE"], path: /\/statuses\/[^/]+\/bookmark$/ } as const;

export class Post {
  readonly page: Page;
  readonly article: Locator;

  constructor(page: Page, article: Locator) {
    this.page = page;
    this.article = article;
  }

  /** The paragraph of the post's text that holds `text`. */
  paragraph(text: string): Locator {
    return this.article.getByRole("paragraph").filter({ hasText: text });
  }

  /** The attached image whose alt text is `alt`. */
  image(alt: string): Locator {
    return this.article.getByRole("img", { name: alt, exact: true });
  }

  /** The picture that opens in the media viewer: "Open image" or "Open GIF", then ": " and its description if any. */
  picture(name: string): Locator {
    return this.article.getByRole("button", { name, exact: true });
  }

  /**
   * The button of the line above sensitive media, Show or Hide, described by what it covers, `label` ("Sensitive
   * image", "2 sensitive images").
   */
  sensitiveLine(label: string, action: "Show" | "Hide"): Locator {
    return this.article.getByRole("button", { name: action, description: label, exact: true });
  }

  /**
   * The color [red, green, blue] at the middle of the first blurhash drawn in the post in place of covered media, a
   * picture hidden from screen readers that only its pixels tell; none while none is drawn.
   */
  async coveredColor(): Promise<number[]> {
    return this.article.evaluate((article) => {
      const canvas = article.querySelector("canvas");
      const pixel = canvas?.getContext("2d")?.getImageData(canvas.width / 2, canvas.height / 2, 1, 1).data;
      return pixel === undefined ? [] : [...pixel.slice(0, 3)];
    });
  }

  /** The link card of the post's first link, named by its text: the site, the title, and so on. */
  card(name: RegExp): Locator {
    return this.article.getByRole("link", { name });
  }

  /** The button that shows the text behind a content warning. */
  get showButton(): Locator {
    return this.article.getByRole("button", { name: "Show", exact: true });
  }

  /** The favourite button, which is pressed when the reader has favourited the post. */
  get favouriteButton(): Locator {
    return this.article.getByRole("button", { name: counted("Favourite") });
  }

  /** The boost button, which is pressed when the reader has boosted the post. */
  get boostButton(): Locator {
    return this.article.getByRole("button", { name: counted("Boost") });
  }

  /** The reader's favourite of the post: the favourite button, pressed. */
  get favourited(): Locator {
    return this.article.getByRole("button", { name: counted("Favourite"), pressed: true });
  }

  /** The reader's boost of the post: the boost button, pressed. */
  get boosted(): Locator {
    return this.article.getByRole("button", { name: counted("Boost"), pressed: true });
  }

  /** Favourites the post, and returns once the server has answered. */
  async favourite(): Promise<void> {
    await answered(this.page, { methods: ["POST", "DELETE"], path: /\/statuses\/[^/]+\/favourite$/ }, async () => {
      await this.favouriteButton.click();
    });
  }

  /**
   * Boosts the post, from the boost button's menu (which also offers a quote), and returns once the server answered.
   * The item's name is "Boost", followed by the line saying the boost waits when the author approves boosts.
   */
  async boost(): Promise<void> {
    await answered(this.page, { methods: ["POST", "DELETE"], path: /\/statuses\/[^/]+\/boost$/ }, async () => {
      await this.boostButton.click();
      await this.page
        .getByRole("menu", { name: "Boost", exact: true })
        .getByRole("menuitem", { name: /^Boost(?: Your boost goes out once \S+ approves it)?$/ })
        .click();
    });
  }

  /** The React button, which opens the emoji picker. */
  get reactButton(): Locator {
    return this.article.getByRole("button", { name: "React", exact: true });
  }

  /** The row of the post's emoji reactions, which shows only while it has one. */
  get reactions(): Locator {
    return this.article.getByRole("group", { name: "Reactions", exact: true });
  }

  /** The chips of the post's emoji reactions, each named by its emoji and count ("😀, 2"). */
  get reactionChips(): Locator {
    return this.reactions.getByRole("button");
  }

  /** The reader's own reaction: its chip, pressed, named "{emoji}, {count}, including yours". */
  get ownReaction(): Locator {
    return this.reactions.getByRole("button", { pressed: true });
  }

  /** Opens the emoji picker from the React button, by pointer or with a key pressed on it, and returns it. */
  async react(by: "pointer" | "Enter" | "Space" = "pointer"): Promise<EmojiPicker> {
    if (by === "pointer") await this.reactButton.click();
    else await this.reactButton.press(by);
    return new EmojiPicker(this.page.getByRole("dialog", { name: "React", exact: true }));
  }

  /**
   * Does `act`, a pick in the picker by pointer or key that sets or takes back the reader's reaction, and returns once
   * the server has answered it.
   */
  async reacting(act: () => Promise<void>): Promise<void> {
    await answered(this.page, { methods: ["PUT", "DELETE"], path: /\/statuses\/[^/]+\/reaction$/ }, act);
  }

  /** Picks `emoji`, a cell of the picker, with the pointer, and returns once the server has answered the reaction. */
  async pick(emoji: Locator): Promise<void> {
    await this.reacting(async () => {
      await emoji.click();
    });
  }

  /** The row under the post on its own page that leads to who reacted with one emoji ("😀 2 people reacted"). */
  whoReacted(name: string): Locator {
    return this.article.getByRole("link", { name, exact: true });
  }

  /** The post's poll, named "Poll" in en and "投票" in ja. */
  poll(name = "Poll"): Locator {
    return this.article.getByRole("group", { name, exact: true });
  }

  /** The option of the open poll titled `title`: a radio, or a checkbox where several may be chosen. */
  option(title: string, kind: "radio" | "checkbox" = "radio"): Locator {
    return this.poll().getByRole(kind, { name: title, exact: true });
  }

  /** The poll's Vote button, named in the reader's language. */
  voteButton(name = "Vote"): Locator {
    return this.poll().getByRole("button", { name, exact: true });
  }

  /** The poll's results, an item per option: its title, "(your vote)" after the reader's choice, and its share. */
  get results(): Locator {
    return this.poll().getByRole("listitem");
  }

  /** The line under the poll: who voted, and when it ends. */
  pollLine(name = "Poll"): Locator {
    return this.poll(name).getByRole("paragraph");
  }

  /** Chooses the option of the poll titled `title`, from its label. */
  async choose(title: string): Promise<void> {
    await this.poll().getByText(title, { exact: true }).click();
  }

  /** Presses Vote, and returns once the server has answered the vote. */
  async vote(): Promise<void> {
    await answered(this.page, votes, async () => {
      await this.voteButton().click();
    });
  }

  /** Opens the composer of a reply to the post. */
  async reply(): Promise<Composer> {
    await this.article.getByRole("button", { name: counted("Reply") }).click();
    return new Composer(this.page.getByRole("dialog", { name: "Reply", exact: true }));
  }

  /** The post's more menu, once open. */
  get menu(): Locator {
    return this.page.getByRole("menu", { name: "More", exact: true });
  }

  /** Opens the post's more menu, and returns its items. */
  async more(): Promise<Locator> {
    await this.article.getByRole("button", { name: "More", exact: true }).click();
    return this.menu.getByRole("menuitem");
  }

  /** Chooses `item` of the post's more menu, by its English words. */
  async menuItem(item: string): Promise<void> {
    await this.more();
    await this.menu.getByRole("menuitem", { name: item, exact: true }).click();
  }

  /** Deletes the reader's post from Delete… and its confirmation, and returns once the server has answered. */
  async delete(): Promise<void> {
    await this.menuItem("Delete…");
    const confirm = this.page.getByRole("alertdialog", { name: "Delete this post?", exact: true });
    await answered(this.page, { methods: ["DELETE"], path: /\/statuses\/[^/]+$/ }, async () => {
      await confirm.getByRole("button", { name: "Delete", exact: true }).click();
    });
  }

  /** Bookmarks the post from its more menu, and returns once the server has answered. */
  async bookmark(): Promise<void> {
    await answered(this.page, bookmark, async () => {
      await this.menuItem("Bookmark");
    });
  }

  /** Takes the reader's bookmark of the post back from its more menu, and returns once the server has answered. */
  async removeBookmark(): Promise<void> {
    await answered(this.page, bookmark, async () => {
      await this.menuItem("Remove bookmark");
    });
  }

  /** Mutes the thread of the post (`Mute thread`) or unmutes it from its more menu, once the server has answered. */
  async muteThread(item: "Mute thread" | "Unmute thread" = "Mute thread"): Promise<void> {
    await answered(this.page, { methods: ["POST", "DELETE"], path: /\/statuses\/[^/]+\/mute$/ }, async () => {
      await this.menuItem(item);
    });
  }

  /** The link of the hashtag `#name` in the post's text. */
  hashtag(name: string): Locator {
    return this.article.getByRole("link", { name: `#${name}`, exact: true });
  }

  /** Opens the composer that edits the reader's post. */
  async edit(): Promise<Composer> {
    await this.menuItem("Edit");
    return new Composer(this.page.getByRole("dialog", { name: "Edit post", exact: true }));
  }

  /** The word "edited" after the time of an edited post, which opens its edit history. */
  get edited(): Locator {
    return this.article.getByRole("button", { name: "Edited, show history", exact: true });
  }

  /** Opens the post's edit history from "edited", and returns its versions, newest first. */
  async history(): Promise<Locator> {
    await this.edited.click();
    return this.page
      .getByRole("dialog", { name: "Edit history", exact: true })
      .getByRole("list", { name: "Edit history", exact: true })
      .getByRole("listitem")
      .filter({ has: this.page.getByRole("time") });
  }

  /** Reveals the text behind the content warning. */
  async show(): Promise<void> {
    await this.showButton.click();
  }

  /**
   * The line over the reader's own reply or boost while its author's approval is not given: "Waiting for approval" or
   * "Not approved" over the reply, "Boost waiting for approval" over the post boosted.
   */
  approvalLine(words: "Waiting for approval" | "Not approved" | "Boost waiting for approval"): Locator {
    return this.article.getByText(words, { exact: true });
  }

  /** Opens the post's own screen, with its thread, from the link of its time. */
  async open(): Promise<void> {
    await this.article
      .getByRole("link")
      .filter({ has: this.page.getByRole("time") })
      .click();
  }
}

export class Timeline {
  readonly page: Page;
  readonly feed: Locator;

  constructor(page: Page, feed: Locator) {
    this.page = page;
    this.feed = feed;
  }

  /** The post of `author` (a display name) that holds `text`, itself and not a boost of it. */
  post(author: string, text: string): Post {
    return new Post(
      this.page,
      this.feed
        .getByRole("article", { name: author, exact: true })
        .filter({ hasText: text })
        .filter({ hasNotText: "boosted" }),
    );
  }

  /** The posts of the feed, in the feed's order. */
  get posts(): Locator {
    return this.feed.getByRole("article");
  }
}

/** The local timeline: the public posts of the server's people, which a visitor reads too. Its name is in ja for ja. */
export function localTimeline(page: Page, name = "Local timeline"): Timeline {
  return new Timeline(page, page.getByRole("feed", { name, exact: true }));
}

/** The reader's bookmarks: the posts they bookmarked, the newest bookmark first. */
export function bookmarks(page: Page): Timeline {
  return new Timeline(page, page.getByRole("feed", { name: "Bookmarks", exact: true }));
}

/** The home timeline: the posts of the people the reader follows, and their own. */
export function homeTimeline(page: Page): Timeline {
  return new Timeline(page, page.getByRole("feed", { name: "Home timeline", exact: true }));
}
