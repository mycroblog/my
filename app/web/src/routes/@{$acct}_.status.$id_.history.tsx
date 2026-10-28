// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { StatusHistory, statusHistoryQueryOptions, statusQueryOptions } from "../features/status";
import { TimelineSkeleton } from "../features/timeline";
import { ApiError } from "../lib/errors";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/**
 * `/@{acct}/status/{id}/history`: as on the post's page, the handle only makes the address readable. The `_`s keep
 * the page out of the layouts of the profile and of the post's page.
 */
const paramsSchema = z.object({ acct: z.string().min(1), id: z.string().min(1) });

/** The edit history of a post (FR-109): every revision, newest first. */
export const Route = createFileRoute("/@{$acct}_/status/$id_/history")({
  staticData: { title: "client:status.history.title", back: true },
  params: { parse: (params) => paramsSchema.parse(params) },
  loader: async ({ context: { queryClient, api }, params: { id } }) => {
    await Promise.all([
      queryClient.query({ ...statusQueryOptions(api, id), staleTime: Infinity }),
      queryClient.query({ ...statusHistoryQueryOptions(api, id), staleTime: Infinity }),
    ]);
  },
  component: StatusHistoryPage,
  pendingComponent: StatusHistoryPending,
  errorComponent: StatusHistoryError,
});

function StatusHistoryPage() {
  const t = useT();
  const { id } = Route.useParams();
  return (
    <>
      <PageHeading>{t("client:status.history.title")}</PageHeading>
      {/* Another post is another history: nothing of the previous one carries over (web-client/README.md R10). */}
      <StatusHistory key={id} id={id} />
    </>
  );
}

function StatusHistoryPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:status.history.title")}</PageHeading>
      <TimelineSkeleton />
    </>
  );
}

/** A post that does not exist and one the viewer may not see are the same 404 (api/openapi.yaml, getStatusHistory). */
function StatusHistoryError(props: ErrorComponentProps) {
  const t = useT();
  if (!(props.error instanceof ApiError && props.error.status === 404)) return <RouteError {...props} />;
  return (
    <>
      <PageHeading>{t("client:status.not_found.title")}</PageHeading>
      <p>{t("client:status.not_found.body")}</p>
    </>
  );
}
