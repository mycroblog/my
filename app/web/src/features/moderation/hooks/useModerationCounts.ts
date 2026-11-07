// SPDX-License-Identifier: AGPL-3.0-only
// The navigation's Moderation entry and the counts on it (FR-712, ADR-0058): whether the viewer may use a
// moderation screen, and how many open reports, registrations awaiting approval, and appeals awaiting a decision
// wait for them. The counts load only for a viewer whose roles have a permission that one of them needs; the
// server leaves out the others.
import { useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { shellQueryRetry } from "../../../lib/query";
import { useSingleUser } from "../../../lib/singleUser";
import { type AdminCounts, adminCountsQueryOptions, countPermissions } from "../api/counts";
import { moderationPages } from "../components/moderationPages";
import { useShownStaffPages } from "../components/StaffPages";
import { useViewerPermissions } from "./usePermissions";

/**
 * The counts the viewer may see: none while signed out, while they load, and when they cannot be loaded (the
 * badges then stay away rather than showing a guess). Counts loaded before stay while a refetch fails.
 */
export function useModerationCounts(): AdminCounts {
  const api = useApi();
  const status = useSessionStatus();
  const permissions = useViewerPermissions();
  const counted = Object.values(countPermissions).some((permission) => permissions.includes(permission));
  const counts = useQuery({
    ...adminCountsQueryOptions(api),
    enabled: status === "signed-in" && counted,
    retry: shellQueryRetry,
  });
  const singleUser = useSingleUser();
  if (status !== "signed-in" || !counted) return {};
  // A one-person server reviews no registrations (ADR-0152 §4): their count is no queue there.
  if (singleUser && counts.data !== undefined) {
    const { pending_registrations: _registrations, ...rest } = counts.data;
    return rest;
  }
  return counts.data ?? {};
}

/** The navigation's Moderation entry: whether it shows, and the number of things that await the viewer there. */
type ModerationEntry = { available: boolean; pending: number };

export function useModerationEntry(): ModerationEntry {
  const shown = useShownStaffPages(moderationPages);
  const counts = useModerationCounts();
  return {
    available: shown.length > 0,
    pending: (counts.open_reports ?? 0) + (counts.pending_registrations ?? 0) + (counts.pending_appeals ?? 0),
  };
}
