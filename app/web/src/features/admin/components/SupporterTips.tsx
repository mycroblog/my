// SPDX-License-Identifier: AGPL-3.0-only
// The tips on the first page of the supporters screen (ADR-0162 §1, §4, §6, §16, §17, manage_supporters;
// web-client/screens/admin-supporters.md): whether users may tip now and why not, the signs of card testing, what the
// tips cost and show (the dispute fee of each chargeback, refunds and chargebacks subtracted from the income, and the
// public totals that never break the tips out, since a small server's breakdown would tell who paid), with the operator's
// guide; and the form that enables them with the amounts users choose from, in one currency, with one save.
import {
  Button,
  Card,
  Checkbox,
  DescriptionItem,
  DescriptionList,
  ExternalLink,
  FormActionBar,
  Hint,
  Radio,
  RadioGroup,
  Stack,
  Text,
  TextField,
} from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { docsSiteUrl } from "../../../lib/docsSite";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { useFormatMoney } from "../../costs";
import { DateTime, Section } from "../../moderation";
import { instanceQueryOptions } from "../api/documents";
import { maxTipAmounts, type TipsFieldError, useSupporterTipsForm } from "../hooks/useSupporterTips";

function fieldError(error: TipsFieldError | undefined, field: TipsFieldError["field"]) {
  return error?.field === field ? { isInvalid: true, errorMessage: error.message } : {};
}

/** The section of the tips: their state and what they cost and show, then their form. */
export function SupporterTips() {
  const t = useT();
  return (
    <Section surface="card" title={t("client:admin.supporters.tips.title")}>
      <Card>
        <TipsForm />
      </Card>
    </Section>
  );
}

/** The operator's guide to the tips, at the server's version and in the UI's locale. */
function useTipsGuide(): string {
  const api = useApi();
  const intl = useIntl();
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  return docsSiteUrl(instance?.version, intl.locale, "operations/supporters/#tips");
}

function TipsForm() {
  const t = useT();
  const format = useFormatMoney();
  const form = useSupporterTipsForm();
  const guide = useTipsGuide();
  const { tips } = form;
  const currencies = tips.limits.map((limit) => limit.minimum.currency);
  return (
    <Stack
      as="form"
      gap="6"
      onSubmit={(event) => {
        void form.submit(event);
      }}
    >
      <Stack gap="4">
        <Hint size="inherit">{t("client:admin.supporters.tips.description")}</Hint>
        <DescriptionList columns={2}>
          <DescriptionItem term={t("client:admin.supporters.tips.accepting")}>
            {t(
              tips.accepting
                ? "client:admin.supporters.tips.accepting.yes"
                : "client:admin.supporters.tips.accepting.no",
            )}
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.supporters.tips.changed_at")}>
            {tips.changed_at === undefined ? (
              t("client:admin.supporters.tips.never")
            ) : (
              <DateTime value={tips.changed_at} />
            )}
          </DescriptionItem>
        </DescriptionList>
        {(!tips.one_time_payments || (tips.enabled && !tips.accepting) || tips.server_limit_reached) && (
          <Stack as="ul" gap="1">
            {!tips.one_time_payments && (
              <Text as="li" tone="warning">
                {t("client:admin.supporters.tips.warning.no_one_time")}
              </Text>
            )}
            {tips.one_time_payments && tips.enabled && !tips.accepting && (
              <Text as="li" tone="warning">
                {t("client:admin.supporters.tips.warning.not_accepting")}
              </Text>
            )}
            {tips.server_limit_reached && (
              <Text as="li" tone="warning">
                {t("client:admin.supporters.tips.warning.server_limit")}
              </Text>
            )}
          </Stack>
        )}
        <Stack as="ul" gap="1" marker="disc" tone="muted">
          <li>{t("client:admin.supporters.tips.note.disputes")}</li>
          <li>{t("client:admin.supporters.tips.note.refunds")}</li>
          <li>{t("client:admin.supporters.tips.note.breakdown")}</li>
          <li>{t("client:admin.supporters.tips.note.provider")}</li>
        </Stack>
        <Text as="p" size="sm">
          <ExternalLink href={guide} variant="standalone">
            {t("client:admin.supporters.tips.guide")}
          </ExternalLink>
        </Text>
        <Checkbox
          isSelected={form.enabled}
          isDisabled={!tips.enabled && !tips.one_time_payments}
          onChange={form.setEnabled}
          description={t("client:admin.supporters.tips.switch.description")}
        >
          {t("client:admin.supporters.tips.switch")}
        </Checkbox>
        {form.enabled && (
          <>
            {currencies.length > 0 && (
              <RadioGroup
                label={t("client:admin.supporters.tips.currency")}
                value={form.currency}
                onChange={form.setCurrency}
                {...fieldError(form.error, "currency")}
              >
                {currencies.map((currency) => (
                  <Radio key={currency} value={currency}>
                    <span dir="ltr">{currency}</span>
                  </Radio>
                ))}
              </RadioGroup>
            )}
            <TextField
              label={t("client:admin.supporters.tips.amounts")}
              description={
                form.limits === undefined
                  ? t("client:admin.supporters.tips.amounts.description", { max: String(maxTipAmounts) })
                  : t("client:admin.supporters.tips.amounts.range", {
                      max: String(maxTipAmounts),
                      minimum: format(form.limits.minimum),
                      maximum: format(form.limits.maximum),
                    })
              }
              value={form.amounts}
              onChange={form.setAmounts}
              inputMode="decimal"
              autoComplete="off"
              {...fieldError(form.error, "amounts")}
            />
          </>
        )}
      </Stack>
      <FormActionBar result={form.result}>
        <Button type="submit" variant="primary" isPending={form.isSaving}>
          {t("client:admin.supporters.tips.save")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}
