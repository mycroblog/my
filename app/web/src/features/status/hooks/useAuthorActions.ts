// SPDX-License-Identifier: AGPL-3.0-only
// What the viewer can do about the author of another account's post, from the post's menu (FR-301 to FR-303): follow, a
// direct message, mute and block, with the rules, the changes and the questions of a profile (features/relationship).
// A post has no relationship on it, so it is read when the menu opens, and a list of posts reads none until then. Until it is known, the items that depend on it stay in place, disabled, in the
// words of an account with no relationship; a failure takes their place with one item that reads it again. The menu
// closes on a choice, so each result shows in a toast (web-client/16-look-states-and-wording.md G24).
import { useState } from "react";
import { isolate } from "../../../lib/handle";
import { type Translate, useT } from "../../../lib/i18n";
import { useDirectMessage } from "../../compose";
import {
  type ConfirmableRelationshipChange,
  type RelationshipConfirmDialogProps,
  type RelationshipState,
  useRelationship,
} from "../../relationship";
import { useViewer } from "../../session";
import type { Status } from "../api/status";

/** One item of the menu. */
type AuthorAction = { label: string; isDisabled: boolean; run: () => void };

export type AuthorActions = {
  /** Null where no follow can be started or undone: the account is blocked, or the instance follows no other server. */
  follow: AuthorAction | null;
  /** Null when the account is blocked or cannot be reached. */
  directMessage: AuthorAction | null;
  mute: AuthorAction | null;
  /** `isUndo` when the item unblocks. */
  block: (AuthorAction & { isUndo: boolean }) | null;
  /** The item that reads a failed relationship again, in place of those that depend on it. */
  failure: { label: string; retry: () => void } | null;
  /** The question before a change that asks first. */
  confirmation: RelationshipConfirmDialogProps;
};

/** The state of a relationship the menu acts on. */
type Known = Extract<RelationshipState, { kind: "other" }>;

/** The follow item for where the follow stands, or null when there is none to offer (as the profile's button). */
function followAction(
  t: Translate,
  account: Status["account"],
  state: Known,
  handle: string,
  confirm: (kind: ConfirmableRelationshipChange) => void,
): AuthorAction | null {
  const isDisabled = state.pending !== null;
  switch (state.follow) {
    case "none": {
      if (!state.canFollow) return null;
      const label = account.locked
        ? t("client:status.author.request", { handle })
        : state.relationship.followed_by
          ? t("client:status.author.follow_back", { handle })
          : t("client:status.author.follow", { handle });
      return {
        label,
        isDisabled,
        run: () => {
          state.change("follow");
        },
      };
    }
    case "requested":
      return {
        label: t("client:status.author.withdraw", { handle }),
        isDisabled,
        run: () => {
          confirm("withdraw");
        },
      };
    case "following":
      return {
        label: t("client:status.author.unfollow", { handle }),
        isDisabled,
        run: () => {
          confirm("unfollow");
        },
      };
    case "blocking":
      return null;
  }
}

/**
 * The actions on the author of `post`, or null on the viewer's own post and while nobody is signed in. `isMenuOpen` is
 * whether the post's menu is open: the relationship is read then, and a failed one again each time it opens.
 */
export function useAuthorActions(post: Status, isMenuOpen: boolean): AuthorActions | null {
  const t = useT();
  const viewer = useViewer();
  const account = post.account;
  const directMessage = useDirectMessage();
  const state = useRelationship(account, { enabled: isMenuOpen, result: "toast" });
  // The kind stays while the dialog closes, so that it shows what it asked until it is gone.
  const [dialog, setDialog] = useState<{ isOpen: boolean; kind: ConfirmableRelationshipChange }>({
    isOpen: false,
    kind: "unfollow",
  });
  if (viewer.status !== "signed-in" || viewer.account.id === account.id || directMessage === null) return null;

  // The short handle: a menu item has no room for a long domain, and the post above it shows the whole handle.
  const handle = isolate(`@${account.username}`);
  const confirm = (kind: ConfirmableRelationshipChange) => {
    setDialog({ isOpen: true, kind });
  };
  const close = () => {
    setDialog({ ...dialog, isOpen: false });
  };
  const dm: AuthorAction = {
    label: t("client:status.author.direct_message", { handle }),
    isDisabled: false,
    run: () => {
      directMessage(account.acct);
    },
  };
  const confirmation: RelationshipConfirmDialogProps = {
    isOpen: dialog.isOpen,
    kind: dialog.kind,
    name: account.display_name || account.username,
    onCancel: close,
    onConfirm: (action) => {
      close();
      if (state.kind === "other") state.change(action);
    },
  };
  const noop = () => {
    // A disabled item is not chosen.
  };

  switch (state.kind) {
    case "other": {
      const isDisabled = state.pending !== null;
      const { muting, blocking } = state.relationship;
      return {
        follow: followAction(t, account, state, handle, confirm),
        directMessage: blocking ? null : dm,
        mute: {
          label: muting ? t("client:status.author.unmute", { handle }) : t("client:status.author.mute", { handle }),
          isDisabled,
          run: () => {
            state.change(muting ? "unmute" : "mute");
          },
        },
        block: {
          label: blocking ? t("client:status.author.unblock", { handle }) : t("client:status.author.block", { handle }),
          isDisabled,
          isUndo: blocking === true,
          run: () => {
            confirm(blocking ? "unblock" : "block");
          },
        },
        failure: null,
        confirmation,
      };
    }
    case "error":
      return {
        follow: null,
        directMessage: dm,
        mute: null,
        block: null,
        failure: {
          label: t("client:status.author.relationship_failed", { handle, reason: state.message }),
          retry: state.retry,
        },
        confirmation,
      };
    case "unavailable":
      return { follow: null, directMessage: null, mute: null, block: null, failure: null, confirmation };
    default:
      // Loading, or the menu has not opened yet.
      return {
        follow: { label: t("client:status.author.follow", { handle }), isDisabled: true, run: noop },
        directMessage: dm,
        mute: { label: t("client:status.author.mute", { handle }), isDisabled: true, run: noop },
        block: { label: t("client:status.author.block", { handle }), isDisabled: true, isUndo: false, run: noop },
        failure: null,
        confirmation,
      };
  }
}
