// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { SaveState } from "@my/design-system";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { Switch } from "./switch";

test("a switch is a switch named by its label, and Space turns it on", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<Switch label="Show boosts" onChange={(on) => changes.push(on)} save={null} />);
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
  render(<Switch label="Mentions and replies" onChange={(on) => changes.push(on)} save={null} />);

  await user.click(screen.getByText("Mentions and replies"));
  expect(screen.getByRole("switch", { name: "Mentions and replies", checked: true })).not.toBeNull();
  await user.click(screen.getByText("Mentions and replies"));
  expect(screen.getByRole("switch", { name: "Mentions and replies", checked: false })).not.toBeNull();

  expect(changes).toEqual([true, false]);
});

test("a switch's description describes it and is no part of its name", () => {
  render(
    <Switch label="Mentions and replies" description="Also when someone you do not follow writes to you" save={null} />,
  );

  expect(
    screen.getByRole("switch", {
      name: "Mentions and replies",
      description: "Also when someone you do not follow writes to you",
    }),
  ).not.toBeNull();
});

test("a switch starts on when its default says so", () => {
  render(<Switch label="Boosts" defaultSelected save={null} />);

  expect(screen.getByRole("switch", { name: "Boosts", checked: true })).not.toBeNull();
});

test("a switch whose owner keeps its state asks for the change and stays as the owner says", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<Switch label="Favourites" isSelected={false} onChange={(on) => changes.push(on)} save={null} />);

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
      save={null}
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

test("a switch's save state stands in a status region that is there from the start, and describes the switch only while a state stands", () => {
  const toggle = (save: ReactNode) => (
    <Switch label="Show new posts as they arrive" description="Off, a button counts them" save={save} />
  );
  const described = (description: string | RegExp) =>
    screen.queryAllByRole("switch", { name: "Show new posts as they arrive", description });
  const { rerender } = render(toggle(null));
  const place = screen.getByRole("status");
  expect(place.textContent).toBe("");
  expect(described("Off, a button counts them")).toHaveLength(1);

  rerender(toggle(<SaveState state="saved" />));

  // The same region, so that what comes into it is read.
  expect(screen.getByRole("status") === place).toBe(true);
  expect(place.textContent).toBe("Saved");
  expect(described(/^Off, a button counts them\s*Saved$/)).toHaveLength(1);

  rerender(toggle(null));

  expect(screen.getByRole("status") === place).toBe(true);
  expect(place.textContent).toBe("");
  expect(described("Off, a button counts them")).toHaveLength(1);
});

test("a switch with no description is described by its save state alone, and by nothing at rest", () => {
  const toggle = (save: ReactNode) => <Switch label="Boosts" save={save} />;
  const { rerender } = render(toggle(null));
  expect(screen.getByRole("switch", { name: "Boosts" }).getAttribute("aria-describedby")).toBeNull();

  rerender(toggle(<SaveState state="saving" />));

  expect(screen.getByRole("switch", { name: "Boosts", description: "Saving…" })).not.toBeNull();
});

test("Try again of a switch's failed save puts the focus on the switch, then asks once", async () => {
  const user = userEvent.setup();
  // Whether the focus was on the switch when the owner was asked: the owner then takes the failed state away, and
  // the button with it.
  const asked: boolean[] = [];
  render(
    <Switch
      label="Boosts"
      save={
        <SaveState
          state="failed"
          onRetry={() => asked.push(document.activeElement === screen.getByRole("switch", { name: "Boosts" }))}
        />
      }
    />,
  );

  await user.click(screen.getByRole("button", { name: "Try again" }));

  expect(asked).toEqual([true]);
  expect(document.activeElement === screen.getByRole("switch", { name: "Boosts" })).toBe(true);
  // The press was on the state, not on the switch: it stays as it was.
  expect(screen.getByRole("switch", { name: "Boosts", checked: false })).not.toBeNull();
});

// ADR-0110 §6: a control that saves at once takes its save state as a required prop. The compiler holds it
// (web:typecheck reads this file): the line below is an error, and stops being one if the prop becomes optional.
test("a switch cannot be written without its save state", () => {
  // @ts-expect-error -- no `save`.
  const without = <Switch label="Boosts" />;

  expect(without).not.toBeNull();
});

test("a switch in a form posts under its name only while it is on", async () => {
  const user = userEvent.setup();
  render(
    <form aria-label="Notifications">
      <Switch label="Boosts" name="boosts" defaultSelected save={null} />
      <Switch label="Favourites" name="favourites" save={null} />
    </form>,
  );
  const posted = () => [...new FormData(screen.getByRole<HTMLFormElement>("form", { name: "Notifications" })).keys()];
  expect(posted()).toEqual(["boosts"]);

  await user.click(screen.getByRole("switch", { name: "Favourites" }));
  await user.click(screen.getByRole("switch", { name: "Boosts" }));

  expect(posted()).toEqual(["favourites"]);
});
