// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import {
  OwnCustomEmojis,
  ownCustomEmojiRequestsQueryOptions,
  ownCustomEmojisQueryOptions,
} from "../features/custom-emojis";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { loadWithPermission } from "./-admin/adminRoute";
import { SettingsHeading } from "./-layout/SettingsHeading";

/**
 * The custom emoji the user made and their requests for new ones (ADR-0070), for a viewer whose roles have the member
 * permission create_custom_emojis; a viewer without it is told why, and the page reads nothing for them.
 */
export const Route = createFileRoute("/settings/custom-emojis")({
  staticData: { title: "client:custom_emojis.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "create_custom_emojis", () =>
      Promise.all([
        context.queryClient.infiniteQuery({ ...ownCustomEmojisQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.infiniteQuery({ ...ownCustomEmojiRequestsQueryOptions(context.api), staleTime: Infinity }),
      ]),
    ),
  component: CustomEmojisPage,
});

function CustomEmojisPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:custom_emojis.title")}</SettingsHeading>
      <OwnCustomEmojis />
    </>
  );
}
