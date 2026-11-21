// SPDX-License-Identifier: AGPL-3.0-only

// The reader's data in the settings (#3203; FR-008, FR-009, FR-118, FR-121, ADR-0089 §2, ADR-0151 §7, §8), as the app
// places it: the entry Account and data at /settings/account, its parts and the doors of each; Export, Import, Delete
// old posts automatically and Aliases under it, each at an address of its own; and the address the closure's emails
// carry. What each screen asks of the server and says of its answer is pinned here, on the fake server through the
// whole app; where the focus goes when it lands on what is no control (a status, a title) is the feature's jsdom test
// (app-features, settings/account-data.jsdom.test.tsx), and how each screen says it loads is the feature's too
// (settings/account-data.dom.test.tsx). The screens that leave the account are settings-account-leave.dom.test.tsx.

import { csvKinds, type CsvKind } from "@my/app-data";
import { settingsFindings } from "@my/design-system/test/settings";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, test } from "bun:test";
import {
  aTurn,
  buttons,
  confirmations,
  dayOf,
  describedBy,
  focusOn,
  heldBy,
  rowsOf,
  said,
  saying,
  section,
  sectionRead,
  sections,
  titleOf,
} from "./test/account-data";
import { main, renderApp, words, type TestApp } from "./test/app";
import { chooseFiles } from "./test/files";
import { fakeApp } from "./test/server";

const entry = "/settings/account";
const exportAt = "/settings/account/export";
const importAt = "/settings/account/import";
const autoDeleteAt = "/settings/account/auto-delete";
const aliasesAt = "/settings/account/aliases";

type Settings = NonNullable<FakeOptions["settings"]>;
type Import = NonNullable<Settings["imports"]>[number];
type Policy = NonNullable<Settings["autoDelete"]>;
type Limit = NonNullable<Settings["autoDeleteLimit"]>;

/** An import of follows that merged 120 entries, all of them done. */
const follows: Import = {
  kind: "following",
  mode: "merge",
  state: "finished",
  created_at: "-2d",
  total: 120,
  processed: 120,
  failed: 0,
};

/** An import of blocked accounts that replaces them, in progress, a third of the way. */
const running: Import = {
  kind: "blocks",
  mode: "overwrite",
  state: "in_progress",
  created_at: "-0d",
  total: 300,
  processed: 96,
  failed: 2,
};

/** A policy on, for posts older than 90 days, that keeps the pinned, the bookmarked, direct posts and the favourited. */
const policy: Policy = {
  enabled: true,
  min_age_days: 90,
  keep_pinned: true,
  keep_bookmarked: true,
  keep_favourited: false,
  keep_media: false,
  keep_polls: false,
  keep_articles: true,
  keep_visibilities: ["direct"],
  keep_min_boosts: 0,
  keep_min_favourites: 25,
};

/** A maximum of half a year under which the three most recently pinned posts alone can be kept. */
const strict: Limit = {
  max_age_days: 180,
  allow_pinned: true,
  kept_pins: 3,
  allow_bookmarked: false,
  allow_favourited: false,
  allow_media: false,
  allow_polls: false,
  allow_articles: false,
  allow_visibilities: false,
  allow_thresholds: false,
};

/** A server whose reader saved `saved`, with no maximum of the server unless `limit` gives one. */
const deleting = (saved: Policy, limit?: Limit): FakeOptions => ({
  settings: { autoDelete: saved, autoDeleteLimit: limit },
});

/** What the links of the list named `name` say, each as a reader hears its line, and where each leads. */
const doors = (name: string) =>
  within(main().getByRole("list", { name }))
    .getAllByRole("link")
    .map((link) => [words(link), link.getAttribute("href")]);

// ---- The entry ------------------------------------------------------------------------------------------------------

describe("the entry Account and data", () => {
  test("has the account's status, then Data and posts, Moving to this account, Moving to another account, and last Deactivating or deleting this account", async () => {
    await renderApp(entry);
    await main().findByRole("link", { name: /^Export/ });

    expect(sections()).toEqual([
      "A moderation action is in effect on your account",
      "Data and posts",
      "Moving to this account",
      "Moving to another account",
      "Deactivating or deleting this account",
    ]);
  });

  test("leads through a door of each part, a link with one line: what it holds, or where it stands now", async () => {
    const on = fakeApp();
    const limit = await heldBy<{ server_limit: { enforce_from: string } }>(on.fake, "/preferences/auto_delete");
    await renderApp(entry, { on });
    await main().findByRole("link", { name: /^Export/ });

    expect(doors("Data and posts")).toEqual([
      ["Export An archive of your account, and your follows, blocks, mutes and bookmarks as CSV files", exportAt],
      ["Import Follows, blocks, mutes and bookmarks from CSV files", importAt],
      [
        `Delete old posts automatically From ${dayOf(limit.server_limit.enforce_from)}, this server's limit applies to your posts`,
        autoDeleteAt,
      ],
    ]);
    expect(doors("Moving to this account")).toEqual([["Aliases 1 account can move its followers here", aliasesAt]]);
    expect(doors("Moving to another account")).toEqual([
      ["Move account Move your followers to another account of yours", "/settings/account/move"],
    ]);
    expect(doors("Deactivating or deleting this account")).toEqual([
      [
        "Deactivate account Hide your account and posts, which are deleted unless you sign in again in time",
        "/settings/account/deactivate",
      ],
      ["Delete account Delete your account and posts at once, for good", "/settings/account/delete"],
    ]);
  });

  test("says on each door what goes on behind it: an archive being prepared, an import in progress, the deletion on", async () => {
    await renderApp(entry, {
      dataExport: "building",
      settings: { imports: [running, follows], autoDelete: policy, autoDeleteLimit: undefined },
    });
    await main().findByRole("link", { name: /^Export/ });

    expect(doors("Data and posts").map(([says]) => says)).toEqual([
      "Export An archive is being prepared",
      "Import An import is in progress: 96 of 300 done",
      "Delete old posts automatically On: posts older than 90 days are deleted",
    ]);
  });

  test("with nothing behind a door, says what it leads to, and that the deletion is off", async () => {
    await renderApp(entry, {
      settings: { imports: [], aliases: [], autoDelete: { ...policy, enabled: false }, autoDeleteLimit: undefined },
    });
    await main().findByRole("link", { name: /^Export/ });

    expect(doors("Data and posts").map(([says]) => says)).toEqual([
      "Export An archive of your account, and your follows, blocks, mutes and bookmarks as CSV files",
      "Import Follows, blocks, mutes and bookmarks from CSV files",
      "Delete old posts automatically Off",
    ]);
    expect(doors("Moving to this account").map(([says]) => says)).toEqual([
      "Aliases Name the account you are moving from, so that its followers can move here",
    ]);
  });

  test("of an account that moved, says on the door of Move account where", async () => {
    await renderApp(entry, { settings: { movedTo: "sora@photos.example" } });

    await main().findByRole("link", { name: /^Move account/ });
    expect(doors("Moving to another account").map(([says]) => says)).toEqual([
      "Move account Moved to @sora@photos.example",
    ]);
  });
});

// ---- Export ---------------------------------------------------------------------------------------------------------

/** The words of the kinds of CSV files, as the export lists them. */
const kindWords: Record<CsvKind, string> = {
  following: "Follows",
  followed_tags: "Followed hashtags",
  blocks: "Blocked accounts",
  mutes: "Muted accounts",
  domain_mutes: "Muted servers",
  bookmarks: "Bookmarks",
};

/** The row of the CSV file of `kind`. */
const csvRow = (kind: CsvKind) =>
  within(within(section("CSV files")).getByRole("list", { name: "CSV files" }))
    .getAllByRole("listitem")
    .filter((item) => within(item).queryAllByRole("group", { name: kindWords[kind] }).length > 0)[0] ?? document.body;

describe("Export", () => {
  test("with no archive, says so and offers Request an archive; a request says it once, and the new archive's row says it is being prepared, with no Request meanwhile", async () => {
    const app = await renderApp(exportAt);
    const archive = await sectionRead("Archive");
    expect(words(archive)).toContain("You have no archive.");
    const requested = app.fake.script("POST /exports");

    await app.user.click(within(archive).getByRole("button", { name: "Request an archive" }));

    await saying(() => section("Archive"), "The archive was requested.");
    expect(requested.seen).toHaveLength(1);
    await waitFor(() => {
      if (within(section("Archive")).queryAllByRole("list", { name: "Archive" }).length === 0)
        throw new Error("test: no archive is listed yet");
    });
    const [row, ...others] = rowsOf(section("Archive"), "Archive");
    expect(others).toEqual([]);
    expect(row).toMatch(
      /^Archive of .+ This archive is being prepared\. You can leave this page meanwhile, and request another once it is ready\.$/,
    );
    expect(buttons(section("Archive"))).not.toContain("Request an archive");
    expect(said(section("Archive"))).toEqual(["The archive was requested."]);
  });

  test("while an archive is being prepared, its row says so, and nothing requests another", async () => {
    await renderApp(exportAt, { dataExport: "building" });
    const archive = await sectionRead("Archive");

    await waitFor(() => within(archive).getByRole("list", { name: "Archive" }));
    expect(rowsOf(archive, "Archive")).toEqual([
      expect.stringMatching(/^Archive of .+ This archive is being prepared\./) as unknown as string,
    ]);
    expect(buttons(archive)).toEqual([]);
  });

  test("an archive ready is downloaded from its row, and Request an archive stays at the head", async () => {
    const app = await renderApp(exportAt, { dataExport: "ready" });
    const archive = await sectionRead("Archive");
    await waitFor(() => within(archive).getByRole("list", { name: "Archive" }));
    const [row] = rowsOf(archive, "Archive");
    expect(row).toMatch(/^Archive of .+ You can download this archive until .+\. Download$/);
    expect(buttons(archive)).toEqual(["Request an archive", "Download"]);
    const downloaded = app.fake.script("GET /exports/{id}/archive");

    await app.user.click(within(archive).getByRole("button", { name: "Download" }));

    await waitFor(() => {
      if (downloaded.seen.length === 0) throw new Error("test: the archive was not asked for");
    });
  });

  for (const [state, says] of [
    ["expired", /^Archive of .+ This archive expired on .+\. Request a new one\.$/],
    ["failed", /^Archive of .+ This archive could not be prepared\. Request a new one\.$/],
  ] as const)
    test(`an archive that ${state === "expired" ? "expired" : "could not be prepared"} says so, with Request an archive and no Download`, async () => {
      await renderApp(exportAt, { dataExport: state });
      const archive = await sectionRead("Archive");

      await waitFor(() => within(archive).getByRole("list", { name: "Archive" }));
      expect(rowsOf(archive, "Archive")).toHaveLength(1);
      expect(rowsOf(archive, "Archive")[0]).toMatch(says);
      expect(buttons(archive)).toEqual(["Request an archive"]);
    });

  test("lists a CSV file of each kind, in the order of `csvKinds`, each with Download", async () => {
    await renderApp(exportAt);
    const csv = await sectionRead("CSV files");

    expect(rowsOf(csv, "CSV files")).toEqual(csvKinds.map((kind) => `${kindWords[kind]} Download`));
  });

  test("a CSV file's Download says in its own row that it started, and keeps the focus on the button", async () => {
    const app = await renderApp(exportAt);
    await sectionRead("CSV files");
    const asked = app.fake.script("GET /csv_exports/{kind}");
    const button = within(csvRow("blocks")).getByRole("button", { name: "Download" });

    await app.user.click(button);

    await waitFor(() => {
      if (said(csvRow("blocks")).length === 0) throw new Error("test: the row says nothing yet");
    });
    const [says, ...more] = said(csvRow("blocks"));
    expect(more).toEqual([]);
    expect(says).toMatch(/^The download of .+ started\.$/);
    expect(asked.seen.map((s) => s.url.pathname.split("/").at(-1))).toEqual(["blocks"]);
    for (const kind of csvKinds.filter((other) => other !== "blocks")) expect(said(csvRow(kind))).toEqual([]);
    expect(document.activeElement === button).toBe(true);
  });

  test("a CSV file that could not be downloaded says so in its own row", async () => {
    const app = await renderApp(exportAt);
    await sectionRead("CSV files");
    app.fake.script("GET /csv_exports/{kind}", { fail: 500 });

    await app.user.click(within(csvRow("mutes")).getByRole("button", { name: "Download" }));

    await saying(() => csvRow("mutes"), "The file could not be downloaded. Try again.");
    for (const kind of csvKinds.filter((other) => other !== "mutes")) expect(said(csvRow(kind))).toEqual([]);
  });

  test("/settings/import-export, the address in the closure's emails already sent, lands on Export", async () => {
    await renderApp("/settings/import-export", { dataExport: "ready" });

    await sectionRead("Archive");
    expect(main().getByRole("heading", { level: 1 }).textContent).toBe("Export");
    expect(sections()).toEqual(["Archive", "CSV files"]);
  });

  test("whose archives could not be read says so, and Try again reads them", async () => {
    const on = fakeApp();
    const failing = on.fake.script("GET /exports", { fail: 500 });
    const app = await renderApp(entry, { on });
    await app.navigate(exportAt);

    expect(words(await main().findByText("Your archives could not be loaded."))).toBe(
      "Your archives could not be loaded.",
    );
    failing.end();
    await app.user.click(within(section("Archive")).getByRole("button", { name: "Try again" }));

    await within(section("Archive")).findByRole("button", { name: "Request an archive" });
  });
});

// ---- Import ---------------------------------------------------------------------------------------------------------

/** A CSV file of follows as an export writes it: three accounts, one of them twice, and a line that is no account. */
const followsFile = () =>
  new File(
    [
      [
        "Account address,Show boosts,Notify on new posts,Languages",
        "marco@demo.test,true,false,",
        "sam@demo.test,true,false,",
        "not an account",
        "marco@demo.test,true,false,",
        "hana@demo.test,true,false,",
      ].join("\n"),
    ],
    "follows.csv",
    { type: "text/csv" },
  );

/** The button that opens the file chooser, whatever a file chosen made of its name. */
const fileButton = () => within(section("File")).getByRole("button", { name: /^Choose (a|another) file$/ });

/** The submit bar's Import. */
const importButton = () => main().getAllByRole("button", { name: "Import" }).at(-1) ?? document.body;

/** Chooses `file` for the import, and waits until the preview read it. */
async function chosen(app: TestApp, file: File = followsFile()) {
  await chooseFiles(app, fileButton(), file);
  await waitFor(() => within(section("Preview")).getByRole("list", { name: /./ }));
}

/** The lists of the preview, by their names, each with what its rows say. */
const previewed = () =>
  within(section("Preview"))
    .queryAllByRole("list")
    .map((list) => [
      list.getAttribute("aria-label") ?? titleOf(list),
      within(list).getAllByRole("listitem").map(words),
    ]);

describe("Import", () => {
  test("asks for no preview before a file is chosen", async () => {
    const on = fakeApp();
    const previews = on.fake.script("POST /imports/preview");
    await renderApp(importAt, { on });
    await sectionRead("Recent imports");
    await aTurn();

    expect(previews.seen).toHaveLength(0);
    expect(said(section("Preview"))).toEqual(["Choose a file to see what importing it would do."]);
  });

  test("a file chosen is previewed with what it lists and how it imports, and again on a change of either", async () => {
    const app = await renderApp(importAt);
    await sectionRead("Recent imports");
    const previews = app.fake.script("POST /imports/preview");
    const file = followsFile();
    const text = await file.text();

    await chosen(app, file);
    expect(previews.seen.map((s) => s.body)).toEqual([{ kind: "following", mode: "merge", text }]);

    await app.user.selectOptions(main().getByRole("combobox", { name: "What the file lists" }), "Blocked accounts");
    await waitFor(() => {
      if (previews.seen.length < 2) throw new Error("test: no second preview");
    });
    await app.user.selectOptions(main().getByRole("combobox", { name: "How to import" }), "Replace");
    await waitFor(() => {
      if (previews.seen.length < 3) throw new Error("test: no third preview");
    });
    expect(previews.seen.map((s) => s.body)).toEqual([
      { kind: "following", mode: "merge", text },
      { kind: "blocks", mode: "merge", text },
      { kind: "blocks", mode: "overwrite", text },
    ]);
  });

  test("the preview says what is new, what the reader has already, and the lines skipped, in the file's own words", async () => {
    const app = await renderApp(importAt);
    await sectionRead("Recent imports");

    await chosen(app);

    const [[, lines] = ["", []], ...others] = previewed();
    expect(others).toEqual([]);
    // The fake counts a quarter of the new entries as present already: none of three.
    expect(lines).toEqual([
      "3 new accounts to follow",
      "2 lines are skipped Line 4 cannot be read: “not an account” Line 5 repeats an earlier line: “marco@demo.test,true,false,”",
    ]);
  });

  test("the preview counts what the reader has already apart from what is new", async () => {
    const app = await renderApp(importAt);
    await sectionRead("Recent imports");
    const handles = ["marco", "sam", "hana", "ren", "mio", "layla", "sora", "kobo"].map((name) => `${name}@demo.test`);

    await chosen(app, new File([handles.join("\n")], "eight.csv", { type: "text/csv" }));

    // Of the eight, the fake counts a quarter as followed already.
    expect(previewed().map(([, lines]) => lines)).toEqual([
      ["6 new accounts to follow", "2 accounts you already follow"],
    ]);
  });

  test("with Replace, lists apart what “Replace” removes, and says it beside Import, which it describes; with Merge, nothing", async () => {
    const app = await renderApp(importAt);
    await sectionRead("Recent imports");
    await chosen(app);
    expect(previewed().map(([name]) => name)).not.toContain("What “Replace” removes");
    expect(describedBy(importButton())).toEqual([]);

    await app.user.selectOptions(main().getByRole("combobox", { name: "How to import" }), "Replace");

    await waitFor(() => {
      if (!previewed().some(([name]) => name === "What “Replace” removes"))
        throw new Error(`test: the preview lists ${JSON.stringify(previewed())}`);
    });
    expect(previewed().at(-1)).toEqual([
      "What “Replace” removes",
      ["5 accounts to unfollow, which the file does not list"],
    ]);
    expect(describedBy(importButton())).toEqual(["Importing unfollows 5 accounts."]);
  });

  test("Import with Replace that removes something asks first, and Cancel sends nothing", async () => {
    const app = await renderApp(importAt);
    await sectionRead("Recent imports");
    await chosen(app);
    await app.user.selectOptions(main().getByRole("combobox", { name: "How to import" }), "Replace");
    await waitFor(() => {
      if (describedBy(importButton()).length === 0) throw new Error("test: Import says nothing of what it removes");
    });
    const started = app.fake.script("POST /imports");

    await app.user.click(importButton());

    const asking = await screen.findByRole("alertdialog", { name: "Replace your follows?" });
    expect(words(asking)).toContain("You will unfollow 5 accounts that the file does not list. This cannot be undone.");
    expect(buttons(asking)).toEqual(["Close", "Cancel", "Replace"]);

    await app.user.click(within(asking).getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      if (confirmations().length > 0) throw new Error("test: the confirmation is still open");
    });
    await aTurn();
    expect(started.seen).toHaveLength(0);

    await app.user.click(importButton());
    await app.user.click(
      within(await screen.findByRole("alertdialog", { name: "Replace your follows?" })).getByRole("button", {
        name: "Replace",
      }),
    );

    await waitFor(() => {
      if (started.seen.length === 0) throw new Error("test: the import was not started");
    });
    expect(started.seen.map((s) => [s.body.kind, s.body.mode])).toEqual([["following", "overwrite"]]);
  });

  test("Import with no file says so under the file's field, with the focus on its button, and sends nothing", async () => {
    const app = await renderApp(importAt);
    await sectionRead("Recent imports");
    const started = app.fake.script("POST /imports");

    await app.user.click(importButton());

    // The field is the group of the file's button and what names the file chosen.
    const field = () => within(section("File")).getByRole("group", { name: "CSV file" });
    await waitFor(() => {
      if (!describedBy(field()).includes("Choose the CSV file to import."))
        throw new Error(`test: the file's field is described by ${JSON.stringify(describedBy(field()))}`);
    });
    await focusOn(fileButton);
    expect(main().getAllByRole("alert").map(words)).toEqual(["Nothing was imported. Fix what is marked."]);
    expect(started.seen).toHaveLength(0);
  });

  test("an import refused because another is in progress is said in the submit bar, and the file is kept", async () => {
    const app = await renderApp(importAt, { settings: { imports: [running, follows] } });
    await sectionRead("Recent imports");
    await chosen(app);

    await app.user.click(importButton());

    await waitFor(() => {
      if (
        !words(screen.getByRole("main")).includes(
          "Another import is still in progress. Start this one once it is done.",
        )
      )
        throw new Error("test: the refusal is not said");
    });
    expect(fileButton().textContent).toBe("Choose another file");
    expect(describedBy(fileButton())).toEqual(["follows.csv"]);
    expect(previewed()).toHaveLength(1);
  });

  test("an import started empties the form, says it started, and lists the import in progress", async () => {
    const app = await renderApp(importAt);
    await sectionRead("Recent imports");
    await chosen(app);
    const started = app.fake.script("POST /imports");

    await app.user.click(importButton());

    await waitFor(() => {
      if (
        !words(screen.getByRole("main")).includes(
          "The import started. It goes on in the background, and you can leave this page.",
        )
      )
        throw new Error("test: the start is not said");
    });
    expect(started.seen).toHaveLength(1);
    expect(fileButton().textContent).toBe("Choose a file");
    expect(describedBy(fileButton())).toEqual(["No file chosen"]);
    await waitFor(() => {
      const [newest] = rowsOf(section("Recent imports"), "Recent imports");
      if (newest === undefined || !newest.startsWith("Follows Merge"))
        throw new Error(`test: the newest import is ${String(newest)}`);
    });
    expect(rowsOf(section("Recent imports"), "Recent imports")[0]).toMatch(/In progress: 0 of 3 done$/);
  });

  test("the recent imports say how each went; one partly or wholly failed says to import again with “Merge”", async () => {
    await renderApp(importAt, {
      settings: {
        imports: [
          running,
          { ...follows, failed: 3 },
          { ...follows, kind: "bookmarks", total: 14, processed: 14, failed: 14 },
          { ...follows, kind: "domain_mutes", total: 0, processed: 0 },
          follows,
        ],
      },
    });
    const recent = await sectionRead("Recent imports");
    await waitFor(() => within(recent).getByRole("list", { name: "Recent imports" }));

    expect(
      rowsOf(recent, "Recent imports").map((line) => line.replace(/ · Started on .+? (?=In progress|Finished)/, " · ")),
    ).toEqual([
      "Blocked accounts Replace · In progress: 96 of 300 done",
      "Follows Merge · Finished: 117 of 120 done, 3 failed. Import the file again with “Merge” to try the rest.",
      "Bookmarks Merge · Finished: all 14 failed. Import the file again with “Merge” to try them again.",
      "Muted servers Merge · Finished: there was nothing to do",
      "Follows Merge · Finished: all 120 done",
    ]);
  });

  test("with no import in the last days says so", async () => {
    await renderApp(importAt, { settings: { imports: [] } });

    expect(words(await main().findByText("No import in the last 7 days."))).toBe("No import in the last 7 days.");
  });
});

// ---- Delete old posts automatically -------------------------------------------------------------------------------------

/** The field of the age past which posts are deleted. */
const ageField = () => main().getByRole<HTMLInputElement>("textbox", { name: "Delete posts older than" });

/** The submit bar's Save. */
const saveButton = () => main().getAllByRole("button", { name: "Save" }).at(-1) ?? document.body;

/** What the submit bar says beside Save: what describes it. */
const beside = () => describedBy(saveButton());

/** Waits until the screen drew its form and counted what it deletes, and returns the app. */
async function autoDeleteOn(options: FakeOptions): Promise<TestApp> {
  const app = await renderApp(autoDeleteAt, options);
  await sectionRead("When posts are deleted");
  await waitFor(() => {
    if (
      said(section("Posts that would be deleted")).includes("Counting your posts…") ||
      app.data.queryClient.isFetching() > 0
    )
      throw new Error("test: the posts are still being counted");
  });
  return app;
}

/** Writes `days` in the age's field and leaves it. */
async function ageOf(app: TestApp, days: string) {
  await app.user.click(ageField());
  await app.user.clear(ageField());
  if (days !== "") await app.user.type(ageField(), days);
  await app.user.tab();
}

/** Waits until the submit bar says `says` beside Save, or nothing for `[]`. */
const barSays = (says: string[]) =>
  waitFor(() => {
    if (JSON.stringify(beside()) !== JSON.stringify(says))
      throw new Error(`test: the bar says ${JSON.stringify(beside())}`);
  });

/** The estimate of the fake for a policy of `days` that keeps nothing in particular. */
const estimate = (days: number) => Math.max(0, Math.round(1500 / (1 + days / 20)));

describe("Delete old posts automatically: the count beside Save", () => {
  test("says what the settings as they stand delete, then what saving deletes once they change", async () => {
    const app = await autoDeleteOn(deleting(policy));
    await barSays([`These settings delete ${String(estimate(90))} of your posts.`]);

    await ageOf(app, "30");

    await barSays([`Saving deletes ${String(estimate(30))} of your posts.`]);
  });

  test("says that it counts while the count is on its way", async () => {
    const app = await autoDeleteOn(deleting(policy));
    const held = app.fake.script("POST /preferences/auto_delete/preview", { hold: "response" });

    await ageOf(app, "60");

    await held.served();
    await barSays(["Counting your posts…"]);
    held.release();
    await barSays([`Saving deletes ${String(estimate(60))} of your posts.`]);
  });

  test("says nothing while the deletion is off", async () => {
    await autoDeleteOn(deleting({ ...policy, enabled: false }));

    expect(beside()).toEqual([]);
  });

  test("says nothing while no age is written", async () => {
    const app = await autoDeleteOn(deleting(policy));

    await ageOf(app, "");

    await barSays([]);
  });

  test("says nothing when nothing would be deleted", async () => {
    const on = fakeApp(deleting(policy));
    on.fake.script("POST /preferences/auto_delete/preview", {
      change: (body) => ({ ...body, count: 0, examples: [] }),
    });
    await renderApp(autoDeleteAt, { on });
    await sectionRead("When posts are deleted");
    await saying(() => section("Posts that would be deleted"), "None of your posts would be deleted.");

    expect(beside()).toEqual([]);
  });

  test("says nothing when the posts could not be counted", async () => {
    const on = fakeApp(deleting(policy));
    on.fake.script("POST /preferences/auto_delete/preview", { fail: 500 });
    await renderApp(autoDeleteAt, { on });
    await sectionRead("When posts are deleted");
    await within(await sectionRead("Posts that would be deleted")).findByText("Your posts could not be counted.");

    expect(beside()).toEqual([]);
  });
});

/** Presses Save and returns once a confirmation opened, or the save was sent with none. */
async function saved(app: TestApp, put: { seen: readonly unknown[] }) {
  await app.user.click(saveButton());
  await waitFor(() => {
    if (confirmations().length === 0 && put.seen.length === 0) throw new Error("test: Save neither asked nor saved");
  });
}

describe("Delete old posts automatically: Save asks first exactly when the save deletes more than now", () => {
  /** A change of the form, as the reader makes it. */
  type Change = (app: TestApp) => Promise<void>;
  const checkbox = (name: string) => main().getByRole("checkbox", { name });
  const toggled =
    (name: string): Change =>
    (app) =>
      app.user.click(checkbox(name));
  const threshold =
    (name: string, value: string): Change =>
    async (app) => {
      const field = main().getByRole("textbox", { name });
      await app.user.click(field);
      await app.user.clear(field);
      await app.user.type(field, value);
      await app.user.tab();
    };
  const turnedOn: Change = (app) =>
    app.user.selectOptions(main().getByRole("combobox", { name: "Automatic deletion" }), "On");

  const asks: readonly [string, FakeOptions, Change][] = [
    ["the deletion turned on", deleting({ ...policy, enabled: false }), turnedOn],
    ["a younger age", deleting(policy), (app) => ageOf(app, "30")],
    ["a kind no longer kept", deleting(policy), toggled("Posts you bookmarked")],
    ["an audience no longer kept", deleting(policy), toggled("Direct")],
    ["a threshold raised", deleting(policy), threshold("Keep posts favourited at least", "50")],
    ["a threshold set to 0", deleting(policy), threshold("Keep posts favourited at least", "0")],
    ["an account the limit binds, before the limit applies, unchanged", {}, () => Promise.resolve()],
  ];
  for (const [what, options, change] of asks)
    test(`asks on ${what}, and sends nothing before the reader confirms`, async () => {
      const app = await autoDeleteOn(options);
      await change(app);
      await waitFor(() => {
        if (app.data.queryClient.isFetching() > 0) throw new Error("test: still counting");
      });
      const put = app.fake.script("PUT /preferences/auto_delete");

      await saved(app, put);

      const [asking, ...others] = confirmations();
      expect(others).toEqual([]);
      if (asking === undefined) throw new Error("test: Save did not ask");
      expect(within(asking).getByRole("heading").textContent).toBe("Delete your old posts?");
      expect(words(asking)).toMatch(/Saving deletes \d+ of your posts\. This cannot be undone\./);
      expect(buttons(asking).slice(-2)).toEqual(["Cancel", "Save and delete"]);
      await aTurn();
      expect(put.seen).toHaveLength(0);

      await app.user.click(within(asking).getByRole("button", { name: "Save and delete" }));

      await waitFor(() => {
        if (put.seen.length === 0) throw new Error("test: the save was not sent");
      });
      expect(put.seen).toHaveLength(1);
    });

  const saves: readonly [string, FakeOptions, Change][] = [
    ["an older age", deleting(policy), (app) => ageOf(app, "120")],
    ["a kind newly kept", deleting(policy), toggled("Posts you favourited")],
    ["a threshold lowered", deleting(policy), threshold("Keep posts favourited at least", "10")],
    [
      "the deletion turned off",
      deleting(policy),
      (app) => app.user.selectOptions(main().getByRole("combobox", { name: "Automatic deletion" }), "Off"),
    ],
    [
      "a policy within a limit that does not bind it, unchanged",
      deleting(
        { ...policy, keep_bookmarked: false, keep_articles: false, keep_visibilities: [], keep_min_favourites: 0 },
        strict,
      ),
      () => Promise.resolve(),
    ],
  ];
  for (const [what, options, change] of saves)
    test(`saves at once on ${what}`, async () => {
      const app = await autoDeleteOn(options);
      await change(app);
      await waitFor(() => {
        if (app.data.queryClient.isFetching() > 0) throw new Error("test: still counting");
      });
      const put = app.fake.script("PUT /preferences/auto_delete");

      await saved(app, put);

      expect(confirmations()).toHaveLength(0);
      expect(put.seen).toHaveLength(1);
    });

  test("Cancel of the question sends nothing and keeps the change", async () => {
    const app = await autoDeleteOn(deleting(policy));
    await ageOf(app, "30");
    await waitFor(() => {
      if (app.data.queryClient.isFetching() > 0) throw new Error("test: still counting");
    });
    const put = app.fake.script("PUT /preferences/auto_delete");
    await saved(app, put);

    await app.user.click(within(confirmations()[0] ?? document.body).getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      if (confirmations().length > 0) throw new Error("test: the question is still open");
    });
    await aTurn();
    expect(put.seen).toHaveLength(0);
    expect(ageField().value).toBe("30");
  });
});

describe("Delete old posts automatically under a limit of the server", () => {
  test("draws nothing the server forbids as a control: one checkbox of the kinds, one sentence for the others, none of the audiences or the thresholds", async () => {
    await autoDeleteOn(
      deleting(
        { ...policy, keep_bookmarked: false, keep_articles: false, keep_visibilities: [], keep_min_favourites: 0 },
        strict,
      ),
    );

    expect(main().getByRole("combobox", { name: "Automatic deletion" }).hasAttribute("disabled")).toBe(true);
    expect(
      within(section("Posts that are kept"))
        .getAllByRole("checkbox")
        .map((box) => words(box.closest("label") ?? box)),
    ).toEqual(["Your 3 most recently pinned posts"]);
    const forbidden = within(section("Posts that are kept"))
      .queryAllByText(/^This server does not let you keep /)
      .map(words);
    expect(forbidden).toHaveLength(1);
    for (const kind of [
      "posts you bookmarked",
      "posts you favourited",
      "posts with media",
      "posts with a poll",
      "articles",
    ])
      expect(forbidden[0]).toContain(kind);
    expect(within(section("Posts kept by audience")).queryAllByRole("checkbox")).toHaveLength(0);
    expect(words(section("Posts kept by audience"))).toContain(
      "This server does not let you keep posts by their audience.",
    );
    expect(within(section("Posts kept by their boosts and favourites")).queryAllByRole("textbox")).toHaveLength(0);
    expect(words(section("Posts kept by their boosts and favourites"))).toContain(
      "This server does not let you keep posts by their boosts or favourites.",
    );
  });

  test("an account the limit binds is told from when, what changes and how many posts go then, and not the maximum's number", async () => {
    const on = fakeApp();
    const held = await heldBy<{ server_limit: { enforce_from: string; max_age_days: number } }>(
      on.fake,
      "/preferences/auto_delete",
    );
    const app = await renderApp(autoDeleteAt, { on });
    const title = `From ${dayOf(held.server_limit.enforce_from)}, this server's limit applies to your posts`;

    const notice = await sectionRead(title);

    await waitFor(() => {
      if (!/: \d+ of your posts will be deleted then\./.test(words(notice))) throw new Error(`test: ${words(notice)}`);
    });
    expect(words(notice)).toMatch(
      /^From .+, this server's limit applies to your posts\s*You can then no longer keep posts with media: \d+ of your posts will be deleted then\. Saving this page applies the limit now\.$/,
    );
    expect(words(notice)).not.toContain(String(held.server_limit.max_age_days));
    expect(app.path()).toBe(autoDeleteAt);
  });

  test("an empty age is refused under the field, and nothing is sent", async () => {
    const app = await autoDeleteOn(deleting(policy));
    await ageOf(app, "");
    const put = app.fake.script("PUT /preferences/auto_delete");

    await app.user.click(saveButton());

    await waitFor(() => {
      if (!describedBy(ageField()).includes("Enter a whole number of days from 1 to 3,650."))
        throw new Error(`test: the field is described by ${JSON.stringify(describedBy(ageField()))}`);
    });
    await focusOn(ageField);
    await aTurn();
    expect(put.seen).toHaveLength(0);
  });

  // The rule describes the field, which is marked invalid, as the other refused fields of the settings are (an
  // alias's, a move's target): a reader of assistive technology hears it as what is wrong, not as a hint.
  test("an empty age refused marks the field invalid", async () => {
    const app = await autoDeleteOn(deleting(policy));
    await ageOf(app, "");

    await app.user.click(saveButton());
    await waitFor(() => {
      if (!describedBy(ageField()).includes("Enter a whole number of days from 1 to 3,650."))
        throw new Error("test: the rule is not under the field");
    });

    expect(ageField().getAttribute("aria-invalid")).toBe("true");
  });

  // The server's refusal of one member of the save reaches its field, which says the field's rule.
  test("a threshold the server refuses is marked invalid and described by its rule", async () => {
    const app = await autoDeleteOn(deleting(policy));
    const field = () => main().getByRole("textbox", { name: "Keep posts favourited at least" });
    await app.user.click(field());
    await app.user.clear(field());
    await app.user.type(field(), "10");
    await app.user.tab();
    await waitFor(() => {
      if (app.data.queryClient.isFetching() > 0) throw new Error("test: still counting");
    });
    app.fake.script("PUT /preferences/auto_delete", {
      fail: {
        type: "about:blank",
        title: "This could not be saved.",
        status: 422,
        field: "keep_min_favourites",
        detail: "Enter a number from 0 to 1,000.",
      },
    });

    await app.user.click(saveButton());

    await waitFor(() => {
      if (!describedBy(field()).includes("Enter a whole number from 0 to 1,000,000."))
        throw new Error(`test: the field is described by ${JSON.stringify(describedBy(field()))}`);
    });
    expect(field().getAttribute("aria-invalid")).toBe("true");
  });

  test("a change not saved holds the way back until the reader chooses to leave", async () => {
    const app = await autoDeleteOn(deleting(policy));
    await ageOf(app, "120");

    await app.user.click(main().getByRole("button", { name: "Account and data" }));

    const asking = await screen.findByRole("alertdialog", { name: "Leave without saving?" });
    expect(app.path()).toBe(autoDeleteAt);
    expect(buttons(asking).slice(-2)).toEqual(["Keep editing", "Leave"]);

    await app.user.click(within(asking).getByRole("button", { name: "Leave" }));

    await waitFor(() => {
      if (app.path() !== entry) throw new Error(`test: the app is at ${app.path()}`);
    });
  });
});

test("the notification that the server's limit will bind the account (`auto_delete_limit`) leads to Delete old posts automatically", async () => {
  await renderApp("/notifications", { everyKind: "notices" });
  await main().findByRole("feed", { name: "Notifications" });
  const notice = await waitFor(() => {
    const [found] = main()
      .getAllByRole("article")
      .filter((article) => /posts older than 365 days will be deleted automatically/.test(words(article)));
    if (found === undefined) throw new Error("test: no notification of the limit is listed yet");
    return found;
  });

  const [door, ...others] = within(notice).getAllByRole("link");
  expect(others).toEqual([]);
  if (door === undefined) throw new Error("test: the notification leads nowhere");
  expect(door.getAttribute("href")).toBe(autoDeleteAt);
});

// ---- Aliases --------------------------------------------------------------------------------------------------------

/** The field that adds an alias. */
const aliasField = () => main().getByRole<HTMLInputElement>("textbox", { name: "Account you are moving from" });

/** Writes `handle` in the field that adds an alias and presses Add. */
async function add(app: TestApp, handle: string) {
  if (handle !== "") {
    await app.user.click(aliasField());
    await app.user.paste(handle);
  }
  await app.user.click(within(section("Add an alias")).getByRole("button", { name: "Add" }));
}

describe("Aliases", () => {
  test("counts the aliases in the list's title, and Remove acts at once: the row stays, reading Removed", async () => {
    const app = await renderApp(aliasesAt);
    const list = await sectionRead("1 of 5 aliases");
    expect(rowsOf(list, "1 of 5 aliases")).toEqual([
      expect.stringMatching(/^@yuki@old\.example Added on .+Remove$/) as unknown as string,
    ]);
    const removed = app.fake.script("DELETE /accounts/aliases");

    await app.user.click(
      within(within(list).getByRole("group", { name: "@yuki@old.example" })).getByRole("button", { name: "Remove" }),
    );

    await waitFor(() => {
      if (removed.seen.length === 0) throw new Error("test: nothing was removed");
    });
    expect(confirmations()).toHaveLength(0);
    await waitFor(() => {
      const [row] = main().queryAllByRole("listitem").map(words);
      if (row === undefined || !row.includes("Removed")) throw new Error(`test: the row says ${String(row)}`);
    });
    expect(main().queryAllByRole("button", { name: "Remove" })).toHaveLength(0);
  });

  test("an empty field is refused under it, with the focus in it, and nothing is sent", async () => {
    const app = await renderApp(aliasesAt);
    await sectionRead("Add an alias");
    const added = app.fake.script("POST /accounts/aliases");

    await add(app, "");

    await waitFor(() => {
      if (!describedBy(aliasField()).includes("Enter the handle or the address of the account."))
        throw new Error(`test: ${JSON.stringify(describedBy(aliasField()))}`);
    });
    await focusOn(aliasField);
    expect(added.seen).toHaveLength(0);
  });

  test("a handle no account can be an alias at is refused under the field, with the focus in it", async () => {
    const app = await renderApp(aliasesAt);
    await sectionRead("Add an alias");

    await add(app, "nobody");

    await waitFor(() => {
      if (
        !describedBy(aliasField()).some((part) => part.startsWith("No account that can be an alias was found there."))
      )
        throw new Error(`test: ${JSON.stringify(describedBy(aliasField()))}`);
    });
    expect(aliasField().getAttribute("aria-invalid")).toBe("true");
    await focusOn(aliasField);
  });

  test("an alias added empties the field, says so, and is counted", async () => {
    const app = await renderApp(aliasesAt);
    await sectionRead("Add an alias");

    await add(app, "sora@photos.example");

    await saying(() => section("Add an alias"), "The alias was added.");
    expect(aliasField().value).toBe("");
    await sectionRead("2 of 5 aliases");
  });

  test("at five, the field gives way to the sentence of the limit", async () => {
    await renderApp(aliasesAt, {
      settings: { aliases: ["a@one.example", "b@two.example", "c@three.example", "d@four.example", "e@five.example"] },
    });

    await sectionRead("5 of 5 aliases");
    expect(main().queryAllByRole("textbox")).toHaveLength(0);
    expect(main().queryAllByRole("button", { name: "Add" })).toHaveLength(0);
    expect(words(screen.getByRole("main"))).toContain(
      "You have 5 aliases, the most an account can have. Remove one to add another.",
    );
  });
});

// ---- Every screen ---------------------------------------------------------------------------------------------------

for (const at of [entry, exportAt, importAt, autoDeleteAt, aliasesAt, "/settings/import-export"])
  test(`a reader without a session is asked to sign in at ${at}`, async () => {
    await renderApp(at, { visitor: true });

    expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
    expect(main().queryAllByRole("button")).toHaveLength(0);
  });

test("the closure's banner leads to Export and to Move account", async () => {
  await renderApp("/lists", { closure: "announced" });

  const banner = screen.getByRole("region", { name: "Server and connection status" });
  expect(
    within(banner)
      .getAllByRole("link")
      .map((link) => [words(link), link.getAttribute("href")]),
  ).toEqual([
    ["Export your data", exportAt],
    ["Move your account", "/settings/account/move"],
  ]);
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
describe("the screens of the account's data, as the app draws them, keep the rules of a settings screen (ADR-0110 §6)", () => {
  const drawn: readonly [string, string, FakeOptions, () => Promise<unknown>][] = [
    ["the entry", entry, {}, () => main().findByRole("link", { name: /^Export/ })],
    [
      "the entry of an account that moved",
      entry,
      { settings: { movedTo: "sora@photos.example" } },
      () => main().findByRole("link", { name: /^Export/ }),
    ],
    ["Export with no archive", exportAt, {}, () => sectionRead("Archive")],
    [
      "Export with an archive ready",
      exportAt,
      { dataExport: "ready" },
      () => main().findByRole("list", { name: "Archive" }),
    ],
    [
      "Export over an earlier archive",
      exportAt,
      { dataExport: "rebuilding" },
      () => main().findByRole("list", { name: "Archive" }),
    ],
    ["Import", importAt, {}, () => main().findByRole("list", { name: "Recent imports" })],
    [
      "Import while another runs",
      importAt,
      { settings: { imports: [running, follows] } },
      () => main().findByRole("list", { name: "Recent imports" }),
    ],
    ["Delete old posts automatically, bound", autoDeleteAt, {}, () => sectionRead("Posts that are kept")],
    [
      "Delete old posts automatically, off",
      autoDeleteAt,
      deleting({ ...policy, enabled: false }),
      () => sectionRead("Posts that are kept"),
    ],
    [
      "Delete old posts automatically, under a limit",
      autoDeleteAt,
      deleting(
        { ...policy, keep_bookmarked: false, keep_articles: false, keep_visibilities: [], keep_min_favourites: 0 },
        strict,
      ),
      () => sectionRead("Posts that are kept"),
    ],
    ["Aliases", aliasesAt, {}, () => sectionRead("1 of 5 aliases")],
    ["Aliases, none", aliasesAt, { settings: { aliases: [] } }, () => sectionRead("Add an alias")],
    [
      "Aliases, full",
      aliasesAt,
      {
        settings: {
          aliases: ["a@one.example", "b@two.example", "c@three.example", "d@four.example", "e@five.example"],
        },
      },
      () => sectionRead("5 of 5 aliases"),
    ],
  ];
  for (const [what, at, options, read] of drawn)
    test(what, async () => {
      await renderApp(at, options);
      await read();

      expect(settingsFindings(document.body)).toEqual([]);
    });

  test("Import with a file previewed for Replace", async () => {
    const app = await renderApp(importAt);
    await sectionRead("Recent imports");
    await chosen(app);
    await app.user.selectOptions(main().getByRole("combobox", { name: "How to import" }), "Replace");
    await waitFor(() => {
      if (describedBy(importButton()).length === 0) throw new Error("test: Import says nothing of what it removes");
    });

    expect(settingsFindings(document.body)).toEqual([]);
  });
});
