// SPDX-License-Identifier: AGPL-3.0-only
// The relays screen below its heading (ADR-0061, manage_settings): the form that registers and joins a relay, then
// the relays, in registration order. Each relay shows its protocol, URL, state and since when, whether it takes in
// the posts it relays, what it brought in during the last 24 hours, its actor once it accepted, and a warning while
// a domain action suspends its server. A relay is left while pending or joined, joined again once left or
// rejected, and then deleted after an alert dialog. What a change did stays above the list in a StatusMessage
// (role="status"), and a failure stays there too (role="alert").
import {
  Button,
  Card,
  Checkbox,
  Cluster,
  ConfirmDialog,
  DialogTrigger,
  EmptyState,
  Hint,
  InlineError,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
  TextField,
} from "@my/ui";
import { useRef, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { DateTime, Section, RecordTable, RequirePermission, wholeList } from "../../moderation";
import { type Relay, type RelayProtocol, relayProtocols, type RelayState, relayUrlLimit } from "../api/relays";
import {
  type RelayActions,
  relayHeading,
  relayName,
  type RelayProtocolChoice,
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

export function RelaySettings() {
  const t = useT();
  return (
    <RequirePermission permission="manage_settings">
      <Stack gap="8">
        <Section surface="card" title={t("client:admin.relays.new.title")}>
          <Card>
            <NewRelayForm />
          </Card>
        </Section>
        <Section surface="card" title={t("client:admin.relays.list.title")}>
          <RelayList />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

function NewRelayForm() {
  const t = useT();
  const form = useRelayForm();
  const choices: { id: RelayProtocolChoice; name: string }[] = [
    { id: "auto", name: t("client:admin.relays.protocol.auto") },
    ...relayProtocols.map((protocol) => ({ id: protocol, name: t(protocolLabels[protocol]) })),
  ];
  return (
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
      {form.error?.field === "form" && (
        <InlineError size="inherit" alert>
          {t("client:admin.relays.create_failed", { reason: form.error.message })}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.relays.submit")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function RelayList() {
  const t = useT();
  const tableRef = useRef<HTMLTableElement>(null);
  const relays = useRelays();
  const removed = useFocusAfterRemoval(
    tableRef,
    relays.map((relay) => relay.id),
  );
  const actions = useRelayActions(removed);
  return (
    <Stack gap="3">
      <StatusMessage>{actions.status}</StatusMessage>
      {actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.relays.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      <RecordTable
        list={wholeList(relays)}
        caption={t("client:admin.relays.list.title")}
        getId={(relay) => relay.id}
        tableRef={tableRef}
        empty={
          <EmptyState
            headingLevel={3}
            heading={t("client:admin.relays.empty")}
            description={t("client:admin.relays.empty.description")}
          />
        }
        head={
          <>
            <TableColumnHeader>{t("client:admin.relays.url")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.relays.protocol")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.relays.state")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.relays.receive.term")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.relays.taken")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.relays.dropped")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.list.column.actions")}</TableColumnHeader>
          </>
        }
      >
        {(relay) => <RelayRow relay={relay} actions={actions} />}
      </RecordTable>
    </Stack>
  );
}

function RelayRow({ relay, actions }: { relay: Relay; actions: RelayActions }) {
  const t = useT();
  const name = relayName(relay);
  const isPending = actions.pendingId === relay.id;
  const joined = isJoined(relay);
  return (
    <TableRow data-row-id={relay.id}>
      <TableRowHeader wrap="anywhere">
        <Stack gap="1">
          <Text weight="semibold" wrap="anywhere" dir="ltr">
            {relayHeading(relay)}
          </Text>
          {relay.domain_suspended && (
            <Text as="p" size="sm" tone="warning">
              {t("client:admin.relays.domain_suspended")}
            </Text>
          )}
          <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
            {relay.url}
          </Text>
          {relay.inbox_url !== relay.url && (
            <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
              {relay.inbox_url}
            </Text>
          )}
          {relay.actor_uri !== undefined && (
            <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
              {relay.actor_uri}
            </Text>
          )}
        </Stack>
      </TableRowHeader>
      <TableCell>{t(protocolLabels[relay.protocol])}</TableCell>
      <TableCell>
        <Stack gap="1">
          <span>{t(stateLabels[relay.state])}</span>
          <Text size="sm" tone="muted">
            <DateTime value={relay.state_changed_at} />
          </Text>
        </Stack>
      </TableCell>
      <TableCell>{t(relay.receive ? "client:admin.relays.receive.yes" : "client:admin.relays.receive.no")}</TableCell>
      <TableCell numeric>{t("client:admin.relays.activities", { count: relay.taken_last_day })}</TableCell>
      <TableCell numeric>
        <Text {...(relay.dropped_last_day > 0 && { tone: "warning" as const })}>
          {t("client:admin.relays.activities", { count: relay.dropped_last_day })}
        </Text>
      </TableCell>
      <TableCell>
        <Cluster gap="2" align="center">
          {/* One button that leaves or joins again, so that focus stays on it when the state changes. */}
          <Button
            size="sm"
            isPending={isPending}
            aria-label={t(joined ? "client:admin.relays.unfollow_relay" : "client:admin.relays.follow_relay", { name })}
            onPress={() => {
              actions.setJoined(relay, !joined);
            }}
          >
            {t(joined ? "client:admin.relays.unfollow" : "client:admin.relays.follow")}
          </Button>
          {!joined && <DeleteRelay relay={relay} actions={actions} isPending={isPending} />}
        </Cluster>
      </TableCell>
    </TableRow>
  );
}

/** Deleting the registration of a relay, after an alert dialog: it cannot be undone. */
function DeleteRelay({ relay, actions, isPending }: { relay: Relay; actions: RelayActions; isPending: boolean }) {
  const t = useT();
  const name = relayName(relay);
  const [isDeleting, setIsDeleting] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button
        size="sm"
        variant="danger"
        isDisabled={isPending}
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
