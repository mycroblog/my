// SPDX-License-Identifier: AGPL-3.0-only

// A public timeline of the instance (ADR-0110 §11): the local posts, or the posts it knows from every server, each a
// screen of its own with no tab to the other. A reader without a session reads it while the instance shows it to
// everyone; the actions of their posts that need an account lead to the sign-in the app gives (`signIn`).

import { Column, statusPath, type PostStatus } from "@my/app-components";
import { publicTimelineQuery, usePublicTimeline, type AppData, type PublicTimelineKind } from "@my/app-data";
import { useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { usePostActions } from "../post-actions";

/** What the route's loader waits for: the timeline's first page, from the cache unless a write marked it stale. */
export async function loadPublicTimeline(data: AppData, kind: PublicTimelineKind): Promise<void> {
  await data.queryClient
    .infiniteQuery({ ...publicTimelineQuery(data, kind), staleTime: Infinity })
    .catch(() => undefined);
}

/**
 * The column of the public timeline of `kind`; with `signIn`, for a reader without an account, whom the actions that
 * need one lead to sign in, given the address of the post to come back to.
 */
export function PublicTimeline({
  kind,
  signIn,
}: {
  kind: PublicTimelineKind;
  signIn?: ((returnTo: string) => void) | undefined;
}) {
  const { t } = stringsIn(useLocale().locale);
  const onAction = usePostActions();
  const visitor =
    signIn === undefined
      ? undefined
      : (status: PostStatus) => {
          signIn(statusPath(status));
        };
  return (
    <Column
      label={kind === "local" ? t("timeline.local") : t("timeline.federated")}
      {...usePublicTimeline(kind)}
      onAction={onAction}
      postProps={visitor === undefined ? undefined : () => ({ signIn: visitor })}
    />
  );
}
