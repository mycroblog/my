// SPDX-License-Identifier: AGPL-3.0-only

// A writer writes an article in its own editor (ADR-0160): Publish waits for a cover the server still processes and
// for the preview of the body as it is, the reader's drafts of articles open in it, a visitor is asked to sign in, and
// New article in the main navigation opens it.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp } from "./test/app";
import { paste } from "./test/composer";
import { chooseFiles, imageFile } from "./test/files";
import { feedOf, postOf } from "./test/posts";
import { onAPhone, rail } from "./test/viewport";

/** The texts of `elements`, in their order. */
const texts = (elements: HTMLElement[]) => elements.map((element) => element.textContent);

/** The editor's body, once its editor has come. */
const body = () => screen.findByRole("textbox", { name: "Body" });

/**
 * The paragraphs of the article as it will be published. Without a layout the preview renders in each of its places,
 * beside the form and under it: the first one is read.
 */
function previewed(): string[] {
  const [preview] = screen.getAllByRole("article");
  if (preview === undefined) throw new Error("test: the editor shows no preview");
  return texts(within(preview).getAllByRole("paragraph"));
}

/** Waits until the app is at the page of a post of `username`, and returns the thread it shows. */
async function published(app: { path: () => string }, username: string): Promise<HTMLElement> {
  await waitFor(() => {
    if (!new RegExp(`^/@${username}/status/[^/]+$`).test(app.path()))
      throw new Error(`test: the app is at ${app.path()}, not at a post's page`);
  });
  return feedOf("Thread");
}

test("Publish pressed while the cover is processing says to wait, and publishes once it is processed", async () => {
  const title = "A cover still processing";
  const app = await renderApp("/articles/new", { viewer: "yuki", uploads: "processing" });
  await app.user.type(screen.getByRole("textbox", { name: "Title" }), title);
  await paste(app, await body(), "Its cover is not processed yet.");
  // The server's answers that the upload is processed are held: the cover stays processing.
  const processed = app.fake.script("GET /media/{id}", { hold: "request" });
  const sent = app.fake.script("POST /articles");
  const details = within(screen.getByRole("region", { name: "About the article" }));
  await chooseFiles(app, details.getByRole("button", { name: "Add a cover image" }), imageFile("cover.png"));
  expect(await details.findByRole("button", { name: "Remove the cover image" })).toBeDefined();

  await app.user.click(screen.getByRole("button", { name: "Publish" }));
  // Its words are not settled yet: the alert, and the editor kept.
  expect(texts(screen.getAllByRole("alert")).join("")).toMatch(/^Wait until the cover image has uploaded/);
  expect(screen.getByRole("heading", { level: 1, name: "New article" })).toBeDefined();
  expect(sent.seen).toHaveLength(0);

  // Once the server says the cover is processed, the alert goes and Publish publishes it.
  await processed.arrived();
  processed.release();
  await waitFor(() => {
    if (texts(screen.queryAllByRole("alert")).join("") !== "") throw new Error("test: the editor still says to wait");
  });
  await app.user.click(screen.getByRole("button", { name: "Publish" }));
  const article = postOf(await published(app, "yuki"), "Yuki 雪", title);
  expect(within(article).getByRole("heading", { level: 1, name: title })).toBeDefined();
  expect(sent.seen).toHaveLength(1);
});

test("Publish pressed before the preview of the body as it is asks for it at once, busy, and decides on it", async () => {
  const title = "Published before its preview";
  const nothing = "Nothing written yet. Write the body, and its preview shows here.";
  const app = await renderApp("/articles/new", { viewer: "yuki" });
  await app.user.type(screen.getByRole("textbox", { name: "Title" }), title);
  // The server's previews are held: the one shown stays the one of no body.
  const previews = app.fake.script("POST /articles/preview", { hold: "response" });
  const sent = app.fake.script("POST /articles");
  await paste(app, await body(), "First paragraph. Then a second sentence.");
  const publish = screen.getByRole("button", { name: "Publish" });

  await app.user.click(publish);
  // The preview is asked for at the press, before the rest that follows the writing brings it.
  expect(previews.seen.map((asked) => asked.body)).toContainEqual({ text: "First paragraph. Then a second sentence." });
  expect(publish.getAttribute("aria-busy")).toBe("true");
  expect(previewed()).toContain(nothing);
  expect(sent.seen).toHaveLength(0);

  previews.release();
  const article = postOf(await published(app, "yuki"), "Yuki 雪", title);
  expect(texts(within(article).getAllByRole("paragraph"))).toContain("First paragraph. Then a second sentence.");
  expect(sent.seen).toHaveLength(1);
});

test("the reader's drafts of articles are listed as articles and open in the editor, the summary made from the body", async () => {
  const app = await renderApp("/articles/new", { viewer: "yuki" });
  await app.user.click(screen.getByRole("button", { name: "Drafts" }));
  await app.user.click(
    within(await screen.findByRole("menu", { name: "Drafts" })).getByRole("menuitem", {
      name: "Drafts and scheduled posts",
    }),
  );
  const list = within(await screen.findByRole("dialog", { name: "Drafts and scheduled posts" }));
  /** The rows of the list named `name` that hold `text`. */
  const rows = async (name: string, text: string) =>
    within(await list.findByRole("list", { name }))
      .getAllByRole("listitem")
      .filter((row) => row.textContent.includes(text));

  const [draft] = await rows("Drafts", "A photo walk along the north ridge");
  expect(draft).toBeDefined();
  expect(await rows("Scheduled posts", "Notes from the November photo walk")).toHaveLength(1);
  // Yuki's drafts of posts are not the editor's.
  expect(await rows("Drafts", "Walked along the river this morning.")).toHaveLength(0);

  const [opener] = within(draft ?? document.body)
    .getAllByRole("button")
    .filter((button) => button.textContent.includes("A photo walk along the north ridge"));
  await app.user.click(opener ?? document.body);
  await waitFor(() => {
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Title" }).value).toBe(
      "A photo walk along the north ridge",
    );
  });
  const details = within(screen.getByRole("region", { name: "About the article" }));
  expect(details.getByRole<HTMLTextAreaElement>("textbox", { name: "Summary" }).value).toBe("");
  expect(details.getByRole("button", { name: "Remove #photowalk" })).toBeDefined();
  expect(details.getByRole("button", { name: "Remove #hiking" })).toBeDefined();
  // The preview is the server's rendering of the body: its headings one level under the title's, and the summary made
  // from its start.
  await waitFor(() => {
    if (!previewed().some((paragraph) => /^The trail starts behind the old station\. .*…$/.test(paragraph)))
      throw new Error("test: the preview does not show the summary made from the body");
  });
  const [preview] = screen.getAllByRole("article");
  const shown = within(preview ?? document.body);
  expect(shown.getByRole("heading", { level: 2, name: "A photo walk along the north ridge" })).toBeDefined();
  expect(texts(shown.getAllByRole("heading", { level: 3 }))).toEqual(["Before you go", "The route"]);
  expect(texts(shown.getAllByRole("listitem"))).toEqual([
    "Up the stone steps to the shrine",
    "Along the ridge to the lookout",
    "Down through the cedar forest",
  ]);
});

test("without a session, the editor asks the visitor to sign in", async () => {
  await renderApp("/articles/new", { visitor: true });

  expect(screen.getByRole("link", { name: "Sign in" })).toBeDefined();
  expect(screen.queryByRole("textbox", { name: "Title" })).toBeNull();
});

test("New article under New post in the main navigation opens the editor, and Close goes back", async () => {
  const app = await renderApp("/lists", { viewer: "yuki" });
  await app.user.click(within(rail()).getByRole("button", { name: "New article" }));

  expect(await screen.findByRole("heading", { level: 1, name: "New article" })).toBeDefined();
  expect(app.path()).toBe("/articles/new");

  // Back where it was opened from.
  await app.user.click(screen.getByRole("button", { name: "Close" }));
  expect(await within(screen.getByRole("main")).findByRole("heading", { level: 1, name: "Lists" })).toBeDefined();
  expect(app.path()).toBe("/lists");
});

test("on a phone the floating New post's menu offers New article, which opens the editor", async () => {
  onAPhone();
  // The new post floats on the timelines and the profiles: the local timeline is the one that shows least.
  const app = await renderApp("/local", { viewer: "yuki" });
  // The floating New post is the one outside the rail, the last of the two that render without a layout; a right
  // click opens its menu, as a long press does.
  const floating = screen.getAllByRole("button", { name: "New post" }).at(-1);
  if (floating === undefined || rail().contains(floating))
    throw new Error("test: the shell shows no floating New post");

  await app.user.pointer({ keys: "[MouseRight]", target: floating });
  const menu = await screen.findByRole("menu", { name: "New" });
  expect(screen.queryByRole("dialog", { name: "New post" })).toBeNull();
  await app.user.click(within(menu).getByRole("menuitem", { name: "New article" }));

  expect(await screen.findByRole("heading", { level: 1, name: "New article" })).toBeDefined();
  expect(app.path()).toBe("/articles/new");
});
