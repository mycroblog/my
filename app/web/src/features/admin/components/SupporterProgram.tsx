// SPDX-License-Identifier: AGPL-3.0-only
// The first page of the supporters screen (ADR-0072, manage_supporters; web-client/screens/admin-supporters.md): the
// supporter program in one form with one save (whether it is enabled, its payment provider, what it needs, and who may
// read the supporter list, ADR-0070 §13), and the state of the configured payment provider (its mode, its last webhook,
// its last reconciliation with how to fix its failure, and the signs of a webhook that is not set up right) with
// reconciling with it now (ADR-0074 §4). What a save did is the result of the form's action bar; a refusal is said in
// the server's words there (role="alert").
import {
  Button,
  Card,
  Checkbox,
  DescriptionItem,
  DescriptionList,
  FormActionBar,
  Heading,
  Hint,
  InlineError,
  Link,
  Radio,
  RadioGroup,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useId } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { DateTime, Section, useHasPermission } from "../../moderation";
import { instanceQueryOptions } from "../api/documents";
import type {
  PaymentReconciliationRun,
  PaymentStatus,
  PaymentStatusWarning,
  SupporterProgramWarning,
} from "../api/supporters";
import {
  supporterListVisibilities,
  useConfiguredPaymentProvider,
  usePaymentStatus,
  useRunPaymentReconcile,
  useSupporterProgramForm,
} from "../hooks/useSupporters";
import { SupporterPages } from "./supporterParts";

const warningLabels: Readonly<Record<SupporterProgramWarning, ClientMessageKey>> = {
  no_provider: "client:admin.supporters.warning.no_provider",
  no_email: "client:admin.supporters.warning.no_email",
  provider_mismatch: "client:admin.supporters.warning.provider_mismatch",
  no_commerce_disclosure: "client:admin.supporters.warning.no_commerce_disclosure",
};

const paymentModeLabels: Readonly<Record<PaymentStatus["mode"], ClientMessageKey>> = {
  test: "client:admin.supporters.payment.mode.test",
  live: "client:admin.supporters.payment.mode.live",
};

const paymentWarningLabels: Readonly<Record<PaymentStatusWarning, ClientMessageKey>> = {
  no_webhook: "client:admin.supporters.payment.warning.no_webhook",
  reconciliation_corrected: "client:admin.supporters.payment.warning.reconcile_corrected",
  duplicate_charge: "client:admin.supporters.payment.warning.duplicate_charge",
};

const reconcileOutcomeLabels: Readonly<Record<PaymentReconciliationRun["outcome"], ClientMessageKey>> = {
  succeeded: "client:admin.supporters.payment.outcome.succeeded",
  partial: "client:admin.supporters.payment.outcome.partial",
  failed: "client:admin.supporters.payment.outcome.failed",
};

/** How to fix each reason a reconciliation stopped (ADR-0074 §2); a success has none. */
const reconcileErrorFixes: Readonly<Record<Exclude<PaymentReconciliationRun["error_kind"], "none">, ClientMessageKey>> =
  {
    request_cap: "client:admin.supporters.payment.error.request_cap",
    permission: "client:admin.supporters.payment.error.permission",
    not_found: "client:admin.supporters.payment.error.not_found",
    invalid_request: "client:admin.supporters.payment.error.invalid_request",
    throttled: "client:admin.supporters.payment.error.throttled",
    unavailable: "client:admin.supporters.payment.error.unavailable",
    invalid_signature: "client:admin.supporters.payment.error.invalid_signature",
    stale_signature: "client:admin.supporters.payment.error.stale_signature",
    mode_mismatch: "client:admin.supporters.payment.error.mode_mismatch",
    invalid_response: "client:admin.supporters.payment.error.invalid_response",
  };

/** The words of each value of supporter_list_visibility. */
const visibilityChoices: Readonly<
  Record<(typeof supporterListVisibilities)[number], { label: ClientMessageKey; description: ClientMessageKey }>
> = {
  off: {
    label: "client:admin.settings.supporter_list.off",
    description: "client:admin.settings.supporter_list.off.description",
  },
  everyone: {
    label: "client:admin.settings.supporter_list.everyone",
    description: "client:admin.settings.supporter_list.everyone.description",
  },
  signed_in: {
    label: "client:admin.settings.supporter_list.signed_in",
    description: "client:admin.settings.supporter_list.signed_in.description",
  },
};

export function SupporterProgram() {
  const t = useT();
  return (
    <SupporterPages current="program">
      <Stack gap="8">
        <Section surface="card" title={t("client:admin.supporters.program.title")}>
          <Card>
            <ProgramForm />
          </Card>
        </Section>
        <PaymentProvider />
      </Stack>
    </SupporterPages>
  );
}

/**
 * The program's form: its facts and warnings, enabling it (a checkbox, saved with the form), and who may read the
 * supporter list for a viewer with manage_settings (a server setting; the others read what it is now).
 */
function ProgramForm() {
  const t = useT();
  const maySetVisibility = useHasPermission("manage_settings");
  const form = useSupporterProgramForm(maySetVisibility);
  const { program } = form;
  const none = t("client:admin.supporters.program.none");
  const listHeadingId = useId();
  return (
    <Stack
      as="form"
      gap="6"
      onSubmit={(event) => {
        void form.submit(event);
      }}
    >
      <Stack gap="4">
        <Hint size="inherit">{t("client:admin.supporters.program.description")}</Hint>
        <DescriptionList columns={2}>
          <DescriptionItem term={t("client:admin.supporters.program.provider")}>
            <span dir="ltr">{program.provider ?? none}</span>
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.supporters.program.configured_provider")}>
            <span dir="ltr">{program.configured_provider ?? none}</span>
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.supporters.program.live_subscriptions")}>
            {t("client:admin.supporters.program.subscription_count", { count: program.live_subscriptions })}
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.supporters.program.changed_at")}>
            {program.changed_at === undefined ? (
              t("client:admin.supporters.program.never")
            ) : (
              <DateTime value={program.changed_at} />
            )}
          </DescriptionItem>
        </DescriptionList>
        {program.warnings.length > 0 && (
          <Stack as="ul" gap="1">
            {program.warnings.map((warning) => (
              <Text as="li" tone="warning" key={warning}>
                {t(warningLabels[warning])}
                {warning === "no_commerce_disclosure" && (
                  <>
                    {" "}
                    <Link href="/admin/documents/commerce-disclosure" variant="standalone">
                      {t("client:admin.supporters.warning.no_commerce_disclosure.link")}
                    </Link>
                  </>
                )}
              </Text>
            ))}
          </Stack>
        )}
        <Checkbox
          isSelected={form.enabled}
          isDisabled={!program.enabled && !program.can_enable}
          onChange={form.setEnabled}
          description={t("client:admin.supporters.program.switch.description")}
        >
          {t("client:admin.supporters.program.switch")}
        </Checkbox>
      </Stack>
      <Stack as="section" aria-labelledby={listHeadingId} gap="4">
        <Heading level={3} id={listHeadingId}>
          {t("client:admin.settings.supporter_list.title")}
        </Heading>
        {maySetVisibility ? (
          <SupporterListVisibility value={form.visibility} onChange={form.setVisibility} />
        ) : (
          <SupporterListVisibilityNow />
        )}
      </Stack>
      <FormActionBar result={form.result}>
        <Button type="submit" variant="primary" isPending={form.isSaving}>
          {t("client:admin.supporters.save")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}

/** Who may read the supporter list on the about page, for a viewer with manage_settings; nothing until it is read. */
function SupporterListVisibility({
  value,
  onChange,
}: {
  value: string | undefined;
  onChange: (value: string) => void;
}) {
  const t = useT();
  if (value === undefined) return null;
  return (
    <>
      <Hint size="inherit">{t("client:admin.settings.supporter_list.description")}</Hint>
      <RadioGroup label={t("client:admin.settings.supporter_list.visibility")} value={value} onChange={onChange}>
        {supporterListVisibilities.map((choice) => (
          <Radio key={choice} value={choice} description={t(visibilityChoices[choice].description)}>
            {t(visibilityChoices[choice].label)}
          </Radio>
        ))}
      </RadioGroup>
    </>
  );
}

/** Who may read the supporter list now, from GET /instance, for a viewer without manage_settings. */
function SupporterListVisibilityNow() {
  const t = useT();
  const api = useApi();
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  if (instance === undefined) return null;
  return <p>{t("client:admin.supporters.list_visibility", { visibility: instance.supporter_list_visibility })}</p>;
}

/** The state of the configured payment provider and reconciling with it now; nothing without a configured provider. */
function PaymentProvider() {
  const t = useT();
  const provider = useConfiguredPaymentProvider();
  if (provider === undefined) return null;
  return (
    <Section surface="card" title={t("client:admin.supporters.payment.title")}>
      <Card>
        <PaymentStatusView />
      </Card>
    </Section>
  );
}

function PaymentStatusView() {
  const t = useT();
  const intl = useIntl();
  const status = usePaymentStatus();
  const reconcile = useRunPaymentReconcile();
  const last = status.last_reconciliation;
  return (
    <Stack gap="4">
      <Hint size="inherit">{t("client:admin.supporters.payment.description")}</Hint>
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.supporters.payment.provider")}>
          <span dir="ltr">{status.provider}</span>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.payment.mode")}>
          {t(paymentModeLabels[status.mode])}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.payment.time_zone")}>
          <span dir="ltr">{status.time_zone}</span>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.payment.last_webhook")}>
          {status.last_webhook_at === undefined ? (
            t("client:admin.supporters.payment.no_webhook")
          ) : (
            <DateTime value={status.last_webhook_at} />
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.payment.last_reconcile")}>
          {last === undefined ? (
            t("client:admin.supporters.payment.never")
          ) : (
            <Stack as="span">
              <DateTime value={last.occurred_at} />
              <span>{t(reconcileOutcomeLabels[last.outcome])}</span>
            </Stack>
          )}
        </DescriptionItem>
        {last !== undefined && (
          <>
            <DescriptionItem term={t("client:admin.supporters.payment.requests")}>
              {intl.formatNumber(last.requests)}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.supporters.payment.checked")}>
              {intl.formatNumber(last.checked_subscriptions)}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.supporters.payment.corrected")}>
              {intl.formatNumber(last.corrected_subscriptions)}
            </DescriptionItem>
          </>
        )}
      </DescriptionList>
      {last !== undefined && last.error_kind !== "none" && (
        <Text as="p" tone={last.outcome === "failed" ? "danger" : "muted"}>
          {t(reconcileErrorFixes[last.error_kind])}
        </Text>
      )}
      {status.warnings.length > 0 && (
        <Stack as="ul" gap="1">
          {status.warnings.map((warning) => (
            <Text as="li" tone="warning" key={warning}>
              {t(paymentWarningLabels[warning])}
            </Text>
          ))}
        </Stack>
      )}
      <Stack gap="2" align="start">
        <Hint size="inherit">{t("client:admin.supporters.payment.reconcile.description")}</Hint>
        <Button size="sm" isPending={reconcile.isPending} onPress={reconcile.run}>
          {t("client:admin.supporters.payment.reconcile.run")}
        </Button>
      </Stack>
      {reconcile.error !== undefined && (
        <InlineError size="inherit" alert>
          {reconcile.error}
        </InlineError>
      )}
      <StatusMessage>{reconcile.status}</StatusMessage>
    </Stack>
  );
}
