// SPDX-License-Identifier: AGPL-3.0-only
// The region of a remote server's page with what this server sent to it and what it answered (#2748, ADR-0058 §9),
// for manage_domains: a filter by activity URI or by local account (the latter recorded in the audit log, which the
// form says before it runs), then the deliveries, newest first, each with its attempts (outcome, time, status code,
// signature scheme, error) and "Show more". A delivery about a non-public activity shows that its sender, activity
// URI and personal inbox are masked and why, with a button that reveals them (recorded in the audit log). What an
// activity says is never shown: the API does not return it.
import {
  Box,
  Button,
  Cluster,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Grid,
  Heading,
  Hint,
  RegionError,
  Section,
  Spinner,
  Stack,
  StatusMessage,
  Text,
  TextField,
} from "@my/ui";
import { useId, useRef, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type {
  Delivery,
  DeliveryAttempt,
  DeliveryFilter,
  DeliveryMask,
  DeliveryOutcome,
  SignatureScheme,
} from "../api/deliveries";
import { useDeliveries, useDeliveryFilterForm, useRevealDelivery } from "../hooks/useDeliveries";
import { handleOf } from "../hooks/useReports";
import { DateTime, RecordList, useFocusAfterChange } from "./parts";

const maskLabels: Readonly<Record<DeliveryMask, ClientMessageKey>> = {
  private: "client:admin.instance.deliveries.mask.private",
  direct: "client:admin.instance.deliveries.mask.direct",
  local: "client:admin.instance.deliveries.mask.local",
  approves_followers: "client:admin.instance.deliveries.mask.approves_followers",
  unknown: "client:admin.instance.deliveries.mask.unknown",
};

const outcomeLabels: Readonly<Record<DeliveryOutcome, ClientMessageKey>> = {
  succeeded: "client:admin.instance.deliveries.outcome.succeeded",
  retryable_failure: "client:admin.instance.deliveries.outcome.retryable_failure",
  permanent_failure: "client:admin.instance.deliveries.outcome.permanent_failure",
  skipped: "client:admin.instance.deliveries.outcome.skipped",
};

const schemeLabels: Readonly<Record<SignatureScheme, ClientMessageKey>> = {
  rfc9421: "client:admin.instance.deliveries.scheme.rfc9421",
  cavage: "client:admin.instance.deliveries.scheme.cavage",
};

/** The longest activity URI the filter takes. */
const activityUriLimit = 2048;

export type InstanceDeliveriesProps = { domain: string };

export function InstanceDeliveries({ domain }: InstanceDeliveriesProps) {
  const t = useT();
  const form = useDeliveryFilterForm();
  return (
    <Section
      surface="card"
      title={t("client:admin.instance.deliveries.title")}
      description={t("client:admin.instance.deliveries.description")}
    >
      <Stack gap="4">
        <FilterForm form={form} />
        {/* Another filter is another list: it starts at its first page, and nothing revealed carries over. */}
        <DeliveryResults
          key={JSON.stringify(form.applied)}
          domain={domain}
          filter={form.applied}
          isFiltered={form.isFiltered}
        />
      </Stack>
    </Section>
  );
}

function FilterForm({ form }: { form: ReturnType<typeof useDeliveryFilterForm> }) {
  const t = useT();
  return (
    <Stack
      as="form"
      role="search"
      aria-label={t("client:admin.instance.deliveries.filter.label")}
      onSubmit={form.submit}
      container
      gap="3"
    >
      <Grid at="md" gap="4">
        <TextField
          label={t("client:admin.instance.deliveries.filter.activity_uri")}
          description={t("client:admin.instance.deliveries.filter.activity_uri.description")}
          value={form.draft.activityUri}
          onChange={(activityUri) => {
            form.change({ activityUri });
          }}
          maxLength={activityUriLimit}
          autoComplete="off"
          spellCheck="false"
          dir="ltr"
        />
        <TextField
          label={t("client:admin.instance.deliveries.filter.account")}
          description={t("client:admin.instance.deliveries.filter.account.description")}
          value={form.draft.username}
          onChange={(username) => {
            form.change({ username });
          }}
          validate={(value) =>
            value.trim().replace(/^@/u, "").includes("@")
              ? t("client:admin.instance.deliveries.filter.account.not_local")
              : null
          }
          autoComplete="off"
          spellCheck="false"
          dir="ltr"
        />
      </Grid>
      <Cluster gap="2">
        <Button type="submit">{t("client:admin.instance.deliveries.filter.submit")}</Button>
        {form.isFiltered && (
          <Button variant="ghost" onPress={form.clear}>
            {t("client:admin.instance.deliveries.filter.clear")}
          </Button>
        )}
      </Cluster>
    </Stack>
  );
}

type DeliveryResultsProps = { domain: string; filter: DeliveryFilter; isFiltered: boolean };

function DeliveryResults({ domain, filter, isFiltered }: DeliveryResultsProps) {
  const t = useT();
  const deliveries = useDeliveries(domain, filter);
  switch (deliveries.status) {
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner size="sm" label={t("client:admin.instance.deliveries.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:admin.instance.deliveries.failed")}
          headingLevel={3}
          reason={deliveries.reason}
          retryLabel={t("client:admin.instance.deliveries.retry")}
          onRetry={deliveries.retry}
          isRetrying={deliveries.isRetrying}
        />
      );
    case "ready":
      return (
        <RecordList
          list={deliveries.list}
          getId={(delivery) => delivery.id}
          empty={
            isFiltered ? (
              <EmptyState headingLevel={3} heading={t("client:admin.instance.deliveries.empty.filtered")} />
            ) : (
              <EmptyState
                headingLevel={3}
                heading={t("client:admin.instance.deliveries.empty")}
                description={t("client:admin.instance.deliveries.empty.description")}
              />
            )
          }
          {...(deliveries.moreError !== undefined && {
            moreError: (
              <RegionError heading={t("client:admin.instance.deliveries.more_failed")} reason={deliveries.moreError} />
            ),
          })}
        >
          {(delivery) => <DeliveryRecord domain={domain} delivery={delivery} />}
        </RecordList>
      );
  }
}

function DeliveryRecord({ domain, delivery }: { domain: string; delivery: Delivery }) {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const attemptsId = useId();
  const revealId = useId();
  const [revealShown, setRevealShown] = useState(false);
  const focusHeading = useFocusAfterChange(headingRef, revealShown);
  // The reveal button goes away once the values are shown: focus moves to the delivery's heading.
  const reveal = useRevealDelivery(domain, delivery.id, () => {
    setRevealShown(true);
    focusHeading();
  });
  const revealed = reveal.data;
  const masked = delivery.mask !== undefined && revealed === undefined;
  const sender = revealed?.sender ?? delivery.sender;
  const activityUri = revealed?.activity_uri ?? delivery.activity_uri;
  const inboxUrl = revealed?.inbox_url ?? delivery.inbox_url;
  const maskedValue = t("client:admin.instance.deliveries.masked");
  return (
    <Stack gap="3">
      <Cluster gap="2" rowGap="0" align="baseline">
        <Heading level={3} ref={headingRef} focusable mono wrap="anywhere">
          <span dir="ltr">{delivery.activity_type}</span>
        </Heading>
        <Text size="sm" tone="muted">
          <DateTime value={delivery.created_at} />
        </Text>
      </Cluster>
      <StatusMessage>
        {revealed === undefined ? undefined : t("client:admin.instance.deliveries.reveal.shown")}
      </StatusMessage>
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.instance.deliveries.sender")}>
          {sender === undefined ? (
            masked ? (
              maskedValue
            ) : (
              t("client:admin.instance.none")
            )
          ) : sender.username === undefined ? (
            t("client:admin.instance.deliveries.sender.gone")
          ) : (
            <Text dir="ltr" wrap="anywhere">
              {handleOf({ acct: sender.username })}
            </Text>
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.instance.deliveries.inbox")}>
          <Stack as="span" gap="1">
            {inboxUrl === undefined ? (
              masked ? (
                maskedValue
              ) : (
                t("client:admin.instance.none")
              )
            ) : (
              <Text mono dir="ltr" wrap="anywhere">
                {inboxUrl}
              </Text>
            )}
            {delivery.shared_inbox && <Hint as="span">{t("client:admin.instance.deliveries.inbox.shared")}</Hint>}
          </Stack>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.instance.deliveries.activity_uri")}>
          {activityUri === undefined ? (
            masked ? (
              maskedValue
            ) : (
              t("client:admin.instance.none")
            )
          ) : (
            <Text mono dir="ltr" wrap="anywhere">
              {activityUri}
            </Text>
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.instance.deliveries.activity_created_at")}>
          <DateTime value={delivery.activity_created_at} />
        </DescriptionItem>
      </DescriptionList>
      {masked && delivery.mask !== undefined && (
        <Stack align="start" gap="2">
          <Text>{t(maskLabels[delivery.mask])}</Text>
          <Stack align="start" gap="1">
            <Button size="sm" onPress={reveal.reveal} isPending={reveal.isPending} aria-describedby={revealId}>
              {t("client:admin.instance.deliveries.reveal")}
            </Button>
            <Hint id={revealId}>{t("client:admin.instance.deliveries.reveal.description")}</Hint>
          </Stack>
        </Stack>
      )}
      <Stack gap="2">
        <Heading level={4} size="label" id={attemptsId}>
          {t("client:admin.instance.deliveries.attempts")}
        </Heading>
        {delivery.attempts.length === 0 ? (
          <Hint>{t("client:admin.instance.deliveries.attempts.none")}</Hint>
        ) : (
          <Stack as="ol" aria-labelledby={attemptsId} gap="2">
            {delivery.attempts.map((attempt) => (
              <Box as="li" key={attempt.id}>
                <AttemptRecord attempt={attempt} />
              </Box>
            ))}
          </Stack>
        )}
      </Stack>
    </Stack>
  );
}

function AttemptRecord({ attempt }: { attempt: DeliveryAttempt }) {
  const t = useT();
  const failed = attempt.outcome === "retryable_failure" || attempt.outcome === "permanent_failure";
  return (
    <Stack gap="1">
      <Cluster as="span" gap="2" rowGap="0">
        <Text weight="semibold" {...(failed && { tone: "warning" as const })}>
          {t(outcomeLabels[attempt.outcome])}
        </Text>
        <Text size="sm" tone="muted">
          <DateTime value={attempt.attempted_at} />
        </Text>
        {attempt.status_code !== undefined && (
          <Text size="sm" tone="muted" dir="ltr">
            {t("client:admin.instance.deliveries.status_code", { code: String(attempt.status_code) })}
          </Text>
        )}
        {attempt.signature_scheme !== undefined && (
          <Text size="sm" tone="muted">
            {t(schemeLabels[attempt.signature_scheme])}
          </Text>
        )}
      </Cluster>
      {attempt.error !== undefined && (
        <Text size="sm" wrap="anywhere" whitespace="preWrap" dir="auto">
          {attempt.error}
        </Text>
      )}
    </Stack>
  );
}
