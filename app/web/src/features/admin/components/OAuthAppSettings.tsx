// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps screen below its heading (ADR-0090 §1, manage_oauth_apps): two pages with a local navigation
// (web-client §14). The apps page (/admin/oauth-apps; web-client/screens/admin-oauth-apps.md) lists every client,
// newest first, one page at a time, which a Select keeps to one route of registration (kept in the URL by the route,
// web-client/README.md S3); an app is disabled after an alert dialog (it revokes the tokens at once) and enabled again
// with one press, and the server's own clients cannot be disabled. An app that registered by its metadata document can
// have its host denied. The denied hosts page (/admin/oauth-apps/hosts; web-client/screens/admin-oauth-apps-hosts.md)
// lists the denied hosts, denies one in a dialog (its one primary action) and lifts a denial. What a
// change did stays above the list in a StatusMessage (role="status"), and a failure stays there too (role="alert").
import {
  Button,
  Cluster,
  ConfirmDialog,
  DialogTrigger,
  EmptyState,
  Hint,
  InlineError,
  LocalNav,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  Text,
  TextField,
} from "@my/ui";
import { useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { DateTime, Section, type RecordColumns, RecordTable, RequirePermission } from "../../moderation";
import {
  hostLimit,
  type HostDenial,
  type OAuthClient,
  type OAuthClientFilter,
  type OAuthClientRegistration,
  oauthClientRegistrations,
} from "../api/oauthApps";
import {
  appName,
  metadataHost,
  type OAuthAppActions,
  useHostDenials,
  useOAuthAppActions,
  useOAuthClients,
} from "../hooks/useOAuthApps";

const registrationLabels: Readonly<Record<OAuthClientRegistration, ClientMessageKey>> = {
  first_party: "client:admin.oauth_apps.registration.first_party",
  user: "client:admin.oauth_apps.registration.user",
  metadata_document: "client:admin.oauth_apps.registration.metadata_document",
  dynamic: "client:admin.oauth_apps.registration.dynamic",
};

/** The key of the filter's "every route" choice; a registration route is never named so. */
const anyRegistration = "all";

export type OAuthAppListProps = {
  /** The list's filter, from the URL. */
  filter: OAuthClientFilter;
  /** Shows another filter: the route puts it into the URL. */
  onFilterChange: (filter: OAuthClientFilter) => void;
};

/** A page of the OAuth apps screen. */
type OAuthAppsPage = "apps" | "hosts";

/** The local navigation under the screen's heading, `current` being the page on screen. */
function OAuthAppsNav({ current }: { current: OAuthAppsPage }) {
  const t = useT();
  return (
    <LocalNav
      label={t("client:admin.oauth_apps.pages")}
      items={[
        { href: "/admin/oauth-apps", label: t("client:admin.oauth_apps.pages.apps"), current: current === "apps" },
        {
          href: "/admin/oauth-apps/hosts",
          label: t("client:admin.oauth_apps.pages.hosts"),
          current: current === "hosts",
        },
      ]}
    />
  );
}

/** The apps page (/admin/oauth-apps): every app under the filter. */
export function OAuthAppList({ filter, onFilterChange }: OAuthAppListProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_oauth_apps">
      <Stack gap="6">
        <OAuthAppsNav current="apps" />
        <Section surface="card" title={t("client:admin.oauth_apps.list.title")}>
          <ClientsContent filter={filter} onFilterChange={onFilterChange} />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

/** The denied hosts page (/admin/oauth-apps/hosts). */
export function OAuthHostDenials() {
  const t = useT();
  return (
    <RequirePermission permission="manage_oauth_apps">
      <Stack gap="6">
        <OAuthAppsNav current="hosts" />
        <Section surface="card" title={t("client:admin.oauth_apps.hosts.title")}>
          <HostsContent />
        </Section>
      </Stack>
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

function ClientsContent({ filter, onFilterChange }: OAuthAppListProps) {
  const t = useT();
  const actions = useOAuthAppActions();
  const choices = [
    { id: anyRegistration, name: t("client:admin.oauth_apps.filter.all") },
    ...oauthClientRegistrations.map((registration) => ({
      id: registration,
      name: t(registrationLabels[registration]),
    })),
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
  const columns: RecordColumns<OAuthClient> = [
    {
      id: "app",
      header: t("client:admin.oauth_apps.column.app"),
      wrap: "anywhere",
      cell: (client) => <ClientName client={client} />,
    },
    {
      id: "registration",
      header: t("client:admin.oauth_apps.column.registration"),
      cell: (client) => (
        <Stack gap="1">
          <span>{t(registrationLabels[client.registration])}</span>
          <Text size="sm" tone="muted">
            {t(client.confidential ? "client:admin.oauth_apps.confidential" : "client:admin.oauth_apps.public")}
          </Text>
        </Stack>
      ),
    },
    {
      id: "created",
      header: t("client:admin.oauth_apps.column.created"),
      cell: (client) => <DateTime value={client.created_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (client) => <ClientActions client={client} actions={actions} />,
    },
  ];
  return (
    <RecordTable
      list={clients}
      caption={t("client:admin.oauth_apps.list.title")}
      columns={columns}
      getId={(client) => client.id}
      headingLevel={3}
      empty={
        <EmptyState
          headingLevel={3}
          heading={t("client:admin.oauth_apps.empty")}
          description={t("client:admin.oauth_apps.empty.description")}
        />
      }
    />
  );
}

/**
 * The app's name and state, its client ID, its website, who registered it, and the most it may be granted: an app
 * has no page of its own, so its facts stay under its name and the table keeps four columns (web-client §14).
 */
function ClientName({ client }: { client: OAuthClient }) {
  const t = useT();
  return (
    <Stack gap="1">
      <Cluster gap="2" align="center">
        <Text weight="semibold" wrap="anywhere" dir="auto">
          {client.name}
        </Text>
        {client.state === "disabled" && (
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
      {client.owner !== undefined && (
        <Text size="sm" tone="muted" wrap="anywhere">
          {t("client:admin.oauth_apps.owner", { handle: `\u2068@${client.owner.acct}\u2069` })}
        </Text>
      )}
      {client.owner_deleted && (
        <Text size="sm" tone="muted" wrap="anywhere">
          {t("client:admin.oauth_apps.owner.deleted")}
        </Text>
      )}
      <Text size="sm" tone="muted" wrap="anywhere">
        {t("client:admin.oauth_apps.scope", { scope: `\u2066${client.scopes.join(" ")}\u2069` })}
      </Text>
    </Stack>
  );
}

/** Enabling or disabling the app, and denying the host of its metadata document. */
function ClientActions({ client, actions }: { client: OAuthClient; actions: OAuthAppActions }) {
  const t = useT();
  const isPending = actions.pendingId === client.id;
  const host = metadataHost(client);
  return (
    <Cluster gap="2" align="center">
      {client.registration === "first_party" ? (
        <Text size="sm" tone="muted">
          {t("client:admin.oauth_apps.first_party_note")}
        </Text>
      ) : client.state === "disabled" ? (
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
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.oauth_apps.change_failed", { reason: actions.error }),
          })}
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

function HostsContent() {
  const t = useT();
  const actions = useOAuthAppActions();
  const denials = useHostDenials();
  const columns: RecordColumns<HostDenial> = [
    {
      id: "host",
      header: t("client:admin.oauth_apps.hosts.column.host"),
      wrap: "anywhere",
      cell: (denial) => (
        <Text weight="semibold" mono wrap="anywhere" dir="ltr">
          {denial.host}
        </Text>
      ),
    },
    {
      id: "denied",
      header: t("client:admin.oauth_apps.hosts.column.denied"),
      cell: (denial) => <DateTime value={denial.denied_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (denial) => (
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
      ),
    },
  ];
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
        columns={columns}
        getId={(denial) => denial.id}
        headingLevel={3}
        empty={
          <EmptyState
            headingLevel={3}
            heading={t("client:admin.oauth_apps.hosts.empty")}
            description={t("client:admin.oauth_apps.hosts.empty.description")}
          />
        }
      />
    </Stack>
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
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.oauth_apps.change_failed", { reason: actions.error }),
          })}
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
