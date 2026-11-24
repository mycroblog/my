// SPDX-License-Identifier: AGPL-3.0-only

// What the server publishes of its finance, at /admin/finance/publication (ADR-0067 §8, §16, §17, §22; FR-903, FR-905,
// FR-917): of the costs, nothing, their totals, or the breakdown by provider and category too; of the income,
// nothing, its totals, or the members of each plan too; the funding meter of the month, which shows the share alone
// only while both sides are private and the amounts only while both are published, with its target and whether the
// wide layout's column shows it; and what a holder of `view_finance` reads on /finance. Each choice says what it
// publishes. These are instance settings, which a holder of `manage_settings` changes, saved as a whole; to a reader
// without it the screen says what is published now, from the instance.

import {
  amountFits,
  loadAdminSettings,
  useFinanceSettings,
  useInstance,
  usePermissions,
  useSaveFinanceSettings,
  viewerQuery,
  type AdminSetting,
  type AppData,
} from "@my/app-data";
import {
  Box,
  Button,
  ChoiceList,
  FactList,
  FeedNotice,
  FieldError,
  focusRefusedField,
  FormAlert,
  Row,
  ScreenStack,
  ScreenTitle,
  Select,
  Stack,
  TextField,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { fieldError, placeRefusal, type Refused } from "../../field-errors";
import { moneyText } from "../../finance-totals";
import type { StaffUnderProps } from "../entries";
import { errorOf, ReadNotice, Section, useStaffStrings, useSaid, type T } from "../parts";
import { FinanceBack, currencyOptions } from "./parts";

/** The settings of this screen. */
const keys = [
  "finance_visibility",
  "income_visibility",
  "funding_meter_visibility",
  "funding_meter_target_amount",
  "funding_meter_target_currency",
  "funding_meter_rail",
  "finance_viewer_item_names",
  "finance_viewer_item_memos",
  "finance_viewer_income",
] as const satisfies readonly AdminSetting["key"][];
type Key = (typeof keys)[number];
type Values = Record<Key, string>;

const costChoices = ["private", "totals", "breakdown"] as const;
const incomeChoices = ["private", "totals", "plans"] as const;
const meterChoices = ["off", "percent", "amounts"] as const;
const viewerIncomeChoices = ["totals", "plans"] as const;

/**
 * The meter's choices the visibilities allow (ADR-0067 §22): off always; the share alone while both sides are
 * private; the amounts too while both are published; nothing more while one side alone is.
 */
function meterAllowed(values: Values): readonly (typeof meterChoices)[number][] {
  const costs = values.finance_visibility !== "private";
  const income = values.income_visibility !== "private";
  if (!costs && !income) return ["off", "percent"];
  if (costs && income) return ["off", "amounts"];
  return ["off"];
}

/** The meter's value as it would be saved: off where the visibilities no longer allow what was chosen. */
const meterOf = (values: Values) => meterAllowed(values).find((m) => m === values.funding_meter_visibility) ?? "off";

/** Reads what the screen shows, for the route's loader: the settings, for a holder of `manage_settings`. */
export async function loadFinancePublication(data: AppData): Promise<void> {
  const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
  if (viewer?.permissions.includes("manage_settings") === true) await loadAdminSettings(data);
}

/** A choice of the form: its options, each its name over what it publishes. */
function Choice<V extends string>({
  id,
  label,
  choices,
  value,
  words,
  error,
  onChange,
  shown = false,
}: {
  id: string;
  label: string;
  /** Its name stands over it, where no heading names it. */
  shown?: boolean;
  choices: readonly V[];
  value: string;
  words: (choice: V) => { name: string; about: string };
  error: string | undefined;
  onChange: (value: V) => void;
}) {
  const chosen = choices.indexOf(value as V);
  return (
    <Stack gap="entry-line">
      {/* Named as a field is, under the section it belongs to, never as a peer of the sections. */}
      {shown && (
        <Box as="p" weight="label">
          {label}
        </Box>
      )}
      <ChoiceList
        id={id}
        label={label}
        multiple={false}
        describedBy={errorOf(id, error)}
        items={choices.map((c) => {
          const { name, about } = words(c);
          return {
            key: c,
            label: (
              <Stack gap="entry-line">
                <Box as="span">{name}</Box>
                <Box as="span" text="caption" tone="muted" breaks="phrase">
                  {about}
                </Box>
              </Stack>
            ),
          };
        })}
        chosen={chosen < 0 ? [] : [chosen]}
        onChange={([i]) => {
          const next = i === undefined ? undefined : choices[i];
          if (next !== undefined) onChange(next);
        }}
      />
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}

/**
 * A choice that is on or off, in the look of the screen's other choices (`ChoiceList`), so that a checkbox and a radio
 * stand alike: the same indent, the same size of label.
 */
function Check({
  id,
  label,
  on,
  error,
  onChange,
}: {
  id: string;
  label: string;
  on: boolean;
  error: string | undefined;
  onChange: (on: boolean) => void;
}) {
  return (
    <Stack gap="entry-line">
      <ChoiceList
        id={id}
        label={label}
        multiple
        describedBy={errorOf(id, error)}
        items={[{ key: id, label }]}
        chosen={on ? [0] : []}
        onChange={(chosen) => {
          onChange(chosen.length > 0);
        }}
      />
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}

/** The words of an option of a setting: its name, and what it publishes. */
type Words = { name: string; about: string };

const costWords = (t: T, value: string): Words => ({
  name: t("staff_finance.publication.costs.option", { value }),
  about: t("staff_finance.publication.costs.about", { value }),
});
const incomeWords = (t: T, value: string): Words => ({
  name: t("staff_finance.publication.income.option", { value }),
  about: t("staff_finance.publication.income.about", { value }),
});
const meterWords = (t: T, value: string): Words => ({
  name: t("staff_finance.publication.meter.option", { value }),
  about: t("staff_finance.publication.meter.about", { value }),
});
const viewerIncomeWords = (t: T, value: string): Words => ({
  name: t("staff_finance.publication.viewer_income.option", { value }),
  about: t("staff_finance.publication.viewer_income.about", { value }),
});

/** The form of what is published, for a holder of `manage_settings`. */
function PublicationForm({ saved }: { saved: Values }) {
  const { t, locale } = useStaffStrings();
  const save = useSaveFinanceSettings();
  const currencies = currencyOptions();
  const { say, status } = useSaid();
  const ids = useId();
  const idOf = (key: Key) => `${ids}-${key}`;
  const [values, setValues] = useState<Values>(saved);
  const [fault, setFault] = useState<string | undefined>(undefined);
  const [refused, setRefused] = useState<Refused<Key> | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const set = (key: Key, value: string) => {
    setValues((v) => ({ ...v, [key]: value }));
    setFault(undefined);
    setRefused(undefined);
  };
  const shown: Values = { ...values, funding_meter_visibility: meterOf(values) };
  const errorAt = (key: Key) =>
    key === "funding_meter_target_amount" && fault !== undefined ? fault : fieldError(refused, key);
  const members = Object.fromEntries(keys.map((key) => [`values.${key}`, key])) as Record<string, Key>;
  const send = () => {
    if (busy) return;
    const amount = shown.funding_meter_target_amount.trim();
    const currency = shown.funding_meter_target_currency;
    if (amount !== "" && (currency === "" ? !/^\d+(\.\d+)?$/.test(amount) : !amountFits(amount, currency))) {
      setFault(t("staff_finance.publication.target.invalid"));
      requestAnimationFrame(() => {
        focusRefusedField(idOf("funding_meter_target_amount"));
      });
      return;
    }
    const changed = Object.fromEntries(
      keys.flatMap((key) => {
        const value = key === "funding_meter_target_amount" ? amount : shown[key];
        return value === saved[key] ? [] : [[key, value]];
      }),
    );
    if (Object.keys(changed).length === 0) {
      say(t("staff_finance.publication.unchanged"));
      return;
    }
    setBusy(true);
    save(changed).then(
      () => {
        setBusy(false);
        say(t("staff_finance.publication.saved"));
      },
      (e: unknown) => {
        setBusy(false);
        const placed = placeRefusal(e, members);
        setRefused(placed);
        const key = placed.field?.input;
        if (key !== undefined)
          requestAnimationFrame(() => {
            focusRefusedField(idOf(key));
          });
      },
    );
  };
  const target = shown.funding_meter_target_amount.trim();
  const targetCurrency = shown.funding_meter_target_currency;
  const currencyError = errorAt("funding_meter_target_currency");
  // The target as money, once it is one; while it is empty, what an empty target means.
  const targetHint =
    target !== "" && targetCurrency !== "" && amountFits(target, targetCurrency)
      ? t("staff_finance.publication.target.shown", {
          amount: moneyText(locale, { amount: target, currency: targetCurrency }),
        })
      : t("staff_finance.publication.target.hint");
  const allowed = meterAllowed(values);
  const meterNote = allowed.length === 1 ? "one_side" : allowed.includes("percent") ? "both_private" : "both_published";
  return (
    <Stack
      gap="screen-field-gap"
      as={
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault();
            send();
          }}
        />
      }
    >
      {status}
      <Section title={t("staff_finance.publication.costs")}>
        <Choice
          id={idOf("finance_visibility")}
          label={t("staff_finance.publication.costs")}
          choices={costChoices}
          value={shown.finance_visibility}
          words={(c) => costWords(t, c)}
          error={errorAt("finance_visibility")}
          onChange={(v) => {
            set("finance_visibility", v);
          }}
        />
      </Section>
      <Section title={t("staff_finance.publication.income")}>
        <Choice
          id={idOf("income_visibility")}
          label={t("staff_finance.publication.income")}
          choices={incomeChoices}
          value={shown.income_visibility}
          words={(c) => incomeWords(t, c)}
          error={errorAt("income_visibility")}
          onChange={(v) => {
            set("income_visibility", v);
          }}
        />
      </Section>
      <Section title={t("staff_finance.publication.meter")}>
        {meterNote === "one_side" && (
          <Box as="p">{t("staff_finance.publication.meter.allowed", { case: meterNote })}</Box>
        )}
        <Choice
          id={idOf("funding_meter_visibility")}
          label={t("staff_finance.publication.meter")}
          choices={allowed}
          value={shown.funding_meter_visibility}
          words={(c) => meterWords(t, c)}
          error={errorAt("funding_meter_visibility")}
          onChange={(v) => {
            set("funding_meter_visibility", v);
          }}
        />
        <Stack gap="form-page-field-gap">
          <TextField
            id={idOf("funding_meter_target_amount")}
            name="target"
            label={t("staff_finance.publication.target")}
            hint={targetHint}
            inputMode="decimal"
            figures
            defaultValue={saved.funding_meter_target_amount}
            error={errorAt("funding_meter_target_amount")}
            onChange={(v) => {
              set("funding_meter_target_amount", v);
            }}
            after={
              <Select
                id={idOf("funding_meter_target_currency")}
                label={t("staff_finance.publication.target.currency")}
                describedBy={errorOf(idOf("funding_meter_target_currency"), currencyError)}
                options={[{ value: "", label: t("staff_finance.publication.target.costs_currency") }, ...currencies]}
                value={shown.funding_meter_target_currency}
                onChange={(v) => {
                  set("funding_meter_target_currency", v);
                }}
              />
            }
          />
          {currencyError !== undefined && (
            <FieldError id={`${idOf("funding_meter_target_currency")}-error`}>{currencyError}</FieldError>
          )}
        </Stack>
        <Check
          id={idOf("funding_meter_rail")}
          label={t("staff_finance.publication.rail")}
          on={shown.funding_meter_rail === "true"}
          error={errorAt("funding_meter_rail")}
          onChange={(on) => {
            set("funding_meter_rail", on ? "true" : "false");
          }}
        />
      </Section>
      <Section title={t("staff_finance.publication.viewer")}>
        <Check
          id={idOf("finance_viewer_item_names")}
          label={t("staff_finance.publication.viewer.names")}
          on={shown.finance_viewer_item_names === "true"}
          error={errorAt("finance_viewer_item_names")}
          onChange={(on) => {
            set("finance_viewer_item_names", on ? "true" : "false");
          }}
        />
        <Check
          id={idOf("finance_viewer_item_memos")}
          label={t("staff_finance.publication.viewer.memos")}
          on={shown.finance_viewer_item_memos === "true"}
          error={errorAt("finance_viewer_item_memos")}
          onChange={(on) => {
            set("finance_viewer_item_memos", on ? "true" : "false");
          }}
        />
        <Choice
          id={idOf("finance_viewer_income")}
          label={t("staff_finance.publication.viewer.income")}
          shown
          choices={viewerIncomeChoices}
          value={shown.finance_viewer_income}
          words={(c) => viewerIncomeWords(t, c)}
          error={errorAt("finance_viewer_income")}
          onChange={(v) => {
            set("finance_viewer_income", v);
          }}
        />
      </Section>
      {(fault !== undefined || refused !== undefined) && (
        <FormAlert regular>
          {refused?.field === undefined && refused?.reason !== undefined
            ? refused.reason
            : t("staff_finance.publication.not_saved")}
        </FormAlert>
      )}
      <Row>
        <Button type="submit" kind="primary" busy={busy}>
          {t("staff_finance.publication.save")}
        </Button>
      </Row>
    </Stack>
  );
}

/** What is published now, for a reader who may not change it, from the instance. */
function PublishedNow(): ReactNode {
  const { t } = useStaffStrings();
  const instance = useInstance();
  if (instance === undefined) return null;
  return (
    <Stack gap="entry-room">
      <FactList
        label={t("staff_finance.publication")}
        items={[
          {
            term: t("staff_finance.publication.costs"),
            value: t("staff_finance.publication.costs.option", { value: instance.finance_visibility }),
          },
          {
            term: t("staff_finance.publication.income"),
            value: t("staff_finance.publication.income.option", { value: instance.income_visibility }),
          },
          {
            term: t("staff_finance.publication.meter"),
            value: t("staff_finance.publication.meter.option", {
              value: instance.funding_meter?.visibility ?? "off",
            }),
          },
        ]}
      />
      <FeedNotice start>{t("staff_finance.publication.not_yours")}</FeedNotice>
    </Stack>
  );
}

/** What the server publishes of its finance: the form, or what is published now. */
export function FinancePublicationScreen({ go }: StaffUnderProps): ReactNode {
  const { t } = useStaffStrings();
  const may = usePermissions().includes("manage_settings");
  const read = useFinanceSettings(keys, may);
  const title = <ScreenTitle back={<FinanceBack to="root" go={go} />}>{t("staff_finance.publication")}</ScreenTitle>;
  if (!may)
    return (
      <ScreenStack>
        {title}
        <PublishedNow />
      </ScreenStack>
    );
  if (read.value === undefined)
    return (
      <ScreenStack>
        {title}
        <ReadNotice
          status={read.status === "error" ? "error" : "pending"}
          loading={t("staff_finance.publication.loading")}
          failed={t("staff_finance.publication.failed")}
          retry={read.refetch}
        />
      </ScreenStack>
    );
  const saved = Object.fromEntries(keys.map((key) => [key, read.value?.[key].value ?? ""])) as Values;
  return (
    <ScreenStack>
      {title}
      <PublicationForm saved={saved} />
    </ScreenStack>
  );
}
