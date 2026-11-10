// SPDX-License-Identifier: AGPL-3.0-only
// The blocklist subscriptions of the domain actions screen (ADR-0159 §4, §5, manage_domains), in two of its pages:
// - the list (/moderation/domain-actions/subscriptions, below the screen's heading and local navigation): "Add
//   subscription", then the subscriptions newest first in the admin mode's table, each with its title linking to its
//   page, its mode, and its last fetch with the failures in a row;
// - the form of a new one (/moderation/domain-actions/subscriptions/new): the title, the https URL and the format of
//   the list, and how the subscription applies it (domainListParts' ApplyFields); subscribing opens its page.
import {
  Button,
  ButtonLink,
  Card,
  EmptyState,
  FormActionBar,
  Hint,
  Link,
  Radio,
  RadioGroup,
  Stack,
  TextField,
} from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { useT } from "../../../lib/i18n";
import {
  type DomainListSubscription,
  domainListFormats,
  subscriptionTitleLimit,
  subscriptionUrlLimit,
} from "../api/domainLists";
import { useDomainListSubscriptionList, useNewDomainListSubscription } from "../hooks/useDomainListSubscriptions";
import {
  ApplyFields,
  defaultApplyDraft,
  formatDescriptions,
  formatLabels,
  LastFetch,
  modeLabels,
} from "./domainListParts";
import { type RecordColumns, RecordTable, RequirePermission } from "./parts";

/** The address of a subscription's page. */
function subscriptionHref(id: string): string {
  return `/moderation/domain-actions/subscriptions/${encodeURIComponent(id)}`;
}

/** The list of the subscriptions, with the link to the form of a new one. */
export function DomainListSubscriptions() {
  const t = useT();
  return (
    <RequirePermission permission="manage_domains">
      <Stack gap="4">
        <div>
          <ButtonLink href="/moderation/domain-actions/subscriptions/new" variant="primary">
            {t("client:admin.domain_lists.new.title")}
          </ButtonLink>
        </div>
        <SubscriptionList />
      </Stack>
    </RequirePermission>
  );
}

function SubscriptionList() {
  const t = useT();
  const list = useDomainListSubscriptionList();
  const columns: RecordColumns<DomainListSubscription> = [
    {
      id: "title",
      header: t("client:admin.domain_lists.column.title"),
      wrap: "anywhere",
      cell: (subscription) => (
        <Link href={subscriptionHref(subscription.id)} variant="standalone" wrap="anywhere">
          <span dir="auto" lang="">
            {subscription.title}
          </span>
        </Link>
      ),
    },
    {
      id: "mode",
      header: t("client:admin.domain_lists.mode"),
      cell: (subscription) => t(modeLabels[subscription.mode]),
    },
    {
      id: "last_fetch",
      header: t("client:admin.domain_lists.last_fetch"),
      cell: (subscription) => (
        <LastFetch fetch={subscription.last_fetch} failures={subscription.consecutive_failures} />
      ),
    },
  ];
  return (
    <RecordTable
      list={list}
      caption={t("client:admin.domain_lists.title")}
      columns={columns}
      getId={(subscription) => subscription.id}
      headingLevel={2}
      empty={
        <EmptyState
          headingLevel={2}
          heading={t("client:admin.domain_lists.empty")}
          description={t("client:admin.domain_lists.empty.description")}
        />
      }
    />
  );
}

/** The form of a new subscription; subscribing opens its page. */
export function NewDomainListSubscription() {
  return (
    <RequirePermission permission="manage_domains">
      <NewSubscriptionForm />
    </RequirePermission>
  );
}

function NewSubscriptionForm() {
  const t = useT();
  const navigate = useNavigate();
  const form = useNewDomainListSubscription(
    { title: "", url: "", format: "mastodon_csv", ...defaultApplyDraft },
    (subscription) => {
      void navigate({ to: "/moderation/domain-actions/subscriptions/$id", params: { id: subscription.id } });
    },
  );
  const { draft } = form;
  return (
    <Card>
      <Stack as="form" gap="6" onSubmit={form.submit}>
        <Hint size="inherit">{t("client:admin.domain_lists.new.description")}</Hint>
        <TextField
          label={t("client:admin.domain_lists.title_field")}
          description={t("client:admin.domain_lists.title_field.description")}
          value={draft.title}
          onChange={(title) => {
            form.change({ title });
          }}
          isRequired
          maxLength={subscriptionTitleLimit}
          autoComplete="off"
          dir="auto"
          errorMessage={t("client:admin.domain_lists.title_field.required")}
        />
        <TextField
          type="url"
          label={t("client:admin.domain_lists.url")}
          description={t("client:admin.domain_lists.url.description")}
          value={draft.url}
          onChange={(url) => {
            form.change({ url });
          }}
          isRequired
          maxLength={subscriptionUrlLimit}
          autoComplete="off"
          spellCheck="false"
          errorMessage={t("client:admin.domain_lists.url.required")}
        />
        <RadioGroup
          label={t("client:admin.domain_lists.format")}
          value={draft.format}
          onChange={(value) => {
            const format = domainListFormats.find((item) => item === value);
            if (format !== undefined) form.change({ format });
          }}
        >
          {domainListFormats.map((format) => (
            <Radio key={format} value={format} description={t(formatDescriptions[format])}>
              {t(formatLabels[format])}
            </Radio>
          ))}
        </RadioGroup>
        <ApplyFields draft={draft} change={form.change} />
        <FormActionBar
          result={
            form.error === undefined
              ? undefined
              : { tone: "danger", message: t("client:admin.domain_lists.create_failed", { reason: form.error }) }
          }
        >
          <Button type="submit" variant="primary" isPending={form.isPending}>
            {t("client:admin.domain_lists.submit")}
          </Button>
        </FormActionBar>
      </Stack>
    </Card>
  );
}
