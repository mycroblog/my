// SPDX-License-Identifier: AGPL-3.0-only
// Every moderation action on an account (screens/moderation-account-actions.md) and one action's page
// (screens/moderation-account-action.md), in the list → detail form of the admin screens (web-client/14-admin.md §14):
// a paged RecordTable of the actions, newest first, with the kind linking to the action's page, its state, when it
// was issued and the lift of one in effect; the action's page has every fact of it and its lift. A lift asks first,
// with an optional reason, in the same dialog everywhere (LiftActionDialog), and what it did stays on the page in a
// StatusMessage. The review page shows the newest page of the same actions with their facts (ActionFacts).
import {
  Button,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Link,
  ReasonField,
  RegionError,
  Section,
  Spinner,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useSuspenseQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import type { AdminAccountDetail } from "../api/accounts";
import { type AccountAction, accountActionQueryOptions, accountActionTextLimit } from "../api/reports";
import { type LiftAccountAction, useAdminAccount, useLiftAccountAction } from "../hooks/useAdminAccounts";
import {
  accountActionKindLabels,
  accountActionStateLabels,
  handleOf,
  isolate,
  reportCategoryLabels,
  useTargetActions,
} from "../hooks/useReports";
import { AccountNav, AccountOfPage } from "./AccountNav";
import { DateTime, type RecordColumns, RecordTable, RequirePermission, useFocusAfterChange } from "./parts";
import { AssigneeName } from "./reportParts";

/** A ban is never lifted (the API refuses it); it is undone only by overturning its appeal. */
function isLiftable(action: AccountAction): boolean {
  return action.state === "active" && action.kind !== "ban";
}

/** The address of action `actionId`'s page, on account `accountId`. */
export function accountActionHref(accountId: string, actionId: string): string {
  return `/moderation/accounts/${encodeURIComponent(accountId)}/actions/${encodeURIComponent(actionId)}`;
}

/**
 * The facts of an action: its state and dates, the category of its reason, who issued it, whether a local account
 * was told, its posts and its report, then the texts of the moderators.
 */
export function ActionFacts({ action, local }: { action: AccountAction; local: boolean }) {
  const t = useT();
  return (
    <>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.account_actions.state")}>
          <Text {...(action.state === "active" && { weight: "semibold" as const })}>
            {t(accountActionStateLabels[action.state])}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account_actions.issued_at")}>
          <DateTime value={action.issued_at} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account_actions.expires_at")}>
          {action.expires_at === undefined ? (
            t("client:admin.account_actions.duration.indefinite")
          ) : (
            <DateTime value={action.expires_at} />
          )}
        </DescriptionItem>
        {action.lifted_at !== undefined && (
          <DescriptionItem term={t("client:admin.account.actions.lifted_at")}>
            <DateTime value={action.lifted_at} />
          </DescriptionItem>
        )}
        <DescriptionItem term={t("client:admin.account_actions.category")}>
          {t(reportCategoryLabels[action.reason_category])}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account.actions.issuer")}>
          {action.issued_by_account_id === undefined ? (
            t("client:admin.account.actions.issuer.cli")
          ) : (
            <AssigneeName id={action.issued_by_account_id} />
          )}
        </DescriptionItem>
        {local && (
          <DescriptionItem term={t("client:admin.account.actions.notified")}>
            {action.notify_target
              ? t("client:admin.account.actions.notified.yes")
              : t("client:admin.account.actions.notified.no")}
          </DescriptionItem>
        )}
        {action.status_ids !== undefined && (
          <DescriptionItem term={t("client:admin.report.posts")}>
            <Text numeric>{t("client:admin.report.posts.count", { count: action.status_ids.length })}</Text>
          </DescriptionItem>
        )}
        {action.report_id !== undefined && (
          <DescriptionItem term={t("client:admin.account_actions.report")}>
            <Link href={`/moderation/reports/${encodeURIComponent(action.report_id)}`} variant="inline">
              {t("client:admin.account_actions.report.other")}
            </Link>
          </DescriptionItem>
        )}
      </DescriptionList>
      {(action.public_reason !== undefined ||
        action.private_note !== undefined ||
        action.lift_reason !== undefined) && (
        <DescriptionList>
          {action.public_reason !== undefined && (
            <DescriptionItem term={t("client:admin.account_actions.public_reason")}>
              {/* Written by a moderator, in any language. */}
              <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
                {action.public_reason}
              </Text>
            </DescriptionItem>
          )}
          {action.private_note !== undefined && (
            <DescriptionItem term={t("client:admin.account_actions.private_note")}>
              <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
                {action.private_note}
              </Text>
            </DescriptionItem>
          )}
          {action.lift_reason !== undefined && (
            <DescriptionItem term={t("client:admin.account.actions.lift_reason")}>
              <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
                {action.lift_reason}
              </Text>
            </DescriptionItem>
          )}
        </DescriptionList>
      )}
    </>
  );
}

/** The lift of an action in effect, with its kind and date in its accessible name: rows of one kind tell apart. */
export function LiftButton({ action, lift }: { action: AccountAction; lift: LiftAccountAction }) {
  const t = useT();
  const intl = useIntl();
  if (!isLiftable(action)) return null;
  return (
    <Button
      size="sm"
      isPending={lift.isPending && lift.confirmation?.action.id === action.id}
      aria-label={t("client:admin.account.actions.lift_action", {
        kind: t(accountActionKindLabels[action.kind]),
        date: intl.formatDate(action.issued_at, { dateStyle: "medium", timeStyle: "short" }),
      })}
      onPress={() => {
        lift.open(action);
      }}
    >
      {t("client:admin.account.actions.lift")}
    </Button>
  );
}

/** The confirmation of a lift, with the optional reason, which takes focus when it opens; a failure stays in it. */
export function LiftActionDialog({ lift, handle }: { lift: LiftAccountAction; handle: string }) {
  const t = useT();
  const { confirmation } = lift;
  return (
    <ConfirmDialog
      isOpen={lift.isOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen) lift.cancel();
      }}
      title={t("client:admin.account.actions.confirm_lift.title", {
        kind: confirmation === null ? "" : t(accountActionKindLabels[confirmation.action.kind]),
        handle,
      })}
      description={t("client:admin.account.actions.confirm_lift.description")}
      confirmLabel={t("client:admin.account.actions.lift")}
      cancelLabel={t("client:admin.account_actions.confirm.cancel")}
      tone="primary"
      onConfirm={() => {
        lift.confirm();
      }}
      isPending={lift.isPending}
      {...(lift.error === undefined
        ? {}
        : { errorMessage: t("client:admin.account.actions.lift_failed", { reason: lift.error }) })}
    >
      <ReasonField
        label={t("client:admin.account.actions.lift_reason")}
        description={t("client:admin.account.actions.lift_reason.description")}
        value={confirmation?.reason ?? ""}
        onChange={lift.changeReason}
        characterLimit={accountActionTextLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.account_actions.characters_over", { count: -remaining })
            : t("client:admin.account_actions.characters_left", { count: remaining })
        }
        errorMessage={t("client:admin.account_actions.too_long", { max: accountActionTextLimit })}
      />
    </ConfirmDialog>
  );
}

/** What a lift did, in words. */
function useLiftedMessage(): (action: AccountAction) => string {
  const t = useT();
  return (action) => t("client:admin.account.actions.lifted", { kind: t(accountActionKindLabels[action.kind]) });
}

export function AccountActionsView({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_accounts">
      <AccountActionsPage id={id} />
    </RequirePermission>
  );
}

function AccountActionsPage({ id }: { id: string }) {
  const account = useAdminAccount(id);
  return (
    <Stack gap="6">
      <AccountNav account={account} current="actions" />
      <AccountOfPage account={account} />
      <ActionTable account={account} />
    </Stack>
  );
}

function ActionTable({ account }: { account: AdminAccountDetail }) {
  const t = useT();
  const listRef = useRef<HTMLDivElement>(null);
  const actions = useTargetActions(account.id);
  const [lifted, setLifted] = useState<string | undefined>(undefined);
  const liftedMessage = useLiftedMessage();
  // The row stays, its lift goes away: focus moves to the row's header.
  const keepFocus = useFocusHeadingAfterLift(listRef);
  const lift = useLiftAccountAction(account.id, (action) => {
    keepFocus(action.id);
    setLifted(liftedMessage(action));
  });
  const columns: RecordColumns<AccountAction> = [
    {
      id: "kind",
      header: t("client:admin.account.actions.column.kind"),
      cell: (action) => (
        <Link href={accountActionHref(account.id, action.id)} variant="standalone">
          {t(accountActionKindLabels[action.kind])}
        </Link>
      ),
    },
    {
      id: "state",
      header: t("client:admin.account_actions.state"),
      cell: (action) => (
        <Text {...(action.state === "active" && { weight: "semibold" as const })}>
          {t(accountActionStateLabels[action.state])}
        </Text>
      ),
    },
    {
      id: "issued_at",
      header: t("client:admin.account_actions.issued_at"),
      cell: (action) => <DateTime value={action.issued_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (action) => <LiftButton action={action} lift={lift} />,
    },
  ];
  return (
    <>
      <StatusMessage tone="success">{lifted}</StatusMessage>
      {actions.isLoading ? (
        <Spinner size="sm" label={t("client:admin.report.history.loading")} />
      ) : actions.items.length === 0 && actions.error !== undefined ? (
        <RegionError heading={t("client:admin.report.history.failed")} headingLevel={2} reason={actions.error} />
      ) : (
        <RecordTable
          list={actions}
          caption={t("client:admin.account.actions.title")}
          columns={columns}
          getId={(action) => action.id}
          headingLevel={2}
          listRef={listRef}
          empty={<EmptyState headingLevel={2} heading={t("client:admin.report.history.empty")} />}
          moreError={
            actions.error !== undefined && (
              <RegionError heading={t("client:admin.report.history.failed")} reason={actions.error} />
            )
          }
        />
      )}
      <LiftActionDialog lift={lift} handle={isolate(handleOf(account.account))} />
    </>
  );
}

export function AccountActionDetailView({ id, actionId }: { id: string; actionId: string }) {
  return (
    <RequirePermission permission="manage_accounts">
      <AccountActionPage id={id} actionId={actionId} />
    </RequirePermission>
  );
}

function AccountActionPage({ id, actionId }: { id: string; actionId: string }) {
  const t = useT();
  const api = useApi();
  const account = useAdminAccount(id);
  const action = useSuspenseQuery(accountActionQueryOptions(api, actionId)).data;
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [lifted, setLifted] = useState<string | undefined>(undefined);
  const liftedMessage = useLiftedMessage();
  // The button goes away with the lift: focus moves to the region's heading.
  const focusHeading = useFocusAfterChange(headingRef, action.state);
  const lift = useLiftAccountAction(account.id, (liftedAction) => {
    focusHeading();
    setLifted(liftedMessage(liftedAction));
  });
  return (
    <Stack gap="6">
      <AccountOfPage account={account} />
      <Section surface="card" title={t(accountActionKindLabels[action.kind])} headingRef={headingRef}>
        <StatusMessage tone="success">{lifted}</StatusMessage>
        <ActionFacts action={action} local={account.local} />
        {isLiftable(action) && (
          <Stack align="start">
            <Button
              onPress={() => {
                lift.open(action);
              }}
              isPending={lift.isPending}
            >
              {t("client:admin.account.actions.lift")}
            </Button>
          </Stack>
        )}
      </Section>
      <LiftActionDialog lift={lift} handle={isolate(handleOf(account.account))} />
    </Stack>
  );
}
