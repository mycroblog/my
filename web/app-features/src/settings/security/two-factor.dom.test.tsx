// SPDX-License-Identifier: AGPL-3.0-only

// Two-factor authentication, under Security (#3205): for an account without it, the screen asks the server for a
// setup as it opens, shows its key as a QR code and as text to copy, and takes the code of the authenticator app; a
// code the screen or the server refuses is said under its field, which takes the focus, a setup that expired leaves
// the screen while a new one is asked for, and the right code turns it on and shows the recovery codes, once. For an account with it, the screen makes new recovery codes and turns it off, each after a
// code. That the entry shows it on and off afterwards is the app's test (web/app, settings-security.dom.test.tsx),
// and a code of a real authenticator on a real server the E2E test's (web/e2e, specs/security.spec.ts).

import { describe, expect, onTestFinished, test } from "bun:test";
import { fakeTotpCode } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { before } from "../../test/apps";
import {
  atRest,
  buttonsOf,
  closed,
  dayOf,
  describedBy,
  dialogs,
  focusOn,
  heldBy,
  inOrder,
  row,
  rows,
  section,
  sections,
  words,
} from "../../test/security";
import { settingsOn, type Access, type Given } from "../../test/settings";
import { SecurityTwoFactor } from "./two-factor";

/** What the server was asked of an operation. */
type Asked = { readonly seen: readonly unknown[] };

/**
 * The screen of two-factor authentication on a server whose account stands as `access` says, once it is at rest, with
 * the setups the server was asked for since before the screen opened.
 */
async function read(access: Access = {}, given: Given = {}) {
  let setups: Asked = { seen: [] };
  const on = await settingsOn(access, (address) => <SecurityTwoFactor {...address} />, {
    ...given,
    before: (server) => {
      setups = server.fake.script("POST /two_factor/totp/setup");
      given.before?.(server);
    },
  });
  await screen.findAllByRole("region");
  await atRest(on);
  return { ...on, setups: () => setups.seen.length };
}

/** The field of the authenticator's code, on the setup. */
const code = () => screen.getByRole<HTMLInputElement>("textbox", { name: "Authentication code" });
/** What the status regions say, the empty ones left out. */
const said = () =>
  screen
    .queryAllByRole("status")
    .map(words)
    .filter((text) => text !== "");
const alerts = () => screen.queryAllByRole("alert").map(words);
/** The items of the list named `name`: the setup key, the recovery codes. */
const kept = (name: string) =>
  screen.queryAllByRole("list", { name }).flatMap((list) => within(list).getAllByRole("listitem").map(words));

/**
 * The clipboard the reader's hands brought to the simulation, reached as a stub reaches it: only @my/platform reads
 * the platform's globals in the app.
 */
function clipboard(): Clipboard {
  const browser = Reflect.get(globalThis, "navigator") as object;
  return Reflect.get(browser, "clipboard") as Clipboard;
}

/** Enters `value` as the authenticator's code and presses Turn on. */
async function turnOn(user: UserEvent, value: string) {
  await user.clear(code());
  if (value !== "") await user.type(code(), value);
  await user.click(screen.getByRole("button", { name: "Turn on" }));
}

/** The dialog that asks for a code before two-factor authentication changes, once it is open. */
const asking = () => screen.findByRole("dialog", { name: "Enter a code" });

const hint = "The six-digit code your authenticator app shows now";
const notShownAgain =
  "Each code signs you in once when you cannot use your authenticator app. Keep them somewhere safe: they are not shown again.";

describe("an account without it", () => {
  test("the screen asks for one setup as it opens, and shows its key as a QR code and as text, then the field of the code", async () => {
    const { setups } = await read();

    expect(setups()).toBe(1);
    expect(sections()).toEqual(["Add to your authenticator app", "Enter the code"]);
    const add = section("Add to your authenticator app");
    expect(words(add)).toContain("Scan the QR code with your authenticator app, or enter the setup key by hand.");
    expect(within(add).getAllByRole("img", { name: "QR code of the setup key" })).toHaveLength(1);
    expect(kept("Setup key")).toEqual(["JBSWY3DPEHPK3PXP"]);
    expect(describedBy(code())).toEqual([hint]);
    expect(screen.getAllByRole("button").map(words)).toEqual(["Security", "Copy", "Turn on"]);
  });

  test("Copy puts the setup key on the clipboard and says so", async () => {
    const { user } = await read();

    await user.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(said()).toEqual(["Copied"]);
    expect(await clipboard().readText()).toBe("JBSWY3DPEHPK3PXP");
  });

  test("a clipboard that refuses says to select the text and copy it", async () => {
    const { user } = await read();
    // The clipboard refuses what is written to it, until the test ends.
    const held = clipboard();
    const writeText = held.writeText.bind(held);
    Object.defineProperty(held, "writeText", {
      configurable: true,
      value: () => Promise.reject(new DOMException("test: the clipboard refused", "NotAllowedError")),
    });
    onTestFinished(() => {
      Object.defineProperty(held, "writeText", { configurable: true, value: writeText });
    });

    await user.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(said()).toEqual(["Could not copy. Select the text and copy it yourself."]);
  });

  test("without a code, the field says to enter the app's and takes the focus back from the button, and nothing is sent", async () => {
    const { user, fake } = await read();
    const sent = fake.script("POST /two_factor/totp");

    await turnOn(user, "");

    await waitFor(() => {
      if (code().getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked yet");
    });
    expect(describedBy(code())).toEqual([hint, "Enter the code your authenticator app shows."]);
    expect(document.activeElement === code()).toBe(true);
    expect(alerts()).toEqual([]);
    expect(sent.seen).toHaveLength(0);
  });

  test("a code the server refuses is said under the field in the server's words, the focus goes back to it, nothing goes beside the button, and the setup stays as it is", async () => {
    const { user, fake, setups, address } = await read();
    const sent = fake.script("POST /two_factor/totp");

    await turnOn(user, "000000");

    await waitFor(() => {
      if (code().getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked yet");
    });
    expect(describedBy(code())).toEqual([hint, "Wrong code"]);
    expect(document.activeElement === code()).toBe(true);
    expect(alerts()).toEqual([]);
    expect(said()).toEqual([]);
    expect(sent.seen).toHaveLength(1);
    expect(sections()).toEqual(["Add to your authenticator app", "Enter the code"]);
    expect(setups()).toBe(1);
    expect(address.backs).toBe(0);
  });

  test("the right code turns it on: the screen shows ten recovery codes, says they are not shown again and that it is on, and Finish goes back", async () => {
    const { user, fake, address, setups } = await read();
    const sent = fake.script("POST /two_factor/totp");

    await turnOn(user, fakeTotpCode);

    await screen.findByRole("region", { name: "Recovery codes" });
    expect(sections()).toEqual(["Recovery codes"]);
    const codes = kept("Recovery codes");
    expect(codes).toHaveLength(10);
    expect(new Set(codes).size).toBe(10);
    expect(codes.every((one) => one !== "")).toBe(true);
    expect(words(section("Recovery codes"))).toContain(notShownAgain);
    // What happened is said as a status; that the codes' heading takes the focus is jsdom's (focus.jsdom.test.tsx).
    expect(said()).toEqual(["Two-factor authentication is on."]);
    expect(sent.seen.map((s) => s.body.code)).toEqual([fakeTotpCode]);
    expect((await heldBy<{ enabled: boolean }>(fake, "/two_factor")).enabled).toBe(true);
    // The codes are the screen now: its way out is Finish.
    expect(screen.getAllByRole("button").map(words)).toEqual(["Copy", "Finish"]);
    expect(address.backs).toBe(0);

    await user.click(screen.getByRole("button", { name: "Finish" }));

    expect(address.backs).toBe(1);
    expect(setups()).toBe(1);
  });

  test("in Japanese, the screen says that two-factor authentication is on in the reviewed words", async () => {
    const { user } = await read({}, { locale: "ja" });
    const [field, ...others] = screen.getAllByRole("textbox");
    if (field === undefined || others.length > 0) throw new Error("test: the setup has not one field");
    await user.type(field, fakeTotpCode);
    // The form's own button, whatever its name is in Japanese.
    const [submit] = screen.getAllByRole<HTMLButtonElement>("button").filter((button) => button.type === "submit");
    if (submit === undefined) throw new Error("test: the setup has no button that submits it");

    await user.click(submit);

    await waitFor(() => {
      if (screen.queryAllByRole("listitem").length !== 10) throw new Error("test: the codes are not shown yet");
    });
    expect(words(screen.getByRole("main"))).toContain("二要素認証をオンにしました。");
  });

  // The key on the screen is of a setup the server no longer knows: an entry made from it in the authenticator app
  // never gives a code the server takes, so nothing of it stays to be scanned. That the screen's title takes the
  // focus meanwhile is jsdom's (focus.jsdom.test.tsx).
  describe("a setup that expired", () => {
    const expired = { type: "/api/v1/problems/totp-setup-expired", title: "The setup expired", status: 422 } as const;
    const scanTheNew =
      "The setup of the authenticator app expired. Scan the new QR code, then enter the code of the new entry.";
    const firstKey = "JBSWY3DPEHPK3PXP";
    const newKey = "KRSXG5CTMVRXEZLU";

    test("leaves the screen at once, which says that it loads the settings while one new setup is asked for; the new key then shows, with the code's field empty and focused and what happened said beside the button; the next code goes with the new setup", async () => {
      const { user, fake, setups } = await read();
      const sent = fake.script("POST /two_factor/totp");
      fake.script("POST /two_factor/totp", { times: 1, fail: expired });
      const next = fake.script("POST /two_factor/totp/setup", {
        hold: "request",
        change: (body) => ({ ...body, secret: newKey }),
      });
      expect(kept("Setup key")).toEqual([firstKey]);

      await turnOn(user, fakeTotpCode);

      // The new setup has no answer yet, and the old one is gone: its QR code, its key, and the bar that turns on.
      await next.arrived();
      await waitFor(() => {
        if (sections().length > 0) throw new Error("test: the expired setup is still shown");
      });
      expect(said()).toEqual(["Loading your settings…"]);
      expect(alerts()).toEqual([]);
      expect(screen.queryAllByRole("img", { name: "QR code of the setup key" })).toHaveLength(0);
      expect(screen.queryAllByRole("textbox")).toHaveLength(0);
      expect(screen.getAllByRole("button").map(words)).toEqual(["Security"]);
      expect(words(screen.getByRole("main"))).not.toContain(firstKey);
      expect(setups()).toBe(2);

      next.release();

      await screen.findByRole("textbox", { name: "Authentication code" });
      expect(sections()).toEqual(["Add to your authenticator app", "Enter the code"]);
      expect(kept("Setup key")).toEqual([newKey]);
      expect(alerts()).toEqual([scanTheNew]);
      const [told] = screen.getAllByRole("alert");
      const button = screen.getByRole("button", { name: "Turn on" });
      expect(told === undefined ? false : before(told, button)).toBe(true);
      expect(told === undefined ? false : before(code(), told)).toBe(true);
      // The old code is of the old setup: the field is empty for the new one, and has the focus, unmarked.
      expect(code().value).toBe("");
      await focusOn(code);
      expect(describedBy(code())).toEqual([hint]);
      expect(setups()).toBe(2);

      await turnOn(user, fakeTotpCode);

      await screen.findByRole("region", { name: "Recovery codes" });
      const [first, second] = sent.seen.map((s) => s.body.setup_token);
      expect(first).toBeDefined();
      expect(second).toBeDefined();
      expect(second).not.toBe(first);
    });

    test("whose new setup the server could not make says that the settings could not be loaded, without the old key, and Try again asks for a setup once more", async () => {
      const { user, fake, setups } = await read();
      fake.script("POST /two_factor/totp", { times: 1, fail: expired });
      fake.script("POST /two_factor/totp/setup", { fail: 500, times: 1 });

      await turnOn(user, fakeTotpCode);

      const failed = await screen.findByRole("alert");
      expect(words(failed)).toMatch(/^Your settings could not be loaded\.\s*Try again$/);
      expect(sections()).toEqual([]);
      expect(screen.queryAllByRole("img", { name: "QR code of the setup key" })).toHaveLength(0);
      expect(screen.queryAllByRole("textbox")).toHaveLength(0);
      expect(words(screen.getByRole("main"))).not.toContain(firstKey);
      expect(setups()).toBe(2);

      await user.click(within(failed).getByRole("button", { name: "Try again" }));

      await screen.findByRole("textbox", { name: "Authentication code" });
      expect(sections()).toEqual(["Add to your authenticator app", "Enter the code"]);
      expect(alerts()).toEqual([scanTheNew]);
      expect(setups()).toBe(3);
    });
  });
});

describe("an account with it", () => {
  test("the screen says since when it is on and how many recovery codes are left, offers new codes and to turn it off, and asks for no setup", async () => {
    const { fake, setups } = await read({ twoFactor: true });
    const held = await heldBy<{ enrolled_at: string }>(fake, "/two_factor");

    expect(sections()).toEqual(["Two-factor authentication is on", "Turn off two-factor authentication"]);
    expect(words(section("Two-factor authentication is on"))).toContain(
      `Whichever way you sign in, you then enter a code from your authenticator app. On since ${dayOf(held.enrolled_at)}.`,
    );
    expect(rows("Two-factor authentication is on")).toEqual([
      row("Recovery codes 10 codes left. Each signs you in once when you cannot use your authenticator app.", {
        offers: ["Make new codes"],
      }),
    ]);
    expect(words(section("Turn off two-factor authentication"))).toMatch(
      inOrder(
        "Turn off two-factor authentication",
        "With it off, you sign in without a code from your authenticator app.",
        "Turn off",
      ),
    );
    expect(setups()).toBe(0);
  });

  test("Make new codes asks for a code: a wrong one stays in the dialog in the server's words, and the right one shows the new codes and says the old ones no longer work", async () => {
    const { user, fake, address } = await read({ twoFactor: true });
    const sent = fake.script("POST /two_factor/recovery_codes");

    await user.click(screen.getByRole("button", { name: "Make new codes" }));

    const dialog = await asking();
    expect(words(dialog)).toContain(
      "To make new recovery codes, enter a code from your authenticator app, or a recovery code you have not used. The codes you have now stop working.",
    );
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Make new codes"]);
    const field = within(dialog).getByRole<HTMLInputElement>("textbox", { name: "Authentication code" });
    expect(document.activeElement === field).toBe(true);
    await user.type(field, "000000");
    await user.click(within(dialog).getByRole("button", { name: "Make new codes" }));

    expect(words(await within(dialog).findByRole("alert"))).toBe("Wrong code");
    expect(describedBy(field)).toEqual(["Wrong code"]);
    expect(kept("Recovery codes")).toEqual([]);

    await user.clear(field);
    await user.type(field, fakeTotpCode);
    await user.click(within(dialog).getByRole("button", { name: "Make new codes" }));

    await closed();
    await screen.findByRole("region", { name: "Recovery codes" });
    expect(new Set(kept("Recovery codes")).size).toBe(10);
    expect(words(section("Recovery codes"))).toContain(notShownAgain);
    expect(said()).toEqual(["Your new recovery codes are ready. The old ones no longer work."]);
    expect(sent.seen.map((s) => s.body.code)).toEqual(["000000", fakeTotpCode]);

    await user.click(screen.getByRole("button", { name: "Finish" }));

    expect(address.backs).toBe(1);
  });

  test("Turn off asks for a code, and once the server took it the screen goes back, asking for no new setup on the way", async () => {
    const { user, fake, address, setups, data } = await read({ twoFactor: true });
    const sent = fake.script("POST /two_factor/totp/disable");

    await user.click(screen.getByRole("button", { name: "Turn off" }));

    const dialog = await asking();
    expect(words(dialog)).toContain(
      "To turn two-factor authentication off, enter a code from your authenticator app, or a recovery code you have not used.",
    );
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Turn off"]);
    expect(sent.seen).toHaveLength(0);
    await user.type(within(dialog).getByRole("textbox", { name: "Authentication code" }), fakeTotpCode);
    await user.click(within(dialog).getByRole("button", { name: "Turn off" }));

    await waitFor(() => {
      if (address.backs === 0) throw new Error("test: the screen did not go back yet");
    });
    expect(address.backs).toBe(1);
    expect(sent.seen.map((s) => s.body.code)).toEqual([fakeTotpCode]);
    expect((await heldBy<{ enabled: boolean }>(fake, "/two_factor")).enabled).toBe(false);
    // The account is without it now, and the screen is still drawn: it does not start a setup nobody asked for.
    await atRest({ data });
    expect(dialogs()).toHaveLength(0);
    expect(setups()).toBe(0);
  });
});
