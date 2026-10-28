// SPDX-License-Identifier: AGPL-3.0-only
// The apps the user authorized (ADR-0090 §5): each with where it comes from (the server's own client, the host of its
// Client ID Metadata Document, the user who registered it, or self-declared), the scopes granted, when it was first
// authorized and when it was last used (exact to the hour), and a button that revokes it after a confirmation. The name
// is what the app says about itself; only the origin is what the server knows, so a third-party app says it is not
// verified. Revoking the web client signs the user out of every browser, which the confirmation says.
import {
  Box,
  Button,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Heading,
  Hint,
  Stack,
  Text,
} from "@my/ui";
import { useRef, useState } from "react";
import { type IntlShape, useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import type { AuthorizedApp } from "../api/authorizedApps";
import { type RevokeApp, useAuthorizedAppList, useRevokeApp } from "../hooks/useAuthorizedApps";

export function AuthorizedApps() {
  const t = useT();
  return (
    <Stack container gap="6">
      <p>{t("client:settings.authorized_apps.description")}</p>
      <AppList />
    </Stack>
  );
}

/** The host of the app's `client_id` when it is a URL (a Client ID Metadata Document), else the `client_id`. */
function hostOf(clientId: string): string {
  try {
    return new URL(clientId).host;
  } catch {
    return clientId;
  }
}

/** Where the app comes from, in words. */
function useOrigin(app: AuthorizedApp): string {
  const t = useT();
  switch (app.registration) {
    case "first_party":
      return t("client:settings.authorized_apps.origin.first_party");
    case "user":
      return t("client:settings.authorized_apps.origin.user", { username: app.registered_by ?? "" });
    case "metadata_document":
      return t("client:settings.authorized_apps.origin.metadata_document", { host: hostOf(app.client_id) });
    case "dynamic":
      return t("client:settings.authorized_apps.origin.dynamic");
  }
}

/** The moment an app was last used, shown to the hour, or that none was recorded. */
function lastUsed(t: ReturnType<typeof useT>, intl: IntlShape, app: AuthorizedApp): string {
  if (app.last_used_at === undefined) return t("client:settings.authorized_apps.last_used.never");
  return t("client:settings.authorized_apps.last_used.at", {
    date: intl.formatDate(app.last_used_at, { year: "numeric", month: "short", day: "numeric", hour: "numeric" }),
  });
}

function AppList() {
  const t = useT();
  const apps = useAuthorizedAppList();
  const revoke = useRevokeApp();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    apps.map((app) => app.id),
  );
  const [confirming, setConfirming] = useState<AuthorizedApp | null>(null);
  if (apps.length === 0) return <EmptyState headingLevel={2} heading={t("client:settings.authorized_apps.empty")} />;
  return (
    <>
      <Stack as="ul" ref={listRef} divided>
        {apps.map((app) => (
          <Box as="li" key={app.id} data-row-id={app.id} container paddingY="4">
            <AppRow
              app={app}
              revoke={revoke}
              onRevoke={() => {
                revoke.reset();
                setConfirming(app);
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
        title={t("client:settings.authorized_apps.revoke.title", { name: confirming?.name ?? "" })}
        description={
          confirming?.first_party === true
            ? t("client:settings.authorized_apps.revoke.description_first_party")
            : t("client:settings.authorized_apps.revoke.description")
        }
        confirmLabel={t("client:settings.authorized_apps.revoke")}
        cancelLabel={t("client:settings.authorized_apps.revoke.cancel")}
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
    </>
  );
}

/** An app: its name and origin, what it may do and when, with the button that revokes it. */
function AppRow({ app, revoke, onRevoke }: { app: AuthorizedApp; revoke: RevokeApp; onRevoke: () => void }) {
  const t = useT();
  const intl = useIntl();
  const origin = useOrigin(app);
  const label = t("client:settings.authorized_apps.revoke");
  return (
    <Stack gap="3">
      <Stack gap="1">
        <Heading level={3} dir="auto">
          {app.name}
        </Heading>
        <Hint as="p">{origin}</Hint>
        {!app.first_party && <Hint as="p">{t("client:settings.authorized_apps.unverified")}</Hint>}
      </Stack>
      <DescriptionList columns={1}>
        <DescriptionItem term={t("client:settings.authorized_apps.scope")}>
          <Text as="span" wrap="anywhere" mono dir="ltr">
            {app.scope}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.authorized_apps.authorized_at")}>
          {intl.formatDate(app.authorized_at, { dateStyle: "medium", timeStyle: "short" })}
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.authorized_apps.last_used")}>
          {lastUsed(t, intl, app)}
        </DescriptionItem>
      </DescriptionList>
      <div>
        <Button
          size="sm"
          variant="danger"
          isPending={revoke.pendingId === app.id}
          aria-label={t("client:settings.list.action", { action: label, name: app.name })}
          onPress={onRevoke}
        >
          {label}
        </Button>
      </div>
    </Stack>
  );
}
