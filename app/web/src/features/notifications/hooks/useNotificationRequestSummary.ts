// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { type NotificationRequestSummary, notificationRequestSummaryQueryOptions } from "../api/requests";

/**
 * The counts of the notification requests (ADR-0158 §7), which the route's loader has tried and the stream keeps up to
 * date; null while none waits, while they load, and when they failed (the row is left out rather than shown wrong).
 */
export function useNotificationRequestSummary(): NotificationRequestSummary | null {
  const api = useApi();
  const signedIn = useSessionStatus() === "signed-in";
  const summary = useQuery({ ...notificationRequestSummaryQueryOptions(api), ...shellQueryReader, enabled: signedIn });
  if (!signedIn || summary.data === undefined || summary.data.accounts_count === 0) return null;
  return summary.data;
}
