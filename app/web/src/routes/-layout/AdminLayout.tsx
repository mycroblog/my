// SPDX-License-Identifier: AGPL-3.0-only
// The frame of the admin and moderation mode (ADR-0058): the pages under
// /moderation and /admin are a mode of their own, apart from the app, with a menu of their own and a way back to the
// app, never the app's rail. The menu (@my/ui's NavMenu) has the groups of each area, moderation first: the area's own
// group, named after it, with its overview, then the groups its screens name (moderationPages, adminPages). It lists
// only the screens the viewer's roles allow; the moderation screens that have things awaiting the moderators show the
// count (FR-712). Its look is @my/ui's AdminShell; this file decides what goes in it.
import {
  AdminNavBack,
  AdminShell,
  BrandMark,
  NavMenu,
  type NavMenuGroup,
  type NavMenuItem,
  OverviewIcon,
} from "@my/ui";
import { useLocation, useRouter } from "@tanstack/react-router";
import { type ReactNode, type Ref, useEffect, useState } from "react";
import { adminPages } from "../../features/admin";
import {
  moderationPages,
  type StaffPage,
  type StaffPageBadge,
  useModerationBadges,
  useShownStaffPages,
} from "../../features/moderation";
import { type ClientMessageKey, useT } from "../../lib/i18n";
import { useInstanceName } from "../../lib/instanceName";

/** Whether `pathname` is in the admin and moderation mode: the pages of the two areas of ADR-0058. */
export function isStaffMode(pathname: string): boolean {
  return /^\/(?:moderation|admin)(?:\/|$)/.test(pathname);
}

/** Whether `pathname` is `href` or a page below it. */
function isUnder(pathname: string, href: string): boolean {
  return pathname === href || pathname.startsWith(`${href}/`);
}

type Area = {
  /** The area's key and the heading of its own group. */
  id: string;
  heading: string;
  /** The area's own page, the first item of its own group. */
  overview: { href: string; label: string };
  pages: readonly StaffPage[];
  badgeOf?: StaffPageBadge;
};

/**
 * The groups of the menu for one area: the area's own group (its overview and its pages without a group), then a
 * group for each `group` of its pages, in the order the pages first name it; only the screens that the viewer's roles
 * allow, and nothing for an area without any.
 */
function useAreaGroups(area: Area, pathname: string): NavMenuGroup[] {
  const t = useT();
  const shown = useShownStaffPages(area.pages);
  if (shown.length === 0) return [];
  const item = (page: StaffPage): NavMenuItem => {
    const label = t(page.label);
    const count = area.badgeOf?.(page) ?? null;
    const Icon = page.icon;
    return {
      href: page.href,
      icon: <Icon />,
      label,
      current: isUnder(pathname, page.href),
      ...(count !== null &&
        count.count > 0 && { badge: { count: count.count, label: count.label, name: `${label} ${count.label}` } }),
    };
  };
  const own: NavMenuGroup = {
    id: area.id,
    heading: area.heading,
    items: [
      {
        href: area.overview.href,
        icon: <OverviewIcon />,
        label: area.overview.label,
        current: pathname.replace(/(.)\/$/, "$1") === area.overview.href,
      },
      ...shown.filter((page) => page.group === undefined).map(item),
    ],
  };
  const named = new Map<ClientMessageKey, NavMenuItem[]>();
  for (const page of shown) {
    if (page.group === undefined) continue;
    named.set(page.group, [...(named.get(page.group) ?? []), item(page)]);
  }
  return [
    own,
    ...Array.from(named, ([heading, items]) => ({ id: `${area.id}:${heading}`, heading: t(heading), items })),
  ];
}

export type AdminLayoutProps = {
  children: ReactNode;
  skipLinkLabel: string;
  mainId: string;
  mainRef: Ref<HTMLElement>;
};

export function AdminLayout({ children, skipLinkLabel, mainId, mainRef }: AdminLayoutProps) {
  const t = useT();
  const router = useRouter();
  const instanceName = useInstanceName();
  const pathname = useLocation({ select: (location) => location.pathname });
  const moderationBadges = useModerationBadges();
  const moderationGroups = useAreaGroups(
    {
      id: "moderation",
      heading: t("client:admin.moderation.title"),
      overview: { href: "/moderation", label: t("client:admin.mode.overview") },
      pages: moderationPages,
      badgeOf: moderationBadges,
    },
    pathname,
  );
  const adminGroups = useAreaGroups(
    {
      id: "admin",
      heading: t("client:admin.menu.title"),
      overview: { href: "/admin", label: t("client:admin.mode.overview") },
      pages: adminPages,
    },
    pathname,
  );
  const [menuOpen, setMenuOpen] = useState(false);
  // The router is outside React: subscribe to its navigations, and unsubscribe on unmount.
  useEffect(
    () =>
      router.subscribe("onBeforeNavigate", (event) => {
        if (event.pathChanged) setMenuOpen(false);
      }),
    [router],
  );
  return (
    <AdminShell
      brand={<BrandMark wordmarkName="my" instanceName={instanceName} size="sm" />}
      modeLabel={t("client:admin.mode.title")}
      navigation={<NavMenu groups={[...moderationGroups, ...adminGroups]} />}
      footer={<AdminNavBack href="/home" label={t("client:admin.mode.back")} />}
      menuButtonLabel={t("client:admin.mode.menu")}
      closeLabel={t("client:shell.menu.close")}
      menuOpen={menuOpen}
      onMenuOpenChange={setMenuOpen}
      skipLinkLabel={skipLinkLabel}
      mainId={mainId}
      mainRef={mainRef}
    >
      {children}
    </AdminShell>
  );
}
