// SPDX-License-Identifier: AGPL-3.0-only

// Finance in the administration (#3264, ADR-0067, ADR-0068; FR-901–FR-905, FR-917), on the fake API, as what follows
// a press: a cost recorded once or every N weeks, months or years, in its currency with the digits ISO 4217 gives it,
// an amount with more refused before it is sent; a price revised from a date, which the screen says it applies from,
// and one before the first payment refused; a cost ended, its end removed, relabelled, and deleted only once the
// question that says it cannot be undone is answered; the totals by week, month or year and the two ways of counting a
// payment asked of the server; the money movements of one kind from the address; a billing import run now, and the
// hour's limit said; what is published saved through the instance settings, with the meter's target and rail; and the
// screens closed to a reader without `manage_finance`, the publication to one without `manage_settings`. What each
// screen shows at rest is the stories' (finance-admin.stories.tsx); the cache's part is app-data's
// (finance-admin.test.ts).

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

/** The fake of the administrator hana, with `options` besides. */
const server = (options: FakeOptions = {}) => fakeApp({ viewer: "hana", ...options });

/** Renders the app on `on` at `at`, and returns once the screen's reads are in. */
async function open(on: ReturnType<typeof server>, at: string) {
  const app = await renderApp(at, { on });
  await main().findByRole("heading", { level: 1 });
  await settled(app);
  return app;
}

const button = (name: string) => main().getByRole("button", { name });
const textbox = (name: string) => main().getByRole("textbox", { name });
const field = (name: string) => main().getByLabelText(name, { exact: true });

/** Replaces what `element` holds with `value`, as a reader types it. */
async function typeIn(app: TestApp, element: HTMLElement, value: string) {
  await app.user.clear(element);
  await app.user.type(element, value);
}

/** Sets the date field `name` to `date`, as a browser's date picker does. */
const pick = (name: string, date: string) => {
  fireEvent.change(field(name), { target: { value: date } });
};

/** Waits until `element` is marked invalid. */
const marked = (element: () => HTMLElement) =>
  waitFor(() => {
    if (element().getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked");
  });

/** What the page's live regions and alerts say, each its words. */
const said = () => [...screen.queryAllByRole("status"), ...screen.queryAllByRole("alert")].map(words);

/** Waits until a live region or an alert says `sentence`. */
const says = (sentence: string) =>
  waitFor(() => {
    if (!said().includes(sentence)) throw new Error(`test: the page says ${JSON.stringify(said())}`);
  });

/** Waits until a live region or an alert says something besides `before`, and returns what it says. */
const saysAnew = (before: readonly string[]) =>
  waitFor(() => {
    const now = said().filter((text) => text !== "" && !before.includes(text));
    if (now.length === 0) throw new Error(`test: the page says ${JSON.stringify(said())}`);
    return now;
  });

/** The rows of the table `name`, each as its head and its cells' words. */
const rows = (name: string) =>
  within(main().getByRole("table", { name }))
    .getAllByRole("row")
    .slice(1)
    .map((row) => [...within(row).queryAllByRole("rowheader"), ...within(row).queryAllByRole("cell")].map(words));

/** Fills the form of a new cost with what every cost has. */
async function describeCost(app: TestApp) {
  await typeIn(app, textbox("Name"), "Object storage");
  await typeIn(app, textbox("Provider"), "Example Cloud");
  await app.user.selectOptions(field("Category"), "storage");
}

// Recording a cost.

test("a cost paid every 2 weeks reaches the server with its cycle, and opens on its screen with its first price", async () => {
  const on = server();
  const recorded = on.fake.script("POST /admin/cost_items");
  const app = await open(on, "/admin/finance/costs/new");

  await describeCost(app);
  await typeIn(app, textbox("Every"), "2");
  await app.user.selectOptions(field("How often"), "weeks");
  pick("First payment on", "2026-01-05");
  expect(textbox("Amount").getAttribute("inputmode")).toBe("decimal");
  await typeIn(app, textbox("Amount"), "12.50");
  await app.user.selectOptions(field("Currency"), "USD");
  await app.user.click(button("Record the cost"));

  const body = (await recorded.arrived()).body;
  expect(body).toMatchObject({
    name: "Object storage",
    provider: "Example Cloud",
    category: "storage",
    schedule: "weeks",
    interval: 2,
    starts_on: "2026-01-05",
    price: { amount: "12.50", currency: "USD" },
  });
  expect(body.ends_on).toBeUndefined();
  expect(await main().findByRole("heading", { level: 2, name: "Object storage" })).toBeDefined();
  expect(app.path()).toMatch(/^\/admin\/finance\/costs\/[^/]+$/);
  expect(rows("Prices")).toEqual([["Jan 5, 2026", "$12.50"]]);
});

for (const [schedule, interval] of [
  ["months", 3],
  ["years", 1],
] as const)
  test(`a cost paid every ${String(interval)} ${schedule} through a last payment is sent with its end`, async () => {
    const on = server();
    const recorded = on.fake.script("POST /admin/cost_items");
    const app = await open(on, "/admin/finance/costs/new");

    await describeCost(app);
    await typeIn(app, textbox("Every"), String(interval));
    await app.user.selectOptions(field("How often"), schedule);
    pick("First payment on", "2026-01-01");
    pick("Last payment on", "2028-01-01");
    await typeIn(app, textbox("Amount"), "1650");
    await app.user.selectOptions(field("Currency"), "JPY");
    await app.user.click(button("Record the cost"));

    expect((await recorded.arrived()).body).toMatchObject({
      schedule,
      interval,
      starts_on: "2026-01-01",
      ends_on: "2028-01-01",
      price: { amount: "1650", currency: "JPY" },
    });
  });

test("a cost paid once is sent as one payment, with no cycle and no end", async () => {
  const on = server();
  const recorded = on.fake.script("POST /admin/cost_items");
  const app = await open(on, "/admin/finance/costs/new");

  await describeCost(app);
  await app.user.selectOptions(field("How often"), "once");
  expect(main().queryAllByRole("textbox", { name: "Every" })).toHaveLength(0);
  expect(main().queryAllByLabelText("Last payment on")).toHaveLength(0);
  pick("Paid on", "2026-02-01");
  await typeIn(app, textbox("Amount"), "5000");
  await app.user.selectOptions(field("Currency"), "JPY");
  await app.user.click(button("Record the cost"));

  const body = (await recorded.arrived()).body;
  expect(body).toMatchObject({ schedule: "once", interval: 1, starts_on: "2026-02-01" });
  expect(body.ends_on).toBeUndefined();
});

for (const [currency, amount, shown] of [
  ["IQD", "12.345", "IQD"],
  ["HUF", "1000.50", "HUF"],
] as const)
  test(`an amount in ${currency} is taken with the digits ISO 4217 gives it, and read back in its currency`, async () => {
    const on = server();
    const recorded = on.fake.script("POST /admin/cost_items");
    const app = await open(on, "/admin/finance/costs/new");

    await describeCost(app);
    pick("First payment on", "2026-01-01");
    await typeIn(app, textbox("Amount"), amount);
    await app.user.selectOptions(field("Currency"), currency);
    await app.user.click(button("Record the cost"));

    expect((await recorded.arrived()).body.price).toEqual({ amount, currency });
    await main().findByRole("heading", { level: 2, name: "Object storage" });
    const [price] = rows("Prices");
    expect(price?.[1]).toContain(shown);
    expect(price?.[1]).toContain(amount === "1000.50" ? "1,000.50" : amount);
  });

for (const [currency, amount] of [
  ["JPY", "1650.5"],
  ["USD", "12.345"],
  ["USD", "1,000"],
] as const)
  test(`${amount} in ${currency} is marked under the amount, and nothing is recorded`, async () => {
    const on = server();
    const recorded = on.fake.script("POST /admin/cost_items");
    const app = await open(on, "/admin/finance/costs/new");

    await describeCost(app);
    pick("First payment on", "2026-01-01");
    await typeIn(app, textbox("Amount"), amount);
    await app.user.selectOptions(field("Currency"), currency);
    await app.user.click(button("Record the cost"));

    await marked(() => textbox("Amount"));
    expect(recorded.seen).toHaveLength(0);
    expect(app.path()).toBe("/admin/finance/costs/new");
  });

// A cost's prices.

test("a price revised from a date is said to apply from that date, and joins the prices before the older ones", async () => {
  const on = server();
  const id = on.fake.financeAdmin.costId("vps");
  const revised = on.fake.script("POST /admin/cost_items/{id}/price_revisions");
  const app = await open(on, `/admin/finance/costs/${id}`);
  expect(textbox("New price in JPY").getAttribute("inputmode")).toBe("decimal");

  pick("Applies from", "2026-12-01");
  await typeIn(app, textbox("New price in JPY"), "4000");
  await app.user.click(button("Revise the price"));

  expect((await revised.arrived()).body).toEqual({
    effective_on: "2026-12-01",
    price: { amount: "4000", currency: "JPY" },
  });
  await says("From Dec 1, 2026, the price is ¥4,000.");
  await waitFor(() => {
    if (rows("Prices").length !== 3) throw new Error("test: the new price is not among the prices");
  });
  expect(rows("Prices")).toEqual([
    ["Dec 1, 2026", "¥4,000"],
    ["Apr 15, 2026", "¥3,630"],
    ["Jan 15, 2025", "¥3,300"],
  ]);
});

test("a price from before the first payment is marked under its date, and the prices stay as they were", async () => {
  const on = server();
  const id = on.fake.financeAdmin.costId("vps");
  const app = await open(on, `/admin/finance/costs/${id}`);

  pick("Applies from", "2024-12-01");
  await typeIn(app, textbox("New price in JPY"), "4000");
  await app.user.click(button("Revise the price"));

  await marked(() => field("Applies from"));
  const { data } = await app.data.api.GET("/admin/cost_items/{id}", { params: { path: { id } } });
  expect(data?.prices).toHaveLength(2);
});

// Ending, relabelling and deleting a cost.

test("a cost ended on a date is sent its last payment, and then offers to move or remove the end", async () => {
  const on = server();
  const id = on.fake.financeAdmin.costId("vps");
  const ended = on.fake.script("PUT /admin/cost_items/{id}/end");
  const app = await open(on, `/admin/finance/costs/${id}`);

  pick("Last payment on", "2027-01-15");
  await app.user.click(button("End on this date"));

  expect((await ended.arrived()).body).toEqual({ ends_on: "2027-01-15" });
  expect(await main().findByRole("button", { name: "Move the end" })).toBeDefined();
  expect(button("Remove the end")).toBeDefined();
});

test("a cost's end removed is sent with no date, and the cost goes on", async () => {
  const on = server();
  const id = on.fake.financeAdmin.costId("domain");
  const ended = on.fake.script("PUT /admin/cost_items/{id}/end");
  const app = await open(on, `/admin/finance/costs/${id}`);

  await app.user.click(button("Remove the end"));
  const question = screen.queryByRole("alertdialog");
  if (question !== null) await app.user.click(within(question).getAllByRole("button").at(-1) ?? question);

  expect((await ended.arrived()).body.ends_on).toBeUndefined();
  expect(await main().findByRole("button", { name: "End on this date" })).toBeDefined();
  const { data } = await app.data.api.GET("/admin/cost_items/{id}", { params: { path: { id } } });
  expect(data?.ends_on).toBeUndefined();
});

test("a cost renamed and given another provider saves its labels alone", async () => {
  const on = server();
  const id = on.fake.financeAdmin.costId("vps");
  const relabelled = on.fake.script("PATCH /admin/cost_items/{id}");
  const app = await open(on, `/admin/finance/costs/${id}`);

  await typeIn(app, textbox("Name"), "VPS Tokyo");
  await typeIn(app, textbox("Provider"), "Sakura Internet");
  await app.user.click(button("Save"));

  expect((await relabelled.arrived()).body).toMatchObject({ name: "VPS Tokyo", provider: "Sakura Internet" });
  expect(await main().findByRole("heading", { level: 2, name: "VPS Tokyo" })).toBeDefined();
});

test("a cost is deleted only once the question that says it cannot be undone is answered", async () => {
  const on = server();
  const id = on.fake.financeAdmin.costId("vps");
  const deleted = on.fake.script("DELETE /admin/cost_items/{id}");
  const app = await open(on, `/admin/finance/costs/${id}`);

  await app.user.click(button("Delete the cost…"));
  const first = within(await screen.findByRole("alertdialog", { name: "Delete VPS?" }));
  expect(words(screen.getByRole("alertdialog"))).toContain("This cannot be undone.");
  await app.user.click(first.getByRole("button", { name: "Cancel" }));
  await waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the question is still open");
  });
  expect(deleted.seen).toHaveLength(0);

  await app.user.click(button("Delete the cost…"));
  const second = within(await screen.findByRole("alertdialog", { name: "Delete VPS?" }));
  await app.user.click(second.getByRole("button", { name: "Delete" }));

  expect((await deleted.arrived()).url.pathname).toEndWith(`/admin/cost_items/${id}`);
  await app.arrived("/admin/finance/costs");
  await main().findByRole("table", { name: "Costs" });
  await settled(app);
  expect(rows("Costs").map(([head]) => head)).not.toContain("VPSSakura Cloud");
  expect(main().queryAllByRole("link", { name: "VPS" })).toHaveLength(0);
});

// The totals.

test("the totals are asked by week, by year and by month again, each heading its table", async () => {
  const on = server();
  const totals = on.fake.script("GET /admin/cost_totals");
  const app = await open(on, "/admin/finance");
  await main().findByRole("table", { name: "Totals by month" });

  for (const [granularity, label] of [
    ["week", "Week"],
    ["year", "Year"],
    ["month", "Month"],
  ] as const) {
    await app.user.click(main().getByRole("radio", { name: label }));
    expect(await main().findByRole("table", { name: `Totals by ${granularity}` })).toBeDefined();
    const asked = totals.seen.map((request) => request.url.searchParams.get("granularity"));
    expect(asked).toContain(granularity);
  }
});

test("a long cost is spread over its cycle at first, and counted in the period paid on asking", async () => {
  const on = server();
  const totals = on.fake.script("GET /admin/cost_totals");
  const app = await open(on, "/admin/finance");
  await main().findByRole("table", { name: "Totals by month" });
  const spread = totals.seen.at(-1)?.url.searchParams.get("basis");
  expect(spread === null || spread === "amortized").toBe(true);

  await app.user.click(main().getByRole("radio", { name: /^In the period paid/ }));
  await waitFor(() => {
    if (totals.seen.at(-1)?.url.searchParams.get("basis") !== "cash") throw new Error("test: no read by payment");
  });
  await settled(app);
  expect(main().getByRole("radio", { name: /^In the period paid/ })).toHaveProperty("checked", true);
  expect(main().getByRole("table", { name: "Totals by month" })).toBeDefined();
});

// The income.

test("the income's totals are a figures table by period, and the movements of one kind come from the address", async () => {
  const on = server();
  const movements = on.fake.script("GET /admin/income_movements");
  await open(on, "/admin/finance/income?kind=refund");

  const heads = within(main().getByRole("table", { name: "Income per period" }))
    .getAllByRole("columnheader")
    .map(words);
  for (const head of ["Gross", "Fees", "Net", "Balance"]) expect(heads).toContain(head);
  expect(movements.seen.some((request) => request.url.searchParams.get("kind") === "refund")).toBe(true);
  const listed = await waitFor(() => {
    const found = rows("Money movements");
    if (found.length === 0) throw new Error("test: no movement listed");
    return found;
  });
  for (const row of listed) expect(row.join(" ")).toContain("Refund");
});

// The billing imports.

/** What the imports say once Import now is pressed on a fake whose finance is `financeAdmin`. */
async function importNow(financeAdmin: NonNullable<FakeOptions["financeAdmin"]>) {
  const on = server({ financeAdmin });
  const run = on.fake.script("POST /admin/billing_imports/{source}/runs");
  const app = await open(on, "/admin/finance/imports");
  const before = said();
  await app.user.click(button("Import now"));
  expect((await run.arrived()).url.pathname).toEndWith("/admin/billing_imports/aws/runs");
  return saysAnew(before);
}

test("a billing import run now is said to be on its way", async () => {
  const queued = await importNow({});
  expect(queued.join(" ")).not.toMatch(/hour/i);
});

test("an import within the hour of the last is refused, and the refusal says to wait", async () => {
  const refused = await importNow({ importTooSoon: true });
  expect(refused.join(" ")).toMatch(/hour/i);
});

test("a source whose last import failed says why, beside its state", async () => {
  await open(server({ financeAdmin: { imports: "failing" } }), "/admin/finance/imports");
  const page = words(screen.getByRole("main"));
  expect(page).toContain("Failed");
  expect(page).toContain("The credentials may not read the billing data.");
});

// What is published.

test("what is published is saved as the instance settings, with the meter's target and rail, and the instance read again", async () => {
  const on = server({ finance: { meter: "percent" } });
  const saved = on.fake.script("PATCH /admin/settings");
  const app = await open(on, "/admin/finance/publication");
  const instance = on.fake.script("GET /instance");

  await app.user.click(
    within(main().getByRole("radiogroup", { name: "Costs" })).getByRole("radio", { name: /^Totals(?! and)/ }),
  );
  await app.user.click(
    within(main().getByRole("radiogroup", { name: "Income" })).getByRole("radio", { name: /^Totals(?! and)/ }),
  );
  // With both sides published, the meter may show its amounts, and a percentage alone no longer.
  const meter = within(main().getByRole("radiogroup", { name: "Funding meter" })).getAllByRole("radio");
  expect(meter).toHaveLength(2);
  const amounts = meter.find((radio) => !(radio.closest("label")?.textContent ?? "").startsWith("Not shown"));
  if (amounts === undefined) throw new Error("test: the meter offers no amounts");
  await app.user.click(amounts);
  expect(textbox("Monthly target").getAttribute("inputmode")).toBe("decimal");
  await typeIn(app, textbox("Monthly target"), "30000");
  const rail = main().getByRole("checkbox", { name: /^Also show the meter in the side column/ });
  const railBefore = (rail as HTMLInputElement).checked;
  await app.user.click(rail);
  await app.user.click(button("Save"));

  expect((await saved.arrived()).body.values).toMatchObject({
    finance_visibility: "totals",
    income_visibility: "totals",
    funding_meter_visibility: "amounts",
    funding_meter_target_amount: "30000",
    funding_meter_rail: String(!railBefore),
  });
  await waitFor(() => {
    if (instance.seen.length === 0) throw new Error("test: the instance, which holds the meter, is not read again");
  });
});

test("while one side alone is published, the meter offers no way to be shown, and says why", async () => {
  await open(server({ finance: { costs: "totals", meter: "none" } }), "/admin/finance/publication");
  const meter = within(main().getByRole("radiogroup", { name: "Funding meter" }));
  expect(meter.getAllByRole("radio").map(words)).toHaveLength(1);
  expect(words(screen.getByRole("main"))).toContain(
    "While only one of the costs and the income is published, the meter cannot be shown",
  );
});

// Who may.

test("a reader without manage_finance is shown none of the finance", async () => {
  const on = fakeApp({ viewer: "marco" });
  const costs = on.fake.script("GET /admin/cost_items");
  await renderApp("/admin/finance/costs", { on });
  await main().findByRole("heading", { level: 1 });
  expect(words(screen.getByRole("main"))).toContain("This address leads to no screen open to you.");
  expect(main().queryAllByRole("table", { name: "Costs" })).toHaveLength(0);
  expect(main().queryAllByRole("link", { name: "Record a cost" })).toHaveLength(0);
  expect(costs.seen).toHaveLength(0);
});

/** A fake of hana with the permissions `permissions` alone. */
function holding(permissions: ("manage_finance" | "manage_settings")[]) {
  const on = server();
  on.fake.script("GET /accounts/verify_credentials", { change: (account) => ({ ...account, permissions }) });
  return on;
}

test("a holder of manage_finance without manage_settings reads what is published, and is not offered to change it", async () => {
  const on = holding(["manage_finance"]);
  const saved = on.fake.script("PATCH /admin/settings");
  await open(on, "/admin/finance/publication");

  expect(main().queryAllByRole("button", { name: "Save" })).toHaveLength(0);
  expect(main().queryAllByRole("radio")).toHaveLength(0);
  expect(words(screen.getByRole("main"))).toContain(
    "Only an administrator who may change the server's settings changes what is published.",
  );
  expect(saved.seen).toHaveLength(0);
});

test("a holder of manage_settings without manage_finance is told the finance is not open to them", async () => {
  const on = holding(["manage_settings"]);
  const costs = on.fake.script("GET /admin/cost_totals");
  await open(on, "/admin/finance");

  expect(words(screen.getByRole("main"))).toContain("This address leads to no screen open to you.");
  expect(main().queryAllByRole("link", { name: /^Costs/ })).toHaveLength(0);
  expect(costs.seen).toHaveLength(0);
});
