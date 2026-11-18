// SPDX-License-Identifier: AGPL-3.0-only

// Where the focus goes once a change under Security is said (#3205), on jsdom: what takes it is no control (the
// status a section says its result in, the heading of the recovery codes, the title of a screen whose setup expired),
// and happy-dom gives the focus to any element a script asks it for, focusable or not. A change on the entry says its
// result at the top of its section, as a status that takes the focus once the confirmation has left, also when the
// server no longer held what was to be removed, and when the section has no row left; the recovery codes' view takes
// it to its heading as it appears; and a setup that expired gives it to the screen's title until the new one is there. What the status says and where it stands is pinned here with the focus, and the rest of each change on
// happy-dom (security.dom.test.tsx, two-factor.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { fakeExternalConfirmation, fakeTotpCode } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import {
  atRest,
  closed,
  focused,
  focusOn,
  heldBy,
  noNewLinks,
  offered,
  passkeyBrowser,
  removedElsewhere,
  section,
} from "../../test/security";
import { requestsOnJsdom, settingsOn, type Access, type Given } from "../../test/settings";
import { SettingsEntryScreen } from "../screen";
import { SecurityTwoFactor } from "./two-factor";

/** Security with every read answered, on a server whose account stands as `access` says. */
async function entry(access: Access = {}, given: Given = {}) {
  requestsOnJsdom();
  const on = await settingsOn(
    access,
    (address) => <SettingsEntryScreen entry="security" languages={["en"]} {...address} />,
    given,
  );
  await screen.findByRole("region", { name: "Sign-in" });
  await atRest(on);
  return on;
}

/**
 * Waits until the focus is on a status of the section titled `title`, and returns what that status says: the focus
 * comes once the confirmation has left, a frame after the dialog closed.
 */
async function saidWithFocus(title: string): Promise<string> {
  await waitFor(() => {
    const statuses: Element[] = within(section(title)).queryAllByRole("status");
    if (document.activeElement === null || !statuses.includes(document.activeElement))
      throw new Error(`test: the focus is on ${focused()}`);
  });
  return focused();
}

/** Presses the confirmation's button `press` in the alert dialog named `name`. */
async function confirm(user: Awaited<ReturnType<typeof entry>>["user"], name: string, press: string) {
  const dialog = await screen.findByRole("alertdialog", { name });
  await user.click(within(dialog).getByRole("button", { name: press }));
  await closed();
}

describe("a change on the entry says its result at the top of its section, which takes the focus", () => {
  // Each with the list the server holds it in, and its place there.
  const changes = [
    [
      "a passkey removed",
      "Passkeys",
      "YubiKey 5C",
      "Remove",
      "Remove the passkey “YubiKey 5C”?",
      "Remove",
      "The passkey “YubiKey 5C” is removed.",
      "/passkeys",
      2,
    ],
    [
      "an account unlinked",
      "Linked accounts",
      "GitHub",
      "Unlink",
      "Unlink your GitHub account?",
      "Unlink",
      "Your GitHub account is unlinked.",
      "/linked_identities",
      0,
    ],
    [
      "a browser signed out",
      "Signed-in browsers",
      "Safari on iPhone",
      "Sign out",
      "Sign out Safari on iPhone?",
      "Sign out",
      "Safari on iPhone is signed out.",
      "/sessions",
      1,
    ],
    [
      "the browser without a recorded origin signed out",
      "Signed-in browsers",
      "Unknown browser",
      "Sign out",
      "Sign out this unknown browser?",
      "Sign out",
      "The unknown browser is signed out.",
      "/sessions",
      3,
    ],
  ] as const;

  for (const [what, title, row, button, dialog, press, result] of changes)
    test(what, async () => {
      const { user } = await entry();

      await user.click(offered(title, row, button));
      await confirm(user, dialog, press);

      expect(await saidWithFocus(title)).toBe(`status: ${result}`);
    });

  // Another browser of the reader's removed it while the confirmation was open: the server answers 404, and what the
  // reader asked for is done. One of each kind of thing.
  for (const [what, title, row, button, dialog, press, result, list, at] of changes.slice(0, 3))
    test(`${what}, which the server no longer holds (404): the same is said, no row offers it any longer, and the list is read again once`, async () => {
      const on = await entry();
      const { user, fake, requests } = on;
      const { items } = await heldBy<{ items: { id: string }[] }>(fake, list);
      const id = items[at]?.id ?? "";
      await user.click(offered(title, row, button));
      await screen.findByRole("alertdialog", { name: dialog });
      await removedElsewhere(fake, `${list}/${id}`);
      const before = requests.length;

      await confirm(user, dialog, press);

      expect(await saidWithFocus(title)).toBe(`status: ${result}`);
      await atRest(on);
      expect(requests.slice(before)).toEqual([`DELETE ${list}/${id}`, `GET ${list}`]);
      // A passkey's and a browser's row is gone; a provider's stays, to link an account at it again.
      expect(
        within(section(title))
          .queryAllByRole("group", { name: row })
          .flatMap((group) => within(group).queryAllByRole("button", { name: button })),
      ).toHaveLength(0);
      expect(focused()).toBe(`status: ${result}`);
    });

  // No row is left in the section, which stays for what it says: the focus has the status to go to.
  test("an account's only link unlinked, at a provider that takes no new link: the status takes the focus, and keeps it once the links are read again", async () => {
    const on = await entry({}, noNewLinks);

    await on.user.click(offered("Linked accounts", "GitHub", "Unlink"));
    await confirm(on.user, "Unlink your GitHub account?", "Unlink");

    expect(await saidWithFocus("Linked accounts")).toBe("status: Your GitHub account is unlinked.");
    // A confirmation gives the focus back a frame after it left, to the button that opened it, which is gone.
    await atRest(on);
    await new Promise((frames) => setTimeout(frames, 50));
    expect(focused()).toBe("status: Your GitHub account is unlinked.");
    expect(within(section("Linked accounts")).queryAllByRole("list")).toHaveLength(0);
  });

  test("the other browsers signed out", async () => {
    const { user } = await entry();

    await user.click(within(section("Signed-in browsers")).getByRole("button", { name: "Sign out all others" }));
    await confirm(user, "Sign out all other browsers?", "Sign out");

    expect(await saidWithFocus("Signed-in browsers")).toBe("status: Your other browsers are signed out.");
  });

  const removed = "status: Your password is removed, and your other browsers and apps are signed out.";

  test("the password removed through the dialog, once a passkey confirmed it", async () => {
    passkeyBrowser();
    const { user } = await entry();

    await user.click(offered("Sign-in", "Remove password", "Remove"));
    const dialog = await screen.findByRole("dialog", { name: "Confirm it's you" });
    await user.click(within(dialog).getByRole("button", { name: "Use a passkey" }));
    await closed();

    expect(await saidWithFocus("Sign-in")).toBe(removed);
  });

  test("the password removed on a provider's confirmation, with no dialog", async () => {
    const { user } = await entry(
      {},
      {
        fragment: (f) =>
          `external_confirmation=${fakeExternalConfirmation}&operation=password_removal&provider=${f.loginProviderId("github")}`,
      },
    );

    await user.click(offered("Sign-in", "Remove password", "Remove"));

    expect(await saidWithFocus("Sign-in")).toBe(removed);
  });
});

/** Two-factor authentication's screen, once it is at rest. */
async function twoFactor(access: Access) {
  requestsOnJsdom();
  const on = await settingsOn(access, (address) => <SecurityTwoFactor {...address} />);
  await screen.findAllByRole("region");
  await atRest(on);
  return on;
}

// The field that had the focus is gone with the setup it belonged to: the screen's title takes it, the one part
// that stays, and the new setup's field once it is there.
test("a setup of two-factor authentication that expired gives the focus to the screen's title while the new one is on its way, and to the code's field once it is there", async () => {
  const { user, fake } = await twoFactor({});
  fake.script("POST /two_factor/totp", {
    times: 1,
    fail: { type: "/api/v1/problems/totp-setup-expired", title: "The setup expired", status: 422 },
  });
  const next = fake.script("POST /two_factor/totp/setup", { hold: "request" });

  await user.type(screen.getByRole("textbox", { name: "Authentication code" }), fakeTotpCode);
  await user.click(screen.getByRole("button", { name: "Turn on" }));

  await next.arrived();
  await focusOn(() => screen.queryAllByRole("heading", { name: "Two-factor authentication" })[0]);
  expect(focused()).toBe("heading: Two-factor authentication");
  expect(screen.queryAllByRole("textbox")).toHaveLength(0);

  next.release();

  await focusOn(() => screen.queryAllByRole("textbox", { name: "Authentication code" })[0]);
  expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Authentication code" }).value).toBe("");
});

describe("the recovery codes' view takes the focus to its heading as it appears", () => {
  /** Waits until the heading named "Recovery codes" has the focus. */
  const headingFocused = () =>
    waitFor(() => {
      const [heading] = screen.queryAllByRole("heading", { name: "Recovery codes" });
      if (heading === undefined || document.activeElement !== heading)
        throw new Error(`test: the focus is on ${focused()}`);
    });

  test("after turning two-factor authentication on", async () => {
    const { user } = await twoFactor({});

    await user.type(screen.getByRole("textbox", { name: "Authentication code" }), fakeTotpCode);
    await user.click(screen.getByRole("button", { name: "Turn on" }));

    await headingFocused();
    expect(focused()).toBe("heading: Recovery codes");
  });

  test("after making new codes", async () => {
    const { user } = await twoFactor({ twoFactor: true });

    await user.click(screen.getByRole("button", { name: "Make new codes" }));
    const dialog = await screen.findByRole("dialog", { name: "Enter a code" });
    await user.type(within(dialog).getByRole("textbox", { name: "Authentication code" }), fakeTotpCode);
    await user.click(within(dialog).getByRole("button", { name: "Make new codes" }));
    await closed();

    await headingFocused();
    expect(focused()).toBe("heading: Recovery codes");
  });
});
