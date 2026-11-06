// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { LinkCardDomainDetail } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/link-card-domains`. */
const paramsSchema = z.object({ domain: z.string().min(1).max(253) });

/**
 * The rule of the link card crawler for one domain: changing what it stops and its note, and removing it, for
 * manage_settings. The API has no endpoint for one rule, so the page reads it from the list, inside its section (G16).
 */
export const Route = createFileRoute("/admin/link-card-domains_/$domain")({
  staticData: { title: "client:admin.link_card_domains.detail.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  component: RulePage,
  errorComponent: RuleError,
});

function RuleHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.link_card_domains.detail.title")}</PageHeading>
      <Link href="/admin/link-card-domains" variant="standalone" size="sm">
        {t("client:admin.link_card_domains.back")}
      </Link>
    </Stack>
  );
}

function RulePage() {
  const { domain } = Route.useParams();
  return (
    <>
      <RuleHeading />
      {/* Another rule is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <LinkCardDomainDetail key={domain} domain={domain} />
    </>
  );
}

function RuleError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError {...props} permission="manage_settings" heading={t("client:admin.link_card_domains.detail.title")} />
  );
}
