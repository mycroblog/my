// SPDX-License-Identifier: AGPL-3.0-only
// The publication settings as a form next to the screen whose data they publish (audit A8 of #1638): the costs and
// the income on the costs screen, with the funding meter that depends on both (the supporter list's is in the
// program's form of the supporters screen). They are
// server settings, so only a viewer with manage_settings gets the form; the screen says the current values to the
// others. The form has one FormActionBar with the result of its last save (ADR-0110 §5).
import {
  Button,
  Checkbox,
  Cluster,
  FormActionBar,
  type FormActionResult,
  Heading,
  Hint,
  Radio,
  RadioGroup,
  Stack,
  TextField,
} from "@my/ui";
import { type ReactNode, useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { financePublicationKeys, type PublicationKey, usePublicationSettings } from "../hooks/usePublicationSettings";

type Choice = { value: string; label: ClientMessageKey; description: ClientMessageKey };

/** The choices of each setting chosen with radio buttons, in their order. */
const choices = {
  finance_visibility: [
    {
      value: "private",
      label: "client:admin.settings.finance.private",
      description: "client:admin.settings.finance.private.description",
    },
    {
      value: "totals",
      label: "client:admin.settings.finance.totals",
      description: "client:admin.settings.finance.totals.description",
    },
    {
      value: "breakdown",
      label: "client:admin.settings.finance.breakdown",
      description: "client:admin.settings.finance.breakdown.description",
    },
  ],
  finance_viewer_income: [
    {
      value: "totals",
      label: "client:admin.settings.finance.viewer.income.totals",
      description: "client:admin.settings.finance.viewer.income.totals.description",
    },
    {
      value: "plans",
      label: "client:admin.settings.finance.viewer.income.plans",
      description: "client:admin.settings.finance.viewer.income.plans.description",
    },
  ],
  income_visibility: [
    {
      value: "private",
      label: "client:admin.settings.income.private",
      description: "client:admin.settings.income.private.description",
    },
    {
      value: "totals",
      label: "client:admin.settings.income.totals",
      description: "client:admin.settings.income.totals.description",
    },
    {
      value: "plans",
      label: "client:admin.settings.income.plans",
      description: "client:admin.settings.income.plans.description",
    },
  ],
} as const satisfies Partial<Record<PublicationKey, readonly Choice[]>>;

type Form = ReturnType<typeof usePublicationSettings>;

function Radios({ form, setting, label }: { form: Form; setting: keyof typeof choices; label: string }) {
  const t = useT();
  return (
    <RadioGroup
      label={label}
      value={form.value(setting)}
      onChange={(value) => {
        form.change(setting, value);
      }}
    >
      {choices[setting].map((choice) => (
        <Radio key={choice.value} value={choice.value} description={t(choice.description)}>
          {t(choice.label)}
        </Radio>
      ))}
    </RadioGroup>
  );
}

function Switch({
  form,
  setting,
  label,
  description,
}: {
  form: Form;
  setting: PublicationKey;
  label: string;
  description: string;
}) {
  return (
    <Checkbox
      isSelected={form.value(setting) === "true"}
      onChange={(selected) => {
        form.change(setting, selected ? "true" : "false");
      }}
      description={description}
    >
      {label}
    </Checkbox>
  );
}

/** A part of a form under its own heading (`h3`), named by it. */
function Part({ heading, children }: { heading: string; children: ReactNode }) {
  const id = useId();
  return (
    <Stack as="section" aria-labelledby={id} gap="4">
      <Heading level={3} id={id}>
        {heading}
      </Heading>
      {children}
    </Stack>
  );
}

/** The result of the last save for the FormActionBar: the failure with its reason, or what the save did. */
function saveResult(form: Form, failed: (reason: string) => string): FormActionResult | undefined {
  if (form.saveError !== undefined) return { tone: "danger", message: failed(form.saveError) };
  if (form.status !== undefined) return { tone: "success", message: form.status };
  return undefined;
}

function ActionBar({ form }: { form: Form }) {
  const t = useT();
  return (
    <FormActionBar result={saveResult(form, (reason) => t("client:admin.publication.save_failed", { reason }))}>
      <Button type="submit" variant="primary" isPending={form.isSaving}>
        {t("client:admin.publication.save")}
      </Button>
    </FormActionBar>
  );
}

/** Why a choice of the funding meter cannot be chosen with the costs and the income published as they are in the form. */
type FundingUnavailable = "needs_private" | "needs_public" | undefined;

/**
 * Whether the funding meter may publish `value` with the costs and the income as the form has them (ADR-0067 §22):
 * the percentage alone only while both are private, the amounts only while both are public, and nothing while one
 * is, since the published side and the percentage would give the hidden one away. The server checks it again.
 */
function fundingUnavailable(form: Form, value: string): FundingUnavailable {
  const costsPrivate = (form.value("finance_visibility") || "private") === "private";
  const incomePrivate = (form.value("income_visibility") || "private") === "private";
  if (value === "percent" && !(costsPrivate && incomePrivate)) return "needs_private";
  if (value === "amounts" && (costsPrivate || incomePrivate)) return "needs_public";
  return undefined;
}

const fundingChoices = [
  {
    value: "off",
    label: "client:admin.settings.funding.off",
    description: "client:admin.settings.funding.off.description",
  },
  {
    value: "percent",
    label: "client:admin.settings.funding.percent",
    description: "client:admin.settings.funding.percent.description",
  },
  {
    value: "amounts",
    label: "client:admin.settings.funding.amounts",
    description: "client:admin.settings.funding.amounts.description",
  },
] as const satisfies readonly Choice[];

/**
 * The funding meter (FR-905, ADR-0067 §22): what it publishes, each choice that the costs and the income published
 * as the form has them rule out disabled with the reason in its description, the monthly target, and whether the
 * third column shows it too. When the costs or the income changed in the form rule out the saved choice, the form
 * says that saving turns the meter off, as the server does.
 */
function FundingMeterPart({ form }: { form: Form }) {
  const t = useT();
  const visibility = form.value("funding_meter_visibility") || "off";
  const chosenUnavailable = fundingUnavailable(form, visibility);
  return (
    <Part heading={t("client:admin.settings.funding.title")}>
      <Hint size="inherit">{t("client:admin.settings.funding.description")}</Hint>
      <RadioGroup
        label={t("client:admin.settings.funding.visibility")}
        value={visibility}
        onChange={(value) => {
          form.change("funding_meter_visibility", value);
        }}
      >
        {fundingChoices.map((choice) => {
          const unavailable = fundingUnavailable(form, choice.value);
          return (
            <Radio
              key={choice.value}
              value={choice.value}
              // The chosen value stays selectable, so that the group keeps a focusable radio and says what is saved.
              isDisabled={unavailable !== undefined && choice.value !== visibility}
              description={
                unavailable === undefined
                  ? t(choice.description)
                  : t("client:admin.settings.funding.unavailable", { reason: unavailable })
              }
            >
              {t(choice.label)}
            </Radio>
          );
        })}
      </RadioGroup>
      {chosenUnavailable !== undefined && (
        <Hint size="inherit">{t("client:admin.settings.funding.turns_off", { reason: chosenUnavailable })}</Hint>
      )}
      <Hint size="inherit">{t("client:admin.settings.funding.target.description")}</Hint>
      <Cluster gap="4">
        <TextField
          label={t("client:admin.settings.funding.target.amount")}
          description={t("client:admin.settings.funding.target.amount.description")}
          value={form.value("funding_meter_target_amount")}
          onChange={(amount) => {
            form.change("funding_meter_target_amount", amount.trim());
          }}
          inputMode="decimal"
          grow
        />
        <TextField
          label={t("client:admin.settings.funding.target.currency")}
          description={t("client:admin.settings.funding.target.currency.description")}
          value={form.value("funding_meter_target_currency")}
          onChange={(currency) => {
            form.change("funding_meter_target_currency", currency.trim().toUpperCase());
          }}
          maxLength={3}
          spellCheck="false"
        />
      </Cluster>
      <Switch
        form={form}
        setting="funding_meter_rail"
        label={t("client:admin.settings.funding.rail")}
        description={t("client:admin.settings.funding.rail.description")}
      />
    </Part>
  );
}

/**
 * What the about page publishes of the costs and of the income, what the holders of view_finance read of them on
 * the finances page, and the funding meter (finance_visibility, finance_viewer_*, income_visibility,
 * funding_meter_*).
 */
export function FinancePublicationForm() {
  const t = useT();
  const form = usePublicationSettings(financePublicationKeys);
  if (!form.ready) return null;
  return (
    <Stack as="form" gap="6" onSubmit={form.submit}>
      <Part heading={t("client:admin.settings.finance.title")}>
        <Hint size="inherit">{t("client:admin.settings.finance.description")}</Hint>
        <Radios form={form} setting="finance_visibility" label={t("client:admin.settings.finance.visibility")} />
        <Hint size="inherit">{t("client:admin.settings.finance.viewer.description")}</Hint>
        <Switch
          form={form}
          setting="finance_viewer_item_names"
          label={t("client:admin.settings.finance.viewer.item_names")}
          description={t("client:admin.settings.finance.viewer.item_names.description")}
        />
        <Switch
          form={form}
          setting="finance_viewer_item_memos"
          label={t("client:admin.settings.finance.viewer.item_memos")}
          description={t("client:admin.settings.finance.viewer.item_memos.description")}
        />
        <Radios form={form} setting="finance_viewer_income" label={t("client:admin.settings.finance.viewer.income")} />
      </Part>
      <Part heading={t("client:admin.settings.income.title")}>
        <Hint size="inherit">{t("client:admin.settings.income.description")}</Hint>
        <Radios form={form} setting="income_visibility" label={t("client:admin.settings.income.visibility")} />
      </Part>
      <FundingMeterPart form={form} />
      <ActionBar form={form} />
    </Stack>
  );
}
