// SPDX-License-Identifier: AGPL-3.0-only

// The follow of an account's row, wired to the reader's relationship (FR-301): app-components' AccountFollow, fed by
// the relationship in the cache and following through the server. Every list of accounts uses it: the search's and
// the suggested people, an account's follows, a notification's follow.

import { AccountFollow } from "@my/app-components";
import { useChangeRelationship, useRelationship, type Account } from "@my/app-data";

/** The follow of the row of `account`. */
export function FollowOf({ account }: { account: Account }) {
  const relationship = useRelationship(account.id);
  const change = useChangeRelationship();
  return <AccountFollow account={account} relationship={relationship} onFollow={() => change(account.id, "follow")} />;
}
