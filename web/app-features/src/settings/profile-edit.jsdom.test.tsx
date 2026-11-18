// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so where Edit profile (#3200) puts the
// focus after a Save it did not send is decided here (ADR-0171 §4). What the form sends and says is
// profile-edit.dom.test.tsx.

import { expect, test } from "bun:test";
import { waitFor } from "@testing-library/react";
import { bioField, nameField, profileForm, rewrite, saveButton, type DrawnForm } from "../test/profile-form";

/** Waits until `field` has the focus; the page's own description of what has it otherwise. */
const focused = (field: () => HTMLElement, name: string) =>
  waitFor(() => {
    if (document.activeElement !== field()) {
      const has = document.activeElement;
      throw new Error(
        `test: the focus is not on ${name} but on <${has?.tagName.toLowerCase() ?? "nothing"}> "${has?.getAttribute("aria-label") ?? has?.textContent ?? ""}"`,
      );
    }
  });

/** The writes the form sent since `from`. */
const writes = ({ requests }: DrawnForm, from: number) => requests.slice(from).filter((r) => !r.startsWith("GET "));

for (const [name, field, limit] of [
  ["Display name", nameField, 30],
  ["Bio", bioField, 500],
] as const)
  test(`Save with ${name} past its limit sends nothing and moves the focus from Save to ${name}`, async () => {
    const form = await profileForm();
    const before = form.requests.length;
    await rewrite(form.user, field(), "a".repeat(limit + 1));

    await form.user.click(saveButton());

    await focused(field, name);
    expect(writes(form, before)).toEqual([]);
  });

test("Save with both past their limits moves the focus to the first of them, Display name", async () => {
  const form = await profileForm();
  const before = form.requests.length;
  await rewrite(form.user, nameField(), "a".repeat(31));
  await rewrite(form.user, bioField(), "a".repeat(501));

  await form.user.click(saveButton());

  await focused(nameField, "Display name");
  expect(writes(form, before)).toEqual([]);
});
