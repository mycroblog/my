// SPDX-License-Identifier: AGPL-3.0-only

// Leaving an account in the settings (#3203; FR-008, FR-009, ADR-0089 §3, §8, §11–§17), as the app places it: Move
// account, Deactivate account and Delete account under the entry Account and data, each at an address of its own.
// What each asks before it acts (the confirmation that the act names, which a login provider's return makes a plain
// question), what it sends, and how it says the server's refusals, on the fake server through the whole app. That the
// server ended the session is told to the route once (the feature's settings/account-leave.dom.test.tsx counts it);
// where the focus lands when it is no control (a summary's title) is the feature's jsdom test
// (settings/account-data.jsdom.test.tsx).

import { confirmationKeptKey } from "@my/app-features";
import { settingsFindings } from "@my/design-system/test/settings";
import { fakeExternalConfirmation, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, onTestFinished, test } from "bun:test";
import {
  aTurn,
  buttons,
  closed,
  confirmations,
  dayOf,
  describedBy,
  focusOn,
  heldBy,
  section,
  sectionRead,
  sections,
} from "./test/account-data";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

const moveAt = "/settings/account/move";
const deactivateAt = "/settings/account/deactivate";
const deleteAt = "/settings/account/delete";

type Settings = NonNullable<FakeOptions["settings"]>;

/** The account the reader moves to: a person of the fake's scenario on another server. */
const target = "sora@photos.example";
const targetHandle = `@${target}`;
const password = "demo-password-yuki";

/** The field that names the account to move to. */
const targetField = () => main().getByRole<HTMLInputElement>("textbox", { name: "Account you are moving to" });

/** The button that moves, at the screen's end. */
const moveButton = () =>
  within(section("Move this account")).queryAllByRole("button", { name: "Move account" })[0] ??
  main().getAllByRole("button", { name: "Move account" }).at(-1) ??
  document.body;

/** Writes `handle` as the account to move to and presses Move account. */
async function moving(app: TestApp, handle: string) {
  if (handle !== "") {
    await app.user.click(targetField());
    await app.user.paste(handle);
  }
  await app.user.click(moveButton());
}

/** Waits until the dialog titled `title` is open, of either kind, and returns it. */
const dialogTitled = (title: string) =>
  waitFor(() => {
    const [found] = [
      ...screen.queryAllByRole("dialog", { name: title }),
      ...confirmations().filter((d) => within(d).queryAllByRole("heading", { name: title }).length > 0),
    ];
    if (found === undefined) throw new Error(`test: no dialog "${title}" is open`);
    return found;
  });

/** The password's field of `dialog`. */
const passwordOf = (dialog: HTMLElement) => within(dialog).getByLabelText<HTMLInputElement>("Current password");

/** Confirms `dialog` with `typed`, pressing its button `act`. */
async function confirmWith(app: TestApp, dialog: HTMLElement, act: string, typed = password) {
  await app.user.type(passwordOf(dialog), typed);
  await app.user.click(within(dialog).getByRole("button", { name: act }));
}

/** Waits until `script` saw `count` requests. */
const sentTimes = (script: { seen: readonly unknown[] }, count = 1) =>
  waitFor(() => {
    if (script.seen.length < count) throw new Error(`test: ${String(script.seen.length)} requests were sent`);
  });

/** Waits until the field that names the account is described by `rule`. */
const refusedWith = (rule: string) =>
  waitFor(() => {
    if (!describedBy(targetField()).includes(rule))
      throw new Error(`test: the field is described by ${JSON.stringify(describedBy(targetField()))}`);
  });

/** A provider's return to `at`, which confirmed `operation` there. */
const confirmedAt = (app: { fake: { loginProviderId: (kind: "github") => string } }, at: string, operation: string) =>
  `${at}#external_confirmation=${fakeExternalConfirmation}&operation=${operation}&provider=${app.fake.loginProviderId("github")}`;

/** When the reader's account last moved and from when it can move again, as the server says them now. */
async function moveWait(fake: FakeApi): Promise<{ last: string | undefined; next: string }> {
  const account = await heldBy<{ last_moved_at?: string; can_move_at?: string }>(fake, "/accounts/verify_credentials");
  if (account.can_move_at === undefined) throw new Error("test: the account can move now");
  return { last: account.last_moved_at, next: account.can_move_at };
}

/** `text` with its isolating marks taken out and every run of spaces as one space. */
const spaced = (text: string) =>
  text
    .replace(new RegExp(`[${String.fromCodePoint(0x2068)}${String.fromCodePoint(0x2069)}]`, "gu"), "")
    .replace(/\s+/gu, " ");

const escaped = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** `iso` as a day, and as a day and a time, in English, as patterns. */
const dayEn = (iso: string) => escaped(dayOf(iso));
const dayTimeEn = (iso: string) =>
  `${dayEn(iso)},? (?:at )?${escaped(spaced(new Intl.DateTimeFormat("en", { timeStyle: "short" }).format(new Date(iso))))}`;

/** `iso` as a day, and as a day and a time, in Japanese, as patterns by its numbers (2026/11/10 or 2026年11月10日). */
const partsOf = (iso: string) => {
  const parts = new Intl.DateTimeFormat("en", {
    year: "numeric",
    month: "numeric",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(iso));
  const part = (type: Intl.DateTimeFormatPartTypes) => Number(parts.find((p) => p.type === type)?.value ?? "");
  return { year: part("year"), month: part("month"), day: part("day"), hour: part("hour"), minute: part("minute") };
};
const dayJa = (iso: string) => {
  const { year, month, day } = partsOf(iso);
  return `${String(year)}[/年]0?${String(month)}[/月]0?${String(day)}日?(?:\\([^)]*\\))?`;
};
const dayTimeJa = (iso: string) => {
  const { hour, minute } = partsOf(iso);
  return `${dayJa(iso)} ?0?${String(hour)}:${String(minute).padStart(2, "0")}`;
};

/** The line under "This account cannot move now" that says from when it can move again, in English. */
const laterAbout = ({ last, next }: { last: string | undefined; next: string }) =>
  new RegExp(
    last === undefined
      ? `This account can move again from ${dayTimeEn(next)}\\. The wait counts from its last move, even if that move was cancelled\\.`
      : `This account last moved on ${dayEn(last)} \\(a cancelled move counts too\\)\\. It can move again from ${dayTimeEn(next)}\\.`,
  );

// ---- Move account ---------------------------------------------------------------------------------------------------

describe("Move account", () => {
  test("an empty field is refused under it, with the focus in it", async () => {
    const app = await renderApp(moveAt);
    await sectionRead("Move this account");
    const moved = app.fake.script("POST /accounts/move");

    await moving(app, "");

    await refusedWith("Enter the handle or the address of the account you are moving to.");
    expect(targetField().getAttribute("aria-invalid")).toBe("true");
    await focusOn(targetField);
    expect(dialogsOpen()).toBe(0);
    expect(moved.seen).toHaveLength(0);
  });

  for (const [what, handle, rule] of [
    [
      "a handle no account is found at",
      "nobody@nowhere.example",
      "No account was found there. Check the handle, such as @name@example.com, or the address of the profile.",
    ],
    ["the account's own", "yuki", "That is this account. Enter the account you are moving to."],
  ] as const)
    test(`${what} is refused under the field, and nothing asks or moves`, async () => {
      const app = await renderApp(moveAt);
      await sectionRead("Move this account");
      const moved = app.fake.script("POST /accounts/move");

      await moving(app, handle);

      await refusedWith(rule);
      await focusOn(targetField);
      await aTurn();
      expect(dialogsOpen()).toBe(0);
      expect(moved.seen).toHaveLength(0);
    });

  test("an account found opens the confirmation the move names: its title, what it does, and its button", async () => {
    const app = await renderApp(moveAt);
    await sectionRead("Move this account");

    await moving(app, target);

    const dialog = await dialogTitled(`Move this account to ${targetHandle}?`);
    expect(words(dialog)).toContain(
      `Your followers move to ${targetHandle}, and everything else stays here. Cancelling the move later does not bring them back, and this account cannot move again for 30 days.`,
    );
    expect(buttons(dialog).at(-1)).toBe("Move account");
    expect(within(dialog).queryAllByLabelText("Current password")).toHaveLength(1);
  });

  test("a wrong password stays in the dialog, and the right one moves the account, which then says where it moved", async () => {
    const app = await renderApp(moveAt);
    await sectionRead("Move this account");
    const moved = app.fake.script("POST /accounts/move");
    await moving(app, target);
    const dialog = await dialogTitled(`Move this account to ${targetHandle}?`);

    await confirmWith(app, dialog, "Move account", "not-the-password");

    expect(words(await within(dialog).findByRole("alert"))).toBe("The password is wrong. Enter your current password.");
    await sentTimes(moved);

    await app.user.clear(passwordOf(dialog));
    await confirmWith(app, dialog, "Move account");

    await sectionRead(`This account moved to ${targetHandle}`);
    await closed();
    expect(moved.seen.map((s) => s.body)).toEqual([
      { target_account_id: app.fake.accountId(target), password: "not-the-password" },
      { target_account_id: app.fake.accountId(target), password },
    ]);
    expect(main().queryAllByRole("textbox")).toHaveLength(0);
  });

  const refusals: readonly [NonNullable<Settings["moveRefusal"]>, string][] = [
    [
      "alias_missing",
      `${targetHandle} does not list this account as an alias yet. On that account, add this account under “Aliases”, then try again.`,
    ],
    ["target_moved", `${targetHandle} has moved to another account itself. Enter the account it moved to.`],
    [
      "target_unavailable",
      `You cannot move to ${targetHandle}: this server cannot reach that account, or does not accept it. Check the handle, or choose another account.`,
    ],
  ];
  for (const [reason, rule] of refusals)
    test(`a move the server refuses (${reason}) says why under the field`, async () => {
      const app = await renderApp(moveAt, { settings: { moveRefusal: reason } });
      await sectionRead("Move this account");
      await moving(app, target);

      await confirmWith(app, await dialogTitled(`Move this account to ${targetHandle}?`), "Move account");

      await closed();
      await refusedWith(rule);
      expect(targetField().getAttribute("aria-invalid")).toBe("true");
      expect(targetField().value).toBe(target);
    });

  test("a move too soon after the last takes the place of the form: the account cannot move now, with the dates the account read again says, no field and no button", async () => {
    const app = await renderApp(moveAt, { settings: { moveRefusal: "too_soon" } });
    await sectionRead("Move this account");
    await moving(app, target);

    await confirmWith(app, await dialogTitled(`Move this account to ${targetHandle}?`), "Move account");

    const later = await sectionRead("This account cannot move now");
    await closed();
    expect(sections().at(-1)).toBe("This account cannot move now");
    expect(spaced(words(later))).toMatch(laterAbout(await moveWait(app.fake)));
    expect(main().queryAllByRole("textbox")).toHaveLength(0);
    expect(main().queryAllByRole("button", { name: "Move account" })).toHaveLength(0);
  });

  test("a move to an account it moved to before is refused under the field, which takes the focus, and the form stays", async () => {
    const app = await renderApp(moveAt, { settings: { moveRefusal: "repeated" } });
    await sectionRead("Move this account");
    await moving(app, target);

    await confirmWith(app, await dialogTitled(`Move this account to ${targetHandle}?`), "Move account");

    await closed();
    await refusedWith(
      `This account has moved to ${targetHandle} before, and can move to each account only once, even if that move was cancelled. Choose another account.`,
    );
    expect(targetField().getAttribute("aria-invalid")).toBe("true");
    await focusOn(targetField);
    expect(sections()).not.toContain("This account cannot move now");
    expect(main().queryAllByRole("button", { name: "Move account" }).length).toBeGreaterThan(0);
  });

  describe("of an account that moved within 30 days, the dates in the act's place and no form", () => {
    test("with the day it last moved, and the day and time from which it can move again", async () => {
      const app = await renderApp(moveAt, { settings: { lastMoved: "-12d" } });

      const later = await sectionRead("This account cannot move now");

      const wait = await moveWait(app.fake);
      expect(wait.last).toBeDefined();
      expect(sections().at(-1)).toBe("This account cannot move now");
      expect(sections()).not.toContain("Move this account");
      expect(spaced(words(later))).toMatch(laterAbout(wait));
      expect(main().queryAllByRole("textbox")).toHaveLength(0);
      expect(main().queryAllByRole("button", { name: "Move account" })).toHaveLength(0);
    });

    test("where the account does not say when it last moved, from when it can move again alone", async () => {
      const on = fakeApp({ settings: { lastMoved: "-12d" } });
      const { next } = await moveWait(on.fake);
      on.fake.script("GET /accounts/verify_credentials", {
        change: ({ last_moved_at: _left, ...account }) => account,
      });
      await renderApp(moveAt, { on });

      const later = await sectionRead("This account cannot move now");

      expect(spaced(words(later))).toMatch(laterAbout({ last: undefined, next }));
      expect(spaced(words(later))).not.toMatch(/last moved on/);
      expect(main().queryAllByRole("textbox")).toHaveLength(0);
      expect(main().queryAllByRole("button", { name: "Move account" })).toHaveLength(0);
    });

    test("in Japanese", async () => {
      const app = await renderApp(moveAt, {
        settings: { lastMoved: "-12d" },
        preferences: { locale: "ja", locale_follows_browser: false },
      });

      const later = await sectionRead("このアカウントは今は移転できません");

      const { last, next } = await moveWait(app.fake);
      expect(spaced(words(later))).toMatch(
        new RegExp(
          `このアカウントが最後に移転したのは ?${dayJa(last ?? "")} ?です ?\\(取り消した移転も数えます\\)。 ?${dayTimeJa(next)} ?から再び移転できます。`,
        ),
      );
      expect(main().queryAllByRole("textbox")).toHaveLength(0);
    });
  });

  describe("back from a login provider that confirmed the move", () => {
    /** The screen as the provider's return opens it, with the account named before leaving kept by the tab. */
    async function returned(options: FakeOptions = {}) {
      const key = confirmationKeptKey("account_move");
      sessionStorage.setItem(key, target);
      onTestFinished(() => {
        sessionStorage.removeItem(key);
      });
      const on = fakeApp(options);
      const app = await renderApp(confirmedAt(on, moveAt, "account_move"), { on });
      await sectionRead("Move this account");
      return { app, key };
    }

    test("keeps the account named in the field, once: the tab's copy is gone", async () => {
      const { key } = await returned();

      await waitFor(() => {
        if (targetField().value !== target) throw new Error(`test: the field holds "${targetField().value}"`);
      });
      expect(sessionStorage.getItem(key)).toBeNull();
      expect(words(screen.getByRole("main"))).toContain(
        "You confirmed it's you with GitHub. Do it again below: this time you are only asked whether to go ahead.",
      );
    });

    test("Move account asks only whether to, with no password, and moves with the provider's confirmation", async () => {
      const { app } = await returned();
      const moved = app.fake.script("POST /accounts/move");
      await waitFor(() => {
        if (targetField().value !== target) throw new Error("test: the field is empty");
      });

      await app.user.click(moveButton());

      const [asking, ...others] = await waitFor(() => {
        if (confirmations().length === 0) throw new Error("test: nothing asks");
        return confirmations();
      });
      expect(others).toEqual([]);
      if (asking === undefined) throw new Error("test: nothing asks");
      expect(screen.queryAllByLabelText("Current password")).toHaveLength(0);
      expect(within(asking).getByRole("heading").textContent).toBe(`Move this account to ${targetHandle}?`);
      expect(buttons(asking).at(-1)).toBe("Move account");

      await app.user.click(within(asking).getByRole("button", { name: "Move account" }));

      await sectionRead(`This account moved to ${targetHandle}`);
      expect(moved.seen.map((s) => s.body)).toEqual([
        { target_account_id: app.fake.accountId(target), external_confirmation: fakeExternalConfirmation },
      ]);
    });

    test("a confirmation the server no longer takes falls back to the confirmation with a password, in the server's words", async () => {
      const { app } = await returned();
      app.fake.script("POST /accounts/move", {
        times: 1,
        fail: {
          type: "/api/v1/problems/external-confirmation-expired",
          title: "The confirmation expired",
          status: 422,
        },
      });
      await waitFor(() => {
        if (targetField().value !== target) throw new Error("test: the field is empty");
      });
      await app.user.click(moveButton());
      const asking = await waitFor(() => {
        const [one] = confirmations();
        if (one === undefined) throw new Error("test: nothing asks");
        return one;
      });

      await app.user.click(within(asking).getByRole("button", { name: "Move account" }));

      const dialog = await waitFor(() => {
        const [one] = screen.queryAllByRole("dialog");
        if (one === undefined || within(one).queryAllByLabelText("Current password").length === 0)
          throw new Error("test: no confirmation with a password");
        return one;
      });
      expect(words(within(dialog).getByRole("alert"))).toBe("The confirmation expired");
    });
  });

  describe("of an account that moved", () => {
    const moved = { settings: { movedTo: target } } satisfies FakeOptions;

    test("Cancel the move asks first, and Keep the move sends nothing and gives the focus back", async () => {
      const app = await renderApp(moveAt, moved);
      const cancelling = await sectionRead("Cancelling the move");
      const cancelled = app.fake.script("DELETE /accounts/move");
      const button = within(cancelling).getByRole("button", { name: "Cancel the move" });

      await app.user.click(button);

      const asking = await dialogTitled(`Cancel the move to ${targetHandle}?`);
      expect(words(asking)).toContain(
        "Followers who already moved do not come back, and this account still cannot move again until 30 days after this move.",
      );
      expect(buttons(asking).slice(-2)).toEqual(["Keep the move", "Cancel the move"]);

      await app.user.click(within(asking).getByRole("button", { name: "Keep the move" }));

      await closed();
      await focusOn(() => within(section("Cancelling the move")).getByRole("button", { name: "Cancel the move" }));
      await aTurn();
      expect(cancelled.seen).toHaveLength(0);
    });

    test("confirmed, the move is cancelled, and the account read again says from when it can move again", async () => {
      const app = await renderApp(moveAt, moved);
      await sectionRead("Cancelling the move");
      const cancelled = app.fake.script("DELETE /accounts/move");

      await app.user.click(within(section("Cancelling the move")).getByRole("button", { name: "Cancel the move" }));
      await app.user.click(
        within(await dialogTitled(`Cancel the move to ${targetHandle}?`))
          .getAllByRole("button", { name: "Cancel the move" })
          .at(-1) ?? document.body,
      );

      await sectionRead("The move is cancelled");
      const later = await sectionRead("This account cannot move now");
      expect(cancelled.seen).toHaveLength(1);
      expect(sections().at(-1)).toBe("This account cannot move now");
      const wait = await moveWait(app.fake);
      expect(wait.last).toBeDefined();
      expect(spaced(words(later))).toMatch(laterAbout(wait));
      expect(main().queryAllByRole("textbox")).toHaveLength(0);
      expect(main().queryAllByRole("button", { name: "Move account" })).toHaveLength(0);
    });
  });
});

/** How many dialogs of either kind are open. */
const dialogsOpen = () => screen.queryAllByRole("dialog").length + confirmations().length;

// ---- Deactivate account ---------------------------------------------------------------------------------------------

describe("Deactivate account", () => {
  test("says until when signing in again brings the account back, from what the server says of it", async () => {
    const on = fakeApp();
    on.fake.script("GET /accounts/deactivation", {
      change: (body) => ({ ...body, deletion_due_at: "2031-03-09T12:00:00.000Z" }),
    });
    await renderApp(deactivateAt, { on });

    const back = await sectionRead("Coming back");
    expect(words(back)).toMatch(/^Coming back\s*Sign in by Mar 9, 2031, \d{1,2}:\d{2}\s[AP]M to reactivate it /);
    expect(words(section("Deactivate this account"))).toMatch(/the account is deleted on Mar 9, 2031, /);
  });

  // Each refusal the server says beforehand, and the way it leads to: to the move's screen, to Account status, or none.
  const refusals: readonly [string, FakeOptions, string, [string, string] | undefined][] = [
    [
      "moved",
      { settings: { movedTo: target } },
      "This account moved to another account. Cancel the move first, under “Move account”.",
      ["Cancel the move", moveAt],
    ],
    [
      "login_restricted",
      { settings: { deactivationRefusal: "login_restricted" } },
      "A moderation action keeps this account from signing in. “Account status” says which, and how to appeal it.",
      ["Account status", "/settings/account/status"],
    ],
    [
      "deletion_scheduled",
      { settings: { deactivationRefusal: "deletion_scheduled" } },
      "This account is already waiting to be deleted. “Account status” says why.",
      ["Account status", "/settings/account/status"],
    ],
    [
      "last_administrator",
      { settings: { deactivationRefusal: "last_administrator" } },
      "This is the only administrator account that can sign in, and a deactivated account is deleted in the end. Make someone else an administrator first.",
      undefined,
    ],
    [
      "paid_subscription",
      { settings: { deactivationRefusal: "paid_subscription" } },
      "Your supporter plan is still running. Cancel it first, then deactivate the account.",
      undefined,
    ],
  ];
  for (const [reason, options, says, door] of refusals)
    test(`refused beforehand (${reason}), says why in the act's place, ${door === undefined ? "with no door" : `leads to ${door[0]}`}, and offers no act`, async () => {
      await renderApp(deactivateAt, options);

      const summary = await sectionRead("This account cannot be deactivated now");
      expect(words(summary)).toContain(says);
      expect(
        within(summary)
          .queryAllByRole("link")
          .map((link) => [words(link), link.getAttribute("href")]),
      ).toEqual(door === undefined ? [] : [door]);
      expect(main().queryAllByRole("button", { name: "Deactivate account" })).toHaveLength(0);
      expect(sections()).not.toContain("Deactivate this account");
    });

  test("asks in the confirmation the act names, and sends the deactivation once confirmed", async () => {
    const app = await renderApp(deactivateAt);
    await sectionRead("Deactivate this account");
    const deactivated = app.fake.script("POST /accounts/deactivation");

    await app.user.click(main().getByRole("button", { name: "Deactivate account" }));

    const dialog = await dialogTitled("Deactivate this account?");
    expect(words(dialog)).toMatch(
      /You are signed out, and the account is deleted on .+ unless you sign in again before then\./,
    );
    expect(buttons(dialog).at(-1)).toBe("Deactivate account");

    await confirmWith(app, dialog, "Deactivate account");

    await sentTimes(deactivated);
    expect(deactivated.seen.map((s) => s.body)).toEqual([{ password }]);
  });

  test("refused once confirmed, as the server learned meanwhile, reads what deactivating would do again and says why", async () => {
    const on = fakeApp();
    const app = await renderApp(deactivateAt, { on });
    await sectionRead("Deactivate this account");
    const reread = on.fake.script("GET /accounts/deactivation", {
      change: (body) => ({ ...body, refusal: "paid_subscription" as const }),
    });
    on.fake.script("POST /accounts/deactivation", {
      fail: {
        type: "/api/v1/problems/deactivation-refused",
        title: "This account cannot be deactivated now.",
        status: 409,
        detail: "Cancel your supporter plan first.",
      },
    });

    await app.user.click(main().getByRole("button", { name: "Deactivate account" }));
    await confirmWith(app, await dialogTitled("Deactivate this account?"), "Deactivate account");

    const summary = await sectionRead("This account cannot be deactivated now");
    await closed();
    expect(reread.seen.length).toBeGreaterThan(0);
    expect(words(summary)).toContain(
      "Your supporter plan is still running. Cancel it first, then deactivate the account.",
    );
    expect(main().queryAllByRole("button", { name: "Deactivate account" })).toHaveLength(0);
  });
});

// ---- Delete account -------------------------------------------------------------------------------------------------

describe("Delete account", () => {
  test("asks in the confirmation the act names, and sends the deletion once confirmed", async () => {
    const app = await renderApp(deleteAt);
    await sectionRead("Delete this account");
    const deleted = app.fake.script("POST /accounts/delete");

    await app.user.click(main().getByRole("button", { name: "Delete account" }));

    const dialog = await dialogTitled("Delete this account?");
    expect(words(dialog)).toContain(
      "The account and its posts are deleted at once, and you are signed out. This cannot be undone.",
    );
    expect(buttons(dialog).at(-1)).toBe("Delete account");

    await confirmWith(app, dialog, "Delete account");

    await sentTimes(deleted);
    expect(deleted.seen.map((s) => s.body)).toEqual([{ password }]);
  });

  test("the only administrator is refused in the act's place as the screen opens, with no act", async () => {
    await renderApp(deleteAt, { settings: { deactivationRefusal: "last_administrator" } });

    const summary = await sectionRead("This account cannot be deleted now");
    expect(words(summary)).toContain(
      "This is the only administrator account that can sign in. Make someone else an administrator first, then delete it.",
    );
    expect(main().queryAllByRole("button", { name: "Delete account" })).toHaveLength(0);
  });

  test("the only administrator whom the server refuses once confirmed sees the refusal take the act's place", async () => {
    const app = await renderApp(deleteAt, { settings: { deletionRefusal: "last_administrator" } });
    await sectionRead("Delete this account");

    await app.user.click(main().getByRole("button", { name: "Delete account" }));
    await confirmWith(app, await dialogTitled("Delete this account?"), "Delete account");

    await sectionRead("This account cannot be deleted now");
    await closed();
    expect(sections()).not.toContain("Delete this account");
    expect(main().queryAllByRole("button", { name: "Delete account" })).toHaveLength(0);
  });
});

// ---- Every screen ---------------------------------------------------------------------------------------------------

for (const at of [moveAt, deactivateAt, deleteAt])
  test(`a reader without a session is asked to sign in at ${at}`, async () => {
    await renderApp(at, { visitor: true });

    expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
    expect(main().queryAllByRole("button")).toHaveLength(0);
  });

// The screens that read the account itself, which the shell read already, say a failed read on the feature's test.
test("Deactivate account whose read of what deactivating would do failed says so, and Try again reads it", async () => {
  const on = fakeApp();
  const app = await renderApp("/settings/account", { on });
  const failing = on.fake.script("GET /accounts/deactivation", { fail: 500 });
  await app.navigate(deactivateAt);

  await main().findByText("Your account could not be loaded.");
  failing.end();
  await app.user.click(main().getByRole("button", { name: "Try again" }));

  await sectionRead("Deactivate this account");
  expect(main().queryAllByText("Your account could not be loaded.")).toHaveLength(0);
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
describe("the screens of leaving the account, as the app draws them, keep the rules of a settings screen (ADR-0110 §6)", () => {
  const drawn: readonly [string, string, FakeOptions, string][] = [
    ["Move account", moveAt, {}, "Move this account"],
    ["Move account, moved", moveAt, { settings: { movedTo: target } }, "Cancelling the move"],
    ["Deactivate account", deactivateAt, {}, "Deactivate this account"],
    [
      "Deactivate account, refused",
      deactivateAt,
      { settings: { deactivationRefusal: "login_restricted" } },
      "This account cannot be deactivated now",
    ],
    ["Delete account", deleteAt, {}, "Delete this account"],
    [
      "Delete account, refused",
      deleteAt,
      { settings: { deactivationRefusal: "last_administrator" } },
      "This account cannot be deleted now",
    ],
  ];
  for (const [what, at, options, read] of drawn)
    test(what, async () => {
      await renderApp(at, options);
      await sectionRead(read);

      expect(settingsFindings(document.body)).toEqual([]);
    });

  test("Move account once a move was refused under the field", async () => {
    const app = await renderApp(moveAt);
    await sectionRead("Move this account");

    await moving(app, "");
    await refusedWith("Enter the handle or the address of the account you are moving to.");

    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("Move account once the account moved", async () => {
    const app = await renderApp(moveAt);
    await sectionRead("Move this account");
    await moving(app, target);
    await confirmWith(app, await dialogTitled(`Move this account to ${targetHandle}?`), "Move account");
    await sectionRead(`This account moved to ${targetHandle}`);
    await closed();

    expect(settingsFindings(document.body)).toEqual([]);
  });
});
