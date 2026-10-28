// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  AccountSearch,
  adminAccountContactFields,
  adminAccountOrigins,
  type AdminAccountSearch,
  adminAccountSearchLimit,
  adminAccountsQueryOptions,
  adminAccountStates,
  isMissingPermission,
} from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

const text = z.string().trim().min(1).max(adminAccountSearchLimit).optional().catch(undefined);

/**
 * The search of the accounts, with the API's names (web-client/README.md S3); omitted, or anything unknown, keeps every
 * account. Of `email`, `email_domain` and `ip`, the first given is kept: the API takes one at a time.
 */
const searchSchema = z.object({
  username: text,
  domain: text,
  origin: z.enum(adminAccountOrigins).optional().catch(undefined),
  state: z.enum(adminAccountStates).optional().catch(undefined),
  email: text,
  email_domain: text,
  ip: text,
});

type Search = z.infer<typeof searchSchema>;

function searchOf(search: Search): AdminAccountSearch {
  const contact = adminAccountContactFields.find((field) => search[field] !== undefined);
  return {
    username: search.username,
    domain: search.domain,
    origin: search.origin,
    state: search.state,
    ...(contact === undefined ? {} : { [contact]: search[contact] }),
  };
}

/** The local and remote accounts (FR-714), for manage_accounts. */
export const Route = createFileRoute("/moderation/accounts")({
  staticData: { title: "client:admin.accounts.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { search: AdminAccountSearch } => ({ search: searchOf(search) }),
  beforeLoad: requireSignIn,
  // A search the server refuses (an IP address that is none, say) is the results' own failure, shown below the
  // form with what to do (useAccountSearchResults), so the loader lets the page open; a missing permission is
  // the page's.
  loader: ({ context, deps: { search } }) =>
    loadWithPermission(context, "manage_accounts", () =>
      context.queryClient
        .infiniteQuery({ ...adminAccountsQueryOptions(context.api, search), staleTime: Infinity })
        .catch((error: unknown) => {
          if (isMissingPermission(error)) throw error;
        }),
    ),
  component: AccountsPage,
  errorComponent: AccountsError,
});

function AccountsPage() {
  const t = useT();
  const { search } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.accounts.title")}</PageHeading>
      <AccountSearch
        search={search}
        onSearch={(next) => {
          void navigate({ search: next });
        }}
      />
    </>
  );
}

function AccountsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.accounts.title")} />;
}
