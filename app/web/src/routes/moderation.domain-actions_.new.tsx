// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { NewDomainAction } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `domain`: the domain to fill in (the link from the list of one domain's actions); omitted, or anything that is
 * not a domain's length, leaves the field empty (web-client/README.md S3).
 */
const searchSchema = z.object({
  domain: z.string().trim().min(1).max(253).optional().catch(undefined),
});

/** The form of a new moderation action on a remote domain (FR-702), for manage_domains. The `_` keeps it out of the list's layout. */
export const Route = createFileRoute("/moderation/domain-actions_/new")({
  validateSearch: searchSchema,
  staticData: { title: "client:admin.domain_actions.new.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => loadWithPermission(context, "manage_domains", () => Promise.resolve()),
  component: NewDomainActionPage,
  errorComponent: NewDomainActionError,
});

function NewDomainActionHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.domain_actions.new.title")}</PageHeading>
      <Link href="/moderation/domain-actions" variant="standalone" size="sm">
        {t("client:admin.domain_actions.back")}
      </Link>
    </Stack>
  );
}

function NewDomainActionPage() {
  const { domain } = Route.useSearch();
  return (
    <>
      <NewDomainActionHeading />
      <NewDomainAction domain={domain} />
    </>
  );
}

function NewDomainActionError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_domains" heading={t("client:admin.domain_actions.new.title")} />;
}
