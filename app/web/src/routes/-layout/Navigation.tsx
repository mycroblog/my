// SPDX-License-Identifier: AGPL-3.0-only
// The main navigation, one landmark for every width (web-client/README.md D1, D2, ADR-0039): a tab bar on the bottom
// edge of phones, a side rail of icons with short labels on tablets (md), and a sidebar with full labels on
// desktops (lg). The link to the current section has aria-current="page".
//
// The user chooses the items and their order in the preferences (features/settings, kept on this browser); by
// default they are Home, Local, Federated, Notifications, Search, Profile, Bookmarks and Settings. The local and
// the federated timeline are items of their own at every width, never tabs of one screen. From md the rail holds
// every shown item. Below md the tab bar holds the first five shown items (tabBarLimit) and Menu, a button that
// opens a sheet with the other shown items, the color theme and the server's documents. Both are one list in one
// order, and each item hides at the widths where it does not belong (display: none, so it leaves the
// accessibility tree too).
//
// The notifications item shows the number of unread notifications as a badge on the icon's corner. The
// badge's digits are hidden from assistive technology; the link is named with the count instead
// ("Notifications, 3 unread"), a name that starts with the visible label (WCAG 2.5.3).
//
// The staff's screens are two areas (ADR-0058). A viewer whose roles have a permission of a moderation screen
// also has Moderation, and one with a permission of a server administration screen has Administration, in that
// order after the user's items in the rail and the sidebar and in the menu's sheet, set apart from them by a line
// (the first of them is `separated`); a moderator never sees
// Administration. Only the permissions of those screens count: a permission of the user's side (ADR-0070, the
// member category, such as view_finance) shows neither. A viewer with view_finance has Finances, after the user's
// items and before the staff's areas, in the rail, the sidebar and the menu's sheet, never in the tab bar.
// While the server has voice chat on (ADR-0100 §2), Voice chat, the list of the open rooms, comes right after the
// user's items the same way.
// While open reports, registrations or appeals await the moderators (FR-712), Moderation shows
// their number as a badge the same way ("Moderation, 3 pending"), and so does the tab bar's Menu button, which
// leads there ("Menu, 3 pending in moderation"): the count is in the names and drawn as digits, never shown by
// color alone (WCAG 1.4.1). Administration has no count.
//
// The local and federated timelines appear only while the instance serves them (FR-202). An item the server does
// not offer keeps its place among the first five, so that the tab bar does not change while GET /instance loads:
// it has up to five items and Menu.
//
// The sidebar (lg) also says where and who the user is: the server's name above the list, as a link to the root
// address (the landing page, or the home timeline when signed in), and, below it, the signed-in account (avatar,
// display name and handle) as a link to its profile. Both are outside the list, so the list's links stay the same
// and in the same order at every width, and narrower layouts hide both.
//
// The items above need a session, so people who are not signed in get other ones (web-client/09-app-shell.md §9, "signed-out
// navigation"): Local, Federated, Search, About and Sign in, the tab bar's five items, and Sign up, in the rail and
// in the menu's sheet, while the server takes new accounts. Sign in opens the sign-in page, which comes back to the
// page it was opened from. While the session resumes, the list has no items yet, so that neither set shows to the
// wrong person; a session that cannot be resumed because the server is not reached (offline) keeps the user's items.
import {
  AboutIcon,
  AdminIcon,
  AppNav,
  AppNavAccount,
  AppNavAction,
  AppNavBrand,
  type AppNavBadge,
  AppNavItem,
  AppNavMenu,
  AppNavMenuLink,
  AppNavMenuList,
  AppNavMenuSection,
  Avatar,
  FinanceIcon,
  Link,
  ModerationIcon,
  PlusIcon,
  SignInIcon,
  SignUpIcon,
  Text,
  VoiceIcon,
} from "@my/ui";
import { useLocation, useRouter } from "@tanstack/react-router";
import { type ReactNode, useEffect, useState } from "react";
import { useOpenCompose } from "../../features/compose";
import { useAdminEntry } from "../../features/admin";
import { useFinanceEntry } from "../../features/costs";
import { DocumentLinks } from "../../features/documents";
import { useSignUpHref } from "../../features/landing";
import { useModerationEntry } from "../../features/moderation";
import { useUnreadNotificationCount } from "../../features/notifications";
import { useViewer } from "../../features/session";
import {
  type NavigationEntryId,
  navigationEntryIcons,
  navigationEntryLabels,
  tabBarLimit,
  ThemeRadioGroup,
  useNavigationEntries,
} from "../../features/settings";
import { type TimelineAvailability, useTimelineAvailability } from "../../features/timeline";
import { useVoiceAvailable } from "../../features/voice";
import { useSessionStatus, useSessionUnreachable } from "../../lib/auth/SessionProvider";
import { handleOf } from "../../lib/handle";
import { type ClientMessageKey, useT } from "../../lib/i18n";
import { aboutPath, followListPath, profilePath } from "../../lib/paths";
import { imageSources } from "../../lib/images";
import { ServerNameLink } from "./ServerNameLink";

type Section =
  | "/home"
  | "/local"
  | "/public"
  | "/search"
  | "/notifications"
  | "/profile"
  | "/bookmarks"
  | "/settings"
  | "/voice"
  | "/finance"
  | "/moderation"
  | "/admin"
  | "/about"
  | "/login";

type NavigationItem = {
  /** The item's key in the list. */
  id: string;
  /** Where the link goes. */
  href: string;
  /** The app's section the link is the place of, for its aria-current; none for a page outside the app (Sign up). */
  section?: Section;
  label: ClientMessageKey;
  icon: ReactNode;
  /** Where the item shows: everywhere, only in the phones' tab bar, or only in the rail and sidebar from md (and the menu's sheet below md). */
  shows: "always" | "tab-bar" | "rail";
  /** The timeline the item opens, offered only while the instance serves it. */
  timeline?: keyof TimelineAvailability;
  /** The staff's area (ADR-0058) whose screens the item leads to, for a viewer who may use one of them only. */
  staff?: "moderation" | "admin";
};

/** The page and the timeline of each item that the user arranges. */
const entryTargets: Record<NavigationEntryId, { section: Section; timeline?: keyof TimelineAvailability }> = {
  home: { section: "/home" },
  local: { section: "/local", timeline: "local" },
  public: { section: "/public", timeline: "federated" },
  notifications: { section: "/notifications" },
  search: { section: "/search" },
  profile: { section: "/profile" },
  bookmarks: { section: "/bookmarks" },
  settings: { section: "/settings" },
};

/**
 * The page of the user's side that the member permission view_finance opens (ADR-0070): after the user's items and
 * before the staff's areas in the rail and in the menu's sheet, never in the tab bar.
 */
const financeItem: NavigationItem = {
  id: "finance",
  href: "/finance",
  section: "/finance",
  label: "client:shell.nav.finance",
  icon: <FinanceIcon />,
  shows: "rail",
};

/**
 * The list of the open voice chat rooms (ADR-0100 §8), while the server has voice chat on: after the user's items in
 * the rail and in the menu's sheet, never in the tab bar.
 */
const voiceItem: NavigationItem = {
  id: "voice",
  href: "/voice",
  section: "/voice",
  label: "client:voice.title",
  icon: <VoiceIcon />,
  shows: "rail",
};

/** The staff's areas, after the user's items in the rail and in the menu's sheet, never in the tab bar. */
const staffItems: readonly NavigationItem[] = [
  {
    id: "moderation",
    href: "/moderation",
    section: "/moderation",
    label: "client:shell.nav.moderation",
    icon: <ModerationIcon />,
    shows: "rail",
    staff: "moderation",
  },
  {
    id: "admin",
    href: "/admin",
    section: "/admin",
    label: "client:shell.nav.admin",
    icon: <AdminIcon />,
    shows: "rail",
    staff: "admin",
  },
];

/**
 * The items of people who are not signed in: the pages they can use without a session, the tab bar's five, and the
 * server's registration page (a page of the server, loaded in place of the app) while it takes new accounts.
 */
function signedOutItems(signInHref: string, signUpHref: string | undefined): NavigationItem[] {
  return [
    {
      id: "local",
      href: "/local",
      section: "/local",
      label: navigationEntryLabels.local,
      icon: navigationEntryIcons.local,
      shows: "always",
      timeline: "local",
    },
    {
      id: "public",
      href: "/public",
      section: "/public",
      label: navigationEntryLabels.public,
      icon: navigationEntryIcons.public,
      shows: "always",
      timeline: "federated",
    },
    {
      id: "search",
      href: "/search",
      section: "/search",
      label: navigationEntryLabels.search,
      icon: navigationEntryIcons.search,
      shows: "always",
    },
    {
      id: "about",
      href: aboutPath,
      section: "/about",
      label: "client:shell.nav.about",
      icon: <AboutIcon />,
      shows: "always",
    },
    {
      id: "sign-in",
      href: signInHref,
      section: "/login",
      label: "client:shell.nav.sign_in",
      icon: <SignInIcon />,
      shows: "always",
    },
    ...(signUpHref === undefined
      ? []
      : [
          {
            id: "sign-up",
            href: signUpHref,
            label: "client:shell.nav.sign_up",
            icon: <SignUpIcon />,
            shows: "rail",
          } satisfies NavigationItem,
        ]),
  ];
}

/** The sign-in page, which returns to `location` afterwards; on the sign-in page itself, that page as it is. */
function signInHrefOf(location: { pathname: string; href: string }): string {
  if (location.pathname === "/login") return location.href;
  return `/login?${new URLSearchParams({ return_to: location.href }).toString()}`;
}

/** Whether `pathname` is `path` or a page below it. */
function isUnder(pathname: string, path: string): boolean {
  return pathname === path || pathname.startsWith(`${path}/`);
}

/**
 * Whether `pathname` is in `section`: the page itself or one below it. "/profile" redirects to the viewer's own
 * profile page, so that page and its lists of followers and follows are its section too. The viewer's own posts
 * (`/@acct/status/{id}`) are below the profile's address but are pages of posts, not of the profile.
 */
function inSection(pathname: string, section: Section, ownAcct: string | null): boolean {
  if (isUnder(pathname, section)) return true;
  if (section !== "/profile" || ownAcct === null) return false;
  return [profilePath(ownAcct), followListPath(ownAcct, "followers"), followListPath(ownAcct, "following")].includes(
    pathname,
  );
}

/** The badge of an item and the name of its link while it shows one, or undefined for an item without a count now. */
type ItemBadge = AppNavBadge | undefined;

/** Whether nobody is signed in (offline, a session that could not be resumed counts as still there): the pages then have a top bar. */
export function useSignedOut(): boolean {
  const status = useSessionStatus();
  const unreachable = useSessionUnreachable();
  return status === "signed-out" && !unreachable;
}

export function Navigation() {
  const t = useT();
  const pathname = useLocation({ select: (location) => location.pathname });
  const signInHref = useLocation({ select: signInHrefOf });
  const signUpHref = useSignUpHref();
  const status = useSessionStatus();
  const unread = useUnreadNotificationCount();
  const moderationEntry = useModerationEntry();
  const adminEntry = useAdminEntry();
  const financeEntry = useFinanceEntry();
  const voiceAvailable = useVoiceAvailable();
  const staffAvailable = (item: NavigationItem) =>
    item.staff === undefined || (item.staff === "moderation" ? moderationEntry.available : adminEntry.available);
  const viewer = useViewer();
  const ownAcct = viewer.status === "signed-in" ? viewer.account.acct : null;
  const timelines = useTimelineAvailability();
  const openCompose = useOpenCompose();
  // The user's shown items in their order, the first ones in the tab bar, then the staff's areas.
  const userItems = useNavigationEntries()
    .filter((entry) => entry.shown)
    .map((entry, index): NavigationItem => ({
      ...entryTargets[entry.id],
      id: entry.id,
      href: entryTargets[entry.id].section,
      label: navigationEntryLabels[entry.id],
      icon: navigationEntryIcons[entry.id],
      shows: index < tabBarLimit ? "always" : "rail",
    }));
  // Offline, a session that could not be resumed is most likely still there: the user's items stay (web-client/11-pwa.md §11).
  const signedOut = useSignedOut();
  let items: readonly NavigationItem[] = [];
  if (signedOut) items = signedOutItems(signInHref, signUpHref);
  else if (status !== "unknown")
    items = [
      ...userItems,
      ...(voiceAvailable ? [voiceItem] : []),
      ...(financeEntry.available ? [financeItem] : []),
      ...staffItems,
    ];
  const shown = items.filter(
    (item) => (item.timeline === undefined || timelines[item.timeline]) && staffAvailable(item),
  );
  const badgeOf = (section: Section | undefined): ItemBadge => {
    if (section === "/notifications" && unread > 0) {
      return {
        count: unread,
        label: t("client:notifications.unread_count", { count: unread }),
        name: t("client:shell.nav.notifications_unread", { count: unread }),
      };
    }
    if (section === "/moderation" && moderationEntry.available && moderationEntry.pending > 0) {
      const count = moderationEntry.pending;
      return {
        count,
        label: t("client:admin.pending_count", { count }),
        name: t("client:shell.nav.moderation_pending", { count }),
      };
    }
    return undefined;
  };
  const current = (section: Section | undefined) => section !== undefined && inSection(pathname, section, ownAcct);
  // A line sets the staff's areas apart from the user's items, in the rail and in the menu's sheet.
  const firstStaff = shown.find((item) => item.staff !== undefined)?.id;
  return (
    <AppNav
      label={t("client:shell.nav.label")}
      // The wordmark and the server's name, a link to the root address (web-client/16-look-states-and-wording.md G9).
      header={
        <AppNavBrand>
          <ServerNameLink size="lg" />
        </AppNavBrand>
      }
      // Signed out, the navigation is a light bar along the top (AppLayout puts it above the page).
      placement={signedOut ? "top" : "side"}
      action={
        openCompose === null ? undefined : (
          <AppNavAction label={t("client:compose.open")} icon={<PlusIcon />} onPress={openCompose} />
        )
      }
      footer={<ViewerLink />}
    >
      {shown.map((item) => {
        const badge = badgeOf(item.section);
        return (
          <AppNavItem
            key={item.id}
            href={item.href}
            icon={item.icon}
            label={t(item.label)}
            current={current(item.section)}
            shows={item.shows}
            separated={item.id === firstStaff}
            {...(item.id === "sign-up" && { prominent: true })}
            {...(badge !== undefined && { badge })}
          />
        );
      })}
      {status !== "unknown" && (
        <NavigationMenu
          items={shown.filter((item) => item.shows === "rail")}
          current={current}
          badgeOf={badgeOf}
          firstStaff={firstStaff}
        />
      )}
    </AppNav>
  );
}

/**
 * The sidebar's last item: the signed-in account, a link to its profile named by its display name and handle,
 * each cut to one line (web-client/16-look-states-and-wording.md G7). The list's Profile item is the place, so this second link to it has
 * no aria-current. It shows nothing until the account has loaded, and while signed out.
 */
function ViewerLink() {
  const t = useT();
  const viewer = useViewer();
  if (viewer.status !== "signed-in") return null;
  const { account } = viewer;
  const name = account.display_name || account.username;
  return (
    <AppNavAccount
      href={profilePath(account.acct)}
      avatar={<Avatar sources={imageSources(account.avatar)} name={name} seed={account.id} size="sm" decorative />}
      name={name}
      handle={handleOf(account.acct)}
      description={t("client:shell.nav.viewer")}
    />
  );
}

type NavigationMenuProps = {
  /** The items of the sheet that the viewer may use. */
  items: readonly NavigationItem[];
  /** Whether a section's link is the current page. */
  current: (section: Section | undefined) => boolean;
  /** The badge of a section's link, the same as in the rail. */
  badgeOf: (section: Section | undefined) => ItemBadge;
  /** The id of the first item of the staff's areas, which a line sets apart; none without one. */
  firstStaff: string | undefined;
};

/**
 * The tab bar's Menu: a button that opens a sheet (a modal dialog, WAI-ARIA APG) with the items of the rail that
 * are not in the tab bar. The button is not a place, so it never has aria-current; the sheet marks the current
 * item. A navigation to another page closes the sheet when it starts, so that the new page's heading can take
 * focus (useViewChange); a navigation that the unsaved-changes guard holds does not start, and the sheet stays.
 * The button carries the counts of the sheet's items (what awaits the moderators), so that they show in the tab
 * bar too, and the sheet's items show them as the rail does. Under the items, the sheet switches the color theme
 * (the same control as the preferences page, in a row). Under that it names the server, as a link to the root
 * address (phones have no sidebar that names it), and links to the server's information page and to its terms of
 * service and privacy policy, which the third column shows on wide screens (the documents when the server
 * publishes them).
 */
function NavigationMenu({ items, current, badgeOf, firstStaff }: NavigationMenuProps) {
  const t = useT();
  const router = useRouter();
  const [isOpen, setIsOpen] = useState(false);
  const pending = items.reduce((sum, item) => sum + (badgeOf(item.section)?.count ?? 0), 0);
  // The router is outside React: subscribe to its navigations, and unsubscribe on unmount.
  useEffect(
    () =>
      router.subscribe("onBeforeNavigate", (event) => {
        if (event.pathChanged) setIsOpen(false);
      }),
    [router],
  );
  return (
    <AppNavMenu
      label={t("client:shell.nav.menu")}
      {...(pending > 0 && {
        badge: {
          count: pending,
          label: t("client:admin.pending_count", { count: pending }),
          name: t("client:shell.nav.menu_pending", { count: pending }),
        },
      })}
      isOpen={isOpen}
      onOpenChange={setIsOpen}
      title={t("client:shell.menu.title")}
      closeLabel={t("client:shell.menu.close")}
    >
      {items.length > 0 && (
        <AppNavMenuList>
          {items.map((item) => {
            const badge = badgeOf(item.section);
            return (
              <AppNavMenuLink
                key={item.id}
                href={item.href}
                icon={item.icon}
                label={t(item.label)}
                current={current(item.section)}
                separated={item.id === firstStaff}
                {...(badge !== undefined && { badge })}
              />
            );
          })}
        </AppNavMenuList>
      )}
      {/* The theme is a setting of this browser, switched here too so that phones reach it in one step. */}
      <AppNavMenuSection separator="4">
        <ThemeRadioGroup layout="sheet" />
      </AppNavMenuSection>
      <AppNavMenuSection separator="3">
        <ServerNameLink />
      </AppNavMenuSection>
      <AppNavMenuSection>
        <Text as="p" size="sm">
          <Link href={aboutPath} variant="standalone">
            {t("client:shell.about.more")}
          </Link>
        </Text>
      </AppNavMenuSection>
      <DocumentLinks placement="sheet" />
    </AppNavMenu>
  );
}
