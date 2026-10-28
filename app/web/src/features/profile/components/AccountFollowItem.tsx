// SPDX-License-Identifier: AGPL-3.0-only
// An account in a list that offers to follow it (the suggestions, the account search): the account row as a link to
// its profile, with the profile's follow button beside it (below it on narrow rows) and without the menu, which the
// profile keeps (FR-301).
import { AccountItem } from "@my/ui";
import type { ReactNode } from "react";
import { handleOf } from "../../../lib/handle";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import type { Account } from "../api/accounts";
import { useRelationship } from "../hooks/useRelationship";
import { nameOf } from "../hooks/useProfile";
import { RelationshipActions } from "./RelationshipActions";

export type AccountFollowItemProps = {
  account: Account;
  /** Shown before the follow button, such as a badge that says why the account is suggested. */
  badge?: ReactNode;
};

export function AccountFollowItem({ account, badge }: AccountFollowItemProps) {
  const relationship = useRelationship(account);
  const name = nameOf(account);
  return (
    <AccountItem
      name={name}
      handle={handleOf(account.acct)}
      avatar={imageSources(account.avatar)}
      avatarSeed={account.id}
      href={profilePath(account.acct)}
      layout="padded"
      trailing={
        <>
          {badge}
          <RelationshipActions account={account} name={name} state={relationship} showMenu={false} />
        </>
      }
      trailingPlacement="below"
    />
  );
}
