// SPDX-License-Identifier: AGPL-3.0-only

// Finding people as a feature: an account found by the search or suggested is followed from its row; what the screen
// of the suggested people says where the server suggests no one or its suggestions failed; and the people suggested
// over the home of a reader who follows no one.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { Search } from "../search/search";
import { HomeTimeline, loadHomeTimeline } from "../timeline/home";
import { FindPeople, loadPeople } from "./people";

const mio = "Mio 澪";

/** The row of the account named `name` in the list named `list`, once the screen shows it. */
const rowOf = (list: string, name: string) =>
  waitFor(() => {
    const [row, ...more] = within(screen.getByRole("list", { name: list }))
      .getAllByRole("listitem")
      .filter((item) => within(item).queryByRole("link", { name }) !== null);
    if (row === undefined || more.length > 0) throw new Error(`test: no one row of ${list} is ${name}'s`);
    return row;
  });

/** Follows the account of `row` with its Follow button, and waits until the row says so. */
async function follow(
  { fake, user }: { fake: ReturnType<typeof fakeData>["fake"]; user: ReturnType<typeof renderFeature> },
  row: HTMLElement,
) {
  const asked = fake.script("POST /accounts/{id}/follow");
  await user.click(within(row).getByRole("button", { name: "Follow" }));
  expect(await within(row).findByText("Following")).not.toBeNull();
  expect(asked.seen).toHaveLength(1);
}

test("the search finds an account by its name, and the reader follows it from the results", async () => {
  const { fake, data } = fakeData({ viewer: "sam" });
  const user = renderFeature(data, <Search q={mio} onQuery={() => undefined} />);

  await follow({ fake, user }, await rowOf("People", mio));
});

test("finding people suggests an account, and the reader follows it from there", async () => {
  const { fake, data } = fakeData({ viewer: "sam" });
  await loadPeople(data);
  const user = renderFeature(data, <FindPeople />);

  await follow({ fake, user }, await rowOf("Find people", mio));
});

/** The accounts the screen leads to: the links to a profile. */
const accounts = () => screen.queryAllByRole("link").filter((link) => link.getAttribute("href")?.startsWith("/@"));

test("where the server suggests no one, finding people says so in one line, and shows no account", async () => {
  const { fake, data } = fakeData({ viewer: "sam" });
  fake.script("GET /accounts/suggestions", { change: (page) => ({ ...page, items: [] }) });
  await loadPeople(data);
  renderFeature(data, <FindPeople />);

  const said = await screen.findByText("No one to suggest right now. Search for people by name instead.");
  expect(said.getAttribute("role")).toBe("status");
  expect(accounts()).toHaveLength(0);
});

test("a one-person server suggests no one to its person", async () => {
  // hana is the one person of the server.
  const { data } = fakeData({ viewer: "hana", singleUser: true });
  await loadPeople(data);
  renderFeature(data, <FindPeople />);

  expect(await screen.findByText("No one to suggest right now. Search for people by name instead.")).not.toBeNull();
  expect(accounts()).toHaveLength(0);
});

test("suggestions that could not be read say so once, with one Try again", async () => {
  const { fake, data } = fakeData({ viewer: "sam" });
  fake.script("GET /accounts/suggestions", { fail: 500 });
  await loadPeople(data);
  renderFeature(data, <FindPeople />);

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toMatch(/^The suggestions could not be loaded\.\s?Try again$/);
  expect(screen.getAllByRole("alert")).toHaveLength(1);
  expect(screen.getAllByRole("button", { name: "Try again" })).toHaveLength(1);
  expect(accounts()).toHaveLength(0);
});

/** The home timeline of quickdeals, who follows no one and has one post of their own, loaded and drawn. */
async function newHome(script: (fake: ReturnType<typeof fakeData>["fake"]) => void = () => undefined) {
  const made = fakeData({ viewer: "quickdeals" });
  script(made.fake);
  await loadHomeTimeline(made.data);
  renderFeature(
    made.data,
    <PostActions onCompose={() => undefined}>
      <HomeTimeline />
    </PostActions>,
  );
  return made;
}

test("the home of a reader who follows no one suggests five people, each with its follow, before its posts, then the way to the rest", async () => {
  await newHome();

  const suggested = await screen.findByRole("list", { name: "Suggested for you" });
  const rows = within(suggested).getAllByRole("listitem");
  expect(rows).toHaveLength(5);
  for (const row of rows) expect(within(row).getAllByRole("button", { name: "Follow" })).toHaveLength(1);
  expect(screen.getByRole("link", { name: "Find more people" }).getAttribute("href")).toBe("/people");
  // In the order a reader meets them; where they stand on the page is the layout test's (web/e2e/catalog/specs/layout/home.spec.ts).
  const [post] = within(screen.getByRole("feed", { name: "Home timeline" })).getAllByRole("article");
  if (post === undefined) throw new Error("test: the home timeline shows no post");
  expect(suggested.compareDocumentPosition(post) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
});

test("where the server suggests no one, that home shows nothing of finding people", async () => {
  let answered = false;
  const { data } = await newHome((fake) => {
    const suggestions = fake.script("GET /accounts/suggestions", { change: (page) => ({ ...page, items: [] }) });
    void suggestions.served().then(() => (answered = true));
  });

  // Nothing shows while the suggestions are on their way either: the test waits for the answer itself.
  await waitFor(() => {
    if (!answered || data.queryClient.isFetching() > 0) throw new Error("the server's answers are still on their way");
  });
  expect(screen.getByRole("feed", { name: "Home timeline" })).not.toBeNull();
  expect(screen.queryAllByRole("list")).toHaveLength(0);
  expect(screen.queryAllByRole("link", { name: "Find more people" })).toHaveLength(0);
});
