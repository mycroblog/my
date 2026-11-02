// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { LinkCardDomainSettings } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The rules of the link card crawler: which domains it fetches nothing or no images from, for manage_settings. */
export const Route = createFileRoute("/admin/link-card-domains")({
  staticData: { title: "client:admin.link_card_domains.title" },
  beforeLoad: requireSignIn,
  component: LinkCardDomainsPage,
  errorComponent: LinkCardDomainsError,
});

function LinkCardDomainsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.link_card_domains.title")}</PageHeading>
      <LinkCardDomainSettings />
    </>
  );
}

function LinkCardDomainsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.link_card_domains.title")} />;
}
