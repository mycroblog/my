// SPDX-License-Identifier: AGPL-3.0-only
import { Box, Stack } from "@my/ui";
import { useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { useIntl } from "react-intl";
import { useApi, useSessionStatus, useSessionUnreachable } from "../../../lib/auth/SessionProvider";
import { SignInPrompt } from "../../session";
import { useT } from "../../../lib/i18n";
import { useIsOffline } from "../../../lib/network";
import { keepHomeTimelineSaved } from "../api/savedHome";
import { timelineQueryOptions } from "../api/timelines";
import { NewPostsFeed } from "./NewPostsFeed";
import { TimelineFeed } from "./TimelineFeed";

export type HomeTimelineProps = {
  /** The ID of the page heading that names the timeline. */
  "aria-labelledby": string;
  /**
   * Whether the route loader put the home timeline kept on this browser into the query cache, because the
   * session could not be resumed for the network (a page opened offline, FR-604).
   */
  saved?: boolean;
};

/**
 * The home timeline of the signed-in user, or the way to sign in. The route loads it only for a signed-in user,
 * or, while the server cannot be reached to resume the session, the posts last loaded on this browser.
 */
export function HomeTimeline({ "aria-labelledby": labelledBy, saved = false }: HomeTimelineProps) {
  const t = useT();
  const status = useSessionStatus();
  const unreachable = useSessionUnreachable();
  if (status === "signed-in") return <SignedInHomeTimeline aria-labelledby={labelledBy} />;
  if (saved && unreachable) {
    return (
      <Stack>
        <OfflineNotice />
        <TimelineFeed timeline={{ kind: "home" }} aria-labelledby={labelledBy} />
      </Stack>
    );
  }
  return (
    <SignInPrompt
      heading={t("client:timeline.signed_out.heading")}
      description={t("client:timeline.signed_out.description")}
      returnTo="/home"
    />
  );
}

/**
 * The feed with the "N new posts" button over its top (NewPostsFeed). While it is shown, the newest posts are kept on
 * this browser for reading offline (FR-604).
 */
function SignedInHomeTimeline({ "aria-labelledby": labelledBy }: HomeTimelineProps) {
  const queryClient = useQueryClient();
  const api = useApi();
  const offline = useIsOffline();
  // Synchronizes the browser's storage with the query cache, two stores outside React (web-client/README.md R8).
  useEffect(() => keepHomeTimelineSaved(queryClient, api), [queryClient, api]);
  return (
    <Stack>
      {offline && <OfflineNotice />}
      <NewPostsFeed timeline={{ kind: "home" }} aria-labelledby={labelledBy} />
    </Stack>
  );
}

/**
 * Says, above the posts, that they are the ones loaded last and when. The shell's offline bar announces the
 * change itself (web-client/09-app-shell.md §9), so this is plain text in the reading order, not another live region.
 */
function OfflineNotice() {
  const t = useT();
  const intl = useIntl();
  const api = useApi();
  const { dataUpdatedAt } = useSuspenseInfiniteQuery(timelineQueryOptions(api, { kind: "home" }));
  const time = intl.formatDate(dataUpdatedAt, { dateStyle: "medium", timeStyle: "short" });
  return (
    <Box as="p" border="bottom" paddingX="4" paddingY="3" textSize="sm" tone="muted">
      {t("client:timeline.offline.home", { time })}
    </Box>
  );
}
