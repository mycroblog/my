// SPDX-License-Identifier: AGPL-3.0-only

// Adding a passkey, under Security (#3205; ADR-0106 §26): the name is checked before anything is asked, and a name the
// screen or the server refuses is said under its field, which takes the focus; a browser
// without passkeys is told so at once; and in one with them, once the reader confirmed it's them, the server starts
// the registration, the browser creates the passkey, the server keeps it, and the screen goes back to the entry. What
// the browser refuses (a dialog closed, a passkey that exists) and what the server does (as many passkeys as an
// account may hold) is said beside the button. The browser's passkeys are stubbed whole (../../test/settings.tsx):
// what the browser's own parts answer is the platform's test (web/platform, passkeys.test.ts), and a passkey a real
// browser makes and signs in with is the E2E test's (web/e2e, specs/security.spec.ts).

import { expect, test } from "bun:test";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { atRest, closed, describedBy, dialogs, passkeyBrowser, proving, words } from "../../test/security";
import { settingsOn, type Access } from "../../test/settings";
import { SecurityPasskey } from "./passkey";

/** The screen that adds a passkey, on a server whose account stands as `access` says, once it is at rest. */
async function read(access: Access = {}) {
  const on = await settingsOn(access, (address) => <SecurityPasskey {...address} />);
  await screen.findByRole("textbox", { name: "Name" });
  await atRest(on);
  return on;
}

const name = () => screen.getByRole<HTMLInputElement>("textbox", { name: "Name" });
const add = () => screen.getByRole("button", { name: "Add a passkey" });
/** What the screen says beside its button, as alerts. */
const alerts = () => screen.queryAllByRole("alert").map(words);

const hint = "A name to tell it from your other passkeys, such as the device it is on";
const yuki = "demo-password-yuki";

/** Names the passkey, presses Add, and confirms with the password. */
async function addNamed(user: UserEvent, called: string) {
  await user.clear(name());
  await user.click(name());
  await user.paste(called);
  await user.click(add());
  const dialog = await proving();
  await user.type(within(dialog).getByLabelText("Current password"), yuki);
  await user.click(within(dialog).getByRole("button", { name: "Confirm" }));
  await closed();
}

for (const [what, value] of [
  ["no name", ""],
  ["a name of 65 characters", "n".repeat(65)],
] as const)
  test(`with ${what}, the field says what a name is, and nothing is asked or sent`, async () => {
    passkeyBrowser();
    const { user, requests } = await read();
    // The field takes no more than a name's length from a reader's hands: a longer value is set as a script sets it.
    if (value !== "") fireEvent.change(name(), { target: { value } });
    const before = requests.length;

    await user.click(add());

    await waitFor(() => {
      if (name().getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked yet");
    });
    expect(describedBy(name())).toEqual([hint, "Enter a name of up to 64 characters."]);
    // The press left the focus on the button: the screen gives it back to the field.
    expect(document.activeElement === name()).toBe(true);
    expect(alerts()).toEqual([]);
    expect(dialogs()).toHaveLength(0);
    expect(requests.slice(before)).toEqual([]);
  });

test("a name the server refuses is said under its field in the server's words, once the dialog left, with the focus in it and nothing beside the button", async () => {
  const browser = passkeyBrowser();
  const { user, fake, address } = await read();
  fake.script("POST /passkeys/registration", {
    fail: {
      type: "about:blank",
      title: "The request is not valid",
      status: 422,
      field: "name",
      detail: "Use a name without control characters.",
    },
  });

  await addNamed(user, "My laptop");

  await waitFor(() => {
    if (name().getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked yet");
  });
  expect(describedBy(name())).toEqual([hint, "Use a name without control characters."]);
  expect(document.activeElement === name()).toBe(true);
  expect(alerts()).toEqual([]);
  expect(browser.created).toBe(0);
  expect(address.backs).toBe(0);
  expect(name().value).toBe("My laptop");
});

test("a browser without passkeys is told so at once, beside the button: no dialog, and nothing sent", async () => {
  const { user, requests, address } = await read();
  await user.click(name());
  await user.paste("My laptop");
  const before = requests.length;

  await user.click(add());

  await waitFor(() => {
    if (alerts().length === 0) throw new Error("test: the screen says nothing yet");
  });
  expect(alerts()).toEqual(["This browser cannot use passkeys. Try another browser or device."]);
  expect(dialogs()).toHaveLength(0);
  expect(requests.slice(before)).toEqual([]);
  expect(address.backs).toBe(0);
});

test("once the reader confirmed it's them, the server starts the registration under the name, the browser creates the passkey, the server keeps it, and the screen goes back", async () => {
  const browser = passkeyBrowser();
  const { user, fake, address } = await read();
  const begun = fake.script("POST /passkeys/registration");
  const kept = fake.script("POST /passkeys");
  await user.click(name());
  await user.paste("My laptop");

  await user.click(add());

  const dialog = await proving();
  expect(words(dialog)).toContain("To add a passkey, confirm it's you. Your browser then asks you to create it.");
  expect(begun.seen).toHaveLength(0);
  await user.type(within(dialog).getByLabelText("Current password"), yuki);
  await user.click(within(dialog).getByRole("button", { name: "Confirm" }));

  await waitFor(() => {
    if (address.backs === 0) throw new Error("test: the screen did not go back yet");
  });
  expect(address.backs).toBe(1);
  expect(begun.seen.map((s) => s.body)).toEqual([{ name: "My laptop", password: yuki }]);
  expect(browser.created).toBe(1);
  // The registration the server started, with what the browser made.
  expect(kept.seen.map((s) => [typeof s.body.registration_token, typeof s.body.credential])).toEqual([
    ["string", "object"],
  ]);
  expect(alerts()).toEqual([]);
});

for (const [answer, sentence] of [
  ["cancelled", "The passkey was not added: the browser's dialog was closed or timed out. Try again."],
  ["exists", "This device already has a passkey of your account. Use that one, or add a passkey on another device."],
] as const)
  test(`a passkey the browser does not create (${answer}) is said beside the button, and nothing is kept`, async () => {
    const browser = passkeyBrowser();
    browser.answer = answer;
    const { user, fake, address } = await read();
    const kept = fake.script("POST /passkeys");

    await addNamed(user, "My laptop");

    await waitFor(() => {
      if (alerts().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(alerts()).toEqual([sentence]);
    expect(browser.created).toBe(1);
    expect(kept.seen).toHaveLength(0);
    expect(address.backs).toBe(0);
    // The name stays for the next try.
    expect(name().value).toBe("My laptop");
  });

test("an account that holds as many passkeys as it may is told so in the server's words", async () => {
  const browser = passkeyBrowser();
  const { user, address } = await read({ passkeys: 20 });

  await addNamed(user, "One more");

  await waitFor(() => {
    if (alerts().length === 0) throw new Error("test: the screen says nothing yet");
  });
  expect(alerts()).toEqual(["You have as many passkeys as you may"]);
  expect(browser.created).toBe(0);
  expect(address.backs).toBe(0);
});
