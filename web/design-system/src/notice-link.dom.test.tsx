// SPDX-License-Identifier: AGPL-3.0-only

// A notice whose action is a link (#3216): the refusal of a restricted account leads to Account status. With `action`
// and `href` the action is a link to the address, with `onAction` alone a button as before; a text link tells its
// caller of a press and still leads to its address. The app's navigation stands in a RouterProvider, which reads where
// a link asked to go (a React Aria link stops the click, and happy-dom would follow it otherwise).

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RouterProvider } from "react-aria-components";
import type { ReactNode } from "react";
import { TextLink } from "./button";
import { UndoNotice } from "./composer";

/** `children` under the app's navigation; `went` holds where a link asked to go, in order. */
function navigating(children: ReactNode) {
  const went: string[] = [];
  render(<RouterProvider navigate={(to) => went.push(to)}>{children}</RouterProvider>);
  return { user: userEvent.setup(), went };
}

test("a notice with an action and an address offers the action as a link to the address, and no button", async () => {
  const { user, went } = navigating(
    <UndoNotice inline action="Account status" href="/settings/account/status">
      A moderation action keeps your account from posting for now.
    </UndoNotice>,
  );
  const notice = screen.getByRole("status");

  expect(within(notice).getByText("A moderation action keeps your account from posting for now.")).not.toBeNull();
  const link = within(notice).getByRole("link", { name: "Account status" });
  expect(link.getAttribute("href")).toBe("/settings/account/status");
  expect(within(notice).queryAllByRole("button")).toHaveLength(0);

  await user.click(link);
  expect(went).toEqual(["/settings/account/status"]);
});

test("the notice at the bottom of the page offers its link too", () => {
  navigating(
    <UndoNotice action="Account status" href="/settings/account/status">
      A moderation action keeps your account from interacting for now.
    </UndoNotice>,
  );

  expect(within(screen.getByRole("status")).getByRole("link", { name: "Account status" }).getAttribute("href")).toBe(
    "/settings/account/status",
  );
});

test("a notice with an action to call and no address still offers a button, and no link", async () => {
  let pressed = 0;
  const { user } = navigating(
    <UndoNotice inline action="Try again" onAction={() => pressed++}>
      The favourite did not finish.
    </UndoNotice>,
  );
  const notice = screen.getByRole("status");

  expect(within(notice).queryAllByRole("link")).toHaveLength(0);
  await user.click(within(notice).getByRole("button", { name: "Try again" }));
  expect(pressed).toBe(1);
});

test("a text link pressed tells its caller, and still leads to its address", async () => {
  let pressed = 0;
  const { user, went } = navigating(
    <p>
      Your account is restricted.{" "}
      <TextLink href="/settings/account/status" onPress={() => pressed++}>
        Account status
      </TextLink>
    </p>,
  );
  const link = screen.getByRole("link", { name: "Account status" });
  expect(link.getAttribute("href")).toBe("/settings/account/status");

  await user.click(link);

  expect(pressed).toBe(1);
  expect(went).toEqual(["/settings/account/status"]);
});
