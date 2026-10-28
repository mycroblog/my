// SPDX-License-Identifier: AGPL-3.0-only
// The frame of the admin and moderation mode (ADR-0058): the pages under
// /moderation and /admin are a mode of their own, apart from the app, with a menu of their own and a way back to the
// app, never the app's rail. The menu has a group for each area and lists only the screens the viewer's roles allow;
// the moderation screens that have things awaiting the moderators show the count (FR-712). Its look is @my/ui's
// AdminShell; this file decides what goes in it.
import { AdminNavBack, AdminNavSection, AdminShell, BrandMark, RailItem } from "@my/ui";
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
import { useT } from "../../lib/i18n";
import { useInstanceName } from "../../lib/instanceName";

/** Whether `pathname` is in the admin and moderation mode: the pages of the two areas of ADR-0058. */
export function isStaffMode(pathname: string): boolean {
  return /^\/(?:moderation|admin)(?:\/|$)/.test(pathname);
}

/** Whether `pathname` is `href` or a page below it. */
function isUnder(pathname: string, href: string): boolean {
  return pathname === href || pathname.startsWith(`${href}/`);
}

type AreaProps = {
  heading: string;
  /** The area's own page, its first item. */
  overview: { href: string; label: string };
  pages: readonly StaffPage[];
  pathname: string;
  badgeOf?: StaffPageBadge;
};

/** One group of the menu: the area's page and the screens of the area that the viewer's roles allow; nothing for none. */
function Area({ heading, overview, pages, pathname, badgeOf }: AreaProps) {
  const t = useT();
  const shown = useShownStaffPages(pages);
  if (shown.length === 0) return null;
  return (
    <AdminNavSection heading={heading}>
      <li>
        <RailItem
          form="section"
          href={overview.href}
          icon={null}
          label={overview.label}
          current={pathname.replace(/(.)\/$/, "$1") === overview.href}
        />
      </li>
      {shown.map((page) => {
        const label = t(page.label);
        const count = badgeOf?.(page) ?? null;
        return (
          <li key={page.href}>
            <RailItem
              form="section"
              href={page.href}
              icon={null}
              label={label}
              current={isUnder(pathname, page.href)}
              {...(count !== null &&
                count.count > 0 && {
                  badge: { count: count.count, label: count.label, name: `${label} ${count.label}` },
                })}
            />
          </li>
        );
      })}
    </AdminNavSection>
  );
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
      navigation={
        <>
          <Area
            heading={t("client:admin.moderation.title")}
            overview={{ href: "/moderation", label: t("client:admin.mode.overview") }}
            pages={moderationPages}
            pathname={pathname}
            badgeOf={moderationBadges}
          />
          <Area
            heading={t("client:admin.menu.title")}
            overview={{ href: "/admin", label: t("client:admin.mode.overview") }}
            pages={adminPages}
            pathname={pathname}
          />
        </>
      }
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
