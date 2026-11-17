// SPDX-License-Identifier: AGPL-3.0-only

// An article (ADR-0160) on its page, the thread's first post: its title (the post's heading), its summary, its cover,
// and its text in full, read by the parts a reader sees.

import type { Locator } from "@playwright/test";
import type { Post } from "./timeline";

export class ArticleText {
  readonly article: Locator;

  constructor(post: Post) {
    this.article = post.article;
  }

  /** The article's title, the heading of the post. */
  title(title: string): Locator {
    return this.article.getByRole("heading", { level: 1, name: title, exact: true });
  }

  /** A paragraph of the article: its summary, or one of its text, whole. */
  paragraph(text: string): Locator {
    return this.article.getByRole("paragraph").filter({ hasText: new RegExp(`^${escape(text)}$`) });
  }

  /** The article's cover, or an image of its text, by its description. */
  image(alt: string): Locator {
    return this.article.getByRole("img", { name: alt, exact: true });
  }

  /** The headings of the text at `level` (2 to 6), in their order. */
  headings(level: 2 | 3 | 4 | 5 | 6): Locator {
    return this.article.getByRole("heading", { level });
  }

  /** The link of the text named `name`: a link the author wrote, or one for an image the server did not keep. */
  link(name: string): Locator {
    return this.article.getByRole("link", { name, exact: true });
  }
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
