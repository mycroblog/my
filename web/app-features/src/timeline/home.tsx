// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline (ADR-0025 §2, §4): the column of @my/app-components fed by useHomeTimeline() of @my/app-data, and
// what a route's loader reads before the column shows (ADR-0028 §6). A post's actions are not connected yet: the
// column's buttons ask nothing of the server.

import { Column } from "@my/app-components";
import { homeTimelineQuery, useHomeTimeline, type AppData } from "@my/app-data";
import { useLocale } from "@my/design-system";
import { stringsIn } from "../locale";

/**
 * What the route's loader waits for: the home timeline's first page, from the cache unless a write marked it stale. A
 * failure is left to the column, which shows it with a retry.
 */
export async function loadHomeTimeline(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The home timeline's column. */
export function HomeTimeline() {
  const { t } = stringsIn(useLocale().locale);
  return <Column label={t("timeline.home")} {...useHomeTimeline()} />;
}
