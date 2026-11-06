// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { DomainActionDetail, domainActionQueryOptions, DomainActionTitle } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/moderation/domain-actions`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One moderation action on a remote domain (FR-702): its comments and dates, and lifting it, for manage_domains. */
export const Route = createFileRoute("/moderation/domain-actions_/$id")({
  staticData: { title: "client:admin.domain_actions.detail.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_domains", () =>
      context.queryClient.query({ ...domainActionQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: DomainActionPage,
  errorComponent: DomainActionError,
});

function DomainActionHeading({ id }: { id: string }) {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>
        <DomainActionTitle id={id} />
      </PageHeading>
      <Link href="/moderation/domain-actions" variant="standalone" size="sm">
        {t("client:admin.domain_actions.back")}
      </Link>
    </Stack>
  );
}

function DomainActionPage() {
  const { id } = Route.useParams();
  return (
    <>
      <DomainActionHeading id={id} />
      {/* Another action is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <DomainActionDetail key={id} id={id} />
    </>
  );
}

function DomainActionError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_domains" heading={t("client:admin.domain_actions.detail.title")} />;
}
