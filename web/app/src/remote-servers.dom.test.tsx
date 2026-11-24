// SPDX-License-Identifier: AGPL-3.0-only

// The staff servers screen (#3478), on the fake API, as marco reads it: it lists every server this one knows, in
// domain order, each with its state, its software and its failed days where deliveries failed; its State filter lists
// every server, those with delivery problems, or those marked unavailable, and the address says which; an empty list
// says whether no server is known or none matches the filter. A server's own screen says its software, "Not reported"
// where it reports none, and when it was first seen, left out where the server does not know. What each screen shows at
// rest at each width is the stories' (domains.stories.tsx, Screens/Moderation/Domains).

import { expect, test } from "bun:test";
import { waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** `text` without the marks that isolate a name or a number inside a sentence, and with plain spaces. */
const plain = (text: string) =>
  text
    .replaceAll(String.fromCodePoint(0x2068), "")
    .replaceAll(String.fromCodePoint(0x2069), "")
    .replaceAll(String.fromCodePoint(0xa0), " ")
    .replaceAll(String.fromCodePoint(0x202f), " ");

/** What `element` says, the isolating marks taken out. */
const said = (element: Element) => plain(words(element));

/** The app at `path`, read by marco, who manages domains, once its heading is in place. */
async function at(path: string, options: Parameters<typeof fakeApp>[0] = {}) {
  const on = fakeApp({ viewer: "marco", ...options });
  const app = await renderApp(path, { on });
  await main().findByRole("heading", { level: 1 });
  return app;
}

/** Each row of the table of remote servers, as what its cells say: the server, its state, its software, its failures. */
const rows = () =>
  within(main().getByRole("table", { name: "Remote servers" }))
    .getAllByRole("row")
    .slice(1)
    .map((row) => [
      said(within(row).getByRole("rowheader")),
      ...within(row)
        .getAllByRole("cell")
        .map((cell) => said(cell)),
    ]);

/** The domains the table lists, once it lists `count` of them. */
const listed = (count: number) =>
  waitFor(() => {
    const domains = main().queryAllByRole("table", { name: "Remote servers" }).length === 0 ? [] : rows();
    if (domains.length !== count)
      throw new Error(`test: the table lists ${JSON.stringify(domains.map((row) => row[0]))}`);
    return domains.map((row) => row[0]);
  });

/** Waits until the screen says `sentence` in a status. */
const says = (sentence: string) =>
  waitFor(() => {
    const statuses = main()
      .queryAllByRole("status")
      .map((status) => said(status));
    if (!statuses.includes(sentence)) throw new Error(`test: the statuses say ${JSON.stringify(statuses)}`);
  });

/** Chooses `option` of the State filter. */
async function filterBy(app: TestApp, option: string) {
  const filters = within(main().getByRole("search", { name: "Filter remote servers" }));
  await app.user.click(filters.getByRole("button", { name: /^State\b/ }));
  const menu = await waitFor(() => within(document.body).getByRole("menu", { name: "State" }));
  await app.user.click(within(menu).getByRole("menuitemradio", { name: option }));
}

/** The query of each read of the list the script saw, as `troubled` and `state`. */
const asked = (script: { seen: readonly { url: URL }[] }) =>
  script.seen.map(({ url }) => ({
    troubled: url.searchParams.get("troubled"),
    state: url.searchParams.get("state"),
  }));

/** What the facts of the server say, a term and its value joined with no space. */
const facts = () => said(main().getByLabelText("About this server"));

/** The fake's own servers, in domain order. */
const every = [
  "a-rather-long-subdomain.of-a-community-run-server.social.example",
  "dead.example",
  "flaky.example",
  "gone.example",
  "mastodon.example",
  "misskey.example",
  "photos.example",
  "quiet.example",
  "social.example",
];

// The list.

test("with no filter, the list holds every known server in domain order, each with its state, software and failures", async () => {
  const on = fakeApp({ viewer: "marco" });
  const read = on.fake.script("GET /admin/instances");
  await renderApp("/moderation/instances", { on });
  expect(await listed(every.length)).toEqual(every);

  const [server, state, software, failures] = [0, 1, 2, 3].map((i) => rows().map((row) => row[i]));
  expect(server).toEqual(every);
  expect(state).toEqual([
    "Available",
    "Unavailable",
    "Available",
    "Unavailable",
    "Available",
    "Available",
    "Available",
    "Available",
    "Available",
  ]);
  // A name and its version; the name alone where the version is empty; Not reported where the server reports none.
  expect(software).toEqual([
    "snac 2.63",
    "mastodon 4.2.10",
    "misskey 2024.11.0",
    "Not reported",
    "mastodon 4.3.2",
    "misskey 2025.1.0",
    "pixelfed 0.12.4",
    "Not reported",
    "gotosocial",
  ]);
  // Failed days only where deliveries failed.
  expect((failures ?? []).map((cell) => /(\d+) days?$/.exec(cell ?? "")?.[1] ?? cell)).toEqual([
    "",
    "9",
    "2",
    "21",
    "",
    "",
    "",
    "",
    "",
  ]);
  expect(asked(read).every((q) => q.troubled === null && q.state === null)).toBe(true);
});

test("each server of the list leads to its own screen", async () => {
  await at("/moderation/instances");
  await listed(every.length);
  const row = within(main().getByRole("table", { name: "Remote servers" }))
    .getAllByRole("row")
    .find((r) => said(within(r).queryAllByRole("rowheader")[0] ?? r) === "photos.example");
  if (row === undefined) throw new Error("test: no row of photos.example");
  const links = within(row).getAllByRole("link");
  expect(links.map((link) => link.getAttribute("href"))).toEqual(["/moderation/instances/photos.example"]);
});

test("the State filter offers Any state, Delivery problems and Unavailable, Any state chosen at first", async () => {
  const app = await at("/moderation/instances");
  await listed(every.length);
  const filters = within(main().getByRole("search", { name: "Filter remote servers" }));
  await app.user.click(filters.getByRole("button", { name: /^State\b/ }));
  const menu = await waitFor(() => within(document.body).getByRole("menu", { name: "State" }));
  const options = within(menu).getAllByRole("menuitemradio");
  expect(options.map((option) => [said(option), option.getAttribute("aria-checked")])).toEqual([
    ["Any state", "true"],
    ["Delivery problems", "false"],
    ["Unavailable", "false"],
  ]);
});

test("Delivery problems lists only the servers deliveries failed to reach or marked unavailable, asked as troubled=true", async () => {
  const app = await at("/moderation/instances");
  await listed(every.length);
  const read = app.fake.script("GET /admin/instances");

  await filterBy(app, "Delivery problems");

  expect(await listed(3)).toEqual(["dead.example", "flaky.example", "gone.example"]);
  expect(new URL(app.path(), "https://demo.test").searchParams.get("state")).toBe("troubled");
  expect(asked(read)).toContainEqual({ troubled: "true", state: null });
  expect(said(main().getByRole("search", { name: "Filter remote servers" }))).toContain("State: Delivery problems");
});

test("Unavailable lists only the servers marked unavailable, asked as state=unavailable", async () => {
  const app = await at("/moderation/instances");
  await listed(every.length);
  const read = app.fake.script("GET /admin/instances");

  await filterBy(app, "Unavailable");

  expect(await listed(2)).toEqual(["dead.example", "gone.example"]);
  expect(new URL(app.path(), "https://demo.test").searchParams.get("state")).toBe("unavailable");
  expect(asked(read)).toContainEqual({ troubled: null, state: "unavailable" });
});

test("Any state, chosen again, lists every known server", async () => {
  const app = await at("/moderation/instances?state=troubled");
  await listed(3);

  await filterBy(app, "Any state");

  expect(await listed(every.length)).toEqual(every);
  expect(new URL(app.path(), "https://demo.test").searchParams.get("state")).toBeNull();
});

for (const [state, domains] of [
  ["troubled", ["dead.example", "flaky.example", "gone.example"]],
  ["unavailable", ["dead.example", "gone.example"]],
] as const)
  test(`the address ?state=${state} opens the list filtered`, async () => {
    await at(`/moderation/instances?state=${state}`);
    expect(await listed(domains.length)).toEqual([...domains]);
  });

test("the list pages through every known server", async () => {
  const app = await at("/moderation/instances", { staffPage: 1 });
  expect(await listed(1)).toEqual([every[0]]);

  await app.user.click(main().getByRole("button", { name: "Next" }));

  expect(await listed(1)).toEqual([every[1]]);
});

test("with no server known, the list says no remote server is known yet", async () => {
  await at("/moderation/instances", { domains: "none" });
  await says("No remote server is known yet.");
  expect(main().queryAllByRole("table", { name: "Remote servers" })).toHaveLength(0);
});

test("with no server with problems, the filtered list says none matches and how to see every server", async () => {
  await at("/moderation/instances?state=troubled", { domains: "none" });
  await says("No server matches. Clear the filter to see every server.");
  expect(main().queryAllByRole("table", { name: "Remote servers" })).toHaveLength(0);
});

// A server's own screen.

/** The fake's own record of `domain`. */
async function recordOf(app: TestApp, domain: string) {
  const { data } = await app.data.api.GET("/admin/instances/{domain}", { params: { path: { domain } } });
  if (data === undefined) throw new Error(`test: the fake does not know ${domain}`);
  return data;
}

test("a server's screen says its software and version, and when it was first seen", async () => {
  const app = await at("/moderation/instances/photos.example");
  await waitFor(() => main().getByLabelText("About this server"));
  const { first_seen_at: first } = await recordOf(app, "photos.example");
  if (first === undefined) throw new Error("test: the fake has no first sighting of photos.example");
  const day = plain(new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(new Date(first)));

  expect(facts()).toMatch(/Software\s*pixelfed 0\.12\.4/);
  expect(facts()).toContain(`First seen${day}`);
  expect(facts()).toMatch(/State\s*Available/);
  expect(facts()).toMatch(/Failed days\s*No failure/);
});

test("a server that reports no software says Not reported, and leaves out a first sighting it does not know", async () => {
  await at("/moderation/instances/quiet.example");
  await waitFor(() => main().getByLabelText("About this server"));

  expect(facts()).toMatch(/Software\s*Not reported/);
  expect(facts()).not.toContain("First seen");
  expect(facts()).toMatch(/State\s*Available/);
});

test("an unavailable server's screen says its software, first sighting, state, since when, and failed days", async () => {
  await at("/moderation/instances/dead.example");
  await waitFor(() => main().getByLabelText("About this server"));

  expect(facts()).toMatch(/Software\s*mastodon 4\.2\.10/);
  expect(facts()).toContain("First seen");
  expect(facts()).toMatch(/State\s*Unavailable/);
  expect(facts()).toContain("Since");
  expect(facts()).toMatch(/Failed days\s*9 days/);
  expect(facts()).toContain("Last failed day");
});
