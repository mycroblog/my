// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { AccountActionsView, adminAccountQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_`s keep the page out of the layouts of the search and of the account's review page. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** Every moderation action on an account, a page at a time, with the lift of those in effect (FR-703), for manage_accounts. */
export const Route = createFileRoute("/moderation/accounts_/$id_/actions")({
  staticData: { title: "client:admin.account.actions.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_accounts", () =>
      context.queryClient.query({ ...adminAccountQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: AccountActionsRoute,
  errorComponent: AccountActionsError,
});

function AccountActionsRoute() {
  const t = useT();
  const { id } = Route.useParams();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.account.actions.title")}</PageHeading>
        <Link href="/moderation/accounts" variant="standalone" size="sm">
          {t("client:admin.account.back")}
        </Link>
      </Stack>
      {/* Another account is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <AccountActionsView key={id} id={id} />
    </>
  );
}

function AccountActionsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.account.actions.title")} />;
}
