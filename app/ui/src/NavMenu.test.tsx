// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import { describe, expect, test } from "vitest";
import { AccountsIcon, ReportIcon, ServerSettingsIcon } from "./Icons";
import { NavMenu, type NavMenuItem, navMenuGroupLimit } from "./NavMenu";
import { RailItem } from "./RailItem";
import { expectValidNavMenus, navMenuProblems, watchNavMenus } from "./testing";

const pages = (count: number): NavMenuItem[] =>
  Array.from({ length: count }, (_, index) => ({
    href: `#page-${index}`,
    icon: <ReportIcon />,
    label: `Page ${index}`,
  }));

describe("NavMenu", () => {
  test("names each group by its heading and shows its pages with their icons and counts", () => {
    render(
      <NavMenu
        groups={[
          {
            id: "moderation",
            heading: "Moderation",
            items: [
              {
                href: "#reports",
                icon: <ReportIcon />,
                label: "Reports",
                current: true,
                badge: { count: 3, label: "3 open", name: "Reports 3 open" },
              },
              { href: "#accounts", icon: <AccountsIcon />, label: "Accounts" },
            ],
          },
          { id: "empty", heading: "Empty", items: [] },
          {
            id: "admin",
            heading: "Administration",
            items: [{ href: "#settings", icon: <ServerSettingsIcon />, label: "Server settings" }],
          },
        ]}
      />,
    );

    const moderation = screen.getByRole("group", { name: "Moderation" });
    expect(within(moderation).getByRole("link", { name: "Reports 3 open" })).toHaveAttribute("aria-current", "page");
    expect(within(moderation).getByRole("link", { name: "Accounts" })).not.toHaveAttribute("aria-current");
    expect(within(screen.getByRole("group", { name: "Administration" })).getAllByRole("link")).toHaveLength(1);
    // A group without pages is left out, and the groups are not headings of the page beside the menu.
    expect(screen.queryByRole("group", { name: "Empty" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading")).not.toBeInTheDocument();
    expect(navMenuProblems()).toEqual([]);
  });
});

describe("the check of the grouped menus", () => {
  test("accepts a group of the limit's size", () => {
    render(<NavMenu groups={[{ id: "full", heading: "Full", items: pages(navMenuGroupLimit) }]} />);

    expect(() => {
      expectValidNavMenus();
    }).not.toThrow();
  });

  test("finds a group over the limit", () => {
    render(<NavMenu groups={[{ id: "long", heading: "Long", items: pages(navMenuGroupLimit + 1) }]} />);

    expect(navMenuProblems()).toEqual(['the group "Long" has 8 pages, more than 7']);
    expect(() => {
      expectValidNavMenus();
    }).toThrow(/at most 7 pages a group/);
  });

  test("finds a page without an icon", () => {
    // The type asks for an icon; a menu built by hand around the same markup is checked too.
    render(
      <div role="group" aria-labelledby="heading" data-nav-menu-group="">
        <p id="heading">Security</p>
        <ul>
          <li>
            <RailItem form="section" href="#password" icon={null} label="Password" />
          </li>
        </ul>
      </div>,
    );

    expect(navMenuProblems()).toEqual(['the page "Password" of the group "Security" has no icon']);
  });

  test("the watch catches a menu that was shown and taken away during the test", async () => {
    const watch = watchNavMenus();
    const { unmount } = render(
      <NavMenu groups={[{ id: "long", heading: "Long", items: pages(navMenuGroupLimit + 1) }]} />,
    );
    // The observer reports the change after the current task's microtasks.
    await Promise.resolve();
    unmount();

    expect(() => {
      watch.stop();
    }).toThrow(/"Long" has 8 pages/);
  });
});
