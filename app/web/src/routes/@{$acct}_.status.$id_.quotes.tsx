// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { useId } from "react";
import { z } from "zod";
import { TimelineFeed, TimelineSkeleton, timelineQueryOptions } from "../features/timeline";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { ApiError } from "../lib/errors";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/**
 * `/@{acct}/status/{id}/quotes`: as on the post's page, the handle only makes the address readable. The `_`s keep
 * the page out of the layouts of the profile and of the post's page.
 */
const paramsSchema = z.object({ acct: z.string().min(1), id: z.string().min(1) });

/**
 * The posts that quote one of the signed-in user's posts, newest first (FR-114, ADR-0069 §5), each with its quote. Only
 * the author lists them, so it needs a session; the approval of a quote is revoked from the quoting post's menu.
 */
export const Route = createFileRoute("/@{$acct}_/status/$id_/quotes")({
  staticData: { title: "client:status.quotes.title", back: true },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context: { queryClient, api }, params: { id } }) =>
    queryClient.infiniteQuery({ ...timelineQueryOptions(api, { kind: "quotes", id }), staleTime: Infinity }),
  component: StatusQuotesPage,
  pendingComponent: StatusQuotesPending,
  errorComponent: StatusQuotesError,
});

function StatusQuotesPage() {
  const t = useT();
  const headingId = useId();
  const { id } = Route.useParams();
  return (
    <>
      <PageHeading id={headingId}>{t("client:status.quotes.title")}</PageHeading>
      {/* Another post is another list: nothing of the previous one carries over (web-client/README.md R10). */}
      <TimelineFeed key={id} timeline={{ kind: "quotes", id }} aria-labelledby={headingId} />
    </>
  );
}

function StatusQuotesPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:status.quotes.title")}</PageHeading>
      <TimelineSkeleton />
    </>
  );
}

/** Another account's post, a missing one and one the viewer may not see are the same 404 (api/openapi.yaml, listStatusQuotes). */
function StatusQuotesError(props: ErrorComponentProps) {
  const t = useT();
  if (!(props.error instanceof ApiError && props.error.status === 404)) return <RouteError {...props} />;
  return (
    <>
      <PageHeading>{t("client:status.not_found.title")}</PageHeading>
      <p>{t("client:status.not_found.body")}</p>
    </>
  );
}
