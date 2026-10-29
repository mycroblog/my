// SPDX-License-Identifier: AGPL-3.0-only
// The emoji reactions under a post (FR-116, ADR-0052 §5): one toggle per emoji with its count, which the viewer
// presses to react with that emoji or to take the reaction back, and a button that lists the accounts that reacted.
// A custom emoji of another server is shown with its count but is no button: nobody here can react with it.
import {
  AccountItem,
  Box,
  Button,
  Cluster,
  Dialog,
  EmojiText,
  EmptyState,
  ReactionChip as UIReactionChip,
  ReactionEmoji as UIReactionEmoji,
  ReactionRow,
  RegionError,
  Stack,
} from "@my/ui";
import { useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import type { ReactionEmoji, Status } from "../api/status";
import { type ReactionChip, useReactors, useStatusReactions } from "../hooks/useStatusReactions";

export type StatusReactionsProps = {
  status: Status;
};

/** The reactions of `status` in a group, or nothing without any. */
export function StatusReactions({ status }: StatusReactionsProps) {
  const t = useT();
  const chips = useStatusReactions(status);
  const [isListOpen, setIsListOpen] = useState(false);
  if (chips.length === 0) return null;
  const post = status.boost ?? status;
  return (
    <ReactionRow
      label={t("client:reaction.list.label")}
      listLabel={t("client:reaction.list.open")}
      onOpenList={() => {
        setIsListOpen(true);
      }}
      after={<ReactorsDialog statusId={post.id} isOpen={isListOpen} onOpenChange={setIsListOpen} />}
    >
      {chips.map((chip) => (
        <Chip key={chip.key} chip={chip} />
      ))}
    </ReactionRow>
  );
}

/** The emoji of a reaction: the Unicode emoji, or the image of a custom emoji (its `:shortcode:` without one). */
function Emoji({ emoji, text }: { emoji: ReactionEmoji; text: string }) {
  return <UIReactionEmoji image={emoji.emoji} text={emoji.shortcode === undefined ? emoji.name : text} />;
}

/** One emoji with its count: a toggle (named by the emoji and the count it shows), or the same shown only. */
function Chip({ chip }: { chip: ReactionChip }) {
  const t = useT();
  const intl = useIntl();
  const count = intl.formatNumber(chip.count, { notation: "compact" });
  return (
    <UIReactionChip
      image={chip.emoji.emoji}
      text={chip.emoji.shortcode === undefined ? chip.emoji.name : chip.text}
      count={count}
      label={t("client:reaction.toggle", { emoji: chip.text, count })}
      isSelected={chip.isSelected}
      onChange={chip.onChange}
    />
  );
}

type ReactorsDialogProps = {
  statusId: string;
  isOpen: boolean;
  onOpenChange: (isOpen: boolean) => void;
};

/**
 * The accounts that reacted to the post, newest first, each with its emoji, in a dialog. A button rather than loading
 * at the end of the scroll loads the next page, which keeps the end of the list reachable by keyboard (WCAG 2.1.1).
 */
function ReactorsDialog({ statusId, isOpen, onOpenChange }: ReactorsDialogProps) {
  const t = useT();
  const state = useReactors(statusId, isOpen);
  return (
    <Dialog
      title={t("client:reaction.list.title")}
      closeLabel={t("client:reaction.list.close")}
      isOpen={isOpen}
      onOpenChange={onOpenChange}
    >
      {state.status === "loading" ? (
        <p role="status">{t("client:reaction.list.loading")}</p>
      ) : state.status === "error" ? (
        <RegionError
          heading={t("client:reaction.list.failed")}
          reason={state.error}
          retryLabel={t("client:reaction.list.retry")}
          onRetry={state.retry}
          alert
        />
      ) : (
        <Stack gap="3">
          {state.items.length === 0 ? (
            <EmptyState headingLevel={3} heading={t("client:reaction.list.empty")} />
          ) : (
            <Stack as="ul" divided>
              {state.items.map((item) => (
                <Box as="li" key={item.key} paddingY="2">
                  <AccountItem
                    name={item.name}
                    nameContent={<EmojiText text={item.name} emojis={item.emojis} />}
                    handle={item.handle}
                    avatar={item.avatar}
                    avatarSeed={item.id}
                    href={item.href}
                    trailing={<Emoji emoji={item.emoji} text={item.emojiText} />}
                  />
                </Box>
              ))}
            </Stack>
          )}
          {state.loadMoreError !== null && (
            <p>{t("client:reaction.list.load_more_failed", { reason: state.loadMoreError })}</p>
          )}
          {state.hasMore && (
            <Cluster justify="center">
              {/* A pending button keeps its focus while the page loads (React Aria Button, isPending). */}
              <Button isPending={state.isLoadingMore} onPress={state.loadMore}>
                {t("client:reaction.list.load_more")}
              </Button>
            </Cluster>
          )}
        </Stack>
      )}
    </Dialog>
  );
}
