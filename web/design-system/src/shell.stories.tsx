// SPDX-License-Identifier: AGPL-3.0-only

// The app's frame in every state of the shell (ADR-0110 §11, ADR-0125 §4, ADR-0088, ADR-0028 §11–§12), a story per
// state, whole and edge to edge, so that each reads at the phone, tablet and PC widths of the toolbar's viewport: a
// reader at home with every section around them; a moderator, whose places end with moderation (its count), admin and
// finance; a server with its local and federated timelines off; a single-user server; a visitor not signed in; the
// closure announced; the reader offline and the server down; a call; the toasts of a newer version; the column beside
// the main one with none of its sections but the notifications, which have none yet; and the menu's sheet open. Its words are samples in English.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { userEvent, waitFor, within } from "storybook/test";
import { Avatar } from "./avatar";
import { Button } from "./button";
import { Byline, Entry, Stamp } from "./entry";
import { FormActions, FormHead, FormLink, FormLinks, FormPage, FormPageFooter, FormText } from "./form-page";
import { images } from "./samples";
import {
  CallBar,
  Frame,
  FrameAction,
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
import type { IconName } from "./icons";

// A stand-in for the brand's wordmark, a short word about twice as wide as it is tall.
const wordmark = (fill: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 44 24" width="44" height="24"><text x="2" y="19" font-family="system-ui, sans-serif" font-size="21" font-style="italic" font-weight="800" fill="${fill}">my</text></svg>`)}`;

type Who = "reader" | "staff" | "visitor";
type At = "/" | "/local" | "/public" | "/notifications" | "/search" | "/@yuki" | "/settings";

/** A state of the shell: who reads, what the server offers, and what crosses the screens now. */
type ShellState = {
  who?: Who;
  at?: At;
  /** The timelines the server offers besides home. */
  timelines?: ("local" | "federated")[];
  singleUser?: boolean;
  /** The server's own logo in place of the wordmark. */
  logo?: boolean;
  closure?: boolean;
  connection?: "offline" | "outage";
  call?: boolean;
  /** The reader's microphone muted in the call. */
  muted?: boolean;
  toast?: "stale" | "required";
  /** The sections of the column beside the main one: all, some (rooms and the meter left out), or none. */
  around?: "all" | "some" | "none";
};

type Place = {
  href:
    | At
    | "/bookmarks"
    | "/moderation"
    | "/admin"
    | "/finance"
    | "/announcements"
    | "/voice"
    | "/events"
    | "/about#funding"
    | "/login";
  icon: IconName;
  label: string;
};

const unreadNotifications = 3;
const reportsToHandle = 12;

function places({ who = "reader", timelines = ["local", "federated"], singleUser = false }: ShellState) {
  const offered = singleUser ? [] : timelines;
  const timeline: Place[] = [
    ...(offered.includes("local") ? [{ href: "/local", icon: "local", label: "Local" } as const] : []),
    ...(offered.includes("federated") ? [{ href: "/public", icon: "federated", label: "Federated" } as const] : []),
  ];
  if (who === "visitor")
    return {
      tabs: [...timeline, { href: "/search", icon: "search", label: "Search" }] as Place[],
      rest: [] as Place[],
      rights: [] as Place[],
    };
  return {
    tabs: [
      { href: "/", icon: "home", label: "Home" },
      ...timeline,
      { href: "/notifications", icon: "notifications", label: "Notifications" },
      { href: "/search", icon: "search", label: "Search" },
    ] as Place[],
    rest: [
      { href: "/@yuki", icon: "profile", label: "Profile" },
      { href: "/bookmarks", icon: "bookmark", label: "Bookmarks" },
      { href: "/settings", icon: "settings", label: "Settings" },
    ] as Place[],
    rights:
      who === "staff"
        ? ([
            { href: "/moderation", icon: "moderation", label: "Moderation" },
            { href: "/admin", icon: "admin", label: "Admin" },
            { href: "/finance", icon: "finance", label: "Finance" },
          ] as Place[])
        : [],
  };
}

function Item({ place, at }: { place: Place; at: At }) {
  const count =
    place.href === "/notifications" ? unreadNotifications : place.href === "/moderation" ? reportsToHandle : undefined;
  return (
    <NavItem
      href={place.href}
      icon={place.icon}
      label={place.label}
      current={place.href === at}
      count={count}
      countLabel={
        count === undefined ? undefined : `${String(count)} ${place.href === "/notifications" ? "unread" : "waiting"}`
      }
    />
  );
}

const footer = (
  <FrameFooter
    label="This server"
    links={[
      { href: "/about", label: "About" },
      { href: "/terms", label: "Terms" },
      { href: "/privacy", label: "Privacy" },
    ]}
  />
);

const announcements = [
  { id: "a1", title: "Scheduled maintenance on Sunday, 03:00–04:00 UTC", unread: true, when: "2h ago" },
  { id: "a2", title: "Custom emoji requests are open", unread: true, when: "1d ago" },
  { id: "a3", title: "Welcome to the server: read the rules before you post", unread: false, when: "12d ago" },
];

/** The unread announcements alone; the menu leads to all of them. */
function Announcements() {
  return (
    <FrameSection
      id="around-announcements"
      icon="announcements"
      href="/announcements"
      heading="Announcements"
      all={{ text: "All", name: "All announcements" }}
    >
      <SectionList>
        {announcements
          .filter((a) => a.unread)
          .map((a) => (
            <SectionLine key={a.id} href={`/announcements/${a.id}`} title={a.title} caption={a.when} unread="Unread" />
          ))}
      </SectionList>
    </FrameSection>
  );
}

/** The latest notifications, the unread marked; with none, the section says so. */
function Notifications({ none = false }: { none?: boolean }) {
  return (
    <FrameSection
      id="around-notifications"
      icon="notifications"
      href="/notifications"
      heading="Notifications"
      all={{ text: "All", name: "All notifications" }}
    >
      {none ? (
        <SectionNote>No notifications yet.</SectionNote>
      ) : (
        <SectionList>
          <SectionLine
            href="/@mika/status/1"
            icon="favourite"
            title={
              <>
                <strong>Mika</strong> favorited your post
              </>
            }
            caption="5m ago"
            unread="Unread"
          />
          <SectionLine
            href="/@ren"
            icon="follow"
            title={
              <>
                <strong>Ren</strong> followed you
              </>
            }
            caption="1h ago"
            unread="Unread"
          />
          <SectionLine
            href="/@sora/status/2"
            icon="boost"
            title={
              <>
                <strong>Sora with a display name long enough to wrap</strong> boosted your post
              </>
            }
            caption="3h ago"
          />
          <SectionLine
            href="/events/1"
            icon="events"
            title="Fediverse meetup in Tokyo starts in 1 day"
            caption="1d ago"
          />
        </SectionList>
      )}
    </FrameSection>
  );
}

function Around({ who = "reader", at = "/", around = "all" }: ShellState) {
  const reader = who !== "visitor";
  const all = around === "all";
  return (
    <FrameAside label="Server activity">
      {around !== "none" && reader && <Announcements />}
      {all && reader && (
        <FrameSection
          id="around-rooms"
          icon="room"
          href="/voice"
          heading="Voice rooms"
          all={{ text: "All", name: "All voice rooms" }}
        >
          <SectionList>
            <SectionLine href="/voice/1" title="Morning coffee chat" caption="Hosted by Mika · 12 in the room" />
            <SectionLine href="/voice/2" title="Release party for v0.2" caption="Hosted by Ren · 48 in the room" />
            <SectionLine
              href="/voice/3"
              title="A room with a name long enough to wrap onto a second line in this column"
              caption="Hosted by Sora · 3 in the room"
            />
          </SectionList>
        </FrameSection>
      )}
      {around !== "none" && (
        <FrameSection id="around-events" icon="events" href="/events" heading="Upcoming events">
          <SectionList>
            <SectionLine href="/events/1" title="Fediverse meetup in Tokyo" caption="Sat, Nov 21, 19:00" />
            <SectionLine href="/events/2" title="Moderators' office hours" caption="Tue, Nov 24, 12:00" />
          </SectionList>
        </FrameSection>
      )}
      {reader && at !== "/notifications" && <Notifications none={around === "none"} />}
      {all && (
        <FrameSection id="around-funding" icon="funding" href="/about#funding" heading="Funding">
          <Meter value={0.72} label="Share of this month's costs covered" caption="of November's costs covered" />
        </FrameSection>
      )}
      {footer}
    </FrameAside>
  );
}

const posts = [
  {
    name: "Mika",
    handle: "@mika@my.example",
    avatar: images.dusk,
    text: "The sunset over the bay tonight was something else. Took the long way home just to keep watching it.",
  },
  {
    name: "Ren",
    handle: "@ren@my.example",
    avatar: images.snow,
    text: "v0.2 is out: quotes, polls and the new composer. Thanks to everyone who tested it.",
  },
  {
    name: "Sora",
    handle: "@sora@my.example",
    avatar: images.logo,
    text: "Reminder that the voice room for the meetup opens at 19:00. Bring questions!",
  },
];

function Feed() {
  return (
    <div className="grid gap-feed">
      {posts.map((p, i) => (
        <Entry key={p.handle} labelledBy={`post-${String(i)}`}>
          <Byline
            avatar={<Avatar name={p.name} src={p.avatar} size="post" decorative />}
            name={p.name}
            nameId={`post-${String(i)}`}
            handle={p.handle}
            end={
              <Stamp glyph={{ icon: "public", label: "Public" }} date={new Date(Date.now() - (i + 1) * 3_600_000)} />
            }
          />
          <p className="m-0">{p.text}</p>
        </Entry>
      ))}
    </div>
  );
}

function Banners({ closure, connection }: ShellState) {
  if (!closure && connection === undefined) return null;
  return (
    <FrameBanners label="Server and connection status">
      {closure && (
        <FrameBanner
          icon="caution"
          strong
          links={[
            { href: "/settings/export", label: "Export your data" },
            { href: "/settings/move", label: "Move your account" },
          ]}
        >
          This server closes on March 31, 2027. Export your data by April 30, 2027.
        </FrameBanner>
      )}
      {connection === "offline" && (
        <FrameBanner
          icon="offline"
          links={[{ href: "https://status.my.example", label: "Status page", newTab: "(opens in a new tab)" }]}
        >
          You are offline. What you see may be out of date until you reconnect.
        </FrameBanner>
      )}
      {connection === "outage" && (
        <FrameBanner
          icon="error"
          strong
          links={[{ href: "https://status.my.example", label: "Status page", newTab: "(opens in a new tab)" }]}
        >
          The server is not answering. What you see may be out of date until it is back.
        </FrameBanner>
      )}
    </FrameBanners>
  );
}

function Shell(state: ShellState) {
  const { who = "reader", at = "/", call, toast, logo, around = "all" } = state;
  const [muted, setMuted] = useState(state.muted ?? false);
  const { tabs, rest, rights } = places(state);
  const reader = who !== "visitor";
  const composes = reader && (at === "/" || at === "/local" || at === "/public" || at === "/@yuki");
  const primary = reader ? (
    <FrameAction icon="compose" label="New post" onPress={() => undefined} />
  ) : (
    <FrameAction icon="signIn" label="Sign in" onPress={() => undefined} />
  );
  const callBar = call ? (
    <CallBar
      href="/voice/1"
      room="Morning coffee chat"
      detail="Hosted by Mika · 12 in the room"
      muted={muted}
      muteLabel="Mute"
      onMute={() => {
        setMuted(!muted);
      }}
      leaveLabel="Leave"
      onLeave={() => undefined}
    />
  ) : undefined;
  const list = (items: Place[]) =>
    items.length === 0 ? null : (
      <NavList>
        {items.map((p) => (
          <Item key={p.href} place={p} at={at} />
        ))}
      </NavList>
    );
  // What is around the reader, whose sections the column beside the main one shows from `wide`: the menu leads to
  // their screens at every width (the funding meter's while the server shows it).
  const server: Place[] = reader
    ? [
        { href: "/announcements", icon: "announcements", label: "Announcements" },
        { href: "/voice", icon: "room", label: "Voice rooms" },
        { href: "/events", icon: "events", label: "Events" },
        ...(around === "all" ? [{ href: "/about#funding", icon: "funding", label: "Funding" } as const] : []),
      ]
    : [
        { href: "/login", icon: "signIn", label: "Sign in" },
        { href: "/events", icon: "events", label: "Events" },
      ];
  const staff = rights.length > 0;
  // The rail and the sidebar hold the reader's and the staff's places themselves; their menu holds the rest.
  const sideMenu = (
    <NavList>
      <NavMenu label="Menu" closeLabel="Close" footer={footer}>
        {list(server)}
      </NavMenu>
    </NavList>
  );
  let toasts: ReactNode;
  if (toast === "stale")
    toasts = (
      <Toast action="Update" onAction={() => undefined} closeLabel="Close" onClose={() => undefined}>
        A newer version of the app is ready.
      </Toast>
    );
  if (toast === "required")
    toasts = (
      <Toast action="Update" onAction={() => undefined}>
        This version of the app no longer works with the server. Update it to continue.
      </Toast>
    );
  return (
    <Frame>
      <FrameSide label="Main">
        <FrameMark
          href="/"
          label="my.example"
          light={wordmark("#111111")}
          dark={wordmark("#f2e9e7")}
          logo={logo ? images.logo : undefined}
        />
        {primary}
        {list([...tabs, ...rest])}
        {list(rights)}
        {sideMenu}
        {callBar}
      </FrameSide>
      <FrameMain>
        <Banners {...state} />
        <FrameHeading>Home</FrameHeading>
        {reader && at === "/" && (
          <FrameFold icon="announcements" href="/announcements">
            {`${String(announcements.filter((a) => a.unread).length)} unread announcements`}
          </FrameFold>
        )}
        <Feed />
      </FrameMain>
      <Around {...state} />
      <FrameDock
        label="Main"
        tabs={
          <NavList>
            {tabs.map((p) => (
              <Item key={p.href} place={p} at={at} />
            ))}
            <NavMenu
              label="Menu"
              closeLabel="Close"
              marked={staff}
              markLabel={`${String(reportsToHandle)} waiting in moderation`}
              footer={footer}
            >
              {list(rest)}
              {list(rights)}
              {list(server)}
            </NavMenu>
          </NavList>
        }
        call={callBar}
        action={
          reader ? (
            composes ? (
              <FrameAction icon="compose" label="New post" onPress={() => undefined} />
            ) : undefined
          ) : (
            <FrameAction icon="signIn" label="Sign in" labelled onPress={() => undefined} />
          )
        }
        toasts={toasts}
      />
    </Frame>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = {
  title: "Shell",
  tags: ["themes"],
  parameters: { layout: "fullscreen" },
  render: (args) => <Shell {...args} />,
} satisfies Meta<ShellState>;
export default meta;

type Story = StoryObj<Meta<ShellState>>;

export const Home: Story = { args: {} };
export const Staff: Story = { args: { who: "staff", at: "/notifications" } };
export const TimelinesOff: Story = { args: { timelines: [], at: "/@yuki" } };
export const SingleUser: Story = { args: { singleUser: true, logo: true } };
export const SignedOut: Story = { args: { who: "visitor", at: "/local", around: "some" } };
export const ClosureAnnounced: Story = { args: { closure: true } };
export const Offline: Story = { args: { closure: true, connection: "offline" } };
export const Outage: Story = { args: { connection: "outage", at: "/notifications" } };
export const InCall: Story = { args: { call: true } };
export const InCallMuted: Story = { args: { call: true, muted: true, who: "staff" } };
export const UpdateReady: Story = { args: { toast: "stale", call: false } };
export const UpdateRequired: Story = { args: { toast: "required", at: "/notifications" } };
export const AroundEmpty: Story = { args: { around: "none" } };
/** Waits until a dialog over the page has risen into place, so that the story tests check it as the reader sees it. */
async function dialogRisen(doc: Document) {
  await waitFor(
    () => {
      const panel = doc.querySelector('[role="dialog"]')?.parentElement;
      const style = panel === null || panel === undefined ? undefined : getComputedStyle(panel);
      if (style?.opacity !== "1" || !["none", "matrix(1, 0, 0, 1, 0, 0)"].includes(style.transform))
        throw new Error("the dialog is still rising");
    },
    { timeout: 5000 },
  );
}

// The menu pressed where it shows: the tab bar's sheet on a phone, the popover beside the rail's or the sidebar's
// entry from `tablet`.
export const MenuOpen: Story = {
  args: { who: "staff" },
  play: async ({ canvasElement }) => {
    const [menu] = within(canvasElement).getAllByRole("button", { name: /^Menu/ });
    if (menu === undefined) throw new Error("no menu shows");
    await userEvent.click(menu);
    await dialogRisen(canvasElement.ownerDocument);
  },
};

const server = {
  name: "my.example",
  description: "A small server for people who like the sea, photography and slow mornings.",
};
const pageWordmark = { light: wordmark("#111111"), dark: wordmark("#f2e9e7") };
const statusFooter = (
  <FormPageFooter
    label="This server"
    links={[
      { href: "https://status.my.example", label: "Status page" },
      { href: "/about", label: "About" },
    ]}
  />
);

/** When the maintenance is expected to end: two hours from now, on the hour. */
const backBy = new Date(Math.ceil((Date.now() + 2 * 3_600_000) / 3_600_000) * 3_600_000);

// In place of every screen while the server is down for maintenance (ADR-0088 §10, §11): what it is, when it is
// expected back right under it, strong, in the reader's time zone, named; the operator's words; and the status page,
// once. The server reads no database meanwhile, so the page shows none of it: no name, no logo, no link to its pages.
export const Maintenance: Story = {
  render: () => (
    <FormPage wordmark={pageWordmark}>
      <FormHead title="Under maintenance">
        <FormText>
          <strong>
            Expected to end by{" "}
            {new Intl.DateTimeFormat("en", {
              weekday: "long",
              hour: "numeric",
              minute: "2-digit",
              timeZoneName: "long",
            }).format(backBy)}
          </strong>
        </FormText>
        <FormText>
          The server is under maintenance. Your posts and other data are kept, and this page reloads when it ends.
        </FormText>
      </FormHead>
      <FormText>Moving the database to a larger disk. Thanks for your patience! — the admins</FormText>
      <FormLinks>
        <FormLink href="https://status.my.example" newTab="(opens in a new tab)">
          Check the status page
        </FormLink>
      </FormLinks>
    </FormPage>
  ),
};

// After the closing date, in place of every screen of a signed-in reader (ADR-0088 §2): the closure, the export
// until its deadline, and signing out.
export const Closed: Story = {
  render: () => (
    <FormPage server={server} wordmark={pageWordmark} footer={statusFooter}>
      <FormHead title="my.example has closed">
        <FormText>You can download your data until April 30, 2027.</FormText>
      </FormHead>
      <FormText>Thank you for six good years. — the admins</FormText>
      <FormActions>
        <Button kind="primary">Request an export of your data</Button>
        <Button>Download your follows as CSV</Button>
      </FormActions>
      <FormLinks>
        <FormLink href="/logout">Sign out</FormLink>
      </FormLinks>
    </FormPage>
  ),
};
