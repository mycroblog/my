// SPDX-License-Identifier: AGPL-3.0-only

// An account as the screens name it, from the API's Account (status.ts's PostAccount): the name it goes by with its
// custom emoji, its handle, its row (who followed the reader, who asks to), whose name leads to its profile, and the
// follow at a row's end, from the reader's relationship with it.

import { AccountRow as Row, Avatar, Button, StateText, useLocale } from "@my/design-system";
import { useRef, type ReactNode } from "react";
import { stringsIn } from "./locale";
import { accountPath } from "./paths";
import { EmojiText } from "./post-content";
import type { PostAccount } from "./status";

/** The account's handle, `@user@domain`, whether it is local or remote; its domain alone without a confirmed one. */
export const handleOf = (account: PostAccount) =>
  account.username === "" ? account.domain : `@${account.username}@${account.domain}`;

/**
 * The handle a sentence names an account by: `@user` for a local account and `@user@domain` for a remote one, so that
 * two accounts of one name on two servers read differently; its domain alone without a confirmed username. A
 * byline's or a profile's head shows the handle in a part of its own (`handleOf`); every message that takes an
 * account's handle takes this.
 */
export const sentenceHandleOf = (account: { username: string; acct: string; domain?: string }) =>
  account.username === "" ? (account.domain ?? account.acct) : `@${account.acct || account.username}`;

/** The name an account goes by: its display name, else its username. */
export const nameOf = (account: PostAccount) => account.display_name || account.username || account.domain;

/** The account's display name with its custom emoji. */
export function AccountName({ account }: { account: PostAccount }) {
  return <EmojiText text={nameOf(account)} emojis={account.emojis} />;
}

/** What a row's follow reads of the reader's relationship with its account. */
export type FollowState = {
  readonly following?: boolean | undefined;
  readonly requested?: boolean | undefined;
  readonly followed_by?: boolean | undefined;
  readonly blocking?: boolean | undefined;
};

/**
 * The follow of an account's row (a search's, a list's of follows, a notification's): the primary button, Follow,
 * Follow back for an account that follows the reader, Request to follow for a locked one, until the reader follows it
 * or asked to, then what is so; nothing until the relationship is known, nor for an account the reader blocks. A press
 * while `onFollow` runs does nothing; the button never shows itself busy, which React Aria would announce by an id the
 * answer takes away.
 */
export function AccountFollow({
  account,
  relationship,
  onFollow,
}: {
  account: PostAccount & { readonly locked?: boolean | undefined };
  relationship: FollowState | undefined;
  onFollow: () => Promise<unknown>;
}) {
  const { t } = stringsIn(useLocale().locale);
  const sending = useRef(false);
  if (relationship === undefined || relationship.blocking === true) return null;
  if (relationship.following === true) return <StateText>{t("account.following")}</StateText>;
  if (relationship.requested === true) return <StateText>{t("account.requested")}</StateText>;
  return (
    <Button
      size="timeline"
      kind="primary"
      onPress={() => {
        if (sending.current) return;
        sending.current = true;
        onFollow()
          .catch(() => undefined)
          .finally(() => {
            sending.current = false;
          });
      }}
    >
      {t(
        account.locked === true
          ? "account.request_follow"
          : relationship.followed_by === true
            ? "account.follow_back"
            : "account.follow",
      )}
    </Button>
  );
}

/**
 * An account on a row of its own, a line under its name (`note`: what a mute of it covers), what the reader may do
 * with it at the end; `heading` makes its name the heading of the screen it heads.
 */
export function AccountLine({
  account,
  note,
  end,
  heading = false,
}: {
  account: PostAccount;
  note?: ReactNode;
  end?: ReactNode;
  heading?: boolean;
}) {
  return (
    <Row
      avatar={<Avatar name={nameOf(account)} src={account.avatar?.small.url} size="post" decorative />}
      name={<AccountName account={account} />}
      handle={handleOf(account)}
      href={accountPath(account)}
      note={note}
      end={end}
      heading={heading}
    />
  );
}
