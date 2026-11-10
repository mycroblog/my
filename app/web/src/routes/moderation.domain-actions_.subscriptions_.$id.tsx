// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  domainListDraftsQueryOptions,
  domainListFetchesQueryOptions,
  DomainListSubscriptionDetail,
  domainListSubscriptionQueryOptions,
  DomainListSubscriptionTitle,
} from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/moderation/domain-actions/subscriptions`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One subscription to a domain blocklist (ADR-0159 §5): its drafts, its fetches, its settings and deleting it, for
 * manage_domains.
 */
export const Route = createFileRoute("/moderation/domain-actions_/subscriptions_/$id")({
  staticData: { title: "client:admin.domain_lists.detail.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_domains", () =>
      // Every section of the page is read before it shows (web-client/README.md S7).
      Promise.all([
        context.queryClient.query({ ...domainListSubscriptionQueryOptions(context.api, id), staleTime: Infinity }),
        context.queryClient.query({ ...domainListDraftsQueryOptions(context.api, id), staleTime: Infinity }),
        context.queryClient.infiniteQuery({ ...domainListFetchesQueryOptions(context.api, id), staleTime: Infinity }),
      ]),
    ),
  component: SubscriptionPage,
  errorComponent: SubscriptionError,
});

function SubscriptionPage() {
  const t = useT();
  const { id } = Route.useParams();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>
          <DomainListSubscriptionTitle id={id} />
        </PageHeading>
        <Link href="/moderation/domain-actions/subscriptions" variant="standalone" size="sm">
          {t("client:admin.domain_lists.back")}
        </Link>
      </Stack>
      {/* Another subscription is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <DomainListSubscriptionDetail key={id} id={id} />
    </>
  );
}

function SubscriptionError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_domains" heading={t("client:admin.domain_lists.detail.title")} />;
}
