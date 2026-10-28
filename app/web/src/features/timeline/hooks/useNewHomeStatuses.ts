// SPDX-License-Identifier: AGPL-3.0-only
import { useNotice } from "@my/ui";
import { useQuery, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { newHomeStatusesQueryOptions, showNewHomeStatuses, unseenStatuses } from "../api/newStatuses";
import { timelineQueryOptions } from "../api/timelines";

/** The new home entries that wait behind the "N new posts" button (web-client/README.md Y7). */
export type NewHomeStatuses = {
  /** How many new entries wait; 0 hides the button. */
  count: number;
  /** Whether the waiting entries may not join the loaded ones: showing them loads the first page again. */
  gap: boolean;
  /** Inserts the waiting entries at the top of the timeline. A failure is announced. */
  show: () => void;
};

/**
 * The waiting entries of the home timeline, which the route has loaded. The count is computed from the
 * waiting entries and the loaded timeline on every render (R6), so entries that a refetch of the timeline
 * has loaded meanwhile are not counted again.
 */
export function useNewHomeStatuses(): NewHomeStatuses {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const notice = useNotice();
  const waiting = useQuery(newHomeStatusesQueryOptions()).data;
  const home = useSuspenseInfiniteQuery(timelineQueryOptions(api, { kind: "home" }));
  const count = unseenStatuses(
    waiting,
    home.data.pages.flatMap((page) => page.items),
  ).length;

  const show = () => {
    showNewHomeStatuses(queryClient, api).catch((error: unknown) => {
      notice.inPlace(t("client:timeline.new_posts.failed", { reason: errorMessage(error) }), "assertive");
    });
  };

  const gap = waiting?.gap === true;
  return { count: gap ? Math.max(count, 1) : count, gap, show };
}
