// SPDX-License-Identifier: AGPL-3.0-only

// The frame of the settings (#3198, ADR-0110 §5), in the app's shell, in grays: the menu at /settings, and two of its
// screens, Display and posting (every control saves at once) and Security (a status summary over lists), each whole
// and edge to edge so that it reads at the phone, tablet and PC widths of the toolbar's viewport. The options
// `settings-frame` differ in how the menu and a screen stand to each other at each width:
//
// - `drill`: one column at every width. /settings is the menu; an entry opens its screen under the way back; the
//   column beside the main one stays what is around the reader, as on every screen.
// - `aside`: from `wide`, the menu takes the column beside the main one, at the end, the screen the reader is at
//   marked, and /settings opens the first screen; under `wide`, as `drill`.
// - `split`: from `tablet`, the menu is a column of its own between the navigation and the screen, and /settings
//   opens the first screen; on a phone, as `drill`.
//
// What the design system lacks yet (a choice from a long list, the save state, the status summary, a list's row) is
// drawn here as a stand-in, and the words are samples in English.

import { color } from "@my/design-tokens";
import { Options } from "@my/devtools/options";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLayoutEffect, type ReactNode } from "react";
import { Avatar } from "./avatar";
import { Button } from "./button";
import { Badge } from "./chip";
import { CheckField, TextField } from "./form-page";
import { Icon, type IconName } from "./icons";
import { BackButton } from "./list";
import { SettingsGroup, SettingsItem, SettingsMenu, SettingsScreen, SettingsSection } from "./settings";
import {
  Frame,
  FrameAction,
  FrameActions,
  FrameAside,
  FrameDock,
  FrameFooter,
  FrameHeading,
  FrameMain,
  FrameMark,
  FrameSection,
  FrameSide,
  NavItem,
  NavList,
  NavMenu,
  SectionLine,
  SectionList,
} from "./shell";
import { Switch } from "./switch";

type Variant = "drill" | "aside" | "split";
type At = "menu" | "profile" | "display" | "security";

/** The page in grays while the frame is settled: what is on it, its order and its grouping, before any color. */
function useGrays() {
  useLayoutEffect(() => {
    const root = document.documentElement;
    root.style.filter = "grayscale(1)";
    return () => {
      root.style.filter = "";
    };
  }, []);
}

// ---- The menu's content ---------------------------------------------------------------------------------------------

type Entry = { id: string; icon: IconName; label: string; hint: string; count?: number };

const groups: { heading: string; entries: Entry[] }[] = [
  {
    heading: "General",
    entries: [
      { id: "profile", icon: "profile", label: "Profile", hint: "Name, bio, pictures, badges, how people find you" },
      {
        id: "display",
        icon: "display",
        label: "Display and posting",
        hint: "Language, time zone, new posts, timelines",
      },
      { id: "notifications", icon: "notifications", label: "Notifications", hint: "What reaches you, and where" },
    ],
  },
  {
    heading: "People and filters",
    entries: [
      {
        id: "relationships",
        icon: "members",
        label: "Relationships",
        hint: "Blocked and muted accounts, muted servers, followed hashtags",
      },
      { id: "filters", icon: "filter", label: "Keyword filters", hint: "Words hidden from your timelines" },
    ],
  },
  {
    heading: "Sign-in and access",
    entries: [
      {
        id: "security",
        icon: "security",
        label: "Security",
        hint: "Password, passkeys, two-factor authentication, signed-in browsers",
      },
      { id: "apps", icon: "apps", label: "Apps", hint: "Apps you let into your account, apps you made" },
      { id: "invites", icon: "invite", label: "Invites", hint: "Invite people to this server" },
    ],
  },
  {
    heading: "Account",
    entries: [
      {
        id: "account",
        icon: "accountData",
        label: "Account and data",
        hint: "Export and import, auto-delete, move, moderation history, delete",
        count: 1,
      },
    ],
  },
];

const entryOf = (id: string) => groups.flatMap((g) => g.entries).find((e) => e.id === id);

function Menu({ compact = false, current }: { compact?: boolean; current?: string }) {
  return (
    <SettingsMenu label="Settings" compact={compact}>
      {groups.map((g, i) => (
        <SettingsGroup key={g.heading} heading={g.heading}>
          {g.entries.map((e) => (
            <SettingsItem
              key={e.id}
              icon={e.icon}
              label={e.label}
              hint={e.hint}
              count={e.count}
              countLabel={e.count === undefined ? undefined : `${String(e.count)} unread`}
              href={`/settings/${e.id}`}
              current={e.id === current}
            />
          ))}
          {i === groups.length - 1 && <SettingsItem icon="signOut" label="Sign out" count={undefined} />}
        </SettingsGroup>
      ))}
    </SettingsMenu>
  );
}

// ---- Stand-ins for the parts of a section the design system lacks yet -------------------------------------------------

/** The save state of a control that saves at once, beside it. */
function Saved({ state }: { state: "saving" | "saved" }) {
  return (
    <span className="inline-flex items-center gap-entry-caption text-caption text-muted-text">
      {state === "saved" && <Icon name="chosen" place="caption" />}
      {state === "saved" ? "Saved" : "Saving…"}
    </span>
  );
}

/** A setting chosen from a list: its name and a line about it at the start, the control that holds its value at the end. */
function Choice({ label, about, value, state }: { label: string; about?: string; value: string; state?: "saved" }) {
  return (
    <div className="flex flex-wrap items-center justify-end gap-x-screen-row-gap gap-y-screen-air">
      <div className="grid min-w-0 flex-[1_1_7rem]">
        <span>{label}</span>
        {about !== undefined && <span className="text-caption text-muted-text">{about}</span>}
      </div>
      {state !== undefined && <Saved state={state} />}
      <button
        type="button"
        className="inline-flex h-control-form cursor-pointer items-center gap-entry-caption rounded-control border-0 bg-surface-raised px-control-timeline-inline font-sans text-body text-text"
      >
        {value}
        <span className="inline-flex text-muted-text">
          <Icon name="expand" place="caption" />
        </span>
      </button>
    </div>
  );
}

/** A row of a list in a section: its glyph, a title over a caption, and what the reader may do with it at the end. */
function Line({ icon, title, caption, end }: { icon: IconName; title: string; caption: string; end?: ReactNode }) {
  return (
    <div className="flex items-center gap-screen-row-gap">
      <span className="inline-flex flex-none text-muted-text">
        <Icon name={icon} place="action" />
      </span>
      <div className="grid min-w-0 flex-1">
        <span>{title}</span>
        <span className="text-caption text-muted-text">{caption}</span>
      </div>
      {end !== undefined && <span className="flex-none">{end}</span>}
    </div>
  );
}

/** The status summary: a state, what it means, and the next thing to do, on the raised surface. */
function Summary({ title, about, action }: { title: string; about: string; action: string }) {
  return (
    <div className="grid justify-items-start gap-screen-row-gap rounded-overlay bg-surface-raised p-embed">
      <div className="flex items-start gap-screen-row-gap">
        <span className="mt-[0.125rem] inline-flex flex-none">
          <Icon name="caution" place="action" />
        </span>
        <div className="grid min-w-0">
          <span className="font-strong">{title}</span>
          <span className="text-caption text-muted-text">{about}</span>
        </div>
      </div>
      <Button kind="primary">{action}</Button>
    </div>
  );
}

const rows = (children: ReactNode) => <div className="grid gap-screen-row-gap">{children}</div>;

// ---- The screens' content -------------------------------------------------------------------------------------------

type ScreenProps = { back: ReactNode };

function Display({ back }: ScreenProps) {
  return (
    <SettingsScreen icon="display" title="Display and posting" back={back}>
      <SettingsSection heading="Display">
        {rows(
          <>
            <Choice label="Language" value="English" />
            <Choice label="Time zone" value="Tokyo (UTC+9)" state="saved" />
            <Choice label="Link previews" about="How large a link's card is in a post" value="Large" />
          </>,
        )}
      </SettingsSection>
      <SettingsSection heading="New posts" about="What a new post starts with. You can change each as you write.">
        {rows(
          <>
            <Choice label="Audience" value="Public" />
            <Choice label="Who may quote" value="Anyone" />
            <Choice label="Language" value="Same as display" />
            <Switch
              label="Ask for a description of media"
              description="Before a post goes out with a picture or a video that has none"
              defaultSelected
            />
          </>,
        )}
      </SettingsSection>
      <SettingsSection heading="Timelines">
        {rows(
          <>
            <Choice label="Languages" about="Posts in other languages stay out of the timelines" value="All" />
            <Switch
              label="Show new posts as they arrive"
              description="Off, a button over the timeline counts them until you ask"
            />
          </>,
        )}
      </SettingsSection>
    </SettingsScreen>
  );
}

function Security({ back }: ScreenProps) {
  return (
    <SettingsScreen icon="security" title="Security" back={back}>
      <Summary
        title="Two-factor authentication is off"
        about="Anyone who learns your password can sign in. A code from your phone stops them."
        action="Turn on two-factor authentication"
      />
      <SettingsSection heading="Sign-in">
        {rows(
          <>
            <Line
              icon="security"
              title="Password"
              caption="Changed 3 months ago"
              end={<Button size="timeline">Change</Button>}
            />
            <Line
              icon="time"
              title="Two-factor authentication"
              caption="A code from an app on your phone, after the password"
              end={<Badge>Off</Badge>}
            />
          </>,
        )}
      </SettingsSection>
      <SettingsSection heading="Passkeys" count="2" about="Sign in with your device's lock in place of the password.">
        {rows(
          <>
            <Line
              icon="passkey"
              title="MacBook Touch ID"
              caption="Added Mar 3 · used today"
              end={<Button size="timeline">Remove</Button>}
            />
            <Line
              icon="passkey"
              title="Pixel 9"
              caption="Added Jun 12 · used 5d ago"
              end={<Button size="timeline">Remove</Button>}
            />
            <div>
              <Button>Add a passkey</Button>
            </div>
          </>,
        )}
      </SettingsSection>
      <SettingsSection heading="Linked accounts" about="Sign in through an account you have elsewhere.">
        {rows(
          <>
            <Line icon="github" title="GitHub" caption="yuki-snow" end={<Button size="timeline">Unlink</Button>} />
            <Line icon="discord" title="Discord" caption="Not linked" end={<Button size="timeline">Link</Button>} />
          </>,
        )}
      </SettingsSection>
      <SettingsSection heading="Signed-in browsers" count="3">
        {rows(
          <>
            <Line icon="public" title="Firefox on macOS" caption="This browser · Tokyo" />
            <Line
              icon="public"
              title="Safari on iPhone"
              caption="Tokyo · 2d ago"
              end={<Button size="timeline">Sign out</Button>}
            />
            <Line
              icon="public"
              title="Chrome on Windows"
              caption="Osaka · 3w ago"
              end={<Button size="timeline">Sign out</Button>}
            />
          </>,
        )}
      </SettingsSection>
    </SettingsScreen>
  );
}

/** The first screen, a form saved as a whole: what /settings opens where the menu stands beside a screen. */
function Profile({ back }: ScreenProps) {
  return (
    <SettingsScreen icon="profile" title="Profile" back={back}>
      <SettingsSection heading="About you">
        <div className="grid gap-form-page-form-gap">
          <div className="flex items-center gap-screen-row-gap">
            <Avatar name="Yuki" size="post" decorative />
            <Button size="timeline">Change picture</Button>
            <Button size="timeline">Change header</Button>
          </div>
          <TextField id="profile-name" name="name" label="Name" defaultValue="Yuki 雪" />
          <TextField
            id="profile-bio"
            name="bio"
            label="Bio"
            multiline
            defaultValue="Slow mornings, film cameras, and the sea."
          />
          <CheckField id="profile-locked" name="locked" value="1" label="Approve each follower yourself" />
          <CheckField id="profile-bot" name="bot" value="1" label="This account is automated" />
          <div>
            <Button kind="primary">Save</Button>
          </div>
        </div>
      </SettingsSection>
    </SettingsScreen>
  );
}

// ---- The app's shell around them ------------------------------------------------------------------------------------

// A stand-in for the brand's wordmark, in the text's color of a scheme.
const wordmark = (fill: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 44 24" width="44" height="24"><text x="2" y="19" font-family="system-ui, sans-serif" font-size="21" font-style="italic" font-weight="800" fill="${fill}">my</text></svg>`)}`;

type Place = { href: string; icon: IconName; label: string; count?: number };

const tabs: Place[] = [
  { href: "/", icon: "home", label: "Home" },
  { href: "/local", icon: "local", label: "Local" },
  { href: "/public", icon: "federated", label: "Federated" },
  { href: "/notifications", icon: "notifications", label: "Notifications", count: 3 },
  { href: "/search", icon: "search", label: "Search" },
];
const direct: Place = { href: "/direct", icon: "directMessages", label: "Direct messages", count: 2 };
const rest: Place[] = [
  { href: "/@yuki", icon: "profile", label: "Profile" },
  { href: "/bookmarks", icon: "bookmark", label: "Bookmarks" },
  { href: "/settings", icon: "settings", label: "Settings" },
];
const side = [...tabs, ...rest].flatMap((p) => (p.href === "/notifications" ? [p, direct] : [p]));
const server: Place[] = [
  { href: "/announcements", icon: "announcements", label: "Announcements" },
  { href: "/voice", icon: "room", label: "Voice rooms" },
  { href: "/events", icon: "events", label: "Events" },
];

const places = (items: Place[]) => (
  <NavList>
    {items.map((p) => (
      <NavItem
        key={p.href}
        href={p.href}
        icon={p.icon}
        label={p.label}
        current={p.href === "/settings"}
        count={p.count}
        countLabel={p.count === undefined ? undefined : `${String(p.count)} unread`}
      />
    ))}
  </NavList>
);

const footer = (
  <FrameFooter
    label="This server"
    links={[
      { href: "/about", label: "About", icon: "about" },
      { href: "/terms", label: "Terms", icon: "file" },
      { href: "/privacy", label: "Privacy", icon: "file" },
    ]}
  />
);

/** What is around the reader, as on every screen: the column beside the main one of `drill`. */
function Around() {
  return (
    <FrameAside label="Server activity">
      <FrameSection
        id="around-announcements"
        icon="announcements"
        href="/announcements"
        heading="Announcements"
        all={{ text: "All", name: "All announcements" }}
      >
        <SectionList>
          <SectionLine
            href="/announcements/1"
            title="Scheduled maintenance on Sunday, 03:00–04:00 UTC"
            caption="2h ago"
            unread="Unread"
          />
          <SectionLine
            href="/announcements/2"
            title="Custom emoji requests are open"
            caption="1d ago"
            unread="Unread"
          />
        </SectionList>
      </FrameSection>
      <FrameSection id="around-events" icon="events" href="/events" heading="Upcoming events">
        <SectionList>
          <SectionLine href="/events/1" title="Fediverse meetup in Tokyo" caption="Sat, Nov 21, 19:00" />
          <SectionLine href="/events/2" title="Moderators' office hours" caption="Tue, Nov 24, 12:00" />
        </SectionList>
      </FrameSection>
      <FrameSection
        id="around-notifications"
        icon="notifications"
        href="/notifications"
        heading="Notifications"
        all={{ text: "All", name: "All notifications" }}
      >
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
        </SectionList>
      </FrameSection>
      {footer}
    </FrameAside>
  );
}

const screens = { profile: Profile, display: Display, security: Security } as const;

/** The first screen of the menu, which /settings opens where the menu stands beside a screen. */
const first = "profile";

/**
 * The settings in the shell, as a variant places them. From the width at which the variant's menu stands beside the
 * screen, the way back is gone, and /settings shows the first screen in place of the menu.
 */
function Settings({ variant, at }: { variant: Variant; at: At }) {
  useGrays();
  // What the main column shows under the width the menu stands beside from, and from it.
  const under = { drill: "", aside: "wide:hidden", split: "tablet:hidden" }[variant];
  const from = { drill: "hidden", aside: "hidden wide:block", split: "hidden tablet:block" }[variant];
  const back = (
    <div className={under}>
      <BackButton onPress={() => undefined}>Settings</BackButton>
    </div>
  );
  const Screen = screens[at === "menu" ? first : at];
  const current = at === "menu" ? first : at;
  return (
    <Frame>
      <FrameSide label="Main">
        <FrameMark href="/" label="my.example" light={wordmark(color.text.light)} dark={wordmark(color.text.dark)} />
        <FrameActions>
          <FrameAction icon="compose" label="New post" onPress={() => undefined} />
          <FrameAction icon="article" label="New article" secondary onPress={() => undefined} />
        </FrameActions>
        {places(side)}
        <NavList>
          <NavMenu label="Menu" footer={footer}>
            {places(server)}
          </NavMenu>
        </NavList>
      </FrameSide>
      {variant === "split" && (
        // The menu as a column of its own, before the screen, from `tablet`.
        <div className="hidden flex-none self-start pt-shell-gap tablet:sticky tablet:top-0 tablet:block tablet:w-[13rem] desktop:w-shell-sidebar">
          <Menu compact current={current} />
        </div>
      )}
      <FrameMain>
        <FrameHeading>{at === "menu" ? "Settings" : (entryOf(at)?.label ?? "Settings")}</FrameHeading>
        {at === "menu" ? (
          <>
            <div className={under}>
              <Menu />
            </div>
            {variant !== "drill" && (
              <div className={from}>
                <Screen back={undefined} />
              </div>
            )}
          </>
        ) : (
          <Screen back={back} />
        )}
      </FrameMain>
      {variant === "drill" && <Around />}
      {variant === "aside" && (
        <FrameAside label="Settings">
          <Menu compact current={current} />
          {footer}
        </FrameAside>
      )}
      <FrameDock
        label="Main"
        tabs={
          <NavList>
            {tabs.map((p) => (
              <NavItem
                key={p.href}
                href={p.href}
                icon={p.icon}
                label={p.label}
                count={p.count}
                countLabel={p.count === undefined ? undefined : `${String(p.count)} unread`}
              />
            ))}
            <NavMenu label="Menu" count={direct.count} markLabel="2 unread in direct messages" footer={footer}>
              {places([direct, ...rest])}
              {places(server)}
            </NavMenu>
          </NavList>
        }
      />
    </Frame>
  );
}

const options = (at: At) => (
  <Options
    id="settings-frame"
    variants={{
      drill: <Settings variant="drill" at={at} />,
      aside: <Settings variant="aside" at={at} />,
      split: <Settings variant="split" at={at} />,
    }}
  />
);

const meta = {
  title: "Settings",
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** /settings: the menu, or the first screen where the variant's menu stands beside it. */
export const AtMenu: Story = { render: () => options("menu") };
/** /settings/display: every control saves at once. */
export const AtDisplay: Story = { render: () => options("display") };
/** /settings/security: a status summary over lists. */
export const AtSecurity: Story = { render: () => options("security") };
