// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "@my/design-system";
import { FrameStream } from "./shell";

const labels = {
  open: "Connected",
  connecting: "Connecting…",
  reconnecting: "Reconnecting…",
  stopped: "Disconnected",
} as const;
const states = ["open", "connecting", "reconnecting", "stopped"] as const;

/**
 * The figure the mark draws for `state`: the markup of its drawing, which has no role or name of its own (the mark's
 * name is its label), so it is read as markup.
 */
function figureOf(state: (typeof states)[number]): string {
  const { unmount } = render(<FrameStream state={state} label={labels[state]} />);
  const figure = screen.getByRole("img", { name: labels[state] }).querySelector("svg")?.innerHTML ?? "";
  unmount();
  return figure;
}

test.each(states)("the stream's mark while %s is an image named by its label, which says it once", (state) => {
  render(<FrameStream state={state} label={labels[state]} />);

  const mark = screen.getByRole("img", { name: labels[state] });
  // The label is also the tooltip shown on hover, which assistive technology does not hear a second time.
  const tooltip = within(mark).getByText(labels[state]);
  expect(isInaccessible(tooltip)).toBe(true);
  expect(screen.getAllByRole("img")).toHaveLength(1);
});

test("the stream's mark draws one shape while open, another while connecting or reconnecting, and a third once stopped", () => {
  const [open, connecting, reconnecting, stopped] = states.map(figureOf);

  expect(open).not.toBe("");
  expect(connecting).toBe(reconnecting);
  expect(new Set([open, connecting, stopped]).size).toBe(3);
});

test("the stream's mark is no stop of the keyboard", async () => {
  const user = userEvent.setup();
  render(
    <>
      <Button onPress={() => undefined}>Before</Button>
      <FrameStream state="open" label={labels.open} />
      <Button onPress={() => undefined}>After</Button>
    </>,
  );

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Before" }));
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "After" }));
});
