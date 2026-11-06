// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  adminCustomEmojiRequestsQueryOptions,
  type CustomEmojiRequestState,
  CustomEmojiRequestsPage,
} from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `state`: the state of the listed requests; omitted, or anything unknown, shows the pending ones. */
const searchSchema = z.object({
  state: z.enum(["approved", "rejected", "withdrawn", "lapsed"]).optional().catch(undefined),
});

/**
 * The requests for new custom emoji of the holders of create_custom_emojis (ADR-0070 §2), a page of the custom emoji
 * screen, which an administrator's notification of a request opens, for manage_custom_emojis. The `_` keeps the page
 * out of the list's layout at `/admin/custom-emojis`.
 */
export const Route = createFileRoute("/admin/custom-emojis_/requests")({
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { state: CustomEmojiRequestState } => ({ state: search.state ?? "pending" }),
  staticData: { title: "client:admin.custom_emoji_requests.title" },
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_custom_emojis", () =>
      context.queryClient.infiniteQuery({
        ...adminCustomEmojiRequestsQueryOptions(context.api, deps.state),
        staleTime: Infinity,
      }),
    ),
  component: CustomEmojiRequestsRoute,
  errorComponent: CustomEmojiRequestsError,
});

function CustomEmojiRequestsRoute() {
  const t = useT();
  const { state } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.custom_emojis.title")}</PageHeading>
      <CustomEmojiRequestsPage
        state={state}
        onStateChange={(next) => {
          void navigate({ search: { state: next === "pending" ? undefined : next } });
        }}
      />
    </>
  );
}

function CustomEmojiRequestsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_custom_emojis" heading={t("client:admin.custom_emojis.title")} />;
}
