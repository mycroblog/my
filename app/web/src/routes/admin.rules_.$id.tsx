// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { RuleDetail, ruleQueryOptions, rulesQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/rules`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One rule (FR-705): what it is, changing its text and hint, and retiring it, for manage_rules. */
export const Route = createFileRoute("/admin/rules_/$id")({
  staticData: { title: "client:admin.rule.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_rules", () =>
      Promise.all([
        context.queryClient.query({ ...ruleQueryOptions(context.api, id), staleTime: Infinity }),
        // Its place among the rules in effect.
        context.queryClient.query({ ...rulesQueryOptions(context.api), staleTime: Infinity }),
      ]),
    ),
  component: RulePage,
  errorComponent: RuleError,
});

function RuleHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.rule.title")}</PageHeading>
      <Link href="/admin/rules" variant="standalone" size="sm">
        {t("client:admin.rule.back")}
      </Link>
    </Stack>
  );
}

function RulePage() {
  const { id } = Route.useParams();
  return (
    <>
      <RuleHeading />
      {/* Another rule is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <RuleDetail key={id} id={id} />
    </>
  );
}

function RuleError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_rules" heading={t("client:admin.rule.title")} />;
}
