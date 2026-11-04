// SPDX-License-Identifier: AGPL-3.0-only
// The browsers signed in to the user's account (ADR-0090, web-client/screens/settings-sessions.md): each with its browser
// and operating system, the IP address it signed in from, when it signed in and when it was last used, this one marked,
// and a button that signs it out after a confirmation. Above the list, a button signs out all the others.
import {
  Box,
  Button,
  CheckIcon,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  Heading,
  Hint,
  Link,
  Stack,
  StatusBadge,
  Text,
} from "@my/ui";
import { useRef, useState } from "react";
import { type IntlShape, useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import type { BrowserSession } from "../api/browserSessions";
import {
  type RevokeBrowserSession,
  useBrowserSessionList,
  useRevokeBrowserSession,
  useRevokeOtherBrowserSessions,
} from "../hooks/useBrowserSessions";
import { describeUserAgent } from "../lib/describeUserAgent";

export function BrowserSessions() {
  const t = useT();
  const intl = useIntl();
  return (
    <Stack container gap="6">
      <p>{t("client:settings.sessions.description")}</p>
      <SessionList />
      <Hint as="p">
        {intl.formatMessage(
          { id: "client:settings.sessions.unknown_sign_in" },
          {
            password: <Link href="/settings/password">{t("client:settings.password.title")}</Link>,
            apps: <Link href="/settings/authorized-apps">{t("client:settings.authorized_apps.title")}</Link>,
          },
        )}
      </Hint>
    </Stack>
  );
}

/** The browser of a row in words: its browser and operating system, the original string, or that it is not known. */
function useSessionName(session: BrowserSession): string {
  const t = useT();
  if (session.user_agent === undefined) return t("client:settings.sessions.name.unknown");
  const { browser, os } = describeUserAgent(session.user_agent);
  if (browser !== undefined && os !== undefined)
    return t("client:settings.sessions.name.browser_on_os", { browser, os });
  return browser ?? os ?? session.user_agent;
}

/** The moment a browser was last used, shown to the hour, or that none was recorded. */
function lastUsed(t: ReturnType<typeof useT>, intl: IntlShape, session: BrowserSession): string {
  if (session.last_used_at === undefined) return t("client:settings.sessions.last_used.never");
  return t("client:settings.sessions.last_used.at", {
    date: intl.formatDate(session.last_used_at, { year: "numeric", month: "short", day: "numeric", hour: "numeric" }),
  });
}

function SessionList() {
  const t = useT();
  const sessions = useBrowserSessionList();
  const revoke = useRevokeBrowserSession();
  const revokeOthers = useRevokeOtherBrowserSessions();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    sessions.map((session) => session.id),
  );
  const [confirming, setConfirming] = useState<BrowserSession | null>(null);
  const [confirmingOthers, setConfirmingOthers] = useState(false);
  const hasOthers = sessions.some((session) => !session.current);
  return (
    <>
      {hasOthers && (
        <div>
          <Button
            variant="danger"
            onPress={() => {
              revokeOthers.reset();
              setConfirmingOthers(true);
            }}
          >
            {t("client:settings.sessions.revoke_others")}
          </Button>
        </div>
      )}
      <Stack as="ul" ref={listRef} divided>
        {sessions.map((session) => (
          <Box as="li" key={session.id} data-row-id={session.id} container paddingY="4">
            <SessionRow
              session={session}
              revoke={revoke}
              onRevoke={() => {
                revoke.reset();
                setConfirming(session);
              }}
            />
          </Box>
        ))}
      </Stack>
      <ConfirmDialog
        isOpen={confirming !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setConfirming(null);
        }}
        title={t("client:settings.sessions.revoke.title")}
        description={t("client:settings.sessions.revoke.description")}
        confirmLabel={t("client:settings.sessions.revoke")}
        cancelLabel={t("client:settings.sessions.revoke.cancel")}
        tone="danger"
        isPending={confirming !== null && revoke.pendingId === confirming.id}
        {...(revoke.error === undefined ? {} : { errorMessage: revoke.error })}
        onConfirm={(close) => {
          if (confirming === null) return;
          revoke.run(confirming, () => {
            removed(confirming.id);
            close();
          });
        }}
      />
      <ConfirmDialog
        isOpen={confirmingOthers}
        onOpenChange={setConfirmingOthers}
        title={t("client:settings.sessions.revoke_others.title")}
        description={t("client:settings.sessions.revoke_others.description")}
        confirmLabel={t("client:settings.sessions.revoke_others")}
        cancelLabel={t("client:settings.sessions.revoke.cancel")}
        tone="danger"
        isPending={revokeOthers.isPending}
        {...(revokeOthers.error === undefined ? {} : { errorMessage: revokeOthers.error })}
        onConfirm={(close) => {
          revokeOthers.run(close);
        }}
      />
    </>
  );
}

/** A browser: its name and whether it is this one, where and when it signed in, and the button that signs it out. */
function SessionRow({
  session,
  revoke,
  onRevoke,
}: {
  session: BrowserSession;
  revoke: RevokeBrowserSession;
  onRevoke: () => void;
}) {
  const t = useT();
  const intl = useIntl();
  const name = useSessionName(session);
  const label = t("client:settings.sessions.revoke");
  return (
    <Stack gap="3">
      <Stack gap="1">
        <Heading level={2} dir="auto">
          {name}
        </Heading>
        {session.current && (
          <div>
            <StatusBadge tone="success" icon={<CheckIcon />} label={t("client:settings.sessions.current")} />
          </div>
        )}
      </Stack>
      <DescriptionList columns={1}>
        <DescriptionItem term={t("client:settings.sessions.ip")}>
          {session.ip === undefined ? (
            t("client:settings.sessions.ip.unknown")
          ) : (
            <Text as="span" wrap="anywhere" mono dir="ltr">
              {session.ip}
            </Text>
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.sessions.signed_in_at")}>
          {intl.formatDate(session.signed_in_at, { dateStyle: "medium", timeStyle: "short" })}
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.sessions.last_used")}>{lastUsed(t, intl, session)}</DescriptionItem>
      </DescriptionList>
      {!session.current && (
        <div>
          <Button
            size="sm"
            variant="danger"
            isPending={revoke.pendingId === session.id}
            aria-label={t("client:settings.list.action", { action: label, name })}
            onPress={onRevoke}
          >
            {label}
          </Button>
        </div>
      )}
    </Stack>
  );
}
