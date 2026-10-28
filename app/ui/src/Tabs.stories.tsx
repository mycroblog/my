// SPDX-License-Identifier: AGPL-3.0-only
// Tabs stories. The play functions test the arrow keys in the reading direction (in ar-XB, Left moves
// to the next tab), controlled selection, the way a route binds the selected tab to the URL, and tabs that
// do not fit: the list scrolls the focused or selected tab into view and fades out where tabs are hidden.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { Key } from "react-aria-components";
import { expect, fn, waitFor } from "storybook/test";
import { Tab, TabList, TabPanel, Tabs, type TabsProps } from "./Tabs";

const timelines = [
  { id: "home", label: "Home", body: "Posts from the people you follow." },
  { id: "local", label: "Local", body: "Public posts from this instance." },
  { id: "hashtags", label: "Hashtags", body: "Posts with the hashtags you follow." },
  { id: "lists", label: "Lists", body: "Your lists." },
] as const;

function Timelines(props: Omit<TabsProps, "children">) {
  return (
    <Tabs {...props}>
      <TabList aria-label="Timelines">
        {timelines.map(({ id, label }) => (
          <Tab key={id} id={id} isDisabled={id === "lists"}>
            {label}
          </Tab>
        ))}
      </TabList>
      {timelines.map(({ id, body }) => (
        <TabPanel key={id} id={id}>
          <p>{body}</p>
        </TabPanel>
      ))}
    </Tabs>
  );
}

/** The keys that move to the next and the previous tab in the page's reading direction. */
function arrowKeys() {
  return document.documentElement.dir === "rtl"
    ? { next: "{ArrowLeft}", previous: "{ArrowRight}" }
    : { next: "{ArrowRight}", previous: "{ArrowLeft}" };
}

const meta = {
  title: "Navigation/Tabs",
  component: Timelines,
} satisfies Meta<typeof Timelines>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const ArrowKeys: Story = {
  play: async ({ canvas, userEvent }) => {
    const keys = arrowKeys();
    await userEvent.tab();
    await expect(canvas.getByRole("tab", { name: "Home" })).toHaveFocus();

    await userEvent.keyboard(keys.next);
    const local = canvas.getByRole("tab", { name: "Local" });
    await expect(local).toHaveFocus();
    await expect(local).toHaveAttribute("aria-selected", "true");
    await expect(canvas.getByRole("tabpanel")).toHaveTextContent("Public posts from this instance.");

    // The disabled last tab is skipped, so moving on wraps around to the first.
    await userEvent.keyboard(keys.next);
    await userEvent.keyboard(keys.next);
    await expect(canvas.getByRole("tab", { name: "Home" })).toHaveFocus();
    await userEvent.keyboard(keys.previous);
    await expect(canvas.getByRole("tab", { name: "Hashtags" })).toHaveFocus();
    await userEvent.keyboard("{Home}");
    await expect(canvas.getByRole("tab", { name: "Home" })).toHaveAttribute("aria-selected", "true");
  },
};

/**
 * The selection lives outside, as a route keeps it in the search params: the tabs show `selectedKey`
 * and report a change through `onSelectionChange`, which updates the "URL".
 */
function UrlBoundTabs({ onNavigate }: { onNavigate: (key: Key) => void }) {
  const [tab, setTab] = useState<Key>("local");
  return (
    <div className="flex flex-col gap-2">
      <code className="font-mono text-xs text-fg-muted">?tab={String(tab)}</code>
      <Timelines
        selectedKey={tab}
        onSelectionChange={(key) => {
          onNavigate(key);
          setTab(key);
        }}
      />
    </div>
  );
}

export const Controlled: StoryObj<typeof UrlBoundTabs> = {
  args: { onNavigate: fn() },
  render: (args) => <UrlBoundTabs {...args} />,
  play: async ({ args, canvas, userEvent }) => {
    await expect(canvas.getByRole("tab", { name: "Local" })).toHaveAttribute("aria-selected", "true");
    await userEvent.click(canvas.getByRole("tab", { name: "Hashtags" }));
    await expect(args.onNavigate).toHaveBeenCalledWith("hashtags");
    await expect(canvas.getByText("?tab=hashtags")).toBeInTheDocument();
    await expect(canvas.getByRole("tab", { name: "Hashtags" })).toHaveAttribute("aria-selected", "true");
  },
};

const types = ["All", "Mentions", "Follows", "Boosts", "Favourites", "Polls", "Edits"] as const;

/** More tabs than fit in a 288 px column, as the notification types on a 320 px phone. */
function NarrowTabs({ defaultSelectedKey }: { defaultSelectedKey?: (typeof types)[number] }) {
  return (
    <div className="max-w-72">
      <Tabs {...(defaultSelectedKey === undefined ? {} : { defaultSelectedKey })}>
        <TabList aria-label="Notification types">
          {types.map((type) => (
            <Tab key={type} id={type}>
              {type}
            </Tab>
          ))}
        </TabList>
        {types.map((type) => (
          <TabPanel key={type} id={type}>
            <p>{type}</p>
          </TabPanel>
        ))}
      </Tabs>
    </div>
  );
}

/**
 * Whether the tab lies inside the list and clear of its fades: each is as wide as the list's scroll-padding,
 * on an inline edge with tabs behind it. The scroll position says which edges those are right now; the
 * list's data-overflow-* attributes follow it a scroll event later.
 */
function isClearOfFades(list: HTMLElement, tab: HTMLElement) {
  const style = getComputedStyle(list);
  const rtl = style.direction === "rtl";
  const offset = Math.abs(list.scrollLeft);
  const start = offset > 1;
  const end = list.scrollWidth - list.clientWidth - offset > 1;
  const fadeLeft = rtl ? end : start;
  const fadeRight = rtl ? start : end;
  const listRect = list.getBoundingClientRect();
  const tabRect = tab.getBoundingClientRect();
  const left = listRect.left + (fadeLeft ? Number.parseFloat(style.scrollPaddingLeft) : 0);
  const right = listRect.right - (fadeRight ? Number.parseFloat(style.scrollPaddingRight) : 0);
  // A pixel of slack for fractional positions.
  return tabRect.left >= left - 1 && tabRect.right <= right + 1;
}

/**
 * The tabs scroll sideways inside the list, which fades out at the edge behind which tabs are hidden.
 * Each arrow key moves to the tab on its side (in ar-XB, Left is the next tab and lies to the left), and
 * the list scrolls the same way, so that the focused tab is whole and clear of the fades.
 */
export const Overflow: StoryObj<typeof NarrowTabs> = {
  render: (args) => <NarrowTabs {...args} />,
  play: async ({ canvas, userEvent }) => {
    const keys = arrowKeys();
    const rtl = document.documentElement.dir === "rtl";
    const list = canvas.getByRole("tablist");
    await expect(list.scrollWidth).toBeGreaterThan(list.clientWidth);
    // The list measures itself when its ResizeObserver first reports, before the first paint.
    await waitFor(() => expect(list).toHaveAttribute("data-overflow-end"));
    await expect(list).not.toHaveAttribute("data-overflow-start");

    await userEvent.tab();
    let previous = canvas.getByRole("tab", { name: "All" });
    await expect(previous).toHaveFocus();
    for (const name of types.slice(1)) {
      await userEvent.keyboard(keys.next);
      const tab = canvas.getByRole("tab", { name });
      await expect(tab).toHaveFocus();
      await expect(tab).toHaveAttribute("aria-selected", "true");
      // The next tab lies on the side of the key that was pressed.
      if (rtl)
        await expect(tab.getBoundingClientRect().right).toBeLessThanOrEqual(previous.getBoundingClientRect().left + 1);
      else
        await expect(tab.getBoundingClientRect().left).toBeGreaterThanOrEqual(
          previous.getBoundingClientRect().right - 1,
        );
      await expect(isClearOfFades(list, tab)).toBe(true);
      previous = tab;
    }
    // At the end, tabs are hidden only behind the inline start.
    await waitFor(() => expect(list).not.toHaveAttribute("data-overflow-end"));
    await expect(list).toHaveAttribute("data-overflow-start");

    await userEvent.keyboard("{Home}");
    const first = canvas.getByRole("tab", { name: "All" });
    await expect(first).toHaveFocus();
    await expect(isClearOfFades(list, first)).toBe(true);
    await waitFor(() => expect(list).not.toHaveAttribute("data-overflow-start"));
    await expect(list).toHaveAttribute("data-overflow-end");
  },
};

/** A tab selected from outside (the URL) that starts out hidden scrolls into view on the first render. */
export const SelectedOutOfView: StoryObj<typeof NarrowTabs> = {
  args: { defaultSelectedKey: "Edits" },
  render: (args) => <NarrowTabs {...args} />,
  play: async ({ canvas }) => {
    const list = canvas.getByRole("tablist");
    await waitFor(() => expect(list).toHaveAttribute("data-overflow-start"));
    await expect(list).not.toHaveAttribute("data-overflow-end");
    await expect(isClearOfFades(list, canvas.getByRole("tab", { name: "Edits" }))).toBe(true);
  },
};

/** A panel whose sections sit one above another (`gap="8"`), with more room under the tab list (`offset="4"`). */
export const PanelLayout: Story = {
  render: () => (
    <Tabs>
      <TabList aria-label="Custom emoji">
        <Tab id="emoji">Emoji</Tab>
        <Tab id="requests">Requests</Tab>
      </TabList>
      <TabPanel id="emoji" gap="8" offset="4">
        <p>Add an emoji: its shortcode, its image and its category.</p>
        <p>The emoji of this server, a-very-long-category-name-without-any-spaces-that-wraps-anywhere.</p>
      </TabPanel>
      <TabPanel id="requests" gap="8" offset="4">
        <p>Requests from supporters.</p>
      </TabPanel>
    </Tabs>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("tabpanel", { name: "Emoji" })).toHaveClass("gap-8", "pt-4");
  },
};
