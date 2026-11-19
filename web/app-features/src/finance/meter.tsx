// SPDX-License-Identifier: AGPL-3.0-only

// The funding meter of this month (ADR-0067 §22, FR-905), as the column beside the timeline shows it and as the page
// of the server's costs and income does: the share of the month's target its income covers, the month and whether it
// is covered, the currency it counts in where the server publishes its amounts (income in another counts for nothing,
// and the share is no balance of the month), and, while the month's costs hold an estimate, that it may change: the
// one place the page says it. The page says more under it: the income and the target where the server publishes the
// amounts, and that the target is the month's costs where it is.

import type { FundingMeter } from "@my/app-data";
import { Box, dateFormat, Meter, Stack } from "@my/design-system";
import { moneyText, useFinanceStrings } from "../finance-totals";

/** A line under the meter. */
function Line({ children }: { children: string }) {
  return (
    <Box as="p" text="caption" tone="muted" breaks="phrase">
      {children}
    </Box>
  );
}

/** The funding meter of `meter`'s month; `detail`, with what the page says under it. */
export function FundingMeterView({ meter, detail = false }: { meter: FundingMeter; detail?: boolean | undefined }) {
  const { t, locale } = useFinanceStrings();
  // The month of a date of the calendar, which is no moment: read where it is written, in UTC.
  const month = dateFormat(locale, "UTC", { month: "long" }).format(new Date(`${meter.starts_on}T00:00:00Z`));
  const covered = meter.covered ? "yes" : "no";
  const caption =
    meter.target === undefined
      ? t("finance.meter.caption", { covered, month })
      : t("finance.meter.caption_in", { covered, month, currency: meter.target.currency });
  const shown = (
    <Meter
      value={meter.percent / 100}
      label={t("finance.meter.label")}
      caption={
        meter.estimated ? (
          // The estimate is a sentence of its own, on its own line under the caption.
          <>
            {caption}
            <br />
            {t("finance.meter.estimated")}
          </>
        ) : (
          caption
        )
      }
    />
  );
  if (!detail) return shown;
  return (
    <Stack gap="entry-line">
      {shown}
      {meter.income !== undefined && meter.target !== undefined && (
        <Line>
          {t("finance.meter.amounts", {
            income: moneyText(locale, meter.income),
            target: moneyText(locale, meter.target),
          })}
        </Line>
      )}
      {meter.target_is_costs === true && <Line>{t("finance.meter.target_costs")}</Line>}
    </Stack>
  );
}
