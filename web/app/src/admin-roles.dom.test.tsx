// SPDX-License-Identifier: AGPL-3.0-only

// The roles in the app (#3263, FR-720, ADR-0070 §8), on the fake API: what FR-720 forbids is not offered, not only
// refused — a place above the reader's highest role, a permission the reader lacks, the built-in roles' changes — for a
// community lead below the administrators (hana holding `lead` alone, place 500) and for the administrator; a role is
// created, changed, moved and deleted by its name typed; a role is granted and revoked on an account's Management page,
// and revoking one's own asks first. What each screen shows at rest is the stories' (admin-roles.stories.tsx).

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** The fake's options for a community lead: hana holding the role `lead` alone, below the administrators. */
const lead: FakeOptions = { viewer: "hana", staffServer: { roles: { viewer: ["lead"] } } };
const admin: FakeOptions = { viewer: "hana" };

/** The permissions the fake's `lead` grants, in the server's order: what the lead may give. */
const leadPermissions = [
  "Handle reports",
  "Moderate accounts",
  "Manage roles",
  "Read the audit log",
  "Manage custom emoji",
  "Manage webhooks",
  "Write announcements",
  "Request custom emoji",
];

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

/** The app at `at` on the fake `on`, once its screen is read. */
async function opened(at: string, on: ReturnType<typeof fakeApp>) {
  const app = await renderApp(at, { on });
  await main().findAllByRole("heading", { level: 2 });
  await settled(app);
  return app;
}

/** The names of the permissions the form offers as checkboxes, in their order. */
const offeredPermissions = () =>
  within(main().getByRole("region", { name: "Permissions" }))
    .queryAllByRole("checkbox")
    .map((box) => words(box.closest("label") ?? box));

/** The question of `name` once it is open. */
const asking = async (name: string | RegExp) => within(await screen.findByRole("alertdialog", { name }));

/** What the status regions of the screen say, those that say something. */
const said = () =>
  main()
    .queryAllByRole("status")
    .map(words)
    .filter((text) => text !== "");

/** The names of the roles the select "Role to grant" offers. */
const grantable = () =>
  within(main().getByRole("combobox", { name: "Role to grant" }))
    .getAllByRole("option")
    .map(words);

/** The roles the account's table of roles lists, by their row's header; none where it shows no table. */
const heldRows = () =>
  main()
    .queryAllByRole("table", { name: "Roles" })
    .flatMap((table) => within(table).queryAllByRole("rowheader"))
    .map(words);

/** The row of the role `key` in the account's table of roles. */
const heldRow = (key: string) => {
  const row = within(main().getByRole("table", { name: "Roles" }))
    .getAllByRole("row")
    .find((r) =>
      within(r)
        .queryAllByRole("rowheader")
        .some((header) => words(header) === key),
    );
  if (row === undefined) throw new Error(`test: no row holds ${key}`);
  return row;
};

/** Replaces what the field `field` holds with `value`, as a reader types it. */
async function typeIn(app: TestApp, field: HTMLElement, value: string) {
  await app.user.clear(field);
  await app.user.type(field, value);
}

// What FR-720 forbids, not offered.

test("a new role's form offers the lead only the permissions they hold, and no place above theirs", async () => {
  const on = fakeApp(lead);
  await opened("/admin/roles/new", on);

  expect(offeredPermissions()).toEqual(leadPermissions);
  expect(
    main().getByRole("textbox", {
      name: "Place",
      description: "From 0 to 500. A role outranks those at a lower place.",
    }),
  ).toBeDefined();
});

test("a place typed above the lead's highest role becomes theirs, and the role is created there", async () => {
  const on = fakeApp(lead);
  const created = on.fake.script("POST /admin/roles");
  const app = await opened("/admin/roles/new", on);

  await typeIn(app, main().getByRole("textbox", { name: "Name" }), "greeter");
  await typeIn(app, main().getByRole("textbox", { name: "Place" }), "900");
  await app.user.click(main().getByText("Handle reports", { exact: true }));
  await app.user.click(main().getByRole("button", { name: "Create role" }));

  const sent = await created.arrived();
  expect(sent.body).toEqual({ key: "greeter", position: 500, permissions: ["manage_reports"] });
  expect(created.seen).toHaveLength(1);
});

test("the administrator's new role offers every permission their roles grant", async () => {
  const on = fakeApp(admin);
  const app = await opened("/admin/roles/new", on);

  const { data } = await app.data.api.GET("/accounts/verify_credentials");
  expect(offeredPermissions()).toHaveLength(data?.permissions.length ?? -1);
});

test("a role holding permissions the lead lacks names them, offers only the lead's, and keeps the others on Save", async () => {
  const on = fakeApp(lead);
  on.fake.script("GET /admin/roles", {
    change: (list) => ({
      ...list,
      items: list.items.map((role) =>
        role.key === "helper" ? { ...role, permissions: ["manage_domains" as const, "manage_reports" as const] } : role,
      ),
    }),
  });
  const changed = on.fake.script("PATCH /admin/roles/{role}");
  const app = await opened("/admin/roles/helper", on);

  expect(offeredPermissions()).toEqual(leadPermissions);
  expect(words(screen.getByRole("main"))).toContain("Moderate other servers");
  await app.user.click(main().getByText("Manage custom emoji", { exact: true }));
  await app.user.click(main().getByRole("button", { name: "Save" }));

  const sent = await changed.arrived();
  expect((sent.body.permissions ?? []).map(String).toSorted()).toEqual(
    ["manage_custom_emojis", "manage_domains", "manage_reports"].toSorted(),
  );
});

test("the built-in admin, above the lead's place, offers the lead nothing to change", async () => {
  const on = fakeApp(lead);
  await opened("/admin/roles/admin", on);

  expect(main().queryAllByRole("checkbox")).toHaveLength(0);
  expect(main().queryAllByRole("textbox")).toHaveLength(0);
  expect(main().queryAllByRole("button", { name: /^(Save|Delete)$/ })).toHaveLength(0);
});

test("the built-in admin offers the administrator its badge alone: no place, no permission, no Delete", async () => {
  const on = fakeApp(admin);
  await opened("/admin/roles/admin", on);

  expect(main().queryAllByRole("checkbox")).toHaveLength(0);
  expect(main().queryAllByRole("textbox", { name: "Place" })).toHaveLength(0);
  expect(main().getByRole("textbox", { name: "Label" })).toBeDefined();
  expect(main().queryAllByRole("button", { name: "Delete" })).toHaveLength(0);
});

for (const key of ["admin", "moderator"])
  test(`the built-in ${key} offers no Delete`, async () => {
    const on = fakeApp(admin);
    await opened(`/admin/roles/${key}`, on);
    expect(main().queryAllByRole("button", { name: /^Delete/ })).toHaveLength(0);
  });

test("the roles' list marks admin as above the lead's reach", async () => {
  const on = fakeApp(lead);
  await opened("/admin/roles", on);

  const items = within(main().getByRole("list", { name: "Roles" }))
    .getAllByRole("listitem")
    .map(words);
  expect(items[0]).toBe("admin Above yours Place 1000 26 permissions");
  expect(items.filter((item) => item.includes("Above yours"))).toHaveLength(1);
});

test("Grant offers the lead only the roles at or below theirs whose permissions they hold, and none mio holds", async () => {
  const on = fakeApp(lead);
  await opened(`/moderation/accounts/${on.fake.accountId("mio")}/manage`, on);

  expect(grantable()).toEqual(["lead", "moderator", "helper"]);
});

test("Grant leaves out a role at the lead's place that grants a permission they lack", async () => {
  const on = fakeApp(lead);
  on.fake.script("GET /admin/roles", {
    change: (list) => ({
      ...list,
      items: list.items.map((role) =>
        role.key === "helper" ? { ...role, permissions: ["manage_domains" as const] } : role,
      ),
    }),
  });
  await opened(`/moderation/accounts/${on.fake.accountId("mio")}/manage`, on);

  expect(grantable()).toEqual(["lead", "moderator"]);
});

test("a role above the lead's that the account holds says so, and offers no Revoke", async () => {
  // marco as the lead, on the Management page of hana, who holds admin.
  const on = fakeApp({ viewer: "marco", staffServer: { roles: { viewer: ["lead"] } } });
  await opened(`/moderation/accounts/${on.fake.accountId("hana")}/manage`, on);

  expect(heldRows()).toEqual(["admin"]);
  expect(words(heldRow("admin"))).toContain("Above yours");
  expect(within(heldRow("admin")).queryAllByRole("button")).toHaveLength(0);
});

// Create, change, move, delete.

test("a new role reaches the server with its place and permissions, and opens its page", async () => {
  const on = fakeApp(admin);
  const created = on.fake.script("POST /admin/roles");
  const app = await opened("/admin/roles/new", on);

  await typeIn(app, main().getByRole("textbox", { name: "Name" }), "greeter");
  await typeIn(app, main().getByRole("textbox", { name: "Place" }), "30");
  await app.user.click(main().getByText("Write announcements", { exact: true }));
  await app.user.click(main().getByRole("button", { name: "Create role" }));

  expect((await created.arrived()).body).toEqual({
    key: "greeter",
    position: 30,
    permissions: ["manage_announcements"],
  });
  await app.arrived("/admin/roles/greeter");
  expect(await main().findByRole("heading", { level: 2, name: "greeter" })).toBeDefined();
});

test("a role moved to another place and given a badge is saved, and the list shows it in its new place", async () => {
  const on = fakeApp(admin);
  const changed = on.fake.script("PATCH /admin/roles/{role}");
  const app = await opened("/admin/roles/helper", on);

  await typeIn(app, main().getByRole("textbox", { name: "Place" }), "200");
  await typeIn(app, main().getByRole("textbox", { name: "Label" }), "Helper");
  await app.user.click(main().getByRole("button", { name: "Save" }));

  const sent = await changed.arrived();
  expect(sent.body.position).toBe(200);
  expect(sent.body.badge?.label).toBe("Helper");
  await app.navigate("/admin/roles");
  await waitFor(() => {
    const items = within(main().getByRole("list", { name: "Roles" }))
      .getAllByRole("listitem")
      .map(words);
    if (items[2]?.startsWith("helper Place 200") !== true) throw new Error(`test: the roles read ${items.join(" / ")}`);
  });
});

test("a role is deleted only with its name typed, and leaves the list", async () => {
  const on = fakeApp(lead);
  const deleted = on.fake.script("DELETE /admin/roles/{role}");
  const app = await opened("/admin/roles/helper", on);

  const buttons = main().getAllByRole("button");
  expect(buttons.at(-1)).toBe(main().getByRole("button", { name: "Delete" }));
  await app.user.click(main().getByRole("button", { name: "Delete" }));
  const question = await asking(/^Delete\b.*\bhelper\b.*\?$/);
  const confirm = question.getByRole("button", { name: "Delete" });
  const name = question.getByRole("textbox", { name: "To confirm, enter helper" });
  expect(confirm.getAttribute("aria-disabled") === "true" || confirm.hasAttribute("disabled")).toBe(true);
  await app.user.type(name, "help");
  expect(confirm.getAttribute("aria-disabled") === "true" || confirm.hasAttribute("disabled")).toBe(true);
  expect(deleted.seen).toHaveLength(0);
  await app.user.type(name, "er");
  await app.user.click(question.getByRole("button", { name: "Delete" }));

  expect((await deleted.arrived()).url.pathname).toEndWith("/admin/roles/helper");
  await app.arrived("/admin/roles");
  await waitFor(() => {
    const items = within(main().getByRole("list", { name: "Roles" }))
      .getAllByRole("listitem")
      .map(words);
    if (items.length !== 4) throw new Error(`test: the roles read ${items.join(" / ")}`);
  });
});

// Grant and revoke.

test("a role granted on the account's Management page reaches the server, and the table and the status say it", async () => {
  const on = fakeApp(admin);
  const granted = on.fake.script("PUT /admin/accounts/{id}/roles/{role}");
  const app = await opened(`/moderation/accounts/${on.fake.accountId("mio")}/manage`, on);

  expect(heldRows()).toEqual(["artist"]);
  const staffRead = on.fake.script("GET /admin/accounts/{id}");
  await app.user.selectOptions(main().getByRole("combobox", { name: "Role to grant" }), "helper");
  await app.user.click(main().getByRole("button", { name: "Grant" }));

  expect((await granted.arrived()).url.pathname).toEndWith(`/admin/accounts/${on.fake.accountId("mio")}/roles/helper`);
  await waitFor(() => {
    if (!said().includes("@mio holds helper now.")) throw new Error(`test: the status says ${said().join(" / ")}`);
  });
  await waitFor(() => {
    if (heldRows().length !== 2) throw new Error(`test: the roles read ${heldRows().join(" / ")}`);
  });
  expect(heldRows().toSorted()).toEqual(["artist", "helper"]);
  expect(grantable()).not.toContain("helper");
  // The account, whose badges the page shows, is read again.
  expect(staffRead.seen.length).toBeGreaterThan(0);
});

test("a role revoked from another account reaches the server at once, and the status says it", async () => {
  const on = fakeApp(admin);
  const revoked = on.fake.script("DELETE /admin/accounts/{id}/roles/{role}");
  const app = await opened(`/moderation/accounts/${on.fake.accountId("mio")}/manage`, on);

  await app.user.click(within(main().getByRole("table", { name: "Roles" })).getByRole("button", { name: "Revoke" }));

  expect((await revoked.arrived()).url.pathname).toEndWith(`/roles/artist`);
  expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  await waitFor(() => {
    if (!said().includes("@mio no longer holds artist."))
      throw new Error(`test: the status says ${said().join(" / ")}`);
  });
  expect(heldRows()).toEqual([]);
});

test("revoking a role from yourself asks first, and Cancel revokes nothing", async () => {
  const on = fakeApp(lead);
  const revoked = on.fake.script("DELETE /admin/accounts/{id}/roles/{role}");
  const app = await opened(`/moderation/accounts/${on.fake.accountId("hana")}/manage`, on);

  await app.user.click(within(main().getByRole("table", { name: "Roles" })).getByRole("button", { name: "Revoke" }));
  const question = await asking("Revoke lead from yourself?");
  expect(words(screen.getByRole("alertdialog"))).toContain(
    "You lose what the role lets you do, perhaps also the permission to grant it back.",
  );
  await app.user.click(question.getByRole("button", { name: "Keep the role" }));
  await waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the question is still open");
  });
  expect(revoked.seen).toHaveLength(0);
  expect(heldRows()).toEqual(["lead"]);

  await app.user.click(within(main().getByRole("table", { name: "Roles" })).getByRole("button", { name: "Revoke" }));
  await app.user.click((await asking("Revoke lead from yourself?")).getByRole("button", { name: "Revoke" }));
  expect((await revoked.arrived()).url.pathname).toEndWith(`/admin/accounts/${on.fake.accountId("hana")}/roles/lead`);
});

// The entries, each by its permission.

test("the administration offers the lead the entries their permissions open, and no other", async () => {
  const on = fakeApp(lead);
  const app = await renderApp("/admin", { on });
  await main().findByRole("link", { name: /^Roles/ });
  await settled(app);

  expect(
    main()
      .getAllByRole("link")
      .map((link) => link.getAttribute("href")),
  ).toEqual(["/admin/roles", "/admin/announcements", "/admin/custom-emojis", "/admin/webhooks"]);
});

for (const at of [
  "/admin/roles",
  "/admin/roles/new",
  "/admin/custom-emojis",
  "/admin/announcements",
  "/admin/webhooks/new",
])
  test(`${at} is not open to a moderator, whose permissions do not open it, and asks the server nothing of it`, async () => {
    const on = fakeApp({ viewer: "marco" });
    const reads = [
      on.fake.script("GET /admin/roles"),
      on.fake.script("GET /admin/custom_emojis"),
      on.fake.script("GET /admin/announcements"),
      on.fake.script("GET /admin/webhooks"),
    ];
    const app = await renderApp(at, { on });
    const alert = await main().findByRole("alert");
    await settled(app);

    expect(words(alert)).toBe("This address leads to no screen open to you. All administration screens");
    expect(reads.map((read) => read.seen.length)).toEqual([0, 0, 0, 0]);
  });
