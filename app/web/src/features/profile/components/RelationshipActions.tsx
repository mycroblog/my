// SPDX-License-Identifier: AGPL-3.0-only
// The buttons of a profile that change the viewer's relationship with its account (FR-301 to FR-303):
// the follow button (@my/ui FollowButton), whose look and label are where the follow stands (Follow, the filled main
// action; Requested, Following, Blocked, quiet with their icons), and that keeps its place while the relationship
// loads, and
// a bell toggle button (WAI-ARIA APG, button pattern, `aria-pressed`) that turns on and off the notifications of the
// account's new posts (ADR-0086) at once; on the profile of an account of another server that the viewer does not
// follow, it is shown turned off and cannot be pressed, with the line that says to follow first, and
// a menu button (WAI-ARIA APG, menu button pattern) with "Add to list" for an account the viewer follows
// (features/lists), mute, block and report (features/report). Undoing
// a follow or a request, blocking and unblocking ask first in an alert dialog (WAI-ARIA APG, alert dialog
// pattern). A follow request to an account of another server says which server it waits for, and no new follow of
// such an account is offered while the instance does not follow other servers (ADR-0006). A list of
// accounts (the notifications) can show the follow button alone, and leave mute, block and report to the
// profile.
import {
  Box,
  Cluster,
  FollowButton,
  Hint,
  IconButton,
  IconToggleButton,
  Link,
  Menu,
  MenuItem,
  MenuTrigger,
  MoreIcon,
  SubscribeIcon,
} from "@my/ui";
import { useId, useState } from "react";
import { type Translate, useT } from "../../../lib/i18n";
import { AddToListDialog } from "../../lists";
import { ReportDialog } from "../../report";
import {
  type ConfirmableRelationshipChange,
  RelationshipConfirmDialog,
  type RelationshipState,
} from "../../relationship";
import type { Account } from "../api/accounts";
import { isolate } from "../hooks/useProfile";

export type RelationshipActionsProps = {
  account: Account;
  /** The account's name in the UI. */
  name: string;
  state: RelationshipState;
  /** Whether to show the menu with mute and block next to the follow button (the default). */
  showMenu?: boolean;
  /** `sm` in a row of a list; `md` (the default) on a profile. */
  size?: "sm" | "md";
};

export function RelationshipActions({ account, name, state, showMenu = true, size = "md" }: RelationshipActionsProps) {
  const t = useT();
  switch (state.kind) {
    case "signed-out":
      return (
        <Link href={state.signInHref} variant="standalone">
          {t("client:profile.sign_in_to_follow")}
        </Link>
      );
    case "self":
      return (
        <Link href="/settings/profile" variant="standalone">
          {t("client:profile.edit")}
        </Link>
      );
    case "loading":
    case "unavailable":
      // The button's place, so that nothing moves when the relationship arrives.
      return <FollowButton status="loading" label="" size={size} />;
    case "error":
      return <Hint>{t("client:profile.relationship_failed", { reason: state.message })}</Hint>;
    case "other":
      return <OtherActions account={account} name={name} state={state} showMenu={showMenu} size={size} />;
  }
}

type OtherActionsProps = {
  account: Account;
  name: string;
  state: Extract<RelationshipState, { kind: "other" }>;
  showMenu: boolean;
  size: "sm" | "md";
};

/** The follow button's label, and the change it asks for, for where the follow stands. */
function followButtonOf(t: Translate, account: Account, state: OtherActionsProps["state"]) {
  switch (state.follow) {
    case "none": {
      const label = account.locked
        ? t("client:profile.follow.request")
        : state.relationship.followed_by
          ? t("client:profile.follow.back")
          : t("client:profile.follow");
      return { label, press: "follow" } as const;
    }
    case "requested":
      return { label: t("client:profile.follow.requested"), press: "withdraw" } as const;
    case "following":
      return { label: t("client:profile.follow.following"), press: "unfollow" } as const;
    case "blocking":
      return { label: t("client:profile.follow.blocked"), press: "unblock" } as const;
  }
}

function OtherActions({ account, name, state, showMenu, size }: OtherActionsProps) {
  const t = useT();
  // The kind stays while the dialog closes, so that it shows what it asked until it is gone.
  const [dialog, setDialog] = useState<{ isOpen: boolean; kind: ConfirmableRelationshipChange }>({
    isOpen: false,
    kind: "unfollow",
  });
  const [isReporting, setIsReporting] = useState(false);
  const [isAddingToList, setIsAddingToList] = useState(false);
  // Only an account the viewer follows, accepted, can be put in a list (ADR-0013 §11).
  const offersLists = state.follow === "following";
  const confirm = (kind: ConfirmableRelationshipChange) => {
    setDialog({ isOpen: true, kind });
  };
  const { relationship, pending, change } = state;
  const isPending = pending !== null;
  const followButton = followButtonOf(t, account, state);
  const moreLabel = t("client:profile.more", { name: isolate(name) });
  // Another server answers a follow request on its own, later (federation.md): until it accepts, the
  // request is pending, and the button's description names the server it waits for.
  const pendingId = useId();
  const pendingRemote = state.follow === "requested" && account.acct.includes("@");
  // While the instance does not follow other servers (ADR-0006), no new follow of their accounts is offered; the
  // line that says so takes the button's place.
  const offersFollow = state.follow !== "none" || state.canFollow;
  // The bell of the account's new posts (ADR-0086): only on the profile, with the menu. While a change of it is on its
  // way, it shows the state asked for.
  const subscribeId = useId();
  const showSubscribe = showMenu && state.subscribe !== "hidden";
  const followFirst = showSubscribe && state.subscribe === "follow-first";
  const subscribed =
    pending === "subscribe" ? true : pending === "unsubscribe" ? false : relationship.notify === true && !followFirst;

  return (
    <>
      <Cluster align="center" gap="2">
        {/* One button for every state, so that it keeps the focus when its label changes. */}
        {offersFollow ? (
          <FollowButton
            status={state.follow}
            label={followButton.label}
            size={size}
            isPending={isPending}
            aria-describedby={pendingRemote ? pendingId : undefined}
            onPress={() => {
              if (followButton.press === "follow") change("follow");
              else confirm(followButton.press);
            }}
          />
        ) : (
          <Hint as="span">{t("client:profile.follow.federation_restricted")}</Hint>
        )}
        {showSubscribe && (
          <IconToggleButton
            label={t("client:profile.subscribe")}
            isSelected={subscribed}
            isDisabled={followFirst}
            {...(followFirst && { "aria-describedby": subscribeId })}
            onChange={(on) => {
              change(on ? "subscribe" : "unsubscribe");
            }}
          >
            <SubscribeIcon filled={subscribed} />
          </IconToggleButton>
        )}
        {showMenu && (
          <MenuTrigger>
            <IconButton variant="ghost" size="compact" isPending={isPending} label={moreLabel}>
              <MoreIcon />
            </IconButton>
            <Menu
              aria-label={moreLabel}
              placement="bottom end"
              onAction={(key) => {
                if (key === "mute") change(relationship.muting ? "unmute" : "mute");
                if (key === "block") confirm(relationship.blocking ? "unblock" : "block");
                if (key === "report") setIsReporting(true);
                if (key === "lists") setIsAddingToList(true);
              }}
            >
              {/* The items keep their ids when their action flips: React Aria's collections cannot change an
                  item's id while the menu is still mounted while it closes. */}
              {offersLists && <MenuItem id="lists">{t("client:lists.add_to_list.open")}</MenuItem>}
              <MenuItem id="mute">
                {relationship.muting ? t("client:profile.menu.unmute") : t("client:profile.menu.mute")}
              </MenuItem>
              <MenuItem id="block" intent={relationship.blocking ? "neutral" : "danger"}>
                {relationship.blocking ? t("client:profile.menu.unblock") : t("client:profile.menu.block")}
              </MenuItem>
              <MenuItem id="report">{t("client:report.open")}</MenuItem>
            </Menu>
          </MenuTrigger>
        )}
        {showMenu && <ReportDialog account={account} isOpen={isReporting} onOpenChange={setIsReporting} />}
        {showMenu && offersLists && (
          <AddToListDialog account={account} isOpen={isAddingToList} onOpenChange={setIsAddingToList} />
        )}
        <RelationshipConfirmDialog
          isOpen={dialog.isOpen}
          kind={dialog.kind}
          name={name}
          onCancel={() => {
            setDialog({ ...dialog, isOpen: false });
          }}
          onConfirm={(action) => {
            setDialog({ ...dialog, isOpen: false });
            change(action);
          }}
        />
      </Cluster>
      {/* A line of its own in the header's wrapping row, below the picture and the buttons. */}
      {pendingRemote && (
        <Box fullRow>
          <Hint id={pendingId} wrap="anywhere">
            {t("client:profile.follow.pending_remote", { host: isolate(account.domain) })}
          </Hint>
        </Box>
      )}
      {/* The reason the bell cannot be pressed, which also describes it (a disabled button shows no tooltip). */}
      {followFirst && (
        <Box fullRow>
          <Hint id={subscribeId}>{t("client:profile.subscribe.follow_first")}</Hint>
        </Box>
      )}
    </>
  );
}
