// SPDX-License-Identifier: AGPL-3.0-only

// The follow of an account's row, wired to the reader's relationship (FR-301): app-components' AccountFollow, fed by
// the relationship in the cache and following through the server. Every list of accounts uses it: the search's and
// the suggested people, an account's follows, a notification's follow. A follow that failed is told at the bottom of
// the page as a post's actions tell theirs (usePostNotice).

import { AccountFollow } from "@my/app-components";
import { useChangeRelationship, useRelationship, type Account } from "@my/app-data";
import { useLocale } from "@my/design-system";
import { stringsIn } from "./locale";
import { usePostNotice } from "./post-actions";

/** The follow of the row of `account`. */
export function FollowOf({ account }: { account: Account }) {
  const { t } = stringsIn(useLocale().locale);
  const relationship = useRelationship(account.id);
  const change = useChangeRelationship();
  const failed = usePostNotice();
  const follow = (): Promise<void> =>
    change(account.id, "follow").catch((e: unknown) => {
      failed(e, t("profile.action_failed", { action: "follow" }), () => {
        void follow();
      });
    });
  return <AccountFollow account={account} relationship={relationship} onFollow={follow} />;
}
