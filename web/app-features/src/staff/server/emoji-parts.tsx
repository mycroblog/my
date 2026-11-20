// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of the custom emoji share: their addresses; the tabs over the emoji and the requests, the pending
// requests counted on the second (ADR-0070 §9); an emoji's image as it shows in a text, or a word where it cannot be
// shown; and the words of an emoji's state and of why the server refused one.

import { usePendingEmojiRequests, type AdminCustomEmoji } from "@my/app-data";
import { CustomEmoji, LinkButton, ScreenTitle, StateMark, TabRow } from "@my/design-system";
import type { ReactNode } from "react";
import { useStaffStrings, type T } from "../parts";

/** The address of the custom emoji, of their requests, of a new one, and of the emoji of `id`. */
export const adminEmojisPath = "/admin/custom-emojis";
export const adminEmojiRequestsPath = `${adminEmojisPath}/requests`;
export const newAdminEmojiPath = `${adminEmojisPath}/new`;
export const adminEmojiPath = (id: string): string => `${adminEmojisPath}/${id}`;

/** A shortcode as a text writes it, between colons. */
export const coded = (shortcode: string): string => `:${shortcode}:`;

/** The image of an emoji, still where the reader keeps motion off; its shortcode's words where it cannot be shown. */
export function EmojiImage({
  shortcode,
  url,
  staticUrl,
}: {
  shortcode: string;
  url?: string | undefined;
  staticUrl?: string | undefined;
}): ReactNode {
  const { t } = useStaffStrings();
  if (url === undefined) return t("staff_emoji.no_image");
  return <CustomEmoji shortcode={shortcode} url={url} staticUrl={staticUrl ?? url} />;
}

/**
 * The state of an emoji in a word, as its row and its page say it: disabled, shown but left out of the picker, or
 * shown.
 */
export function EmojiState({ emoji }: { emoji: AdminCustomEmoji }): ReactNode {
  const { t } = useStaffStrings();
  if (emoji.state === "disabled") return <StateMark state="disabled">{t("staff_emoji.state.disabled")}</StateMark>;
  if (!emoji.visible_in_picker) return <StateMark state="limited">{t("staff_emoji.state.hidden")}</StateMark>;
  return <StateMark state="active">{t("staff_emoji.state.enabled")}</StateMark>;
}

/** Where an emoji comes from, in words: this server, or the domain of another, which never breaks inside. */
export const originOf = (t: T, emoji: AdminCustomEmoji): string =>
  emoji.local ? t("staff_emoji.origin.local") : (emoji.domain ?? t("staff_emoji.origin.remote"));

/** The title of the screens of the custom emoji, with its glyph, the same over both tabs. */
export function EmojiTitle(): ReactNode {
  const { t } = useStaffStrings();
  return <ScreenTitle glyph="emojiCustom">{t("staff_emoji.entry")}</ScreenTitle>;
}

/** The tabs over the emoji and the requests, the pending requests counted, and what the current one shows. */
export function EmojiTabs({
  current,
  go,
  children,
}: {
  current: "emoji" | "requests";
  go: (href: string) => void;
  children: ReactNode;
}): ReactNode {
  const { t } = useStaffStrings();
  const pending = usePendingEmojiRequests();
  return (
    <TabRow
      label={t("staff_emoji.entry")}
      // The one way to register an emoji, at the end of the tabs' line on both tabs, so that nothing moves between them.
      end={
        <LinkButton size="timeline" href={newAdminEmojiPath}>
          {t("staff_emoji.add")}
        </LinkButton>
      }
      tabs={[
        { id: "emoji", name: t("staff_emoji.tab.emoji") },
        {
          id: "requests",
          name: t("staff_emoji.tab.requests"),
          // How many wait, as the navigation's badge counts them; the first page's, and "or more" past it.
          count: pending?.count,
          countLabel:
            pending === undefined
              ? undefined
              : t(pending.more ? "staff_emoji.tab.pending.more" : "staff_emoji.tab.pending", { count: pending.count }),
        },
      ]}
      current={current}
      onChange={(tab) => {
        if (tab !== current) go(tab === "emoji" ? adminEmojisPath : adminEmojiRequestsPath);
      }}
    >
      {children}
    </TabRow>
  );
}
