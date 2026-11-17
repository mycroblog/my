// SPDX-License-Identifier: AGPL-3.0-only

// The drafts in the composer (ADR-0156), as its screens show them: the list goes back to the post being written, a
// save from an older copy is refused for the newer one, and a scheduled post that failed says why. The writer is Yuki.
// That a draft is kept, scheduled, deleted and published is the E2E tests' (web/e2e/specs/drafts.spec.ts).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp, words, type TestApp } from "./test/app";

/** Opens the composer of a new post from the navigation, on a screen where no other New post floats. */
async function newPost({ user }: TestApp) {
  await user.click(screen.getByRole("button", { name: "New post" }));
  return screen.findByRole("dialog", { name: "New post" });
}

/** The text box of the composer in `dialog`. */
const textOf = (dialog: HTMLElement) => within(dialog).findByRole("textbox", { name: "Text" });

/** Writes `text` in place of what the composer in `dialog` holds: pasted, as the editor takes no typed key here. */
async function replace({ user }: TestApp, dialog: HTMLElement, text: string) {
  await user.click(await textOf(dialog));
  await user.keyboard("{Control>}a{/Control}{Delete}");
  if (text !== "") await user.paste(text);
}

/** The composer of a new post once it holds `text`: opening a draft puts a new one in its place. */
const holding = (text: string) =>
  waitFor(() => {
    const dialog = screen.getByRole("dialog", { name: "New post" });
    expect(within(dialog).getByRole("textbox", { name: "Text" }).textContent).toBe(text);
    return dialog;
  });

/** Chooses `item` of the drafts' menu in the head of the composer in `dialog`. */
async function draftsMenu(
  { user }: TestApp,
  dialog: HTMLElement,
  item: "Save as draft" | "Drafts and scheduled posts",
) {
  await user.click(within(dialog).getByRole("button", { name: "Drafts" }));
  await user.click(
    await within(await screen.findByRole("menu", { name: "Drafts" })).findByRole("menuitem", { name: item }),
  );
}

/** Opens the writer's drafts and scheduled posts in the place of the composer in `dialog`. */
async function openDrafts(app: TestApp, dialog: HTMLElement) {
  await draftsMenu(app, dialog, "Drafts and scheduled posts");
  return screen.findByRole("dialog", { name: "Drafts and scheduled posts" });
}

/** The row of the drafts without a schedule that holds `text`, once the list shows it. */
const draftRow = (drafts: HTMLElement, text: string) =>
  waitFor(() => {
    const [row, ...more] = within(within(drafts).getByRole("list", { name: "Drafts" }))
      .getAllByRole("listitem")
      .filter((item) => item.textContent.includes(text));
    if (row === undefined || more.length > 0) throw new Error(`test: no one draft holds "${text}"`);
    return row;
  });

/** The button of `row` that opens its draft in the composer: the one that holds the draft's `text`. */
const opener = (row: HTMLElement, text: string) =>
  within(row)
    .getAllByRole("button")
    .filter((button) => button.textContent.includes(text));

test("the list goes back to the post being written, and once closed, New post opens the composer", async () => {
  const text = "Back check: a post still being written.";
  const app = await renderApp("/bookmarks");
  const composer = await newPost(app);
  await replace(app, composer, text);

  const drafts = await openDrafts(app, composer);
  await app.user.click(within(drafts).getByRole("button", { name: "Back to the post" }));
  const back = await holding(text);

  await replace(app, back, "");
  const again = await openDrafts(app, back);
  await app.user.click(within(again).getByRole("button", { name: "Close" }));
  await waitFor(() => {
    expect(screen.queryByRole("dialog")).toBeNull();
  });
  await newPost(app);
  expect(await holding("")).toBeDefined();
});

test("a save from an older copy of a draft is refused, and the composer opens the newer one", async () => {
  const first = "Conflict check: first version.";
  const newer = `${first} Second version from another device.`;
  const app = await renderApp("/bookmarks", {
    seed: { people: scenario.people, drafts: [{ key: "conflict", author: "yuki", text: first, language: "en" }] },
  });
  // This composer opens the draft first.
  const list = await openDrafts(app, await newPost(app));
  for (const button of opener(await draftRow(list, first), first)) await app.user.click(button);
  const stale = await holding(first);

  // Another device of the same writer reads it too, and saves a newer version.
  const path = { id: app.fake.idOf("conflict") };
  const read = await app.data.api.GET("/drafts/{id}", { params: { path } });
  const saved = await app.data.api.PUT("/drafts/{id}", {
    params: { path, header: { "If-Match": read.response.headers.get("ETag") ?? "" } },
    body: { text: newer, language: "en", visibility: "public" },
  });
  expect(saved.response.status).toBe(200);

  // This composer's save is refused: it says so, and opens the newer one.
  await replace(app, stale, `${first} Third version, written on the older copy.`);
  await draftsMenu(app, stale, "Save as draft");
  const alert = await within(stale).findByRole("alert");
  expect(words(alert)).toBe(
    "This draft was saved later on another device, so your changes here were not saved. Open the newer one",
  );
  await app.user.click(within(alert).getByRole("button", { name: "Open the newer one" }));
  const opened = await holding(newer);
  expect(within(opened).queryByRole("alert")).toBeNull();
  await app.user.click(within(opened).getByRole("button", { name: "Close" }));
  await waitFor(() => {
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // One draft, the newer one: the refused save made no copy.
  const after = await openDrafts(app, await newPost(app));
  expect(await within(after).findByRole("heading", { name: "Drafts 1" })).toBeDefined();
  expect(opener(await draftRow(after, newer), newer)).toHaveLength(1);
});

test("a scheduled post that failed says why on its draft and in the composer", async () => {
  const text = "Booked two seats. See you at the pier!";
  const app = await renderApp("/bookmarks");
  const drafts = await openDrafts(app, await newPost(app));

  const row = await draftRow(drafts, text);
  expect(
    within(row).getByText(/^Scheduled for .+, not posted: the post it replies to is no longer available$/),
  ).toBeDefined();
  for (const button of opener(row, text)) await app.user.click(button);

  const composer = await screen.findByRole("dialog", { name: "Reply" });
  expect(words(await within(composer).findByRole("alert"))).toMatch(
    /^Scheduled for .+, not posted: the post it replies to is no longer available\. Edit it if needed, then post it or schedule it again\.$/,
  );
});
