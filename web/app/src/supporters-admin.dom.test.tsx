// SPDX-License-Identifier: AGPL-3.0-only

// Supporters in the administration (#3264, ADR-0072, ADR-0162; FR-910, FR-912, FR-914, FR-918, FR-919), on the fake
// API, as what follows a press: the program turned on, and off only while no subscription goes on; the tips' amounts
// read with their thousands separators and saved, one out of range marked, the tips turned off; a plan created on a
// role offered below the staff roles, changed with its role's position and permissions, a position among the staff
// refused, and retired once its name is typed; the subscriptions narrowed by state from the address, and one ended once
// the subscriber is typed; a reconciliation run now, and the hour's limit said; who reads the supporter list saved;
// and the screens closed to a reader without `manage_supporters`. What each screen shows at rest is the stories'
// (supporters-admin.stories.tsx).

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

type Admin = NonNullable<FakeOptions["supportersAdmin"]>;

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

/** The fake of the administrator hana, its supporter program as `admin` says. */
const server = (admin: Admin = {}) => fakeApp({ viewer: "hana", supportersAdmin: admin });

/** Renders the app on `on` at `at`, and returns once the screen's reads are in. */
async function open(on: ReturnType<typeof server>, at: string) {
  const app = await renderApp(at, { on });
  await main().findByRole("heading", { level: 1 });
  await settled(app);
  return app;
}

const button = (name: string) => main().getByRole("button", { name });
const textbox = (name: string) => main().getByRole("textbox", { name });

/** Replaces what `field` holds with `value`, as a reader types it. */
async function typeIn(app: TestApp, field: HTMLElement, value: string) {
  await app.user.clear(field);
  await app.user.type(field, value);
}

/** Waits until `field` is marked invalid. */
const marked = (field: () => HTMLElement) =>
  waitFor(() => {
    if (field().getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked");
  });

/** What the page's live regions and alerts say, each its words. */
const said = () => [...screen.queryAllByRole("status"), ...screen.queryAllByRole("alert")].map(words);

/** Waits until a live region or an alert says something besides `before`. */
const saysAnew = (before: readonly string[]) =>
  waitFor(() => {
    const now = said().filter((text) => text !== "" && !before.includes(text));
    if (now.length === 0) throw new Error(`test: the page says ${JSON.stringify(said())}`);
    return now;
  });

/** Whether `element` cannot be pressed now. */
const disabled = (element: HTMLElement) =>
  element.getAttribute("aria-disabled") === "true" || element.hasAttribute("disabled");

// The program.

test("a program that is off is turned on, and the server keeps it on", async () => {
  const on = server({ program: "off", subscriptions: "none" });
  const turned = on.fake.script("PUT /admin/supporter_program");
  const app = await open(on, "/admin/supporters");

  await app.user.click(button("Turn the program on"));
  const question = screen.queryByRole("alertdialog");
  if (question !== null) await app.user.click(within(question).getAllByRole("button").at(-1) ?? question);

  expect((await turned.arrived()).body).toEqual({ enabled: true });
  await waitFor(() => {
    if (!/State\s?On since /.test(words(screen.getByRole("main"))))
      throw new Error("test: the program does not read as on");
  });
});

test("a program on with no subscription going on is turned off; with subscriptions going on it cannot be", async () => {
  const quiet = server({ subscriptions: "none" });
  const turned = quiet.fake.script("PUT /admin/supporter_program");
  const app = await open(quiet, "/admin/supporters");
  await app.user.click(button("Turn the program off"));
  const question = screen.queryByRole("alertdialog");
  if (question !== null) await app.user.click(within(question).getAllByRole("button").at(-1) ?? question);
  expect((await turned.arrived()).body).toEqual({ enabled: false });
});

test("with subscriptions going on, the program offers no way to turn it off, and says why", async () => {
  await open(server(), "/admin/supporters");
  expect(main().queryAllByRole("button", { name: "Turn the program off" })).toHaveLength(0);
  expect(words(screen.getByRole("main"))).toContain("The program cannot be turned off while subscriptions go on.");
});

// The tips.

test("the tips' amounts are read with their thousands separators, one a line, and saved in the currency chosen", async () => {
  const on = server();
  const saved = on.fake.script("PUT /admin/supporter_tips");
  const app = await open(on, "/admin/supporters");

  await typeIn(app, textbox("Amounts"), "1,000{Enter}500{Enter}10,000");
  await app.user.click(button("Save the tips"));

  const body = (await saved.arrived()).body;
  expect(body.enabled).toBe(true);
  expect(body.amounts?.map((m) => `${m.amount} ${m.currency}`).toSorted()).toEqual([
    "1000 JPY",
    "10000 JPY",
    "500 JPY",
  ]);
});

test("an amount out of the provider's range is marked under the amounts, and the tips stay as they were", async () => {
  const on = server();
  const app = await open(on, "/admin/supporters");

  await typeIn(app, textbox("Amounts"), "50{Enter}1,000");
  await app.user.click(button("Save the tips"));

  await marked(() => textbox("Amounts"));
  const { data } = await app.data.api.GET("/admin/supporter_tips");
  expect(data?.amounts.map((m) => m.amount)).toEqual(["500", "1000", "3000"]);
});

test("the tips turned off are saved off", async () => {
  const on = server();
  const saved = on.fake.script("PUT /admin/supporter_tips");
  const app = await open(on, "/admin/supporters");

  await app.user.click(main().getByRole("checkbox", { name: "Take tips" }));
  await app.user.click(button("Save the tips"));

  expect((await saved.arrived()).body.enabled).toBe(false);
});

// The plans.

test("a new plan is offered the roles below the staff roles that no plan has, and is created on one", async () => {
  const on = server();
  const created = on.fake.script("POST /admin/supporter_plans");
  const app = await open(on, "/admin/supporters/plans/new");

  const role = main().getByRole("combobox", { name: "Role" });
  expect(
    within(role)
      .getAllByRole("option")
      .map((o) => o.getAttribute("value")),
  ).toEqual(["", "listeners"]);
  expect(textbox("Monthly price").getAttribute("inputmode")).toBe("decimal");

  await app.user.selectOptions(role, "listeners");
  await typeIn(app, textbox("Name"), "Listener");
  await typeIn(app, textbox("Description"), "Opens voice chat rooms.");
  await typeIn(app, textbox("Monthly price"), "800");
  await typeIn(app, textbox("Yearly discount (%)"), "10");
  await typeIn(app, textbox("Order"), "7");
  await app.user.click(button("Create the plan"));

  expect((await created.arrived()).body).toMatchObject({
    role: "listeners",
    name: "Listener",
    description: "Opens voice chat rooms.",
    monthly_price: { amount: "800", currency: "JPY" },
    annual_discount_percent: 10,
    rank: 7,
    visibility: "public",
  });
  expect(await main().findByRole("heading", { level: 2, name: "Listener" })).toBeDefined();
  expect(app.path()).toMatch(/^\/admin\/supporters\/plans\/[^/]+$/);
  expect(main().getByText("¥800 a month, ¥8,640 a year (10% off)", { exact: true })).toBeDefined();
});

test("a plan's price, its role's position and a permission changed are saved together", async () => {
  const on = server();
  const id = on.fake.supportersAdmin.planId("patron");
  const changed = on.fake.script("PATCH /admin/supporter_plans/{id}");
  const app = await open(on, `/admin/supporters/plans/${id}`);

  await typeIn(app, textbox("Monthly price"), "2000");
  await typeIn(app, textbox("Role position"), "30");
  await app.user.click(main().getByRole("checkbox", { name: "Write articles" }));
  await app.user.click(button("Save"));

  const body = (await changed.arrived()).body;
  expect(body).toMatchObject({ monthly_price: { amount: "2000", currency: "JPY" }, role_position: 30 });
  expect(body.role_permissions?.toSorted()).toEqual(["create_articles", "create_custom_emojis", "view_finance"]);
  expect(await main().findByText("¥2,000 a month, ¥19,920 a year (17% off)", { exact: true })).toBeDefined();
});

test("a role position among the staff roles is marked, and the plan keeps its role", async () => {
  const on = server();
  const id = on.fake.supportersAdmin.planId("patron");
  const app = await open(on, `/admin/supporters/plans/${id}`);

  await typeIn(app, textbox("Role position"), "100");
  await app.user.click(button("Save"));

  await waitFor(() => {
    if (!words(screen.getByRole("main")).includes("The role must stand below the moderators."))
      throw new Error("test: the refusal is not under the role's position");
  });
  await marked(() => textbox("Role position"));
  const { data } = await app.data.api.GET("/admin/supporter_plans");
  expect(data?.items.find((plan) => plan.id === id)?.role.position).toBe(20);
});

test("a plan is retired only once its name is typed, and then takes no subscriptions", async () => {
  const on = server();
  const id = on.fake.supportersAdmin.planId("patron");
  const retired = on.fake.script("POST /admin/supporter_plans/{id}/retire");
  const app = await open(on, `/admin/supporters/plans/${id}`);

  await app.user.click(button("Retire the plan…"));
  const question = within(await screen.findByRole("alertdialog"));
  expect(disabled(question.getByRole("button", { name: "Retire" }))).toBe(true);
  await app.user.type(question.getByRole("textbox"), "Patro");
  expect(disabled(question.getByRole("button", { name: "Retire" }))).toBe(true);
  expect(retired.seen).toHaveLength(0);
  await app.user.type(question.getByRole("textbox"), "n");
  await app.user.click(question.getByRole("button", { name: "Retire" }));

  expect((await retired.arrived()).url.pathname).toEndWith(`/admin/supporter_plans/${id}/retire`);
  await waitFor(() => {
    if (main().queryAllByRole("button", { name: "Retire the plan…" }).length > 0)
      throw new Error("test: the plan still offers Retire");
  });
});

// The subscriptions.

test("the subscriptions narrowed by state in the address are asked of the server so, and list those alone", async () => {
  const on = server();
  const listed = on.fake.script("GET /admin/supporter_subscriptions");
  await open(on, "/admin/supporters/subscriptions?state=past_due");

  expect(listed.seen.some((request) => request.url.searchParams.get("state") === "past_due")).toBe(true);
  const rows = within(main().getByRole("table", { name: "Subscriptions" }))
    .getAllByRole("rowheader")
    .map(words);
  expect(rows).toEqual(["@marco"]);
});

test("a subscription is ended now only once what the question asks is typed", async () => {
  const on = server();
  const id = on.fake.supportersAdmin.subscriptionId("sam");
  const ended = on.fake.script("POST /admin/supporter_subscriptions/{id}/end");
  const app = await open(on, `/admin/supporters/subscriptions/${id}`);

  await app.user.click(button("End now…"));
  const dialog = await screen.findByRole("alertdialog");
  const question = within(dialog);
  const field = question.getByRole("textbox");
  const confirm = question.getAllByRole("button").at(-1);
  if (confirm === undefined) throw new Error("test: the question has no button");
  expect(disabled(confirm)).toBe(true);
  await app.user.type(field, "x");
  expect(disabled(confirm)).toBe(true);
  expect(ended.seen).toHaveLength(0);
  // The words to type are the question's own: `@sam` or `sam`, as it names them.
  const asked = /@?sam/.exec(words(dialog))?.[0] ?? "sam";
  await typeIn(app, field, asked);
  await app.user.click(confirm);

  expect((await ended.arrived()).url.pathname).toEndWith(`/admin/supporter_subscriptions/${id}/end`);
  await waitFor(() => {
    if (main().queryAllByRole("button", { name: "End now…" }).length > 0)
      throw new Error("test: the subscription still offers End now");
  });
});

// The payment provider.

/** What the program says once Reconcile now is pressed on a fake whose program is `admin`. */
async function reconcileNow(admin: Admin) {
  const on = server(admin);
  const run = on.fake.script("POST /admin/payment_reconciliation/runs");
  const app = await open(on, "/admin/supporters");
  const before = said();
  await app.user.click(button("Reconcile now"));
  await run.arrived();
  return saysAnew(before);
}

test("a reconciliation run now is said to be on its way", async () => {
  expect((await reconcileNow({})).join(" ")).not.toMatch(/hour/i);
});

test("a reconciliation in the hour of the last is refused, and the refusal says to wait", async () => {
  expect((await reconcileNow({ reconcileTooSoon: true })).join(" ")).toMatch(/hour/i);
});

// The supporter list.

test("who may read the supporter list is saved as the instance setting", async () => {
  const on = server();
  const saved = on.fake.script("PATCH /admin/settings");
  const app = await open(on, "/admin/supporters");

  await app.user.selectOptions(main().getByRole("combobox", { name: "Who can read the list" }), "everyone");
  await app.user.click(button("Save"));

  expect((await saved.arrived()).body.values).toEqual({ supporter_list_visibility: "everyone" });
});

// Who may.

test("a reader without manage_supporters is shown none of the program", async () => {
  const on = fakeApp({ viewer: "marco", supportersAdmin: {} });
  const program = on.fake.script("GET /admin/supporter_program");
  await renderApp("/admin/supporters", { on });
  await main().findByRole("heading", { level: 1 });
  expect(words(screen.getByRole("main"))).toContain("This address leads to no screen open to you.");
  expect(program.seen).toHaveLength(0);
  expect(main().queryAllByRole("button", { name: /^(Reconcile now|Save the tips)$/ })).toHaveLength(0);
  expect(main().queryAllByRole("link", { name: /^Plans/ })).toHaveLength(0);
});
