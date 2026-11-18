// SPDX-License-Identifier: AGPL-3.0-only

// Security, the settings' entry of how an account signs in (#3205): what its sections show of the account as the
// server holds it (two-factor authentication off or on, the password, the passkeys, the linked accounts, the browsers
// signed in, ADR-0090 §11), what each row's button does once the reader confirmed it, and where the focus goes when
// the reader cancels it, what the screen says when the server refuses to remove the last way to sign in (ADR-0106
// §3.6), what it says as the browser comes back from a
// login provider (the fragment of the address), and the advice under the browsers (ADR-0090 §13). The dialog that
// asks to confirm it's the reader is pinned once (../confirmation.dom.test.tsx); the screens under the entry have
// their own tests beside this one, and what needs the routes is the app's (web/app, settings-security.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { fakeExternalConfirmation, fakeOrigin } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import {
  atRest,
  buttonsOf,
  closed,
  dayOf,
  dialogs,
  heldBy,
  hourOf,
  inOrder,
  offered,
  passkeyBrowser,
  proving,
  row,
  rows,
  section,
  sections,
  words,
} from "../../test/security";
import { settingsOn, type Access, type Given } from "../../test/settings";
import { SettingsEntryScreen } from "../screen";

/** Security drawn on a server whose account stands as `access` says, before its reads answered. */
const drawn = (access: Access = {}, given: Given = {}) =>
  settingsOn(
    access,
    (address) => <SettingsEntryScreen entry="security" languages={["en", "ja"]} {...address} />,
    given,
  );

/** Security with its sections drawn: every read answered. */
async function read(access: Access = {}, given: Given = {}) {
  const on = await drawn(access, given);
  await screen.findByRole("region", { name: "Sign-in" });
  await atRest(on);
  return on;
}

/** The five reads the entry shows, each of which it waits for. */
const reads = [
  "GET /accounts/password",
  "GET /passkeys",
  "GET /two_factor",
  "GET /linked_identities",
  "GET /sessions",
] as const;

const paths = {
  password: "/settings/security/password",
  passkey: "/settings/security/passkey",
  twoFactor: "/settings/security/two-factor",
  apps: "/settings/apps",
};

/** The links of `part`, each as its words and where it leads. */
const links = (part: HTMLElement) =>
  within(part)
    .queryAllByRole("link")
    .map((link) => [words(link), link.getAttribute("href")]);

/**
 * What the section titled `title` says of a change made on it, as its statuses say it: an empty status is a place
 * kept for what it will say. That the status takes the focus is jsdom's (focus.jsdom.test.tsx).
 */
const saidIn = (title: string) =>
  within(section(title))
    .queryAllByRole("status")
    .map(words)
    .filter((text) => text !== "");

/** The confirmation that asks `name` before something ends, once it is open. */
const asking = (name: string) => screen.findByRole("alertdialog", { name });

const removing =
  "You then sign in with a passkey or a linked account. Removing your password signs you out of your other browsers and apps.";
const removed = "Your password is removed, and your other browsers and apps are signed out.";
const confirmedElsewhere =
  "You confirmed it's you with GitHub. Repeat what you started: this time you are not asked to confirm.";

describe("until the entry is read", () => {
  for (const route of reads)
    test(`it says that it loads the settings while ${route} has no answer, and shows them once it has`, async () => {
      let held: { release: () => void } | undefined;
      const { data } = await drawn(
        {},
        {
          before: ({ fake }) => {
            held = fake.script(route, { hold: "request" });
          },
        },
      );
      // The four other reads answered, and this one alone keeps the screen waiting.
      await waitFor(() => {
        if (data.queryClient.isFetching() !== 1)
          throw new Error(`test: ${String(data.queryClient.isFetching())} reads are on their way`);
      });
      expect(screen.getByRole("status").textContent).toBe("Loading your settings…");
      expect(sections()).toEqual([]);

      held?.release();

      await screen.findByRole("region", { name: "Sign-in" });
      expect(screen.queryAllByRole("status").map(words)).not.toContain("Loading your settings…");
    });

  for (const route of reads)
    test(`it says that the settings could not be loaded when ${route} fails, and Try again reads them`, async () => {
      const { user } = await drawn(
        {},
        {
          before: ({ fake }) => {
            fake.script(route, { fail: 500, times: 1 });
          },
        },
      );

      const failed = await screen.findByRole("alert");
      expect(words(failed)).toMatch(/^Your settings could not be loaded\.\s*Try again$/);
      expect(sections()).toEqual([]);

      await user.click(within(failed).getByRole("button", { name: "Try again" }));

      await screen.findByRole("region", { name: "Sign-in" });
      expect(screen.queryAllByRole("alert")).toHaveLength(0);
    });
});

test("a typical account's entry has its sections in their order, and keeps the rules of a settings screen (ADR-0110 §6)", async () => {
  await read();

  expect(sections()).toEqual([
    "Two-factor authentication is off",
    "Sign-in",
    "Passkeys 3",
    "Linked accounts",
    "Signed-in browsers 4",
  ]);
  expect(settingsFindings(document.body)).toEqual([]);
});

describe("two-factor authentication", () => {
  const stops = "Anyone who learns your password can sign in. A code from your authenticator app stops them.";
  const alsoAsks =
    "With it on, signing in also asks for a code from your authenticator app, so someone who can use a passkey or a linked account of yours still cannot sign in.";

  for (const [who, access, sentence] of [
    ["an account that a password signs in", {}, stops],
    ["an account without a password", { password: false }, alsoAsks],
    ["an account on a server that limits sign-in to login providers", { providersOnly: true }, alsoAsks],
  ] as const)
    test(`off, for ${who}: the summary says so and what it would stop, and Turn on leads to its screen`, async () => {
      await read(access);

      const summary = section("Two-factor authentication is off");
      expect(words(summary)).toMatch(inOrder("Two-factor authentication is off", sentence, "Turn on"));
      expect(links(summary)).toEqual([["Turn on", paths.twoFactor]]);
      // The summary is the one door to it: Sign-in has no row of it.
      expect(rows("Sign-in").map((r) => r.leads)).not.toContain(paths.twoFactor);
    });

  test("on: no summary, and a row of Sign-in says since when and leads to its screen", async () => {
    const { fake } = await read({ twoFactor: true });
    const held = await heldBy<{ enrolled_at: string }>(fake, "/two_factor");

    expect(sections()).toEqual(["Sign-in", "Passkeys 3", "Linked accounts", "Signed-in browsers 4"]);
    expect(rows("Sign-in")).toContainEqual(
      row(`Two-factor authentication On since ${dayOf(held.enrolled_at)}`, { leads: paths.twoFactor }),
    );
    expect(settingsFindings(document.body)).toEqual([]);
  });

  for (const [left, says] of [
    [4, ""],
    [3, " 3 recovery codes left"],
  ] as const)
    test(`on, with ${String(left)} recovery codes left: the row ${says === "" ? "does not count them" : "says how many"}`, async () => {
      const { fake } = await read(
        { twoFactor: true },
        {
          before: (on) => {
            on.fake.script("GET /two_factor", { change: (body) => ({ ...body, recovery_codes_remaining: left }) });
          },
        },
      );
      const held = await heldBy<{ enrolled_at: string }>(fake, "/two_factor");

      expect(rows("Sign-in")).toContainEqual(
        row(`Two-factor authentication On since ${dayOf(held.enrolled_at)}${says}`, { leads: paths.twoFactor }),
      );
    });
});

describe("Sign-in", () => {
  const change = row("Password Change your password", { leads: paths.password });
  const limited =
    "On this server, you sign in only with another service, so you cannot set a password or add a passkey now.";

  for (const [who, access, expected] of [
    ["an account with a password", {}, [change, row(`Remove password ${removing}`, { offers: ["Remove"] })]],
    ["an account without a password", { password: false }, [row("Password Not set", { leads: paths.password })]],
    [
      "an account whose password is its only way to sign in: no button, and why",
      { passkeys: 0, linked: "none" },
      [
        change,
        row(
          "Remove password Your password is your only way to sign in. To remove it, add a passkey or link an account first.",
        ),
      ],
    ],
    [
      "a server that limits sign-in to login providers: no way to the password's screen",
      { providersOnly: true },
      [
        row(
          "Remove password Your password does not sign you in on this server now. Removing it signs you out of your other browsers and apps.",
          { offers: ["Remove"] },
        ),
      ],
    ],
  ] as const)
    test(`the rows of ${who}`, async () => {
      await read(access);

      expect(rows("Sign-in")).toEqual([...expected]);
      // Only the server that limits sign-in says so over the rows.
      expect(words(section("Sign-in")).includes(limited)).toBe(access.providersOnly === true);
    });

  test("Remove asks to confirm with no password field, and once a passkey confirmed, the password is removed: the section says so, the row is gone, and the other browsers too", async () => {
    const browser = passkeyBrowser();
    const { user, fake } = await read();
    const sent = fake.script("POST /accounts/password/remove");

    await user.click(offered("Sign-in", "Remove password", "Remove"));

    const dialog = await proving();
    expect(words(dialog)).toContain("To remove your password, confirm it's you.");
    // The password does not confirm its own removal: a passkey does, or a linked account.
    expect(within(dialog).queryAllByLabelText("Current password")).toHaveLength(0);
    expect(buttonsOf(dialog)).toEqual(["Close", "Continue with GitHub instead", "Cancel", "Use a passkey"]);
    expect(sent.seen).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Use a passkey" }));

    await closed();
    expect(browser.asked).toBe(1);
    expect(sent.seen.map((s) => Object.keys(s.body))).toEqual([["passkey"]]);
    await within(section("Sign-in")).findByText(removed);
    expect(rows("Sign-in")).toEqual([row("Password Not set", { leads: paths.password })]);
    // The server ended the other browsers, and the list is read again.
    await waitFor(() => {
      if (rows("Signed-in browsers").length !== 1)
        throw new Error(`test: ${String(rows("Signed-in browsers").length)} browsers are listed`);
    });
  });

  test("back from a provider that confirmed the removal, the section says to repeat it, and Remove then asks nothing", async () => {
    const { user, fake, address } = await read(
      {},
      {
        fragment: (f) =>
          `external_confirmation=${fakeExternalConfirmation}&operation=password_removal&provider=${f.loginProviderId("github")}`,
      },
    );
    const sent = fake.script("POST /accounts/password/remove");
    expect(words(section("Sign-in"))).toContain(confirmedElsewhere);
    expect(address.fragmentsRead).toBe(1);

    await user.click(offered("Sign-in", "Remove password", "Remove"));

    await within(section("Sign-in")).findByText(removed);
    expect(sent.seen.map((s) => s.body)).toEqual([{ external_confirmation: fakeExternalConfirmation }]);
    expect(dialogs()).toHaveLength(0);
    expect(rows("Sign-in")).toEqual([row("Password Not set", { leads: paths.password })]);
  });
});

describe("Passkeys", () => {
  type Held = { items: { name: string; created_at: string }[] };
  const about = "Sign in with your device's screen lock in place of a password.";

  test("the heading counts them, each row says its name and the day it was added and offers Remove, and Add leads to its screen", async () => {
    const { fake } = await read();
    const held = await heldBy<Held>(fake, "/passkeys");

    expect(held.items.map((p) => p.name)).toEqual(["MacBook Touch ID", "iPhone (iCloud Keychain)", "YubiKey 5C"]);
    expect(rows("Passkeys")).toEqual(
      held.items.map((p) => row(`${p.name} Added ${dayOf(p.created_at)}`, { offers: ["Remove"] })),
    );
    expect(words(section("Passkeys"))).toContain(about);
    expect(links(section("Passkeys"))).toEqual([["Add a passkey", paths.passkey]]);
  });

  test("a passkey added in another year says the year", async () => {
    await read(
      {},
      {
        before: ({ fake }) => {
          fake.script("GET /passkeys", {
            change: (body) => ({
              ...body,
              items: body.items.slice(0, 1).map((p) => ({ ...p, created_at: "2024-03-05T12:00:00Z" })),
            }),
          });
        },
      },
    );

    expect(rows("Passkeys")).toEqual([row("MacBook Touch ID Added Mar 5, 2024", { offers: ["Remove"] })]);
  });

  test("Remove asks first, naming the passkey, and once confirmed the passkey is gone and the count is one less", async () => {
    const { user, fake } = await read();
    const sent = fake.script("DELETE /passkeys/{id}");

    await user.click(offered("Passkeys", "MacBook Touch ID", "Remove"));

    const dialog = await asking("Remove the passkey “MacBook Touch ID”?");
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Remove"]);
    expect(sent.seen).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Remove" }));

    await closed();
    await waitFor(() => {
      if (rows("Passkeys").length !== 2) throw new Error("test: the passkey is still listed");
    });
    expect(rows("Passkeys").map((r) => r.says.split(" Added ")[0])).toEqual(["iPhone (iCloud Keychain)", "YubiKey 5C"]);
    expect(sections()).toContain("Passkeys 2");
    expect(sent.seen).toHaveLength(1);
  });

  test("without a passkey the section says so in the list's place and offers Add once, with no count", async () => {
    await read({ passkeys: 0 });

    expect(sections()).toContain("Passkeys");
    expect(rows("Passkeys")).toEqual([]);
    expect(words(section("Passkeys"))).toMatch(
      inOrder("Passkeys", about, "No passkeys yet. Passkeys you add appear here.", "Add a passkey"),
    );
    expect(links(section("Passkeys"))).toEqual([["Add a passkey", paths.passkey]]);
  });

  test("where sign-in is limited to login providers, an account without a passkey has no such section", async () => {
    await read({ providersOnly: true, passkeys: 0 });

    expect(sections()).toEqual([
      "Two-factor authentication is off",
      "Sign-in",
      "Linked accounts",
      "Signed-in browsers 4",
    ]);
  });

  test("where sign-in is limited to login providers, an account's passkeys are listed to remove, and none can be added", async () => {
    await read({ providersOnly: true });

    expect(rows("Passkeys").map((r) => r.offers)).toEqual([["Remove"], ["Remove"], ["Remove"]]);
    expect(words(section("Passkeys"))).toContain(
      "Passkeys do not sign you in on this server now, and you cannot add one. You can still remove yours.",
    );
    expect(links(section("Passkeys"))).toEqual([]);
  });
});

describe("the last way to sign in", () => {
  for (const [way, access, list, named, press, title] of [
    [
      "passkey",
      { password: false, passkeys: 1, linked: "none" },
      "Passkeys",
      "MacBook Touch ID",
      "Remove",
      "Remove the passkey “MacBook Touch ID”?",
    ],
    [
      "linked account",
      { password: false, passkeys: 0 },
      "Linked accounts",
      "GitHub",
      "Unlink",
      "Unlink your GitHub account?",
    ],
  ] as const)
    test(`the last ${way} of an account without a password is not removed: the confirmation stays open and says the server's words`, async () => {
      const { user } = await read(access);
      const before = rows(list);

      await user.click(offered(list, named, press));
      await user.click(within(await asking(title)).getByRole("button", { name: press }));

      const refused = await screen.findByRole("alert");
      expect(words(refused)).toBe("This is your last way to sign in");
      expect((await asking(title)).contains(refused)).toBe(true);
      expect(rows(list)).toEqual(before);
      // Nothing was removed, so the section says nothing of it.
      expect(saidIn(list)).toEqual([]);
    });
});

describe("a confirmation of the entry left alone", () => {
  for (const [what, title, opens, dialog] of [
    ["Remove of a passkey", "Passkeys", () => offered("Passkeys", "YubiKey 5C", "Remove"), "alertdialog"],
    ["Unlink", "Linked accounts", () => offered("Linked accounts", "GitHub", "Unlink"), "alertdialog"],
    [
      "Sign out of a browser",
      "Signed-in browsers",
      () => offered("Signed-in browsers", "Safari on iPhone", "Sign out"),
      "alertdialog",
    ],
    [
      "Sign out all others",
      "Signed-in browsers",
      () => within(section("Signed-in browsers")).getByRole("button", { name: "Sign out all others" }),
      "alertdialog",
    ],
    ["Remove of the password", "Sign-in", () => offered("Sign-in", "Remove password", "Remove"), "dialog"],
  ] as const)
    test(`Cancel of ${what} gives the focus back to the button that opened it, and nothing is said`, async () => {
      const { user, requests } = await read();
      const button = opens();

      await user.click(button);
      const [open] = await screen.findAllByRole(dialog);
      if (open === undefined) throw new Error("test: no dialog opened");
      const before = requests.length;
      await user.click(within(open).getByRole("button", { name: "Cancel" }));

      await closed();
      await waitFor(() => {
        if (document.activeElement !== button) throw new Error("test: the focus is not back on the button yet");
      });
      expect(saidIn(title)).toEqual([]);
      expect(
        screen
          .queryAllByRole("status")
          .map(words)
          .filter((text) => text !== ""),
      ).toEqual([]);
      expect(requests.slice(before)).toEqual([]);
    });

  test("pressing a row's button again clears what the section said of the last change", async () => {
    const { user } = await read();
    await user.click(offered("Passkeys", "YubiKey 5C", "Remove"));
    await user.click(within(await asking("Remove the passkey “YubiKey 5C”?")).getByRole("button", { name: "Remove" }));
    await closed();
    await waitFor(() => {
      if (saidIn("Passkeys").length === 0) throw new Error("test: the section says nothing yet");
    });
    expect(saidIn("Passkeys")).toEqual(["The passkey “YubiKey 5C” is removed."]);

    await user.click(offered("Passkeys", "MacBook Touch ID", "Remove"));

    await asking("Remove the passkey “MacBook Touch ID”?");
    expect(saidIn("Passkeys")).toEqual([]);
  });
});

describe("Linked accounts", () => {
  type Held = { items: { provider_name: string; linked_at: string }[] };

  test("a linked account says since when and offers Unlink, a provider that allows linking offers Link, and one that does not is not listed", async () => {
    const { fake } = await read();
    const [github] = (await heldBy<Held>(fake, "/linked_identities")).items;
    if (github === undefined) throw new Error("test: the fake links no account");

    expect(rows("Linked accounts")).toEqual([
      row(`GitHub Linked ${dayOf(github.linked_at)}`, { offers: ["Unlink"] }),
      row("Example SSO Not linked", { offers: ["Link"] }),
    ]);
    expect(words(section("Linked accounts"))).toContain("Sign in with an account you have on another service.");
  });

  test("a server without login providers has no such section", async () => {
    await read({ linked: "no-providers" });

    expect(sections()).toEqual(["Two-factor authentication is off", "Sign-in", "Passkeys 3", "Signed-in browsers 4"]);
  });

  test("Unlink asks first, naming the provider, and once confirmed the provider offers Link again", async () => {
    const { user, fake } = await read();
    const sent = fake.script("DELETE /linked_identities/{id}");

    await user.click(offered("Linked accounts", "GitHub", "Unlink"));

    const dialog = await asking("Unlink your GitHub account?");
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Unlink"]);
    expect(sent.seen).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Unlink" }));

    await closed();
    await waitFor(() => {
      if (rows("Linked accounts")[0]?.says !== "GitHub Not linked") throw new Error("test: GitHub is still linked");
    });
    expect(rows("Linked accounts")).toEqual([
      row("GitHub Not linked", { offers: ["Link"] }),
      row("Example SSO Not linked", { offers: ["Link"] }),
    ]);
    expect(sent.seen).toHaveLength(1);
  });

  test("Link asks to confirm it's the reader, then asks the server for the way to the provider, back to this entry, and the app leaves for it", async () => {
    const { user, fake, address } = await read();
    const sent = fake.script("POST /linked_identities");

    await user.click(offered("Linked accounts", "Example SSO", "Link"));

    const dialog = await proving();
    expect(words(dialog)).toContain(
      "To link an account, confirm it's you. You then sign in at the other service and come back here.",
    );
    expect(sent.seen).toHaveLength(0);
    await user.type(within(dialog).getByLabelText("Current password"), "demo-password-yuki");
    await user.click(within(dialog).getByRole("button", { name: "Confirm" }));

    await waitFor(() => {
      if (address.left.length === 0) throw new Error("test: the app did not leave yet");
    });
    const provider = fake.loginProviderId("oidc");
    expect(sent.seen.map((s) => s.body)).toEqual([
      { provider_id: provider, password: "demo-password-yuki", return_to: "/settings/security" },
    ]);
    // The server's answer, as it is: the address of its own page that starts the provider's sign-in.
    expect(address.left).toHaveLength(1);
    expect(address.left[0]?.startsWith(`${fakeOrigin}/auth/external/${provider}/link?`)).toBe(true);
  });
});

describe("back from a login provider", () => {
  const linked = (name: string) => `Your ${name} is linked. You can sign in with it now.`;

  test("with an account linked, Linked accounts says whose, and the fragment is read once", async () => {
    const { address } = await read({}, { fragment: (f) => `linked=${f.loginProviderId("github")}` });

    expect(words(section("Linked accounts"))).toContain(linked("GitHub account"));
    expect(address.fragmentsRead).toBe(1);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });

  test("with an account linked at a provider the server lists nowhere, it says so without a name", async () => {
    await read({}, { fragment: () => "linked=01900000-0000-7000-8000-000000000000" });

    expect(words(section("Linked accounts"))).toContain(linked("account at the other service"));
  });

  for (const [reason, sentence] of [
    [
      "denied",
      "This server does not accept that account of the other service. Try another account there, or ask the administrators.",
    ],
    [
      "linked_elsewhere",
      "That account of the other service is linked to another account on this server. Unlink it there first, or link a different one.",
    ],
    [
      "mismatch",
      "The account you signed in with at the other service is not linked to yours. Try again with the linked account.",
    ],
    ["unavailable", "That service cannot be used now. Try again later, or use another way."],
    ["expired", "That took too long. Try again."],
    ["failed", "The sign-in with the other service did not complete. Try again."],
    // A reason a newer server sends reads as a sign-in that did not complete.
    ["something_new", "The sign-in with the other service did not complete. Try again."],
  ] as const)
    test(`with the error ${reason}, Linked accounts says why as an alert`, async () => {
      const { address } = await read(
        {},
        { fragment: (f) => `external_error=${reason}&operation=link&provider=${f.loginProviderId("github")}` },
      );

      const [alert, ...others] = screen.getAllByRole("alert");
      expect(others).toHaveLength(0);
      expect(alert === undefined ? "" : words(alert)).toBe(sentence);
      expect(section("Linked accounts").contains(alert ?? null)).toBe(true);
      expect(address.fragmentsRead).toBe(1);
    });

  test("with a provider's confirmation of a link, Linked accounts says to repeat it, and Link then sends the confirmation with no dialog", async () => {
    const { user, fake, address } = await read(
      {},
      {
        fragment: (f) =>
          `external_confirmation=${fakeExternalConfirmation}&operation=link&provider=${f.loginProviderId("github")}`,
      },
    );
    const sent = fake.script("POST /linked_identities");
    expect(words(section("Linked accounts"))).toContain(confirmedElsewhere);
    expect(address.fragmentsRead).toBe(1);

    await user.click(offered("Linked accounts", "Example SSO", "Link"));

    await waitFor(() => {
      if (address.left.length === 0) throw new Error("test: the app did not leave yet");
    });
    expect(dialogs()).toHaveLength(0);
    expect(sent.seen.map((s) => s.body)).toEqual([
      {
        provider_id: fake.loginProviderId("oidc"),
        external_confirmation: fakeExternalConfirmation,
        return_to: "/settings/security",
      },
    ]);
  });
});

describe("Signed-in browsers", () => {
  type Session = { id: string; ip?: string; signed_in_at: string; last_used_at?: string };
  type Held = { items: Session[] };
  const others = "Sign out all others";

  /** The lines of a browser's row after its name: when it signed in, from where, and when it was last used. */
  const lines = (s: Session) =>
    [
      `Signed in ${dayOf(s.signed_in_at)}`,
      s.ip === undefined ? "Signed in from an unknown address" : `Signed in from ${s.ip}`,
      ...(s.last_used_at === undefined ? [] : [`Last used ${hourOf(s.last_used_at)}`]),
    ].join(" ");

  test("this browser comes first, marked, with no Sign out; each other says its browser and system, when and from where it signed in, and when it was last used", async () => {
    const { fake } = await read();
    const [own, phone, desktop, unknown, ...more] = (await heldBy<Held>(fake, "/sessions")).items;
    if (own === undefined || phone === undefined || desktop === undefined || unknown === undefined)
      throw new Error("test: the fake holds fewer than four browsers");

    expect(more).toEqual([]);
    expect(rows("Signed-in browsers")).toEqual([
      row(`Firefox on Linux This browser ${lines(own)}`),
      row(`Safari on iPhone ${lines(phone)}`, { offers: ["Sign out"] }),
      row(`Chrome on Windows ${lines(desktop)}`, { offers: ["Sign out"] }),
      // Neither its browser nor its address is kept (ADR-0090 §11), and it was never used since.
      row(`Unknown browser ${lines(unknown)}`, { offers: ["Sign out"] }),
    ]);
    expect(within(section("Signed-in browsers")).getAllByRole("button", { name: others })).toHaveLength(1);
  });

  test("a User-Agent that names no browser and system the app knows shows as it is", async () => {
    await read(
      {},
      {
        before: ({ fake }) => {
          fake.script("GET /sessions", {
            change: (body) => ({
              ...body,
              items: body.items.map((s) => (s.current ? s : { ...s, user_agent: "curl/8.5.0" })).slice(0, 2),
            }),
          });
        },
      },
    );

    expect(rows("Signed-in browsers").map((r) => r.says.split(" Signed in ")[0])).toEqual([
      "Firefox on Linux This browser",
      "curl/8.5.0",
    ]);
  });

  test("Sign out asks first, naming the browser, with no password to enter, and once confirmed that browser is gone", async () => {
    const { user, fake } = await read();
    const [, phone] = (await heldBy<Held>(fake, "/sessions")).items;
    const sent = fake.script("DELETE /sessions/{id}");

    await user.click(offered("Signed-in browsers", "Safari on iPhone", "Sign out"));

    const dialog = await asking("Sign out Safari on iPhone?");
    expect(words(dialog)).toContain(
      "That browser has to sign in again to use your account. Your password, two-factor authentication and apps stay as they are.",
    );
    // A confirmation, and nothing to prove (ADR-0090 §13).
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Sign out"]);
    expect(within(dialog).queryAllByLabelText(/password/i)).toHaveLength(0);
    expect(sent.seen).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Sign out" }));

    await closed();
    await waitFor(() => {
      if (rows("Signed-in browsers").length !== 3) throw new Error("test: the browser is still listed");
    });
    expect(rows("Signed-in browsers").map((r) => r.says.split(" Signed in ")[0])).toEqual([
      "Firefox on Linux This browser",
      "Chrome on Windows",
      "Unknown browser",
    ]);
    expect(sections()).toContain("Signed-in browsers 3");
    expect(sent.seen.map((s) => s.url.pathname.split("/").at(-1))).toEqual([phone?.id]);
  });

  test("the confirmation of a browser the server knows nothing of asks about this unknown browser", async () => {
    const { user } = await read();

    await user.click(offered("Signed-in browsers", "Unknown browser", "Sign out"));

    expect(buttonsOf(await asking("Sign out this unknown browser?"))).toEqual(["Close", "Cancel", "Sign out"]);
  });

  test("Sign out all others asks first, and once confirmed only this browser is listed and the button is gone", async () => {
    const { user, fake } = await read();
    const sent = fake.script("POST /sessions/revoke_others");

    await user.click(within(section("Signed-in browsers")).getByRole("button", { name: others }));

    const dialog = await asking("Sign out all other browsers?");
    expect(words(dialog)).toContain(
      "Every browser but this one has to sign in again to use your account. Your password, two-factor authentication and apps stay as they are.",
    );
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Sign out"]);
    expect(sent.seen).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Sign out" }));

    await closed();
    await waitFor(() => {
      if (rows("Signed-in browsers").length !== 1) throw new Error("test: other browsers are still listed");
    });
    expect(rows("Signed-in browsers")[0]?.says.startsWith("Firefox on Linux This browser")).toBe(true);
    expect(sections()).toContain("Signed-in browsers 1");
    expect(within(section("Signed-in browsers")).queryAllByRole("button")).toHaveLength(0);
    expect(sent.seen).toHaveLength(1);
  });

  test("an account signed in to this browser alone is offered no Sign out", async () => {
    await read({ sessions: "alone" });

    expect(rows("Signed-in browsers").map((r) => r.offers)).toEqual([[]]);
    expect(within(section("Signed-in browsers")).queryAllByRole("button")).toHaveLength(0);
  });

  // ADR-0090 §13: the advice for a sign-in the reader does not recognize.
  for (const [who, access, advice, leads] of [
    [
      "an account that a password signs in is advised to change it and to check its apps",
      {},
      "If you do not recognize a sign-in, sign that browser out, change your password, and check which apps can use your account.",
      [
        ["Change password", paths.password],
        ["Check your apps", paths.apps],
      ],
    ],
    [
      "an account without a password is advised to check its apps",
      { password: false },
      "If you do not recognize a sign-in, sign that browser out and check which apps can use your account.",
      [["Check your apps", paths.apps]],
    ],
    [
      "an account whose password does not sign in on this server is advised to check its apps",
      { providersOnly: true },
      "If you do not recognize a sign-in, sign that browser out and check which apps can use your account.",
      [["Check your apps", paths.apps]],
    ],
  ] as const)
    test(`under the list, ${who}`, async () => {
      await read(access);

      const browsers = section("Signed-in browsers");
      // The advice ends the section, with its links after it.
      expect(words(browsers).endsWith(`${advice} ${leads.map(([name]) => name).join(" · ")}`)).toBe(true);
      expect(links(browsers)).toEqual(leads.map((lead) => [...lead]));
    });
});
