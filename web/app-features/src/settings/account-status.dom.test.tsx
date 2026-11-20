// SPDX-License-Identifier: AGPL-3.0-only

// The account's status in the settings (#3204, FR-715, ADR-0058 §9, §10). In the entry Account and data: while a
// moderation action is in effect, a summary at the top says so, with what Account status holds and the way to it;
// while none is, a section with one row says so and leads to the same page; never both. Account status: the actions in
// effect and the past ones, each a row that leads to its own screen and says what the action does in the server's
// words, the moderators' reason, when it ends or ended, and where its appeal stands. And the count the settings' menu
// shows on the entry: the actions in effect the reader can still appeal. The entry holds other parts (export, import,
// automatic deletion, aliases): a test here finds the status' part by its name and says nothing of the rest. One
// action and its appeal is account-status-action.dom.test.tsx; the way from one screen to the next and the menu's
// count through an appeal are the app's (web/app/src/settings-account-status.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { within } from "@testing-library/react";
import {
  acted,
  actionId,
  alerts,
  answered,
  drawnEntry,
  drawnPart,
  drawnStatus,
  interactionUpheld,
  limitWaiting,
  linesOf,
  main,
  moderatorsOnly,
  noPosting,
  postsOverturned,
  said,
  screenWords,
  section,
  sectionsTitled,
  sensitivePassed,
  serverOf,
  warning,
  warnings,
  withTheModerators,
  wordsOf,
  type Seed,
} from "../test/account-status";
import { SettingsIndex } from "./menu";

const statusPath = "/settings/account/status";
const one = "A moderation action is in effect on your account";
const loading = "Loading your account status…";
const failed = "Your account status could not be loaded.";

/** The ways to Account status the entry shows, by their names. */
const doors = () =>
  main()
    .queryAllByRole("link")
    .filter((link) => link.getAttribute("href") === statusPath)
    .map(wordsOf);

/**
 * The sections of the entry that are the status' part, by their titles: the summary of the actions in effect, and
 * Account status. The entry's other sections are not this test's.
 */
const statusParts = () =>
  sectionsTitled("").filter(
    (title) => title === "Account status" || /moderation actions? (?:is|are) in effect on your account$/.test(title),
  );

/** Actions in effect and past that the reader may still appeal with no last day, and after its end. */
const { appeal: _waiting, appeal_deadline: _day, ...limit } = limitWaiting;
const openEnded: Seed = { ...limit, appealable: true };
const { appeal: _decided, ...ended } = interactionUpheld;
const endedOpen: Seed = { ...ended, appealable: true, appeal_deadline: "2031-03-09T12:00:00.000Z" };

describe("the entry's status part, while an action is in effect", () => {
  test("one: a summary says so, with what Account status holds and until when the reader can appeal, and leads to it", async () => {
    const on = await drawnEntry(acted(noPosting, postsOverturned));
    await answered(on);

    expect(linesOf(section(one))).toEqual([
      "“Account status” says what each action does, why, and when it ends. You can appeal until Mar 9, 2031.",
    ]);
    const way = within(section(one)).getByRole("link", { name: "See account status" });
    expect(way.getAttribute("href")).toBe(statusPath);
    // The summary alone: no row says that nothing is in effect.
    expect(statusParts()).toEqual([one]);
    expect(doors()).toEqual(["See account status"]);
  });

  test("several: the summary counts them, and the day it names is the nearest an action can be appealed until", async () => {
    const sooner: Seed = { ...noPosting, kind: "restrict_interaction", appeal_deadline: "2030-01-05T12:00:00.000Z" };
    const on = await drawnEntry(acted(noPosting, sooner, limitWaiting, interactionUpheld));
    await answered(on);

    const several = "3 moderation actions are in effect on your account";
    expect(linesOf(section(several))).toEqual([
      "“Account status” says what each action does, why, and when it ends. You can appeal until Jan 5, 2030.",
    ]);
    expect(statusParts()).toEqual([several]);
    expect(doors()).toEqual(["See account status"]);
  });

  // What the summary's line ends with follows what the reader can still do.
  const lines: readonly [string, Seed, string][] = [
    [
      "an action that can be appealed with no last day",
      openEnded,
      "“Account status” says what each action does, why, and when it ends. You can appeal an action once.",
    ],
    [
      "an action whose appeal waits",
      limitWaiting,
      "“Account status” says what each action does, why, when it ends, and where your appeal stands.",
    ],
    [
      "an action whose time to appeal has passed",
      sensitivePassed,
      "“Account status” says what each action does, why, when it ends, and where your appeal stands.",
    ],
  ];
  for (const [when, action, line] of lines)
    test(`${when}: the summary names no day`, async () => {
      const on = await drawnEntry(acted(action));
      await answered(on);

      expect(linesOf(section(one))).toEqual([line]);
    });

  test("in Japanese the summary and its way read in it", async () => {
    const on = await drawnEntry(acted(noPosting), { locale: "ja" });
    await answered(on);

    const summary = section("このアカウントに適用中のモデレーションの処分が 1 件あります");
    expect(within(summary).getByRole("link", { name: "アカウントの状態を確認する" }).getAttribute("href")).toBe(
      statusPath,
    );
  });
});

describe("the entry's status part, while no action is in effect", () => {
  const none: readonly [string, FakeOptions][] = [
    ["an account never acted on", acted()],
    ["an account whose actions all ended, one of them appealed", acted(interactionUpheld, postsOverturned, warning)],
  ];
  for (const [who, options] of none)
    test(`${who}: the section Account status holds one row, which says so and leads to Account status`, async () => {
      const on = await drawnEntry(options);
      await answered(on);

      const [row, ...others] = within(section("Account status")).getAllByRole("link");
      expect(others).toHaveLength(0);
      expect(row?.getAttribute("href")).toBe(statusPath);
      expect(wordsOf(row ?? document.body)).toMatch(
        /^Moderation actions and appeals\s*No moderation action is in effect on your account\.$/,
      );
      // The row alone: no summary of actions in effect.
      expect(statusParts()).toEqual(["Account status"]);
      expect(doors()).toHaveLength(1);
      expect(main().queryAllByRole("link", { name: "See account status" })).toHaveLength(0);
    });
});

describe("the entry's status part, before the actions are read", () => {
  test("while they are on their way the section says so, with neither the summary nor the row, and then shows what came", async () => {
    const server = serverOf(acted(noPosting));
    const held = server.fake.script("GET /moderation/actions", { hold: "request" });
    const on = await drawnEntry(server);

    expect(await within(section("Account status")).findAllByText(loading)).toHaveLength(1);
    expect(doors()).toEqual([]);
    expect(sectionsTitled(one)).toEqual([]);

    held.release();

    await answered(on);
    expect(statusParts()).toEqual([one]);
    expect(main().queryAllByText(loading)).toHaveLength(0);
  });

  test("when they could not be read the section says so, and Try again reads them", async () => {
    const server = serverOf(acted(noPosting));
    const failing = server.fake.script("GET /moderation/actions", { fail: 500 });
    const on = await drawnEntry(server);

    const part = within(section("Account status"));
    expect(await part.findAllByText(failed)).toHaveLength(1);
    expect(doors()).toEqual([]);
    expect(sectionsTitled(one)).toEqual([]);
    failing.end();
    const before = on.requests.length;

    await on.user.click(part.getByRole("button", { name: "Try again" }));

    await answered(on, before);
    expect(statusParts()).toEqual([one]);
    expect(main().queryAllByText(failed)).toHaveLength(0);
  });
});

/** The rows of the list named `name` on Account status: where each leads, and what it says. */
const rows = (name: string) =>
  within(main().getByRole("list", { name }))
    .getAllByRole("link")
    .map((row) => [row.getAttribute("href"), wordsOf(row)] as const);

/** The words of a row, part by part, as a pattern: a row's parts stand on lines of their own, which no simulation draws. */
const saying = (...parts: string[]) =>
  new RegExp(`^${parts.map((part) => part.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("\\s*")}$`);

/** Holds the list named `name` to `expected`: where each row leads, in their order, and what each says. */
function expectRows(name: string, expected: readonly (readonly [string, RegExp])[]): void {
  const found = rows(name);
  expect(found.map(([href]) => href)).toEqual(expected.map(([href]) => href));
  for (const [at, [, says]] of expected.entries()) expect(found[at]?.[1]).toMatch(says);
}

const inEffect = "Actions in effect";
const past = "Past actions";

/** What the heading of the section titled `name` says: its name, then how many it holds once that is known. */
const titled = (name: string) => wordsOf(within(section(name)).getAllByRole("heading")[0] ?? section(name));

describe("Account status", () => {
  test("lists the actions in effect and the past ones apart, each a row to its own screen that says what the action does, why, when it ends, and where its appeal stands", async () => {
    const on = await drawnStatus(
      acted(noPosting, limitWaiting, sensitivePassed, interactionUpheld, postsOverturned, warning),
    );
    await answered(on);

    expect(titled(inEffect)).toMatch(/^Actions in effect\s*3$/);
    expectRows(inEffect, [
      [
        on.fake.actionPath(0),
        saying(
          "No posting, replying, boosting, or editing",
          "The same link was posted to forty hashtags within an hour.",
          "Ends on May 20, 2031",
          "You can appeal until Mar 9, 2031",
        ),
      ],
      [
        on.fake.actionPath(1),
        saying(
          "Limited: posts shown to followers only and kept off the public timelines",
          "Repeated replies that took other people's threads off their subject.",
          "Lasts until a moderator lifts it",
          "Appeal under review",
        ),
      ],
      [
        on.fake.actionPath(2),
        saying(
          "Media always marked as sensitive",
          "Photos of injuries were posted without a content warning several times.",
          "Lasts until a moderator lifts it",
          "The time to appeal passed on Jun 1, 2024",
        ),
      ],
    ]);
    expect(titled(past)).toMatch(/^Past actions\s*3$/);
    expectRows(past, [
      [
        on.fake.actionPath(3),
        saying(
          "No following, favouriting, boosting, or replying",
          "Following and unfollowing hundreds of accounts a day.",
          "Ended on Feb 15, 2024",
          "Appeal reviewed: action kept",
        ),
      ],
      [
        on.fake.actionPath(4),
        saying(
          "Posts marked as sensitive",
          "Photos of a medical procedure without a content warning.",
          "Lifted on Jan 18, 2024",
          "Appeal reviewed: action lifted",
        ),
      ],
      // A past action never appealed, whose time to appeal passed: nothing of an appeal, and no reason was given.
      [on.fake.actionPath(5), saying("Warning", "Ended on Mar 2, 2023")],
    ]);
    expect(on.fake.actionPath(0)).toBe(`${statusPath}/${actionId(on, 0)}`);
  });

  test("a row's title is the server's own words for the action, whatever they are", async () => {
    const on = await drawnStatus(acted({ ...noPosting, kind_label: "Posting is paused" }));
    await answered(on);

    expectRows(inEffect, [
      [
        on.fake.actionPath(0),
        saying(
          "Posting is paused",
          "The same link was posted to forty hashtags within an hour.",
          "Ends on May 20, 2031",
          "You can appeal until Mar 9, 2031",
        ),
      ],
    ]);
  });

  test("shows nothing a moderator keeps to the moderators, should a server ever send it", async () => {
    const server = serverOf(acted(noPosting, interactionUpheld));
    const leaking = server.fake.script("GET /moderation/actions", { change: withTheModerators });
    const on = await drawnStatus(server);
    await answered(on);

    expect(leaking.seen.length).toBeGreaterThan(0);
    expect(rows(inEffect)).toHaveLength(1);
    expect(rows(past)).toHaveLength(1);
    expect(screenWords()).not.toMatch(moderatorsOnly);
  });

  test("an account never acted on: each list says what would stand there, and counts nothing", async () => {
    const on = await drawnStatus(acted());
    await answered(on);

    expect(linesOf(section(inEffect))).toEqual([
      "No moderation action is in effect on your account. If a moderator acts on your account, the action appears here with its reason, and you can appeal it.",
    ]);
    expect(linesOf(section(past))).toEqual([
      "No past actions. An action that ended or was lifted stays listed here, with your appeal and its result.",
    ]);
    expect([titled(inEffect), titled(past)]).toEqual([inEffect, past]);
    expect(main().queryAllByRole("list")).toHaveLength(0);
  });

  test("more past actions than a page: the first page of them with Show more, which brings the rest and then is gone", async () => {
    const on = await drawnStatus(acted(...warnings(24)));
    await answered(on);

    expect(rows(past)).toHaveLength(20);
    // How many there are is not known yet.
    expect(titled(past)).toBe(past);
    const before = on.requests.length;

    await on.user.click(within(section(past)).getByRole("button", { name: "Show more" }));

    await answered(on, before);
    expectRows(
      past,
      Array.from({ length: 24 }, (_, i) => [
        on.fake.actionPath(i),
        saying("Warning", `Warning number ${String(i + 1)}.`, "Ended on Mar 2, 2023"),
      ]),
    );
    expect(titled(past)).toMatch(/^Past actions\s*24$/);
    expect(within(section(past)).queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
  });

  test("a Show more that fails keeps the rows, says so, and its button, named Try again, brings the rest", async () => {
    const on = await drawnStatus(acted(...warnings(24)));
    await answered(on);
    const failing = on.fake.script("GET /moderation/actions", { fail: 500 });
    const before = on.requests.length;

    await on.user.click(within(section(past)).getByRole("button", { name: "Show more" }));

    await answered(on, before);
    expect(failing.seen.length).toBeGreaterThan(0);
    expect(rows(past)).toHaveLength(20);
    expect(within(section(past)).getByRole("alert").textContent).toBe("The rest of the list could not be loaded.");
    const again = within(section(past)).getByRole("button", { name: "Try again" });
    expect(within(section(past)).queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
    // The whole screen did not give way to the failure.
    expect(alerts().some((words) => words.startsWith(failed))).toBe(false);
    failing.end();
    const retried = on.requests.length;

    await on.user.click(again);

    await answered(on, retried);
    expect(rows(past)).toHaveLength(24);
    expect(within(section(past)).queryAllByRole("button", { name: /^(Show more|Try again)$/ })).toHaveLength(0);
    expect(within(section(past)).queryAllByText("The rest of the list could not be loaded.")).toHaveLength(0);
  });

  test("while the actions are on their way the screen says so in a status, and then lists what came", async () => {
    const server = serverOf(acted(noPosting));
    const held = server.fake.script("GET /moderation/actions", { hold: "request" });
    const on = await drawnStatus(server);

    expect(said()).toEqual([loading]);
    expect(main().queryAllByRole("region")).toHaveLength(0);

    held.release();

    await answered(on);
    expect(rows(inEffect)).toHaveLength(1);
    expect(said()).toEqual([]);
  });

  test("when the actions could not be read the screen says so in an alert, and Try again reads them", async () => {
    const server = serverOf(acted(noPosting));
    const failing = server.fake.script("GET /moderation/actions", { fail: 500 });
    const on = await drawnStatus(server);

    const again = await main().findByRole("button", { name: "Try again" });
    expect(alerts()).toHaveLength(1);
    expect(alerts()[0]).toMatch(/^Your account status could not be loaded\.\s*Try again$/);
    expect(main().queryAllByRole("region")).toHaveLength(0);
    failing.end();
    const before = on.requests.length;

    await on.user.click(again);

    await answered(on, before);
    expect(rows(inEffect)).toHaveLength(1);
    expect(alerts()).toEqual([]);
  });

  test("the way back, named after the entry, is the owner's to follow", async () => {
    const on = await drawnStatus(acted(noPosting));
    await answered(on);

    await on.user.click(main().getByRole("button", { name: "Account and data" }));

    expect(on.told).toEqual(["back"]);
  });

  // The action's own words are the server's, in the language the app asks it for (the app's test).
  test("in Japanese the lists and a row's lines read in it", async () => {
    const on = await drawnStatus(acted(noPosting), { locale: "ja" });
    await answered(on);

    expect(titled("適用中の処分")).toMatch(/^適用中の処分\s*1$/);
    expect(sectionsTitled("過去の処分")).toEqual(["過去の処分"]);
    const [row] = within(main().getByRole("list", { name: "適用中の処分" })).getAllByRole("link");
    expect(wordsOf(row ?? document.body)).toMatch(
      /2031\/05\/20 に終わります\s*2031\/03\/09 まで異議を申し立てられます$/,
    );
  });
});

// The entry's row in the settings' menu counts what waits for the reader there: the actions in effect that they can
// still appeal. That the default server's one shows as "1 item to check" is the menu's own test
// (web/app/src/settings.dom.test.tsx).
describe("the count on Account and data in the settings' menu", () => {
  /** The counts the entry's row shows, as assistive technology reads them. */
  const counts = () =>
    within(main().getByRole("link", { name: /^Account and data/ }))
      .queryAllByText(/^\d+ items? to check$/)
      .map(wordsOf);

  const cases: readonly [string, FakeOptions, string[]][] = [
    ["an action in effect that can be appealed", acted(noPosting, warning), ["1 item to check"]],
    [
      "two actions in effect that can be appealed, and one whose appeal waits",
      acted(noPosting, { ...noPosting, kind: "restrict_interaction" }, limitWaiting),
      ["2 items to check"],
    ],
    ["no action at all", acted(), []],
    ["an action in effect already appealed", acted(limitWaiting), []],
    ["an action in effect whose time to appeal has passed", acted(sensitivePassed), []],
    ["only a past action that could still be appealed", acted(endedOpen), []],
  ];
  for (const [when, options, shown] of cases)
    test(`${when}: ${shown[0] ?? "none"}`, async () => {
      const on = await drawnPart(options, () => <SettingsIndex signOutHref="/settings?sign_out=true" />);
      await answered(on);

      expect(counts()).toEqual(shown);
    });
});
