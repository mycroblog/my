// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { LinkCardDomainSettings } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `domain`: the search of the list (web-client/README.md S3), a part of the domain; omitted lists every rule. */
const searchSchema = z.object({
  domain: z.string().trim().min(1).max(253).optional().catch(undefined),
});

/** The rules of the link card crawler: which domains it fetches nothing or no images from, for manage_settings. */
export const Route = createFileRoute("/admin/link-card-domains")({
  validateSearch: searchSchema,
  staticData: { title: "client:admin.link_card_domains.title" },
  beforeLoad: requireSignIn,
  component: LinkCardDomainsPage,
  errorComponent: LinkCardDomainsError,
});

function LinkCardDomainsPage() {
  const t = useT();
  const { domain } = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.link_card_domains.title")}</PageHeading>
      <LinkCardDomainSettings
        search={domain}
        onSearchChange={(search) => {
          void navigate({ search: search === undefined ? {} : { domain: search } });
        }}
      />
    </>
  );
}

function LinkCardDomainsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.link_card_domains.title")} />;
}
