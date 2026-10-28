// SPDX-License-Identifier: AGPL-3.0-only
// The muted domains page below its heading (FR-304): what muting a domain does, the form that mutes one, and the muted
// domains in the order of the domains, each with its button that unmutes it. The form follows the browser's constraint
// validation (React Aria's native validation); muting is undone as easily, so neither asks first. What muting did stays
// on the page in a StatusMessage (role="status") under the form; unmuting is announced and moves focus to the next row.
import {
  Box,
  Button,
  Cluster,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  Stack,
  StatusMessage,
  Text,
  TextField,
} from "@my/ui";
import { useId, useRef } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { domainToUnicode } from "../../../lib/idn";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { domainLimit, type DomainMute } from "../api/domainMutes";
import { type UnmuteDomain, useDomainMuteList, useMuteDomainForm, useUnmuteDomain } from "../hooks/useDomainMutes";

export function DomainMutes() {
  const t = useT();
  const formHeadingId = useId();
  const listHeadingId = useId();
  return (
    <Stack gap="8">
      <p>{t("client:settings.domain_mutes.description")}</p>
      <Stack as="section" aria-labelledby={formHeadingId} gap="3">
        <Heading level={2} id={formHeadingId}>
          {t("client:settings.domain_mutes.new.title")}
        </Heading>
        <MuteDomainForm />
      </Stack>
      <Stack as="section" aria-labelledby={listHeadingId} gap="3">
        <Heading level={2} id={listHeadingId}>
          {t("client:settings.domain_mutes.list.title")}
        </Heading>
        <DomainMuteList />
      </Stack>
    </Stack>
  );
}

function MuteDomainForm() {
  const t = useT();
  const form = useMuteDomainForm();
  return (
    <Stack as="form" onSubmit={form.submit} gap="4">
      <TextField
        label={t("client:settings.domain_mutes.domain")}
        description={t("client:settings.domain_mutes.domain.description")}
        value={form.domain}
        onChange={form.setDomain}
        isRequired
        maxLength={domainLimit}
        autoComplete="off"
        autoCorrect="off"
        spellCheck="false"
        inputMode="url"
        errorMessage={t("client:settings.domain_mutes.domain.required")}
      />
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:settings.domain_mutes.mute_failed", {
            reason: form.error,
          })}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:settings.domain_mutes.mute")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function DomainMuteList() {
  const t = useT();
  const list = useDomainMuteList();
  const unmute = useUnmuteDomain();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((mute) => mute.domain),
  );
  if (list.items.length === 0) return <EmptyState heading={t("client:settings.domain_mutes.empty")} />;
  return (
    <Stack gap="4">
      <Stack as="ul" ref={listRef} divided>
        {list.items.map((mute) => (
          <Box as="li" key={mute.domain} data-row-id={mute.domain} container paddingY="3">
            <DomainMuteRow
              mute={mute}
              unmute={unmute}
              onUnmuted={() => {
                removed(mute.domain);
              }}
            />
          </Box>
        ))}
      </Stack>
      {list.hasMore && (
        <div>
          <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
            {t("client:settings.list.more")}
          </Button>
        </div>
      )}
    </Stack>
  );
}

type DomainMuteRowProps = { mute: DomainMute; unmute: UnmuteDomain; onUnmuted: () => void };

/** A muted domain: its U-labels (and its A-labels when they differ) and when it was muted, with the button that unmutes it. */
function DomainMuteRow({ mute, unmute, onUnmuted }: DomainMuteRowProps) {
  const t = useT();
  const intl = useIntl();
  const unicode = domainToUnicode(mute.domain);
  const label = t("client:settings.domain_mutes.unmute");
  return (
    <Stack gap="3" rowAt="md">
      <Stack as="p" shrink grow>
        {/* A domain reads left to right in every locale. */}
        <Text dir="ltr" wrap="truncate" align="start" weight="semibold">
          {unicode}
        </Text>
        {unicode !== mute.domain && (
          <Text dir="ltr" wrap="truncate" align="start" size="sm" tone="muted">
            {mute.domain}
          </Text>
        )}
        <Hint as="span">
          {t("client:settings.domain_mutes.since", {
            date: intl.formatDate(mute.created_at, { dateStyle: "medium", timeStyle: "short" }),
          })}
        </Hint>
      </Stack>
      <Cluster gap="2">
        <Button
          size="sm"
          isPending={unmute.pendingDomain === mute.domain}
          aria-label={t("client:settings.list.action", { action: label, name: unicode })}
          onPress={() => {
            unmute.run(mute.domain, onUnmuted);
          }}
        >
          {label}
        </Button>
      </Cluster>
    </Stack>
  );
}
