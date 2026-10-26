// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { useId } from "react";
import { z } from "zod";
import { TimelineFeed, TimelineSkeleton, timelineQueryOptions } from "../features/timeline";
import { TagVoiceRooms } from "../features/voice";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/** The hashtag, without the `#` (api/openapi.yaml, getHashtagTimeline); the hashtag links of posts point here. */
const paramsSchema = z.object({ tag: z.string().min(1).max(256) });

/** The timeline of one hashtag (FR-203), under the open voice chat rooms with the hashtag (ADR-0100 §8). */
export const Route = createFileRoute("/tags/$tag")({
  // A feed to add to: the shell floats the new-post button over this page (web-client/16-look-states-and-wording.md G4).
  staticData: { composeButton: true },
  params: { parse: (params) => paramsSchema.parse(params) },
  loader: async ({ context: { queryClient, api }, params: { tag } }) => {
    // Cached pages that no write has marked stale are shown at once (and refetched in the background once
    // old); marked or missing ones are fetched and waited for (web-client/README.md S7).
    await queryClient.infiniteQuery({ ...timelineQueryOptions(api, { kind: "tag", tag }), staleTime: Infinity });
  },
  component: TagPage,
  pendingComponent: TagPending,
  errorComponent: TagError,
});

function TagHeading({ tag, id }: { tag: string; id?: string }) {
  const t = useT();
  return (
    <PageHeading {...(id !== undefined && { id })}>
      {/* The hashtag is the user's text, in any script (web-client/README.md Y10). */}
      <span dir="auto">{t("client:tag.title", { tag })}</span>
    </PageHeading>
  );
}

function TagPage() {
  const { tag } = Route.useParams();
  const headingId = useId();
  return (
    <>
      <TagHeading tag={tag} id={headingId} />
      <TagVoiceRooms key={tag} tag={tag} />
      {/* A new hashtag is a new timeline: nothing of the previous one carries over (web-client/README.md R10). */}
      <TimelineFeed key={tag} timeline={{ kind: "tag", tag }} aria-labelledby={headingId} />
    </>
  );
}

function TagPending() {
  const { tag } = Route.useParams();
  return (
    <>
      <TagHeading tag={tag} />
      <TimelineSkeleton />
    </>
  );
}

/** The page keeps its name, the hashtag, when it fails (web-client/16-look-states-and-wording.md G16). */
function TagError(props: ErrorComponentProps) {
  const t = useT();
  const { tag } = Route.useParams();
  return <RouteError {...props} heading={<span dir="auto">{t("client:tag.title", { tag })}</span>} />;
}
