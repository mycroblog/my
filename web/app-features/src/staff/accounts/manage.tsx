// SPDX-License-Identifier: AGPL-3.0-only

// The Management page of an account (FR-714, ADR-0058 §2, §9): for a local account, its email address and the IP
// addresses it signed up and signed in from, shown on demand alone, which the server records each time, with the
// accounts that used the same addresses and the way to those of the same email domain; the suppression of its email,
// lifted; and, for a holder of `manage_login_recovery`, a password reset link to hand over, shown once, and removing
// two-factor authentication, each asked first. For a remote account, refetching its posts from its server
// (`manage_domains`). Each section shows only to the reader whose permissions it needs.

import { moderatedAccountPath, sentenceHandleOf } from "@my/app-components";
import {
  usePermissions,
  useAccountWrites,
  type AccountPrivateData,
  type ModeratedAccountDetail,
  type PasswordResetLink,
} from "@my/app-data";
import {
  Button,
  ConfirmDialog,
  FactList,
  FeedNotice,
  ModalPresence,
  RecordTable,
  RelativeTime,
  Row,
  ScreenStack,
  Stack,
  TextLink,
  type Fact,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { staffEntryPath, staffHref } from "../entries";
import { Titled, useMoment, useStaffStrings } from "../parts";
import { AccountStateMark, CopyValue, refusalWords, useSaid } from "./parts";
import { emailDomainSearch } from "./search";

/** The sections of Management the reader's permissions show for `account`. */
export function ManagePage({ account }: { account: ModeratedAccountDetail }): ReactNode {
  const permissions = usePermissions();
  const { say, status } = useSaid();
  return (
    <ScreenStack>
      {status}
      {account.local && <PrivateData account={account} />}
      {account.local && account.email_suppression !== undefined && <EmailSuppression account={account} say={say} />}
      {account.local && permissions.includes("manage_login_recovery") && <LoginRecovery account={account} say={say} />}
      {!account.local && permissions.includes("manage_domains") && <Refetch account={account} say={say} />}
    </ScreenStack>
  );
}

/** The email and IP addresses, each Show until the reader asks, one showing for all; and who shares them. */
function PrivateData({ account }: { account: ModeratedAccountDetail }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const { reveal } = useAccountWrites();
  const [data, setData] = useState<AccountPrivateData | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState<string | undefined>(undefined);
  const ask = () => {
    if (busy) return;
    setBusy(true);
    setFailed(undefined);
    reveal(account.id).then(
      (answer) => {
        setBusy(false);
        setData(answer);
      },
      (e: unknown) => {
        setBusy(false);
        setFailed(refusalWords(t, e));
      },
    );
  };
  const kept = { onReveal: ask, busy };
  const domain = data?.email?.slice(data.email.lastIndexOf("@") + 1);
  const items: Fact[] = [
    {
      term: t("staff_accounts.private.email"),
      code: true,
      reveal: kept,
      value: data === undefined ? undefined : (data.email ?? t("staff_accounts.private.no_email")),
    },
    {
      term: t("staff_accounts.private.registration_ip"),
      code: true,
      reveal: kept,
      value:
        data === undefined
          ? undefined
          : data.registration_ip === undefined
            ? t("staff_accounts.private.unknown")
            : data.registration_ip,
    },
    ...(data?.registered_at === undefined
      ? []
      : [{ term: t("staff_accounts.private.registered_at"), value: moment(data.registered_at) }]),
  ];
  return (
    <Titled title={t("staff_accounts.private.title")}>
      <Stack gap="entry-room">
        <FactList label={t("staff_accounts.private.title")} items={items} />
        {failed !== undefined && (
          <FeedNotice alert start>
            {failed}
          </FeedNotice>
        )}
        {domain !== undefined && (
          <Row>
            <TextLink href={staffHref(staffEntryPath("moderation", "accounts"), emailDomainSearch(domain))}>
              {t("staff_accounts.private.same_domain", { domain })}
            </TextLink>
          </Row>
        )}
        {data !== undefined && <SignIns data={data} />}
        {data !== undefined && <SameIp data={data} />}
      </Stack>
    </Titled>
  );
}

type SignInColumn = "agent" | "when";

/** The newest sign-ins: where each came from, with what, and when. */
function SignIns({ data }: { data: AccountPrivateData }) {
  const { t } = useStaffStrings();
  const columns: RecordColumn<SignInColumn>[] = [
    { key: "agent", label: t("staff_accounts.private.column.agent"), bare: true },
    { key: "when", label: t("staff_accounts.private.column.when"), kind: "fit", beside: true },
  ];
  const rows = data.sign_ins.map((s, i): RecordRow<SignInColumn> => ({
    id: `${s.signed_in_at}-${i}`,
    head: s.ip ?? t("staff_accounts.private.unknown"),
    cells: {
      agent: s.user_agent === undefined ? t("staff_accounts.private.unknown") : <bdi>{s.user_agent}</bdi>,
      when: <RelativeTime date={s.signed_in_at} />,
    },
  }));
  return (
    <Titled title={t("staff_accounts.private.sign_ins")}>
      <RecordTable
        label={t("staff_accounts.private.sign_ins")}
        head={{ label: t("staff_accounts.private.column.ip") }}
        columns={columns}
        rows={rows}
        empty={<FeedNotice start>{t("staff_accounts.private.sign_ins.none")}</FeedNotice>}
      />
    </Titled>
  );
}

type SameColumn = "ip" | "state";

/** The other accounts that used an address of this one, each opening its own screen. */
function SameIp({ data }: { data: AccountPrivateData }) {
  const { t } = useStaffStrings();
  const columns: RecordColumn<SameColumn>[] = [
    { key: "ip", label: t("staff_accounts.private.column.ip"), bare: true },
    { key: "state", label: t("staff_accounts.search.column.state"), kind: "fit", beside: true },
  ];
  const rows = data.same_ip_accounts.flatMap((group) =>
    group.accounts.map((a): RecordRow<SameColumn> => ({
      id: `${group.ip}-${a.id}`,
      head: sentenceHandleOf(a.account),
      href: moderatedAccountPath(a.id),
      cells: { ip: group.ip, state: <AccountStateMark state={a.state} /> },
    })),
  );
  return (
    <Titled title={t("staff_accounts.private.same_ip")}>
      <RecordTable
        label={t("staff_accounts.private.same_ip")}
        head={{ label: t("staff_accounts.search.column.account") }}
        columns={columns}
        rows={rows}
        empty={<FeedNotice start>{t("staff_accounts.private.same_ip.none")}</FeedNotice>}
      />
    </Titled>
  );
}

/** That the server stopped sending email to the account's address, and why, with the way to let it through again. */
function EmailSuppression({
  account,
  say,
}: {
  account: ModeratedAccountDetail;
  say: (w: string, a?: boolean) => void;
}) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const { liftEmailSuppression } = useAccountWrites();
  const [busy, setBusy] = useState(false);
  const suppression = account.email_suppression;
  if (suppression === undefined) return null;
  return (
    <Titled title={t("staff_accounts.suppression.title")}>
      <Stack gap="entry-room">
        <FactList
          label={t("staff_accounts.suppression.title")}
          items={[
            {
              term: t("staff_accounts.suppression.kind"),
              value: t("staff_accounts.suppression.why", { kind: suppression.kind }),
            },
            { term: t("staff_accounts.suppression.since"), value: moment(suppression.state_changed_at) },
          ]}
        />
        <Row>
          <Button
            busy={busy}
            onPress={() => {
              setBusy(true);
              liftEmailSuppression(account.id).then(
                () => {
                  setBusy(false);
                  say(t("staff_accounts.suppression.done"));
                },
                (e: unknown) => {
                  setBusy(false);
                  say(refusalWords(t, e), true);
                },
              );
            }}
          >
            {t("staff_accounts.suppression.lift")}
          </Button>
        </Row>
      </Stack>
    </Titled>
  );
}

/** The login recovery: a password reset link, shown once, and removing two-factor authentication, each asked first. */
function LoginRecovery({ account, say }: { account: ModeratedAccountDetail; say: (w: string, a?: boolean) => void }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const { passwordResetLink, disableTwoFactor } = useAccountWrites();
  const handle = sentenceHandleOf(account.account);
  const [asking, setAsking] = useState<"link" | "two_factor" | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [link, setLink] = useState<PasswordResetLink | undefined>(undefined);
  const done = () => {
    setBusy(false);
    setAsking(undefined);
  };
  const confirm = () => {
    setBusy(true);
    if (asking === "link")
      passwordResetLink(account.id).then(
        (answer) => {
          done();
          setLink(answer);
          say(t("staff_accounts.recovery.link.done"));
        },
        (e: unknown) => {
          done();
          say(refusalWords(t, e), true);
        },
      );
    else
      disableTwoFactor(account.id).then(
        () => {
          done();
          say(t("staff_accounts.recovery.two_factor.done", { handle }));
        },
        (e: unknown) => {
          done();
          say(refusalWords(t, e), true);
        },
      );
  };
  return (
    <Titled title={t("staff_accounts.recovery.title")}>
      <Stack gap="entry-room">
        {link === undefined ? (
          <Row>
            <Button
              onPress={() => {
                setAsking("link");
              }}
            >
              {t("staff_accounts.recovery.link")}
            </Button>
          </Row>
        ) : (
          <Stack gap="entry-line">
            <CopyValue
              label={t("staff_accounts.recovery.link.value")}
              value={link.url}
              copy={t("staff_accounts.recovery.link.copy")}
            />
            <FeedNotice start>
              {t(
                link.login_providers_only
                  ? "staff_accounts.recovery.link.providers"
                  : "staff_accounts.recovery.link.once",
                {
                  until: moment(link.expires_at),
                },
              )}
            </FeedNotice>
          </Stack>
        )}
        {account.two_factor_enabled === true && (
          <Row>
            <Button
              onPress={() => {
                setAsking("two_factor");
              }}
            >
              {t("staff_accounts.recovery.two_factor")}
            </Button>
          </Row>
        )}
      </Stack>
      <ModalPresence>
        {asking !== undefined && (
          <ConfirmDialog
            title={t(
              asking === "link" ? "staff_accounts.recovery.link.confirm" : "staff_accounts.recovery.two_factor.confirm",
              { handle },
            )}
            cancel={t("staff_accounts.cancel")}
            confirm={t(
              asking === "link" ? "staff_accounts.recovery.link.issue" : "staff_accounts.recovery.two_factor.remove",
            )}
            busy={busy}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={confirm}
          >
            {t(
              asking === "link"
                ? "staff_accounts.recovery.link.confirm.about"
                : "staff_accounts.recovery.two_factor.confirm.about",
            )}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </Titled>
  );
}

/** Refetching a remote account's posts from its server. */
function Refetch({ account, say }: { account: ModeratedAccountDetail; say: (w: string, a?: boolean) => void }) {
  const { t } = useStaffStrings();
  const { refetchStatuses } = useAccountWrites();
  const [busy, setBusy] = useState(false);
  return (
    <Titled title={t("staff_accounts.refetch.title")}>
      <Row>
        <Button
          busy={busy}
          onPress={() => {
            setBusy(true);
            refetchStatuses(account.id).then(
              () => {
                setBusy(false);
                say(t("staff_accounts.refetch.done"));
              },
              (e: unknown) => {
                setBusy(false);
                say(refusalWords(t, e), true);
              },
            );
          }}
        >
          {t("staff_accounts.refetch.submit")}
        </Button>
      </Row>
    </Titled>
  );
}
