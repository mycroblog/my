// SPDX-License-Identifier: AGPL-3.0-only
// What the instance publishes of the costs and of the income (`/admin/finance/publication`, ADR-0067 §8, ADR-0067 §17,
// ADR-0070 §5, manage_finance; web-client/screens/admin-finance-publication.md), next to the totals it publishes
// (audit A8): for a viewer with manage_settings, the form of those server settings with its own save; for the others,
// what they are now, read from GET /instance and from what the holders of view_finance read.
import { Cluster, Stack } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { financeCostTotalsQueryOptions } from "../../costs";
import { Section, useHasPermission } from "../../moderation";
import { instanceQueryOptions } from "../api/documents";
import { FinanceScreen } from "./FinanceParts";
import { FinancePublicationForm } from "./PublicationSettings";

export function FinancePublication({ today }: { today: string }) {
  const t = useT();
  const maySet = useHasPermission("manage_settings");
  return (
    <FinanceScreen page="publication">
      <Section surface="card" title={t("client:admin.finance.publication.title")}>
        {maySet ? (
          <FinancePublicationForm />
        ) : (
          <Stack gap="2">
            <Publication />
            <ViewerSettings today={today} />
          </Stack>
        )}
      </Section>
    </FinanceScreen>
  );
}

/**
 * What the instance publishes of these costs and of the income (the instance settings `finance_visibility` and
 * `income_visibility`, ADR-0067 §8, ADR-0067 §17), read from GET /instance.
 */
function Publication() {
  const t = useT();
  const api = useApi();
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  if (instance === undefined) return null;
  return (
    <Cluster as="p" gap="2" rowGap="0">
      <span>{t("client:admin.finance.visibility", { visibility: instance.finance_visibility })}</span>
      <span>{t("client:admin.finance.income_visibility", { visibility: instance.income_visibility })}</span>
    </Cluster>
  );
}

/**
 * What the holders of view_finance read of these costs on the finances page (the instance settings
 * `finance_viewer_*`, ADR-0070 §5), read from GET /finance/cost_totals of the months up to today.
 */
function ViewerSettings({ today }: { today: string }) {
  const t = useT();
  const api = useApi();
  const { data } = useQuery({
    ...financeCostTotalsQueryOptions(api, "month", "amortized", today),
    ...shellQueryReader,
  });
  if (data === undefined) return null;
  const { viewer } = data;
  return (
    <p>
      {t("client:admin.finance.viewer", {
        names: viewer.item_names ? "shown" : "hidden",
        memos: viewer.item_memos ? "shown" : "hidden",
        income: viewer.income,
      })}
    </p>
  );
}
