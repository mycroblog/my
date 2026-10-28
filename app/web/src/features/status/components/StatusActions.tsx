// SPDX-License-Identifier: AGPL-3.0-only
import {
  BookmarkIcon,
  BoostIcon,
  Button,
  Dialog,
  DialogActions,
  Hint,
  Menu,
  MenuItem,
  MenuTrigger,
  MoreIcon,
  QuoteIcon,
  ReplyIcon,
  Stack,
  StarIcon,
  Text,
  ToggleButton,
  type ToggleButtonProps,
} from "@my/ui";
import { type ReactNode, useId, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { type ReactionChoice, ReactionPickerButton } from "../../emoji";
import { ReportDialog } from "../../report";
import type { ReactionEmoji, Status } from "../api/status";
import { type RelationToggle, useStatusActions } from "../hooks/useStatusActions";
import { EditStatusDialog } from "./EditStatusDialog";
import { QuotePolicyDialog } from "./QuotePolicyDialog";

export type StatusActionsProps = {
  status: Status;
  /** Called after the post is deleted, for a page that shows only this post. */
  onDeleted?: () => void;
};

/** The emoji of a reaction picked in the picker: a Unicode emoji, or a custom emoji of this server. */
function reactionEmojiOf(choice: ReactionChoice): ReactionEmoji {
  if ("unicode" in choice) return { name: choice.unicode };
  const { shortcode, url, static_url } = choice.custom;
  return { name: shortcode, shortcode, emoji: { shortcode, url, static_url } };
}

/**
 * The action bar of a post: reply, boost, quote (FR-114), favourite, the picker of emoji reactions (ADR-0052 §5), bookmark, and a menu with
 * the link, the report of another account's post (features/report), the revocation of this post's approved quote of a post of the
 * viewer (ADR-0069 §3) and, for the viewer's own posts, edit (EditStatusDialog), who may quote it (QuotePolicyDialog), the posts that
 * quote it, pin or unpin, and delete behind a confirmation (WAI-ARIA APG, alert dialog pattern). Nothing while
 * nobody is signed in. The toggles keep their names in both states and expose the state with
 * `aria-pressed` (APG, button pattern); a count is the toggle's description. On a post of another server while the
 * instance sends nothing out (ADR-0076), quoting is not offered and a note at the end of the bar, which also
 * describes the reply, boost and favourite, says that the actions do not reach that server.
 */
export function StatusActions({ status, onDeleted }: StatusActionsProps) {
  const t = useT();
  const repliesId = useId();
  const staysLocalId = useId();
  const actions = useStatusActions(status);
  const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
  const [isReporting, setIsReporting] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [isChoosingQuotePolicy, setIsChoosingQuotePolicy] = useState(false);
  if (actions === null) return null;
  const post = status.boost ?? status;
  const remove = actions.delete;
  // The note that the actions on a post of another server do not reach it (ADR-0076) describes each of them.
  const note = actions.staysLocal ? staysLocalId : undefined;
  const replyDescription = [post.replies_count > 0 ? repliesId : undefined, note]
    .filter((id) => id !== undefined)
    .join(" ");

  return (
    <>
      <Button
        variant="subtle"
        size="toolbar"
        aria-label={t("client:status.action.reply")}
        {...(replyDescription !== "" && { "aria-describedby": replyDescription })}
        onPress={actions.reply}
      >
        <ReplyIcon />
        <Count id={repliesId} value={post.replies_count} />
      </Button>
      <RelationButton
        label={t("client:status.action.boost")}
        toggle={actions.boost}
        tone="success"
        icon={(on) => <BoostIcon filled={on} />}
        describedBy={note}
      />
      {actions.quote !== null && (
        <Button
          variant="subtle"
          size="toolbar"
          aria-label={t("client:status.action.quote")}
          onPress={actions.quote.onPress}
          isDisabled={actions.quote.isDisabled}
        >
          <QuoteIcon />
        </Button>
      )}
      <RelationButton
        label={t("client:status.action.favourite")}
        toggle={actions.favourite}
        tone="warning"
        icon={(on) => <StarIcon filled={on} />}
        describedBy={note}
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
      <MenuTrigger>
        <Button variant="subtle" size="toolbar" aria-label={t("client:status.action.more")}>
          <MoreIcon />
        </Button>
        <Menu aria-label={t("client:status.action.more")} placement="bottom end">
          <MenuItem onAction={actions.copyLink}>{t("client:status.action.copy_link")}</MenuItem>
          {actions.canReport && (
            <MenuItem
              onAction={() => {
                setIsReporting(true);
              }}
            >
              {t("client:report.open")}
            </MenuItem>
          )}
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
  tone: NonNullable<ToggleButtonProps["tone"]>;
  icon: (on: boolean) => ReactNode;
  /** The id of another description of the toggle: the note that the action does not reach the post's server. */
  describedBy?: string | undefined;
};

function RelationButton({ label, toggle, tone, icon, describedBy }: RelationButtonProps) {
  const countId = useId();
  const hasCount = toggle.count !== null && toggle.count > 0;
  const description = [hasCount ? countId : undefined, describedBy].filter((id) => id !== undefined).join(" ");
  return (
    <ToggleButton
      aria-label={label}
      {...(description !== "" && { "aria-describedby": description })}
      tone={tone}
      isSelected={toggle.isSelected}
      isDisabled={toggle.isDisabled}
      onChange={toggle.onChange}
    >
      {icon(toggle.isSelected)}
      {toggle.count !== null && <Count id={countId} value={toggle.count} />}
    </ToggleButton>
  );
}

/** A count next to an icon, compact ("1.2K") in the page's locale; nothing for zero. */
function Count({ value, id }: { value: number; id?: string }) {
  const intl = useIntl();
  if (value === 0) return null;
  return (
    <Text id={id} numeric>
      {intl.formatNumber(value, { notation: "compact" })}
    </Text>
  );
}
