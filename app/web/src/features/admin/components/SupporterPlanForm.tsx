// SPDX-License-Identifier: AGPL-3.0-only
// The form of a supporter plan (ADR-0072, manage_supporters): its fields, shared by the page that creates a plan
// (web-client/screens/admin-supporters-plan-new.md) and the plan's page that changes it, and the page that creates one.
// A refusal of the server is said in its words next to the field it is about (or under the form), and the draft stays.
import {
  Button,
  ButtonLink,
  Card,
  Cluster,
  FormActionBar,
  Hint,
  InlineError,
  Radio,
  RadioGroup,
  Stack,
  TextArea,
  TextField,
} from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { RequirePermission } from "../../moderation";
import {
  supporterPlanDescriptionLimit,
  type SupporterPlanVisibility,
  supporterPlanVisibilities,
} from "../api/supporters";
import { type PlanDraft, type PlanFieldError, useNewPlanForm } from "../hooks/useSupporters";

const visibilityLabels: Readonly<Record<SupporterPlanVisibility, ClientMessageKey>> = {
  public: "client:admin.supporters.plan.visibility.public",
  hidden: "client:admin.supporters.plan.visibility.hidden",
};

/** The error props of a field, when the refusal is about it. */
function fieldError(error: PlanFieldError | undefined, field: PlanFieldError["field"]) {
  return error?.field === field ? { isInvalid: true, errorMessage: error.message } : {};
}

type PlanFieldsProps = {
  draft: PlanDraft;
  change: (change: Partial<PlanDraft>) => void;
  error: PlanFieldError | undefined;
  /** A new plan picks its role and currency, which never change; an existing one sets its role's position. */
  isNew: boolean;
};

/** The fields of a plan, shared by the form of a new plan and the form on a plan's page that changes it. */
export function PlanFields({ draft, change, error, isNew }: PlanFieldsProps) {
  const t = useT();
  const formatRemaining = (remaining: number) =>
    remaining < 0
      ? t("client:admin.supporters.plan.characters_over", { count: -remaining })
      : t("client:admin.supporters.plan.characters_left", { count: remaining });
  return (
    <>
      {isNew && (
        <TextField
          label={t("client:admin.supporters.plan.role")}
          description={t("client:admin.supporters.plan.role.description")}
          value={draft.role}
          onChange={(role) => {
            change({ role });
          }}
          dir="ltr"
          isRequired
          autoComplete="off"
          spellCheck="false"
          {...fieldError(error, "role")}
        />
      )}
      <TextField
        label={t("client:admin.supporters.plan.name")}
        value={draft.name}
        onChange={(name) => {
          change({ name });
        }}
        isRequired
        autoComplete="off"
        {...fieldError(error, "name")}
      />
      <TextArea
        label={t("client:admin.supporters.plan.description")}
        description={t("client:admin.supporters.plan.description.description")}
        value={draft.description}
        onChange={(description) => {
          change({ description });
        }}
        rows={3}
        characterLimit={supporterPlanDescriptionLimit}
        formatRemaining={formatRemaining}
        {...fieldError(error, "description")}
      />
      <Cluster gap="4">
        <TextField
          label={t("client:admin.supporters.plan.amount")}
          description={t("client:admin.supporters.plan.amount.description")}
          value={draft.amount}
          onChange={(amount) => {
            change({ amount });
          }}
          inputMode="decimal"
          isRequired
          autoComplete="off"
          {...fieldError(error, "amount")}
        />
        {isNew && (
          <TextField
            label={t("client:admin.supporters.plan.currency")}
            description={t("client:admin.supporters.plan.currency.description")}
            value={draft.currency}
            onChange={(currency) => {
              change({ currency: currency.toUpperCase() });
            }}
            isRequired
            autoComplete="off"
            maxLength={3}
            {...fieldError(error, "currency")}
          />
        )}
      </Cluster>
      <Cluster gap="4">
        <TextField
          label={t("client:admin.supporters.plan.discount")}
          description={t("client:admin.supporters.plan.discount.description")}
          value={draft.discount}
          onChange={(discount) => {
            change({ discount });
          }}
          inputMode="numeric"
          isRequired
          autoComplete="off"
          {...fieldError(error, "discount")}
        />
        <TextField
          label={t("client:admin.supporters.plan.emoji_limit")}
          description={t("client:admin.supporters.plan.emoji_limit.description")}
          value={draft.emojiLimit}
          onChange={(emojiLimit) => {
            change({ emojiLimit });
          }}
          inputMode="numeric"
          isRequired
          autoComplete="off"
          {...fieldError(error, "emojiLimit")}
        />
        <TextField
          label={t("client:admin.supporters.plan.rank")}
          description={t("client:admin.supporters.plan.rank.description")}
          value={draft.rank}
          onChange={(rank) => {
            change({ rank });
          }}
          inputMode="numeric"
          isRequired
          autoComplete="off"
          {...fieldError(error, "rank")}
        />
        {!isNew && (
          <TextField
            label={t("client:admin.supporters.plan.role_position")}
            description={t("client:admin.supporters.plan.role_position.description")}
            value={draft.rolePosition}
            onChange={(rolePosition) => {
              change({ rolePosition });
            }}
            inputMode="numeric"
            isRequired
            autoComplete="off"
            {...fieldError(error, "rolePosition")}
          />
        )}
      </Cluster>
      <RadioGroup
        label={t("client:admin.supporters.plan.visibility")}
        orientation="horizontal"
        value={draft.visibility}
        onChange={(value) => {
          const visibility = supporterPlanVisibilities.find((v) => v === value);
          if (visibility !== undefined) change({ visibility });
        }}
        {...fieldError(error, "visibility")}
      >
        {supporterPlanVisibilities.map((visibility) => (
          <Radio key={visibility} value={visibility}>
            {t(visibilityLabels[visibility])}
          </Radio>
        ))}
      </RadioGroup>
      {error?.field === "form" && (
        <InlineError size="inherit" alert>
          {t("client:admin.supporters.plan.save_failed", { reason: error.message })}
        </InlineError>
      )}
    </>
  );
}

/** The page that creates a plan: its form, and once it is created, the plan's page. */
export function NewSupporterPlan() {
  const t = useT();
  const form = useNewPlanForm();
  return (
    <RequirePermission permission="manage_supporters">
      <Card>
        <Stack as="form" container gap="4" onSubmit={form.submit} noValidate>
          <Hint size="inherit">{t("client:admin.supporters.new.description")}</Hint>
          <PlanFields draft={form.draft} change={form.change} error={form.error} isNew />
          <FormActionBar>
            <Button type="submit" variant="primary" isPending={form.isPending}>
              {t("client:admin.supporters.new.submit")}
            </Button>
            <ButtonLink href="/admin/supporters/plans">{t("client:admin.supporters.cancel")}</ButtonLink>
          </FormActionBar>
        </Stack>
      </Card>
    </RequirePermission>
  );
}
