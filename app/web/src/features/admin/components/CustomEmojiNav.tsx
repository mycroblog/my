// SPDX-License-Identifier: AGPL-3.0-only
// The pages of the custom emoji screen (web-client §14 画面の分け方): the emoji (/admin/custom-emojis) and the requests
// of the holders of create_custom_emojis (/admin/custom-emojis/requests, ADR-0070 §2), whose link carries the number
// of the pending ones (ADR-0070 §9): a decorative Badge, and the number in words for assistive technology.
import { Badge, LocalNav, VisuallyHidden } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { usePendingCustomEmojiRequestCount } from "../hooks/useCustomEmojiRequests";

/** A page of the custom emoji screen. */
export type CustomEmojiPage = "emoji" | "requests";

/** The local navigation under the screen's heading, `current` being the page on screen. */
export function CustomEmojiNav({ current }: { current: CustomEmojiPage }) {
  const t = useT();
  const pending = usePendingCustomEmojiRequestCount();
  const badge =
    pending !== undefined && pending.count > 0 ? (
      <>
        <Badge
          count={pending.more ? pending.count + 1 : pending.count}
          max={pending.more ? pending.count : 99}
          decorative
        />
        <VisuallyHidden>
          {t(
            pending.more ? "client:admin.custom_emojis.pages.pending_more" : "client:admin.custom_emojis.pages.pending",
            {
              count: pending.count,
            },
          )}
        </VisuallyHidden>
      </>
    ) : undefined;
  return (
    <LocalNav
      label={t("client:admin.custom_emojis.pages")}
      items={[
        {
          href: "/admin/custom-emojis",
          label: t("client:admin.custom_emojis.pages.emoji"),
          current: current === "emoji",
        },
        {
          href: "/admin/custom-emojis/requests",
          label: t("client:admin.custom_emojis.pages.requests"),
          current: current === "requests",
          ...(badge !== undefined && { badge }),
        },
      ]}
    />
  );
}
