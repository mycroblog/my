// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewSignupBlock } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The form of a new block on sign-ups (FR-719), for manage_accounts. The `_` keeps it out of the list's layout. */
export const Route = createFileRoute("/moderation/signup-blocks_/new")({
  staticData: { title: "client:admin.signup_blocks.new.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => loadWithPermission(context, "manage_accounts", () => Promise.resolve()),
  component: NewSignupBlockPage,
  errorComponent: NewSignupBlockError,
});

function NewSignupBlockPage() {
  const t = useT();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.signup_blocks.new.title")}</PageHeading>
        <Link href="/moderation/signup-blocks" variant="standalone" size="sm">
          {t("client:admin.signup_blocks.back")}
        </Link>
      </Stack>
      <NewSignupBlock />
    </>
  );
}

function NewSignupBlockError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.signup_blocks.new.title")} />;
}
