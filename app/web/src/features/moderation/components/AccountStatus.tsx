// SPDX-License-Identifier: AGPL-3.0-only
// The account status page below its heading (FR-715, FR-707): the moderation actions in effect on the user's
// account and the past ones, each with what it does and its state's badge, how long it lasts (in the words of the
// server's status page, web-client/screens/account-status.md), its reason, when it was taken and when it ends, the
// moderators' explanation, the posts it acted on, and its appeal with its state's badge. An action that can still be appealed has a
// button that opens the appeal in an alert dialog (ConfirmDialog: an action is appealed once, and the appeal
// cannot be changed); one appealed already shows the appeal's state instead, and one whose window closed
// says so. Dates are in the user's locale and time zone (react-intl's, from the preferences).
import {
  AppealIcon,
  Box,
  Button,
  CheckIcon,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  Section,
  EmptyState,
  Heading,
  Hint,
  ReasonField,
  Spinner,
  Stack,
  StatusBadge,
  type StatusBadgeProps,
  StatusMessage,
  Text,
  WaitingIcon,
  WarningIcon,
} from "@my/ui";
import { type ReactNode, useId } from "react";
import { useIntl } from "react-intl";
import { useNow } from "../../../lib/clock";
import { type ClientMessageKey, type Translate, useT } from "../../../lib/i18n";
import { StatusCard } from "../../status";
import {
  appealTextLimit,
  type ModerationAction,
  type ModerationActionKind,
  type ModerationActionState,
  type ModerationAppeal,
  type ModerationAppealState,
} from "../api/actions";
import {
  type ActionList,
  type AppealForm,
  appealHeadingId,
  useActionStatuses,
  useActiveActions,
  useAppealForm,
  usePastActions,
} from "../hooks/useAccountStatus";

const stateLabels: Readonly<Record<ModerationActionState, ClientMessageKey>> = {
  active: "client:account_status.state.active",
  expired: "client:account_status.state.expired",
  lifted: "client:account_status.state.lifted",
};

const appealStateLabels: Readonly<Record<ModerationAppealState, ClientMessageKey>> = {
  pending: "client:account_status.appeal.state.pending",
  upheld: "client:account_status.appeal.state.upheld",
  overturned: "client:account_status.appeal.state.overturned",
};

type Badge = { icon: StatusBadgeProps["icon"]; tone: StatusBadgeProps["tone"] };

/** The shape of each state of an action: in effect is a warning; ended waits no more; lifted is done. */
const stateBadges: Readonly<Record<ModerationActionState, Badge>> = {
  active: { icon: <WarningIcon />, tone: "warning" },
  expired: { icon: <WaitingIcon />, tone: "neutral" },
  lifted: { icon: <CheckIcon />, tone: "success" },
};

/** The shape of each state of an appeal. */
const appealStateBadges: Readonly<Record<ModerationAppealState, Badge>> = {
  pending: { icon: <WaitingIcon />, tone: "accent" },
  upheld: { icon: <AppealIcon />, tone: "neutral" },
  overturned: { icon: <CheckIcon />, tone: "success" },
};

const dayMs = 24 * 60 * 60 * 1000;

/** The whole days left until `value` from `now`, rounded up, as the server's status page counts them. */
function daysUntil(now: number, value: string): number {
  return Math.max(0, Math.ceil((Date.parse(value) - now) / dayMs));
}

/**
 * The kinds that can last for a time; the others (a notice, the actions on posts, a ban) have no end. One of
 * these in effect without an end lasts until a moderator lifts it.
 */
const timedKinds: ReadonlySet<ModerationActionKind> = new Set([
  "force_sensitive",
  "limit",
  "restrict_posting",
  "restrict_interaction",
  "disable_login",
  "suspend",
]);

/** The format of every date and time on the page. */
const dateTimeFormat = { dateStyle: "medium", timeStyle: "short" } as const;

/** A moment in the user's locale and time zone, as a `<time>`. */
function DateTime({ value }: { value: string }) {
  const intl = useIntl();
  return <time dateTime={value}>{intl.formatDate(value, dateTimeFormat)}</time>;
}

export function AccountStatus() {
  const t = useT();
  const active = useActiveActions();
  const past = usePastActions();
  const form = useAppealForm();
  return (
    <Stack gap="8">
      <StatusMessage tone="success">{form.status}</StatusMessage>
      <Section surface="card" title={t("client:account_status.active.title")}>
        <ActionListView
          list={active}
          form={form}
          empty={
            <EmptyState
              headingLevel={3}
              heading={t("client:account_status.active.empty")}
              description={t("client:account_status.active.empty.description")}
            />
          }
        />
      </Section>
      <Section surface="card" title={t("client:account_status.past.title")}>
        <ActionListView
          list={past}
          form={form}
          empty={<EmptyState headingLevel={3} heading={t("client:account_status.past.empty")} />}
        />
      </Section>
      <AppealDialog form={form} />
    </Stack>
  );
}

/** The actions of a list, divided by lines (web-client/16-look-states-and-wording.md G3), then "show more" while there are more. */
function ActionListView({ list, form, empty }: { list: ActionList; form: AppealForm; empty: ReactNode }) {
  const t = useT();
  // The past actions are the whole list without those in effect, so a page can have none of them.
  if (list.items.length === 0 && !list.hasMore) return empty;
  return (
    <Stack gap="4">
      {list.items.length > 0 && (
        <Stack as="ul" divided>
          {list.items.map((action) => (
            <Box as="li" key={action.id} container paddingY="3">
              <ActionRow action={action} form={form} />
            </Box>
          ))}
        </Stack>
      )}
      {list.hasMore && (
        <Box>
          <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
            {t("client:account_status.more")}
          </Button>
        </Box>
      )}
    </Stack>
  );
}

function ActionRow({ action, form }: { action: ModerationAction; form: AppealForm }) {
  const t = useT();
  const now = useNow();
  const badge = stateBadges[action.state];
  // How long an action in effect lasts, first, as the server's status page says it.
  const lasts =
    action.state !== "active"
      ? undefined
      : action.expires_at !== undefined
        ? t("client:account_status.ends_in", { days: daysUntil(now, action.expires_at) })
        : timedKinds.has(action.kind)
          ? t("client:account_status.until_lifted")
          : undefined;
  return (
    <Stack as="article" aria-labelledby={`action-${action.id}`} gap="3">
      <Stack gap="1">
        <Cluster align="center" gap="2" rowGap="1">
          {/* The kind in the user's language, as the server names it. */}
          <Heading level={3} id={`action-${action.id}`} wrap="anywhere">
            {action.kind_label}
          </Heading>
          <StatusBadge icon={badge.icon} tone={badge.tone} label={t(stateLabels[action.state])} />
        </Cluster>
        {lasts !== undefined && <p>{lasts}</p>}
      </Stack>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:account_status.reason_category")}>
          {action.reason_category_label}
        </DescriptionItem>
        <DescriptionItem term={t("client:account_status.issued_at")}>
          <DateTime value={action.issued_at} />
        </DescriptionItem>
        {action.expires_at !== undefined ? (
          <DescriptionItem term={t("client:account_status.expires_at")}>
            <DateTime value={action.expires_at} />
          </DescriptionItem>
        ) : (
          action.state === "active" &&
          timedKinds.has(action.kind) && (
            <DescriptionItem term={t("client:account_status.expires_at")}>
              {t("client:account_status.expires_at.none")}
            </DescriptionItem>
          )
        )}
        {action.lifted_at !== undefined && (
          <DescriptionItem term={t("client:account_status.lifted_at")}>
            <DateTime value={action.lifted_at} />
          </DescriptionItem>
        )}
      </DescriptionList>
      <DescriptionList>
        <DescriptionItem term={t("client:account_status.public_reason")}>
          {action.public_reason === undefined ? (
            t("client:account_status.public_reason.none")
          ) : (
            // Written by a moderator, in any language and direction (web-client/README.md Y10).
            <Text dir="auto" whitespace="preWrap" wrap="anywhere">
              {action.public_reason}
            </Text>
          )}
        </DescriptionItem>
      </DescriptionList>
      {action.status_ids.length > 0 && <ActionPosts statusIds={action.status_ids} />}
      <AppealPart action={action} form={form} />
    </Stack>
  );
}

/** The posts an action on posts acted on; a deleted one says so. */
function ActionPosts({ statusIds }: { statusIds: readonly string[] }) {
  const t = useT();
  const headingId = useId();
  const posts = useActionStatuses(statusIds);
  return (
    <Stack as="section" aria-labelledby={headingId} gap="2">
      <Heading level={4} id={headingId}>
        {t("client:account_status.posts.title")}
      </Heading>
      <Stack as="ul" divided border="lg">
        {posts.map((post) => (
          <li key={post.id}>
            {post.status === "found" ? (
              <StatusCard status={post.value} withActions={false} />
            ) : post.status === "loading" ? (
              <Box paddingX="4" paddingY="3">
                <Spinner size="sm" label={t("client:account_status.posts.loading")} />
              </Box>
            ) : (
              <Box paddingX="4" paddingY="3">
                <Hint size="inherit">{t("client:account_status.posts.missing")}</Hint>
              </Box>
            )}
          </li>
        ))}
      </Stack>
    </Stack>
  );
}

/** An action's appeal: the appeal and its decision, the button that opens one, or why there can be none. */
function AppealPart({ action, form }: { action: ModerationAction; form: AppealForm }) {
  const t = useT();
  const intl = useIntl();
  const now = useNow();
  const headingId = appealHeadingId(action.id);
  let body: ReactNode;
  if (action.appeal !== undefined) {
    body = <AppealFacts appeal={action.appeal} />;
  } else if (action.appealable) {
    const deadline = action.appeal_deadline;
    body = (
      <Stack align="start" gap="2">
        {deadline !== undefined && (
          <p>
            {t("client:account_status.appeal.until", {
              days: daysUntil(now, deadline),
              date: intl.formatDate(deadline, dateTimeFormat),
            })}
          </p>
        )}
        <Button
          size="sm"
          aria-label={t("client:account_status.appeal.open_of", {
            date: intl.formatDate(action.issued_at, dateTimeFormat),
          })}
          onPress={() => {
            form.open(action);
          }}
        >
          {t("client:account_status.appeal.open")}
        </Button>
      </Stack>
    );
  } else {
    body = (
      <p>
        {action.appeal_deadline === undefined
          ? t("client:account_status.appeal.unavailable")
          : t("client:account_status.appeal.closed", { date: intl.formatDate(action.appeal_deadline, dateTimeFormat) })}
      </p>
    );
  }
  return (
    <Stack as="section" aria-labelledby={headingId} gap="2">
      {/* Focus moves here once an appeal is sent, as the button that opened it is gone. */}
      <Heading level={4} id={headingId} focusable>
        {t("client:account_status.appeal.title")}
      </Heading>
      {body}
    </Stack>
  );
}

function AppealFacts({ appeal }: { appeal: ModerationAppeal }) {
  const t = useT();
  const badge = appealStateBadges[appeal.state];
  return (
    <Stack gap="3">
      <div>
        <StatusBadge icon={badge.icon} tone={badge.tone} label={t(appealStateLabels[appeal.state])} />
      </div>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:account_status.appeal.submitted_at")}>
          <DateTime value={appeal.submitted_at} />
        </DescriptionItem>
        {appeal.decided_at !== undefined && (
          <DescriptionItem term={t("client:account_status.appeal.decided_at")}>
            <DateTime value={appeal.decided_at} />
          </DescriptionItem>
        )}
      </DescriptionList>
      <DescriptionList>
        <DescriptionItem term={t("client:account_status.appeal.text")}>
          {/* The user's own text (web-client/README.md Y10). */}
          <Text dir="auto" whitespace="preWrap" wrap="anywhere">
            {appeal.text}
          </Text>
        </DescriptionItem>
        {appeal.decision_reason !== undefined && (
          <DescriptionItem term={t("client:account_status.appeal.decision_reason")}>
            <Text dir="auto" whitespace="preWrap" wrap="anywhere">
              {appeal.decision_reason}
            </Text>
          </DescriptionItem>
        )}
      </DescriptionList>
    </Stack>
  );
}

/** Why an appeal's text cannot be sent, or null when it can. */
function validateAppealText(t: Translate, text: string): string | null {
  if (text.trim() === "") return t("client:account_status.appeal.text.required");
  if (Array.from(text).length > appealTextLimit)
    return t("client:account_status.appeal.text.too_long", { max: appealTextLimit });
  return null;
}

function AppealDialog({ form }: { form: AppealForm }) {
  const t = useT();
  const intl = useIntl();
  const { action } = form;
  // The last action stays while the dialog fades out; before the first one the dialog is closed and empty.
  const date = action === null ? "" : intl.formatDate(action.issued_at, dateTimeFormat);
  return (
    <ConfirmDialog
      isOpen={form.isOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen) form.cancel();
      }}
      title={t("client:account_status.appeal.confirm.title")}
      description={t("client:account_status.appeal.confirm.description", { kind: action?.kind_label ?? "", date })}
      confirmLabel={t("client:account_status.appeal.send")}
      cancelLabel={t("client:account_status.appeal.cancel")}
      tone="primary"
      onConfirm={() => {
        form.submit();
      }}
      isPending={form.isPending}
      {...(form.error === undefined
        ? {}
        : { errorMessage: t("client:account_status.appeal.failed", { reason: form.error }) })}
    >
      <ReasonField
        label={t("client:account_status.appeal.text.label")}
        description={t("client:account_status.appeal.text.description")}
        value={form.text}
        onChange={form.changeText}
        isRequired
        // Text of white space only is empty to the server too, and too long a text is refused; either one
        // stops the dialog's form before it sends (the browser's constraint validation).
        validate={(value) => validateAppealText(t, value)}
        characterLimit={appealTextLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:account_status.appeal.characters_over", { count: -remaining })
            : t("client:account_status.appeal.characters_left", { count: remaining })
        }
        errorMessage={validateAppealText(t, form.text) ?? t("client:account_status.appeal.text.required")}
      />
    </ConfirmDialog>
  );
}
