// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so it cannot tell a heading that can
// take the focus from one that cannot (ADR-0171 §4).

import { expect, test } from "bun:test";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "./button";
import { focusFrameHeading, FrameHeading, FrameMain } from "./shell";

test("the screen's heading takes the focus the app moves to it, and Tab goes on into the screen", async () => {
  const user = userEvent.setup();
  render(
    <>
      <Button>Home</Button>
      <FrameMain>
        <FrameHeading>Notifications</FrameHeading>
        <Button>Mark all as read</Button>
      </FrameMain>
    </>,
  );
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Home" }));

  act(() => {
    focusFrameHeading(document);
  });
  expect(document.activeElement).toBe(screen.getByRole("heading", { level: 1, name: "Notifications" }));
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Mark all as read" }));
});

test("the screen's heading is no stop of Tab", async () => {
  const user = userEvent.setup();
  render(
    <FrameMain>
      <FrameHeading>Notifications</FrameHeading>
      <Button>Mark all as read</Button>
    </FrameMain>,
  );

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Mark all as read" }));
});
