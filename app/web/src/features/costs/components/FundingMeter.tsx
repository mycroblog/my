// SPDX-License-Identifier: AGPL-3.0-only
// The funding meter (FR-905, ADR-0067 §22): how far this month's income goes toward the instance's monthly target, as
// GET /instance publishes it (`funding_meter`). It reads as text, not only as a bar: the value in words is shown and is
// the meter's `aria-valuetext`. Its wording is calm (ADR-0110): no appeal and no alarm color, and once the month is
// covered it says so instead of a larger number. A target that holds estimated costs is marked as an estimate.
import { AsidePanel, Hint, Link, ProgressMeter, Stack, Text } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useIntl } from "react-intl";
import type { components } from "@mycroblog/mykit";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { aboutPath } from "../../../lib/paths";
import { shellQueryReader } from "../../../lib/query";
import { instanceQueryOptions } from "../../about";
import { utcDate } from "../api/costs";
import { useFormatMoney } from "../hooks/useCostFormat";

/** The funding meter as GET /instance returns it. */
type FundingMeterData = components["schemas"]["FundingMeter"];

/**
 * The funding meter the instance publishes, from GET /instance (which the shell reads on every page); undefined when
 * it publishes none (the setting is off, or no target is known this month) or the instance has not loaded.
 */
function useFundingMeter(): FundingMeterData | undefined {
  const api = useApi();
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  return instance?.funding_meter;
}

/**
 * The meter of this month with its value in words and, under it, what the target is and whether it is an estimate.
 * It places nothing around itself, so that a section, a page or the third column can hold it.
 */
export function FundingMeter({ meter }: { meter: FundingMeterData }) {
  const t = useT();
  const intl = useIntl();
  const formatMoney = useFormatMoney();
  const month = intl.formatDate(utcDate(meter.starts_on), { year: "numeric", month: "long", timeZone: "UTC" });
  // The percentage is whole and rounded down by the server; it is formatted in the viewer's locale.
  const percent = intl.formatNumber(meter.percent / 100, { style: "percent", maximumFractionDigits: 0 });
  const estimated = meter.estimated ? "true" : "false";
  // The amounts come only with `amounts`; they are formatted in their currency, never converted.
  const amounts =
    meter.visibility === "amounts" && meter.income !== undefined && meter.target !== undefined
      ? { income: formatMoney(meter.income), target: formatMoney(meter.target) }
      : undefined;
  const valueText = meter.covered
    ? t("client:funding.meter.covered", { estimated })
    : amounts !== undefined
      ? t("client:funding.meter.amounts", { ...amounts, percent, estimated })
      : t("client:funding.meter.percent", { percent, estimated });
  return (
    <Stack gap="2">
      <ProgressMeter
        kind="meter"
        label={t("client:funding.meter.label", { month })}
        value={Math.min(meter.percent, 100)}
        maxValue={100}
        valueText={valueText}
      />
      {amounts !== undefined && (
        <Hint>
          {meter.target_is_costs === true
            ? t("client:funding.meter.target.costs")
            : t("client:funding.meter.target.amount", { target: amounts.target })}
        </Hint>
      )}
      {meter.estimated && <Hint>{t("client:funding.meter.estimated")}</Hint>}
    </Stack>
  );
}

/** The funding meter the instance publishes, or nothing. */
export function PublishedFundingMeter() {
  const meter = useFundingMeter();
  return meter === undefined ? null : <FundingMeter meter={meter} />;
}

/**
 * The funding meter in the third column of wide screens, while the instance shows it there (`rail`), with a link to
 * the information page, whose section of the running costs holds it too, and `tip`, the way to the tips that the
 * layout puts under the meter (the supporters feature's). Nothing otherwise.
 */
export function FundingMeterPanel({ tip }: { tip?: ReactNode }) {
  const t = useT();
  const meter = useFundingMeter();
  if (meter === undefined || !meter.rail) return null;
  return (
    <AsidePanel heading={t("client:funding.panel.title")}>
      <FundingMeter meter={meter} />
      {tip}
      <Text as="p" size="sm">
        <Link href={aboutPath} variant="standalone">
          {t("client:funding.panel.more")}
        </Link>
      </Text>
    </AsidePanel>
  );
}
