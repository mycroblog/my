// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { type RuleFilter, ruleFilters, RuleSettings, rulesQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `filter`: the retired rules instead of those in effect; omitted, or anything unknown, shows the rules in effect. */
const searchSchema = z.object({
  filter: z.enum(ruleFilters).exclude(["active"]).optional().catch(undefined),
});

/** The server's rules (FR-705): the rules in effect in order and moving them, or the retired ones, for manage_rules. */
export const Route = createFileRoute("/admin/rules")({
  staticData: { title: "client:admin.rules.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search: { filter } }): { filter: RuleFilter } => ({ filter: filter ?? "active" }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { filter } }) =>
    loadWithPermission(context, "manage_rules", () =>
      context.queryClient.query({
        ...rulesQueryOptions(context.api, { includeRetired: filter === "retired" }),
        staleTime: Infinity,
      }),
    ),
  component: RulesPage,
  errorComponent: RulesError,
});

function RulesPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.rules.title")}</PageHeading>
      <RuleSettings
        filter={filter}
        onFilterChange={(next) => {
          void navigate({ search: { filter: next === "active" ? undefined : next } });
        }}
      />
    </>
  );
}

function RulesError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_rules" heading={t("client:admin.rules.title")} />;
}
