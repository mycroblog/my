// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfirmDialog } from "./dialog";

/** A confirmation over a page that has a button of its own, and what it answered, in order. */
function confirmation() {
  const answers: string[] = [];
  render(
    <>
      <button type="button">Behind</button>
      <ConfirmDialog
        title="Delete this post?"
        cancel="Cancel"
        confirm="Delete"
        onCancel={() => answers.push("cancel")}
        onConfirm={() => answers.push("confirm")}
      >
        It cannot be undone.
      </ConfirmDialog>
    </>,
  );
  return answers;
}

test("a confirmation is an alertdialog named by its title, and Tab never takes the focus out of it", async () => {
  const user = userEvent.setup();
  confirmation();
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  expect(dialog.contains(document.activeElement)).toBe(true);

  // More presses than it has buttons, each way: the focus wraps inside it.
  for (const shift of [false, false, false, true, true, true]) {
    await user.tab({ shift });
    expect(dialog.contains(document.activeElement)).toBe(true);
  }
});

test("Escape in a confirmation cancels it", async () => {
  const user = userEvent.setup();
  const answers = confirmation();
  await screen.findByRole("alertdialog", { name: "Delete this post?" });

  await user.keyboard("{Escape}");

  expect(answers).toEqual(["cancel"]);
});
