// SPDX-License-Identifier: AGPL-3.0-only

// The account's status in the settings (#3204), as the app places it: the entry Account and data at /settings/account,
// a row of the menu's Account group; Account status at /settings/account/status, which the entry leads to; and one
// moderation action with its appeal at /settings/account/status/<id>, which a row of Account status leads to. Each is
// a screen of the same entry, with the menu beside it and a way back to the screen before. The count the menu shows on
// the entry goes once the reader's appeal is sent, or refused because its time has passed; the app holds a navigation
// away from an appeal written and not sent until the reader chose to leave or to keep writing; and the three screens
// keep the rules of a settings screen. What each screen shows and says, and what the form sends, is the feature's
// (app-features, settings/account-status.dom.test.tsx and account-status-action.dom.test.tsx); the menu itself and
// its count on the default server are settings.dom.test.tsx. The entry holds other parts than the account's status:
// a test here finds the status' part by its name.

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp, type FakeApp } from "./test/server";
import { rail } from "./test/viewport";

const entry = "/settings/account";
const status = "/settings/account/status";

type Seed = NonNullable<NonNullable<FakeOptions["settings"]>["moderation"]>[number];

// The moments of the seeds are dates far enough from today that what is to come stays to come.

/** An action that keeps the account from posting, in effect, which can be appealed until March 9, 2031. */
const noPosting: Seed = {
  kind: "restrict_posting",
  reason_category: "spam",
  public_reason: "The same link was posted to forty hashtags within an hour.",
  state: "active",
  issued_at: "2024-11-02T12:00:00.000Z",
  expires_at: "2031-05-20T12:00:00.000Z",
  appealable: true,
  appeal_deadline: "2031-03-09T12:00:00.000Z",
};

/** A limit in effect, whose appeal waits for the moderators. */
const limitWaiting: Seed = {
  kind: "limit",
  reason_category: "violation",
  public_reason: "Repeated replies that took other people's threads off their subject.",
  state: "active",
  issued_at: "2024-10-20T12:00:00.000Z",
  appealable: false,
  appeal_deadline: "2031-04-18T12:00:00.000Z",
  appeal: {
    id: "waiting",
    text: "I replied to three threads that day, each on its subject.",
    state: "pending",
    submitted_at: "2024-10-21T12:00:00.000Z",
  },
};

/** An action on two of the reader's posts and one that is gone, lifted when its appeal was decided. */
const postsOverturned: Seed = {
  kind: "mark_statuses_sensitive",
  reason_category: "other",
  public_reason: "Photos of a medical procedure without a content warning.",
  ownPosts: 2,
  status_ids: ["01HZZZZZZZZZZZZZZZZZZZZZZZ"],
  state: "lifted",
  issued_at: "2024-01-10T12:00:00.000Z",
  lifted_at: "2024-01-18T12:00:00.000Z",
  appealable: false,
  appeal: {
    id: "overturned",
    text: "The photos are of my own recovery and I added a warning to each.",
    state: "overturned",
    submitted_at: "2024-01-12T12:00:00.000Z",
    decided_at: "2024-01-18T12:00:00.000Z",
    decision_reason: "On a second look the posts carried the warning. The action is lifted.",
  },
};

/** A warning that ended, never appealed, whose time to appeal has passed. */
const warning: Seed = {
  kind: "warn",
  reason_category: "violation",
  state: "expired",
  issued_at: "2023-03-01T12:00:00.000Z",
  expires_at: "2023-03-02T12:00:00.000Z",
  appealable: false,
  appeal_deadline: "2023-08-28T12:00:00.000Z",
};

/** A server whose reader has the actions `moderation`, newest first. */
const acted = (...moderation: Seed[]) => ({ settings: { moderation } }) satisfies FakeOptions;

/** The title of the screen in place, as its heading says it. */
const heading = () =>
  main()
    .queryAllByRole("heading", { level: 1 })
    .map((title) => title.textContent)
    .join();

/** The settings' menus outside the screen: the one the frame places beside an entry. */
const beside = () =>
  screen.queryAllByRole("navigation", { name: "Settings" }).filter((menu) => !screen.getByRole("main").contains(menu));

/** The one menu beside the screen. */
function menu(): HTMLElement {
  const [one, ...others] = beside();
  if (one === undefined || others.length > 0)
    throw new Error(`test: ${String(beside().length)} settings menus stand beside the screen`);
  return one;
}

/** The row of the menu beside the screen that is the entry the reader is at, by its name without its count. */
const marked = () =>
  within(menu())
    .getAllByRole("link")
    .filter((link) => link.getAttribute("aria-current") === "page")
    .map((link) => link.textContent.replace(/\s*\d+\s*\d+ items? to check$/, ""));

/** The counts the menu beside the screen shows on Account and data, as assistive technology reads them. */
const counts = () =>
  within(within(menu()).getByRole("link", { name: /^Account and data/ }))
    .queryAllByText(/^\d+ items? to check$/)
    .map((count) => count.textContent);

/** Waits until the menu beside the screen counts one thing on Account and data: the actions are read. */
const counted = () => within(menu()).findByText("1 item to check");

/** Waits until the entry drew its summary of the actions in effect, and returns the way to Account status. */
const entryRead = () => main().findByRole("link", { name: "See account status" });
/** Waits until Account status drew the list named `name`, and returns its rows. */
const listRead = async (name = "Actions in effect") =>
  within(await main().findByRole("list", { name })).getAllByRole("link");
/** Waits until the screen of an action that can be appealed drew its form, and returns the appeal's field. */
const formRead = () => main().findByRole<HTMLTextAreaElement>("textbox", { name: "Your appeal" });
/** The appeal's field. */
const field = () => main().getByRole<HTMLTextAreaElement>("textbox", { name: "Your appeal" });

/** What the status regions of the screen say, the ones with nothing to say left out. */
const said = () =>
  main()
    .queryAllByRole("status")
    .map(words)
    .filter((line) => line !== "");

/** The app opened at the screen of the reader's newest action, on a server of `options`. */
async function atAction(options: FakeOptions = acted(noPosting, warning)): Promise<TestApp & { at: string }> {
  const on = fakeApp(options);
  const at = on.fake.actionPath(0);
  return { ...(await renderApp(at, { on })), at };
}

/** Writes `text` in the appeal's field, as one paste. */
async function write({ user }: TestApp, text: string): Promise<void> {
  await user.click(field());
  await user.paste(text);
}

/** Presses Send appeal and returns once a status of the screen says `says`. */
async function send({ user }: TestApp, says = "Your appeal was sent."): Promise<void> {
  await user.click(main().getByRole("button", { name: "Send appeal" }));
  await waitFor(() => {
    if (!said().includes(says)) throw new Error(`test: no status says "${says}" yet: ${JSON.stringify(said())}`);
  });
}

/** The confirmations open over the screen, by their titles. */
const confirmations = () =>
  screen.queryAllByRole("alertdialog").map((dialog) => within(dialog).getAllByRole("heading")[0]?.textContent);

/** Waits until the app asks whether to leave without sending the appeal, and returns the confirmation. */
const asked = () => screen.findByRole("alertdialog", { name: "Leave without sending your appeal?" });

/** Waits until no confirmation is open and the app is at `path`. */
const settledAt = (app: TestApp, path: string) =>
  waitFor(() => {
    if (confirmations().length > 0 || app.path() !== path)
      throw new Error(`test: the app is at ${app.path()}, with the confirmations ${JSON.stringify(confirmations())}`);
  });

describe("the entry Account and data", () => {
  test("is a row of the menu's Account group, and opens at its address with the menu beside it marking it", async () => {
    const app = await renderApp("/settings", acted(noPosting));
    const account = within(main().getByRole("navigation", { name: "Settings" })).getByRole("region", {
      name: "Account",
    });
    const row = within(account).getByRole("link", { name: /^Account and data/ });
    expect(row.getAttribute("href")).toBe(entry);

    await app.user.click(row);

    await entryRead();
    expect(app.path()).toBe(entry);
    expect(heading()).toBe("Account and data");
    expect(app.title()).toContain("Account and data");
    expect(marked()).toEqual(["Account and data"]);
  });

  test("leads to Account status from its summary of the actions in effect, and Account status' way back, named after the entry, leads to it", async () => {
    const app = await renderApp(entry, acted(noPosting));

    await app.user.click(await entryRead());

    expect(await listRead()).toHaveLength(1);
    expect(app.path()).toBe(status);
    expect(heading()).toBe("Account status");
    expect(app.title()).toContain("Account status");
    // A screen of the same entry: the menu beside it still marks Account and data.
    expect(marked()).toEqual(["Account and data"]);

    await app.user.click(main().getByRole("button", { name: "Account and data" }));

    await entryRead();
    expect(app.path()).toBe(entry);
    expect(heading()).toBe("Account and data");
  });

  test("leads to Account status from its row when no action is in effect", async () => {
    const app = await renderApp(entry, acted(warning));

    await app.user.click(await main().findByRole("link", { name: /^Moderation actions and appeals/ }));

    expect(await listRead("Past actions")).toHaveLength(1);
    expect(app.path()).toBe(status);
    expect(heading()).toBe("Account status");
  });
});

describe("one action", () => {
  test("opens from its row of Account status at an address of its own, and its way back, named Account status, leads to the list", async () => {
    const on = fakeApp(acted(noPosting, warning));
    const app = await renderApp(status, { on });
    const [row] = await listRead();
    if (row === undefined) throw new Error("test: no action is listed");
    expect(row.getAttribute("href")).toBe(on.fake.actionPath(0));

    await app.user.click(row);

    await formRead();
    expect(app.path()).toBe(on.fake.actionPath(0));
    expect(heading()).toBe("Moderation action");
    expect(app.title()).toContain("Moderation action");
    expect(marked()).toEqual(["Account and data"]);

    await app.user.click(main().getByRole("button", { name: "Account status" }));

    expect(await listRead()).toHaveLength(1);
    expect(app.path()).toBe(status);
    expect(confirmations()).toEqual([]);
  });

  test("opens at its address as the app's first screen, with the menu beside it marking Account and data", async () => {
    const app = await atAction();

    expect((await formRead()).value).toBe("");
    expect(heading()).toBe("Moderation action");
    expect(app.path()).toBe(app.at);
    expect(marked()).toEqual(["Account and data"]);
  });

  test("an address that names no action of the reader's says so under the screen's name, and leads to Account status", async () => {
    const app = await renderApp(`${status}/nothing`, acted(noPosting));

    const missing = await main().findByRole("alert");
    expect(words(missing)).toMatch(/^This moderation action is not listed on your account\. Check the address\./);
    expect(heading()).toBe("Moderation action");

    await app.user.click(within(missing).getByRole("link", { name: "Account status" }));

    expect(await listRead()).toHaveLength(1);
    expect(app.path()).toBe(status);
  });
});

for (const at of [entry, status, `${status}/anything`])
  test(`a reader without a session is asked to sign in at ${at}`, async () => {
    await renderApp(at, { visitor: true });

    expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
    expect(main().queryAllByRole("textbox")).toHaveLength(0);
    expect(main().queryAllByRole("link", { name: "See account status" })).toHaveLength(0);
  });

describe("the count on Account and data in the menu", () => {
  test("is there while the reader can appeal an action in effect, and gone once their appeal is sent; Account status then says the appeal is under review", async () => {
    const app = await atAction();
    await formRead();
    await counted();
    await write(app, "Please look again.");

    await send(app);

    await waitFor(() => {
      if (counts().length > 0) throw new Error("test: the menu still counts something on Account and data");
    });

    await app.user.click(main().getByRole("button", { name: "Account status" }));

    const [row] = await listRead();
    expect(words(row ?? document.body)).toMatch(/Appeal under review$/);
    expect(counts()).toEqual([]);
  });

  test("is gone once the server said that the time to appeal has passed", async () => {
    const app = await atAction({ settings: { moderation: [noPosting], appealSave: "closed" } });
    await formRead();
    await counted();
    await write(app, "Please look again.");

    await send(
      app,
      "The time to appeal this action has passed, so your appeal was not sent. To reach the moderators, see the contact on this server's About page.",
    );

    await waitFor(() => {
      if (counts().length > 0) throw new Error("test: the menu still counts something on Account and data");
    });
  });

  test("stays while an appeal could not be sent", async () => {
    const on = fakeApp(acted(noPosting));
    const app = await renderApp(on.fake.actionPath(0), { on });
    await formRead();
    await counted();
    const failing = on.fake.script("POST /moderation/actions/{id}/appeal", { fail: 500 });
    await write(app, "Please look again.");

    await app.user.click(main().getByRole("button", { name: "Send appeal" }));

    await main().findByText("The appeal could not be sent. Check your connection and try again.");
    await failing.arrived();
    expect(counts()).toEqual(["1 item to check"]);
  });
});

describe("leaving an appeal written and not sent", () => {
  /** The screen of an action with an appeal written and not sent. */
  async function written() {
    const app = await atAction();
    await formRead();
    await write(app, "Please look again.");
    return app;
  }

  // Each way out of the screen: what the reader presses, and where it leads once they chose to leave.
  const ways = [
    {
      way: "the screen's way back",
      leave: (app: TestApp) => app.user.click(main().getByRole("button", { name: "Account status" })),
      to: status,
    },
    {
      way: "a row of the menu beside it",
      leave: (app: TestApp) => app.user.click(within(menu()).getByRole("link", { name: "Display and posting" })),
      to: "/settings/display",
    },
    {
      way: "a link of the navigation",
      leave: (app: TestApp) => app.user.click(within(rail()).getByRole("link", { name: "Bookmarks" })),
      to: "/bookmarks",
    },
  ] as const;

  for (const { way, leave, to } of ways) {
    test(`${way} asks first, and Keep writing stays on the screen with what was written`, async () => {
      const app = await written();

      await leave(app);

      const asking = await asked();
      expect(app.path()).toBe(app.at);
      expect(
        within(asking)
          .getAllByRole("button")
          .map((button) => button.getAttribute("aria-label") ?? button.textContent),
      ).toEqual(["Close", "Keep writing", "Leave"]);

      await app.user.click(within(asking).getByRole("button", { name: "Keep writing" }));

      await settledAt(app, app.at);
      expect(heading()).toBe("Moderation action");
      expect(field().value).toBe("Please look again.");
    });

    test(`${way} asks first, and Leave goes there without sending`, async () => {
      const app = await written();
      const sent = app.fake.script("POST /moderation/actions/{id}/appeal");

      await leave(app);
      await app.user.click(within(await asked()).getByRole("button", { name: "Leave" }));

      await settledAt(app, to);
      expect(main().queryAllByRole("textbox", { name: "Your appeal" })).toHaveLength(0);
      expect(sent.seen).toHaveLength(0);
    });
  }

  test("what was written and left is gone: the screen opened again has an empty field", async () => {
    const app = await written();
    await app.user.click(main().getByRole("button", { name: "Account status" }));
    await app.user.click(within(await asked()).getByRole("button", { name: "Leave" }));
    await settledAt(app, status);
    const [row] = await listRead();
    if (row === undefined) throw new Error("test: no action is listed");

    await app.user.click(row);

    expect((await formRead()).value).toBe("");
  });

  test("a screen with nothing written is left without a question", async () => {
    const app = await atAction();
    await formRead();

    await app.user.click(main().getByRole("button", { name: "Account status" }));

    await listRead();
    expect(app.path()).toBe(status);
    expect(confirmations()).toEqual([]);
  });

  test("a screen whose appeal was sent is left without a question", async () => {
    const app = await written();
    await send(app);

    await app.user.click(main().getByRole("button", { name: "Account status" }));

    await listRead();
    expect(app.path()).toBe(status);
    expect(confirmations()).toEqual([]);
  });
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
describe("the screens of the account's status, as the app draws them, keep the rules of a settings screen (ADR-0110 §6)", () => {
  const actionAt = ({ fake }: FakeApp) => fake.actionPath(0);
  const actionRead = () => main().findByRole("region", { name: "What this action does" });
  // The screen, the server it is drawn on, its address there, and what shows that it is drawn.
  const drawn: readonly [string, FakeOptions, (on: FakeApp) => string, () => Promise<unknown>][] = [
    ["the entry with an action in effect", acted(noPosting, warning), () => entry, entryRead],
    ["the entry with several", acted(noPosting, limitWaiting, postsOverturned), () => entry, entryRead],
    [
      "the entry with none",
      acted(),
      () => entry,
      () => main().findByRole("link", { name: /^Moderation actions and appeals/ }),
    ],
    ["Account status", acted(noPosting, limitWaiting, postsOverturned, warning), () => status, () => listRead()],
    [
      "Account status of an account never acted on",
      acted(),
      () => status,
      () => main().findByText(/^No past actions\./),
    ],
    [
      "Account status with more past actions than a page",
      acted(...Array.from({ length: 24 }, () => warning)),
      () => status,
      () => main().findByRole("button", { name: "Show more" }),
    ],
    ["an action that can be appealed", acted(noPosting), actionAt, formRead],
    ["an action whose appeal waits", acted(limitWaiting), actionAt, actionRead],
    [
      "an action on posts, with its appeal decided and the moderators' answer",
      acted(postsOverturned),
      actionAt,
      () => main().findByRole("list", { name: "Posts this action is about" }),
    ],
    ["an action whose time to appeal has passed", acted(warning), actionAt, actionRead],
    ["an address that names no action", acted(noPosting), () => `${status}/nothing`, () => main().findByRole("alert")],
  ];

  for (const [what, options, at, read] of drawn)
    test(what, async () => {
      const on = fakeApp(options);
      await renderApp(at(on), { on });
      await read();

      // The menu stands beside the screen.
      expect(beside()).toHaveLength(1);
      expect(settingsFindings(document.body)).toEqual([]);
    });

  test("an action's form once Send appeal found nothing to send", async () => {
    const app = await atAction();
    await formRead();

    await app.user.click(main().getByRole("button", { name: "Send appeal" }));

    await main().findByText("Write your appeal before you send it.");
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("an action once its appeal was sent", async () => {
    const app = await atAction();
    await formRead();
    await write(app, "Please look again.");

    await send(app);

    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("an action once the server refused its appeal for good", async () => {
    const app = await atAction({ settings: { moderation: [noPosting], appealSave: "closed" } });
    await formRead();
    await write(app, "Please look again.");

    await send(
      app,
      "The time to appeal this action has passed, so your appeal was not sent. To reach the moderators, see the contact on this server's About page.",
    );

    expect(settingsFindings(document.body)).toEqual([]);
  });
});
