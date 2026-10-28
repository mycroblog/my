// SPDX-License-Identifier: AGPL-3.0-only
// An account's admin page below its heading (FR-714, FR-703, FR-713): the account with its state, registration
// (with the invite it signed up with and who created it, ADR-0108 §5) and roles; what its actions take away now; its email address and IP addresses, which a local account shows only
// when the moderator presses "show" (each time is recorded in the audit log, ADR-0038); whether its user has
// two-factor authentication, with its removal, which asks first (recorded in the audit log); for manage_login_recovery,
// issuing a password reset link for its user, which asks first, is recorded in the audit log, and shows the link
// once with a copy button; the moderation actions
// on it, each in effect with a lift that asks for an optional reason first; its reports; and the form of a new
// action, which offers a remote account only the kinds a remote account takes. What a change did stays on the
// page in a StatusMessage (role="status") next to what caused it (web-client/14-admin.md §14). A viewer with manage_roles
// also has the account's roles, to grant and revoke (AccountRoles); one with manage_domains, the refetch of a
// remote account's posts from its server (RefetchStatusesSection).
import {
  AccountItem,
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DetailSection,
  EmptyState,
  Heading,
  Hint,
  Link,
  ReasonField,
  RegionError,
  SectionHeader,
  Spinner,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useId, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import type { AdminAccountDetail, AdminAccountPrivateData, EmailSuppression } from "../api/accounts";
import { type AccountAction, accountActionTextLimit, type Report } from "../api/reports";
import {
  adminAccountRegistrationStateLabels,
  adminAccountRestrictionLabels,
  adminAccountStateLabels,
  type LiftAccountAction,
  useAdminAccount,
  useLiftAccountAction,
  usePrivateData,
  useCopyPasswordResetLink,
  useIssuePasswordResetLink,
  useLiftEmailSuppression,
  useRemoveTwoFactor,
} from "../hooks/useAdminAccounts";
import { useHasPermission } from "../hooks/usePermissions";
import {
  accountActionKindLabels,
  accountActionStateLabels,
  accountName,
  handleOf,
  isolate,
  reportCategoryLabels,
  reportOutcomeLabels,
  useTargetActions,
} from "../hooks/useReports";
import { AccountActionForm } from "./AccountActionForm";
import { AccountRoles } from "./AccountRoles";
import { AccountOrigin } from "./AccountSearch";
import { DateTime, RecordList, RequirePermission, useFocusAfterChange } from "./parts";
import { RefetchStatusesSection } from "./RefetchStatuses";
import { InviteCreator, inviteCreatorKindLabels } from "./inviteParts";
import { AccountLabel, AssigneeName } from "./reportParts";
import { imageSources } from "../../../lib/images";

export function AccountDetailView({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_accounts">
      <AccountPage id={id} />
    </RequirePermission>
  );
}

function AccountPage({ id }: { id: string }) {
  const t = useT();
  const account = useAdminAccount(id);
  const [issued, setIssued] = useState<string | undefined>(undefined);
  const handle = handleOf(account.account);
  const canManageRoles = useHasPermission("manage_roles");
  // Refetching a remote account's posts from its server (FR-407) is for a viewer with manage_domains.
  const canRefetch = useHasPermission("manage_domains");
  // Recovering a user's sign-in is server administration (ADR-0058).
  const canRecoverSignIn = useHasPermission("manage_login_recovery");
  return (
    <Stack gap="8">
      <DetailSection title={t("client:admin.account.summary.title")}>
        <AccountSummary account={account} />
      </DetailSection>
      <DetailSection title={t("client:admin.account.restrictions.title")}>
        {account.restrictions.length === 0 ? (
          <p>{t("client:admin.account.restrictions.none")}</p>
        ) : (
          <Stack as="ul" marker="discInside" gap="1">
            {account.restrictions.map((restriction) => (
              <li key={restriction}>{t(adminAccountRestrictionLabels[restriction])}</li>
            ))}
          </Stack>
        )}
      </DetailSection>
      {!account.local && account.state !== "deleted" && canRefetch && (
        <RefetchStatusesSection target={{ kind: "account", id: account.id }} />
      )}
      {account.local && canManageRoles && <AccountRoles accountId={account.id} handle={isolate(handle)} />}
      {account.local && account.email_verified !== undefined && (
        <EmailSuppressionSection id={account.id} suppression={account.email_suppression} handle={isolate(handle)} />
      )}
      {account.local && account.two_factor_enabled !== undefined && (
        <TwoFactorSection id={account.id} enabled={account.two_factor_enabled} handle={isolate(handle)} />
      )}
      {account.local && account.two_factor_enabled !== undefined && canRecoverSignIn && (
        <PasswordResetSection id={account.id} handle={isolate(handle)} />
      )}
      {account.local && <PrivateDataSection id={account.id} />}
      <ActionHistory account={account} />
      <DetailSection title={t("client:admin.account.reports.title")}>
        <AccountReports reports={account.reports} />
      </DetailSection>
      <DetailSection title={t("client:admin.account.action.title")}>
        <StatusMessage tone="success">{issued}</StatusMessage>
        {account.state === "deleted" ? (
          <p>{t("client:admin.account.action.deleted")}</p>
        ) : (
          <>
            <Hint>{t("client:admin.account.action.posts")}</Hint>
            <AccountActionForm
              target={{
                accountId: account.id,
                remote: !account.local,
                statusIds: [],
                category: "other",
                hasEmail: account.email_verified !== undefined,
              }}
              handle={handle}
              posts={[]}
              onIssued={(action) => {
                setIssued(t("client:admin.account.action.issued", { kind: t(accountActionKindLabels[action.kind]) }));
              }}
            />
          </>
        )}
      </DetailSection>
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
 * Whether emails no longer reach the primary address of a local account (a hard bounce or a complaint, ADR-0086 §5),
 * since when, and lifting it, which asks first; the lift is recorded in the audit log. Nothing shows while the address
 * is not suppressed, except what a lift just did.
 */
function EmailSuppressionSection({
  id,
  suppression,
  handle,
}: {
  id: string;
  suppression: EmailSuppression | undefined;
  handle: string;
}) {
  const t = useT();
  const intl = useIntl();
  const headingRef = useRef<HTMLHeadingElement>(null);
  // The button goes away with the lift: focus moves to the region's heading.
  const focusHeading = useFocusAfterChange(headingRef, suppression === undefined);
  const lift = useLiftEmailSuppression(id, handle, focusHeading);
  const canLift = useHasPermission("manage_accounts");
  if (suppression === undefined && lift.status === undefined) return null;
  return (
    <DetailSection title={t("client:admin.account.email_suppression.title")} headingRef={headingRef}>
      <StatusMessage tone="success">{lift.status}</StatusMessage>
      {suppression !== undefined && (
        <Stack align="start" gap="3">
          <p>
            {suppression.kind === "hard_bounce"
              ? t("client:admin.account.email_suppression.hard_bounce")
              : t("client:admin.account.email_suppression.complaint")}
          </p>
          <p>
            {t("client:admin.account.email_suppression.since", {
              since: intl.formatDate(suppression.since, { dateStyle: "medium", timeStyle: "short" }),
            })}
          </p>
          {canLift && (
            <>
              <Hint>{t("client:admin.account.email_suppression.lift.description")}</Hint>
              <Button onPress={lift.open} isPending={lift.isPending}>
                {t("client:admin.account.email_suppression.lift")}
              </Button>
            </>
          )}
        </Stack>
      )}
      <ConfirmDialog
        isOpen={lift.isOpen}
        onOpenChange={(isOpen) => {
          if (!isOpen) lift.cancel();
        }}
        title={t("client:admin.account.email_suppression.confirm.title", { handle })}
        description={t("client:admin.account.email_suppression.confirm.description")}
        confirmLabel={t("client:admin.account.email_suppression.lift")}
        cancelLabel={t("client:admin.account_actions.confirm.cancel")}
        onConfirm={() => {
          lift.confirm();
        }}
        isPending={lift.isPending}
        {...(lift.error === undefined
          ? {}
          : { errorMessage: t("client:admin.account.email_suppression.failed", { reason: lift.error }) })}
      />
    </DetailSection>
  );
}

/**
 * Whether the user of a local account signs in with a code too, and removing it for a user who lost their
 * authenticator app and recovery codes, which asks first; the removal is recorded in the audit log.
 */
function TwoFactorSection({ id, enabled, handle }: { id: string; enabled: boolean; handle: string }) {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  // The button goes away with the removal: focus moves to the region's heading.
  const focusHeading = useFocusAfterChange(headingRef, enabled);
  const remove = useRemoveTwoFactor(id, handle, focusHeading);
  // Removing it is server administration (ADR-0058): moderators see the state but not the button.
  const canRemove = useHasPermission("manage_login_recovery");
  return (
    <DetailSection title={t("client:admin.account.two_factor.title")} headingRef={headingRef}>
      <StatusMessage tone="success">{remove.status}</StatusMessage>
      <p>{enabled ? t("client:admin.account.two_factor.on") : t("client:admin.account.two_factor.off")}</p>
      {enabled && canRemove && (
        <Stack align="start" gap="3">
          <Hint>{t("client:admin.account.two_factor.remove.description")}</Hint>
          <Button onPress={remove.open} isPending={remove.isPending}>
            {t("client:admin.account.two_factor.remove")}
          </Button>
        </Stack>
      )}
      <ConfirmDialog
        isOpen={remove.isOpen}
        onOpenChange={(isOpen) => {
          if (!isOpen) remove.cancel();
        }}
        title={t("client:admin.account.two_factor.confirm.title", { handle })}
        description={t("client:admin.account.two_factor.confirm.description")}
        confirmLabel={t("client:admin.account.two_factor.remove")}
        cancelLabel={t("client:admin.account_actions.confirm.cancel")}
        onConfirm={() => {
          remove.confirm();
        }}
        isPending={remove.isPending}
        {...(remove.error === undefined
          ? {}
          : { errorMessage: t("client:admin.account.two_factor.failed", { reason: remove.error }) })}
      />
    </DetailSection>
  );
}

/**
 * Issuing a link that sets a new password for the user of a local account, to hand to them when email does not
 * reach them (FR-002); it asks first, and the issuance is recorded in the audit log. The link is shown once, with
 * a button that copies it; issuing again replaces it. Like removing two-factor authentication, it lets the holder of
 * the link into the account, so it is server administration (ADR-0058): the region is only for manage_login_recovery.
 */
function PasswordResetSection({ id, handle }: { id: string; handle: string }) {
  const t = useT();
  const intl = useIntl();
  const issue = useIssuePasswordResetLink(id, handle);
  const copy = useCopyPasswordResetLink();
  const linkLabelId = useId();
  const { link } = issue;
  return (
    <DetailSection title={t("client:admin.account.password_reset.title")}>
      <StatusMessage tone="success">{issue.status}</StatusMessage>
      {link !== undefined && (
        <Stack gap="2" border="md" padding="3">
          <Text as="p" weight="medium" id={linkLabelId}>
            {t("client:admin.account.password_reset.link")}
          </Text>
          <Hint>
            {t("client:admin.account.password_reset.link.notice", {
              expiresAt: intl.formatDate(link.expires_at, { dateStyle: "medium", timeStyle: "short" }),
            })}
          </Hint>
          {/* A URL reads left to right in every locale. */}
          <Text as="p" mono wrap="anywhere" selectAll aria-labelledby={linkLabelId} dir="ltr">
            {link.url}
          </Text>
          <Box>
            <Button
              size="sm"
              onPress={() => {
                copy(link.url);
              }}
            >
              {t("client:admin.account.password_reset.copy")}
            </Button>
          </Box>
        </Stack>
      )}
      <Stack align="start" gap="3">
        <Hint>{t("client:admin.account.password_reset.description")}</Hint>
        <Button onPress={issue.open} isPending={issue.isPending}>
          {t("client:admin.account.password_reset.issue")}
        </Button>
      </Stack>
      <ConfirmDialog
        isOpen={issue.isOpen}
        onOpenChange={(isOpen) => {
          if (!isOpen) issue.cancel();
        }}
        title={t("client:admin.account.password_reset.confirm.title", { handle })}
        description={t("client:admin.account.password_reset.confirm.description")}
        confirmLabel={t("client:admin.account.password_reset.issue")}
        cancelLabel={t("client:admin.account_actions.confirm.cancel")}
        tone="primary"
        onConfirm={() => {
          issue.confirm();
        }}
        isPending={issue.isPending}
        {...(issue.error === undefined
          ? {}
          : { errorMessage: t("client:admin.account.password_reset.failed", { reason: issue.error }) })}
      />
    </DetailSection>
  );
}

function PrivateDataSection({ id }: { id: string }) {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [revealed, setRevealed] = useState(false);
  const focusHeading = useFocusAfterChange(headingRef, revealed);
  const privateData = usePrivateData(id, () => {
    setRevealed(true);
    focusHeading();
  });
  return (
    <DetailSection title={t("client:admin.account.private.title")} headingRef={headingRef}>
      <StatusMessage>
        {privateData.data === undefined ? undefined : t("client:admin.account.private.shown")}
      </StatusMessage>
      {privateData.data === undefined ? (
        <Stack align="start" gap="3">
          <Hint>{t("client:admin.account.private.description")}</Hint>
          <Button onPress={privateData.reveal} isPending={privateData.isPending}>
            {t("client:admin.account.private.reveal")}
          </Button>
        </Stack>
      ) : (
        <PrivateData data={privateData.data} />
      )}
    </DetailSection>
  );
}

/** The search of the accounts with `query`, from the first page. */
function searchHref(query: Record<string, string>): string {
  return `/moderation/accounts?${new URLSearchParams(query).toString()}`;
}

/** An IP address, which reads left to right in every locale. */
function IpAddress({ ip }: { ip: string | undefined }) {
  const t = useT();
  if (ip === undefined) return t("client:admin.account.private.unknown");
  return (
    <Text dir="ltr" wrap="anywhere">
      {ip}
    </Text>
  );
}

function PrivateData({ data }: { data: AdminAccountPrivateData }) {
  const t = useT();
  const emailDomain = data.email?.split("@").at(-1);
  return (
    <Stack gap="6">
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.account.private.email")}>
          {data.email === undefined ? (
            t("client:admin.account.email.none")
          ) : (
            <Stack as="span" align="start" gap="1">
              <Text dir="ltr" wrap="anywhere">
                {data.email}
              </Text>
              <Hint as="span">
                {data.email_verified === true
                  ? t("client:admin.account.email_verified.yes")
                  : t("client:admin.account.email_verified.no")}
              </Hint>
              {emailDomain !== undefined && emailDomain !== "" && (
                <Link href={searchHref({ email_domain: emailDomain })} variant="standalone" size="sm">
                  {t("client:admin.account.private.search_email_domain", { domain: `⁦${emailDomain}⁩` })}
                </Link>
              )}
            </Stack>
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account.private.registration_ip")}>
          {data.registration_ip === undefined ? (
            t("client:admin.account.private.unknown")
          ) : (
            <Stack as="span" align="start" gap="1">
              <IpAddress ip={data.registration_ip} />
              {data.registered_at !== undefined && (
                <Hint as="span">
                  <DateTime value={data.registered_at} />
                </Hint>
              )}
            </Stack>
          )}
        </DescriptionItem>
      </DescriptionList>
      <Stack gap="3">
        <Heading level={3}>{t("client:admin.account.private.sign_ins")}</Heading>
        {data.sign_ins.length === 0 ? (
          <Hint size="inherit">{t("client:admin.account.private.sign_ins.none")}</Hint>
        ) : (
          <Stack as="ul" divided>
            {data.sign_ins.map((signIn, index) => (
              // A sign-in has no ID: its time and address keep the rows of one answer apart, which never changes.
              <Box as="li" paddingY="3" key={`${signIn.signed_in_at} ${signIn.ip ?? ""} ${String(index)}`}>
                <DescriptionList columns={3}>
                  <DescriptionItem term={t("client:admin.account.private.signed_in_at")}>
                    <DateTime value={signIn.signed_in_at} />
                  </DescriptionItem>
                  <DescriptionItem term={t("client:admin.account.private.ip")}>
                    <IpAddress ip={signIn.ip} />
                  </DescriptionItem>
                  <DescriptionItem term={t("client:admin.account.private.user_agent")}>
                    {signIn.user_agent === undefined ? (
                      t("client:admin.account.private.unknown")
                    ) : (
                      <Text dir="ltr" wrap="anywhere">
                        {signIn.user_agent}
                      </Text>
                    )}
                  </DescriptionItem>
                </DescriptionList>
              </Box>
            ))}
          </Stack>
        )}
      </Stack>
      <Stack gap="3">
        <Heading level={3}>{t("client:admin.account.private.same_ip")}</Heading>
        {data.same_ip_accounts.length === 0 ? (
          <Hint size="inherit">{t("client:admin.account.private.same_ip.none")}</Hint>
        ) : (
          <Stack as="ul" divided>
            {data.same_ip_accounts.map((group) => (
              <Stack as="li" gap="2" paddingY="3" key={group.ip}>
                <Heading level={4}>
                  <IpAddress ip={group.ip} />
                </Heading>
                <Stack as="ul" gap="2">
                  {group.accounts.map((other) => (
                    <Cluster as="li" align="baseline" gap="3" rowGap="0" key={other.id}>
                      <Link href={`/moderation/accounts/${encodeURIComponent(other.id)}`} variant="standalone">
                        <AccountLabel account={other.account} />
                      </Link>
                      <Hint as="span">{t(adminAccountStateLabels[other.state])}</Hint>
                    </Cluster>
                  ))}
                </Stack>
                <Link href={searchHref({ ip: group.ip })} variant="standalone" size="sm">
                  {t("client:admin.account.private.search_ip", { ip: `⁦${group.ip}⁩` })}
                </Link>
              </Stack>
            ))}
          </Stack>
        )}
      </Stack>
    </Stack>
  );
}

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
  const { confirmation } = lift;
  const handle = isolate(handleOf(account.account));
  return (
    <DetailSection title={t("client:admin.account.actions.title")}>
      <StatusMessage tone="success">{lifted}</StatusMessage>
      {actions.isLoading ? (
        <Spinner size="sm" label={t("client:admin.report.history.loading")} />
      ) : actions.items.length === 0 && actions.error !== undefined ? (
        <RegionError heading={t("client:admin.report.history.failed")} headingLevel={3} reason={actions.error} />
      ) : (
        <RecordList
          list={actions}
          getId={(action) => action.id}
          listRef={listRef}
          empty={<EmptyState headingLevel={3} heading={t("client:admin.report.history.empty")} />}
          moreError={
            actions.error !== undefined && (
              <RegionError heading={t("client:admin.report.history.failed")} reason={actions.error} />
            )
          }
        >
          {(action) => <ActionRow action={action} local={account.local} lift={lift} />}
        </RecordList>
      )}
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
    </DetailSection>
  );
}

const issuerLabels: Readonly<Record<AccountAction["issuer"], ClientMessageKey>> = {
  moderator: "client:admin.account.actions.issuer.cli",
  system: "client:admin.account.actions.issuer.system",
  remote: "client:admin.account.actions.issuer.remote",
};

function ActionRow({ action, local, lift }: { action: AccountAction; local: boolean; lift: LiftAccountAction }) {
  const t = useT();
  const intl = useIntl();
  const kind = t(accountActionKindLabels[action.kind]);
  // A ban is never lifted (the API refuses it); it is undone only by overturning its appeal.
  const liftable = action.state === "active" && action.kind !== "ban";
  return (
    <Stack gap="3">
      <SectionHeader
        actions={
          liftable && (
            <Button
              size="sm"
              isPending={lift.isPending && lift.confirmation?.action.id === action.id}
              aria-label={t("client:admin.account.actions.lift_action", {
                kind,
                date: intl.formatDate(action.issued_at, { dateStyle: "medium", timeStyle: "short" }),
              })}
              onPress={() => {
                lift.open(action);
              }}
            >
              {t("client:admin.account.actions.lift")}
            </Button>
          )
        }
      >
        <Heading level={3} focusable>
          {kind}
        </Heading>
      </SectionHeader>
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
            t(issuerLabels[action.issuer])
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
    </Stack>
  );
}

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
  );
}
