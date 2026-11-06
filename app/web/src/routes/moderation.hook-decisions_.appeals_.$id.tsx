// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { HookAppealDetailView, hookAppealQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_`s keep the page out of the layouts of the screen and of the list of appeals. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One appeal of a refused post next to its decision, and upholding or overturning it (ADR-0147 §9), for manage_reports. */
export const Route = createFileRoute("/moderation/hook-decisions_/appeals_/$id")({
  staticData: { title: "client:admin.hook_appeal.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_reports", () =>
      context.queryClient.query({ ...hookAppealQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: HookAppealPage,
  errorComponent: HookAppealError,
});

function HookAppealHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.hook_appeal.title")}</PageHeading>
      <Link href="/moderation/hook-decisions/appeals" variant="standalone" size="sm">
        {t("client:admin.hook_appeal.back")}
      </Link>
    </Stack>
  );
}

function HookAppealPage() {
  const { id } = Route.useParams();
  return (
    <>
      <HookAppealHeading />
      {/* Another appeal is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <HookAppealDetailView key={id} id={id} />
    </>
  );
}

function HookAppealError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_reports" heading={t("client:admin.hook_appeal.title")} />;
}
