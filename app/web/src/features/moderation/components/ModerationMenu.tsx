// SPDX-License-Identifier: AGPL-3.0-only
// The badges of the moderation screens in the admin mode's menu (ADR-0058). The screens with things awaiting the moderators (open reports,
// registrations awaiting approval, appeals awaiting a decision; FR-712) show the count as a badge.
import { useT } from "../../../lib/i18n";
import type { AdminCount } from "../api/counts";
import { useModerationCounts } from "../hooks/useModerationCounts";
import type { StaffPageBadge } from "./StaffPages";

/** The count that a screen's link shows, by the screen's address, and the words of its badge. */
const pageCounts: Readonly<
  Record<
    string,
    {
      count: AdminCount;
      label:
        | "client:admin.menu.open_reports"
        | "client:admin.menu.pending_registrations"
        | "client:admin.menu.pending_appeals";
    }
  >
> = {
  "/moderation/reports": { count: "open_reports", label: "client:admin.menu.open_reports" },
  "/moderation/registrations": { count: "pending_registrations", label: "client:admin.menu.pending_registrations" },
  "/moderation/appeals": { count: "pending_appeals", label: "client:admin.menu.pending_appeals" },
};

/** The badges of the moderation screens: what awaits the moderators on each. */
export function useModerationBadges(): StaffPageBadge {
  const t = useT();
  const counts = useModerationCounts();
  return (page) => {
    const counted = pageCounts[page.href];
    if (counted === undefined) return null;
    const count = counts[counted.count] ?? 0;
    return { count, label: t(counted.label, { count }) };
  };
}
