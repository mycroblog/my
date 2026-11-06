// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewLinkCardDomainRule } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * Setting the rule of the link card crawler for a domain, for manage_settings; saving opens the rule's page. The `_`
 * keeps the page out of the list's layout at `/admin/link-card-domains`.
 */
export const Route = createFileRoute("/admin/link-card-domains_/new")({
  staticData: { title: "client:admin.link_card_domains.new.title" },
  beforeLoad: requireSignIn,
  component: NewRulePage,
  errorComponent: NewRuleError,
});

function NewRulePage() {
  const t = useT();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.link_card_domains.new.title")}</PageHeading>
        <Link href="/admin/link-card-domains" variant="standalone" size="sm">
          {t("client:admin.link_card_domains.back")}
        </Link>
      </Stack>
      <NewLinkCardDomainRule />
    </>
  );
}

function NewRuleError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.link_card_domains.new.title")} />;
}
