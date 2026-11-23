// SPDX-License-Identifier: AGPL-3.0-only

// An event whose organizer the reader may not see (#3387), on the fake API: the server names the organizer as an
// unavailable account, a stub with no name and no profile. The event's page says who sees it without a name, and lists
// the organizer among its organizers as an unavailable account with no link to a profile, in English and in Japanese.
// What it looks like is the stories' (events.stories.tsx, OrganizerUnavailable).

import { scenario, type FakeOptions } from "@my/e2e/fake";
import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words } from "./test/app";
import { fakeApp } from "./test/server";

/** The scenario with the organizer of the class the reader co-organizes (sam, for their followers) unavailable. */
const organizerUnavailable: FakeOptions = {
  seed: {
    ...scenario,
    events: scenario.events.map((plan) =>
      plan.key === "bread-class" ? { ...plan, organizerUnavailable: true } : plan,
    ),
  },
};

for (const [locale, said] of [
  ["en", { visibility: "Shown to the followers of the organizer", name: "Unavailable account" }],
  ["ja", { visibility: "主催者のフォロワーに公開", name: "表示できないアカウント" }],
] as const)
  test(`the event's page says who sees it and lists the unavailable organizer with no profile link, in ${locale}`, async () => {
    const on = fakeApp({
      ...organizerUnavailable,
      preferences: { locale, locale_follows_browser: false },
    });
    await renderApp(`/events/${on.fake.events.id("bread-class")}`, { on, locale });
    await main().findByRole("heading", { name: "Sourdough class", level: 2 });
    await waitFor(() => {
      if (!words(screen.getByRole("main")).includes(said.visibility))
        throw new Error(`test: the page says ${words(screen.getByRole("main"))}`);
    });

    // The organizers' list holds a row of the unavailable account, which leads nowhere, and no link to sam's profile.
    const lists = await waitFor(() => {
      const found = main()
        .getAllByRole("list")
        .filter((list) => words(list).includes(said.name));
      if (found.length === 0) throw new Error("test: no list names the unavailable organizer");
      return found;
    });
    for (const list of lists) {
      const rows = within(list)
        .getAllByRole("listitem")
        .filter((row) => words(row).includes(said.name));
      expect(rows.length).toBeGreaterThan(0);
      for (const row of rows) expect(within(row).queryAllByRole("link")).toHaveLength(0);
      const hrefs = within(list)
        .queryAllByRole("link")
        .map((link) => link.getAttribute("href") ?? "");
      expect(hrefs.filter((href) => href.startsWith("/@sam"))).toEqual([]);
      expect(words(list)).not.toContain("Sam Rivera");
    }
  });
