// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { SignupBlockImportPage, SignupBlocksNav } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { notFoundOnSingleUser } from "../lib/singleUser";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The import of a list of blocks on sign-ups (FR-719), with the lines it skipped, for manage_accounts. */
export const Route = createFileRoute("/moderation/signup-blocks_/import")({
  staticData: { title: "client:admin.signup_blocks.title" },
  // A one-person server has none of this (ADR-0152 §4): the address is not found there.
  beforeLoad: async (match) => {
    await requireSignIn(match);
    await notFoundOnSingleUser(match);
  },
  loader: ({ context }) => loadWithPermission(context, "manage_accounts", () => Promise.resolve()),
  component: SignupBlockImportRoute,
  errorComponent: SignupBlockImportError,
});

function SignupBlockImportRoute() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.signup_blocks.title")}</PageHeading>
      <SignupBlocksNav current="import" />
      <SignupBlockImportPage />
    </>
  );
}

function SignupBlockImportError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.signup_blocks.title")} />;
}
