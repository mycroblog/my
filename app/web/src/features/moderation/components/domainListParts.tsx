// SPDX-License-Identifier: AGPL-3.0-only
// What the pages of the blocklist subscriptions share (ADR-0159 §5): the words of their formats, modes, fetches and
// drafts, the local navigation of the domain actions screen's pages, and the fields of how a subscription applies its
// list, which the form of a new subscription and the settings of one have alike.
import { Checkbox, Fieldset, LocalNav, Radio, RadioGroup, Select, SelectItem, Stack, Text } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type DomainListFetch,
  type DomainListFetchError,
  type DomainListFetchOutcome,
  type DomainListFormat,
  type DomainListHold,
  type DomainListMode,
  domainListModes,
  type DomainListSilenceAs,
  domainListSilenceAs,
  type DomainListSuspendAs,
  domainListSuspendAs,
} from "../api/domainLists";
import { DateTime } from "./parts";

export const formatLabels: Readonly<Record<DomainListFormat, ClientMessageKey>> = {
  mastodon_csv: "client:admin.domain_lists.format.mastodon_csv",
  mastodon_json: "client:admin.domain_lists.format.mastodon_json",
  plain_text: "client:admin.domain_lists.format.plain_text",
};

export const formatDescriptions: Readonly<Record<DomainListFormat, ClientMessageKey>> = {
  mastodon_csv: "client:admin.domain_lists.format.mastodon_csv.description",
  mastodon_json: "client:admin.domain_lists.format.mastodon_json.description",
  plain_text: "client:admin.domain_lists.format.plain_text.description",
};

export const modeLabels: Readonly<Record<DomainListMode, ClientMessageKey>> = {
  draft: "client:admin.domain_lists.mode.draft",
  automatic: "client:admin.domain_lists.mode.automatic",
};

const modeDescriptions: Readonly<Record<DomainListMode, ClientMessageKey>> = {
  draft: "client:admin.domain_lists.mode.draft.description",
  automatic: "client:admin.domain_lists.mode.automatic.description",
};

const issueLabels: Readonly<Record<DomainListSuspendAs, ClientMessageKey>> = {
  suspend: "client:admin.domain_actions.kind.suspend",
  limit: "client:admin.domain_actions.kind.limit",
  ignore: "client:admin.domain_lists.ignore",
};

const outcomeLabels: Readonly<Record<DomainListFetchOutcome, ClientMessageKey>> = {
  applied: "client:admin.domain_lists.outcome.applied",
  not_modified: "client:admin.domain_lists.outcome.not_modified",
  failed: "client:admin.domain_lists.outcome.failed",
};

const fetchErrorLabels: Readonly<Record<DomainListFetchError, ClientMessageKey>> = {
  http_status: "client:admin.domain_lists.fetch_error.http_status",
  network: "client:admin.domain_lists.fetch_error.network",
  forbidden_destination: "client:admin.domain_lists.fetch_error.forbidden_destination",
  too_large: "client:admin.domain_lists.fetch_error.too_large",
  too_many_entries: "client:admin.domain_lists.fetch_error.too_many_entries",
  malformed: "client:admin.domain_lists.fetch_error.malformed",
};

export const holdLabels: Readonly<Record<DomainListHold, ClientMessageKey>> = {
  draft_mode: "client:admin.domain_lists.hold.draft_mode",
  unknown_severity: "client:admin.domain_lists.hold.unknown_severity",
  mass_lift: "client:admin.domain_lists.hold.mass_lift",
};

/** The pages of the domain actions screen under its heading: the list, the subscriptions, the allowlist, and Mastodon's CSV. */
export function DomainActionsNav({ current }: { current: "list" | "subscriptions" | "allowed" | "import" }) {
  const t = useT();
  return (
    <LocalNav
      label={t("client:admin.domain_actions.pages")}
      items={[
        {
          href: "/moderation/domain-actions",
          label: t("client:admin.domain_actions.list.title"),
          current: current === "list",
        },
        {
          href: "/moderation/domain-actions/subscriptions",
          label: t("client:admin.domain_lists.title"),
          current: current === "subscriptions",
        },
        {
          href: "/moderation/domain-actions/allowed",
          label: t("client:admin.domain_allows.title"),
          current: current === "allowed",
        },
        {
          href: "/moderation/domain-actions/import",
          label: t("client:admin.domain_actions.csv.title"),
          current: current === "import",
        },
      ]}
    />
  );
}

/** A fetch's outcome in words, a failure with why and the status code; the danger tone only repeats the words. */
export function FetchOutcome({ fetch }: { fetch: DomainListFetch }) {
  const t = useT();
  if (fetch.outcome !== "failed") return t(outcomeLabels[fetch.outcome]);
  const reason = fetch.error_code === undefined ? undefined : t(fetchErrorLabels[fetch.error_code]);
  return (
    <Text tone="danger">
      {reason === undefined
        ? t(outcomeLabels.failed)
        : fetch.http_status === undefined
          ? t("client:admin.domain_lists.outcome.failed_because", { reason })
          : t("client:admin.domain_lists.outcome.failed_status", { reason, status: fetch.http_status })}
    </Text>
  );
}

/** A subscription's last fetch: its outcome and when, the failures in a row, or that there was none yet. */
export function LastFetch({ fetch, failures }: { fetch: DomainListFetch | undefined; failures: number }) {
  const t = useT();
  if (fetch === undefined) return <Text tone="muted">{t("client:admin.domain_lists.never_fetched")}</Text>;
  return (
    <Stack gap="1">
      <span>
        <FetchOutcome fetch={fetch} /> <DateTime value={fetch.fetched_at} />
      </span>
      {failures > 1 && (
        <Text size="sm" tone="danger">
          {t("client:admin.domain_lists.failures", { count: failures })}
        </Text>
      )}
    </Stack>
  );
}

/** How a subscription applies its list, as the forms edit it. */
export type ApplyDraft = {
  mode: DomainListMode;
  suspendAs: DomainListSuspendAs;
  silenceAs: DomainListSilenceAs;
  applyRejectMedia: boolean;
  applyRejectReports: boolean;
  publicCommentFromList: boolean;
};

/** The defaults of a new subscription (CreateDomainListSubscriptionRequest). */
export const defaultApplyDraft: ApplyDraft = {
  mode: "draft",
  suspendAs: "suspend",
  silenceAs: "limit",
  applyRejectMedia: true,
  applyRejectReports: true,
  publicCommentFromList: false,
};

/** The fields of how a subscription applies its list: the mode, what the severities issue, and the rest of a row. */
export function ApplyFields({ draft, change }: { draft: ApplyDraft; change: (change: Partial<ApplyDraft>) => void }) {
  const t = useT();
  return (
    <>
      <RadioGroup
        label={t("client:admin.domain_lists.mode")}
        description={t("client:admin.domain_lists.mode.description")}
        value={draft.mode}
        onChange={(value) => {
          const mode = domainListModes.find((item) => item === value);
          if (mode !== undefined) change({ mode });
        }}
      >
        {domainListModes.map((mode) => (
          <Radio key={mode} value={mode} description={t(modeDescriptions[mode])}>
            {t(modeLabels[mode])}
          </Radio>
        ))}
      </RadioGroup>
      <Fieldset legend={t("client:admin.domain_lists.rows")}>
        <Select
          label={t("client:admin.domain_lists.suspend_as")}
          value={draft.suspendAs}
          onChange={(key) => {
            const suspendAs = domainListSuspendAs.find((item) => item === key);
            if (suspendAs !== undefined) change({ suspendAs });
          }}
        >
          {domainListSuspendAs.map((item) => (
            <SelectItem key={item} id={item}>
              {t(issueLabels[item])}
            </SelectItem>
          ))}
        </Select>
        <Select
          label={t("client:admin.domain_lists.silence_as")}
          value={draft.silenceAs}
          onChange={(key) => {
            const silenceAs = domainListSilenceAs.find((item) => item === key);
            if (silenceAs !== undefined) change({ silenceAs });
          }}
        >
          {domainListSilenceAs.map((item) => (
            <SelectItem key={item} id={item}>
              {t(issueLabels[item])}
            </SelectItem>
          ))}
        </Select>
        <Checkbox
          isSelected={draft.applyRejectMedia}
          onChange={(applyRejectMedia) => {
            change({ applyRejectMedia });
          }}
          description={t("client:admin.domain_lists.apply_reject_media.description")}
        >
          {t("client:admin.domain_lists.apply_reject_media")}
        </Checkbox>
        <Checkbox
          isSelected={draft.applyRejectReports}
          onChange={(applyRejectReports) => {
            change({ applyRejectReports });
          }}
          description={t("client:admin.domain_lists.apply_reject_reports.description")}
        >
          {t("client:admin.domain_lists.apply_reject_reports")}
        </Checkbox>
        <Checkbox
          isSelected={draft.publicCommentFromList}
          onChange={(publicCommentFromList) => {
            change({ publicCommentFromList });
          }}
          description={t("client:admin.domain_lists.public_comment_from_list.description")}
        >
          {t("client:admin.domain_lists.public_comment_from_list")}
        </Checkbox>
      </Fieldset>
    </>
  );
}
