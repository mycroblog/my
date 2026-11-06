// SPDX-License-Identifier: AGPL-3.0-only
// The relays screen below its heading (ADR-0006, manage_settings), in three pages (web-client/14-admin.md 画面の分け方):
// - the list (`RelaySettings`): the relays in registration order, each with its host name linking to its page, its
//   state and since when, and the button that leaves it or joins it again; above it "Join a relay" opens the form.
// - the form (`NewRelay`): the URL, the protocol and whether to take in its posts; joining opens the relay's page.
// - the page of one relay (`RelayDetail`): its facts (protocol, URLs, actor, receiving, the last 24 hours), a warning
//   while a domain action suspends its server, leaving and joining again, and deleting it after an alert dialog,
//   which goes back to the list with a toast.
// What leaving or joining did stays next to the buttons in a StatusMessage (role="status"), and a failure stays there
// too (role="alert").
import {
  Button,
  ButtonLink,
  Card,
  Checkbox,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  EmptyState,
  FormActionBar,
  Hint,
  InlineError,
  Link,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  Text,
  TextField,
  useToast,
} from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { DateTime, Section, type RecordColumns, RecordTable, RequirePermission, wholeList } from "../../moderation";
import { type Relay, type RelayProtocol, relayProtocols, type RelayState, relayUrlLimit } from "../api/relays";
import {
  type RelayActions,
  relayHeading,
  relayName,
  type RelayProtocolChoice,
  useRelay,
  useRelayActions,
  useRelayForm,
  useRelays,
} from "../hooks/useRelays";

const protocolLabels: Readonly<Record<RelayProtocol, ClientMessageKey>> = {
  mastodon: "client:admin.relays.protocol.mastodon",
  litepub: "client:admin.relays.protocol.litepub",
};

const stateLabels: Readonly<Record<RelayState, ClientMessageKey>> = {
  pending: "client:admin.relays.state.pending",
  accepted: "client:admin.relays.state.accepted",
  rejected: "client:admin.relays.state.rejected",
  left: "client:admin.relays.state.left",
};

/** Whether a relay is a member or asking to be: it can be left, and not joined again or deleted. */
function isJoined(relay: Relay): boolean {
  return relay.state === "pending" || relay.state === "accepted";
}

/** The list of the relays, with the link to the form that joins one. */
export function RelaySettings() {
  const t = useT();
  return (
    <RequirePermission permission="manage_settings">
      <Stack gap="6">
        <div>
          <ButtonLink href="/admin/relays/new" variant="primary">
            {t("client:admin.relays.new.title")}
          </ButtonLink>
        </div>
        <Section surface="card" title={t("client:admin.relays.list.title")}>
          <RelayList />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

function RelayList() {
  const t = useT();
  const relays = useRelays();
  const actions = useRelayActions();
  const columns: RecordColumns<Relay> = [
    {
      id: "relay",
      header: t("client:admin.relays.relay"),
      wrap: "anywhere",
      cell: (relay) => (
        <Stack gap="1">
          <Link href={`/admin/relays/${encodeURIComponent(relay.id)}`} wrap="anywhere">
            <span dir="ltr">{relayHeading(relay)}</span>
          </Link>
          {relay.domain_suspended && (
            <Text size="sm" tone="warning">
              {t("client:admin.relays.domain_suspended.short")}
            </Text>
          )}
        </Stack>
      ),
    },
    { id: "state", header: t("client:admin.relays.state"), cell: (relay) => t(stateLabels[relay.state]) },
    {
      id: "state_changed_at",
      header: t("client:admin.relays.state_changed_at"),
      cell: (relay) => <DateTime value={relay.state_changed_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (relay) => <MembershipButton relay={relay} actions={actions} />,
    },
  ];
  return (
    <Stack gap="3">
      <ActionsResult actions={actions} />
      <RecordTable
        list={wholeList(relays)}
        caption={t("client:admin.relays.list.title")}
        columns={columns}
        getId={(relay) => relay.id}
        headingLevel={3}
        empty={
          <EmptyState
            headingLevel={3}
            heading={t("client:admin.relays.empty")}
            description={t("client:admin.relays.empty.description")}
          />
        }
      />
    </Stack>
  );
}

/** What the last change did, and why it failed. */
function ActionsResult({ actions }: { actions: RelayActions }) {
  const t = useT();
  return (
    <>
      <StatusMessage>{actions.status}</StatusMessage>
      {actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.relays.change_failed", { reason: actions.error })}
        </InlineError>
      )}
    </>
  );
}

/** One button that leaves or joins again, so that focus stays on it when the state changes. */
function MembershipButton({ relay, actions }: { relay: Relay; actions: RelayActions }) {
  const t = useT();
  const name = relayName(relay);
  const joined = isJoined(relay);
  return (
    <Button
      size="sm"
      isPending={actions.pendingId === relay.id}
      aria-label={t(joined ? "client:admin.relays.unfollow_relay" : "client:admin.relays.follow_relay", { name })}
      onPress={() => {
        actions.setJoined(relay, !joined);
      }}
    >
      {t(joined ? "client:admin.relays.unfollow" : "client:admin.relays.follow")}
    </Button>
  );
}

/** The form that registers and joins a relay; joining opens the relay's page. */
export function NewRelay() {
  return (
    <RequirePermission permission="manage_settings">
      <NewRelayForm />
    </RequirePermission>
  );
}

function NewRelayForm() {
  const t = useT();
  const navigate = useNavigate();
  const form = useRelayForm((relay) => {
    void navigate({ to: "/admin/relays/$id", params: { id: relay.id } });
  });
  const choices: { id: RelayProtocolChoice; name: string }[] = [
    { id: "auto", name: t("client:admin.relays.protocol.auto") },
    ...relayProtocols.map((protocol) => ({ id: protocol, name: t(protocolLabels[protocol]) })),
  ];
  return (
    <Card>
      <Stack as="form" gap="6" onSubmit={form.submit}>
        <Hint size="inherit">{t("client:admin.relays.new.description")}</Hint>
        <TextField
          type="url"
          label={t("client:admin.relays.url")}
          description={t("client:admin.relays.url.description")}
          value={form.draft.url}
          onChange={(url) => {
            form.change({ url });
          }}
          isRequired
          maxLength={relayUrlLimit}
          autoComplete="off"
          spellCheck="false"
          {...(form.error?.field === "url" && { isInvalid: true, errorMessage: form.error.message })}
        />
        <Select
          label={t("client:admin.relays.protocol")}
          description={t("client:admin.relays.protocol.description")}
          value={form.draft.protocol}
          onChange={(key) => {
            const choice = choices.find((c) => c.id === key);
            if (choice !== undefined) form.change({ protocol: choice.id });
          }}
          items={choices}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
        <Checkbox
          isSelected={form.draft.receive}
          onChange={(receive) => {
            form.change({ receive });
          }}
          description={t("client:admin.relays.receive.description")}
        >
          {t("client:admin.relays.receive")}
        </Checkbox>
        <FormActionBar
          result={
            form.error?.field === "form"
              ? { tone: "danger", message: t("client:admin.relays.create_failed", { reason: form.error.message }) }
              : undefined
          }
        >
          <Button type="submit" variant="primary" isPending={form.isPending}>
            {t("client:admin.relays.submit")}
          </Button>
        </FormActionBar>
      </Stack>
    </Card>
  );
}

/** The page of one relay: its facts, leaving or joining it again, and deleting it. */
export function RelayDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_settings">
      <RelayDetailBody id={id} />
    </RequirePermission>
  );
}

function RelayDetailBody({ id }: { id: string }) {
  const t = useT();
  const relay = useRelay(id);
  if (relay === undefined) {
    return (
      <EmptyState
        heading={t("client:admin.relays.not_found")}
        description={t("client:admin.relays.not_found.description")}
      />
    );
  }
  return <RelayPage relay={relay} />;
}

function RelayPage({ relay }: { relay: Relay }) {
  const t = useT();
  const navigate = useNavigate();
  const toast = useToast();
  const actions = useRelayActions((deleted) => {
    void navigate({ to: "/admin/relays" });
    // The toast is announced too.
    toast.show({ message: t("client:admin.relays.deleted", { name: relayName(deleted) }), tone: "success" });
  });
  const joined = isJoined(relay);
  return (
    <Stack gap="8">
      <Section surface="card" title={<span dir="ltr">{relayHeading(relay)}</span>}>
        <Card>
          <Stack gap="4">
            {relay.domain_suspended && (
              <Text as="p" tone="warning">
                {t("client:admin.relays.domain_suspended")}
              </Text>
            )}
            <RelayFacts relay={relay} />
          </Stack>
        </Card>
      </Section>
      <Section surface="card" title={t("client:admin.relays.membership.title")}>
        <Card>
          <Stack gap="4">
            <Hint size="inherit">
              {t(joined ? "client:admin.relays.membership.joined" : "client:admin.relays.membership.left")}
            </Hint>
            <ActionsResult actions={actions} />
            <Cluster gap="2" align="center">
              <MembershipButton relay={relay} actions={actions} />
              {!joined && <DeleteRelay relay={relay} actions={actions} />}
            </Cluster>
          </Stack>
        </Card>
      </Section>
    </Stack>
  );
}

/** The facts of a relay: how it is joined, its addresses, whether it is received, and the last 24 hours. */
function RelayFacts({ relay }: { relay: Relay }) {
  const t = useT();
  return (
    <DescriptionList columns={2}>
      <DescriptionItem term={t("client:admin.relays.state")}>
        <Cluster as="span" gap="1" rowGap="0">
          <span>{t(stateLabels[relay.state])}</span>
          <DateTime value={relay.state_changed_at} />
        </Cluster>
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.relays.protocol")}>{t(protocolLabels[relay.protocol])}</DescriptionItem>
      <DescriptionItem term={t("client:admin.relays.url")}>
        <Text mono wrap="anywhere" dir="ltr">
          {relay.url}
        </Text>
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.relays.inbox")}>
        <Text mono wrap="anywhere" dir="ltr">
          {relay.inbox_url}
        </Text>
      </DescriptionItem>
      {relay.actor_uri !== undefined && (
        <DescriptionItem term={t("client:admin.relays.actor")}>
          <Text mono wrap="anywhere" dir="ltr">
            {relay.actor_uri}
          </Text>
        </DescriptionItem>
      )}
      <DescriptionItem term={t("client:admin.relays.receive.term")}>
        {t(relay.receive ? "client:admin.relays.receive.yes" : "client:admin.relays.receive.no")}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.relays.taken")}>
        {t("client:admin.relays.activities", { count: relay.taken_last_day })}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.relays.dropped")}>
        <Text {...(relay.dropped_last_day > 0 && { tone: "warning" as const })}>
          {t("client:admin.relays.activities", { count: relay.dropped_last_day })}
        </Text>
      </DescriptionItem>
    </DescriptionList>
  );
}

/** Deleting the registration of a relay, after an alert dialog: it cannot be undone. */
function DeleteRelay({ relay, actions }: { relay: Relay; actions: RelayActions }) {
  const t = useT();
  const name = relayName(relay);
  const [isDeleting, setIsDeleting] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button
        size="sm"
        variant="danger"
        isDisabled={actions.pendingId === relay.id}
        aria-label={t("client:admin.relays.delete_relay", { name })}
      >
        {t("client:admin.relays.delete")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.relays.delete.title", { name })}
        description={t("client:admin.relays.delete.description")}
        confirmLabel={t("client:admin.relays.delete")}
        cancelLabel={t("client:admin.relays.cancel")}
        isPending={isDeleting}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.relays.change_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsDeleting(true);
          setFailed(false);
          void actions.remove(relay).then((deleted) => {
            setIsDeleting(false);
            if (deleted) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}
