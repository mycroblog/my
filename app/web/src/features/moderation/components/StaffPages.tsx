// SPDX-License-Identifier: AGPL-3.0-only
// The lists of links to the staff's screens, the same for both areas of ADR-0058 (moderation and server
// administration): each screen shows only to a viewer whose roles have its permission. A list is the page of its
// area that the navigation's entry opens. A screen with things awaiting the moderators (FR-712) shows the count as a badge
// whose words are in the link's name, so the count is read with the screen and shown by its digits, not by
// color alone.
import { Badge, EmptyState, Hint, Link, Stack } from "@my/ui";
import { type ComponentType, useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { Permission } from "../api/staff";
import { useViewerPermissions } from "../hooks/usePermissions";

/** A staff screen: its address, the permission it needs, the words and the icon of its link, and its group in the menu. */
export type StaffPage = {
  href: string;
  permission: Permission;
  label: ClientMessageKey;
  description: ClientMessageKey;
  /** The icon of its link in the mode's menu (one of the kit's icons). */
  icon: ComponentType;
  /** The heading of its group in the mode's menu; none for the area's own group, which starts with the overview. */
  group?: ClientMessageKey;
};

/** The badge of a screen's link: the count and its words, or none. */
export type StaffPageBadge = (page: StaffPage) => { count: number; label: string } | null;

/** The screens of `pages` that the viewer's roles allow, in their order. */
export function useShownStaffPages(pages: readonly StaffPage[]): readonly StaffPage[] {
  const permissions = useViewerPermissions();
  return pages.filter((page) => permissions.includes(page.permission));
}

const noBadge: StaffPageBadge = () => null;

/** The list of the links to the screens in `shown`, each with its description and its count. */
function StaffPageList({
  shown,
  idPrefix,
  badgeOf,
}: {
  shown: readonly StaffPage[];
  idPrefix: string;
  badgeOf: StaffPageBadge;
}) {
  const t = useT();
  return (
    <Stack as="ul" divided border="lg">
      {shown.map((page) => {
        const descriptionId = `${idPrefix}-${page.href}`;
        const badge = badgeOf(page);
        return (
          <Stack as="li" key={page.href} paddingX="4" paddingY="3">
            <Link href={page.href} variant="standalone" aria-describedby={descriptionId} gap={2}>
              {t(page.label)}
              {badge !== null && badge.count > 0 && (
                <>
                  {/* The space keeps the words apart in the link's name. */}{" "}
                  <Badge count={badge.count} label={badge.label} />
                </>
              )}
            </Link>
            <Hint as="span" id={descriptionId}>
              {t(page.description)}
            </Hint>
          </Stack>
        );
      })}
    </Stack>
  );
}

type StaffPagesProps = {
  pages: readonly StaffPage[];
  badgeOf?: StaffPageBadge;
  none: { heading: ClientMessageKey; description: ClientMessageKey };
};

/** The page of one area: the links to its screens, or why there are none for the viewer's roles. */
export function StaffPages({ pages, badgeOf = noBadge, none }: StaffPagesProps) {
  const t = useT();
  const id = useId();
  const shown = useShownStaffPages(pages);
  if (shown.length === 0) return <EmptyState heading={t(none.heading)} description={t(none.description)} />;
  return <StaffPageList shown={shown} idPrefix={id} badgeOf={badgeOf} />;
}
