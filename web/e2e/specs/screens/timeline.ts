// SPDX-License-Identifier: AGPL-3.0-only

// A feed of posts (the home timeline, a profile's posts, a thread) and a post in it, with what a person does to it.
// A post is found by its author's display name, which names its article, and a part of its text.

import type { Locator, Page } from "@playwright/test";
import { Composer } from "./composer";

/** A count button's name: the action, then the count when there is one ("Favourite", "Favourite, 2"). */
const counted = (action: string) => new RegExp(`^${action}(, \\d+)?$`);

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

  async favourite(): Promise<void> {
    await this.favouriteButton.click();
  }

  /** Boosts the post, from the boost button's menu (which also offers a quote). */
  async boost(): Promise<void> {
    await this.boostButton.click();
    await this.page.getByRole("menu", { name: "Boost" }).getByRole("menuitem", { name: "Boost", exact: true }).click();
  }

  /** Opens the composer of a reply to the post. */
  async reply(): Promise<Composer> {
    await this.article.getByRole("button", { name: counted("Reply") }).click();
    return new Composer(this.page.getByRole("dialog", { name: "Reply" }));
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

/** The home timeline: the posts of the people the reader follows, and their own. */
export function homeTimeline(page: Page): Timeline {
  return new Timeline(page, page.getByRole("feed", { name: "Home timeline" }));
}
