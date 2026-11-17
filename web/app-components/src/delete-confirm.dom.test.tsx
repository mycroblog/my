// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation before a post is deleted: what it asks, what it says follows, and what its buttons answer. That it
// is an alert dialog which holds the focus is the design system's (dialog.dom.test.tsx).

import { expect, test } from "bun:test";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DeleteConfirm, type DeleteConfirmProps } from "./delete-confirm";
import { inLocale, inOrder } from "./test/post";

/** The confirmation of `props`, alone in the flow of the page, for a reader of `locale`, and what it answered. */
function confirmation(props: Partial<DeleteConfirmProps> = {}, locale = "en") {
  cleanup();
  const answers: string[] = [];
  inLocale(
    <DeleteConfirm
      layer={false}
      onCancel={() => answers.push("cancel")}
      onConfirm={() => answers.push("confirm")}
      {...props}
    />,
    locale,
  );
  const dialog = screen.getByRole("alertdialog");
  return { dialog, part: within(dialog), answers, said: () => dialog.textContent };
}

test("Delete… asks before it deletes, says what goes with the post, and offers Cancel then Delete", async () => {
  const user = userEvent.setup();
  const { dialog, part, answers } = confirmation();

  expect(
    screen.getByRole("alertdialog", {
      name: "Delete this post?",
      description: /^Its boosts, favourites and reactions go with it, and other servers are asked to delete it too\./,
    }) === dialog,
  ).toBe(true);
  const cancel = part.getByRole("button", { name: "Cancel" });
  const confirm = part.getByRole("button", { name: "Delete" });
  expect(inOrder(cancel, confirm)).toBe(true);
  await user.click(cancel);
  await user.click(confirm);

  expect(answers).toEqual(["cancel", "confirm"]);
});

test("Delete and redraft… says the text and images become a draft", () => {
  const { dialog, part, said } = confirmation({ redraft: true });

  expect(screen.getByRole("alertdialog", { name: "Delete and redraft this post?" }) === dialog).toBe(true);
  expect(said()).toContain("Its text and images become the draft of a new post.");
  expect(said()).not.toContain("votes of its poll");
  expect(
    inOrder(part.getByRole("button", { name: "Cancel" }), part.getByRole("button", { name: "Delete and redraft" })),
  ).toBe(true);
});

test("redrafting a post with a poll says its votes are lost and its poll becomes the draft", () => {
  const { said } = confirmation({ redraft: true, poll: true });

  expect(said()).toContain("the votes of its poll are lost");
  expect(said()).toContain("Its text and poll become the draft of a new post.");
});

test("while the deletion is in flight, its button is busy and answers nothing more", async () => {
  const user = userEvent.setup();
  const { part, answers } = confirmation({ busy: true });

  await user.click(part.getByRole("button", { name: "Delete", busy: true }));

  expect(answers).toEqual([]);
});

test("it reads in the reader's locale", () => {
  const deleting = confirmation({}, "ja");
  expect(screen.getByRole("alertdialog", { name: "この投稿を削除しますか？" }) === deleting.dialog).toBe(true);
  expect(
    inOrder(
      deleting.part.getByRole("button", { name: "キャンセル" }),
      deleting.part.getByRole("button", { name: "削除する" }),
    ),
  ).toBe(true);

  const redrafting = confirmation({ redraft: true, poll: true }, "ja");
  expect(screen.getByRole("alertdialog", { name: "この投稿を削除して書き直しますか？" }) === redrafting.dialog).toBe(
    true,
  );
  expect(redrafting.said()).toContain("投票に入った票は消え");
});
