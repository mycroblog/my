// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so where the focus is once the Web Push
// switch of the notification settings (#3201) turned disabled under the reader, and once it is theirs again, is decided
// here (ADR-0171 §4), and where it goes once the summary of a stopped email gives way. What the switch says and does is notifications-settings-device.dom.test.tsx; how a switch hands
// its focus over and takes it back is the design system's (switch.jsdom.test.tsx).

import { expect, test } from "bun:test";
import { memberNotificationRows, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor } from "@testing-library/react";
import { changeSitePermission, type PushBrowser } from "../test/browser";
import { readSettings, toggle } from "../test/notification-settings";
import { runtimeSignals } from "../test/profile-form";

const push = "Push notifications";
const blocked = "Your browser blocks notifications from this site. Allow them in the browser's settings.";

/** A member's rows on a server that sends Web Push, to a reader with no subscription. */
const sends = { push: true, settings: { notificationRows: memberNotificationRows } } as const satisfies FakeOptions;

/**
 * What has the focus, as a reader meets it: the switch, the line under it that took its focus (a status region, or the
 * line that describes the switch) by its words, or the page when nothing has it.
 */
function focusIsOn(): string {
  const has = document.activeElement;
  if (has === null || has === document.body) return "the page";
  if (has === toggle(push)) return "the switch";
  const described = (toggle(push).getAttribute("aria-describedby") ?? "").split(" ").includes(has.id);
  if (!described) return `<${has.tagName.toLowerCase()}> ${has.textContent}`;
  return `${has.getAttribute("role") === "status" ? "status" : "line"}: ${has.textContent}`;
}

/** Waits until the focus is on `where`; what has it instead otherwise. */
const focusOn = (where: string) =>
  waitFor(() => {
    const on = focusIsOn();
    if (on !== where) throw new Error(`test: the focus is not on "${where}" but on "${on}"`);
  });

/** The entry in a browser of `browser`, with the focus on the Web Push switch. */
async function onTheSwitch(browser: PushBrowser) {
  runtimeSignals();
  const drawn = await readSettings(sends, { browser });
  await waitFor(() => {
    if (drawn.data.queryClient.isFetching() > 0) throw new Error("test: the screen is still reading");
  });
  toggle(push).focus();
  expect(focusIsOn()).toBe("the switch");
  return drawn;
}

test("the reader turns the switch on from the keyboard and refuses the browser's question: the focus moves to the status that says the browser blocks the site", async () => {
  const { user, seen } = await onTheSwitch({ answer: "denied" });

  await user.keyboard(" ");

  await focusOn(`status: ${blocked}`);
  expect(toggle(push).disabled).toBe(true);
  expect(seen.asked).toBe(1);
});

test("the site is blocked in the browser's settings while the switch has the focus: the focus moves to the line that says so", async () => {
  await onTheSwitch({});

  changeSitePermission("denied");

  await focusOn(`line: ${blocked}`);
  expect(toggle(push).disabled).toBe(true);
});

// The entry's other hand-off of the focus: the summary of an email that stopped gives way under the reader's press.
test("an address the server says is verified already (a 409): the status that takes the summary's place takes the focus from the button", async () => {
  runtimeSignals();
  const verified = "Your address is already verified. Email to it is on again.";
  const { fake, user } = await readSettings({
    ...sends,
    email: true,
    preferences: { email_suppression: { kind: "hard_bounce", state_changed_at: "2026-11-02T09:30:00Z" } },
  });
  fake.script("POST /accounts/email/verification", {
    fail: { type: "/api/v1/problems/email-already-verified", title: "The address is verified already", status: 409 },
  });
  const send = screen.getByRole("button", { name: "Send the verification email" });

  await user.click(send);

  await waitFor(() => {
    const has = document.activeElement;
    const on =
      has === null ? "nothing" : `${has.getAttribute("role") ?? has.tagName.toLowerCase()}: ${has.textContent}`;
    if (on !== `status: ${verified}`) throw new Error(`test: the focus is on "${on}"`);
  });
});

test("the site is unblocked in the browser's settings while the status of the refusal has the focus: the focus returns to the switch", async () => {
  const { user } = await onTheSwitch({ answer: "denied" });
  await user.keyboard(" ");
  await focusOn(`status: ${blocked}`);

  changeSitePermission("default");

  await focusOn("the switch");
  expect(toggle(push).disabled).toBe(false);
});
