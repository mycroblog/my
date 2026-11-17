// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Switch } from "./switch";

test("a switch is a switch named by its label, and Space turns it on", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<Switch label="Show boosts" onChange={(on) => changes.push(on)} />);
  const toggle = screen.getByRole("switch", { name: "Show boosts", checked: false });

  await user.tab();
  expect(document.activeElement).toBe(toggle);
  await user.keyboard(" ");

  expect(changes).toEqual([true]);
  expect(screen.getByRole("switch", { name: "Show boosts", checked: true })).toBe(toggle);
});

test("a press on a switch's label turns it on, and again off", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<Switch label="Mentions and replies" onChange={(on) => changes.push(on)} />);

  await user.click(screen.getByText("Mentions and replies"));
  expect(screen.getByRole("switch", { name: "Mentions and replies", checked: true })).not.toBeNull();
  await user.click(screen.getByText("Mentions and replies"));
  expect(screen.getByRole("switch", { name: "Mentions and replies", checked: false })).not.toBeNull();

  expect(changes).toEqual([true, false]);
});

test("a switch's description describes it and is no part of its name", () => {
  render(<Switch label="Mentions and replies" description="Also when someone you do not follow writes to you" />);

  expect(
    screen.getByRole("switch", {
      name: "Mentions and replies",
      description: "Also when someone you do not follow writes to you",
    }),
  ).not.toBeNull();
});

test("a switch starts on when its default says so", () => {
  render(<Switch label="Boosts" defaultSelected />);

  expect(screen.getByRole("switch", { name: "Boosts", checked: true })).not.toBeNull();
});

test("a switch whose owner keeps its state asks for the change and stays as the owner says", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<Switch label="Favourites" isSelected={false} onChange={(on) => changes.push(on)} />);

  await user.click(screen.getByRole("switch", { name: "Favourites" }));

  expect(changes).toEqual([true]);
  expect(screen.getByRole("switch", { name: "Favourites", checked: false })).not.toBeNull();
});

test("a disabled switch is disabled, described by its reason in place of its description, and does not turn", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(
    <Switch
      label="Push notifications"
      description="On this device"
      isDisabled
      reason="Your browser blocks notifications from this site."
      onChange={(on) => changes.push(on)}
    />,
  );
  const toggle = screen.getByRole<HTMLInputElement>("switch", {
    name: "Push notifications",
    description: "Your browser blocks notifications from this site.",
    checked: false,
  });
  expect(toggle.disabled).toBe(true);

  await user.click(screen.getByText("Push notifications"));
  await user.tab();
  await user.keyboard(" ");

  expect(document.activeElement).not.toBe(toggle);
  expect(changes).toEqual([]);
  expect(toggle.checked).toBe(false);
});

test("a switch in a form posts under its name only while it is on", async () => {
  const user = userEvent.setup();
  render(
    <form aria-label="Notifications">
      <Switch label="Boosts" name="boosts" defaultSelected />
      <Switch label="Favourites" name="favourites" />
    </form>,
  );
  const posted = () => [...new FormData(screen.getByRole<HTMLFormElement>("form", { name: "Notifications" })).keys()];
  expect(posted()).toEqual(["boosts"]);

  await user.click(screen.getByRole("switch", { name: "Favourites" }));
  await user.click(screen.getByRole("switch", { name: "Boosts" }));

  expect(posted()).toEqual(["favourites"]);
});
