// SPDX-License-Identifier: AGPL-3.0-only
import {
  BookmarkIcon,
  BoostIcon,
  Button,
  Dialog,
  DialogActions,
  ActionCount,
  Hint,
  IconButton,
  IconButtonLink,
  IconToggleButton,
  type IconToggleButtonProps,
  Menu,
  MenuItem,
  MenuSeparator,
  MenuTrigger,
  MoreIcon,
  QuoteIcon,
  RepliesIcon,
  ReplyIcon,
  Stack,
  StarIcon,
} from "@my/ui";
import { type ReactNode, useId, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { type ReactionChoice, ReactionPickerButton } from "../../emoji";
import { RelationshipConfirmDialog } from "../../relationship";
import { ReportDialog } from "../../report";
import type { ReactionEmoji, Status } from "../api/status";
import { useAuthorActions } from "../hooks/useAuthorActions";
import { type RelationToggle, useStatusActions } from "../hooks/useStatusActions";
import { EditStatusDialog } from "./EditStatusDialog";
import { QuotePolicyDialog } from "./QuotePolicyDialog";

export type StatusActionsProps = {
  status: Status;
  /** Called after the post is deleted, for a page that shows only this post. */
  onDeleted?: () => void;
  /**
   * The post's page, where its replies are read: the bar then has a link to it with the number of replies, apart from
   * the reply button, which writes one. Left out on that page itself.
   */
  repliesHref?: string | undefined;
};

/** The emoji of a reaction picked in the picker: a Unicode emoji, or a custom emoji of this server. */
function reactionEmojiOf(choice: ReactionChoice): ReactionEmoji {
  if ("unicode" in choice) return { name: choice.unicode };
  const { shortcode, url, static_url } = choice.custom;
  return { name: shortcode, shortcode, emoji: { shortcode, url, static_url } };
}

/**
 * The action bar of a post: reply, boost, quote (FR-114), favourite, the picker of emoji reactions (ADR-0052 §5), bookmark, and a menu with
 * the link, the revocation of this post's approved quote of a post of the viewer (ADR-0069 §3), for another account's post what the
 * viewer can do about its author (useAuthorActions) with the report of the post (features/report) and, with the permission, the
 * links to the moderation page (useStatusActions), and, for the viewer's own posts, edit (EditStatusDialog), who may quote it (QuotePolicyDialog), the posts that
 * quote it, pin or unpin, and delete behind a confirmation (WAI-ARIA APG, alert dialog pattern). Nothing while
 * nobody is signed in. The toggles keep their names in both states and expose the state with
 * `aria-pressed` (APG, button pattern); a count is part of the toggle's name (WCAG 2.5.3). On a post of another server while the
 * instance sends nothing out (ADR-0076), quoting is not offered and a note at the end of the bar, which also
 * describes the reply, boost and favourite, says that the actions do not reach that server.
 */
export function StatusActions({ status, onDeleted, repliesHref }: StatusActionsProps) {
  const t = useT();
  const staysLocalId = useId();
  const actions = useStatusActions(status);
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const author = useAuthorActions(status.boost ?? status, isMenuOpen);
  const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
  const [isReporting, setIsReporting] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [isChoosingQuotePolicy, setIsChoosingQuotePolicy] = useState(false);
  const intl = useIntl();
  if (actions === null) return null;
  const post = status.boost ?? status;
  const remove = actions.delete;
  // The note that the actions on a post of another server do not reach it (ADR-0076) describes each of them.
  const note = actions.staysLocal ? staysLocalId : undefined;

  return (
    <>
      <IconButton
        variant="subtle"
        size="toolbar"
        label={t("client:status.action.reply")}
        {...(note !== undefined && { "aria-describedby": note })}
        onPress={actions.reply}
      >
        <ReplyIcon />
      </IconButton>
      {repliesHref !== undefined && post.replies_count > 0 && (
        <IconButtonLink
          href={repliesHref}
          // The count it shows is part of its name (Y2), formatted as ActionCount shows it.
          label={t("client:status.action.read_replies", {
            count: post.replies_count,
            shown: intl.formatNumber(post.replies_count, { notation: "compact" }),
          })}
        >
          <RepliesIcon />
          <ActionCount value={post.replies_count} />
        </IconButtonLink>
      )}
      <RelationButton
        label={t("client:status.action.boost")}
        toggle={actions.boost}
        tone="success"
        icon={(on) => <BoostIcon filled={on} />}
        describedBy={note}
      />
      {actions.quote !== null && (
        <IconButton
          variant="subtle"
          size="toolbar"
          label={t("client:status.action.quote")}
          onPress={actions.quote.onPress}
          isDisabled={actions.quote.isDisabled}
        >
          <QuoteIcon />
        </IconButton>
      )}
      <RelationButton
        label={t("client:status.action.favourite")}
        toggle={actions.favourite}
        tone="warning"
        icon={(on) => <StarIcon filled={on} />}
        describedBy={note}
        celebrate
      />
      <ReactionPickerButton
        onSelect={(choice) => {
          actions.react(reactionEmojiOf(choice));
        }}
      />
      <RelationButton
        label={t("client:status.action.bookmark")}
        toggle={actions.bookmark}
        tone="accent"
        icon={(on) => <BookmarkIcon filled={on} />}
      />
      <MenuTrigger onOpenChange={setIsMenuOpen}>
        <IconButton variant="subtle" size="toolbar" label={t("client:status.action.more")}>
          <MoreIcon />
        </IconButton>
        <Menu aria-label={t("client:status.action.more")} placement="bottom end">
          <MenuItem onAction={actions.copyLink}>{t("client:status.action.copy_link")}</MenuItem>
          {actions.canEdit && (
            <MenuItem
              onAction={() => {
                setIsEditing(true);
              }}
            >
              {t("client:status.action.edit")}
            </MenuItem>
          )}
          {actions.revokeQuote !== null && (
            <MenuItem onAction={actions.revokeQuote}>{t("client:status.quote.revoke.action")}</MenuItem>
          )}
          {actions.quotePolicy !== null && (
            <MenuItem
              onAction={() => {
                setIsChoosingQuotePolicy(true);
              }}
            >
              {t("client:status.quote_policy.action")}
            </MenuItem>
          )}
          {actions.quotePolicy !== null && (
            <MenuItem href={actions.quotePolicy.quotesPath}>{t("client:status.quotes.action")}</MenuItem>
          )}
          {actions.pin !== null && (
            <MenuItem onAction={actions.pin.toggle}>
              {actions.pin.isPinned ? t("client:status.unpin.action") : t("client:status.pin.action")}
            </MenuItem>
          )}
          {remove !== null && (
            <MenuItem
              intent="danger"
              onAction={() => {
                setIsConfirmingDelete(true);
              }}
            >
              {t("client:status.action.delete")}
            </MenuItem>
          )}
          {author !== null && <MenuSeparator />}
          {/* The items keep their ids when their action flips: React Aria's collections cannot change an item's id
              while the menu is still mounted while it closes. */}
          {author?.follow != null && (
            <MenuItem id="follow" isDisabled={author.follow.isDisabled} onAction={author.follow.run}>
              {author.follow.label}
            </MenuItem>
          )}
          {author?.directMessage != null && (
            <MenuItem id="direct-message" onAction={author.directMessage.run}>
              {author.directMessage.label}
            </MenuItem>
          )}
          {author?.mute != null && (
            <MenuItem id="mute" isDisabled={author.mute.isDisabled} onAction={author.mute.run}>
              {author.mute.label}
            </MenuItem>
          )}
          {author?.block != null && (
            <MenuItem
              id="block"
              intent={author.block.isUndo ? "neutral" : "danger"}
              isDisabled={author.block.isDisabled}
              onAction={author.block.run}
            >
              {author.block.label}
            </MenuItem>
          )}
          {author?.failure != null && (
            <MenuItem id="relationship-retry" onAction={author.failure.retry}>
              {author.failure.label}
            </MenuItem>
          )}
          {actions.canReport && (
            <MenuItem
              onAction={() => {
                setIsReporting(true);
              }}
            >
              {t("client:report.open")}
            </MenuItem>
          )}
          {actions.moderation !== null && <MenuSeparator />}
          {actions.moderation !== null && (
            <MenuItem href={actions.moderation.accountHref}>{t("client:status.moderation.account")}</MenuItem>
          )}
          {actions.moderation !== null && (
            <MenuItem href={actions.moderation.postHref}>{t("client:status.moderation.post")}</MenuItem>
          )}
        </Menu>
      </MenuTrigger>
      {note !== undefined && (
        <Hint as="span" id={note}>
          {t("client:status.federation.stays_local")}
        </Hint>
      )}
      {actions.canReport && (
        <ReportDialog account={post.account} status={post} isOpen={isReporting} onOpenChange={setIsReporting} />
      )}
      {author !== null && <RelationshipConfirmDialog {...author.confirmation} />}
      {actions.canEdit && <EditStatusDialog status={post} isOpen={isEditing} onOpenChange={setIsEditing} />}
      {actions.quotePolicy !== null && (
        <QuotePolicyDialog
          value={actions.quotePolicy.value}
          onChange={actions.quotePolicy.set}
          isPending={actions.quotePolicy.isPending}
          isOpen={isChoosingQuotePolicy}
          onOpenChange={setIsChoosingQuotePolicy}
        />
      )}
      {remove !== null && (
        <Dialog
          role="alertdialog"
          closeLabel={null}
          size="sm"
          title={t("client:status.delete.title")}
          isOpen={isConfirmingDelete}
          onOpenChange={setIsConfirmingDelete}
        >
          {({ close }) => (
            <Stack gap="4">
              <p>{t("client:status.delete.description")}</p>
              <DialogActions>
                {/* The least destructive choice has the focus first (APG, alert dialog pattern). */}
                <Button autoFocus onPress={close}>
                  {t("client:status.delete.cancel")}
                </Button>
                <Button
                  variant="danger"
                  isPending={actions.isDeleting}
                  onPress={() => {
                    void remove().then((isDeleted) => {
                      if (!isDeleted) return;
                      close();
                      onDeleted?.();
                    });
                  }}
                >
                  {t("client:status.delete.confirm")}
                </Button>
              </DialogActions>
            </Stack>
          )}
        </Dialog>
      )}
    </>
  );
}

type RelationButtonProps = {
  label: string;
  toggle: RelationToggle;
  tone: NonNullable<IconToggleButtonProps["tone"]>;
  icon: (on: boolean) => ReactNode;
  /** The id of another description of the toggle: the note that the action does not reach the post's server. */
  describedBy?: string | undefined;
  /** Turning it on is a celebrated moment (web-client §21.5): only the favourite. */
  celebrate?: boolean;
};

function RelationButton({ label, toggle, tone, icon, describedBy, celebrate = false }: RelationButtonProps) {
  const t = useT();
  const intl = useIntl();
  // The count it shows is part of its name (WCAG 2.5.3), formatted as ActionCount shows it.
  const countedLabel =
    toggle.count !== null && toggle.count > 0
      ? t("client:status.action.counted", {
          action: label,
          count: intl.formatNumber(toggle.count, { notation: "compact" }),
        })
      : undefined;
  return (
    <IconToggleButton
      label={label}
      countedLabel={countedLabel}
      {...(describedBy !== undefined && { "aria-describedby": describedBy })}
      tone={tone}
      celebrate={celebrate}
      isSelected={toggle.isSelected}
      isDisabled={toggle.isDisabled}
      onChange={toggle.onChange}
    >
      {icon(toggle.isSelected)}
      {toggle.count !== null && <ActionCount value={toggle.count} />}
    </IconToggleButton>
  );
}
