// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button, LinkButton, TextButton, TextLink } from "./button";

test("a button is a button named by its label, and a click, Enter and Space each press it once", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(<Button onPress={() => presses++}>Save draft</Button>);
  const button = screen.getByRole("button", { name: "Save draft", busy: false });

  await user.tab();
  expect(document.activeElement).toBe(button);
  await user.keyboard("{Enter}");
  expect(presses).toBe(1);
  await user.keyboard(" ");
  expect(presses).toBe(2);
  await user.click(button);
  expect(presses).toBe(3);
});

test("a button submits its form only when it is of type submit", async () => {
  const user = userEvent.setup();
  let submits = 0;
  render(
    <form
      aria-label="Post"
      onSubmit={(event) => {
        event.preventDefault();
        submits++;
      }}
    >
      <Button>Save draft</Button>
      <Button type="submit" kind="primary">
        Post
      </Button>
    </form>,
  );

  await user.click(screen.getByRole("button", { name: "Save draft" }));
  expect(submits).toBe(0);
  await user.click(screen.getByRole("button", { name: "Post" }));
  expect(submits).toBe(1);
});

test("a busy button says it is busy under its label, keeps the focus, and ignores presses", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(
    <Button kind="primary" busy onPress={() => presses++}>
      Post
    </Button>,
  );
  const button = screen.getByRole("button", { name: "Post", busy: true });
  expect(button.getAttribute("aria-disabled")).toBe("true");

  await user.tab();
  expect(document.activeElement).toBe(button);
  await user.keyboard("{Enter}");
  await user.keyboard(" ");
  await user.click(button);
  expect(presses).toBe(0);
});

test("a disabled button is disabled: Tab passes it and a click does not press it", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(
    <>
      <Button isDisabled onPress={() => presses++}>
        Cancel
      </Button>
      <Button>Save draft</Button>
    </>,
  );
  const disabled = screen.getByRole<HTMLButtonElement>("button", { name: "Cancel" });
  expect(disabled.disabled).toBe(true);

  await user.click(disabled);
  expect(presses).toBe(0);
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Save draft" }));
});

// A press that began on the button while it was disabled once took the next press with it (d34afabff).
test("a press on a button while it is disabled loses no press once it is enabled", async () => {
  const user = userEvent.setup();
  let presses = 0;
  const vote = (disabled: boolean) => (
    <Button isDisabled={disabled} onPress={() => presses++}>
      Vote
    </Button>
  );
  const { rerender } = render(vote(true));

  await user.click(screen.getByRole("button", { name: "Vote" }));
  expect(presses).toBe(0);
  rerender(vote(false));
  await user.click(screen.getByRole("button", { name: "Vote" }));

  expect(presses).toBe(1);
});

test("a button tells assistive technology the keys that run its action", () => {
  render(<Button aria-keyshortcuts="Control+Enter">Post</Button>);

  expect(screen.getByRole("button", { name: "Post" }).getAttribute("aria-keyshortcuts")).toBe("Control+Enter");
});

test("a link button is a link named by its label that leads to its href", async () => {
  const user = userEvent.setup();
  render(
    <LinkButton href="/auth/example" kind="primary">
      Sign in with Example
    </LinkButton>,
  );
  const link = screen.getByRole("link", { name: "Sign in with Example" });
  expect(link.getAttribute("href")).toBe("/auth/example");

  await user.tab();
  expect(document.activeElement).toBe(link);
});

test("a text button is a button named by its words, and a click, Enter and Space each press it once", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(
    <p>
      Translated from Japanese. <TextButton onPress={() => presses++}>Show original</TextButton>
    </p>,
  );
  const button = screen.getByRole("button", { name: "Show original" });

  await user.tab();
  expect(document.activeElement).toBe(button);
  await user.keyboard("{Enter}");
  expect(presses).toBe(1);
  await user.keyboard(" ");
  expect(presses).toBe(2);
  await user.click(button);
  expect(presses).toBe(3);
});

test("a disabled text button is disabled and a click does not press it", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(
    <TextButton isDisabled onPress={() => presses++}>
      Try again
    </TextButton>,
  );
  const button = screen.getByRole<HTMLButtonElement>("button", { name: "Try again" });
  expect(button.disabled).toBe(true);

  await user.click(button);
  expect(presses).toBe(0);
});

test("a text link is a link named by its words that leads to its href", async () => {
  const user = userEvent.setup();
  render(
    <p>
      Sign in to see this screen. <TextLink href="/sign-in">Sign in</TextLink>
    </p>,
  );
  const link = screen.getByRole("link", { name: "Sign in" });
  expect(link.getAttribute("href")).toBe("/sign-in");

  await user.tab();
  expect(document.activeElement).toBe(link);
});
