// SPDX-License-Identifier: AGPL-3.0-only
// The account management page of a local account (screens/moderation-account-manage.md, web-client/14-admin.md §14):
// what supports the account's user rather than moderates the account, off the review page, each region for the
// viewer with its own permission. Its roles, to grant and revoke (AccountRoles, manage_roles); the suppression of
// its email address, while there is one, with its lift, which asks first (manage_accounts); for
// manage_login_recovery, removing its two-factor authentication and issuing a password reset link, each of which
// asks first and is recorded in the audit log, the link shown once with a copy button; and its email address and
// IP addresses, which show only when the moderator presses "show" (each time is recorded in the audit log,
// ADR-0058). What a change did stays in its region in a StatusMessage (role="status"). A remote account has none of
// these: the page says so.
import {
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  Link,
  Section,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useId, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import type { AdminAccountPrivateData, EmailSuppression } from "../api/accounts";
import {
  adminAccountStateLabels,
  useAdminAccount,
  useCopyPasswordResetLink,
  useIssuePasswordResetLink,
  useLiftEmailSuppression,
  usePrivateData,
  useRemoveTwoFactor,
} from "../hooks/useAdminAccounts";
import { useHasPermission } from "../hooks/usePermissions";
import { handleOf, isolate } from "../hooks/useReports";
import { AccountNav, AccountOfPage } from "./AccountNav";
import { AccountRoles } from "./AccountRoles";
import { DateTime, RequirePermission, useFocusAfterChange } from "./parts";
import { AccountLabel } from "./reportParts";

export function AccountManageView({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_accounts">
      <AccountManagePage id={id} />
    </RequirePermission>
  );
}

function AccountManagePage({ id }: { id: string }) {
  const t = useT();
  const account = useAdminAccount(id);
  const handle = isolate(handleOf(account.account));
  const canManageRoles = useHasPermission("manage_roles");
  // Recovering a user's sign-in is server administration (ADR-0058).
  const canRecoverSignIn = useHasPermission("manage_login_recovery");
  return (
    <Stack gap="6">
      <AccountNav account={account} current="manage" />
      <AccountOfPage account={account} />
      {account.local ? (
        <>
          {canManageRoles && <AccountRoles accountId={account.id} handle={handle} />}
          {account.email_verified !== undefined && (
            <EmailSuppressionSection id={account.id} suppression={account.email_suppression} handle={handle} />
          )}
          {/* An account with a user of this server has the state of its two-factor authentication. */}
          {canRecoverSignIn && account.two_factor_enabled !== undefined && (
            <>
              <TwoFactorSection id={account.id} enabled={account.two_factor_enabled} handle={handle} />
              <PasswordResetSection id={account.id} handle={handle} />
            </>
          )}
          <PrivateDataSection id={account.id} />
        </>
      ) : (
        <EmptyState
          heading={t("client:admin.account.manage.remote")}
          description={t("client:admin.account.manage.remote.description")}
        />
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
    <Section surface="card" title={t("client:admin.account.email_suppression.title")} headingRef={headingRef}>
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
              since: intl.formatDate(suppression.state_changed_at, { dateStyle: "medium", timeStyle: "short" }),
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
    </Section>
  );
}

/**
 * Whether the user of a local account signs in with a code too, and removing it for a user who lost their
 * authenticator app and recovery codes, which asks first; the removal is recorded in the audit log. Like the password
 * reset, it is sign-in recovery (ADR-0058): the region is only for manage_login_recovery.
 */
function TwoFactorSection({ id, enabled, handle }: { id: string; enabled: boolean; handle: string }) {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  // The button goes away with the removal: focus moves to the region's heading.
  const focusHeading = useFocusAfterChange(headingRef, enabled);
  const remove = useRemoveTwoFactor(id, handle, focusHeading);
  return (
    <Section surface="card" title={t("client:admin.account.two_factor.title")} headingRef={headingRef}>
      <StatusMessage tone="success">{remove.status}</StatusMessage>
      <p>{enabled ? t("client:admin.account.two_factor.on") : t("client:admin.account.two_factor.off")}</p>
      {enabled && (
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
    </Section>
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
    <Section surface="card" title={t("client:admin.account.password_reset.title")}>
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
          {link.login_providers_only && (
            <InlineError>{t("client:admin.account.password_reset.login_providers_only")}</InlineError>
          )}
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
    </Section>
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
    <Section surface="card" title={t("client:admin.account.private.title")} headingRef={headingRef}>
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
    </Section>
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
