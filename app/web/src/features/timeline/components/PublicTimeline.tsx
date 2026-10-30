// SPDX-License-Identifier: AGPL-3.0-only
import { Stack } from "@my/ui";
import { useSessionStatus } from "../../../lib/auth/SessionProvider";
import type { PublicStreamedTimeline } from "../api/newStatuses";
import { usePublicTimelineStream } from "../hooks/usePublicTimelineStream";
import { NewPostsFeed } from "./NewPostsFeed";
import { TimelineFeed } from "./TimelineFeed";

export type PublicTimelineProps = {
  /** The local or the federated timeline. */
  timeline: PublicStreamedTimeline;
  /** The ID of the page heading that names the timeline. */
  "aria-labelledby": string;
};

/**
 * The local or the federated timeline (web-client/screens/local.md, public.md). For a signed-in user it is the home
 * timeline's feed with its "N new posts" button, whose new entries the timeline's own stream delivers while it is shown,
 * under the same auto-insert setting (ADR-0127 §5). A visitor gets no stream (ADR-0127 §2): the posts of the moment the
 * page opened, without the button. The route loads its first page.
 */
export function PublicTimeline({ timeline, "aria-labelledby": labelledBy }: PublicTimelineProps) {
  if (useSessionStatus() !== "signed-in") return <TimelineFeed timeline={timeline} aria-labelledby={labelledBy} />;
  return <StreamedPublicTimeline timeline={timeline} aria-labelledby={labelledBy} />;
}

function StreamedPublicTimeline({ timeline, "aria-labelledby": labelledBy }: PublicTimelineProps) {
  usePublicTimelineStream(timeline);
  return (
    <Stack>
      <NewPostsFeed timeline={timeline} aria-labelledby={labelledBy} />
    </Stack>
  );
}
