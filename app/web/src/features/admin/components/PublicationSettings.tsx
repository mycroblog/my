// SPDX-License-Identifier: AGPL-3.0-only
// The publication settings as forms next to the screens whose data they publish (audit A8 of #1638): the costs and
// the income on the costs screen, the supporter list on the supporters screen. They are server settings, so only a
// viewer with manage_settings gets the form; the screens say the current values to the others. Each form has one
// FormActionBar with the result of its last save (ADR-0110 §5).
import {
  Button,
  Checkbox,
  FormActionBar,
  type FormActionResult,
  Heading,
  Hint,
  Radio,
  RadioGroup,
  Stack,
} from "@my/ui";
import { type ReactNode, useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  financePublicationKeys,
  type PublicationKey,
  supporterPublicationKeys,
  usePublicationSettings,
} from "../hooks/usePublicationSettings";

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
  supporter_list_visibility: [
    {
      value: "off",
      label: "client:admin.settings.supporter_list.off",
      description: "client:admin.settings.supporter_list.off.description",
    },
    {
      value: "public",
      label: "client:admin.settings.supporter_list.public",
      description: "client:admin.settings.supporter_list.public.description",
    },
    {
      value: "signed_in",
      label: "client:admin.settings.supporter_list.signed_in",
      description: "client:admin.settings.supporter_list.signed_in.description",
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

/**
 * What the about page publishes of the costs and of the income, and what the holders of view_finance read of them on
 * the finances page (finance_visibility, finance_viewer_*, income_visibility).
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
      <ActionBar form={form} />
    </Stack>
  );
}

/** Who may read the supporter list on the about page (supporter_list_visibility). */
export function SupporterListPublicationForm() {
  const t = useT();
  const form = usePublicationSettings(supporterPublicationKeys);
  if (!form.ready) return null;
  return (
    <Stack as="form" gap="6" onSubmit={form.submit}>
      <Hint size="inherit">{t("client:admin.settings.supporter_list.description")}</Hint>
      <Radios
        form={form}
        setting="supporter_list_visibility"
        label={t("client:admin.settings.supporter_list.visibility")}
      />
      <ActionBar form={form} />
    </Stack>
  );
}
