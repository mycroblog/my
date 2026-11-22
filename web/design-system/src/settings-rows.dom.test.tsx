// SPDX-License-Identifier: AGPL-3.0-only

// The rows of a settings list that say more than a title (#3202): a row's facts, each an element of its own on a line
// of its own kind; an account's row, whose controls are named by the account; and the result that stands in a row in
// place of its control once the reader used it, short for the eye and a whole sentence for assistive technology. That
// the result takes the focus a script gives it and is no stop of Tab is settings-rows.jsdom.test.tsx; a list, a row's
// title, caption, state and end are settings.dom.test.tsx; how the rows look is a layout test.

import { expect, test } from "bun:test";
import { Button } from "@my/design-system";
import { cleanup, isInaccessible, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { SettingsAccountItem, SettingsList, SettingsListRow } from "./settings";
import { ListAndAdd } from "./settings-parts.stories";

/** Whether `first` stands before `second` in the page, as it is read and as Tab goes. */
const before = (first: Element, second: Element) =>
  (first.compareDocumentPosition(second) & first.DOCUMENT_POSITION_FOLLOWING) !== 0;

/** `text` with each run of spaces as one, without the spaces at its ends. */
const plain = (text: string | null | undefined) => (text ?? "").replace(/\s+/g, " ").trim();

/** A story as this test draws it: what its `render` returns, which takes nothing. */
type Drawn = { render?: (...args: never[]) => ReactNode };

// ---- A row's facts --------------------------------------------------------------------------------------------------

test("a row's facts are read after its title and its state, line by line, each fact an element of its own", () => {
  render(
    <SettingsList label="Filters">
      <SettingsListRow
        title="Marathon week"
        state="Expired"
        facts={[["Folds matching posts", "1 word"], "In home and lists"]}
        href="/settings/filters/3"
      />
    </SettingsList>,
  );

  const row = within(within(screen.getByRole("list", { name: "Filters" })).getByRole("listitem"));
  // The link says all of it, in the order it is drawn.
  const link = row.getByRole("link", {
    name: /^Marathon week\s*Expired\s*Folds matching posts\s*·\s*1 word\s*In home and lists$/,
  });
  expect(link.getAttribute("href")).toBe("/settings/filters/3");
  // A fact wraps as a whole: each has an element that holds it and no other fact.
  const facts = ["Folds matching posts", "1 word", "In home and lists"].map((fact) =>
    row.getByText(fact, { exact: false }),
  );
  expect(new Set(facts).size).toBe(3);
  for (const [i, fact] of facts.entries()) {
    expect(facts.filter((other) => other !== fact && fact.contains(other))).toHaveLength(0);
    const next = facts[i + 1];
    if (next !== undefined) expect(before(fact, next)).toBe(true);
  }
});

test("a row with facts and no address is no link, and what stands at its end is still a group named by its title", () => {
  render(
    <SettingsList label="Filters">
      <SettingsListRow title="Spoilers" facts={[["Folds matching posts", "2 words"]]} end={<Button>Delete</Button>} />
    </SettingsList>,
  );

  const row = within(screen.getByRole("listitem"));
  expect(row.queryAllByRole("link")).toHaveLength(0);
  expect(screen.getByRole("listitem").textContent).toMatch(/^Spoilers\s*Folds matching posts\s*·\s*2 words\s*Delete$/);
  expect(within(row.getByRole("group", { name: "Spoilers" })).getAllByRole("button", { name: "Delete" })).toHaveLength(
    1,
  );
});

// ---- A row that leads somewhere and has its own control -------------------------------------------------------------

/** The glyphs drawn inside `link`: decorations with no role and no words, so found by what they are. */
const glyphs = (link: HTMLElement) => [...link.querySelectorAll("svg")];

/** For each glyph of `link`, whether it stands before the title `title` (its icon) or after it (a chevron). */
const placed = (link: HTMLElement, title: string) => {
  const words = within(link).getByText(title);
  return glyphs(link).map((glyph) => (before(glyph, words) ? "before" : "after"));
};

// #3368: a followed hashtag's row leads to the hashtag's page and holds Unfollow.
test("a row with an address and an end is a link of its glyph and its title, with no chevron, and its end beside the link in a group named by the title", () => {
  render(
    <SettingsList label="Followed hashtags">
      <SettingsListRow
        icon="hashtag"
        title="#東京"
        href={`/tags/${encodeURIComponent("東京")}`}
        end={<Button>Unfollow</Button>}
      />
      <SettingsListRow icon="hashtag" title="#baking" href="/tags/baking" end={<Button>Unfollow</Button>} />
    </SettingsList>,
  );

  const items = within(screen.getByRole("list", { name: "Followed hashtags" })).getAllByRole("listitem");
  expect(items).toHaveLength(2);
  for (const [row, tag] of [
    [items[0], "東京"],
    [items[1], "baking"],
  ] as const) {
    if (row === undefined) throw new Error("test: the list has no two rows");
    const name = `#${tag}`;
    const link = within(row).getByRole("link", { name });
    expect(link.getAttribute("href")).toBe(`/tags/${encodeURIComponent(tag)}`);
    expect(within(row).getAllByRole("link")).toHaveLength(1);
    // The glyph is the link's, before its title, hidden from assistive technology; nothing is drawn after the title.
    expect(placed(link, name)).toEqual(["before"]);
    expect(glyphs(link).map((glyph) => isInaccessible(glyph))).toEqual([true]);
    // The end is beside the link, after it, in a group the title names.
    const group = within(row).getByRole("group", { name });
    expect(within(group).getAllByRole("button", { name: "Unfollow" })).toHaveLength(1);
    expect(link.contains(group)).toBe(false);
    expect(within(link).queryAllByRole("button")).toHaveLength(0);
    expect(before(link, group)).toBe(true);
    expect(row.textContent).toMatch(new RegExp(`^${name}\\s*Unfollow$`));
  }
});

test("a row with an address and no end is still a link whose glyph is followed by a chevron after its title", () => {
  render(
    <SettingsList label="Filters">
      <SettingsListRow icon="hashtag" title="Spoilers" href="/settings/filters/1" />
      <SettingsListRow title="Election" href="/settings/filters/2" />
    </SettingsList>,
  );

  const [spoilers, election] = within(screen.getByRole("list", { name: "Filters" })).getAllByRole("listitem");
  if (spoilers === undefined || election === undefined) throw new Error("test: the list has no two rows");
  const withIcon = within(spoilers).getByRole("link", { name: "Spoilers" });
  expect(withIcon.getAttribute("href")).toBe("/settings/filters/1");
  expect(placed(withIcon, "Spoilers")).toEqual(["before", "after"]);
  const bare = within(election).getByRole("link", { name: "Election" });
  expect(bare.getAttribute("href")).toBe("/settings/filters/2");
  expect(placed(bare, "Election")).toEqual(["after"]);
  // The whole row is the link: no group, no control beside it.
  expect([...within(spoilers).queryAllByRole("group"), ...within(election).queryAllByRole("group")]).toHaveLength(0);
  expect(glyphs(withIcon).map((glyph) => isInaccessible(glyph))).toEqual([true, true]);
});

test("the story Settings parts / List and add holds the followed hashtags, each a link to its tag's page beside its Unfollow", () => {
  cleanup();
  const story: Drawn = ListAndAdd;
  if (story.render === undefined) throw new Error("test: the story has no render");
  render(<>{story.render()}</>);

  const part = screen.getByRole("region", { name: /^Followed hashtags\s*2$/ });
  const items = within(within(part).getByRole("list", { name: "Followed hashtags" })).getAllByRole("listitem");
  expect(
    items.map((row) => {
      const link = within(row).getByRole("link");
      return [
        link.getAttribute("href"),
        within(row).getAllByRole("link").length,
        placed(link, plain(link.textContent)),
        within(within(row).getByRole("group", { name: plain(link.textContent) })).getAllByRole("button", {
          name: "Unfollow",
        }).length,
      ];
    }),
  ).toEqual([
    ["/tags/photography", 1, ["before"], 1],
    ["/tags/baking", 1, ["before"], 1],
  ]);
  expect(items.map((row) => within(row).getByRole("link").textContent)).toEqual(["#photography", "#baking"]);
});

// ---- A row's result -------------------------------------------------------------------------------------------------

const unmuted = { id: "unmuted-spam", shown: "Unmuted", spoken: "You unmuted the server spam.example." };

test("a row's result stands in the row in place of its end: a word for the eye, hidden from assistive technology, which gets the whole sentence", () => {
  render(
    <SettingsList label="Muted servers">
      <SettingsListRow title="spam.example" result={unmuted} />
      <SettingsListRow title="ads.example" end={<Button>Unmute</Button>} />
    </SettingsList>,
  );

  const [done, other] = within(screen.getByRole("list", { name: "Muted servers" })).getAllByRole("listitem");
  if (done === undefined || other === undefined) throw new Error("test: the list has no two rows");
  // The row is still there, by its title, with no control and no group left to name one.
  expect(done.textContent).toMatch(/^spam\.example\s*Unmuted\s*You unmuted the server spam\.example\.$/);
  expect(within(done).queryAllByRole("button")).toHaveLength(0);
  expect(within(done).queryAllByRole("group")).toHaveLength(0);
  expect(isInaccessible(within(done).getByText("Unmuted"))).toBe(true);
  expect(isInaccessible(within(done).getByText("You unmuted the server spam.example."))).toBe(false);
  // Both are one element, which bears the result's id for a script to find.
  const result = document.getElementById(unmuted.id);
  expect(result?.contains(within(done).getByText("Unmuted"))).toBe(true);
  expect(result?.contains(within(done).getByText("You unmuted the server spam.example."))).toBe(true);
  // The row beside it keeps its control.
  expect(within(other).getAllByRole("button", { name: "Unmute" })).toHaveLength(1);
});

// ---- An account's row -----------------------------------------------------------------------------------------------

/** An account as its row takes it: its name, a link to its profile. */
const account = (name: string, to: string) => <a href={to}>{name}</a>;

test("an account's row is a list item that shows the account, then the note about it, then its controls in a group named by its label", () => {
  render(
    <SettingsList label="Muted accounts">
      <SettingsAccountItem
        account={account("Kai", "/@kai")}
        label="@kai"
        note="Notifications hidden too"
        end={<Button>Unmute</Button>}
      />
      <SettingsAccountItem account={account("Mio 澪", "/@mio")} label="@mio" end={<Button>Unmute</Button>} />
    </SettingsList>,
  );

  const [kai, mio] = within(screen.getByRole("list", { name: "Muted accounts" })).getAllByRole("listitem");
  if (kai === undefined || mio === undefined) throw new Error("test: the list has no two rows");
  expect(kai.textContent).toMatch(/^Kai\s*Notifications hidden too\s*Unmute$/);
  expect(within(kai).getByRole("link", { name: "Kai" }).getAttribute("href")).toBe("/@kai");
  // One Unmute is told from the next by the account its group names.
  const groups = ["@kai", "@mio"].map((label) => screen.getByRole("group", { name: label }));
  expect(groups.map((group) => within(group).getAllByRole("button", { name: "Unmute" }).length)).toEqual([1, 1]);
  expect(kai.contains(groups[0] ?? null)).toBe(true);
  expect(mio.contains(groups[1] ?? null)).toBe(true);
  // A row without a note says nothing in its place.
  expect(mio.textContent).toMatch(/^Mio 澪\s*Unmute$/);
});

test("an account's row with a result keeps the account and its link, and shows the result in place of its controls", () => {
  const unblocked = { id: "unblocked-mio", shown: "Unblocked", spoken: "You unblocked @mio." };
  render(
    <SettingsList label="Blocked accounts">
      <SettingsAccountItem account={account("Mio 澪", "/@mio")} label="@mio" result={unblocked} />
    </SettingsList>,
  );

  const row = screen.getByRole("listitem");
  expect(row.textContent).toMatch(/^Mio 澪\s*Unblocked\s*You unblocked @mio\.$/);
  expect(within(row).getByRole("link", { name: "Mio 澪" }).getAttribute("href")).toBe("/@mio");
  expect(within(row).queryAllByRole("button")).toHaveLength(0);
  expect(within(row).queryAllByRole("group")).toHaveLength(0);
  expect(isInaccessible(within(row).getByText("Unblocked"))).toBe(true);
  expect(isInaccessible(within(row).getByText("You unblocked @mio."))).toBe(false);
  expect(document.getElementById(unblocked.id)?.contains(within(row).getByText("You unblocked @mio."))).toBe(true);
});
