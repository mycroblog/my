// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { HookDecisionDetailView, hookDecisionQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the layout of the screen's first page at `/moderation/hook-decisions`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One decision of an external review with every fact of it, and cancelling it (ADR-0147 §9), for manage_reports. */
export const Route = createFileRoute("/moderation/hook-decisions_/$id")({
  staticData: { title: "client:admin.hook_decision.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_reports", () =>
      context.queryClient.query({ ...hookDecisionQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: HookDecisionPage,
  errorComponent: HookDecisionError,
});

function HookDecisionHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.hook_decision.title")}</PageHeading>
      <Link href="/moderation/hook-decisions" variant="standalone" size="sm">
        {t("client:admin.hook_decision.back")}
      </Link>
    </Stack>
  );
}

function HookDecisionPage() {
  const { id } = Route.useParams();
  return (
    <>
      <HookDecisionHeading />
      {/* Another decision is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <HookDecisionDetailView key={id} id={id} />
    </>
  );
}

function HookDecisionError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_reports" heading={t("client:admin.hook_decision.title")} />;
}
