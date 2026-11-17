// SPDX-License-Identifier: AGPL-3.0-only

// The drafts in the composer (ADR-0156), as its screens show them: the list goes back to the post being written, a
// save from an older copy is refused for the newer one. The writer is Yuki. Why a scheduled post failed is the
// feature's (compose/drafts.dom.test.tsx, compose/model.test.ts); that a draft is kept, scheduled, deleted and
// published is the E2E tests' (web/e2e/specs/drafts.spec.ts).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp, words, type TestApp } from "./test/app";
import { composerHolding, newPost, replace } from "./test/composer";

/** The composer of a new post once it holds `text`. */
const holding = (text: string) => composerHolding("New post", text);

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
  await replace(app, composer.text, text);

  const drafts = await openDrafts(app, composer.dialog);
  await app.user.click(within(drafts).getByRole("button", { name: "Back to the post" }));
  const back = await holding(text);

  await replace(app, back.text, "");
  const again = await openDrafts(app, back.dialog);
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
  const list = await openDrafts(app, (await newPost(app)).dialog);
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
  await replace(app, stale.text, `${first} Third version, written on the older copy.`);
  await draftsMenu(app, stale.dialog, "Save as draft");
  const alert = await within(stale.dialog).findByRole("alert");
  expect(words(alert)).toBe(
    "This draft was saved later on another device, so your changes here were not saved. Open the newer one",
  );
  await app.user.click(within(alert).getByRole("button", { name: "Open the newer one" }));
  const opened = (await holding(newer)).dialog;
  expect(within(opened).queryByRole("alert")).toBeNull();
  await app.user.click(within(opened).getByRole("button", { name: "Close" }));
  await waitFor(() => {
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // One draft, the newer one: the refused save made no copy.
  const after = await openDrafts(app, (await newPost(app)).dialog);
  expect(await within(after).findByRole("heading", { name: "Drafts 1" })).toBeDefined();
  expect(opener(await draftRow(after, newer), newer)).toHaveLength(1);
});
