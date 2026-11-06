// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { AccountManageView, adminAccountQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_`s keep the page out of the layouts of the search and of the account's review page. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * The account management of a local account: its roles (manage_roles, FR-720), its email suppression, its two-factor
 * authentication and password reset (manage_login_recovery, FR-005, FR-002) and its email and IP addresses (FR-714),
 * each for the viewer with its permission, on the account screen of manage_accounts.
 */
export const Route = createFileRoute("/moderation/accounts_/$id_/manage")({
  staticData: { title: "client:admin.account.manage.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_accounts", () =>
      context.queryClient.query({ ...adminAccountQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: AccountManageRoute,
  errorComponent: AccountManageError,
});

function AccountManageRoute() {
  const t = useT();
  const { id } = Route.useParams();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.account.manage.title")}</PageHeading>
        <Link href="/moderation/accounts" variant="standalone" size="sm">
          {t("client:admin.account.back")}
        </Link>
      </Stack>
      {/* Another account is another page: nothing of the previous one carries over, its private data least of all (web-client/README.md R10). */}
      <AccountManageView key={id} id={id} />
    </>
  );
}

function AccountManageError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.account.manage.title")} />;
}
