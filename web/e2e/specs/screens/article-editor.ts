// SPDX-License-Identifier: AGPL-3.0-only

// The article editor (ADR-0160), a screen of its own outside the shell: the title, the Markdown body, what is about the
// article (its summary, cover and hashtags), the preview as it will be published beside them on a wide screen, the
// limits of the body, and the row that publishes, schedules or saves it. Its drafts' menu keeps it on the server and
// opens the reader's drafts of articles.

import { expect, type Locator, type Page, type Response } from "@playwright/test";
import { holds } from "./composer";
import { Drafts } from "./drafts";
import { answered } from "./write";

/** A file to upload, as the file picker hands it to the page. */
export type Upload = { name: string; mimeType: string; buffer: Buffer };

/**
 * Holds the server's answers that an upload is processed (GET /media/{id}) until the returned function releases them,
 * so that the page sees its uploads processing for as long as a test needs.
 */
export async function holdProcessing(page: Page): Promise<() => Promise<void>> {
  const { promise: released, resolve: release } = Promise.withResolvers<undefined>();
  const media = (url: URL) => /\/api\/v\d\/media\/[^/]+$/.test(url.pathname);
  await page.route(media, async (route) => {
    await released;
    await route.continue();
  });
  return async () => {
    release(undefined);
    await page.unroute(media);
  };
}

/**
 * Holds the server's previews of a body (POST /articles/preview) until `release`, so that the page keeps showing the
 * preview it had for as long as a test needs; `asked` holds the body of each request, in their order.
 */
export async function holdPreviews(page: Page): Promise<{ asked: unknown[]; release: () => void }> {
  const { promise: released, resolve } = Promise.withResolvers<undefined>();
  const asked: unknown[] = [];
  const previews = (url: URL) => /\/api\/v\d\/articles\/preview$/.test(url.pathname);
  await page.route(previews, async (route) => {
    asked.push(route.request().postDataJSON());
    await released;
    // A request the page gave up on meanwhile (an earlier body's, superseded) is gone: nothing to let through.
    await route.continue().catch(() => undefined);
  });
  return {
    asked,
    // Released, the route lets every preview through at once; it stays until the page closes.
    release: () => {
      resolve(undefined);
    },
  };
}

export class ArticleEditor {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the editor of a new article at its own address. */
  async openNew(): Promise<void> {
    await this.page.goto("/articles/new");
    await expect(this.heading("New article")).toBeVisible();
  }

  /** The main navigation's New article, under New post (from `tablet`), while the reader may write articles. */
  get navigationEntry(): Locator {
    return this.page
      .getByRole("navigation", { name: "Main", exact: true })
      .getByRole("button", { name: "New article", exact: true });
  }

  /** Opens the editor from the main navigation's New article. */
  async openFromNavigation(): Promise<void> {
    await this.navigationEntry.click();
    await expect(this.heading("New article")).toBeVisible();
  }

  /** Opens the editor from the place New article that starts the phone's Menu sheet. */
  async openFromMenuSheet(): Promise<void> {
    await this.page
      .getByRole("navigation", { name: "Main", exact: true })
      .getByRole("button", { name: "Menu", exact: true })
      .click();
    await this.page
      .getByRole("dialog", { name: "Menu", exact: true })
      .getByRole("link", { name: "New article", exact: true })
      .click();
    await expect(this.heading("New article")).toBeVisible();
  }

  /** The menu of what else can be written, which the phone's floating New post opens on a long press or right click. */
  get postMenu(): Locator {
    return this.page.getByRole("menu", { name: "Other things to write", exact: true });
  }

  /** Opens the phone's floating New post's menu with a right click, and the editor from its New article. */
  async openFromPostMenu(): Promise<void> {
    await this.page.getByRole("button", { name: "New post", exact: true }).click({ button: "right" });
    await this.postMenu.getByRole("menuitem", { name: "New article", exact: true }).click();
    await expect(this.heading("New article")).toBeVisible();
  }

  /** The screen's heading: New article, or Edit article for the reader's published one. */
  heading(name: "New article" | "Edit article"): Locator {
    return this.page.getByRole("heading", { level: 1, name, exact: true });
  }

  get title(): Locator {
    return this.page.getByRole("textbox", { name: "Title", exact: true });
  }

  /** The body, Markdown in a plain text editor. */
  get body(): Locator {
    return this.page.getByRole("textbox", { name: "Body", exact: true });
  }

  /** Writes `text` at the end of the body, a key at a time; "\n" is Enter. */
  async write(text: string): Promise<void> {
    await this.body.click();
    await this.body.press("ControlOrMeta+End");
    await this.body.pressSequentially(text);
  }

  /** Selects the whole body and deletes it, as a writer does from the keyboard. */
  async clearBody(): Promise<void> {
    await this.body.click();
    await this.body.press("ControlOrMeta+a");
    await this.body.press("Delete");
    await this.holds("");
  }

  /** Waits until the body holds `text`, its paragraphs joined by "\n". */
  async holds(text: string | RegExp): Promise<void> {
    await holds(this.body, text);
  }

  /** What is about the article, after the body: its summary, its cover and its hashtags. */
  get details(): Locator {
    return this.page.getByRole("region", { name: "About the article", exact: true });
  }

  get summary(): Locator {
    return this.details.getByRole("textbox", { name: "Summary", exact: true });
  }

  /** The field that adds a hashtag. */
  get tagField(): Locator {
    return this.details.getByRole("textbox", { name: "Hashtags", exact: true });
  }

  /** Adds the hashtag `name` with Enter. */
  async addTag(name: string): Promise<void> {
    await this.tagField.fill(name);
    await this.tagField.press("Enter");
    await expect(this.removeTag(name)).toBeVisible();
  }

  /** The button on the hashtag `name` that takes it off. */
  removeTag(name: string): Locator {
    return this.details.getByRole("button", { name: `Remove #${name}`, exact: true });
  }

  /** The description of the cover, once it has one. */
  get coverDescription(): Locator {
    return this.details.getByRole("textbox", { name: "Description", exact: true });
  }

  /**
   * Picks `file` as the cover from Add a cover image, and returns the ID of the upload once the server has processed it
   * (`processed`, by default) or once it has taken it.
   */
  async addCover(file: Upload, { processed = true } = {}): Promise<string> {
    const button = this.details.getByRole("button", { name: "Add a cover image", exact: true });
    return this.#upload(file, button, processed);
  }

  /**
   * Picks `file` from Add an image to the body, and returns the ID of the upload once the server has processed it
   * (`processed`, by default) or once it has taken it.
   */
  async addImage(file: Upload, { processed = true } = {}): Promise<string> {
    const button = this.page.getByRole("button", { name: "Add an image to the body", exact: true });
    return this.#upload(file, button, processed);
  }

  async #upload(file: Upload, button: Locator, processed: boolean): Promise<string> {
    // The server takes an image (202) and processes it; the client reads it again until it is (200).
    const ready = new Set<string>();
    const seen = (r: Response) => {
      const id = /\/media\/([^/]+)$/.exec(new URL(r.url()).pathname)?.[1];
      if (r.request().method() === "GET" && r.status() === 200 && id !== undefined) ready.add(id);
    };
    this.page.on("response", seen);
    try {
      const response = this.page.waitForResponse(
        (r) => r.request().method() === "POST" && /\/media$/.test(new URL(r.url()).pathname),
      );
      response.catch(() => undefined);
      const chooser = this.page.waitForEvent("filechooser");
      await button.click();
      await (await chooser).setFiles(file);
      const answer = await response;
      const { id } = (await answer.json()) as { id: string };
      if (processed && answer.status() !== 200) await expect.poll(() => ready.has(id)).toBe(true);
      return id;
    } finally {
      this.page.off("response", seen);
    }
  }

  /** The article as it will be published, the server's rendering of what is written. */
  get preview(): Locator {
    return this.page.getByRole("article");
  }

  /** The heading of `level` in the preview named `name`: 2 for the title, then 3 and on for the body's own. */
  previewHeading(level: number, name: string): Locator {
    return this.preview.getByRole("heading", { level, name, exact: true });
  }

  /** A paragraph of the preview whose text is `text`, whole. */
  previewParagraph(text: string): Locator {
    return this.preview.getByRole("paragraph").filter({ hasText: new RegExp(`^${escape(text)}$`) });
  }

  /** How many images the body shows of the most it may: "<images> of <max> images". */
  get images(): Locator {
    return this.page.getByText(/^\d+ of [\d,]+ images$/);
  }

  /** The line over the row that says how the article will be published, while it is scheduled or edited. */
  note(text: RegExp): Locator {
    return this.page.getByRole("paragraph").filter({ hasText: text });
  }

  /** The form's alert before the row: why it was not published. */
  get alert(): Locator {
    return this.page.getByRole("alert");
  }

  /** A button of the row, or of the head, by its words. */
  button(name: string): Locator {
    return this.page.getByRole("button", { name, exact: true });
  }

  /** Publishes the article, and returns once the server has answered and the article's page shows. */
  async publish(): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/(?:articles|drafts\/[^/]+\/publish)$/ }, async () => {
      await this.button("Publish").click();
    });
    await expect(this.page).toHaveURL(/\/@[^/]+\/status\/[^/]+$/);
  }

  /** Saves the edit of the reader's article, and returns once the server has answered and the article's page shows. */
  async save(): Promise<void> {
    await answered(this.page, { methods: ["PUT"], path: /\/articles\/[^/]+$/ }, async () => {
      await this.button("Save").click();
    });
    await expect(this.page).toHaveURL(/\/@[^/]+\/status\/[^/]+$/);
  }

  /** Leaves the editor with Close: a new article written or changed is kept as a draft, an edit is discarded. */
  async close(): Promise<void> {
    await this.button("Close").click();
    await expect(this.title).toBeHidden();
  }

  /** Closes a new article, written or changed, and returns once the server has kept it as a draft. */
  async closeKept(): Promise<void> {
    await answered(this.page, { methods: ["POST", "PUT"], path: /\/drafts(?:\/[^/]+)?$/ }, async () => {
      await this.button("Close").click();
    });
    await expect(this.title).toBeHidden();
  }

  /** Chooses `item` of the drafts' menu in the editor's head. */
  async draftsMenu(item: "Save as draft" | "Drafts and scheduled posts"): Promise<void> {
    await this.button("Drafts").click();
    await this.page
      .getByRole("menu", { name: "Drafts", exact: true })
      .getByRole("menuitem", { name: item, exact: true })
      .click();
  }

  /** Keeps the article as a draft on the server, and returns once the server has answered and the editor is left. */
  async saveAsDraft(): Promise<void> {
    await answered(this.page, { methods: ["POST", "PUT"], path: /\/drafts(?:\/[^/]+)?$/ }, async () => {
      await this.draftsMenu("Save as draft");
    });
    await expect(this.title).toBeHidden();
  }

  /** Opens the reader's drafts and scheduled articles over the editor. */
  async openDrafts(): Promise<Drafts> {
    await this.draftsMenu("Drafts and scheduled posts");
    return new Drafts(this.page);
  }

  /** Sets the time the schedule's form starts with: the first whole hour an hour or more from now. */
  async setTime(): Promise<void> {
    await this.button("Schedule").click();
    const form = this.page.getByRole("dialog", { name: "Schedule", exact: true });
    await form.getByRole("button", { name: "Set time", exact: true }).click();
    await expect(form).toBeHidden();
  }

  /** Schedules the article with Schedule, which takes Publish's place once a time is set, and returns once answered. */
  async schedule(): Promise<void> {
    await answered(this.page, { methods: ["PUT"], path: /\/drafts\/[^/]+\/schedule$/ }, async () => {
      await this.button("Schedule").click();
    });
    await expect(this.title).toBeHidden();
  }
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
