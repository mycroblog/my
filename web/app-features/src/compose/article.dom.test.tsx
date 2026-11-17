// SPDX-License-Identifier: AGPL-3.0-only

// What the article editor's fields tell assistive technology: the summary is described by its hint, then by its error.

import { expect, test } from "bun:test";
import { articleFieldLimits } from "@my/app-data";
import { screen } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { ArticleEditor } from "./article";

const none = () => undefined;

/** The editor of a new article, and its summary field. */
function editor() {
  const user = renderFeature(
    fakeData().data,
    <ArticleEditor opening={{ kind: "new" }} onLeave={none} onOpenDraft={none} />,
  );
  return { user, summary: () => screen.findByRole<HTMLTextAreaElement>("textbox", { name: "Summary" }) };
}

/** The texts of what `field` is described by, in order. */
const describedBy = (field: HTMLElement) =>
  (field.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => document.getElementById(id)?.textContent ?? `test: nothing has the ID ${id}`);

test("the summary is described by its hint", async () => {
  const { summary } = editor();
  const field = await summary();
  const texts = describedBy(field);
  expect(texts).toHaveLength(1);
  expect(texts[0]).toMatch(/\S/);
  expect(field.getAttribute("aria-invalid")).toBeNull();
  expect(screen.getByRole("textbox", { name: "Summary", description: texts[0] ?? "" })).toBe(field);
});

test("the summary is described by its hint, then by its error when it is over its limit", async () => {
  const { user, summary } = editor();
  const field = await summary();
  const [hint] = describedBy(field);
  await user.click(field);
  await user.paste("a".repeat(articleFieldLimits.summary + 1));
  const texts = describedBy(await summary());
  expect(texts).toHaveLength(2);
  expect(texts[0]).toBe(hint);
  expect(texts[1]).toMatch(/\S/);
  expect(texts[1]).not.toBe(hint);
});
