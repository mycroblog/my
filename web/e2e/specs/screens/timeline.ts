// SPDX-License-Identifier: AGPL-3.0-only

// A feed of posts (the home timeline, a profile's posts, a thread) and a post in it, with what a person does to it.
// A post is found by its author's display name, which names its article, and a part of its text.

import type { Locator, Page } from "@playwright/test";
import { Composer } from "./composer";
import { answered } from "./write";

/** A count button's name: the action, then the count when there is one ("Favourite", "Favourite, 2"). */
const counted = (action: string) => new RegExp(`^${action}(, \\d+)?$`);

/** A vote in a post's poll. */
const votes = { methods: ["POST"], path: /\/statuses\/[^/]+\/poll\/votes$/ } as const;

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

  /** Boosts the post, from the boost button's menu (which also offers a quote), and returns once the server answered. */
  async boost(): Promise<void> {
    await answered(this.page, { methods: ["POST", "DELETE"], path: /\/statuses\/[^/]+\/boost$/ }, async () => {
      await this.boostButton.click();
      await this.page
        .getByRole("menu", { name: "Boost", exact: true })
        .getByRole("menuitem", { name: "Boost", exact: true })
        .click();
    });
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

  /** Sends the vote again from `tryAgain`, the notice's action, and returns once the server has answered it. */
  async voteAgain(tryAgain: Locator): Promise<void> {
    await answered(this.page, votes, async () => {
      await tryAgain.click();
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

  /**
   * Deletes the reader's post from Delete and redraft… and its confirmation, once the server has answered, and returns
   * the composer it opens as a new post.
   */
  async redraft(): Promise<Composer> {
    await this.menuItem("Delete and redraft…");
    const confirm = this.page.getByRole("alertdialog", { name: "Delete and redraft this post?", exact: true });
    await answered(this.page, { methods: ["DELETE"], path: /\/statuses\/[^/]+$/ }, async () => {
      await confirm.getByRole("button", { name: "Delete and redraft", exact: true }).click();
    });
    return new Composer(this.page.getByRole("dialog", { name: "New post", exact: true }));
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

  /** Opens the post's own screen, with its thread, from the link of its time. */
  async open(): Promise<void> {
    await this.article
      .getByRole("link")
      .filter({ has: this.page.getByRole("time") })
      .click();
  }

  /** Opens the profile of the post's author, from the link of their name. */
  async openAuthor(displayName: string): Promise<void> {
    await this.article.getByRole("link", { name: displayName, exact: true }).click();
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

  /** The boost by `booster` of the post of `author` that holds `text`. */
  boostOf(booster: string, author: string, text: string): Post {
    return new Post(
      this.page,
      this.feed
        .getByRole("article", { name: author, exact: true })
        .filter({ hasText: text })
        .filter({ hasText: `${booster} boosted` }),
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

/** The home timeline: the posts of the people the reader follows, and their own. */
export function homeTimeline(page: Page): Timeline {
  return new Timeline(page, page.getByRole("feed", { name: "Home timeline", exact: true }));
}
