// SPDX-License-Identifier: AGPL-3.0-only
// An account's review page below its heading (FR-714, FR-703, FR-713; screens/moderation-account.md), the first of
// the account's pages (AccountNav), in @my/ui's TriageLayout: the summary with what its actions take away now as
// badges, the form of a new action as the decision (A6: a moderator from a report comes to act), then the evidence.
// The account with its state, registration (with the invite it signed up with and who created it, ADR-0106 §18) and
// roles; what its actions take away now; the newest page of the moderation actions on it, each in effect with a lift
// that asks for an optional reason first, and a link to all of them (AccountActions); its newest reports, as many
// as the API gives; and the form of a new action, which offers a remote account only the kinds a remote account
// takes. What a change did stays on the page in a StatusMessage (role="status") next to what caused it
// (web-client/14-admin.md §14). A viewer with manage_domains also has the refetch of a remote account's posts from
// its server (RefetchStatusesSection). Roles, sign-in recovery and the email and IP addresses are on the account
// management page (AccountManage).
import {
  AccountItem,
  BlockIcon,
  Box,
  Card,
  CheckIcon,
  Cluster,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Heading,
  Hint,
  Link,
  RegionError,
  Section,
  SectionHeader,
  Spinner,
  Stack,
  StatusBadge,
  StatusMessage,
  Text,
  TriageLayout,
} from "@my/ui";
import { useId, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import { CompactStatus } from "../../status";
import type { AdminAccountDetail } from "../api/accounts";
import type { AccountAction, Report } from "../api/reports";
import {
  adminAccountRegistrationStateLabels,
  adminAccountRestrictionLabels,
  adminAccountStateLabels,
  type LiftAccountAction,
  useAdminAccount,
  useLiftAccountAction,
} from "../hooks/useAdminAccounts";
import type { PagedList } from "../hooks/pagedList";
import { useHasPermission } from "../hooks/usePermissions";
import { type StartingPost, useStartingPost } from "../hooks/useStartingPost";
import {
  accountActionKindLabels,
  accountName,
  handleOf,
  isolate,
  reportCategoryLabels,
  reportOutcomeLabels,
  useTargetActions,
} from "../hooks/useReports";
import { AccountActionForm } from "./AccountActionForm";
import { accountActionHref, ActionFacts, LiftActionDialog, LiftButton } from "./AccountActions";
import { AccountNav, accountPageHref } from "./AccountNav";
import { AccountOrigin } from "./AccountSearch";
import { DateTime, RecordList, RequirePermission } from "./parts";
import { RefetchStatusesSection } from "./RefetchStatuses";
import { InviteCreator, inviteCreatorKindLabels } from "./inviteParts";
import { AccountLabel } from "./reportParts";

/** The restrictions that keep the account from signing in or reaching anyone, in the danger tone; the others warn. */
const severeRestrictions: ReadonlySet<string> = new Set(["login", "federation", "profile_view"]);

export function AccountDetailView({ id, statusId }: { id: string; statusId?: string | undefined }) {
  return (
    <RequirePermission permission="manage_accounts">
      <AccountReview id={id} statusId={statusId} />
    </RequirePermission>
  );
}

/**
 * Above the form of an action: the post the page starts from when it has one, in full so that the moderator can read
 * what they came about, with how the form takes it; else where the actions on posts are.
 */
function StartingPostNote({ state, remote }: { state: StartingPost; remote: boolean }) {
  const t = useT();
  switch (state.kind) {
    case "none":
      return <Hint>{t("client:admin.account.action.posts")}</Hint>;
    case "loading":
      return <Hint>{t("client:admin.account.action.post.loading")}</Hint>;
    case "failed":
      return <Hint>{t("client:admin.account.action.post.failed", { reason: state.message })}</Hint>;
    case "ready":
      return (
        <Stack gap="2">
          <CompactStatus status={state.post} label={t("client:admin.account.action.post.label")} />
          <Hint>
            {remote ? t("client:admin.account.action.post.remote") : t("client:admin.account.action.post.chosen")}
          </Hint>
        </Stack>
      );
  }
}

function AccountReview({ id, statusId }: { id: string; statusId: string | undefined }) {
  const t = useT();
  const account = useAdminAccount(id);
  const startingPost = useStartingPost(id, statusId);
  const posts = startingPost.kind === "ready" ? [startingPost.post] : [];
  const [issued, setIssued] = useState<string | undefined>(undefined);
  const handle = handleOf(account.account);
  // Refetching a remote account's posts from its server (FR-407) is for a viewer with manage_domains.
  const canRefetch = useHasPermission("manage_domains");
  const restrictionsId = useId();
  return (
    <Stack gap="6">
      <AccountNav account={account} current="review" />
      <Card>
        <TriageLayout
          summary={
            <Section surface="card" title={t("client:admin.account.summary.title")}>
              <AccountSummary account={account} />
              {/* What the account cannot do now, as badges (ADR-0110 P3). */}
              <Stack gap="2">
                <Heading level={3} id={restrictionsId}>
                  {t("client:admin.account.restrictions.title")}
                </Heading>
                {account.restrictions.length === 0 ? (
                  <StatusBadge
                    icon={<CheckIcon />}
                    label={t("client:admin.account.restrictions.none")}
                    tone="success"
                  />
                ) : (
                  <Cluster as="ul" gap="2" aria-labelledby={restrictionsId}>
                    {account.restrictions.map((restriction) => (
                      <li key={restriction}>
                        <StatusBadge
                          icon={<BlockIcon />}
                          label={t(adminAccountRestrictionLabels[restriction])}
                          tone={severeRestrictions.has(restriction) ? "danger" : "warning"}
                        />
                      </li>
                    ))}
                  </Cluster>
                )}
              </Stack>
            </Section>
          }
          decision={
            <Section surface="card" title={t("client:admin.account.action.title")}>
              <StatusMessage tone="success">{issued}</StatusMessage>
              {account.state === "deleted" ? (
                <p>{t("client:admin.account.action.deleted")}</p>
              ) : (
                <>
                  <StartingPostNote state={startingPost} remote={!account.local} />
                  <AccountActionForm
                    target={{
                      accountId: account.id,
                      remote: !account.local,
                      statusIds: posts.map((post) => post.id),
                      category: "other",
                      hasEmail: account.email_verified !== undefined,
                    }}
                    handle={handle}
                    posts={posts}
                    onIssued={(action) => {
                      setIssued(
                        t("client:admin.account.action.issued", { kind: t(accountActionKindLabels[action.kind]) }),
                      );
                    }}
                  />
                </>
              )}
            </Section>
          }
          evidence={
            <>
              <ActionHistory account={account} />
              {account.reports !== undefined && (
                <Section surface="card" title={t("client:admin.account.reports.title")}>
                  <AccountReports reports={account.reports} />
                </Section>
              )}
              {!account.local && account.state !== "deleted" && canRefetch && (
                <RefetchStatusesSection target={{ kind: "account", id: account.id }} />
              )}
            </>
          }
        />
      </Card>
    </Stack>
  );
}

function AccountSummary({ account }: { account: AdminAccountDetail }) {
  const t = useT();
  const canAudit = useHasPermission("view_audit");
  const { registration } = account;
  return (
    <Stack gap="3">
      <AccountItem
        name={accountName(account.account)}
        handle={account.account.display_name === "" ? undefined : handleOf(account.account)}
        avatar={imageSources(account.account.avatar)}
        avatarSeed={account.account.id}
      />
      <Cluster as="ul" gap="4" rowGap="1">
        {account.state !== "deleted" && (
          <li>
            <Link href={`/@${account.account.acct}`} variant="standalone">
              {t("client:admin.account.profile")}
            </Link>
          </li>
        )}
        {canAudit && (
          <li>
            <Link
              href={`/moderation/audit-log?target_kind=account&target=${encodeURIComponent(account.id)}`}
              variant="standalone"
            >
              {t("client:admin.account.audit")}
            </Link>
          </li>
        )}
      </Cluster>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.accounts.state")}>
          <Text weight="semibold">{t(adminAccountStateLabels[account.state])}</Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.accounts.origin")}>
          <AccountOrigin account={account} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.accounts.created_at")}>
          <DateTime value={account.account.created_at} />
        </DescriptionItem>
        {account.registration_state !== undefined && (
          <DescriptionItem term={t("client:admin.accounts.registration")}>
            {t(adminAccountRegistrationStateLabels[account.registration_state])}
          </DescriptionItem>
        )}
        {registration !== undefined && (
          <>
            <DescriptionItem term={t("client:admin.account.registration.submitted_at")}>
              <DateTime value={registration.submitted_at} />
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.account.registration.state_changed_at")}>
              <DateTime value={registration.state_changed_at} />
            </DescriptionItem>
            {registration.decision_note !== undefined && (
              <DescriptionItem term={t("client:admin.account.registration.decision_note")}>
                <span dir="auto">{registration.decision_note}</span>
              </DescriptionItem>
            )}
            <DescriptionItem term={t("client:admin.account.registration.invite")}>
              {registration.invite === undefined
                ? registration.invite_id === undefined
                  ? t("client:admin.account.registration.invite.none")
                  : t("client:admin.account.registration.invite.used")
                : t(inviteCreatorKindLabels[registration.invite.creator_kind])}
            </DescriptionItem>
            {registration.invite !== undefined && (
              <DescriptionItem term={t("client:admin.account.registration.invited_by")}>
                <InviteCreator invite={registration.invite} linked />
              </DescriptionItem>
            )}
          </>
        )}
        {account.local && (
          <>
            <DescriptionItem term={t("client:admin.account.email_verified")}>
              {account.email_verified === undefined
                ? t("client:admin.account.email.none")
                : account.email_verified
                  ? t("client:admin.account.email_verified.yes")
                  : t("client:admin.account.email_verified.no")}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.account.roles")}>
              {account.roles.length === 0 ? (
                t("client:admin.account.roles.none")
              ) : (
                // Role keys are identifiers, left to right in every locale.
                <Text dir="ltr" wrap="anywhere">
                  {account.roles.join(", ")}
                </Text>
              )}
            </DescriptionItem>
          </>
        )}
      </DescriptionList>
      {registration?.reason !== undefined && (
        <DescriptionList>
          <DescriptionItem term={t("client:admin.account.registration.reason")}>
            {/* Written by the applicant, in any language and direction (web-client/README.md Y10). */}
            <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
              {registration.reason}
            </Text>
          </DescriptionItem>
        </DescriptionList>
      )}
    </Stack>
  );
}

/**
 * The newest page of the actions on the account, each with its facts, as evidence for the decision; every action, a
 * page at a time, is on the account's actions page, which the region links to.
 */
function ActionHistory({ account }: { account: AdminAccountDetail }) {
  const t = useT();
  const listRef = useRef<HTMLUListElement>(null);
  const actions = useTargetActions(account.id);
  const [lifted, setLifted] = useState<string | undefined>(undefined);
  const keepFocus = useFocusHeadingAfterLift(listRef);
  const lift = useLiftAccountAction(account.id, (action) => {
    keepFocus(action.id);
    setLifted(t("client:admin.account.actions.lifted", { kind: t(accountActionKindLabels[action.kind]) }));
  });
  // The newest page only, without "show more": the full history is a table of its own (web-client/14-admin.md §14).
  const newest: PagedList<AccountAction> = {
    items: actions.pages[0] ?? [],
    hasMore: false,
    isLoadingMore: false,
    loadMore: () => undefined,
  };
  return (
    <Section surface="card" title={t("client:admin.account.actions.title")}>
      <StatusMessage tone="success">{lifted}</StatusMessage>
      {actions.isLoading ? (
        <Spinner size="sm" label={t("client:admin.report.history.loading")} />
      ) : actions.items.length === 0 && actions.error !== undefined ? (
        <RegionError heading={t("client:admin.report.history.failed")} headingLevel={3} reason={actions.error} />
      ) : (
        <RecordList
          list={newest}
          getId={(action) => action.id}
          listRef={listRef}
          empty={<EmptyState headingLevel={3} heading={t("client:admin.report.history.empty")} />}
        >
          {(action) => <ActionRow action={action} local={account.local} lift={lift} />}
        </RecordList>
      )}
      {newest.items.length > 0 && (
        <Box>
          <Link href={accountPageHref(account.id, "actions")} variant="standalone">
            {t("client:admin.account.actions.all")}
          </Link>
        </Box>
      )}
      <LiftActionDialog lift={lift} handle={isolate(handleOf(account.account))} />
    </Section>
  );
}

function ActionRow({ action, local, lift }: { action: AccountAction; local: boolean; lift: LiftAccountAction }) {
  return (
    <Stack gap="3">
      <SectionHeader actions={<LiftButton action={action} lift={lift} />}>
        <Heading level={3} focusable>
          <ActionKindLink action={action} />
        </Heading>
      </SectionHeader>
      <ActionFacts action={action} local={local} />
    </Stack>
  );
}

/** The kind of an action, as the link to its page. */
function ActionKindLink({ action }: { action: AccountAction }) {
  const t = useT();
  return (
    <Link href={accountActionHref(action.account_id, action.id)} variant="standalone">
      {t(accountActionKindLabels[action.kind])}
    </Link>
  );
}

/** How many of an account's reports GET /admin/accounts/{id} gives, newest first. */
const accountReportsCap = 20;

const reportStateLabels: Readonly<Record<Report["state"], ClientMessageKey>> = {
  open: "client:admin.reports.filter.open",
  resolved: "client:admin.reports.filter.resolved",
};

function AccountReports({ reports }: { reports: readonly Report[] }) {
  const t = useT();
  const intl = useIntl();
  const canHandle = useHasPermission("manage_reports");
  if (reports.length === 0) return <EmptyState headingLevel={3} heading={t("client:admin.account.reports.empty")} />;
  return (
    <Stack gap="2">
      {/* The API gives the newest reports only, and the report queue has no filter by account: say so. */}
      <Hint>{t("client:admin.account.reports.cap", { count: accountReportsCap })}</Hint>
      <Stack as="ul" divided>
        {reports.map((report) => {
          const title = t("client:admin.account.reports.item", {
            date: intl.formatDate(report.filed_at, { dateStyle: "medium", timeStyle: "short" }),
          });
          return (
            <Stack as="li" container gap="2" paddingY="3" key={report.id}>
              <Heading level={3}>
                {canHandle ? (
                  <Link href={`/moderation/reports/${encodeURIComponent(report.id)}`} variant="standalone">
                    {title}
                  </Link>
                ) : (
                  title
                )}
              </Heading>
              <DescriptionList columns={3}>
                <DescriptionItem term={t("client:admin.report.state")}>
                  {report.outcome === undefined
                    ? t(reportStateLabels[report.state])
                    : t(reportOutcomeLabels[report.outcome])}
                </DescriptionItem>
                <DescriptionItem term={t("client:admin.report.category")}>
                  {t(reportCategoryLabels[report.category])}
                </DescriptionItem>
                <DescriptionItem term={t("client:admin.report.reporter")}>
                  <AccountLabel account={report.reporter_account} />
                </DescriptionItem>
              </DescriptionList>
              {report.comment !== "" && (
                // Written by the reporter, in any language.
                <Text as="p" dir="auto" lineClamp={2} align="start" wrap="anywhere" whitespace="preWrap">
                  {report.comment}
                </Text>
              )}
            </Stack>
          );
        })}
      </Stack>
    </Stack>
  );
}
