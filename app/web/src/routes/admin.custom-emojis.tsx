// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  adminCustomEmojiRequestsQueryOptions,
  adminCustomEmojisQueryOptions,
  type CustomEmojiFilter,
  type CustomEmojiRequestState,
  CustomEmojiSettings,
  type CustomEmojiTab,
} from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `tab`: the requests for new emoji (ADR-0070 §2), which an administrator's notification of a request opens; omitted,
 * or anything unknown, shows the emoji.
 * `state`: the state of the listed requests; omitted, or anything unknown, shows the pending ones.
 * `origin`: the tab of the list, the emoji of other servers; omitted, or anything unknown, shows this server's.
 * `shortcode` and `domain`: the search (web-client/README.md S3); the domain counts for other servers only.
 */
const searchSchema = z.object({
  tab: z.literal("requests").optional().catch(undefined),
  state: z.enum(["approved", "rejected", "withdrawn", "lapsed"]).optional().catch(undefined),
  origin: z.literal("remote").optional().catch(undefined),
  shortcode: z.string().trim().min(1).max(128).optional().catch(undefined),
  domain: z.string().trim().min(1).max(253).optional().catch(undefined),
});

/**
 * The custom emoji (ADR-0051): registering this server's and managing those of other servers, and deciding the
 * requests of the holders of create_custom_emojis (ADR-0070), for manage_custom_emojis.
 */
export const Route = createFileRoute("/admin/custom-emojis")({
  validateSearch: searchSchema,
  loaderDeps: ({
    search,
  }): { tab: CustomEmojiTab; requestState: CustomEmojiRequestState; filter: CustomEmojiFilter } => ({
    tab: search.tab ?? "emoji",
    requestState: search.state ?? "pending",
    filter: {
      origin: search.origin ?? "local",
      shortcode: search.shortcode,
      domain: search.origin === "remote" ? search.domain : undefined,
    },
  }),
  staticData: { title: "client:admin.custom_emojis.title" },
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_custom_emojis", () =>
      deps.tab === "requests"
        ? context.queryClient.infiniteQuery({
            ...adminCustomEmojiRequestsQueryOptions(context.api, deps.requestState),
            staleTime: Infinity,
          })
        : context.queryClient.infiniteQuery({
            ...adminCustomEmojisQueryOptions(context.api, deps.filter),
            staleTime: Infinity,
          }),
    ),
  component: CustomEmojisPage,
  errorComponent: CustomEmojisError,
});

function CustomEmojisPage() {
  const t = useT();
  const { tab, requestState, filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  const changeFilter = (next: CustomEmojiFilter) => {
    void navigate({
      search: {
        origin: next.origin === "remote" ? "remote" : undefined,
        shortcode: next.shortcode,
        domain: next.domain,
      },
    });
  };
  return (
    <>
      <PageHeading>{t("client:admin.custom_emojis.title")}</PageHeading>
      <CustomEmojiSettings
        tab={tab}
        onTabChange={(next) => {
          void navigate({ search: next === "requests" ? { tab: "requests" } : {} });
        }}
        requestState={requestState}
        onRequestStateChange={(state) => {
          void navigate({ search: { tab: "requests", state: state === "pending" ? undefined : state } });
        }}
        filter={filter}
        onFilterChange={changeFilter}
      />
    </>
  );
}

function CustomEmojisError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_custom_emojis" heading={t("client:admin.custom_emojis.title")} />;
}
