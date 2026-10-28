// SPDX-License-Identifier: AGPL-3.0-only
// The muted domains page below its heading (FR-304; web-client/screens/settings-domain-mutes.md): the list-and-add part
// (`ListSection`) with the muted domains in the order of the domains, each with its button that unmutes it, and the
// button that opens the form that mutes one. The form follows the browser's constraint validation (React Aria's native
// validation); muting is undone as easily, so neither asks first. What muting did, and a failed unmute, show above the
// list; unmuting is announced and moves focus to the next row (web-client/16-look-states-and-wording.md G24).
import { Button, Cluster, FormActionBar, Hint, ListSection, Stack, Text, TextField } from "@my/ui";
import { useRef } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { domainToUnicode } from "../../../lib/idn";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { domainLimit, type DomainMute } from "../api/domainMutes";
import {
  type MuteDomainForm,
  type UnmuteDomain,
  useDomainMuteList,
  useMuteDomainForm,
  useUnmuteDomain,
} from "../hooks/useDomainMutes";

export function DomainMutes() {
  const t = useT();
  const list = useDomainMuteList();
  const form = useMuteDomainForm();
  const unmute = useUnmuteDomain();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((mute) => mute.domain),
  );
  const count = list.items.length;
  let result: { tone: "success" | "danger"; message: string } | undefined;
  if (unmute.error !== undefined) result = { tone: "danger", message: unmute.error };
  else if (form.status !== undefined) result = { tone: "success", message: form.status };
  return (
    <ListSection
      title={t("client:settings.domain_mutes.list.title")}
      description={t("client:settings.domain_mutes.description")}
      items={list.items}
      itemKey={(mute) => mute.domain}
      renderItem={(mute) => (
        <DomainMuteRow
          mute={mute}
          unmute={unmute}
          onUnmuted={() => {
            removed(mute.domain);
          }}
        />
      )}
      listRef={listRef}
      countLabel={
        list.hasMore
          ? t("client:settings.domain_mutes.count_at_least", { count })
          : t("client:settings.domain_mutes.count", { count })
      }
      hasMore={list.hasMore}
      more={{ label: t("client:settings.list.more"), onPress: list.loadMore, isPending: list.isLoadingMore }}
      empty={{
        heading: t("client:settings.domain_mutes.empty"),
        description: t("client:settings.domain_mutes.empty.description"),
      }}
      add={{
        label: t("client:settings.domain_mutes.new.title"),
        panel: (close) => <MuteDomainFormFields form={form} close={close} />,
      }}
      result={result}
    />
  );
}

/** The form that mutes a domain, in the panel of the list. */
function MuteDomainFormFields({ form, close }: { form: MuteDomainForm; close: () => void }) {
  const t = useT();
  return (
    <Stack
      as="form"
      onSubmit={(event) => {
        form.submit(event, close);
      }}
      gap="4"
    >
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
      <FormActionBar
        result={
          form.error === undefined
            ? undefined
            : { tone: "danger", message: t("client:settings.domain_mutes.mute_failed", { reason: form.error }) }
        }
      >
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:settings.domain_mutes.mute")}
        </Button>
        <Button onPress={close}>{t("client:settings.list.cancel")}</Button>
      </FormActionBar>
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
