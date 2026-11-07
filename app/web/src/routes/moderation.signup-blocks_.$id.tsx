// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { SignupBlockDetail, signupBlockQueryOptions, SignupBlockTitle } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { notFoundOnSingleUser } from "../lib/singleUser";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/moderation/signup-blocks`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One block on sign-ups (FR-719): its comment and dates, and lifting it, for manage_accounts. */
export const Route = createFileRoute("/moderation/signup-blocks_/$id")({
  staticData: { title: "client:admin.signup_blocks.detail.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  // A one-person server has none of this (ADR-0152 §4): the address is not found there.
  beforeLoad: async (match) => {
    await requireSignIn(match);
    await notFoundOnSingleUser(match);
  },
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_accounts", () =>
      context.queryClient.query({ ...signupBlockQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: SignupBlockPage,
  errorComponent: SignupBlockError,
});

function SignupBlockPage() {
  const t = useT();
  const { id } = Route.useParams();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>
          <SignupBlockTitle id={id} />
        </PageHeading>
        <Link href="/moderation/signup-blocks" variant="standalone" size="sm">
          {t("client:admin.signup_blocks.back")}
        </Link>
      </Stack>
      {/* Another block is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <SignupBlockDetail key={id} id={id} />
    </>
  );
}

function SignupBlockError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.signup_blocks.detail.title")} />;
}
