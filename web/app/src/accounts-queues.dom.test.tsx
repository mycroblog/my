// SPDX-License-Identifier: AGPL-3.0-only

// The moderators' queues beside the accounts (#3260, FR-011, FR-713, FR-719, FR-721, FR-931; ADR-0147), on the fake
// API, as what follows a press: the accounts found by name, domain, email, email domain and IP; the registrations
// approved and rejected by their row and in bulk, of the rows that still wait; the sign-up blocks added, added from a
// list and lifted; the invites revoked and created; the decision hooks' decisions cancelled and their appeals decided;
// a voice chat room ended and an event cancelled from their report; the roots' counts; what each permission opens; and
// a one-person server, which has no registrations, sign-up blocks or invites. What each screen shows at rest is the
// stories' (accounts.stories.tsx).

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp, type FakeApp } from "./test/server";

/** Waits until the screen says `sentence`. */
const says = (sentence: string) =>
  waitFor(() => {
    if (!words(screen.getByRole("main")).includes(sentence))
      throw new Error(`test: the screen does not say ${JSON.stringify(sentence)}`);
  });

/** What the facts labelled `label` say, a term and its value joined with no space. */
const facts = (label: string) => words(main().getByLabelText(label));

/** The confirmation named `name` once it is open, whether it is a dialog or an alert dialog. */
const confirmation = (name: string) =>
  waitFor(() => {
    const [found] = [...screen.queryAllByRole("alertdialog", { name }), ...screen.queryAllByRole("dialog", { name })];
    if (found === undefined) throw new Error(`test: no confirmation ${JSON.stringify(name)}`);
    return within(found);
  });

/** Writes `text` into the field `label` of the screen. */
async function write(app: TestApp, label: string, text: string) {
  await app.user.click(main().getByRole("textbox", { name: label }));
  await app.user.paste(text);
}

/** Waits until the app has no read on its way. */
const settled = (on: { data: FakeApp["data"] }) =>
  waitFor(() => {
    if (on.data.queryClient.isFetching() > 0) throw new Error("test: the app's reads are on their way");
  });

/** The row of the table `table` whose header says `head` first. */
const rowOf = (table: string, head: string) => {
  const row = within(main().getByRole("table", { name: table }))
    .queryAllByRole("row")
    .find((r) => within(r).queryAllByRole("rowheader").length > 0 && words(r).startsWith(head));
  if (row === undefined) throw new Error(`test: no row of ${head} in ${table}`);
  return row;
};

/** The button `name` of the screen that acts on the rows selected of the table `table`, not one row's own. */
const forSelected = (table: string, name: string) => {
  const rows = new Set(within(main().getByRole("table", { name: table })).queryAllByRole("button", { name }));
  const [button, ...more] = main()
    .getAllByRole("button", { name })
    .filter((b) => !rows.has(b));
  if (button === undefined || more.length > 0) throw new Error(`test: not one ${name} for the rows selected`);
  return button;
};

/** The addresses the root's list of screens leads to. */
const listed = () =>
  main()
    .queryAllByRole("navigation", { name: "Moderation screens" })
    .flatMap((nav) =>
      within(nav)
        .queryAllByRole("link")
        .map((link) => link.getAttribute("href")),
    );

/** A server whose reader is `viewer` with every permission of theirs but `without`. */
function withoutPermissions(viewer: string, without: readonly string[], options: FakeOptions = {}) {
  const on = fakeApp({ viewer, ...options });
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({
      ...account,
      permissions: account.permissions.filter((permission) => !without.includes(permission)),
    }),
  });
  return on;
}

// The search of accounts.

for (const [by, q, param, found] of [
  ["domain", "photos.example", "domain", "@sora@photos.example"],
  ["email", "kai@demo.test", "email", "@kai"],
  ["email_domain", "demo.test", "email_domain", "@kai"],
  ["ip", "203.0.113.24", "ip", "@ren"],
] as const)
  test(`the accounts found by ${by} are asked of the server by ${param} and listed`, async () => {
    const on = fakeApp({ viewer: "marco" });
    const searched = on.fake.script("GET /admin/accounts");
    await renderApp(`/moderation/accounts?by=${by}&q=${encodeURIComponent(q)}`, { on });

    await waitFor(() => {
      if (!searched.seen.some((seen) => seen.url.searchParams.get(param) === q))
        throw new Error(`test: no search by ${param}`);
    });
    await waitFor(() => {
      rowOf("Accounts", found);
    });
    for (const seen of searched.seen) expect(seen.url.searchParams.get("username")).toBeNull();
  });

test("choosing what the search finds by names the field after it and keeps it in the address", async () => {
  const app = await renderApp("/moderation/accounts", { viewer: "marco" });

  await app.user.click(main().getByRole("button", { name: "Search by" }));
  await app.user.click(screen.getByRole("menuitemradio", { name: "Email domain" }));

  const box = await main().findByRole("searchbox", { name: "Search by email domain" });
  await app.user.type(box, "demo.test");
  await waitFor(() => {
    if (!app.path().includes("by=email_domain") || !app.path().includes("q=demo.test"))
      throw new Error(`test: the address is ${app.path()}`);
  });
});

// Registrations.

test("a registration is rejected by its row's Reject with a note, after a confirmation, and only it reaches the server", async () => {
  const on = fakeApp({ viewer: "marco" });
  const app = await renderApp("/moderation/registrations", { on });
  await main().findByRole("table", { name: "Registrations" });
  const rejected = app.fake.script("POST /admin/registrations/{id}/reject");
  const note = "The garden is two towns away.";

  await app.user.click(within(rowOf("Registrations", "@newleaf")).getByRole("button", { name: "Reject" }));
  const asking = await confirmation("Reject 1 registration?");
  await app.user.click(asking.getByRole("textbox"));
  await app.user.paste(note);
  await app.user.click(asking.getByRole("button", { name: "Reject" }));

  const { url, body } = await rejected.arrived();
  expect(url.pathname).toEndWith(`/admin/registrations/${app.fake.moderation.id("registration:2")}/reject`);
  expect(body?.note).toBe(note);
  await says("1 registration rejected.");
  await waitFor(() => {
    if (!words(rowOf("Registrations", "@newleaf")).includes("Rejected"))
      throw new Error("test: the row does not say it is rejected");
  });
  expect(rejected.seen).toHaveLength(1);
});

test("the registrations selected are approved together, after one confirmation, each reaching the server", async () => {
  const app = await renderApp("/moderation/registrations", { viewer: "marco" });
  await main().findByRole("table", { name: "Registrations" });
  const approved = app.fake.script("POST /admin/registrations/{id}/approve");

  await app.user.click(main().getByRole("checkbox", { name: "Select @newleaf" }));
  await app.user.click(main().getByRole("checkbox", { name: "Select @breadandmountainsandlongwalks" }));
  expect(await main().findByText("2 selected")).toBeDefined();
  await app.user.click(forSelected("Registrations", "Approve"));
  await app.user.click((await confirmation("Approve 2 registrations?")).getByRole("button", { name: "Approve" }));

  await says("2 registrations approved.");
  expect(approved.seen.map((seen) => seen.url.pathname.split("/").at(-2)).toSorted()).toEqual(
    [app.fake.moderation.id("registration:1"), app.fake.moderation.id("registration:2")].toSorted(),
  );
});

test("a registration decided by its row can no longer be selected: Select all and the bulk Reject reach only those that wait", async () => {
  const app = await renderApp("/moderation/registrations", { viewer: "marco" });
  await main().findByRole("table", { name: "Registrations" });

  await app.user.click(within(rowOf("Registrations", "@x9shop")).getByRole("button", { name: "Approve" }));
  await app.user.click((await confirmation("Approve 1 registration?")).getByRole("button", { name: "Approve" }));
  await says("1 registration approved.");
  await waitFor(() => {
    if (within(rowOf("Registrations", "@x9shop")).queryAllByRole("checkbox").length > 0)
      throw new Error("test: the approved row can still be selected");
  });

  const rejected = app.fake.script("POST /admin/registrations/{id}/reject");
  const [all] = main().getAllByRole("checkbox", { name: "Select all" });
  if (all === undefined) throw new Error("test: no Select all");
  await app.user.click(all);
  expect(await main().findByText("2 selected")).toBeDefined();
  await app.user.click(forSelected("Registrations", "Reject"));
  await app.user.click((await confirmation("Reject 2 registrations?")).getByRole("button", { name: "Reject" }));

  await says("2 registrations rejected.");
  expect(rejected.seen.map((seen) => seen.url.pathname.split("/").at(-2)).toSorted()).toEqual(
    [app.fake.moderation.id("registration:1"), app.fake.moderation.id("registration:2")].toSorted(),
  );
});

test("the root counts the registrations that wait, and counts one fewer once one is approved", async () => {
  const on = fakeApp({ viewer: "marco" });
  const waiting = on.fake.moderation.pendingRegistrations();
  const appeals = on.fake.moderation.pendingAppeals();
  const app = await renderApp("/moderation", { on });

  expect(
    await main().findByRole("link", { name: new RegExp(`^Registrations .*\\b${String(waiting)} registrations wait$`) }),
  ).toBeDefined();
  expect(
    main().getByRole("link", { name: new RegExp(`^Appeals .*\\b${String(appeals)} appeals wait$`) }),
  ).toBeDefined();

  await app.navigate("/moderation/registrations");
  await main().findByRole("table", { name: "Registrations" });
  await app.user.click(within(rowOf("Registrations", "@x9shop")).getByRole("button", { name: "Approve" }));
  await app.user.click((await confirmation("Approve 1 registration?")).getByRole("button", { name: "Approve" }));
  await says("1 registration approved.");
  await app.navigate("/moderation");

  expect(
    await main().findByRole("link", {
      name: new RegExp(`^Registrations .*\\b${String(waiting - 1)} registrations wait$`),
    }),
  ).toBeDefined();
});

// A one-person server.

const oneServerScreens = [
  ["/moderation/registrations", "GET /admin/registrations"],
  ["/moderation/signup-blocks", "GET /admin/signup_blocks"],
  ["/moderation/invites", "GET /admin/invites"],
] as const;

test("a one-person server's Moderation lists no registrations, sign-up blocks or invites, and their addresses name no screen", async () => {
  const on = fakeApp({ viewer: "hana", singleUser: true });
  const app = await renderApp("/moderation", { on });

  await main().findByRole("navigation", { name: "Moderation screens" });
  expect(listed()).toContain("/moderation/accounts");
  for (const [href] of oneServerScreens) expect([href, listed().includes(href)]).toEqual([href, false]);
  for (const [path] of oneServerScreens) {
    await app.navigate(path);
    expect(await main().findByText("This address leads to no screen open to you.", { exact: false })).toBeDefined();
    expect(main().queryAllByRole("table")).toHaveLength(0);
  }
});

for (const [path, read] of oneServerScreens)
  test(`${path}, on a one-person server, asks the server for nothing it cannot have`, async () => {
    const on = fakeApp({ viewer: "hana", singleUser: true });
    const asked = on.fake.script(read);
    await renderApp(path, { on });

    await main().findByText("This address leads to no screen open to you.", { exact: false });
    await settled(on);
    expect(asked.seen).toHaveLength(0);
  });

// Sign-up blocks.

test("a sign-up block of an IP range is added with what it does, its expiry and its comment, and opens once the server took it", async () => {
  const on = fakeApp({ viewer: "marco" });
  const app = await renderApp("/moderation/signup-blocks/new", { on });
  await main().findByRole("heading", { level: 1, name: "Add a block" });
  const added = app.fake.script("POST /admin/signup_blocks");

  await app.user.selectOptions(main().getByRole("combobox", { name: "Matches" }), "IP range");
  await app.user.click(main().getByRole("textbox", { name: "IP range" }));
  await app.user.paste("198.51.100.0/24");
  await app.user.selectOptions(main().getByRole("combobox", { name: "Does" }), "Hold it for approval");
  await app.user.selectOptions(main().getByRole("combobox", { name: "Lasts" }), "30 days");
  await write(app, "Comment for the moderators", "A provider the spam accounts moved to.");
  await app.user.click(main().getByRole("button", { name: "Add the block" }));

  const { body } = await added.arrived();
  expect(body).toEqual({
    kind: "ip_range",
    value: "198.51.100.0/24",
    severity: "require_approval",
    expires_in: 30 * 24 * 60 * 60,
    comment: "A provider the spam accounts moved to.",
  });
  expect(await main().findByRole("heading", { level: 1, name: "198.51.100.0/24" })).toBeDefined();
  expect(app.path()).toMatch(/^\/moderation\/signup-blocks\/[^/]+$/);
});

test("sign-up blocks added from a list say how many were added and why each skipped line was skipped", async () => {
  const app = await renderApp("/moderation/signup-blocks/import", { viewer: "marco" });
  await main().findByRole("heading", { level: 1, name: "Add from a list" });
  const imported = app.fake.script("POST /admin/signup_blocks/import");

  await write(app, "List", "a.example\na.example\nnot a domain\nspam.example");
  await app.user.click(main().getByRole("button", { name: "Add the blocks" }));

  const { body } = await imported.arrived();
  expect([body.kind, body.text, body.severity]).toEqual([
    "email_domain",
    "a.example\na.example\nnot a domain\nspam.example",
    "reject",
  ]);
  expect(await main().findByRole("heading", { name: "1 block added" })).toBeDefined();
  const skipped = within(main().getByRole("table", { name: "3 lines skipped" }));
  const [, ...lines] = skipped.getAllByRole("row");
  expect(lines).toHaveLength(3);
  for (const [i, pattern] of [
    /^Line 2.*a\.example.*Repeats an earlier line$/,
    /^Line 3.*not a domain.*Not valid for its kind$/,
    /^Line 4.*spam\.example.*Already blocked$/,
  ].entries())
    expect(words(lines[i] ?? document.body)).toMatch(pattern);
});

test("a sign-up block in effect is lifted, the server told, and is no longer offered to be lifted", async () => {
  const on = fakeApp({ viewer: "marco" });
  const id = on.fake.moderation.id("block:range");
  const app = await renderApp(`/moderation/signup-blocks/${id}`, { on });
  await main().findByRole("heading", { level: 1, name: "192.0.2.0/24" });
  const lifted = app.fake.script("POST /admin/signup_blocks/{id}/lift");

  await app.user.click(main().getByRole("button", { name: "Lift" }));

  expect((await lifted.arrived()).url.pathname).toEndWith(`/admin/signup_blocks/${id}/lift`);
  await says("The sign-up block is lifted.");
  expect(facts("The sign-up block")).toMatch(/State\s*Lifted/);
  expect(main().queryAllByRole("button", { name: "Lift" })).toHaveLength(0);
});

// Invites.

test("an invite selected is revoked after a confirmation, the server told, and can no longer be selected", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.moderation.id("invite:staff");
  const app = await renderApp("/moderation/invites", { on });
  await main().findByRole("table", { name: "Invites" });
  const revoked = app.fake.script("POST /admin/invites/{id}/revoke");

  await app.user.click(main().getByRole("checkbox", { name: "Select BREADSWAP7K2M9QX" }));
  await app.user.click(main().getByRole("button", { name: "Revoke" }));
  await app.user.click((await confirmation("Revoke 1 invite?")).getByRole("button", { name: "Revoke" }));

  expect((await revoked.arrived()).url.pathname).toEndWith(`/admin/invites/${id}/revoke`);
  await says("1 invite revoked.");
  await waitFor(() => {
    const row = rowOf("Invites", "BREADSWAP7K2M9QX");
    if (!words(row).includes("Revoked") || within(row).queryAllByRole("checkbox").length > 0)
      throw new Error("test: the invite is not shown revoked");
  });
  expect(revoked.seen).toHaveLength(1);
});

test("an invite is created with how many may sign up and how long it works, and its link is shown", async () => {
  const app = await renderApp("/moderation/invites/new", { viewer: "hana" });
  await main().findByRole("heading", { level: 1, name: "Create an invite" });
  const created = app.fake.script("POST /admin/invites");

  await write(app, "How many may sign up (empty for no limit)", "5");
  await app.user.selectOptions(main().getByRole("combobox", { name: "Works for" }), "30 days");
  await app.user.click(main().getByRole("button", { name: "Create" }));

  const { body } = await created.arrived();
  expect([body?.max_uses, body?.expires_in]).toEqual([5, 30 * 24 * 60 * 60]);
  expect(await main().findByRole("heading", { name: "The invite is created" })).toBeDefined();
  expect(words(main().getByRole("list", { name: "Sign-up link" }))).toMatch(
    /^https:\/\/demo\.test\/auth\/sign_up\?invite=[A-Z0-9]{16}$/,
  );
});

// The decision hooks.

test("a decision of a hook is cancelled after a confirmation, the server told", async () => {
  const on = fakeApp({ viewer: "marco" });
  const id = on.fake.moderation.id("decision:drop");
  const app = await renderApp(`/moderation/hook-decisions/${id}`, { on });
  await main().findByRole("heading", { level: 1 });
  const cancelled = app.fake.script("POST /admin/hook_decisions/{id}/override");

  await app.user.click(main().getByRole("button", { name: "Cancel the decision" }));
  const asking = await confirmation("Cancel this decision?");
  expect(cancelled.seen).toHaveLength(0);
  await app.user.click(asking.getByRole("button", { name: "Cancel the decision" }));

  expect((await cancelled.arrived()).url.pathname).toEndWith(`/admin/hook_decisions/${id}/override`);
  await says("The decision is cancelled.");
  expect(facts("The decision")).toMatch(/State\s*Cancelled/);
});

for (const [decision, path, state] of [
  ["Uphold", "uphold", "Upheld"],
  ["Overturn", "overturn", "Overturned"],
] as const)
  test(`an appeal of a refused post is decided by "${decision}" after a confirmation, the server told`, async () => {
    const on = fakeApp({ viewer: "marco" });
    const id = on.fake.moderation.id("hook-appeal:kai");
    const app = await renderApp(`/moderation/hook-decisions/appeals/${id}`, { on });
    await main().findByRole("heading", { level: 1, name: "Refused post of @kai" });
    const decided = app.fake.script(`POST /admin/hook_appeals/{id}/${path}`);

    await app.user.click(main().getByRole("button", { name: decision }));
    await app.user.click(
      (await confirmation(`${decision} the original decision?`)).getByRole("button", { name: decision }),
    );

    expect((await decided.arrived()).url.pathname).toEndWith(`/admin/hook_appeals/${id}/${path}`);
    await waitFor(() => {
      if (!new RegExp(`State\\s*${state}`).test(facts("The appeal")))
        throw new Error(`test: the appeal is not ${state}`);
    });
  });

// A room and an event, from their report.

test("a voice chat room still open is ended for everyone from its report, with the moderators' reason", async () => {
  const on = fakeApp({ viewer: "marco" });
  const app = await renderApp(`/moderation/reports/${on.fake.reports.id("live")}`, { on });
  await main().findByRole("heading", { level: 1, name: /^Report on @/ });
  const ended = app.fake.script("POST /admin/voice_rooms/{id}/end");

  await app.user.click(main().getByRole("button", { name: "End the room" }));
  const asking = await confirmation("End this room for everyone?");
  await app.user.click(asking.getByRole("textbox", { name: "Reason (optional)" }));
  await app.user.paste("Shouting at the guests.");
  await app.user.click(asking.getByRole("button", { name: "End the room" }));

  const { url, body } = await ended.arrived();
  expect(url.pathname).toMatch(/\/admin\/voice_rooms\/[^/]+\/end$/);
  expect(body?.reason).toBe("Shouting at the guests.");
  await waitFor(() => {
    if (!/How it ended\s*By a moderator/.test(words(screen.getByRole("main"))))
      throw new Error("test: the report does not say a moderator ended the room");
  });
  expect(main().queryAllByRole("button", { name: "End the room" })).toHaveLength(0);
});

test("an event is cancelled for everyone from its report, with the moderators' reason", async () => {
  const on = fakeApp({ viewer: "marco" });
  const app = await renderApp(`/moderation/reports/${on.fake.reports.id("event")}`, { on });
  await main().findByRole("heading", { level: 1, name: /^Report on @/ });
  const cancelled = app.fake.script("POST /admin/events/{id}/cancel");

  await app.user.click(main().getByRole("button", { name: "Cancel the event" }));
  const asking = await confirmation("Cancel this event for everyone?");
  await app.user.click(asking.getByRole("textbox", { name: "Reason (optional)" }));
  await app.user.paste("A shop, not a swap.");
  await app.user.click(asking.getByRole("button", { name: "Cancel the event" }));

  const { url, body } = await cancelled.arrived();
  expect(url.pathname).toMatch(/\/admin\/events\/[^/]+\/cancel$/);
  expect(body?.reason).toBe("A shop, not a swap.");
  await waitFor(() => {
    if (!/Cancelled\s*By a moderator/.test(words(screen.getByRole("main"))))
      throw new Error("test: the report does not say a moderator cancelled the event");
  });
  expect(main().queryAllByRole("button", { name: "Cancel the event" })).toHaveLength(0);
});

// What each permission opens.

for (const [without, gone, kept] of [
  [
    "manage_accounts",
    ["/moderation/accounts", "/moderation/appeals", "/moderation/registrations", "/moderation/signup-blocks"],
    ["/moderation/reports", "/moderation/hook-decisions", "/moderation/invites"],
  ],
  ["manage_invites", ["/moderation/invites"], ["/moderation/accounts", "/moderation/registrations"]],
  [
    "manage_reports",
    ["/moderation/reports", "/moderation/hook-decisions"],
    ["/moderation/accounts", "/moderation/invites"],
  ],
] as const)
  test(`Moderation lists none of the screens ${without} opens to a reader without it, and the others`, async () => {
    await renderApp("/moderation", { on: withoutPermissions("hana", [without]) });

    await main().findByRole("navigation", { name: "Moderation screens" });
    for (const href of gone) expect([href, listed().includes(href)]).toEqual([href, false]);
    for (const href of kept) expect([href, listed().includes(href)]).toEqual([href, true]);
  });

test("the accounts' address, for a reader without manage_accounts, lists no account and asks the server for none", async () => {
  const on = withoutPermissions("marco", ["manage_accounts"]);
  const searched = on.fake.script("GET /admin/accounts");
  await renderApp("/moderation/accounts", { on });

  await main().findByRole("heading", { level: 1 });
  await settled(on);
  expect(main().queryAllByRole("table", { name: "Accounts" })).toHaveLength(0);
  expect(searched.seen).toHaveLength(0);
});

test("a report read by a holder of manage_reports without manage_accounts offers no way to act on the account, and still cancels the event", async () => {
  const on = withoutPermissions("marco", ["manage_accounts"]);
  await renderApp(`/moderation/reports/${on.fake.reports.id("event")}`, { on });

  await main().findByRole("heading", { level: 1, name: /^Report on @/ });
  await settled(on);
  expect(main().queryAllByRole("link", { name: /^Act on this account/ })).toHaveLength(0);
  expect(main().getByRole("button", { name: "Cancel the event" })).toBeDefined();
});
