// SPDX-License-Identifier: AGPL-3.0-only
import { Box, NewItemsButton } from "@my/ui";
import { useRef } from "react";
import { useT } from "../../../lib/i18n";
import type { StreamedTimeline } from "../api/newStatuses";
import { useNewPosts } from "../hooks/useNewPosts";
import { TimelineFeed } from "./TimelineFeed";

export type NewPostsFeedProps = {
  /** The streamed timeline: the home timeline, one of the user's lists, the local or the federated timeline. */
  timeline: StreamedTimeline;
  /** The ID of the page heading that names the timeline; pressing the button moves focus there. */
  "aria-labelledby": string;
};

/**
 * The feed of a streamed timeline with the "N new posts" button over its top (web-client/README.md Y7, ADR-0077): the
 * home timeline's, every list's (ADR-0122 §4), and the local and the federated timelines' (ADR-0127 §5), which follow the
 * same setting. By default the stream's new entries
 * wait until the reader presses it; with the reader's auto-insert setting they are inserted at once, the post being
 * read stays in place, and the button counts what came in since the reader left the top. Its region has no height, so
 * neither its appearance nor its count moves the posts being read; the button floats over the feed's top while the
 * reader scrolls and, at the top, space is kept under it only while it shows. Only the button takes pointer input, so
 * the empty region never blocks the post under it. Pressing it moves focus to the page heading, above the inserted
 * posts, since the button goes away (WCAG 2.4.3). Render it inside a `Stack`, whose gap it shares.
 */
export function NewPostsFeed({ timeline, "aria-labelledby": labelledBy }: NewPostsFeedProps) {
  const t = useT();
  const feedRef = useRef<HTMLDivElement>(null);
  const { count, buttonAtTop, press, feed } = useNewPosts(timeline, feedRef, labelledBy);
  return (
    <>
      <NewItemsButton count={count} label={t("client:timeline.new_posts", { count })} onPress={press} sticky />
      <Box {...(buttonAtTop && { paddingTop: "2" as const })}>
        <TimelineFeed timeline={timeline} aria-labelledby={labelledBy} feedRef={feedRef} {...feed} />
      </Box>
    </>
  );
}
