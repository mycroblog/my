// SPDX-License-Identifier: AGPL-3.0-only
import { Stack } from "@my/ui";
import { NewPostsFeed } from "./NewPostsFeed";

export type ListTimelineProps = {
  /** The list's ID. */
  id: string;
  /** The ID of the page heading, the list's name. */
  "aria-labelledby": string;
};

/**
 * The timeline of one of the user's lists (web-client/screens/list-timeline.md, ADR-0122 §4): the home timeline's feed
 * with its "N new posts" button, whose new entries the stream's `list_update` delivers under the same auto-insert
 * setting. The route loads its first page.
 */
export function ListTimeline({ id, "aria-labelledby": labelledBy }: ListTimelineProps) {
  return (
    <Stack>
      <NewPostsFeed timeline={{ kind: "list", id }} aria-labelledby={labelledBy} />
    </Stack>
  );
}
