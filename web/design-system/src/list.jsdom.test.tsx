// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so it cannot tell a text that can
// take the focus from one that cannot (ADR-0171 §4).

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useRef, useState } from "react";
import { Button } from "./button";
import { StateText } from "./list";

/** A request the reader accepts: the button gives way to what it did, which takes the focus the button leaves. */
function Request() {
  const [accepted, setAccepted] = useState(false);
  const state = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (accepted) state.current?.focus();
  }, [accepted]);
  return (
    <>
      {accepted ? (
        <StateText ref={state}>Accepted</StateText>
      ) : (
        <Button
          onPress={() => {
            setAccepted(true);
          }}
        >
          Accept
        </Button>
      )}
      <Button>Next request</Button>
    </>
  );
}

test("what the reader did takes the focus its button leaves, and Tab goes on from there", async () => {
  const user = userEvent.setup();
  render(<Request />);

  await user.tab();
  await user.keyboard("{Enter}");

  expect(document.activeElement).toBe(screen.getByText("Accepted"));
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Next request" }));
});

test("what the reader did is no stop of Tab", async () => {
  const user = userEvent.setup();
  render(
    <>
      <StateText>Following</StateText>
      <Button>Next request</Button>
    </>,
  );

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Next request" }));
});
