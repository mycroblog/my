// SPDX-License-Identifier: AGPL-3.0-only
// The note on the profile of an account that moved to another account (FR-009, ADR-0089): who it moved to, with a link to
// the new account's profile. On the user's own profile the note speaks to the user, and a link leads to the page that
// shows the move and cancels it. Nothing is offered to the moved account itself besides: the relationship actions of
// a profile that is the viewer's own are the ones of editing it.
import { Callout, Link } from "@my/ui";
import { useIntl } from "react-intl";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { accountMovePath, profilePath } from "../../../lib/paths";
import type { Account } from "../api/accounts";
import { isolate } from "../hooks/useProfile";

export type MovedNoteProps = {
  /** The name of the account that moved. */
  name: string;
  /** The account it moved to. */
  target: Account;
  /** Whether the profile is the viewer's own. */
  isSelf: boolean;
};

export function MovedNote({ name, target, isSelf }: MovedNoteProps) {
  const t = useT();
  const intl = useIntl();
  const link = (
    <Link key="target" href={profilePath(target.acct)} variant="standalone">
      <span dir="ltr">{handleOf(target.acct)}</span>
    </Link>
  );
  return (
    <Callout>
      <p>
        {isSelf
          ? intl.formatMessage({ id: "client:account_move.profile.moved_self" }, { target: link })
          : intl.formatMessage({ id: "client:account_move.profile.moved" }, { name: isolate(name), target: link })}
      </p>
      {isSelf && (
        <Link href={accountMovePath} variant="standalone">
          {t("client:account_move.profile.manage")}
        </Link>
      )}
    </Callout>
  );
}
