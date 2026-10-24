// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps screen below its heading (ADR-0090 §1, manage_oauth_apps): two tabs kept in the URL by the route
// (web-client/README.md S3). The first lists every client, newest first, which a Select keeps to one route of
// registration; an app is disabled after an alert dialog (it revokes the tokens at once) and enabled again with
// one press, and the server's own clients cannot be disabled. An app that registered by its metadata document can
// have its host denied. The second tab lists the denied hosts, denies one in a dialog and lifts a denial. What a
// change did stays above the list in a StatusMessage (role="status"), and a failure stays there too (role="alert").
import { Button, Cluster, ConfirmDialog, DialogTrigger, EmptyState, Hint, InlineError, Select, SelectItem, Stack, StatusMessage, Tab, TableCell, TableColumnHeader, TableRow, TableRowHeader, TabList, TabPanel, Tabs, Text, TextField } from "@my/ui";
import { useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { DateTime, DetailSection, RecordTable, RequirePermission } from "../../moderation";
import { hostLimit, type HostDenial, type OAuthClient, type OAuthClientFilter, type OAuthClientRegistration, oauthClientRegistrations } from "../api/oauthApps";
import { appName, metadataHost, type OAuthAppActions, useHostDenials, useOAuthAppActions, useOAuthClients } from "../hooks/useOAuthApps";

/** The tabs of the screen: the apps, and the denied hosts. */
export type OAuthAppTab = "apps" | "hosts";
const screenTabs = ["apps", "hosts"] as const satisfies readonly OAuthAppTab[];

function isScreenTab(key: unknown): key is OAuthAppTab {
  return screenTabs.some((tab) => tab === key);
}

const registrationLabels: Readonly<Record<OAuthClientRegistration, ClientMessageKey>> = {
  first_party: "client:admin.oauth_apps.registration.first_party",
  user: "client:admin.oauth_apps.registration.user",
  metadata_document: "client:admin.oauth_apps.registration.metadata_document",
  dynamic: "client:admin.oauth_apps.registration.dynamic",
};

/** The key of the filter's "every route" choice; a registration route is never named so. */
const anyRegistration = "all";

export type OAuthAppSettingsProps = {
  /** The tab of the screen, from the URL. */
  tab: OAuthAppTab;
  /** Shows another tab: the route puts it into the URL. */
  onTabChange: (tab: OAuthAppTab) => void;
  /** The list's filter, from the URL. */
  filter: OAuthClientFilter;
  /** Shows another filter: the route puts it into the URL. */
  onFilterChange: (filter: OAuthClientFilter) => void;
};

export function OAuthAppSettings({ tab, onTabChange, filter, onFilterChange }: OAuthAppSettingsProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_oauth_apps">
      <Tabs
        selectedKey={tab}
        onSelectionChange={(key) => {
          if (isScreenTab(key)) onTabChange(key);
        }}
      >
        <TabList aria-label={t("client:admin.oauth_apps.tabs.label")}>
          <Tab id="apps">{t("client:admin.oauth_apps.tabs.apps")}</Tab>
          <Tab id="hosts">{t("client:admin.oauth_apps.tabs.hosts")}</Tab>
        </TabList>
        <TabPanel id="apps" gap="4" offset="4">
          <DetailSection title={t("client:admin.oauth_apps.list.title")}>
            <ClientsTab filter={filter} onFilterChange={onFilterChange} />
          </DetailSection>
        </TabPanel>
        <TabPanel id="hosts" gap="4" offset="4">
          <DetailSection title={t("client:admin.oauth_apps.hosts.title")}>
            <HostsTab />
          </DetailSection>
        </TabPanel>
      </Tabs>
    </RequirePermission>
  );
}

function ActionResult({ actions }: { actions: OAuthAppActions }) {
  const t = useT();
  return (
    <>
      <StatusMessage>{actions.status}</StatusMessage>
      {actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.oauth_apps.change_failed", { reason: actions.error })}
        </InlineError>
      )}
    </>
  );
}

function ClientsTab({ filter, onFilterChange }: Pick<OAuthAppSettingsProps, "filter" | "onFilterChange">) {
  const t = useT();
  const actions = useOAuthAppActions();
  const choices = [
    { id: anyRegistration, name: t("client:admin.oauth_apps.filter.all") },
    ...oauthClientRegistrations.map((registration) => ({ id: registration, name: t(registrationLabels[registration]) })),
  ];
  return (
    <Stack gap="4">
      <Hint size="inherit">{t("client:admin.oauth_apps.list.description")}</Hint>
      <div>
        <Select
          label={t("client:admin.oauth_apps.filter.label")}
          value={filter.registration ?? anyRegistration}
          onChange={(key) => {
            onFilterChange({ registration: oauthClientRegistrations.find((registration) => registration === key) });
          }}
          items={choices}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
      </div>
      <ActionResult actions={actions} />
      {/* Keyed by the filter, so that another list starts at its first page. */}
      <ClientList key={filter.registration ?? anyRegistration} filter={filter} actions={actions} />
    </Stack>
  );
}

function ClientList({ filter, actions }: { filter: OAuthClientFilter; actions: OAuthAppActions }) {
  const t = useT();
  const clients = useOAuthClients(filter);
  return (
    <RecordTable
      list={clients}
      caption={t("client:admin.oauth_apps.list.title")}
      getId={(client) => client.id}
      empty={<EmptyState headingLevel={3} heading={t("client:admin.oauth_apps.empty")} description={t("client:admin.oauth_apps.empty.description")} />}
      head={
        <>
          <TableColumnHeader>{t("client:admin.oauth_apps.column.app")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.oauth_apps.column.registration")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.oauth_apps.column.scope")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.oauth_apps.column.created")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.list.column.actions")}</TableColumnHeader>
        </>
      }
    >
      {(client) => <ClientRow client={client} actions={actions} />}
    </RecordTable>
  );
}

function ClientRow({ client, actions }: { client: OAuthClient; actions: OAuthAppActions }) {
  const t = useT();
  const isPending = actions.pendingId === client.id;
  const host = metadataHost(client);
  return (
    <TableRow data-row-id={client.id}>
      <TableRowHeader wrap="anywhere">
        <Stack gap="1">
          <Cluster gap="2" align="center">
            <Text weight="semibold" wrap="anywhere" dir="auto">
              {client.name}
            </Text>
            {client.disabled && (
              <Text size="sm" tone="danger" weight="semibold">
                {t("client:admin.oauth_apps.disabled")}
              </Text>
            )}
          </Cluster>
          <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
            {client.client_id}
          </Text>
          {client.website !== undefined && (
            <Text size="sm" tone="muted" wrap="anywhere" dir="ltr">
              {client.website}
            </Text>
          )}
          {client.owner_id !== undefined && (
            <Text size="sm" tone="muted" wrap="anywhere">
              {t("client:admin.oauth_apps.owner", { id: client.owner_id })}
            </Text>
          )}
        </Stack>
      </TableRowHeader>
      <TableCell>
        <Stack gap="1">
          <span>{t(registrationLabels[client.registration])}</span>
          <Text size="sm" tone="muted">
            {t(client.confidential ? "client:admin.oauth_apps.confidential" : "client:admin.oauth_apps.public")}
          </Text>
        </Stack>
      </TableCell>
      <TableCell wrap="anywhere">
        <Text size="sm" mono dir="ltr">
          {client.scope}
        </Text>
      </TableCell>
      <TableCell>
        <DateTime value={client.created_at} />
      </TableCell>
      <TableCell>
        <Cluster gap="2" align="center">
          {client.registration === "first_party" ? (
            <Text size="sm" tone="muted">
              {t("client:admin.oauth_apps.first_party_note")}
            </Text>
          ) : client.disabled ? (
            <Button
              size="sm"
              isPending={isPending}
              aria-label={t("client:admin.oauth_apps.enable_app", { name: appName(client) })}
              onPress={() => {
                actions.setDisabled(client, false);
              }}
            >
              {t("client:admin.oauth_apps.enable")}
            </Button>
          ) : (
            <DisableApp client={client} actions={actions} />
          )}
          {host !== undefined && <DenyAppHost host={host} actions={actions} />}
        </Cluster>
      </TableCell>
    </TableRow>
  );
}

/** Disabling an app, after an alert dialog: its tokens are revoked at once, and they stay revoked when it is enabled again. */
function DisableApp({ client, actions }: { client: OAuthClient; actions: OAuthAppActions }) {
  const t = useT();
  const name = appName(client);
  return (
    <DialogTrigger>
      <Button size="sm" variant="danger" aria-label={t("client:admin.oauth_apps.disable_app", { name })}>
        {t("client:admin.oauth_apps.disable")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.oauth_apps.disable.title", { name })}
        description={t("client:admin.oauth_apps.disable.description")}
        confirmLabel={t("client:admin.oauth_apps.disable")}
        cancelLabel={t("client:admin.oauth_apps.cancel")}
        onConfirm={(close) => {
          actions.setDisabled(client, true);
          close();
        }}
      />
    </DialogTrigger>
  );
}

/** Denying the host of an app's metadata document, after an alert dialog. */
function DenyAppHost({ host, actions }: { host: string; actions: OAuthAppActions }) {
  const t = useT();
  const [isDenying, setIsDenying] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button size="sm" variant="danger" aria-label={t("client:admin.oauth_apps.deny_host_of", { host })}>
        {t("client:admin.oauth_apps.deny_host")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.oauth_apps.hosts.deny.title", { host })}
        description={t("client:admin.oauth_apps.hosts.deny.description")}
        confirmLabel={t("client:admin.oauth_apps.hosts.deny")}
        cancelLabel={t("client:admin.oauth_apps.cancel")}
        isPending={isDenying}
        {...(failed && actions.error !== undefined && { errorMessage: t("client:admin.oauth_apps.change_failed", { reason: actions.error }) })}
        onConfirm={(close) => {
          setIsDenying(true);
          setFailed(false);
          void actions.denyHost(host).then((denied) => {
            setIsDenying(false);
            if (denied) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}

function HostsTab() {
  const t = useT();
  const actions = useOAuthAppActions();
  const denials = useHostDenials();
  return (
    <Stack gap="4">
      <Hint size="inherit">{t("client:admin.oauth_apps.hosts.description")}</Hint>
      <div>
        <DenyHostByName actions={actions} />
      </div>
      <ActionResult actions={actions} />
      <RecordTable
        list={denials}
        caption={t("client:admin.oauth_apps.hosts.title")}
        getId={(denial) => denial.id}
        empty={<EmptyState headingLevel={3} heading={t("client:admin.oauth_apps.hosts.empty")} description={t("client:admin.oauth_apps.hosts.empty.description")} />}
        head={
          <>
            <TableColumnHeader>{t("client:admin.oauth_apps.hosts.column.host")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.oauth_apps.hosts.column.denied")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.list.column.actions")}</TableColumnHeader>
          </>
        }
      >
        {(denial) => <DenialRow denial={denial} actions={actions} />}
      </RecordTable>
    </Stack>
  );
}

function DenialRow({ denial, actions }: { denial: HostDenial; actions: OAuthAppActions }) {
  const t = useT();
  return (
    <TableRow data-row-id={denial.id}>
      <TableRowHeader wrap="anywhere">
        <Text weight="semibold" mono wrap="anywhere" dir="ltr">
          {denial.host}
        </Text>
      </TableRowHeader>
      <TableCell>
        <DateTime value={denial.denied_at} />
      </TableCell>
      <TableCell>
        <Button
          size="sm"
          isPending={actions.pendingId === denial.host}
          aria-label={t("client:admin.oauth_apps.hosts.allow_host", { host: denial.host })}
          onPress={() => {
            actions.allowHost(denial.host);
          }}
        >
          {t("client:admin.oauth_apps.hosts.allow")}
        </Button>
      </TableCell>
    </TableRow>
  );
}

/** Denying a host by typing its name, in a dialog: the host name field is the dialog's first control. */
function DenyHostByName({ actions }: { actions: OAuthAppActions }) {
  const t = useT();
  const [host, setHost] = useState("");
  const [isDenying, setIsDenying] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger
      onOpenChange={(open) => {
        if (open) {
          setHost("");
          setFailed(false);
        }
      }}
    >
      <Button variant="primary">{t("client:admin.oauth_apps.hosts.deny_new")}</Button>
      <ConfirmDialog
        title={t("client:admin.oauth_apps.hosts.deny_new.title")}
        description={t("client:admin.oauth_apps.hosts.deny.description")}
        confirmLabel={t("client:admin.oauth_apps.hosts.deny")}
        cancelLabel={t("client:admin.oauth_apps.cancel")}
        isPending={isDenying}
        {...(failed && actions.error !== undefined && { errorMessage: t("client:admin.oauth_apps.change_failed", { reason: actions.error }) })}
        onConfirm={(close) => {
          setIsDenying(true);
          setFailed(false);
          void actions.denyHost(host.trim()).then((denied) => {
            setIsDenying(false);
            if (denied) close();
            else setFailed(true);
          });
        }}
      >
        <TextField
          label={t("client:admin.oauth_apps.hosts.host")}
          description={t("client:admin.oauth_apps.hosts.host.description")}
          value={host}
          onChange={setHost}
          isRequired
          maxLength={hostLimit}
          autoComplete="off"
          spellCheck="false"
        />
      </ConfirmDialog>
    </DialogTrigger>
  );
}
