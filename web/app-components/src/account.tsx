// SPDX-License-Identifier: AGPL-3.0-only

// An account as the screens name it, from the API's Account (status.ts's PostAccount): the name it goes by with its
// custom emoji, its handle, and its row (who followed the reader, who asks to), whose name leads to its profile.

import { AccountRow as Row, Avatar } from "@my/design-system";
import type { ReactNode } from "react";
import { accountPath } from "./paths";
import { EmojiText } from "./post-content";
import type { PostAccount } from "./status";

/** The account's handle, `@user@domain`, whether it is local or remote; its domain alone without a confirmed one. */
export const handleOf = (account: PostAccount) =>
  account.username === "" ? account.domain : `@${account.username}@${account.domain}`;

/** The name an account goes by: its display name, else its username. */
export const nameOf = (account: PostAccount) => account.display_name || account.username || account.domain;

/** The account's display name with its custom emoji. */
export function AccountName({ account }: { account: PostAccount }) {
  return <EmojiText text={nameOf(account)} emojis={account.emojis} />;
}

/** An account on a row of its own, what the reader may do with it at the end. */
export function AccountLine({ account, end }: { account: PostAccount; end?: ReactNode }) {
  return (
    <Row
      avatar={<Avatar name={nameOf(account)} src={account.avatar?.small.url} size="post" decorative />}
      name={<AccountName account={account} />}
      handle={handleOf(account)}
      href={accountPath(account)}
      end={end}
    />
  );
}
