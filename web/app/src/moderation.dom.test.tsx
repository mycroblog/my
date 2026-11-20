// SPDX-License-Identifier: AGPL-3.0-only

// The staff's areas and the handling of a report (ADR-0058 §2, ADR-0148; FR-709–FR-712, FR-727, #3259), on the fake
// API, as what follows a press: the roots list the screens the reader's permissions open, and Reports only to a holder
// of manage_reports; the count of Moderation in the navigation leads to the open reports; and a moderator takes a
// report and leaves it, writes a note, translates its comment and its post, forwards a remote one once, resolves it
// with and without an action, and reopens it. What each screen shows at rest is the stories' (moderation.stories.tsx).

import { expect, onTestFinished, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words } from "./test/app";
import { fakeApp } from "./test/server";

/** The app at the report of the fake's `key`, as marco, the scenario's moderator, reads it on a server of `options`. */
async function atReport(key: string, options: FakeOptions = {}) {
  const on = fakeApp({ viewer: "marco", ...options });
  const id = on.fake.reports.id(key);
  const app = await renderApp(`/moderation/reports/${id}`, { on });
  await main().findByRole("heading", { level: 1, name: /^Report on @/ });
  return { app, id };
}

/** The fake's report of `key` as marco reads it, its posts written in French, a language the reader does not read. */
async function atReportInFrench(key: string, options: FakeOptions) {
  const on = fakeApp({ viewer: "marco", ...options });
  on.fake.script("GET /admin/reports/{id}", {
    change: (report) => ({
      ...report,
      statuses: report.statuses.map((s) => ({ ...s, status: { ...s.status, language: "fr" } })),
    }),
  });
  const id = on.fake.reports.id(key);
  const app = await renderApp(`/moderation/reports/${id}`, { on });
  await main().findByRole("heading", { level: 1, name: /^Report on @/ });
  return { app, id };
}

/** The button of the report's screen named `name`, once it shows. */
const button = (name: string) => main().findByRole("button", { name });

/** Waits until the report's screen says `sentence`. */
const says = (sentence: string) =>
  waitFor(() => {
    if (!words(screen.getByRole("main")).includes(sentence))
      throw new Error(`test: the screen does not say ${JSON.stringify(sentence)}`);
  });

/** The facts of the report: its state, its category, who filed it and when. */
const facts = () => main().getByLabelText("About the report");

/** The links of the root's list of screens, each as its address, none when the root lists none. */
const listed = () => {
  const list = main().queryAllByRole("navigation", { name: "Moderation screens" });
  return list.flatMap((nav) =>
    within(nav)
      .queryAllByRole("link")
      .map((link) => link.getAttribute("href")),
  );
};

/** Moderation in the main navigation, with the words of what waits there; none for a reader who may not moderate. */
const moderationPlaces = () => screen.queryAllByRole("link", { name: /^Moderation(?:\s+\d+ waiting)?$/ });

// The roots, and who sees Reports in them.

test("Moderation lists Reports to a holder of manage_reports, with how many are open, and leads to the queue", async () => {
  const on = fakeApp({ viewer: "marco" });
  const open = on.fake.reports.openCount();
  const app = await renderApp("/moderation", { on });

  const reports = await within(await main().findByRole("navigation", { name: "Moderation screens" })).findByRole(
    "link",
    { name: new RegExp(`^Reports .*\\b${String(open)} open reports$`) },
  );
  expect(listed()).toContain("/moderation/reports");
  await app.user.click(reports);
  await app.arrived("/moderation/reports");
  expect(await main().findByRole("table", { name: "Reports" })).toBeDefined();
});

test("a reader with other moderation permissions but not manage_reports is not shown Reports in the root's list", async () => {
  const on = fakeApp({ viewer: "marco" });
  // Permissions that open none of the screens built yet: neither the reports nor the accounts' screens.
  const opening = new Set(["manage_reports", "manage_accounts", "manage_invites"]);
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({
      ...account,
      permissions: account.permissions.filter((permission) => !opening.has(permission)),
    }),
  });
  await renderApp("/moderation", { on });

  await main().findByRole("navigation", { name: "Moderation screens" });
  expect(listed()).toEqual(["/moderation/domain-actions", "/moderation/instances", "/moderation/audit-log"]);
  expect(main().queryAllByRole("link", { name: /^Reports/ })).toHaveLength(0);
});

test("a reader without any moderation permission is told why Moderation opens no screen, and Administration why it opens none", async () => {
  const app = await renderApp("/moderation", { viewer: "yuki" });

  expect(
    await main().findByText("Your account has no moderation permissions, so no moderation screen is open to you."),
  ).toBeDefined();
  expect(listed()).toEqual([]);
  expect(moderationPlaces()).toHaveLength(0);

  await app.navigate("/admin");
  expect(
    await main().findByText(
      "Your account has no server administration permissions, so no administration screen is open to you.",
    ),
  ).toBeDefined();
});

test("the queue's address, for a reader who may not handle reports, shows no report and asks the server for none", async () => {
  const on = fakeApp({ viewer: "yuki" });
  const queue = on.fake.script("GET /admin/reports");
  await renderApp("/moderation/reports", { on });

  await main().findByRole("heading", { level: 1 });
  expect(main().queryAllByRole("table", { name: "Reports" })).toHaveLength(0);
  expect(queue.seen).toHaveLength(0);
});

test("the counts of what waits for moderators are asked for only by a holder of manage_reports", async () => {
  for (const [viewer, asks] of [
    ["yuki", false],
    ["marco", true],
  ] as const) {
    const on = fakeApp({ viewer });
    const counts = on.fake.script("GET /admin/counts");
    await renderApp("/lists", { on });
    await main().findByRole("heading", { level: 1, name: "Lists" });
    await waitFor(() => {
      if (on.data.queryClient.isFetching() > 0) throw new Error("test: the app's reads are on their way");
    });
    expect([viewer, counts.seen.length > 0]).toEqual([viewer, asks]);
    expect([viewer, moderationPlaces().length > 0]).toEqual([viewer, asks]);
    cleanup();
  }
});

// The count of the navigation.

test("Moderation in the navigation counts what waits (the open reports, the registrations and the appeals) and leads, through the root, to the queue of the open reports", async () => {
  const on = fakeApp({ viewer: "marco" });
  const open = on.fake.reports.openCount();
  const waiting = open + on.fake.moderation.pendingRegistrations() + on.fake.moderation.pendingAppeals();
  const app = await renderApp("/lists", { on });

  const [place] = await waitFor(() => {
    const found = screen.queryAllByRole("link", { name: new RegExp(`^Moderation\\s+${String(waiting)} waiting$`) });
    if (found.length === 0) throw new Error("test: the navigation counts nothing in Moderation yet");
    return found;
  });
  if (place === undefined) throw new Error("test: no Moderation in the navigation");
  await app.user.click(place);
  await app.arrived("/moderation");
  await app.user.click(
    await main().findByRole("link", { name: new RegExp(`^Reports .*\\b${String(open)} open reports$`) }),
  );
  await app.arrived("/moderation/reports");

  const table = await main().findByRole("table", { name: "Reports" });
  await waitFor(() => {
    if (within(table).queryAllByRole("rowheader").length !== open)
      throw new Error(`test: the queue lists ${String(within(table).queryAllByRole("rowheader").length)} reports`);
  });
  const answer = await on.data.api.GET("/admin/reports", { params: { query: { state: "open" } } });
  const shown = within(table)
    .getAllByRole("rowheader")
    .map((cell) => within(cell).getByRole("link").getAttribute("href"));
  expect(shown).toEqual((answer.data?.items ?? []).map((report) => `/moderation/reports/${report.id}`));
});

// A report through its states.

test("a moderator takes a report nobody handles, the server told, and leaves it to nobody again", async () => {
  const { app, id } = await atReport("deals");
  const taken = app.fake.script("POST /admin/reports/{id}/assign_to_self");
  const left = app.fake.script("POST /admin/reports/{id}/unassign");

  await app.user.click(await button("Assign to me"));

  expect((await taken.arrived()).url.pathname).toEndWith(`/admin/reports/${id}/assign_to_self`);
  await says("You handle this report now.");
  await app.user.click(await button("Unassign"));

  expect((await left.arrived()).url.pathname).toEndWith(`/admin/reports/${id}/unassign`);
  expect(await button("Assign to me")).toBeDefined();
  expect(main().queryAllByRole("button", { name: "Unassign" })).toHaveLength(0);
});

test("a note a moderator writes reaches the server and joins the report's notes", async () => {
  const { app, id } = await atReport("deals");
  const sent = app.fake.script("POST /admin/reports/{id}/notes");
  const note = "Checked the profile: every post links the same shop.";

  await app.user.click(main().getByRole("textbox", { name: "New note" }));
  await app.user.paste(note);
  await app.user.click(main().getByRole("button", { name: "Add note" }));

  const { url, body } = await sent.arrived();
  expect(url.pathname).toEndWith(`/admin/reports/${id}/notes`);
  expect(body.body).toBe(note);
  const notes = await main().findByRole("list", { name: "Notes" });
  await waitFor(() => {
    if (
      !within(notes)
        .queryAllByRole("listitem")
        .some((item) => words(item).includes(note))
    )
      throw new Error("test: the notes do not hold the new one");
  });
});

test("where the server may send a report's text, the comment translates into the reader's language through it", async () => {
  const { app, id } = await atReport("photo", { translation: { provider: "answers", reports: true } });
  const asked = app.fake.script("POST /admin/reports/{id}/comment/translation");
  const original = "この写真は私が撮ったものです。クレジットなしで転載されています。";

  await app.user.click(await button("Translate"));

  expect(await main().findByText(`[en] ${original}`)).toBeDefined();
  const { url, body } = await asked.arrived();
  expect(url.pathname).toEndWith(`/admin/reports/${id}/comment/translation`);
  expect(body.target_language).toBe("en");
});

test("where the server may send a report's text, a reported post translates into the reader's language through it", async () => {
  const text = "Developed the first roll from the coast. The grain came out just right.";
  const { app, id } = await atReportInFrench("photo", { translation: { provider: "answers", reports: true } });
  const asked = app.fake.script("POST /admin/reports/{id}/statuses/{status_id}/translation");
  const post = within(await postSaying(text));

  await app.user.click(await post.findByRole("button", { name: "Translate" }));

  expect(await post.findByText(`[en] ${text}`)).toBeDefined();
  const { url, body } = await asked.arrived();
  expect(url.pathname).toMatch(new RegExp(`/admin/reports/${id}/statuses/[^/]+/translation$`));
  expect(body.target_language).toBe("en");
});

test("where the server may not send a report's text but the browser translates, a reported post translates in the browser, and nothing reaches the server", async () => {
  browserTranslator();
  const text = "Developed the first roll from the coast. The grain came out just right.";
  const { app } = await atReportInFrench("photo", {
    translation: { provider: "answers", browser: true, reports: false },
  });
  const comment = app.fake.script("POST /admin/reports/{id}/comment/translation");
  const status = app.fake.script("POST /admin/reports/{id}/statuses/{status_id}/translation");
  const post = within(await postSaying(text));

  await app.user.click(await post.findByRole("button", { name: "Translate" }));

  expect(await post.findByText(`[en] ${text}`)).toBeDefined();
  expect([comment.seen.length, status.seen.length]).toEqual([0, 0]);
});

test("where the server may not send a report's text, its comment offers no translation, even where the browser translates", async () => {
  browserTranslator();
  const { app } = await atReport("photo", { translation: { provider: "answers", browser: true, reports: false } });

  await main().findByText("この写真は私が撮ったものです。クレジットなしで転載されています。");
  await waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });
  expect(main().queryAllByRole("button", { name: "Translate" })).toHaveLength(0);
});

test("a remote account's report is sent to its server once, after a confirmation naming the server, and is no longer offered", async () => {
  const { app, id } = await atReport("photo");
  const forwarded = app.fake.script("POST /admin/reports/{id}/forward");

  await app.user.click(await button("Send to their server…"));
  const asking = within(await screen.findByRole("alertdialog", { name: "Send this report to photos.example?" }));
  await app.user.click(asking.getByRole("button", { name: "Send" }));

  expect((await forwarded.arrived()).url.pathname).toEndWith(`/admin/reports/${id}/forward`);
  await waitFor(() => {
    if (main().queryAllByRole("button", { name: "Send to their server…" }).length > 0)
      throw new Error("test: the report still offers to be sent");
  });
  expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  expect(forwarded.seen).toHaveLength(1);
});

for (const [choice, outcome] of [
  ["Action taken", "actioned"],
  ["No action needed", "dismissed"],
] as const)
  test(`a moderator resolves an open report as "${choice}", the server told, and may reopen it`, async () => {
    const { app, id } = await atReport("deals");
    const resolved = app.fake.script("POST /admin/reports/{id}/resolve");

    await app.user.click(await button("Resolve"));
    await app.user.click(
      within(await screen.findByRole("menu", { name: "How was it resolved?" })).getByRole("menuitem", {
        name: choice,
      }),
    );

    const { url, body } = await resolved.arrived();
    expect(url.pathname).toEndWith(`/admin/reports/${id}/resolve`);
    expect(body.outcome).toBe(outcome);
    await says("The report is resolved.");
    expect(await button("Reopen")).toBeDefined();
    expect(main().queryAllByRole("button", { name: "Resolve" })).toHaveLength(0);
    if (outcome === "dismissed") expect(words(facts())).toContain("Resolved, no action needed");
  });

test("a moderator reopens a resolved report, the server told, and may resolve it again", async () => {
  const { app, id } = await atReport("review");
  const reopened = app.fake.script("POST /admin/reports/{id}/reopen");

  await app.user.click(await button("Reopen"));

  expect((await reopened.arrived()).url.pathname).toEndWith(`/admin/reports/${id}/reopen`);
  await says("The report is open again.");
  expect(await button("Resolve")).toBeDefined();
  expect(main().queryAllByRole("button", { name: "Reopen" })).toHaveLength(0);
});

test("a report on the reader's own account does not name who filed it, where another report does", async () => {
  const on = fakeApp({ viewer: "marco" });
  const own = on.fake.reports.file("sam", { account_id: on.fake.accountId("marco"), category: "spam", forward: false });
  const app = await renderApp(`/moderation/reports/${on.fake.reports.id("deals")}`, { on });
  await main().findByRole("heading", { level: 1, name: "Report on @quickdeals" });
  expect(words(facts())).toMatch(/Reported by\s*@sam\s*Handled by/);

  await app.navigate(`/moderation/reports/${own}`);

  await main().findByRole("heading", { level: 1, name: "Report on @marco" });
  expect(words(facts())).not.toMatch(/Reported by/);
  expect(words(screen.getByRole("main"))).not.toMatch(/@sam(?![a-z0-9_])|Sam Rivera/);
});

/** The reported post of the report's screen that says `text`. */
const postSaying = (text: string) =>
  waitFor(() => {
    const found = main()
      .getAllByRole("article")
      .filter((post) => words(post).includes(text));
    const [post] = found;
    if (post === undefined || found.length !== 1) throw new Error(`test: ${String(found.length)} posts say ${text}`);
    return post;
  });

/**
 * Gives the browser a translator that has every pair at hand and puts the language it translates into before each
 * text, as the server's fake provider does, until the test ends.
 */
function browserTranslator() {
  Object.defineProperty(globalThis, "Translator", {
    configurable: true,
    value: {
      availability: () => Promise.resolve("available"),
      create: ({ targetLanguage }: { targetLanguage: string }) =>
        Promise.resolve({
          translate: (text: string) => Promise.resolve(`[${targetLanguage}] ${text}`),
          destroy: () => undefined,
        }),
    },
  });
  onTestFinished(() => {
    Reflect.deleteProperty(globalThis, "Translator");
  });
}
