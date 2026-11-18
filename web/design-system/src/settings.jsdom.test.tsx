// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so it cannot tell a title that can
// take the focus from one that cannot (ADR-0171 §4). A status summary that stands in the place of what the reader
// just did (an invite is created, in the form's place) gives its title to the screen (`titleRef`), which moves the
// focus there.

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { Button } from "./button";
import { StatusSummary } from "./settings";

/** A summary between two buttons, its title given to `ref` or to nobody. */
function summary(ref?: React.RefObject<HTMLHeadingElement | null>) {
  render(
    <>
      <Button>Before</Button>
      <StatusSummary icon="accepted" title="The invite is created" about="Send its link." titleRef={ref} />
      <Button>After</Button>
    </>,
  );
  return screen.getByRole("heading", { name: "The invite is created" });
}

test("a status summary's title that a screen holds takes the focus the screen gives it, and Tab goes on to what follows", async () => {
  const user = userEvent.setup();
  const ref = createRef<HTMLHeadingElement>();
  const title = summary(ref);
  expect(ref.current === title).toBe(true);

  ref.current?.focus();
  expect(document.activeElement === title).toBe(true);
  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: "After" })).toBe(true);
});

test("a status summary's title is no stop of Tab, held or not", async () => {
  const user = userEvent.setup();
  summary(createRef<HTMLHeadingElement>());

  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: "Before" })).toBe(true);
  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: "After" })).toBe(true);
});

test("a status summary's title that no screen holds takes no focus", () => {
  const title = summary();

  title.focus();
  expect(document.activeElement === title).toBe(false);
});
