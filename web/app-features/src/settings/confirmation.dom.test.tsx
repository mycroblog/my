// SPDX-License-Identifier: AGPL-3.0-only

// The dialog that asks a reader to confirm it's them before the settings change how they sign in (#3205; ADR-0106
// §3.7), pinned once, on the operation that offers every way: linking an account, from Security. The ways are the
// password, a passkey, then each linked account, as the account and the browser have them; one shows at a time, with
// the others as buttons under it; a proof the server refuses stays in the dialog, in the server's words; a linked
// account's confirmation leaves for the provider; and Cancel and Close run nothing. Which operation a screen asks it
// for, and what the operation does once confirmed, is each screen's test (security/*.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { fakeOrigin } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import {
  atRest,
  buttonsOf,
  closed,
  describedBy,
  dialogs,
  offered,
  passkeyBrowser,
  proving,
  words,
} from "../test/security";
import { settingsOn, type Access, type Given } from "../test/settings";
import { SettingsEntryScreen } from "./screen";

/**
 * Security on a server whose account stands as `access` says, with the dialog open that Link of Example SSO asks,
 * the provider no account of the fake is linked at: a typical account links GitHub.
 */
async function linking(access: Access = {}, given: Given = {}) {
  const on = await settingsOn(
    access,
    (address) => <SettingsEntryScreen entry="security" languages={["en"]} {...address} />,
    given,
  );
  await screen.findByRole("region", { name: "Linked accounts" });
  await atRest(on);
  const sent = on.fake.script("POST /linked_identities");
  await on.user.click(offered("Linked accounts", "Example SSO", "Link"));
  return { ...on, sent, dialog: await proving() };
}

const password = (dialog: HTMLElement) => within(dialog).getByLabelText<HTMLInputElement>("Current password");
const yuki = "demo-password-yuki";

/** A server whose linked account is at a provider that is not available now. */
const providerDown: Given = {
  before: ({ fake }) => {
    fake.script("GET /linked_identities", {
      change: (body) => ({ ...body, items: body.items.map((item) => ({ ...item, available: false })) }),
    });
  },
};

describe("the ways to confirm", () => {
  // The first way shows, named by the dialog's last button; the others are the buttons before Cancel.
  for (const [who, access, passkeys, given, buttons] of [
    [
      "a password, passkeys and a linked account, in a browser with passkeys: the password first",
      {},
      true,
      {},
      ["Close", "Use a passkey instead", "Continue with GitHub instead", "Cancel", "Confirm"],
    ],
    [
      "the same account in a browser without passkeys: no passkey",
      {},
      false,
      {},
      ["Close", "Continue with GitHub instead", "Cancel", "Confirm"],
    ],
    [
      "an account without passkeys: no passkey",
      { passkeys: 0 },
      true,
      {},
      ["Close", "Continue with GitHub instead", "Cancel", "Confirm"],
    ],
    [
      "an account without a password: a passkey first",
      { password: false },
      true,
      {},
      ["Close", "Continue with GitHub instead", "Cancel", "Use a passkey"],
    ],
    [
      "an account whose linked provider is not available: no linked account",
      {},
      true,
      providerDown,
      ["Close", "Use a passkey instead", "Cancel", "Confirm"],
    ],
    [
      "an account with a linked account alone",
      { password: false, passkeys: 0 },
      true,
      {},
      ["Close", "Cancel", "Continue with GitHub"],
    ],
  ] as const)
    test(who, async () => {
      if (passkeys) passkeyBrowser();

      const { dialog } = await linking(access, given);

      expect(buttonsOf(dialog)).toEqual([...buttons]);
      // The password's field shows only while the password is the way.
      expect(within(dialog).queryAllByLabelText("Current password")).toHaveLength(buttons.at(-1) === "Confirm" ? 1 : 0);
      expect(within(dialog).queryAllByRole("alert")).toHaveLength(0);
    });

  test("the password's field has the focus as the dialog opens", async () => {
    const { dialog } = await linking();

    expect(document.activeElement === password(dialog)).toBe(true);
  });

  test("a way chosen under the one that shows takes its place, and the one that showed joins the others, in the ways' order", async () => {
    passkeyBrowser();
    const { user, dialog, sent, address } = await linking();

    await user.click(within(dialog).getByRole("button", { name: "Use a passkey instead" }));

    await within(dialog).findByRole("button", { name: "Use a passkey" });
    expect(buttonsOf(dialog)).toEqual([
      "Close",
      "Use your password instead",
      "Continue with GitHub instead",
      "Cancel",
      "Use a passkey",
    ]);
    expect(within(dialog).queryAllByLabelText("Current password")).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Continue with GitHub instead" }));

    await within(dialog).findByRole("button", { name: "Continue with GitHub" });
    expect(buttonsOf(dialog)).toEqual([
      "Close",
      "Use your password instead",
      "Use a passkey instead",
      "Cancel",
      "Continue with GitHub",
    ]);
    expect(words(dialog)).toContain(
      "To confirm it's you, sign in to GitHub again. You then come back here and repeat what you started.",
    );

    await user.click(within(dialog).getByRole("button", { name: "Use your password instead" }));

    await within(dialog).findByRole("button", { name: "Confirm" });
    expect(within(dialog).queryAllByLabelText("Current password")).toHaveLength(1);
    // Choosing a way confirms nothing.
    expect(sent.seen).toHaveLength(0);
    expect(address.left).toEqual([]);
  });

  test("an account this browser can confirm in no way is told so, and the dialog only closes", async () => {
    // Passkeys alone, in a browser without them.
    const { user, dialog, sent } = await linking({ password: false, linked: "none" });

    expect(words(within(dialog).getByRole("alert"))).toBe(
      "This browser has no way to confirm it's you. Try a browser or device that can use one of your passkeys.",
    );
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Close"]);

    // The button that says Close, in the place of the one that would confirm; the head's Close is a glyph.
    const [close] = within(dialog)
      .getAllByRole("button", { name: "Close" })
      .filter((button) => button.textContent === "Close");
    if (close === undefined) throw new Error("test: no button says Close");
    await user.click(close);

    await closed();
    expect(sent.seen).toHaveLength(0);
  });
});

describe("a proof", () => {
  test("of a wrong password stays in the dialog, in the server's words, with the focus in its field, and the right one then goes through", async () => {
    const { user, dialog, sent, address } = await linking();

    await user.type(password(dialog), "not-the-password");
    await user.click(within(dialog).getByRole("button", { name: "Confirm" }));

    expect(words(await within(dialog).findByRole("alert"))).toBe("Wrong password");
    expect(describedBy(password(dialog))).toEqual(["Wrong password"]);
    expect(document.activeElement === password(dialog)).toBe(true);
    expect(address.left).toEqual([]);

    await user.clear(password(dialog));
    await user.type(password(dialog), yuki);
    await user.click(within(dialog).getByRole("button", { name: "Confirm" }));

    await waitFor(() => {
      if (address.left.length === 0) throw new Error("test: the app did not leave yet");
    });
    expect(sent.seen.map((s) => s.body.password)).toEqual(["not-the-password", yuki]);
  });

  test("by a passkey asks the server for a confirmation, the browser for the passkey, and sends both with the operation", async () => {
    const browser = passkeyBrowser();
    const { user, dialog, sent, fake, address } = await linking({ password: false });
    const begun = fake.script("POST /passkeys/confirmation");

    await user.click(within(dialog).getByRole("button", { name: "Use a passkey" }));

    await waitFor(() => {
      if (address.left.length === 0) throw new Error("test: the app did not leave yet");
    });
    expect(begun.seen).toHaveLength(1);
    expect(browser.asked).toBe(1);
    const [body] = sent.seen.map((s) => s.body);
    expect(Object.keys(body ?? {})).toEqual(["provider_id", "passkey", "return_to"]);
    // The confirmation the server made, and the credential as the browser gave it.
    expect(body?.passkey?.confirmation_token).toMatch(/^confirmation-\d+$/);
    expect(body?.passkey?.credential["id"]).toBe("AQID");
  });

  test("by a passkey the browser does not give stays in the dialog and says so, and nothing is sent", async () => {
    const browser = passkeyBrowser();
    browser.answer = "cancelled";
    const { user, dialog, sent, address } = await linking({ password: false });

    await user.click(within(dialog).getByRole("button", { name: "Use a passkey" }));

    expect(words(await within(dialog).findByRole("alert"))).toBe(
      "The passkey was not used. Try again, or confirm another way.",
    );
    expect(browser.asked).toBe(1);
    expect(sent.seen).toHaveLength(0);
    expect(address.left).toEqual([]);
  });

  test("by a passkey the server refuses stays in the dialog, in the server's words", async () => {
    passkeyBrowser();
    const { user, dialog, fake, address } = await linking({ password: false });
    fake.script("POST /linked_identities", {
      fail: { type: "/api/v1/problems/invalid-passkey", title: "The passkey did not verify", status: 422 },
    });

    await user.click(within(dialog).getByRole("button", { name: "Use a passkey" }));

    expect(words(await within(dialog).findByRole("alert"))).toBe("The passkey did not verify");
    expect(dialogs()).toHaveLength(1);
    expect(address.left).toEqual([]);
  });

  test("by a linked account asks the server for the way to its provider, for this operation and back to this screen, and the app leaves for it", async () => {
    const { user, dialog, sent, fake, address } = await linking();
    const begun = fake.script("POST /linked_identities/confirmation");

    await user.click(within(dialog).getByRole("button", { name: "Continue with GitHub instead" }));
    await user.click(await within(dialog).findByRole("button", { name: "Continue with GitHub" }));

    await waitFor(() => {
      if (address.left.length === 0) throw new Error("test: the app did not leave yet");
    });
    const github = fake.loginProviderId("github");
    expect(begun.seen.map((s) => s.body)).toEqual([
      { provider_id: github, operation: "link", return_to: "/settings/security" },
    ]);
    expect(address.left).toHaveLength(1);
    expect(address.left[0]?.startsWith(`${fakeOrigin}/auth/external/${github}/link?`)).toBe(true);
    // The operation itself waits for the reader to come back.
    expect(sent.seen).toHaveLength(0);
  });
});

for (const button of ["Cancel", "Close"])
  test(`${button} closes the dialog and runs nothing`, async () => {
    passkeyBrowser();
    const { user, dialog, sent, fake, address, requests } = await linking();
    const begun = fake.script("POST /passkeys/confirmation");
    await user.type(password(dialog), yuki);
    const before = requests.length;

    await user.click(within(dialog).getByRole("button", { name: button }));

    await closed();
    expect(sent.seen).toHaveLength(0);
    expect(begun.seen).toHaveLength(0);
    expect(requests.slice(before)).toEqual([]);
    expect(address.left).toEqual([]);
    // What was typed is not kept for the next time the dialog opens.
    await user.click(offered("Linked accounts", "Example SSO", "Link"));
    expect(password(await proving()).value).toBe("");
  });
