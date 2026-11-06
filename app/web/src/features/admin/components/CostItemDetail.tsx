// SPDX-License-Identifier: AGPL-3.0-only
// One recorded cost's page (`/admin/finance/costs/$id`, ADR-0067 §7, manage_finance;
// web-client/screens/admin-finance-costs-item.md): its facts under its name (provider, category, schedule, dates, today's price, every
// price revision, memo, when it was recorded), then a form for each change, each with its own submit and result in
// its `FormActionBar` (web-client/README.md E2): its label (name, provider, category, memo; the page's primary
// action), a new price from a date on, and the last payment date of a recurring item; last, deleting it for good
// after an alert dialog, which leaves for the list with a toast. Names, providers and memos are the operator's words,
// shown with dir="auto"; amounts and dates are formatted with Intl (ADR-0067 §10).
import {
  Button,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  FormActionBar,
  Hint,
  Stack,
  Text,
  TextField,
  useToast,
} from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useT } from "../../../lib/i18n";
import { categoryLabels, useFormatMoney } from "../../costs";
import { DateTime, RequirePermission, Section } from "../../moderation";
import { type CostItem, priceOn } from "../api/finance";
import {
  type LabelDraft,
  labelOf,
  type PriceDraft,
  useCancelCostItem,
  useCostItem,
  useFormatDay,
  useRelabelCostItem,
  useReviseCostItemPrice,
  useSetCostItemEnd,
} from "../hooks/useFinance";
import { LabelFields, PriceFields, useScheduleText } from "./FinanceParts";

export function CostItemDetail({ id, today }: { id: string; today: string }) {
  return (
    <RequirePermission permission="manage_finance">
      <CostItemSections id={id} today={today} />
    </RequirePermission>
  );
}

function CostItemSections({ id, today }: { id: string; today: string }) {
  const item = useCostItem(id);
  return (
    <Stack gap="8">
      <Facts item={item} today={today} />
      <EditLabel item={item} />
      <RevisePrice item={item} />
      {item.schedule !== "once" && <SetEnd item={item} />}
      <CancelItem item={item} />
    </Stack>
  );
}

function Facts({ item, today }: { item: CostItem; today: string }) {
  const t = useT();
  const formatDay = useFormatDay();
  const format = useFormatMoney();
  const scheduleText = useScheduleText();
  // Before the start date, the price of the first payment.
  const price = priceOn(item, today) ?? item.prices[0]?.price;
  return (
    <Section
      surface="card"
      title={
        <Text wrap="anywhere" dir="auto">
          {item.name}
        </Text>
      }
    >
      <DescriptionList>
        <DescriptionItem term={t("client:admin.finance.provider")}>
          <Text wrap="anywhere" dir="auto">
            {item.provider}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:costs.category")}>{t(categoryLabels[item.category])}</DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.schedule")}>{scheduleText(item)}</DescriptionItem>
        <DescriptionItem
          term={t(item.schedule === "once" ? "client:admin.finance.paid_on" : "client:admin.finance.starts_on")}
        >
          {formatDay(item.starts_on)}
        </DescriptionItem>
        {item.schedule !== "once" && (
          <DescriptionItem term={t("client:admin.finance.ends_on")}>
            {item.ends_on === undefined ? t("client:admin.finance.no_end") : formatDay(item.ends_on)}
          </DescriptionItem>
        )}
        {price !== undefined && (
          <DescriptionItem term={t("client:admin.finance.price")}>
            <Text numeric>{format(price)}</Text>
          </DescriptionItem>
        )}
        {item.prices.length > 1 && (
          <DescriptionItem term={t("client:admin.finance.prices")}>
            <Stack as="ul">
              {item.prices.map((revision) => (
                <Text as="li" numeric key={`${revision.effective_on} ${revision.revised_at}`}>
                  {t("client:admin.finance.price_from", {
                    date: formatDay(revision.effective_on),
                    amount: format(revision.price),
                  })}
                </Text>
              ))}
            </Stack>
          </DescriptionItem>
        )}
        {item.memo !== "" && (
          <DescriptionItem term={t("client:admin.finance.memo")}>
            <Text wrap="anywhere" whitespace="preWrap" dir="auto" lang="">
              {item.memo}
            </Text>
          </DescriptionItem>
        )}
        <DescriptionItem term={t("client:admin.finance.created_at")}>
          <DateTime value={item.created_at} />
        </DescriptionItem>
      </DescriptionList>
    </Section>
  );
}

/** The item's name, provider, category and memo, saved together: the page's primary action. */
function EditLabel({ item }: { item: CostItem }) {
  const t = useT();
  const relabel = useRelabelCostItem(item);
  const [label, setLabel] = useState<LabelDraft>(() => labelOf(item));
  const [submitted, setSubmitted] = useState(false);
  const errors = submitted ? relabel.errorsOf(label) : {};
  return (
    <Section surface="card" title={t("client:admin.finance.edit.title")}>
      <Stack
        as="form"
        gap="6"
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          setSubmitted(true);
          if (Object.keys(relabel.errorsOf(label)).length > 0) return;
          relabel.send(label);
        }}
      >
        <LabelFields
          label={label}
          onChange={(patch) => {
            setLabel((current) => ({ ...current, ...patch }));
          }}
          errors={errors}
        />
        <FormActionBar result={relabel.result}>
          <Button type="submit" variant="primary" isPending={relabel.isPending}>
            {t("client:admin.finance.save")}
          </Button>
        </FormActionBar>
      </Stack>
    </Section>
  );
}

/** A new price from a date on the start or later; the form empties once the price is revised. */
function RevisePrice({ item }: { item: CostItem }) {
  const t = useT();
  const revise = useReviseCostItemPrice(item);
  const currency = item.prices.at(-1)?.price.currency ?? "USD";
  const [effectiveOn, setEffectiveOn] = useState("");
  const [price, setPrice] = useState<PriceDraft>({ amount: "", currency });
  const [submitted, setSubmitted] = useState(false);
  const priceErrors = submitted ? revise.priceErrors(price) : {};
  const effectiveError = submitted ? revise.dateError(effectiveOn) : undefined;
  return (
    <Section surface="card" title={t("client:admin.finance.revise.title", { name: item.name })}>
      <Stack
        as="form"
        gap="6"
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          setSubmitted(true);
          if (revise.dateError(effectiveOn) !== undefined || Object.keys(revise.priceErrors(price)).length > 0) return;
          revise.send({ effectiveOn, price }, () => {
            setEffectiveOn("");
            setPrice((current) => ({ amount: "", currency: current.currency }));
            setSubmitted(false);
          });
        }}
      >
        <TextField
          type="date"
          label={t("client:admin.finance.effective_on")}
          description={t("client:admin.finance.effective_on.description")}
          value={effectiveOn}
          onChange={setEffectiveOn}
          isRequired
          {...(effectiveError !== undefined && { isInvalid: true, errorMessage: effectiveError })}
        />
        <PriceFields
          price={price}
          onChange={(patch) => {
            setPrice((current) => ({ ...current, ...patch }));
          }}
          errors={priceErrors}
        />
        <FormActionBar result={revise.result}>
          <Button type="submit" isPending={revise.isPending}>
            {t("client:admin.finance.revise")}
          </Button>
        </FormActionBar>
      </Stack>
    </Section>
  );
}

/** The last payment date of a recurring item; empty removes the end. */
function SetEnd({ item }: { item: CostItem }) {
  const t = useT();
  const end = useSetCostItemEnd(item);
  const [endsOn, setEndsOn] = useState(item.ends_on ?? "");
  const [submitted, setSubmitted] = useState(false);
  const endError = submitted ? end.dateError(endsOn) : undefined;
  return (
    <Section surface="card" title={t("client:admin.finance.end.title", { name: item.name })}>
      <Stack
        as="form"
        gap="6"
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          setSubmitted(true);
          if (end.dateError(endsOn) !== undefined) return;
          end.send(endsOn === "" ? undefined : endsOn);
        }}
      >
        <TextField
          type="date"
          label={t("client:admin.finance.ends_on")}
          description={t("client:admin.finance.end.description")}
          value={endsOn}
          onChange={setEndsOn}
          {...(endError !== undefined && { isInvalid: true, errorMessage: endError })}
        />
        <FormActionBar result={end.result}>
          <Button type="submit" isPending={end.isPending}>
            {t("client:admin.finance.end")}
          </Button>
        </FormActionBar>
      </Stack>
    </Section>
  );
}

/** Deleting the item for good, after an alert dialog: it leaves every period, past ones included. */
function CancelItem({ item }: { item: CostItem }) {
  const t = useT();
  const toast = useToast();
  const navigate = useNavigate();
  const cancel = useCancelCostItem(item, async (message) => {
    toast.show({ message, tone: "success" });
    await navigate({ to: "/admin/finance/costs" });
  });
  return (
    <Section surface="card" title={t("client:admin.finance.item.delete")}>
      <Hint size="inherit">{t("client:admin.finance.cancel.description")}</Hint>
      <div>
        <DialogTrigger>
          <Button variant="danger">{t("client:admin.finance.cancel_item")}</Button>
          <ConfirmDialog
            title={t("client:admin.finance.cancel.title", { name: item.name })}
            description={t("client:admin.finance.cancel.description")}
            confirmLabel={t("client:admin.finance.cancel_item")}
            cancelLabel={t("client:admin.finance.cancel_dialog")}
            isPending={cancel.isPending}
            {...(cancel.error !== undefined && { errorMessage: cancel.error })}
            onConfirm={() => {
              cancel.cancel();
            }}
          />
        </DialogTrigger>
      </div>
    </Section>
  );
}
