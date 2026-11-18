// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation of signing out (#3198): it asks before the session ends, says what stays and what goes, and when
// the sign-out it was given does not finish it stays open, says so, and its button tries again. What signing out
// does (the token's revocation, what the browser forgets, where the app leaves for) is the app's, and its test.

import { expect, mock, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { SignOutDialog } from "./sign-out";

/** The confirmation over a page, with a sign-out that ends as each of `ends` says, in turn. */
function asking(...ends: ("finishes" | "fails")[]) {
  const onCancel = mock(() => undefined);
  const signOut = mock(() =>
    ends.shift() === "fails" ? Promise.reject(new Error("test: the server did not answer")) : Promise.resolve(),
  );
  const user = renderFeature(fakeData().data, <SignOutDialog onCancel={onCancel} signOut={signOut} />);
  const dialog = screen.getByRole("alertdialog", { name: "Sign out of this browser?" });
  return { user, dialog, onCancel, signOut };
}

test("it asks whether to sign out of this browser, and says what stays and what is removed", () => {
  const { dialog, signOut } = asking();

  expect(dialog.textContent).toContain("Your other browsers and apps stay signed in.");
  expect(dialog.textContent).toContain("Drafts you saved stay.");
  expect(
    within(dialog)
      .getAllByRole("button")
      .map((button) => button.textContent),
  ).toEqual(["Cancel", "Sign out"]);
  expect(within(dialog).queryAllByRole("alert")).toHaveLength(0);
  // Nothing ends before the reader answers.
  expect(signOut).toHaveBeenCalledTimes(0);
});

test("Cancel tells the page, and signs nobody out", async () => {
  const { user, dialog, onCancel, signOut } = asking();

  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

  expect(onCancel).toHaveBeenCalledTimes(1);
  expect(signOut).toHaveBeenCalledTimes(0);
});

test("Sign out signs out, once", async () => {
  const { user, dialog, onCancel, signOut } = asking("finishes");

  await user.click(within(dialog).getByRole("button", { name: "Sign out" }));

  await waitFor(() => {
    expect(signOut).toHaveBeenCalledTimes(1);
  });
  expect(onCancel).toHaveBeenCalledTimes(0);
  expect(screen.queryAllByRole("alert")).toHaveLength(0);
});

test("a sign-out that does not finish keeps the confirmation open and says so, and Sign out tries again", async () => {
  const { user, signOut } = asking("fails", "finishes");

  await user.click(screen.getByRole("button", { name: "Sign out" }));

  const failed = await screen.findByRole("alert");
  expect(failed.textContent).toBe("Signing out did not finish, so this browser may still be signed in. Try again.");
  const dialog = screen.getByRole("alertdialog", { name: "Sign out of this browser?" });
  expect(dialog.contains(failed)).toBe(true);
  expect(signOut).toHaveBeenCalledTimes(1);

  await user.click(within(dialog).getByRole("button", { name: "Sign out" }));

  await waitFor(() => {
    expect(signOut).toHaveBeenCalledTimes(2);
  });
  await waitFor(() => {
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });
});
