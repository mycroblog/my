// SPDX-License-Identifier: AGPL-3.0-only

// Notifications, the entry of the settings (#3201, ADR-0086 §8–§10, §13): the tables of kinds by channel. A kind is a
// row and a channel a column; a cell shows what the server holds and saves a change at once with a PATCH
// /preferences/notifications of that cell alone, saying in its row how the save went; the first row changes a whole
// column in one request; a cell the server fixes stands on and disabled with its reason, and the cells of a kind that
// is off in the app stand off and disabled with theirs; Push is a column only while this device has a subscription,
// Email only on a server that sends it, Sound only while the sound is on. A row's name is the server's name of that
// kind's emails. What the table gives assistive technology is the design system's test (channels.dom.test.tsx); the
// switches of this device, notifications-settings-device.dom.test.tsx; the policy and the email that stopped,
// notifications-settings-policy.dom.test.tsx.

import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  memberNotificationRows,
  moderatorNotificationRows,
  type FakeNotificationRow,
  type FakeOptions,
} from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { changeSitePermission } from "../test/browser";
import {
  cell,
  columnsOf,
  describedBy,
  drawnSettings,
  hasCell,
  moderators,
  people,
  readSettings,
  rowNames,
  rowOf,
  said,
  saidIn,
  section,
  sections,
  server,
  stateOf,
} from "../test/notification-settings";

/** A member's rows on a server that sends email. */
const member = { email: true, settings: { notificationRows: memberNotificationRows } } as const satisfies FakeOptions;

/** A member's rows where the row of `group` starts as `cells` say. */
const withRow = (group: FakeNotificationRow["group"], cells: Partial<FakeNotificationRow>): FakeOptions => ({
  ...member,
  settings: {
    notificationRows: memberNotificationRows.map((row) => (row.group === group ? { ...row, ...cells } : row)),
  },
});

const route = "PATCH /preferences/notifications";
const fixedInApp = "Always arrives in the app.";
const fixedBoth = "Always arrives in the app and by email.";
const offInApp = "“In the app” is off, so it does not arrive any other way.";

describe("the tables", () => {
  test("a member reads the kinds from other people and from the server, each table under its heading and a row a kind", async () => {
    await readSettings(member);

    expect(sections()).toEqual(["This device", people, server, "Notifications from people you may not know"]);
    expect(screen.getAllByRole("table").map((table) => table.getAttribute("aria-label"))).toEqual([people, server]);
    expect(rowNames(people)).toEqual([
      "Mentions",
      "New followers",
      "Follow requests",
      "Accepted follow requests",
      "Boosts",
      "Favourites",
      "Emoji reactions",
      "Quotes",
      "Approval of replies, boosts and quotes",
      "Ended polls",
      "Accounts you follow moving",
      "Voice chats",
      "Events",
      "New posts",
    ]);
    expect(rowNames(server)).toEqual([
      "Notices from the moderators",
      "Closure of this server",
      "Your supporter plan",
      "Announcements",
      "Notices of the automatic deletion of posts",
      "Scheduled posts that could not be published",
    ]);
    // The first row of each table is the whole column's.
    for (const table of [people, server]) expect(hasCell(table, "All", "In the app")).toBe(true);
  });

  test("the first table says what a cell decides, and that a kind off in the app arrives no other way", async () => {
    await readSettings(member);

    expect(
      section(people).getAllByText(
        "Choose how each kind of notification reaches you. A kind with “In the app” off is not created, so it does not arrive any other way either.",
      ),
    ).toHaveLength(1);
  });

  test("a reader with the moderators' kinds has a table of them between the other two", async () => {
    await readSettings({
      ...member,
      viewer: "hana",
      settings: { notificationRows: [...memberNotificationRows, ...moderatorNotificationRows] },
    });

    expect(sections()).toEqual([
      "This device",
      people,
      moderators,
      server,
      "Notifications from people you may not know",
    ]);
    expect(rowNames(moderators)).toHaveLength(moderatorNotificationRows.length);
    expect(rowNames(moderators)).toContain("New reports");
    // A moderator's kinds are on in the app and by email until they say otherwise.
    expect(stateOf(cell(moderators, "All", "In the app"))).toBe("on");
    expect(stateOf(cell(moderators, "All", "Email"))).toBe("on");
    expect(stateOf(cell(moderators, "New reports", "Email"))).toBe("on");
  });

  test("a cell is at what the server holds, and a kind that is never emailed has no cell under Email", async () => {
    await readSettings(withRow("boost", { in_app: false, email: true }));

    expect(stateOf(cell(people, "Mentions", "In the app"))).toBe("on");
    expect(stateOf(cell(people, "Mentions", "Email"))).toBe("off");
    expect(stateOf(cell(people, "Boosts", "In the app"))).toBe("off");
    expect(stateOf(cell(server, "Notices of the automatic deletion of posts", "Email"))).toBe("on");
    for (const [table, row] of [
      [people, "Approval of replies, boosts and quotes"],
      [people, "New posts"],
      [server, "Announcements"],
      [server, "Scheduled posts that could not be published"],
    ] as const) {
      expect([row, hasCell(table, row, "In the app"), hasCell(table, row, "Email")]).toEqual([row, true, false]);
      // The row still has a cell's place under each column.
      expect(within(rowOf(table, row)).getAllByRole("cell")).toHaveLength(2);
    }
  });
});

describe("a read again in the background that fails", () => {
  test("leaves the tables and the policy drawn with what they read", async () => {
    const { fake, data } = await readSettings(member);
    const failed = [
      fake.script("GET /preferences/notifications", { fail: 500 }),
      fake.script("GET /preferences/notification_policy", { fail: 500 }),
    ];

    await data.queryClient.refetchQueries({ type: "active" });
    await waitFor(() => {
      expect(data.queryClient.isFetching()).toBe(0);
    });

    expect(failed.map((script) => script.seen.length > 0)).toEqual([true, true]);
    expect(sections()).toEqual(["This device", people, server, "Notifications from people you may not know"]);
    expect(stateOf(cell(people, "Mentions", "In the app"))).toBe("on");
    expect(screen.getAllByRole("table")).toHaveLength(2);
  });
});

// ADR-0086 §8: Push only while this device has a subscription and the browser lets the site notify, Email only on a
// server that sends it, Sound only while the sound is on.
describe("the columns", () => {
  const rows = { settings: { notificationRows: memberNotificationRows } } as const;
  const subscribed = { push: true, pushState: "subscribed" } as const;
  const cases: [string, FakeOptions, boolean, NotificationPermission, string[]][] = [
    ["on a server without push or email, with the sound off: the app alone", rows, false, "default", ["In the app"]],
    ["on a server that sends email: Email", { ...rows, email: true }, false, "default", ["In the app", "Email"]],
    [
      "while this device has a subscription: Push",
      { ...rows, ...subscribed },
      false,
      "granted",
      ["In the app", "Push"],
    ],
    [
      "on a server that sends push to a device without a subscription: no Push",
      { ...rows, push: true },
      false,
      "granted",
      ["In the app"],
    ],
    [
      "on a device with a subscription whose browser no longer lets the site notify: no Push",
      { ...rows, ...subscribed },
      false,
      "denied",
      ["In the app"],
    ],
    [
      "on a device with a subscription whose browser has not been asked: no Push",
      { ...rows, ...subscribed },
      false,
      "default",
      ["In the app"],
    ],
    ["while the sound is on: Sound", rows, true, "default", ["In the app", "Sound"]],
    [
      "with all of them: the app, Push, Email, Sound",
      { ...rows, ...subscribed, email: true },
      true,
      "granted",
      ["In the app", "Push", "Email", "Sound"],
    ],
  ];

  for (const [when, options, sound, permission, columns] of cases)
    test(when, async () => {
      const { data } = await readSettings(options, { browser: { permission }, ...(sound ? { sounding: [] } : {}) });

      // Once the subscription is read too: the Push column comes with it.
      await waitFor(() => {
        expect(columnsOf(people)).toEqual(columns);
        expect(data.queryClient.isFetching()).toBe(0);
      });
      expect(columnsOf(server)).toEqual(columns);
    });

  test("the Push column goes once the browser's settings stop the site's notifications, with the screen open", async () => {
    await readSettings({ ...rows, ...subscribed }, { browser: { permission: "granted" } });
    await waitFor(() => {
      expect(columnsOf(people)).toEqual(["In the app", "Push"]);
    });

    changeSitePermission("denied");

    await waitFor(() => {
      expect(columnsOf(people)).toEqual(["In the app"]);
    });
  });
});

describe("a cell saves at once, with a PATCH of that cell alone", () => {
  const changes = [
    [
      "a kind off in the app",
      people,
      "Favourites",
      "In the app",
      { group: "favourite", channel: "in_app", enabled: false },
    ],
    ["a kind's email on", people, "Mentions", "Email", { group: "mention", channel: "email", enabled: true }],
    [
      "a kind's email off",
      server,
      "Notices of the automatic deletion of posts",
      "Email",
      { group: "auto_delete_limit", channel: "email", enabled: false },
    ],
    [
      "the email of the moderators' notices, which the app always shows",
      server,
      "Notices from the moderators",
      "Email",
      { group: "moderation", channel: "email", enabled: true },
    ],
  ] as const;

  for (const [what, table, row, column, change] of changes)
    test(what, async () => {
      const { fake, user } = await readSettings(member);
      const sent = fake.script(route);
      const before = stateOf(cell(table, row, column));

      await user.click(cell(table, row, column));

      expect((await sent.served()).body).toEqual({ changes: [change] });
      await waitFor(() => {
        expect(saidIn(table, row)).toMatch(new RegExp(`^${column}\\s*Saved$`));
      });
      expect(sent.seen).toHaveLength(1);
      expect(stateOf(cell(table, row, column))).toBe(before === "on" ? "off" : "on");
    });
});

describe("the row of a cell says how its save goes", () => {
  test("Saving… behind the channel's name until the server answers, then Saved, and no other row speaks", async () => {
    const { fake, user } = await readSettings(member);
    const sent = fake.script(route, { hold: "response" });
    expect(said(screen.getAllByRole("status"))).toEqual([]);

    await user.click(cell(people, "Mentions", "Email"));
    await sent.arrived();

    expect(saidIn(people, "Mentions")).toMatch(/^Email\s*Saving…$/);
    expect(said(screen.getAllByRole("status"))).toHaveLength(1);
    // The cell shows the change before the server answers.
    expect(stateOf(cell(people, "Mentions", "Email"))).toBe("on");

    sent.release();
    await waitFor(() => {
      expect(saidIn(people, "Mentions")).toMatch(/^Email\s*Saved$/);
    });
    expect(stateOf(cell(people, "Mentions", "Email"))).toBe("on");
  });

  test("a change the server fails goes back, the row says so, and Try again sends the same change", async () => {
    const { fake, user } = await readSettings(member);
    const sent = fake.script(route);
    const failing = fake.script(route, { times: 1, fail: 500 });

    await user.click(cell(people, "Favourites", "In the app"));

    await waitFor(() => {
      expect(saidIn(people, "Favourites")).toMatch(/^In the app\s*This could not be saved\.\s*Try again$/);
    });
    expect(stateOf(cell(people, "Favourites", "In the app"))).toBe("on");
    // The other cells of the row are the reader's to change still: the kind is on in the app.
    expect(stateOf(cell(people, "Favourites", "Email"))).toBe("off");
    failing.end();

    await user.click(within(rowOf(people, "Favourites")).getByRole("button", { name: "Try again" }));

    await waitFor(() => {
      expect(saidIn(people, "Favourites")).toMatch(/^In the app\s*Saved$/);
    });
    const change = { changes: [{ group: "favourite" as const, channel: "in_app" as const, enabled: false }] };
    expect(sent.seen.map((request) => request.body)).toEqual([change, change]);
    expect(stateOf(cell(people, "Favourites", "In the app"))).toBe("off");
    expect(within(rowOf(people, "Favourites")).queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
  });
});

describe("the first row, All", () => {
  /** The groups a member may change the email of among the kinds from other people, in the table's order. */
  const emailed = memberNotificationRows
    .filter((row) => row.section === "ordinary" && row.email !== undefined)
    .map((row) => row.group);

  test("is on when every cell of its column the reader may change is on, off when none is, and mixed between", async () => {
    await readSettings(withRow("mention", { email: true }));

    expect(stateOf(cell(people, "All", "In the app"))).toBe("on");
    expect(stateOf(cell(people, "All", "Email"))).toBe("mixed");
    // The cells the server fixes are not counted: of the three it does not, all are on in the app, one by email.
    expect(stateOf(cell(server, "All", "In the app"))).toBe("on");
    expect(stateOf(cell(server, "All", "Email"))).toBe("mixed");
  });

  test("off, it stands off where no cell of the column is on", async () => {
    await readSettings(withRow("auto_delete_limit", { email: false }));

    expect(stateOf(cell(people, "All", "Email"))).toBe("off");
    expect(stateOf(cell(server, "All", "Email"))).toBe("off");
  });

  test("turns its column on for every kind of its table the reader may change, in one request", async () => {
    const { fake, user } = await readSettings(member);
    const sent = fake.script(route);

    await user.click(cell(people, "All", "Email"));

    const { changes } = (await sent.served()).body;
    expect(changes.map((change) => [change.channel, change.enabled])).toEqual(emailed.map(() => ["email", true]));
    expect(changes.map((change) => change.group)).toEqual(emailed);
    await waitFor(() => {
      expect(stateOf(cell(people, "All", "Email"))).toBe("on");
    });
    expect(sent.seen).toHaveLength(1);
    expect(stateOf(cell(people, "Mentions", "Email"))).toBe("on");
    expect(stateOf(cell(people, "Events", "Email"))).toBe("on");
    // The other table's column is its own.
    expect(stateOf(cell(server, "Notices from the moderators", "Email"))).toBe("off");
  });

  test("turns its column off for the kinds the reader may change, and leaves out the cells the server fixes", async () => {
    const { fake, user } = await readSettings(member);
    const sent = fake.script(route);

    await user.click(cell(server, "All", "In the app"));

    expect((await sent.served()).body).toEqual({
      changes: [
        { group: "announcement", channel: "in_app", enabled: false },
        { group: "auto_delete_limit", channel: "in_app", enabled: false },
        { group: "scheduled_status_failed", channel: "in_app", enabled: false },
      ],
    });
    await waitFor(() => {
      expect(stateOf(cell(server, "All", "In the app"))).toBe("off");
    });
    expect(sent.seen).toHaveLength(1);
    expect(stateOf(cell(server, "Announcements", "In the app"))).toBe("off");
    expect(stateOf(cell(server, "Closure of this server", "In the app"))).toBe("on, disabled");
  });

  test("mixed, a press turns the rest of its column on", async () => {
    const { fake, user } = await readSettings(withRow("mention", { email: true }));
    const sent = fake.script(route);

    await user.click(cell(people, "All", "Email"));

    const { changes } = (await sent.served()).body;
    expect(changes.every((change) => change.channel === "email" && change.enabled)).toBe(true);
    await waitFor(() => {
      expect(stateOf(cell(people, "All", "Email"))).toBe("on");
    });
    expect(stateOf(cell(people, "Boosts", "Email"))).toBe("on");
  });
});

describe("a cell the server fixes", () => {
  test("stands on and disabled, and its row says which way the kind always arrives", async () => {
    await readSettings({
      ...member,
      settings: {
        notificationRows: [
          ...memberNotificationRows,
          // A kind whose email alone the server keeps on.
          { group: "admin_report", section: "instance", in_app: true, email: "fixed" },
        ],
      },
    });

    expect(stateOf(cell(server, "Notices from the moderators", "In the app"))).toBe("on, disabled");
    expect(describedBy(cell(server, "Notices from the moderators", "In the app"))).toEqual([fixedInApp]);
    // The cell of the same row the reader may change is theirs.
    expect(stateOf(cell(server, "Notices from the moderators", "Email"))).toBe("off");
    for (const column of ["In the app", "Email"]) {
      expect(stateOf(cell(server, "Closure of this server", column))).toBe("on, disabled");
      expect(describedBy(cell(server, "Closure of this server", column))).toEqual([fixedBoth]);
    }
    expect(stateOf(cell(server, "New reports", "Email"))).toBe("on, disabled");
    expect(describedBy(cell(server, "New reports", "Email"))).toEqual(["Always arrives by email."]);
    expect(stateOf(cell(server, "New reports", "In the app"))).toBe("on");
  });

  test("takes no press: nothing is sent", async () => {
    const { fake, user } = await readSettings(member);
    const sent = fake.script(route);

    await user.click(cell(server, "Closure of this server", "Email"));
    // The next change the reader makes is the first the server hears of.
    await user.click(cell(server, "Announcements", "In the app"));

    expect((await sent.served()).body).toEqual({
      changes: [{ group: "announcement", channel: "in_app", enabled: false }],
    });
    expect(sent.seen).toHaveLength(1);
  });
});

describe("a kind that is off in the app", () => {
  // On a device with a subscription for favourites and their sound on, and with their email on at the server.
  const everyChannel = {
    ...withRow("favourite", { in_app: false, email: true }),
    push: true,
    pushState: "subscribed",
  } as const satisfies FakeOptions;

  test("has its other cells off and disabled, and its row says why", async () => {
    await drawnSettings(
      everyChannel,
      { sounding: ["favourite", "mention"], browser: { permission: "granted" } },
      ({ fake }) => {
        fake.script("GET /push/subscription", {
          change: (held) => ({ ...held, alerts: ["mention" as const, "favourite" as const] }),
        });
      },
    );
    await waitFor(() => {
      expect(columnsOf(people)).toEqual(["In the app", "Push", "Email", "Sound"]);
    });

    expect(stateOf(cell(people, "Favourites", "In the app"))).toBe("off");
    for (const column of ["Push", "Email", "Sound"]) {
      expect([column, stateOf(cell(people, "Favourites", column))]).toEqual([column, "off, disabled"]);
      expect(describedBy(cell(people, "Favourites", column))).toEqual([offInApp]);
    }
    // A kind that is on in the app shows what each channel holds.
    expect(stateOf(cell(people, "Mentions", "Push"))).toBe("on");
    expect(stateOf(cell(people, "Mentions", "Sound"))).toBe("on");
  });

  test("turned on in the app again, its cells are the reader's, at what each channel held", async () => {
    const { user } = await readSettings(withRow("favourite", { in_app: false, email: true }));
    expect(stateOf(cell(people, "Favourites", "Email"))).toBe("off, disabled");

    await user.click(cell(people, "Favourites", "In the app"));

    await waitFor(() => {
      expect(stateOf(cell(people, "Favourites", "Email"))).toBe("on");
    });
    expect(describedBy(cell(people, "Favourites", "Email"))).toEqual([]);
  });

  test("turned off in the app on the screen, its other cells go off and disabled at once", async () => {
    const { user } = await readSettings(withRow("favourite", { email: true }));
    expect(stateOf(cell(people, "Favourites", "Email"))).toBe("on");

    await user.click(cell(people, "Favourites", "In the app"));

    await waitFor(() => {
      expect(stateOf(cell(people, "Favourites", "Email"))).toBe("off, disabled");
    });
    expect(describedBy(cell(people, "Favourites", "Email"))).toEqual([offInApp]);
  });

  test("is not counted by the first row of the other columns", async () => {
    // Every kind's email is on at the server; the one that is off in the app arrives no way.
    await readSettings({
      email: true,
      settings: {
        notificationRows: [
          { group: "mention", section: "ordinary", in_app: true, email: true },
          { group: "favourite", section: "ordinary", in_app: false, email: true },
        ],
      },
    });

    expect(stateOf(cell(people, "All", "In the app"))).toBe("mixed");
    expect(stateOf(cell(people, "All", "Email"))).toBe("on");
  });
});

// The server names a kind in its emails and on its unsubscribe page; the settings' row is the same kind, so it has
// the same name (locales/<lang>/server/mail.json, `mail.group.<group>`), in each language.
describe("a row's name is the server's name of that kind's emails", () => {
  const tables = {
    en: { ordinary: people, moderator: moderators, instance: server },
    ja: { ordinary: "ほかの人から", moderator: "モデレーター向け", instance: "このサーバーから" },
  } as const;
  const every = [...memberNotificationRows, ...moderatorNotificationRows];

  /** The server's messages of its emails in `language`, each by its key. */
  function serverCatalog(language: "en" | "ja"): Record<string, unknown> {
    const file = join(import.meta.dir, "../../../../locales", language, "server/mail.json");
    return JSON.parse(readFileSync(file, "utf8")) as Record<string, unknown>;
  }

  /** The words of a message of the server's catalog: the message itself, or its `defaultMessage`. */
  function wordsOf(message: unknown): string | undefined {
    if (typeof message === "string") return message;
    if (typeof message !== "object" || message === null || !("defaultMessage" in message)) return undefined;
    return typeof message.defaultMessage === "string" ? message.defaultMessage : undefined;
  }

  for (const language of ["en", "ja"] as const)
    test(`in ${language}`, async () => {
      const catalog = serverCatalog(language);
      await drawnSettings(
        {
          viewer: "hana",
          email: true,
          settings: { notificationRows: every },
          preferences: { locale: language, locale_follows_browser: false },
        },
        { locale: language },
      );
      await screen.findAllByRole("table");

      const named: [string, string | undefined][] = [];
      const expected: [string, string | undefined][] = [];
      for (const part of ["ordinary", "moderator", "instance"] as const) {
        const names = rowNames(tables[language][part]);
        every
          .filter((row) => row.section === part)
          .forEach((row, at) => {
            // The server names the kinds a reader may stop the emails of (ADR-0086 §4, §11): a kind that is never
            // emailed, or whose email the server always sends, has no such name.
            if (typeof row.email !== "boolean") return;
            named.push([row.group, names[at]]);
            expected.push([row.group, wordsOf(catalog[`mail.group.${row.group}`])]);
          });
      }
      expect(expected.filter(([, words]) => words === undefined)).toEqual([]);
      expect(named).toEqual(expected);
      // The check compared something: every kind whose email a member or a moderator may turn on and off.
      expect(named.length).toBe(every.filter((row) => typeof row.email === "boolean").length);
      expect(named.length).toBeGreaterThan(20);
    });
});

describe("before the settings are read", () => {
  test("the screen says that it loads them, with no table and no choice, until they come", async () => {
    let reading: { arrived: () => Promise<unknown>; release: () => void } | undefined;
    await drawnSettings(member, {}, ({ fake }) => {
      reading = fake.script("GET /preferences/notifications", { hold: "request" });
    });
    await reading?.arrived();

    expect(said(screen.getAllByRole("status"))).toEqual(["Loading your settings…"]);
    expect(screen.queryAllByRole("table")).toHaveLength(0);
    expect(screen.queryAllByRole("combobox")).toHaveLength(0);

    reading?.release();
    await screen.findAllByRole("table");
    expect(said(screen.queryAllByRole("status"))).toEqual([]);
  });

  for (const [what, read] of [
    ["the kinds", "GET /preferences/notifications"],
    ["the policy", "GET /preferences/notification_policy"],
  ] as const)
    test(`a read of ${what} that fails says so, and Try again reads them`, async () => {
      let reading: { seen: readonly unknown[] } | undefined;
      const { user } = await drawnSettings(member, {}, ({ fake }) => {
        reading = fake.script(read, { times: 1, fail: 500 });
      });

      const failed = await screen.findByRole("alert");
      expect(failed.textContent).toContain("Your settings could not be loaded.");
      expect(screen.queryAllByRole("table")).toHaveLength(0);
      expect(screen.queryAllByRole("combobox")).toHaveLength(0);

      await user.click(within(failed).getByRole("button", { name: "Try again" }));

      await screen.findAllByRole("table");
      expect(reading?.seen).toHaveLength(2);
      expect(screen.queryAllByRole("alert")).toHaveLength(0);
      expect(screen.getAllByRole("combobox")).toHaveLength(5);
    });
});
