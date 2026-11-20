// SPDX-License-Identifier: AGPL-3.0-only

// The pages of the staff's lists (#3279), on their routes and the fake API. A list that is a screen's subject pages
// with Previous and Next, which page it is said after them, its place kept in the address: Next shows the second page
// and says so, a reload of that address shows it again, and Previous goes back to the first. A list inside a detail
// grows under Show more, the rest read only on a press; the account's reports, sign-ins and same-IP accounts read
// whole past the newest 20 its details hold. The fake pages the staff's lists by `staffPage` items, or holds more than
// a page (`crowded`).

import { createAppData, watchServer } from "@my/app-data";
import { fakeOrigin } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { expect, test } from "bun:test";
import { main, renderApp, words, type AppOptions, type TestApp } from "./test/app";
import type { FakeApp } from "./test/server";

/** The pager under the screen's list: the last navigation named Pages (a window of periods may stand above it). */
const pager = () => {
  const pagers = main().getAllByRole("navigation", { name: "Pages" });
  const last = pagers.at(-1);
  if (last === undefined) throw new Error("test: no pager");
  return within(last);
};

/** Waits until the pager says page `n`. */
const atPage = (n: number) =>
  waitFor(() => {
    const said = pager().getByRole("status").textContent;
    if (said !== `Page ${String(n)}`) throw new Error(`test: the pager says ${said}`);
  });

/** What the screen shows of its list and its pager: the main landmark's words. */
const shown = () => words(screen.getByRole("main"));

/**
 * The app loaded anew at the address it is at, on the same server: new data, nothing of the first app's cache kept,
 * as a reload of the page.
 */
async function reloaded(app: TestApp): Promise<TestApp> {
  const at = app.path();
  cleanup();
  const watch = watchServer({ server: fakeOrigin, fetch: app.fake.serve, followDevice: false });
  const data = createAppData(
    createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: watch.fetch }),
    undefined,
    watch,
  );
  const on: FakeApp = {
    fake: app.fake,
    watch,
    data: { ...data, connection: { server: fakeOrigin, fetch: watch.fetch } },
    visitor: false,
  };
  return renderApp(at, { on });
}

/** Next, a reload, then Previous, on the list at `path` read by `viewer`, `perPage` items a page. */
async function pagesThrough(path: string, viewer: string, perPage: number, more: AppOptions = {}) {
  const app = await renderApp(path, { viewer, staffPage: perPage, ...more });
  await atPage(1);
  const first = shown();
  expect(pager().getByRole<HTMLButtonElement>("button", { name: "Previous" }).disabled).toBe(true);

  await app.user.click(pager().getByRole("button", { name: "Next" }));
  await atPage(2);
  expect(app.path()).toContain("cursor=");
  const second = shown();
  expect(second).not.toBe(first);

  const again = await reloaded(app);
  await atPage(2);
  await waitFor(() => {
    if (shown() !== second) throw new Error("test: the reload shows another page");
  });

  await again.user.click(pager().getByRole("button", { name: "Previous" }));
  await atPage(1);
  expect(again.path()).not.toContain("cursor=");
  await waitFor(() => {
    if (shown() !== first) throw new Error("test: Previous shows another page than the first");
  });
}

test("the reports queue: Next shows its second page, a reload of its address the same, Previous the first", async () => {
  await pagesThrough("/moderation/reports", "marco", 2);
});

test("the audit log: Next shows its second page, a reload of its address the same, Previous the first", async () => {
  await pagesThrough("/moderation/audit-log", "marco", 2);
});

// Every other list that is a screen's subject, on its second page and back.
const subjects: readonly [string, string, number, AppOptions?][] = [
  ["/moderation/accounts", "marco", 2],
  ["/moderation/appeals?state=any", "marco", 2],
  ["/moderation/registrations", "marco", 2],
  ["/moderation/hook-decisions", "marco", 2],
  ["/moderation/hook-decisions/appeals?state=any", "marco", 1],
  ["/moderation/signup-blocks?active=all", "marco", 2],
  ["/moderation/invites", "hana", 2],
  ["/moderation/domain-actions?active=all", "marco", 2],
  ["/moderation/domain-actions/subscriptions", "marco", 1],
  ["/moderation/instances", "marco", 1],
  ["/admin/jobs/list", "hana", 2],
  ["/admin/errors", "hana", 1],
  ["/admin/finance/costs", "hana", 2],
  ["/admin/finance/income", "hana", 2],
  ["/admin/supporters/subscriptions", "hana", 2, { supportersAdmin: { subscriptions: "many" } }],
  ["/admin/announcements", "hana", 2],
  ["/admin/custom-emojis", "hana", 2],
  ["/admin/custom-emojis/requests", "hana", 2],
  ["/admin/oauth-apps", "hana", 2],
  ["/admin/oauth-apps/hosts", "hana", 1],
];
for (const [path, viewer, perPage, more] of subjects)
  test(`${path}: Next shows its second page, a reload of its address the same, Previous the first`, async () => {
    await pagesThrough(path, viewer, perPage, more);
  });

test("an account's Actions tab pages through its actions, its place kept in the address", async () => {
  const app = await renderApp("/moderation", { viewer: "marco", staffPage: 1 });
  await app.navigate(`/moderation/accounts/${app.fake.accountId("quickdeals")}/actions?active=all`);
  await atPage(1);
  await app.user.click(pager().getByRole("button", { name: "Next" }));
  await atPage(2);
  expect(app.path()).toContain("cursor=");
});

/** Presses the screen's Show more and waits until `count()` reaches `to`. */
async function showMore(app: TestApp, count: () => number, to: number) {
  await app.user.click(main().getByRole("button", { name: "Show more" }));
  await waitFor(() => {
    if (count() !== to) throw new Error(`test: ${String(count())} of ${String(to)} are shown`);
  });
}

test("a webhook's deliveries grow under Show more, and Show more goes once they are read to their end", async () => {
  const app = await renderApp("/admin", { viewer: "hana", staffPage: 2 });
  await app.navigate(`/admin/webhooks/${app.fake.staffServer.id("webhook", "alerts")}`);
  const table = within(await main().findByRole("table", { name: "Deliveries" }));
  const rows = () => table.getAllByRole("rowheader").length;
  expect(rows()).toBe(2);
  await showMore(app, rows, 4);
  expect(main().queryByRole("button", { name: "Show more" })).toBeNull();
  // The focus went to the first delivery the press added, where the reader goes on.
  await waitFor(() => {
    const added = table.getAllByRole("rowheader")[2];
    const focused = app.focused();
    if (added === undefined || focused === null || !added.contains(focused))
      throw new Error(`test: the focus is on ${focused?.outerHTML.slice(0, 80) ?? "the page"}`);
  });
});

test("a failed job's runs grow under Show more", async () => {
  const app = await renderApp("/admin", { viewer: "hana", staffPage: 1 });
  await app.navigate(`/admin/jobs/${app.fake.operations.id("job:dead-deliver")}`);
  const table = within(await main().findByRole("table", { name: "Failed attempts" }));
  const rows = () => table.getAllByRole("rowheader").length;
  expect(rows()).toBe(1);
  await showMore(app, rows, 2);
});

test("a remote server's availability and deliveries each grow under their own Show more", async () => {
  const app = await renderApp("/moderation/instances/dead.example", { viewer: "marco", staffPage: 1 });
  const availability = within(await main().findByRole("list", { name: "Availability" }));
  const events = () => availability.getAllByRole("listitem").length;
  expect(events()).toBe(1);
  const [first] = main().getAllByRole("button", { name: "Show more" });
  if (first === undefined) throw new Error("test: no Show more");
  await app.user.click(first);
  await waitFor(() => {
    if (events() !== 2) throw new Error("test: the second event is not shown");
  });
  const deliveries = within(main().getByRole("list", { name: "Deliveries" }));
  const sent = () => deliveries.getAllByRole("listitem").filter((item) => item.parentElement?.tagName === "OL").length;
  const before = sent();
  const more = main().getAllByRole("button", { name: "Show more" }).at(-1);
  if (more === undefined) throw new Error("test: no Show more under the deliveries");
  await app.user.click(more);
  await waitFor(() => {
    if (sent() <= before) throw new Error("test: no delivery was added");
  });
});

test("a subscription's fetches grow under Show more", async () => {
  const app = await renderApp("/moderation", { viewer: "marco", staffPage: 2 });
  await app.navigate(`/moderation/domain-actions/subscriptions/${app.fake.domains.id("list", "shared")}`);
  const fetches = within(await main().findByRole("list", { name: "Fetches" }));
  const count = () => fetches.getAllByRole("listitem").length;
  expect(count()).toBe(2);
  await app.user.click(main().getByRole("button", { name: "Show more" }));
  await waitFor(() => {
    if (count() <= 2) throw new Error("test: no fetch was added");
  });
});

test("an account's reports read whole past the newest 20 its details hold, the next only on a press", async () => {
  const app = await renderApp("/moderation", { viewer: "marco", reports: "crowded", moderation: "crowded" });
  const listed = app.fake.script("GET /admin/reports");
  await app.navigate(`/moderation/accounts/${app.fake.accountId("kai")}`);
  const table = within(await main().findByRole("table", { name: "Reports" }));
  const rows = () => table.getAllByRole("rowheader").length;
  expect(rows()).toBe(20);
  expect(listed.seen).toHaveLength(0);
  // kai's two reports of the fake's queue, and the crowd's 24.
  await showMore(app, rows, 26);
  expect(new URL(listed.seen[0]?.url ?? fakeOrigin).searchParams.get("target_account_id")).toBe(
    app.fake.accountId("kai"),
  );
  expect(main().queryByRole("button", { name: "Show more" })).toBeNull();
});

test("an account's sign-ins and the accounts of its address read whole, each page only on a press", async () => {
  const app = await renderApp("/moderation", { viewer: "hana", moderation: "crowded" });
  const signIns = app.fake.script("GET /admin/accounts/{id}/sign_ins");
  const sameIp = app.fake.script("GET /admin/accounts/{id}/same_ip_accounts");
  await app.navigate(`/moderation/accounts/${app.fake.accountId("kai")}/manage`);
  const [show] = within(await main().findByLabelText("Email and IP addresses")).getAllByRole("button", {
    name: "Show",
  });
  if (show === undefined) throw new Error("test: nothing to show");
  await app.user.click(show);

  const table = within(await main().findByRole("table", { name: "Sign-ins" }));
  const rows = () => table.getAllByRole("rowheader").length;
  expect(rows()).toBe(20);
  expect(signIns.seen).toHaveLength(0);
  expect(sameIp.seen).toHaveLength(0);
  const [more] = main().getAllByRole("button", { name: "Show more" });
  if (more === undefined) throw new Error("test: no Show more under the sign-ins");
  await app.user.click(more);
  await waitFor(() => {
    if (rows() !== 30) throw new Error(`test: ${String(rows())} sign-ins are shown`);
  });
  expect(signIns.seen).toHaveLength(1);
  expect(sameIp.seen).toHaveLength(0);

  const accounts = main()
    .getAllByRole("table")
    .find((t) => t.getAttribute("aria-label")?.startsWith("Accounts that used ") === true);
  if (accounts === undefined) throw new Error("test: no table of the accounts of an address");
  const others = () => within(accounts).getAllByRole("rowheader").length;
  expect(others()).toBe(20);
  const [next] = main().getAllByRole("button", { name: "Show more" });
  if (next === undefined) throw new Error("test: no Show more under the accounts of the address");
  await app.user.click(next);
  await waitFor(() => {
    if (others() !== 24) throw new Error(`test: ${String(others())} accounts are shown`);
  });
  expect(sameIp.seen).toHaveLength(1);
});

test("an error group's hours grow under Show more, twelve at a time", async () => {
  const app = await renderApp("/admin", { viewer: "hana", operations: "crowded" });
  await app.navigate(`/admin/errors/${app.fake.operations.id("error:panic")}`);
  const table = within(await main().findByRole("table", { name: "By hour" }));
  const rows = () => table.getAllByRole("rowheader").length;
  expect(rows()).toBe(12);
  await showMore(app, rows, 24);
  await showMore(app, rows, 30);
  expect(main().queryByRole("button", { name: "Show more" })).toBeNull();
});

test("the rules of the link card crawler grow under Show more, forty at a time", async () => {
  const app = await renderApp("/admin/link-card-domains", { viewer: "hana", operations: "crowded" });
  const list = within(await main().findByRole("list", { name: "Rules" }));
  const rules = () => list.getAllByRole("listitem").length;
  expect(rules()).toBe(40);
  await showMore(app, rules, 48);
  expect(main().queryByRole("button", { name: "Show more" })).toBeNull();
});
