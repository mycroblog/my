// SPDX-License-Identifier: AGPL-3.0-only

// An article not yet published is a draft on the server (ADR-0156, ADR-0160): a new article closed is kept as one,
// which the composer's list marks an article and opens in the editor, the editor's own list holds the articles alone,
// and a draft saved again is published from the server; an article given a time is scheduled, waits among the scheduled
// posts and reopens with its time. The writer is Yuki. The drafts' own way to the server and back is an E2E test
// (web/e2e/specs/drafts.spec.ts), and what the server does with a draft of an article is the server's tests'.

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp, words, type TestApp } from "./test/app";
import { heldBy, newPost, paste, replace } from "./test/composer";
import { feedOf, postOf } from "./test/posts";
import { rail } from "./test/viewport";

/** A draft of a post of the writer's, which is no article. */
const note = "Draft check: a post, not an article.";

/** A server where the writer keeps `drafts`, and nothing else. */
const keeping = (drafts: readonly string[]) => ({
  viewer: "yuki",
  seed: {
    people: scenario.people,
    drafts: drafts.map((text, i) => ({ key: `draft-${String(i)}`, author: "yuki", text, language: "en" })),
  },
});

const title = () => screen.getByRole<HTMLInputElement>("textbox", { name: "Title" });
/** The editor's body, once its editor has come. */
const body = () => screen.findByRole("textbox", { name: "Body" });

/** Waits until the editor of `heading` ("New article") shows, and returns its body. */
async function editor(heading = "New article") {
  expect(await screen.findByRole("heading", { level: 1, name: heading })).toBeDefined();
  return body();
}

/** Writes `named` as the title of the article in the editor, once it shows: pasted, a title being one input. */
async function entitle({ user }: TestApp, named: string) {
  await editor();
  await user.click(title());
  await user.paste(named);
}

/** Waits until the editor is left: no title to write. */
const left = () =>
  waitFor(() => {
    if (screen.queryAllByRole("textbox", { name: "Title" }).length > 0) throw new Error("test: the editor is open");
  });

/** Waits until the editor holds the article titled `named` whose body is `text`. */
const holds = (named: string, text: string) =>
  waitFor(async () => {
    if (title().value !== named) throw new Error(`test: the title is "${title().value}"`);
    const held = heldBy(await body());
    if (held !== text) throw new Error(`test: the body holds "${held}"`);
  });

/** Opens the list of the drafts from the drafts' menu in `scope` (the composer's dialog, or the editor's screen). */
async function openDrafts({ user }: TestApp, scope: HTMLElement = document.body) {
  await user.click(within(scope).getByRole("button", { name: "Drafts" }));
  await user.click(
    await within(await screen.findByRole("menu", { name: "Drafts" })).findByRole("menuitem", {
      name: "Drafts and scheduled posts",
    }),
  );
  return screen.findByRole("dialog", { name: "Drafts and scheduled posts" });
}

/** The rows of the list named `name` in `drafts` that hold `text`, once the list shows; none for no such list. */
const rows = (drafts: HTMLElement, name: "Drafts" | "Scheduled posts", text: string) =>
  within(drafts)
    .queryAllByRole("list", { name })
    .flatMap((list) => within(list).getAllByRole("listitem"))
    .filter((row) => row.textContent.includes(text));

/** The one row of the list named `name` that holds `text`, once the list shows it. */
const row = (drafts: HTMLElement, name: "Drafts" | "Scheduled posts", text: string) =>
  waitFor(() => {
    const [found, ...more] = rows(drafts, name, text);
    if (found === undefined || more.length > 0) throw new Error(`test: no one row of ${name} holds "${text}"`);
    return found;
  });

/** Opens the draft of `found` with the button of the row that says its `text`. */
async function open({ user }: TestApp, found: HTMLElement, text: string) {
  const [opener] = within(found)
    .getAllByRole("button")
    .filter((button) => button.textContent.includes(text));
  if (opener === undefined) throw new Error(`test: no button of the row opens "${text}"`);
  await user.click(opener);
}

/** Whether `found` is marked a draft of an article. */
const marked = (found: HTMLElement) => within(found).queryAllByRole("img", { name: "An article" }).length === 1;

test("a new article closed is kept as a draft of an article, which reopens in the editor, is saved again and is published", async () => {
  const named = "Kept when the editor closes";
  const app = await renderApp("/lists", keeping([note]));
  await app.user.click(within(rail()).getByRole("button", { name: "New article" }));
  await entitle(app, named);
  await paste(app, await editor(), "Written, then closed.");

  // Closed, it is kept on the server as a draft with its title, and the app is back where the editor was opened.
  const kept = app.fake.script("POST /drafts");
  await app.user.click(screen.getByRole("button", { name: "Close" }));
  expect(await within(screen.getByRole("main")).findByRole("heading", { level: 1, name: "Lists" })).toBeDefined();
  expect(kept.seen.map((asked) => asked.body)).toMatchObject([
    { text: "Written, then closed.", article: { title: named } },
  ]);

  // The composer's list holds both drafts, and marks the article's, which opens in the editor.
  const all = await openDrafts(app, (await newPost(app)).dialog);
  expect(await within(all).findByRole("heading", { name: "Drafts 2" })).toBeDefined();
  expect(marked(await row(all, "Drafts", named))).toBe(true);
  expect(marked(await row(all, "Drafts", note))).toBe(false);
  await open(app, await row(all, "Drafts", named), named);
  await holds(named, "Written, then closed.");
  expect(app.path()).toMatch(/^\/articles\/new\?draft=[^&/]+$/);

  // The editor's own list holds the drafts of articles alone.
  const own = await openDrafts(app);
  expect(await within(own).findByRole("heading", { name: "Drafts 1" })).toBeDefined();
  expect(await row(own, "Drafts", named)).toBeDefined();
  expect(rows(own, "Drafts", note)).toHaveLength(0);
  await app.user.click(within(own).getByRole("button", { name: "Close" }));

  // Saved again, the same draft is saved over, and the editor is left.
  const changed = "Written, then closed. Then changed and saved.";
  await replace(app, await body(), changed);
  const saved = app.fake.script("PUT /drafts/{id}");
  await app.user.click(screen.getByRole("button", { name: "Drafts" }));
  await app.user.click(
    await within(await screen.findByRole("menu", { name: "Drafts" })).findByRole("menuitem", { name: "Save as draft" }),
  );
  await left();
  expect(saved.seen.map((asked) => asked.body)).toMatchObject([{ text: changed, article: { title: named } }]);

  // Reopened from the editor's list, it is published from the server, and reads on its page.
  await app.navigate("/articles/new");
  await editor();
  await open(app, await row(await openDrafts(app), "Drafts", named), named);
  await holds(named, changed);
  const published = app.fake.script("POST /drafts/{id}/publish");
  await app.user.click(screen.getByRole("button", { name: "Publish" }));
  await waitFor(() => {
    if (!/^\/@yuki\/status\/[^/]+$/.test(app.path())) throw new Error(`test: the app is at ${app.path()}`);
  });
  const article = postOf(await feedOf("Thread"), "Yuki 雪", named);
  expect(within(article).getByRole("heading", { level: 1, name: named })).toBeDefined();
  expect(published.seen).toHaveLength(1);

  // Published, the draft is gone; the post's stays.
  const after = await openDrafts(app, (await newPost(app)).dialog);
  expect(await row(after, "Drafts", note)).toBeDefined();
  expect(rows(after, "Drafts", named)).toHaveLength(0);
});

test("a writer schedules an article, which waits among the scheduled posts, marked an article, and reopens with its time", async () => {
  const named = "Out next week";
  const when = /Scheduled for .+:00\s[AP]M \(.+\)\s*Change time/;
  const app = await renderApp("/lists", keeping([]));
  await app.user.click(within(rail()).getByRole("button", { name: "New article" }));
  await entitle(app, named);
  await paste(app, await editor(), "Scheduled, not yet published.");

  // A time set: the tool is on, the line over the row says when, and Schedule takes Publish's place.
  await app.user.click(screen.getByRole("button", { name: "Schedule" }));
  const form = await screen.findByRole("dialog", { name: "Schedule" });
  await app.user.click(within(form).getByRole("button", { name: "Set time" }));
  expect(await screen.findByRole("button", { name: "Change the scheduled time", pressed: true })).toBeDefined();
  expect(words(document.body)).toMatch(when);
  expect(screen.queryAllByRole("button", { name: "Publish" })).toHaveLength(0);

  const scheduled = app.fake.script("PUT /drafts/{id}/schedule");
  await app.user.click(screen.getByRole("button", { name: "Schedule" }));
  await left();
  expect(scheduled.seen).toHaveLength(1);
  expect(await within(screen.getByRole("main")).findByRole("heading", { level: 1, name: "Lists" })).toBeDefined();

  // It waits among the scheduled posts, marked an article, and reopens in the editor with its time.
  const all = await openDrafts(app, (await newPost(app)).dialog);
  expect(await within(all).findByRole("heading", { name: "Scheduled 1" })).toBeDefined();
  const waiting = await row(all, "Scheduled posts", named);
  expect(marked(waiting)).toBe(true);
  await open(app, waiting, named);
  await holds(named, "Scheduled, not yet published.");
  expect(words(document.body)).toMatch(when);
  expect(screen.getByRole("button", { name: "Schedule" })).toBeDefined();
  expect(screen.queryAllByRole("button", { name: "Publish" })).toHaveLength(0);
});
