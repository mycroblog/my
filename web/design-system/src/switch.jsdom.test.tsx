// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so that the line under a switch can
// take the focus the switch hands it when it becomes disabled while it has the focus (#3201), and that Tab passes that
// line, are decided here (ADR-0171 §4). What a disabled switch is and says is switch.dom.test.tsx.

import { expect, test } from "bun:test";
import { Button } from "@my/design-system";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { Switch } from "./switch";

const offer = "Get your notifications on this device.";
const blocked = "Your browser blocks notifications from this site.";
const refused = "You refused the browser's question.";

/**
 * A switch between two buttons of its page: the reader's to turn, or disabled for the reason `blocked`; `save` is what
 * its row says, as the status region under it.
 */
const page = (disabled: boolean, save: ReactNode = null) => (
  <>
    <Button>Before</Button>
    <Switch
      label="Push notifications"
      description={offer}
      isDisabled={disabled}
      reason={disabled ? blocked : undefined}
      save={save}
    />
    <Button>After</Button>
  </>
);

/**
 * What has the focus, as a reader meets it: the switch, a button by its name, the status region or the line that
 * describes the switch by its words, or the page when nothing has it.
 */
function focusIsOn(): string {
  const has = document.activeElement;
  if (has === null || has === document.body) return "the page";
  if (has.getAttribute("role") === "switch") return "the switch";
  if (has.getAttribute("role") === "status") return `status: ${has.textContent}`;
  return has.tagName === "BUTTON" ? `button: ${has.textContent}` : `line: ${has.textContent}`;
}

/** The page drawn with the switch enabled and the focus on it, reached with Tab. */
async function onTheSwitch() {
  const user = userEvent.setup();
  const drawn = render(page(false));
  await user.tab();
  await user.tab();
  expect(focusIsOn()).toBe("the switch");
  return { user, ...drawn };
}

test("a switch that becomes disabled while it has the focus hands the focus to its status region, which says what happened", async () => {
  const { rerender } = await onTheSwitch();

  rerender(page(true, refused));

  expect(focusIsOn()).toBe(`status: ${refused}`);
  expect(screen.getByRole<HTMLInputElement>("switch", { name: "Push notifications" }).disabled).toBe(true);
});

test("with nothing in its status region, it hands the focus to the reason that describes it", async () => {
  const { rerender } = await onTheSwitch();

  rerender(page(true));

  expect(focusIsOn()).toBe(`line: ${blocked}`);
  expect(screen.getAllByRole("switch", { name: "Push notifications", description: blocked })).toHaveLength(1);
});

for (const [line, save] of [
  ["its status region", refused],
  ["its reason", null],
] as const)
  test(`the line that took the focus (${line}) is no stop of Tab: Tab goes on to what follows the switch, and back to what precedes it`, async () => {
    const { user, rerender } = await onTheSwitch();
    rerender(page(true, save));

    await user.tab();
    expect(focusIsOn()).toBe("button: After");

    // Neither the disabled switch nor a line under it stops Tab on the way back.
    await user.tab({ shift: true });
    expect(focusIsOn()).toBe("button: Before");
  });

for (const [line, save] of [
  ["its status region", refused],
  ["its reason", null],
] as const)
  test(`enabled again while ${line} has the focus, the switch takes the focus back`, async () => {
    const { rerender } = await onTheSwitch();
    rerender(page(true, save));
    expect(focusIsOn()).not.toBe("the switch");

    rerender(page(false));

    expect(focusIsOn()).toBe("the switch");
    expect(screen.getAllByRole("switch", { name: "Push notifications", description: offer })).toHaveLength(1);
  });

test("enabled again after the reader moved on, the switch leaves the focus where the reader put it", async () => {
  const { user, rerender } = await onTheSwitch();
  rerender(page(true, refused));
  await user.tab();
  expect(focusIsOn()).toBe("button: After");

  rerender(page(false));

  expect(focusIsOn()).toBe("button: After");
});

test("a switch that becomes disabled, and enabled again, while something else has the focus leaves the focus there", async () => {
  const user = userEvent.setup();
  const { rerender } = render(page(false));
  await user.tab();
  expect(focusIsOn()).toBe("button: Before");

  rerender(page(true, refused));
  expect(focusIsOn()).toBe("button: Before");

  rerender(page(false));
  expect(focusIsOn()).toBe("button: Before");
});

test("a switch drawn disabled takes no focus, and none when it is enabled", () => {
  const { rerender } = render(page(true));
  expect(focusIsOn()).toBe("the page");

  rerender(page(false));

  expect(focusIsOn()).toBe("the page");
});
