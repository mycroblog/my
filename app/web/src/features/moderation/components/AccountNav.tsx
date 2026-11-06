// SPDX-License-Identifier: AGPL-3.0-only
// The pages of an account's moderation screen (web-client/14-admin.md §14, the split of a screen): the review with
// the decision and its evidence (/moderation/accounts/$id), every action on the account (…/actions), and the
// account management of a local account (…/manage: roles, sign-in recovery, email and IP addresses), which a
// remote account does not have. A LocalNav under the page's h1, with the account it is about below it, so that
// each page says whose it is.
import { AccountItem, LocalNav } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import type { AdminAccountDetail } from "../api/accounts";
import { accountName, handleOf } from "../hooks/useReports";

/** Which page of the account is on screen. */
export type AccountPage = "review" | "actions" | "manage";

/** The address of a page of account `id`. */
export function accountPageHref(id: string, page: AccountPage): string {
  const base = `/moderation/accounts/${encodeURIComponent(id)}`;
  return page === "review" ? base : `${base}/${page}`;
}

export function AccountNav({ account, current }: { account: AdminAccountDetail; current: AccountPage | undefined }) {
  const t = useT();
  const pages: { page: AccountPage; label: string }[] = [
    { page: "review", label: t("client:admin.account.pages.review") },
    { page: "actions", label: t("client:admin.account.pages.actions") },
    // Roles, sign-in recovery and private data belong to a user of this server.
    ...(account.local ? [{ page: "manage" as const, label: t("client:admin.account.pages.manage") }] : []),
  ];
  return (
    <LocalNav
      label={t("client:admin.account.pages")}
      items={pages.map(({ page, label }) => ({
        href: accountPageHref(account.id, page),
        label,
        current: page === current,
      }))}
    />
  );
}

/** The account a sub-page is about: its avatar, name and handle, under the pages. */
export function AccountOfPage({ account }: { account: AdminAccountDetail }) {
  return (
    <AccountItem
      name={accountName(account.account)}
      handle={account.account.display_name === "" ? undefined : handleOf(account.account)}
      avatar={imageSources(account.account.avatar)}
      avatarSeed={account.account.id}
    />
  );
}
