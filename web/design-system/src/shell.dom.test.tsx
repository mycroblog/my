// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RouterProvider } from "react-aria-components";
import { Button } from "./button";
import { MenuGroup, MenuItem } from "./menu";
import {
  CallBar,
  Frame,
  FrameAction,
  FrameActions,
  FrameAside,
  FrameBanner,
  FrameBanners,
  FrameDock,
  FrameFold,
  FrameFooter,
  FrameHeading,
  FrameMain,
  FrameMark,
  FrameSection,
  FrameSide,
  Meter,
  NavItem,
  NavList,
  NavMenu,
  SectionLine,
  SectionList,
  SectionNote,
  Toast,
} from "./shell";

/** Whether `later` comes after `earlier` in the reading order. */
const follows = (later: Element, earlier: Element) =>
  (earlier.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

/** The addresses of the images that say nothing to assistive technology, in the document's order. */
const decorations = () => screen.queryAllByRole("presentation").map((image) => image.getAttribute("src"));

test("the frame's regions are landmarks with their names, and the screen's heading is the one h1", () => {
  render(
    <Frame>
      <FrameSide label="Main">
        <NavList>
          <NavItem href="/" icon="home" label="Home" current />
        </NavList>
      </FrameSide>
      <FrameMain>
        <FrameBanners label="Server and connection status">
          <FrameBanner icon="offline">You are offline.</FrameBanner>
        </FrameBanners>
        <FrameHeading>Home</FrameHeading>
        <p>The feed</p>
      </FrameMain>
      <FrameAside label="Server activity">
        <FrameSection id="events" icon="events" href="/events" heading="Upcoming events">
          <SectionNote>No events yet.</SectionNote>
        </FrameSection>
        <FrameFooter label="This server" links={[{ href: "/about", label: "About", icon: "about" }]} />
      </FrameAside>
      <FrameDock
        label="Main"
        tabs={
          <NavList>
            <NavItem href="/" icon="home" label="Home" current />
          </NavList>
        }
      />
    </Frame>,
  );

  // The rail and the tab bar are the same navigation, each shown at its widths.
  expect(screen.getAllByRole("navigation", { name: "Main" })).toHaveLength(2);
  const main = screen.getByRole("main");
  expect(within(main).getByText("The feed")).not.toBeNull();
  expect(within(main).getByRole("region", { name: "Server and connection status" }).textContent).toBe(
    "You are offline.",
  );
  expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  expect(within(main).getByRole("heading", { level: 1, name: "Home" })).not.toBeNull();
  const aside = screen.getByRole("complementary", { name: "Server activity" });
  expect(within(aside).getByRole("region", { name: "Upcoming events" })).not.toBeNull();
  const pages = within(aside).getByRole("navigation", { name: "This server" });
  expect(within(pages).getByRole("link", { name: "About" }).getAttribute("href")).toBe("/about");
});

test("a place is a link named by its label, and the one where the reader is is the current page", () => {
  render(
    <NavList>
      <NavItem href="/" icon="home" label="Home" current />
      <NavItem href="/search" icon="search" label="Search" />
    </NavList>,
  );

  const items = within(screen.getByRole("list")).getAllByRole("listitem");
  expect(items).toHaveLength(2);
  const home = screen.getByRole("link", { name: "Home", current: "page" });
  expect(home.getAttribute("href")).toBe("/");
  const search = screen.getByRole("link", { name: "Search", current: false });
  expect(search.getAttribute("href")).toBe("/search");
  expect(screen.queryAllByRole("img")).toEqual([]);
});

test("a place with a count is named by its label and the count's label, and shows the count up to 99", () => {
  render(
    <NavList>
      <NavItem href="/notifications" icon="notifications" label="Notifications" count={3} countLabel="3 unread" />
      <NavItem href="/moderation" icon="moderation" label="Moderation" count={120} countLabel="120 waiting" />
      <NavItem href="/direct" icon="directMessages" label="Direct messages" count={0} countLabel="0 unread" />
    </NavList>,
  );

  expect(screen.getByRole("link", { name: "Notifications 3 unread" }).textContent).toContain("3");
  expect(screen.getByRole("link", { name: "Moderation 120 waiting" }).textContent).toContain("99+");
  // Zero shows no badge.
  expect(screen.getByRole("link", { name: /^Direct messages/ }).textContent).not.toMatch(/\d/);
});

test("the mark is a link to the app's start named by its label, its images decorative", () => {
  const { rerender } = render(<FrameMark href="/" label="mycroblog" light="/light.svg" dark="/dark.svg" />);
  const mark = screen.getByRole("link", { name: "mycroblog" });
  expect(mark.getAttribute("href")).toBe("/");
  expect(new Set(decorations())).toEqual(new Set(["/light.svg", "/dark.svg"]));
  expect(screen.queryAllByRole("img")).toEqual([]);

  // The server's own logo takes the wordmark's place.
  rerender(<FrameMark href="/" label="Hanami" light="/light.svg" dark="/dark.svg" logo="/hanami.svg" />);
  expect(screen.getByRole("link", { name: "Hanami" })).not.toBeNull();
  expect(decorations()).toEqual(["/hanami.svg"]);
});

test("the primary action is a button named by its label that carries its keys, and a click, Enter and Space each run it once", async () => {
  const user = userEvent.setup();
  let runs = 0;
  render(<FrameAction icon="compose" label="New post" onPress={() => runs++} aria-keyshortcuts="N" />);
  const action = screen.getByRole("button", { name: "New post" });
  expect(action.getAttribute("aria-keyshortcuts")).toBe("N");
  // An action without a menu says nothing of one.
  expect(action.hasAttribute("aria-haspopup")).toBe(false);

  await user.tab();
  expect(document.activeElement).toBe(action);
  await user.keyboard("{Enter}");
  expect(runs).toBe(1);
  await user.keyboard(" ");
  expect(runs).toBe(2);
  await user.click(action);
  expect(runs).toBe(3);
});

test("an action shown with its label, and a second one under it, are named by their labels once", () => {
  render(
    <FrameActions>
      <FrameAction icon="signIn" label="Sign in" onPress={() => undefined} labelled />
      <FrameAction icon="article" label="New article" onPress={() => undefined} secondary />
    </FrameActions>,
  );

  expect(screen.getByRole("button", { name: "Sign in" })).not.toBeNull();
  expect(screen.getByRole("button", { name: "New article" })).not.toBeNull();
});

/** The primary action with a menu of what else it can start, and what ran, in order. */
function actionWithMenu() {
  const ran: string[] = [];
  render(
    <FrameAction
      icon="compose"
      label="New post"
      onPress={() => ran.push("post")}
      menu={{
        label: "Write",
        children: (
          <MenuGroup>
            <MenuItem id="article" icon="article" onAction={() => ran.push("article")}>
              New article
            </MenuItem>
          </MenuGroup>
        ),
      }}
    />,
  );
  return ran;
}

test("an action that holds a menu says so and how to open it, and a press still runs the action", async () => {
  const user = userEvent.setup();
  const ran = actionWithMenu();
  const action = screen.getByRole("button", { name: "New post", expanded: false, description: /Alt/ });
  expect(action.getAttribute("aria-haspopup")).toBe("true");

  await user.tab();
  await user.keyboard("{Enter}");

  expect(ran).toEqual(["post"]);
  expect(screen.queryByRole("menu")).toBeNull();
});

test("Alt+Down on an action that holds a menu opens it, and Escape closes it with the focus back on the action", async () => {
  const user = userEvent.setup();
  const ran = actionWithMenu();
  await user.tab();

  await user.keyboard("{Alt>}{ArrowDown}{/Alt}");
  const menu = await screen.findByRole("menu", { name: "Write" });
  expect(within(menu).getByRole("menuitem", { name: "New article" })).not.toBeNull();

  await user.keyboard("{Escape}");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "New post" }));
  expect(ran).toEqual([]);
});

test("a right click on an action that holds a menu opens it and does not run the action", async () => {
  const user = userEvent.setup();
  const ran = actionWithMenu();

  await user.pointer({ keys: "[MouseRight]", target: screen.getByRole("button", { name: "New post" }) });

  expect(await screen.findByRole("menu", { name: "Write" })).not.toBeNull();
  expect(ran).toEqual([]);
});

/** The Menu of the tab bar: two places, the second the current one with a count, and the server's pages after them. */
function tabBarMenu(props: { marked?: boolean; count?: number; markLabel?: string } = {}) {
  render(
    <NavList>
      <NavMenu
        label="Menu"
        {...props}
        footer={<FrameFooter label="This server" links={[{ href: "/about", label: "About", icon: "about" }]} />}
      >
        <NavList>
          <NavItem href="/settings" icon="settings" label="Settings" />
          <NavItem
            href="/direct"
            icon="directMessages"
            label="Direct messages"
            current
            count={2}
            countLabel="2 unread"
          />
        </NavList>
      </NavMenu>
    </NavList>,
  );
}

test("the Menu's place is a button that says it opens a menu and is closed", () => {
  tabBarMenu();

  const place = screen.getByRole("button", { name: "Menu", expanded: false });
  expect(place.getAttribute("aria-haspopup")).toBe("true");
  expect(screen.queryByRole("menu")).toBeNull();
});

test("the Menu's place is named by its label and, while something inside waits, by the mark's label", () => {
  tabBarMenu({ marked: true, markLabel: "Something waits" });
  expect(screen.getByRole("button", { name: "Menu Something waits" })).not.toBeNull();
});

test("the Menu's place with a count is named by its label and the count's label, and shows the count", () => {
  tabBarMenu({ count: 2, markLabel: "2 unread" });
  expect(screen.getByRole("button", { name: "Menu 2 unread" }).textContent).toContain("2");
});

test("Enter on the Menu's place opens a menu of its places and the server's pages, as items that lead there", async () => {
  const user = userEvent.setup();
  tabBarMenu();

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Menu" }));
  await user.keyboard("{Enter}");

  const menu = await screen.findByRole("menu", { name: "Menu" });
  const items = within(menu).getAllByRole("menuitem");
  expect(items.map((item) => item.getAttribute("href"))).toEqual(["/settings", "/direct", "/about"]);
  expect(within(menu).getByRole("menuitem", { name: "Settings" })).toBe(items[0] as HTMLElement);
  // The place where the reader is, with how many wait there.
  const current = within(menu).getByRole("menuitem", { name: "Direct messages 2 unread", current: "page" });
  expect(current).toBe(items[1] as HTMLElement);
  expect(within(menu).getByRole("menuitem", { name: "About" })).toBe(items[2] as HTMLElement);
});

test("a click on the Menu's place opens its menu, and Escape closes it with the focus back on the place", async () => {
  const user = userEvent.setup();
  tabBarMenu();

  await user.click(screen.getByRole("button", { name: "Menu" }));
  await screen.findByRole("menu", { name: "Menu" });
  await user.keyboard("{Escape}");

  expect(screen.queryByRole("menu")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Menu", expanded: false }));
});

/** The bar of a call in the room "Coffee", and what the reader did in it, in order. */
function callBar(muted: boolean) {
  const did: string[] = [];
  render(
    <CallBar
      href="/voice/1"
      room="Coffee"
      detail="Hosted by Mika · 12 in the room"
      muted={muted}
      muteLabel="Mute"
      onMute={() => did.push("mute")}
      leaveLabel="Leave"
      onLeave={() => did.push("leave")}
    />,
  );
  return did;
}

test("the call's bar is a group named by its room, whose name and detail lead to the room's screen", () => {
  callBar(false);

  const bar = screen.getByRole("group", { name: "Coffee" });
  const room = within(bar).getByRole("link", { name: "Coffee Hosted by Mika · 12 in the room" });
  expect(room.getAttribute("href")).toBe("/voice/1");
});

test("the call's microphone is a toggle named by its label, on while muted, and leaving comes last", async () => {
  const user = userEvent.setup();
  const did = callBar(true);
  const bar = screen.getByRole("group", { name: "Coffee" });
  const mute = within(bar).getByRole("button", { name: "Mute", pressed: true });
  const leave = within(bar).getByRole("button", { name: "Leave" });

  await user.tab();
  expect(document.activeElement).toBe(within(bar).getByRole("link"));
  await user.tab();
  expect(document.activeElement).toBe(mute);
  await user.keyboard(" ");
  await user.tab();
  expect(document.activeElement).toBe(leave);
  await user.keyboard("{Enter}");

  expect(did).toEqual(["mute", "leave"]);
});

test("the call's microphone is off while the reader is not muted, and a click asks to mute", async () => {
  const user = userEvent.setup();
  const did = callBar(false);

  await user.click(screen.getByRole("button", { name: "Mute", pressed: false }));
  await user.click(screen.getByRole("button", { name: "Leave" }));

  expect(did).toEqual(["mute", "leave"]);
});

test("while the call connects again its bar is busy, and while its sound is held back hearing comes first", async () => {
  const user = userEvent.setup();
  const did: string[] = [];
  render(
    <CallBar
      href="/voice/1"
      room="Coffee"
      detail="Reconnecting…"
      busy
      hear={{ label: "Tap to hear", onPress: () => did.push("hear") }}
      muted={false}
      muteLabel="Mute"
      onMute={() => did.push("mute")}
      leaveLabel="Leave"
      onLeave={() => did.push("leave")}
    />,
  );

  const bar = screen.getByRole("group", { name: "Coffee" });
  expect(bar.getAttribute("aria-busy")).toBe("true");
  expect(within(bar).getByRole("link").textContent).toBe("Coffee Reconnecting…");
  await user.tab();
  await user.tab();
  expect(document.activeElement).toBe(within(bar).getByRole("button", { name: "Tap to hear" }));
  await user.keyboard("{Enter}");
  await user.tab();
  expect(document.activeElement).toBe(within(bar).getByRole("button", { name: "Mute" }));

  expect(did).toEqual(["hear"]);
});

test("a call's bar that connects is not busy and holds no hearing back", () => {
  callBar(false);

  const bar = screen.getByRole("group", { name: "Coffee" });
  expect(bar.hasAttribute("aria-busy")).toBe(false);
  expect(
    within(bar)
      .getAllByRole("button")
      .map((b) => b.textContent),
  ).toEqual(["Mute", "Leave"]);
});

test("a toast is a status that says its words, with its action and its close", async () => {
  const user = userEvent.setup();
  const did: string[] = [];
  render(
    <Toast action="Update" onAction={() => did.push("update")} closeLabel="Close" onClose={() => did.push("close")}>
      A newer version of the app is ready.
    </Toast>,
  );

  const toast = screen.getByRole("status");
  expect(toast.textContent).toContain("A newer version of the app is ready.");
  await user.tab();
  expect(document.activeElement).toBe(within(toast).getByRole("button", { name: "Update" }));
  await user.keyboard("{Enter}");
  await user.tab();
  expect(document.activeElement).toBe(within(toast).getByRole("button", { name: "Close" }));
  await user.keyboard("{Enter}");

  expect(did).toEqual(["update", "close"]);
});

test("a toast that cannot be closed has its action alone", () => {
  render(
    <Toast action="Update" onAction={() => undefined}>
      Update the app to go on.
    </Toast>,
  );

  expect(within(screen.getByRole("status")).getAllByRole("button")).toHaveLength(1);
  expect(screen.getByRole("button", { name: "Update" })).not.toBeNull();
});

test("a meter is a meter named by its label that reads the share, with what it measures beside it", () => {
  render(<Meter value={0.72} label="Share of this month's costs covered" caption="of November's costs covered" />);

  const meter = screen.getByRole("meter", { name: "Share of this month's costs covered" });
  expect(meter.getAttribute("aria-valuemin")).toBe("0");
  expect(meter.getAttribute("aria-valuemax")).toBe("100");
  expect(meter.getAttribute("aria-valuenow")).toBe("72");
  expect(meter.getAttribute("aria-valuetext")).toBe("72%");
  expect(screen.getByText("72%")).not.toBeNull();
  expect(screen.getByText("of November's costs covered")).not.toBeNull();
});

test("a meter beyond its goal keeps its value inside its range and reads the share", () => {
  render(<Meter value={1.2} label="Share of this month's costs covered" caption="of November's costs covered" />);

  const meter = screen.getByRole("meter", { name: "Share of this month's costs covered" });
  expect(meter.getAttribute("aria-valuetext")).toBe("120%");
  expect(Number(meter.getAttribute("aria-valuenow"))).toBeLessThanOrEqual(Number(meter.getAttribute("aria-valuemax")));
});

test("a banner says its notice, and its link to another site opens a new tab, says so, and gives the new page no way back", () => {
  render(
    <FrameBanners label="Server and connection status">
      <FrameBanner
        icon="caution"
        strong
        links={[
          { href: "/settings/export", label: "Export your data" },
          { href: "https://status.example/", label: "Service status", newTab: "(opens in a new tab)" },
        ]}
      >
        This server closes on March 31, 2027.
      </FrameBanner>
    </FrameBanners>,
  );

  const banners = screen.getByRole("region", { name: "Server and connection status" });
  expect(within(banners).getByText("This server closes on March 31, 2027.")).not.toBeNull();
  const here = within(banners).getByRole<HTMLAnchorElement>("link", { name: "Export your data" });
  expect(here.getAttribute("href")).toBe("/settings/export");
  expect(here.target).toBe("");
  const away = within(banners).getByRole<HTMLAnchorElement>("link", { name: "Service status (opens in a new tab)" });
  expect(away.getAttribute("href")).toBe("https://status.example/");
  expect(away.target).toBe("_blank");
  expect(away.rel.split(" ")).toContain("noopener");
});

test("a section is a region named by its heading alone, which links to its place, and its link to all of it has its own name", () => {
  render(
    <FrameSection
      id="announcements"
      icon="announcements"
      href="/announcements"
      heading="Announcements"
      all={{ text: "All", name: "All announcements" }}
    >
      <SectionNote>No announcements yet.</SectionNote>
    </FrameSection>,
  );

  const section = screen.getByRole("region", { name: "Announcements" });
  const heading = within(section).getByRole("heading", { level: 2, name: "Announcements" });
  expect(within(heading).getByRole("link", { name: "Announcements" }).getAttribute("href")).toBe("/announcements");
  const all = within(section).getByRole("link", { name: "All announcements" });
  expect(all.textContent).toBe("All");
  expect(all.getAttribute("href")).toBe("/announcements");
  expect(follows(all, heading)).toBe(true);
});

test("a section that shows all it has ends its heading with no link", () => {
  render(
    <FrameSection id="events" icon="events" href="/events" heading="Upcoming events">
      <SectionNote>No events yet.</SectionNote>
    </FrameSection>,
  );

  expect(within(screen.getByRole("region", { name: "Upcoming events" })).getAllByRole("link")).toHaveLength(1);
});

test("a section folded to one line is one link to its place, named by what it holds", () => {
  render(
    <FrameFold icon="announcements" href="/announcements">
      2 unread announcements
    </FrameFold>,
  );

  const links = screen.getAllByRole("link");
  expect(links).toHaveLength(1);
  expect(screen.getByRole("link", { name: "2 unread announcements" }).getAttribute("href")).toBe("/announcements");
});

test("a section's lines are a list, each a link by its title with its caption, marked while unread", async () => {
  const user = userEvent.setup();
  const followed: string[] = [];
  const went: string[] = [];
  render(
    <RouterProvider navigate={(href) => went.push(href)}>
      <SectionList>
        <SectionLine
          href="/@ren"
          icon="follow"
          title="Ren followed you"
          caption="1h ago"
          unread="Unread"
          onPress={() => followed.push("ren")}
        />
        <SectionLine href="/events/1" title="Fediverse meetup in Tokyo" caption="Sat, Nov 21, 19:00" read />
      </SectionList>
    </RouterProvider>,
  );

  const [unread, read] = within(screen.getByRole("list")).getAllByRole("listitem") as [HTMLElement, HTMLElement];
  const title = within(unread).getByRole("link", { name: "Ren followed you" });
  expect(title.getAttribute("href")).toBe("/@ren");
  expect(follows(within(unread).getByText("1h ago"), title)).toBe(true);
  expect(follows(within(unread).getByRole("img", { name: "Unread" }), title)).toBe(true);
  expect(within(read).getByRole("link", { name: "Fediverse meetup in Tokyo" }).getAttribute("href")).toBe("/events/1");
  expect(within(read).getByText("Sat, Nov 21, 19:00")).not.toBeNull();
  expect(within(read).queryByRole("img")).toBeNull();

  await user.click(title);
  expect(followed).toEqual(["ren"]);
  // It goes there through the app's navigation.
  expect(went).toEqual(["/@ren"]);
});

test("a section's note says what it shows in place of its lines, then what the reader may do about it", async () => {
  const user = userEvent.setup();
  let tries = 0;
  render(
    <SectionNote action={<Button onPress={() => tries++}>Try again</Button>}>Could not load the events.</SectionNote>,
  );

  const note = screen.getByText("Could not load the events.");
  const again = screen.getByRole("button", { name: "Try again" });
  expect(follows(again, note)).toBe(true);
  await user.click(again);
  expect(tries).toBe(1);
});

test("the dock holds the tab bar under its name, the call's bar, the primary action and the toasts", () => {
  render(
    <>
      <FrameDock
        label="Main"
        tabs={
          <NavList>
            <NavItem href="/" icon="home" label="Home" current />
          </NavList>
        }
        call={
          <CallBar
            href="/voice/1"
            room="Coffee"
            detail="12 in the room"
            muted={false}
            muteLabel="Mute"
            onMute={() => undefined}
            leaveLabel="Leave"
            onLeave={() => undefined}
          />
        }
        action={<FrameAction icon="compose" label="New post" onPress={() => undefined} />}
        toasts={
          <Toast action="Update" onAction={() => undefined}>
            A newer version of the app is ready.
          </Toast>
        }
      />
    </>,
  );

  const tabs = screen.getByRole("navigation", { name: "Main" });
  expect(within(tabs).getByRole("link", { name: "Home", current: "page" })).not.toBeNull();
  expect(screen.getByRole("group", { name: "Coffee" })).not.toBeNull();
  expect(screen.getByRole("button", { name: "New post" })).not.toBeNull();
  expect(screen.getByRole("status").textContent).toContain("A newer version of the app is ready.");
});
