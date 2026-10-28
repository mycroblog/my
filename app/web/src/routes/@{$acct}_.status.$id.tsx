// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps, useCanGoBack, useRouter } from "@tanstack/react-router";
import { z } from "zod";
import { StatusThread, statusContextQueryOptions, statusQueryOptions } from "../features/status";
import { TimelineSkeleton } from "../features/timeline";
import { ApiError } from "../lib/errors";
import { useT } from "../lib/i18n";
import { HomeLink } from "./-layout/HomeLink";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/**
 * `/@{acct}/status/{id}` (ADR-0053): the author's handle only makes the address readable; the ID finds the
 * post. The `_` keeps the page out of a profile page's layout at `/@{acct}`.
 */
const paramsSchema = z.object({ acct: z.string().min(1), id: z.string().min(1) });

/** A post and the thread around it (FR-113). */
export const Route = createFileRoute("/@{$acct}_/status/$id")({
  staticData: { title: "client:status.title", back: true },
  params: { parse: (params) => paramsSchema.parse(params) },
  loader: async ({ context: { queryClient, api }, params: { id } }) => {
    // The thread starts loading with the post but does not hold the page back; its failure shows in its
    // own section (StatusThread), not as the page's error.
    queryClient.query(statusContextQueryOptions(api, id)).catch(() => undefined);
    await queryClient.query({ ...statusQueryOptions(api, id), staleTime: Infinity });
  },
  component: StatusPage,
  pendingComponent: StatusPending,
  errorComponent: StatusError,
});

function StatusPage() {
  const t = useT();
  const { id } = Route.useParams();
  const router = useRouter();
  const canGoBack = useCanGoBack();
  // The deleted post's page has nothing left to show: go back to where the reader came from.
  const leave = () => {
    if (canGoBack) router.history.back();
    else void router.navigate({ to: "/home", replace: true });
  };
  return (
    <>
      <PageHeading>{t("client:status.title")}</PageHeading>
      {/* Another post is another thread: nothing of the previous one carries over (web-client/README.md R10). */}
      <StatusThread key={id} id={id} onDeleted={leave} />
    </>
  );
}

function StatusPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:status.title")}</PageHeading>
      <TimelineSkeleton />
    </>
  );
}

/** A post that does not exist and one the viewer may not see are the same 404 (api/openapi.yaml, getStatus). */
function StatusError(props: ErrorComponentProps) {
  const t = useT();
  if (!(props.error instanceof ApiError && props.error.status === 404)) return <RouteError {...props} />;
  return (
    <>
      <PageHeading>{t("client:status.not_found.title")}</PageHeading>
      <p>{t("client:status.not_found.body")}</p>
      <HomeLink />
    </>
  );
}
