// SPDX-License-Identifier: AGPL-3.0-only
// The new item's page of the costs screen (`/admin/finance/costs/new`, ADR-0067 §7, manage_finance;
// web-client/screens/admin-finance-costs-new.md): the form that records a cost paid once or every N weeks, months or
// years, with its name, provider, category, memo, first payment date, last payment date and price, checked before it
// is sent. The page's heading leads back to the recorded costs. Once the cost is recorded, a toast says so and the page leaves for the item's page; a failure stays in the
// form's action bar with what was typed (web-client/README.md E2).
import { Button, ButtonLink, FormActionBar, Radio, RadioGroup, Stack, TextField, useToast } from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { useT } from "../../../lib/i18n";
import { costSchedules, maxCostInterval } from "../api/finance";
import { useNewCostItemForm } from "../hooks/useFinance";
import { RequirePermission } from "../../moderation";
import { LabelFields, PriceFields, scheduleLabels } from "./FinanceParts";

export function NewCostItem() {
  const t = useT();
  const toast = useToast();
  const navigate = useNavigate();
  const form = useNewCostItemForm((item, message) => {
    toast.show({ message, tone: "success" });
    void navigate({ to: "/admin/finance/costs/$id", params: { id: item.id } });
  });
  const { draft, errors } = form;
  return (
    <RequirePermission permission="manage_finance">
      <Stack as="form" gap="6" onSubmit={form.submit} noValidate>
        <LabelFields label={draft} onChange={form.change} errors={errors} />
        <RadioGroup
          label={t("client:admin.finance.schedule")}
          description={t("client:admin.finance.schedule.description")}
          value={draft.schedule}
          onChange={(value) => {
            const schedule = costSchedules.find((item) => item === value);
            if (schedule !== undefined) form.change({ schedule });
          }}
        >
          {costSchedules.map((item) => (
            <Radio key={item} value={item}>
              {t(scheduleLabels[item])}
            </Radio>
          ))}
        </RadioGroup>
        {draft.schedule !== "once" && (
          <TextField
            label={t("client:admin.finance.interval")}
            description={t("client:admin.finance.interval.description", { max: String(maxCostInterval) })}
            value={draft.interval}
            onChange={(interval) => {
              form.change({ interval });
            }}
            inputMode="numeric"
            isRequired
            {...(errors.interval !== undefined && { isInvalid: true, errorMessage: errors.interval })}
          />
        )}
        <TextField
          type="date"
          label={t(draft.schedule === "once" ? "client:admin.finance.paid_on" : "client:admin.finance.starts_on")}
          description={t(
            draft.schedule === "once"
              ? "client:admin.finance.paid_on.description"
              : "client:admin.finance.starts_on.description",
          )}
          value={draft.startsOn}
          onChange={(startsOn) => {
            form.change({ startsOn });
          }}
          isRequired
          {...(errors.startsOn !== undefined && { isInvalid: true, errorMessage: errors.startsOn })}
        />
        {draft.schedule !== "once" && (
          <TextField
            type="date"
            label={t("client:admin.finance.ends_on")}
            description={t("client:admin.finance.ends_on.description")}
            value={draft.endsOn}
            onChange={(endsOn) => {
              form.change({ endsOn });
            }}
            {...(errors.endsOn !== undefined && { isInvalid: true, errorMessage: errors.endsOn })}
          />
        )}
        <PriceFields price={draft} onChange={form.change} errors={errors} />
        <FormActionBar
          result={
            form.failure === undefined
              ? undefined
              : { tone: "danger", message: t("client:admin.finance.create_failed", { reason: form.failure }) }
          }
        >
          <Button type="submit" variant="primary" isPending={form.isPending}>
            {t("client:admin.finance.create")}
          </Button>
          <ButtonLink href="/admin/finance/costs">{t("client:admin.finance.cancel")}</ButtonLink>
        </FormActionBar>
      </Stack>
    </RequirePermission>
  );
}
