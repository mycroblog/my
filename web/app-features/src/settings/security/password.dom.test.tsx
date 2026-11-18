// SPDX-License-Identifier: AGPL-3.0-only

// The password's screen, under Security (#3205): the new password is checked for its length before anything is asked,
// and one the screen or the server refuses is said under its field, which takes the focus;
// Change asks to confirm it's the reader, a wrong password stays in that dialog, and the right one changes the
// password, which the screen then says. An account without a password sets one, and no password confirms that; a
// server that limits sign-in to login providers takes none (ADR-0106 §21); and back from a provider that confirmed
// the change (§3.7), Change asks nothing. The dialog's ways are pinned once (../confirmation.dom.test.tsx), and that
// the entry shows what the change ended is the app's test (web/app, settings-security.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { fakeExternalConfirmation } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import {
  atRest,
  buttonsOf,
  closed,
  describedBy,
  dialogs,
  inOrder,
  passkeyBrowser,
  proving,
  section,
  sections,
  words,
} from "../../test/security";
import { settingsOn, type Access, type Given } from "../../test/settings";
import { SecurityPassword } from "./password";

/** The password's screen on a server whose account stands as `access` says, once its reads answered. */
async function read(access: Access = {}, given: Given = {}) {
  const on = await settingsOn(access, (address) => <SecurityPassword {...address} />, given);
  await screen.findByRole("region");
  await atRest(on);
  return on;
}

/** The field of the new password; a password field has no role. */
const field = () => screen.getByLabelText<HTMLInputElement>("New password");
/** What the screen says of what it did, beside its button. */
const said = () =>
  screen
    .queryAllByRole("status")
    .map(words)
    .filter((text) => text !== "");

const hint = "8 to 128 characters";
const wrongLength = "Use 8 to 128 characters.";
const changed = "Your password is changed, and your other browsers and apps are signed out.";
const yuki = "demo-password-yuki";

describe("an account with a password", () => {
  test("is offered to change it: the new password with its length, what the change ends, and the button", async () => {
    await read();

    expect(sections()).toEqual(["Change password"]);
    expect(words(section("Change password"))).toContain(
      "This signs you out of your other browsers and apps. Your passkeys and two-factor authentication stay as they are.",
    );
    expect(describedBy(field())).toEqual([hint]);
    expect(field().type).toBe("password");
    expect(screen.getAllByRole("button").map(words)).toEqual(["Security", "Change password"]);
    expect(said()).toEqual([]);
  });

  for (const [length, taken] of [
    [0, false],
    [7, false],
    [8, true],
    [128, true],
    [129, false],
  ] as const)
    test(`a new password of ${String(length)} characters ${taken ? "is taken: Change asks to confirm it's the reader" : "is not: the field says the length, and nothing is asked or sent"}`, async () => {
      const { user, fake } = await read();
      const sent = fake.script("POST /accounts/password");
      if (length > 0) {
        await user.click(field());
        await user.paste("p".repeat(length));
      }

      await user.click(screen.getByRole("button", { name: "Change password" }));

      if (taken) {
        await proving();
        expect(describedBy(field())).toEqual([hint]);
      } else {
        await waitFor(() => {
          if (field().getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked yet");
        });
        expect(describedBy(field())).toEqual([hint, wrongLength]);
        // The press left the focus on the button: the screen gives it back to the field.
        expect(document.activeElement === field()).toBe(true);
        expect(dialogs()).toHaveLength(0);
      }
      expect(sent.seen).toHaveLength(0);
    });

  test("a wrong password stays in the dialog, in the server's words, with the focus in its field; the right one changes the password, and the screen says so with its field empty", async () => {
    const { user, fake } = await read();
    const sent = fake.script("POST /accounts/password");
    await user.click(field());
    await user.paste("a-new-password");
    await user.click(screen.getByRole("button", { name: "Change password" }));
    const dialog = await proving();
    expect(words(dialog)).toContain("To change your password, confirm it's you.");
    const current = within(dialog).getByLabelText<HTMLInputElement>("Current password");

    await user.type(current, "not-the-password");
    await user.click(within(dialog).getByRole("button", { name: "Confirm" }));

    const refused = await within(dialog).findByRole("alert");
    expect(words(refused)).toBe("Wrong password");
    expect(describedBy(current)).toEqual(["Wrong password"]);
    expect(document.activeElement === current).toBe(true);
    expect(said()).toEqual([]);

    await user.clear(current);
    await user.type(current, yuki);
    await user.click(within(dialog).getByRole("button", { name: "Confirm" }));

    await closed();
    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(said()).toEqual([changed]);
    expect(field().value).toBe("");
    expect(sent.seen.map((s) => s.body)).toEqual([
      { new_password: "a-new-password", current_password: "not-the-password" },
      { new_password: "a-new-password", current_password: yuki },
    ]);
  });

  test("a new password the server refuses is said under its field in the server's words, once the dialog left, with the focus in it and nothing beside the button", async () => {
    const { user, fake } = await read();
    fake.script("POST /accounts/password", {
      fail: {
        type: "about:blank",
        title: "The request is not valid",
        status: 422,
        field: "new_password",
        detail: "Use a password that is not this common.",
      },
    });
    await user.click(field());
    await user.paste("password123");
    await user.click(screen.getByRole("button", { name: "Change password" }));
    const dialog = await proving();
    await user.type(within(dialog).getByLabelText("Current password"), yuki);

    await user.click(within(dialog).getByRole("button", { name: "Confirm" }));

    await closed();
    await waitFor(() => {
      if (field().getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked yet");
    });
    expect(describedBy(field())).toEqual([hint, "Use a password that is not this common."]);
    expect(document.activeElement === field()).toBe(true);
    expect(screen.queryAllByRole("alert").map(words)).toEqual([]);
    expect(said()).toEqual([]);
    expect(field().value).toBe("password123");
  });
});

describe("an account without a password", () => {
  test("is offered to set one, which no password confirms; once a passkey confirmed, the screen says it is set and offers to change it", async () => {
    const browser = passkeyBrowser();
    const { user, fake } = await read({ password: false });
    const sent = fake.script("POST /accounts/password");
    expect(sections()).toEqual(["Set a password"]);
    expect(screen.getAllByRole("button").map(words)).toEqual(["Security", "Set a password"]);
    await user.click(field());
    await user.paste("a-new-password");

    await user.click(screen.getByRole("button", { name: "Set a password" }));

    const dialog = await proving();
    expect(words(dialog)).toContain("To set a password, confirm it's you.");
    expect(within(dialog).queryAllByLabelText("Current password")).toHaveLength(0);
    expect(buttonsOf(dialog)).toEqual(["Close", "Continue with GitHub instead", "Cancel", "Use a passkey"]);

    await user.click(within(dialog).getByRole("button", { name: "Use a passkey" }));

    await closed();
    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(said()).toEqual(["Your password is set, and your other browsers and apps are signed out."]);
    expect(browser.asked).toBe(1);
    expect(sent.seen.map((s) => Object.keys(s.body))).toEqual([["new_password", "passkey"]]);
    expect(sections()).toEqual(["Change password"]);
    expect(screen.getAllByRole("button").map(words)).toEqual(["Security", "Change password"]);
  });
});

test("where sign-in is limited to login providers, the screen says so and offers no field and no button", async () => {
  await read({ providersOnly: true });

  expect(words(section("Change password"))).toMatch(
    inOrder(
      "Change password",
      "On this server, you sign in only with another service, so you cannot set a password or add a passkey now.",
    ),
  );
  expect(screen.queryAllByLabelText("New password")).toHaveLength(0);
  expect(screen.getAllByRole("button").map(words)).toEqual(["Security"]);
});

describe("back from a provider that confirmed the change", () => {
  const back = (token: string): Given => ({
    fragment: (fake) =>
      `external_confirmation=${token}&operation=password_change&provider=${fake.loginProviderId("github")}`,
  });

  test("the screen says to repeat it, and Change then sends the provider's confirmation with no dialog", async () => {
    const { user, fake, address } = await read({}, back(fakeExternalConfirmation));
    const sent = fake.script("POST /accounts/password");
    expect(words(section("Change password"))).toContain(
      "You confirmed it's you with GitHub. Repeat what you started: this time you are not asked to confirm.",
    );
    expect(address.fragmentsRead).toBe(1);
    await user.click(field());
    await user.paste("a-new-password");

    await user.click(screen.getByRole("button", { name: "Change password" }));

    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(said()).toEqual([changed]);
    expect(dialogs()).toHaveLength(0);
    expect(sent.seen.map((s) => s.body)).toEqual([
      { new_password: "a-new-password", external_confirmation: fakeExternalConfirmation },
    ]);
  });

  test("a confirmation the server refuses opens the dialog, with the server's words, and changes nothing", async () => {
    const { user } = await read({}, back("not-the-confirmation"));
    await user.click(field());
    await user.paste("a-new-password");

    await user.click(screen.getByRole("button", { name: "Change password" }));

    const dialog = await proving();
    expect(words(await within(dialog).findByRole("alert"))).toBe("The confirmation expired");
    expect(said()).toEqual([]);
    expect(field().value).toBe("a-new-password");
  });
});
