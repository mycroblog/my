// SPDX-License-Identifier: AGPL-3.0-only

// An article not yet published is a draft on the server (ADR-0156, ADR-0160): a new article closed is kept as one,
// which the composer's list marks an article and opens in the editor; a draft saved again is saved over, and one
// published is published from the server; an article given a time is scheduled, and reopens with its time. The writer
// is Yuki. Each test opens the editor at its address, on a server that keeps the drafts it reads: the way from one of
// these to the next is the app's navigation, which the editor's own tests hold (article-editor.dom.test.tsx: New
// article opens it, Close goes back, and its own list holds the drafts of articles alone). The drafts' own way to the
// server and back is an E2E test (web/e2e/specs/drafts.spec.ts), and what the server does with a draft of an article is
// the server's tests'.

import { expect, test } from "bun:test";
import { draftListsQuery } from "@my/app-data";
import { scenario, type FakeOptions, type FakeSeed } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp, words, type TestApp } from "./test/app";
import { heldBy, newPost, paste, replace } from "./test/composer";
import { feedOf, postOf } from "./test/posts";
import { fakeApp } from "./test/server";

type Draft = FakeSeed["drafts"][number];

/** A draft of a post of the writer's, which is no article. */
const note: Draft = { key: "note", author: "yuki", text: "Draft check: a post, not an article.", language: "en" };

/** A draft of an article of the writer's. */
const kept: Draft = {
  key: "kept",
  author: "yuki",
  text: "Written, then closed.",
  language: "en",
  article: { title: "Kept when the editor closes", summary: "", tags: [] },
};

/** An article of the writer's that waits for its time. */
const waiting: Draft = {
  key: "waiting",
  author: "yuki",
  text: "Scheduled, not yet published.",
  language: "en",
  schedule: { inDays: 3, hour: 9 },
  article: { title: "Out next week", summary: "", tags: [] },
};

/** What the editor says of an article with a time, set at a whole hour. */
const when = /Scheduled for .+:00\s[AP]M \(.+\)\s*Change time/;

/** A server where the writer keeps `drafts` and nothing else, with nothing around the reader beside the screen. */
const keeping = (...drafts: Draft[]): FakeOptions => ({
  viewer: "yuki",
  around: "none",
  seed: { people: scenario.people, drafts },
});

/**
 * The one of what `found` finds, once the screen shows it. A wait that finds nothing says so in a line: a query that
 * throws describes the whole page each time it is asked again.
 */
const shown = (what: string, found: () => HTMLElement[]) =>
  waitFor(() => {
    const [one, ...more] = found();
    if (one === undefined || more.length > 0) throw new Error(`test: ${String(more.length + 1)} of ${what}`);
    return one;
  });

const title = () => screen.getByRole<HTMLInputElement>("textbox", { name: "Title" });
/** The editor's body, once its editor has come. */
const body = () => shown("the body", () => screen.queryAllByRole("textbox", { name: "Body" }));

/** Opens the editor of a new article at its address, and writes the article titled `named` whose body is `text`. */
async function writing(named: string, text: string): Promise<TestApp> {
  const app = await renderApp("/articles/new", keeping());
  // Pasted, a title being one input: each key typed draws the editor again.
  await app.user.click(title());
  await app.user.paste(named);
  await paste(app, await body(), text);
  return app;
}

/** Waits until the editor holds `draft`: its title, and its text as the body. */
const holding = (draft: Draft) =>
  waitFor(() => {
    const named = screen.queryAllByRole<HTMLInputElement>("textbox", { name: "Title" }).map((field) => field.value);
    if (named[0] !== draft.article?.title) throw new Error(`test: the title is "${String(named[0])}"`);
    const held = screen.queryAllByRole("textbox", { name: "Body" }).map(heldBy);
    if (held[0] !== draft.text) throw new Error(`test: the body holds "${String(held[0])}"`);
  });

/** Opens the editor at the address of the draft `draft`, which the writer keeps with `others`, once it holds it. */
async function editing(draft: Draft, ...others: Draft[]): Promise<TestApp & { id: string }> {
  const on = fakeApp(keeping(draft, ...others));
  const id = on.fake.idOf(draft.key);
  const app = await renderApp(`/articles/new?draft=${id}`, { on });
  await holding(draft);
  return { ...app, id };
}

/** Waits until the editor is left: no title to write. */
const left = () =>
  waitFor(() => {
    if (screen.queryAllByRole("textbox", { name: "Title" }).length > 0) throw new Error("test: the editor is open");
  });

/** The rows of the list named `name` in `drafts` that hold `text`; none for no such list. */
const rows = (drafts: HTMLElement, name: "Drafts" | "Scheduled posts", text: string) =>
  within(drafts)
    .queryAllByRole("list", { name })
    .flatMap((list) => within(list).getAllByRole("listitem"))
    .filter((row) => row.textContent.includes(text));

/** Whether `row` is marked a draft of an article. */
const marked = (row: HTMLElement) => within(row).queryAllByRole("img", { name: "An article" }).length === 1;

test("a new article closed is kept on the server as a draft of an article, with its title", async () => {
  const app = await writing("Kept when the editor closes", "Written, then closed.");
  const saved = app.fake.script("POST /drafts");

  await app.user.click(screen.getByRole("button", { name: "Close" }));

  expect((await saved.served()).body).toMatchObject({
    text: "Written, then closed.",
    article: { title: "Kept when the editor closes" },
  });
  await left();
  expect(saved.seen).toHaveLength(1);
});

/** Opens the composer from the rail, and the list of the writer's drafts in its place. */
async function composersList(app: TestApp): Promise<HTMLElement> {
  const { dialog } = await newPost(app);
  await app.user.click(within(dialog).getByRole("button", { name: "Drafts" }));
  await app.user.click(
    await shown("the drafts' list in the menu", () =>
      screen.queryAllByRole("menuitem", { name: "Drafts and scheduled posts" }),
    ),
  );
  return shown("the list of the drafts", () => screen.queryAllByRole("dialog", { name: "Drafts and scheduled posts" }));
}

test("the composer's list marks the drafts of articles, a scheduled one too, and opens one in the editor", async () => {
  const app = await renderApp("/lists", keeping(note, kept, waiting));
  const list = await composersList(app);
  const article = await shown("the article's draft", () => rows(list, "Drafts", "Kept when the editor closes"));

  expect(within(list).getByRole("heading", { name: "Drafts 2" })).toBeDefined();
  expect(within(list).getByRole("heading", { name: "Scheduled 1" })).toBeDefined();
  expect(marked(article)).toBe(true);
  expect(rows(list, "Drafts", note.text).map(marked)).toEqual([false]);
  expect(rows(list, "Scheduled posts", "Out next week").map(marked)).toEqual([true]);

  const [opener] = within(article)
    .getAllByRole("button")
    .filter((button) => button.textContent.includes("Kept when the editor closes"));
  await app.user.click(opener ?? article);
  await holding(kept);
  expect(app.path()).toBe(`/articles/new?draft=${app.fake.idOf("kept")}`);
});

test("a draft of an article saved again is saved over with what changed, and the editor is left", async () => {
  const app = await editing(kept);
  const changed = "Written, then closed. Then changed and saved.";
  await replace(app, await body(), changed);
  const saved = app.fake.script("PUT /drafts/{id}");

  await app.user.click(screen.getByRole("button", { name: "Drafts" }));
  await app.user.click(
    await shown("Save as draft", () => screen.queryAllByRole("menuitem", { name: "Save as draft" })),
  );

  const asked = await saved.served();
  expect(asked.url.pathname).toEndWith(`/drafts/${app.id}`);
  expect(asked.body).toMatchObject({ text: changed, article: { title: "Kept when the editor closes" } });
  await left();
  expect(saved.seen).toHaveLength(1);
});

test("a draft of an article is published from the server, reads on its page, and leaves the list of the drafts", async () => {
  const app = await editing(kept, note);
  // The writer's drafts as the app holds them before: the list read after the publication is no first read.
  const before = await app.data.queryClient.query(draftListsQuery(app.data));
  expect(before.drafts.map((draft) => draft.id).sort()).toEqual([app.id, app.fake.idOf(note.key)].sort());
  const published = app.fake.script("POST /drafts/{id}/publish");

  await app.user.click(screen.getByRole("button", { name: "Publish" }));

  expect((await published.served()).url.pathname).toEndWith(`/drafts/${app.id}/publish`);
  await waitFor(() => {
    if (!/^\/@yuki\/status\/[^/]+$/.test(app.path())) throw new Error(`test: the app is at ${app.path()}`);
  });
  const article = postOf(await feedOf("Thread"), "Yuki 雪", "Kept when the editor closes");
  expect(within(article).getByRole("heading", { level: 1, name: "Kept when the editor closes" })).toBeDefined();
  expect(published.seen).toHaveLength(1);

  // Published, the draft is gone from the list, with no reload; the post's stays.
  const list = await composersList(app);
  await shown("the draft of the post", () => rows(list, "Drafts", note.text));
  expect(rows(list, "Drafts", "Kept when the editor closes")).toHaveLength(0);
  expect(within(list).getByRole("heading", { name: "Drafts 1" })).toBeDefined();
});

test("an article given a time says when, offers Schedule in Publish's place, and is scheduled on the server", async () => {
  const app = await writing("Out next week", "Scheduled, not yet published.");

  // A time set: the tool is on, the line over the row says when, and Schedule takes Publish's place.
  await app.user.click(screen.getByRole("button", { name: "Schedule" }));
  const form = await shown("the schedule's form", () => screen.queryAllByRole("dialog", { name: "Schedule" }));
  await app.user.click(within(form).getByRole("button", { name: "Set time" }));
  await shown("the schedule tool, on", () =>
    screen.queryAllByRole("button", { name: "Change the scheduled time", pressed: true }),
  );
  expect(words(document.body)).toMatch(when);
  expect(screen.queryAllByRole("button", { name: "Publish" })).toHaveLength(0);

  const saved = app.fake.script("POST /drafts");
  const scheduled = app.fake.script("PUT /drafts/{id}/schedule");
  await app.user.click(screen.getByRole("button", { name: "Schedule" }));

  await scheduled.served();
  expect(saved.seen.map((asked) => asked.body)).toMatchObject([
    { text: "Scheduled, not yet published.", article: { title: "Out next week" } },
  ]);
  await left();
  expect(scheduled.seen).toHaveLength(1);
});

test("a scheduled article reopens in the editor with its time, to be scheduled again and not published", async () => {
  await editing(waiting);

  expect(words(document.body)).toMatch(when);
  expect(screen.getByRole("button", { name: "Change the scheduled time", pressed: true })).toBeDefined();
  expect(screen.getByRole("button", { name: "Schedule" })).toBeDefined();
  expect(screen.queryAllByRole("button", { name: "Publish" })).toHaveLength(0);
});
