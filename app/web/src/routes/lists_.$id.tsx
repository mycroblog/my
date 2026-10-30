// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Text } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { useId } from "react";
import { z } from "zod";
import { ListTimelineView, listQueryOptions, useList } from "../features/lists";
import { joinWaitingStatuses, TimelineSkeleton, timelineQueryOptions } from "../features/timeline";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { errorKind } from "../lib/errors";
import { useT } from "../lib/i18n";
import { listsPath } from "../lib/paths";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/** The list's ID (api/spec ListId). */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * The timeline of one of the user's lists (web-client/screens/list-timeline.md, ADR-0122 §4), named after the list.
 * Another user's list, or one that is gone, is not found, and says so with the way to the lists.
 */
export const Route = createFileRoute("/lists_/$id")({
  // A feed to add to: the shell floats the new-post button over this page (web-client/16-look-states-and-wording.md G4).
  staticData: { composeButton: true, back: true, backTo: listsPath },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api }, params: { id } }) => {
    // Cached data that no write has marked stale is shown at once; marked or missing data is fetched and waited for
    // (web-client/README.md S7).
    await Promise.all([
      queryClient.query({ ...listQueryOptions(api, id), staleTime: Infinity }),
      queryClient.infiniteQuery({ ...timelineQueryOptions(api, { kind: "list", id }), staleTime: Infinity }),
    ]);
    // Opening the list from another page: the news that waited for the reader are on it from the first render.
    joinWaitingStatuses(queryClient, api, { kind: "list", id });
  },
  component: ListPage,
  pendingComponent: ListPending,
  errorComponent: ListError,
});

function ListPage() {
  const { id } = Route.useParams();
  const headingId = useId();
  const list = useList(id);
  return (
    <>
      <PageHeading id={headingId}>
        {/* The list's name is the user's text, in any script (web-client/README.md Y10). */}
        <span dir="auto">{list.name}</span>
      </PageHeading>
      {/* A new list is a new timeline: nothing of the previous one carries over (web-client/README.md R10). */}
      <ListTimelineView key={id} id={id} aria-labelledby={headingId} />
    </>
  );
}

function ListPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:lists.title")}</PageHeading>
      <TimelineSkeleton />
    </>
  );
}

/** A list that is not the user's (or is gone) is not found; other failures keep the page's heading (G16). */
function ListError(props: ErrorComponentProps) {
  const t = useT();
  if (errorKind(props.error) !== "not-found") return <RouteError {...props} heading={t("client:lists.title")} />;
  return (
    <>
      <PageHeading>{t("client:lists.not_found.title")}</PageHeading>
      <Text as="p">{t("client:lists.not_found.body")}</Text>
      <Link href={listsPath} variant="standalone">
        {t("client:lists.not_found.link")}
      </Link>
    </>
  );
}
