// SPDX-License-Identifier: AGPL-3.0-only

// Where the focus goes once an invite is created (#3207), on jsdom: the form the reader pressed in gives way to the
// invite it made, and what takes the focus there is no control (the title of the summary that says the invite is
// created); happy-dom gives the focus to any element a script asks it for, focusable or not. What the view shows is
// pinned on happy-dom (create.dom.test.tsx).

import { expect, onTestFinished, test } from "bun:test";
import { screen, waitFor } from "@testing-library/react";
import { createForm } from "../../test/invites";
import { words } from "../../test/security";

/**
 * Lets the app's reads reach the fake on jsdom until the test ends: jsdom puts its own AbortSignal in the global's
 * place, which the runtime's Request refuses, so a request is made without the signal that would cancel it.
 */
function requestsOnJsdom() {
  const native = globalThis.Request;
  class Unsignalled extends native {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(input, init === undefined ? undefined : { ...init, signal: null });
    }
  }
  globalThis.Request = Unsignalled;
  onTestFinished(() => {
    globalThis.Request = native;
  });
}

/** What has the focus, as a reader's tools name it: its role and its words. */
const focused = (): string => {
  const element = document.activeElement;
  if (element === null || element === document.body) return "nothing";
  const role =
    element.getAttribute("role") ?? (/^H[1-6]$/.test(element.tagName) ? "heading" : element.tagName.toLowerCase());
  return `${role}: ${words(element)}`;
};

test("the invite that was created takes the focus to the title that says so, and Tab goes on to Copy link", async () => {
  requestsOnJsdom();
  const { user } = await createForm();

  await user.click(screen.getByRole("button", { name: "Create invite" }));

  await waitFor(() => {
    const [title] = screen.queryAllByRole("heading", { name: "The invite is created" });
    if (title === undefined || document.activeElement !== title) throw new Error(`test: the focus is on ${focused()}`);
  });
  expect(focused()).toBe("heading: The invite is created");

  await user.tab();

  expect(focused()).toBe("button: Copy link");
});
