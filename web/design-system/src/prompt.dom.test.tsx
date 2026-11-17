// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { InlinePrompt, type InlinePromptProps } from "./prompt";

/** The prompt that offers notifications, and the presses of its action and its dismiss, in order. */
function offer(props: Partial<InlinePromptProps> & { blocked?: boolean } = {}) {
  const presses: string[] = [];
  const { blocked = false, ...rest } = props;
  render(
    <InlinePrompt
      icon="notifications"
      action={blocked ? undefined : { label: "Turn on", onPress: () => presses.push("action") }}
      dismiss={{ label: "Not now", onPress: () => presses.push("dismiss") }}
      {...rest}
    >
      Get notified on this device.
    </InlinePrompt>,
  );
  return presses;
}

test("a prompt is a region named by what it offers, with its action before its dismiss", async () => {
  const user = userEvent.setup();
  const presses = offer();
  const prompt = screen.getByRole("region", { name: "Get notified on this device." });

  expect(within(prompt).queryByRole("alert")).toBeNull();
  await user.tab();
  expect(document.activeElement).toBe(within(prompt).getByRole("button", { name: "Turn on" }));
  await user.keyboard("{Enter}");
  await user.tab();
  expect(document.activeElement).toBe(within(prompt).getByRole("button", { name: "Not now" }));
  await user.keyboard(" ");

  expect(presses).toEqual(["action", "dismiss"]);
});

test("a press on a prompt's action and on its dismiss tells each once", async () => {
  const user = userEvent.setup();
  const presses = offer();

  await user.click(screen.getByRole("button", { name: "Not now" }));
  await user.click(screen.getByRole("button", { name: "Turn on" }));

  expect(presses).toEqual(["dismiss", "action"]);
});

test("the reason an action did not work is an alert in the prompt, which keeps its name and its action", () => {
  offer({ reason: "You closed the browser's question without choosing." });
  const prompt = screen.getByRole("region", { name: "Get notified on this device." });

  expect(within(prompt).getByRole("alert").textContent).toBe("You closed the browser's question without choosing.");
  expect(within(prompt).getByRole("button", { name: "Turn on" })).not.toBeNull();
});

test("with nothing left to try a prompt has its reason and its dismiss alone", () => {
  offer({ blocked: true, reason: "Your browser blocks notifications from this site." });
  const prompt = screen.getByRole("region", { name: "Get notified on this device." });

  expect(within(prompt).getByRole("alert").textContent).toBe("Your browser blocks notifications from this site.");
  expect(within(prompt).getAllByRole("button")).toEqual([within(prompt).getByRole("button", { name: "Not now" })]);
});
