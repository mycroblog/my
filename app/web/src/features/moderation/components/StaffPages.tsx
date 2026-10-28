// SPDX-License-Identifier: AGPL-3.0-only
// The staff's screens, the same shape for both areas of ADR-0058 (moderation and server administration): each screen
// shows in the mode's menu only to a viewer whose roles have its permission. A screen with things awaiting the
// moderators (FR-712) shows the count as a badge whose words are in the link's name, so the count is read with the
// screen and shown by its digits, not by color alone.
import type { ComponentType } from "react";
import type { ClientMessageKey } from "../../../lib/i18n";
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
