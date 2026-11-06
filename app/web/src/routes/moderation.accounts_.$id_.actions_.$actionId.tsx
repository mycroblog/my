// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps, notFound } from "@tanstack/react-router";
import { z } from "zod";
import { accountActionQueryOptions, AccountActionDetailView, adminAccountQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_`s keep the page out of the layouts of the search, the account's review page and its actions. */
const paramsSchema = z.object({
  id: z.string().min(1).max(64),
  actionId: z.string().min(1).max(64),
});

/** One moderation action on an account, with every fact of it and its lift (FR-703), for manage_accounts. */
export const Route = createFileRoute("/moderation/accounts_/$id_/actions_/$actionId")({
  staticData: { title: "client:admin.account.action_page.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id, actionId } }) =>
    loadWithPermission(context, "manage_accounts", async () => {
      const [, action] = await Promise.all([
        context.queryClient.query({
          ...adminAccountQueryOptions(context.api, id),
          staleTime: Infinity,
        }),
        context.queryClient.query({
          ...accountActionQueryOptions(context.api, actionId),
          staleTime: Infinity,
        }),
      ]);
      // An action of another account is not on this account's pages: notFound throws the router's "not found".
      if (action.account_id !== id) notFound({ throw: true });
    }),
  component: AccountActionRoute,
  errorComponent: AccountActionError,
});

function AccountActionRoute() {
  const t = useT();
  const { id, actionId } = Route.useParams();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.account.action_page.title")}</PageHeading>
        <Link href={`/moderation/accounts/${encodeURIComponent(id)}/actions`} variant="standalone" size="sm">
          {t("client:admin.account.action_page.back")}
        </Link>
      </Stack>
      {/* Another action is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <AccountActionDetailView key={actionId} id={id} actionId={actionId} />
    </>
  );
}

function AccountActionError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.account.action_page.title")} />;
}
