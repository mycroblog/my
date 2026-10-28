// SPDX-License-Identifier: AGPL-3.0-only
// The supporters screen below its heading (ADR-0072, manage_supporters): the supporter program (whether it is enabled,
// its payment provider, what it needs, and enabling or disabling it), the state of the configured payment provider
// (its mode, its last webhook, its last reconciliation with how to fix its failure, and the signs of a webhook that is
// not set up right) with reconciling with it now (ADR-0074 §4), the form that creates a plan, the plans by rank
// with changing and retiring them, the retired plans, and the subscriptions narrowed to a plan and a state with ending
// one at once. The plans' names and descriptions are the administrators' text, shown with dir="auto". What a change did
// stays in a StatusMessage (role="status"), and a refusal is said in the server's words next to its field or under the
// form (role="alert"). Retiring a plan and ending a subscription ask first in an alert dialog: neither can be undone.
import {
  Button,
  Card,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  Dialog,
  DialogTrigger,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  Link,
  ListToolbar,
  Radio,
  RadioGroup,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  Switch,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
  TextArea,
  TextField,
} from "@my/ui";
import { type SubmitEvent, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFormatMoney } from "../../costs";
import { DateTime, Section, type Permission, RecordList, RecordTable, RequirePermission } from "../../moderation";
import {
  type PaymentReconcileRun,
  type PaymentStatus,
  type PaymentStatusWarning,
  type SupporterPlan,
  supporterPlanDescriptionLimit,
  type SupporterPlanVisibility,
  supporterPlanVisibilities,
  type SupporterProgramWarning,
  type SupporterSubscription,
  type SupporterSubscriptionFilter,
  type SupporterSubscriptionState,
  supporterSubscriptionStates,
} from "../api/supporters";
import {
  draftOfPlan,
  type PlanActions,
  type PlanDraft,
  type PlanFieldError,
  planName,
  type SubscriptionActions,
  useConfiguredPaymentProvider,
  useNewPlanForm,
  usePaymentStatus,
  usePlanActions,
  useRunPaymentReconcile,
  useSubscriptionActions,
  useSupporterPlans,
  useSupporterProgram,
  useSupporterSubscriptions,
} from "../hooks/useSupporters";

const warningLabels: Readonly<Record<SupporterProgramWarning, ClientMessageKey>> = {
  no_provider: "client:admin.supporters.warning.no_provider",
  no_email: "client:admin.supporters.warning.no_email",
  provider_mismatch: "client:admin.supporters.warning.provider_mismatch",
  no_commerce_disclosure: "client:admin.supporters.warning.no_commerce_disclosure",
};

const paymentModeLabels: Readonly<Record<PaymentStatus["mode"], ClientMessageKey>> = {
  test: "client:admin.supporters.payment.mode.test",
  live: "client:admin.supporters.payment.mode.live",
};

const paymentWarningLabels: Readonly<Record<PaymentStatusWarning, ClientMessageKey>> = {
  no_webhook: "client:admin.supporters.payment.warning.no_webhook",
  reconcile_corrected: "client:admin.supporters.payment.warning.reconcile_corrected",
};

const reconcileOutcomeLabels: Readonly<Record<PaymentReconcileRun["outcome"], ClientMessageKey>> = {
  succeeded: "client:admin.supporters.payment.outcome.succeeded",
  partial: "client:admin.supporters.payment.outcome.partial",
  failed: "client:admin.supporters.payment.outcome.failed",
};

/** How to fix each reason a reconciliation stopped (ADR-0074 §2); a success has none. */
const reconcileErrorFixes: Readonly<Record<Exclude<PaymentReconcileRun["error_kind"], "none">, ClientMessageKey>> = {
  request_cap: "client:admin.supporters.payment.error.request_cap",
  permission: "client:admin.supporters.payment.error.permission",
  not_found: "client:admin.supporters.payment.error.not_found",
  invalid_request: "client:admin.supporters.payment.error.invalid_request",
  throttled: "client:admin.supporters.payment.error.throttled",
  unavailable: "client:admin.supporters.payment.error.unavailable",
  invalid_signature: "client:admin.supporters.payment.error.invalid_signature",
  stale_signature: "client:admin.supporters.payment.error.stale_signature",
  mode_mismatch: "client:admin.supporters.payment.error.mode_mismatch",
  invalid_response: "client:admin.supporters.payment.error.invalid_response",
};

const visibilityLabels: Readonly<Record<SupporterPlanVisibility, ClientMessageKey>> = {
  public: "client:admin.supporters.plan.visibility.public",
  hidden: "client:admin.supporters.plan.visibility.hidden",
};

const stateLabels: Readonly<Record<SupporterSubscriptionState, ClientMessageKey>> = {
  pending: "client:admin.supporters.state.pending",
  active: "client:admin.supporters.state.active",
  canceling: "client:admin.supporters.state.canceling",
  past_due: "client:admin.supporters.state.past_due",
  lapsed: "client:admin.supporters.state.lapsed",
  expired: "client:admin.supporters.state.expired",
  revoked: "client:admin.supporters.state.revoked",
  abandoned: "client:admin.supporters.state.abandoned",
};

/** Whether a subscription goes on, so that it can be ended at once. */
function isLive(state: SupporterSubscriptionState): boolean {
  return state !== "expired" && state !== "revoked" && state !== "abandoned";
}

type SupporterSettingsProps = {
  /** The subscriptions shown, from the URL (web-client/README.md S3). */
  filter: SupporterSubscriptionFilter;
  onFilterChange: (filter: SupporterSubscriptionFilter) => void;
};

export function SupporterSettings({ filter, onFilterChange }: SupporterSettingsProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_supporters">
      <Stack gap="8">
        <Section surface="card" title={t("client:admin.supporters.program.title")}>
          <Card>
            <Program />
          </Card>
        </Section>
        <PaymentProvider />
        <Section surface="card" title={t("client:admin.supporters.new.title")}>
          <Card>
            <NewPlanForm />
          </Card>
        </Section>
        <Plans />
        <Section surface="card" title={t("client:admin.supporters.subscriptions.title")}>
          <Subscriptions filter={filter} onFilterChange={onFilterChange} />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

function Program() {
  const t = useT();
  const { program, set, isPending, error, status } = useSupporterProgram();
  const none = t("client:admin.supporters.program.none");
  return (
    <Stack gap="4">
      <Hint size="inherit">{t("client:admin.supporters.program.description")}</Hint>
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.supporters.program.provider")}>
          <span dir="ltr">{program.provider ?? none}</span>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.program.configured_provider")}>
          <span dir="ltr">{program.configured_provider ?? none}</span>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.program.live_subscriptions")}>
          {t("client:admin.supporters.program.subscription_count", { count: program.live_subscriptions })}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.program.changed_at")}>
          {program.changed_at === undefined ? (
            t("client:admin.supporters.program.never")
          ) : (
            <DateTime value={program.changed_at} />
          )}
        </DescriptionItem>
      </DescriptionList>
      {program.warnings.length > 0 && (
        <Stack as="ul" gap="1">
          {program.warnings.map((warning) => (
            <Text as="li" tone="warning" key={warning}>
              {t(warningLabels[warning])}
              {warning === "no_commerce_disclosure" && (
                <>
                  {" "}
                  <Link href="/admin/documents" variant="standalone">
                    {t("client:admin.supporters.warning.no_commerce_disclosure.link")}
                  </Link>
                </>
              )}
            </Text>
          ))}
        </Stack>
      )}
      <Switch
        isSelected={program.enabled}
        isDisabled={isPending || (!program.enabled && !program.can_enable)}
        onChange={set}
        description={t("client:admin.supporters.program.switch.description")}
      >
        {t("client:admin.supporters.program.switch")}
      </Switch>
      {error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.supporters.program.failed", { reason: error })}
        </InlineError>
      )}
      <StatusMessage>{status}</StatusMessage>
    </Stack>
  );
}

/** The state of the configured payment provider and reconciling with it now; nothing without a configured provider. */
function PaymentProvider() {
  const t = useT();
  const provider = useConfiguredPaymentProvider();
  if (provider === undefined) return null;
  return (
    <Section surface="card" title={t("client:admin.supporters.payment.title")}>
      <Card>
        <PaymentStatusView />
      </Card>
    </Section>
  );
}

function PaymentStatusView() {
  const t = useT();
  const intl = useIntl();
  const status = usePaymentStatus();
  const reconcile = useRunPaymentReconcile();
  const last = status.last_reconcile;
  return (
    <Stack gap="4">
      <Hint size="inherit">{t("client:admin.supporters.payment.description")}</Hint>
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.supporters.payment.provider")}>
          <span dir="ltr">{status.provider}</span>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.payment.mode")}>
          {t(paymentModeLabels[status.mode])}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.payment.time_zone")}>
          <span dir="ltr">{status.time_zone}</span>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.payment.last_webhook")}>
          {status.last_webhook_at === undefined ? (
            t("client:admin.supporters.payment.no_webhook")
          ) : (
            <DateTime value={status.last_webhook_at} />
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.payment.last_reconcile")}>
          {last === undefined ? (
            t("client:admin.supporters.payment.never")
          ) : (
            <Stack as="span">
              <DateTime value={last.occurred_at} />
              <span>{t(reconcileOutcomeLabels[last.outcome])}</span>
            </Stack>
          )}
        </DescriptionItem>
        {last !== undefined && (
          <>
            <DescriptionItem term={t("client:admin.supporters.payment.requests")}>
              {intl.formatNumber(last.requests)}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.supporters.payment.checked")}>
              {intl.formatNumber(last.checked_subscriptions)}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.supporters.payment.corrected")}>
              {intl.formatNumber(last.corrected_subscriptions)}
            </DescriptionItem>
          </>
        )}
      </DescriptionList>
      {last !== undefined && last.error_kind !== "none" && (
        <Text as="p" tone={last.outcome === "failed" ? "danger" : "muted"}>
          {t(reconcileErrorFixes[last.error_kind])}
        </Text>
      )}
      {status.warnings.length > 0 && (
        <Stack as="ul" gap="1">
          {status.warnings.map((warning) => (
            <Text as="li" tone="warning" key={warning}>
              {t(paymentWarningLabels[warning])}
            </Text>
          ))}
        </Stack>
      )}
      <Stack gap="2" align="start">
        <Hint size="inherit">{t("client:admin.supporters.payment.reconcile.description")}</Hint>
        <Button size="sm" isPending={reconcile.isPending} onPress={reconcile.run}>
          {t("client:admin.supporters.payment.reconcile.run")}
        </Button>
      </Stack>
      {reconcile.error !== undefined && (
        <InlineError size="inherit" alert>
          {reconcile.error}
        </InlineError>
      )}
      <StatusMessage>{reconcile.status}</StatusMessage>
    </Stack>
  );
}

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
  autoFocus?: boolean;
};

/** The fields of a plan, shared by the form of a new plan and the dialog that changes one. */
function PlanFields({ draft, change, error, isNew, autoFocus = false }: PlanFieldsProps) {
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
        autoFocus={autoFocus}
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

function NewPlanForm() {
  const t = useT();
  const form = useNewPlanForm();
  return (
    <Stack as="form" container gap="4" onSubmit={form.submit} noValidate>
      <Hint size="inherit">{t("client:admin.supporters.new.description")}</Hint>
      <PlanFields draft={form.draft} change={form.change} error={form.error} isNew />
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.supporters.new.submit")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function Plans() {
  const t = useT();
  const { active, retired } = useSupporterPlans();
  const actions = usePlanActions();
  const list = (items: SupporterPlan[]) => ({ items, hasMore: false, isLoadingMore: false, loadMore: () => undefined });
  return (
    <>
      <Section surface="card" title={t("client:admin.supporters.plans.title")}>
        <Stack gap="3">
          <StatusMessage>{actions.status}</StatusMessage>
          {actions.error !== undefined && (
            <InlineError size="inherit" alert>
              {t("client:admin.supporters.plan.change_failed", { reason: actions.error })}
            </InlineError>
          )}
          <Card>
            <RecordList
              list={list(active)}
              getId={(plan) => plan.id}
              empty={
                <EmptyState
                  headingLevel={3}
                  heading={t("client:admin.supporters.plans.empty")}
                  description={t("client:admin.supporters.plans.empty.description")}
                />
              }
            >
              {(plan) => <PlanRow plan={plan} actions={actions} />}
            </RecordList>
          </Card>
        </Stack>
      </Section>
      {retired.length > 0 && (
        <Section surface="card" title={t("client:admin.supporters.retired.title")}>
          <Card>
            <RecordList list={list(retired)} getId={(plan) => plan.id} empty={null}>
              {(plan) => <PlanRow plan={plan} actions={actions} />}
            </RecordList>
          </Card>
        </Section>
      )}
    </>
  );
}

/** The permissions of a plan's role, by name, or that it has none. */
function RolePermissions({ permissions }: { permissions: readonly Permission[] }) {
  const t = useT();
  if (permissions.length === 0) return t("client:admin.supporters.plan.role.no_permissions");
  return <span dir="ltr">{permissions.join(", ")}</span>;
}

function PlanRow({ plan, actions }: { plan: SupporterPlan; actions: PlanActions }) {
  const t = useT();
  const format = useFormatMoney();
  return (
    <Stack gap="3">
      <Heading level={3} wrap="anywhere" shrink>
        <span dir="auto" lang="">
          {plan.name}
        </span>
      </Heading>
      {!plan.retired && !plan.prices_ready && (
        <Text as="p" tone="warning">
          {t("client:admin.supporters.plan.preparing")}
        </Text>
      )}
      {!plan.retired && plan.visibility === "hidden" && (
        <Hint size="inherit">{t("client:admin.supporters.plan.visibility.hidden")}</Hint>
      )}
      {plan.description !== "" && (
        <Text as="p" whitespace="preLine" dir="auto" lang="">
          {plan.description}
        </Text>
      )}
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.supporters.plan.monthly")}>{format(plan.monthly_price)}</DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.plan.yearly")}>
          {t("client:admin.supporters.plan.yearly_value", {
            amount: format(plan.yearly_price),
            discount: String(plan.annual_discount_percent),
          })}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.plan.rank")}>{plan.rank}</DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.plan.emoji_limit")}>{plan.emoji_limit}</DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.plan.role")}>
          {t("client:admin.supporters.plan.role_value", {
            role: `⁦${plan.role.key}⁩`,
            position: String(plan.role.position),
          })}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.plan.permissions")}>
          <RolePermissions permissions={plan.role.permissions} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.supporters.plan.members")}>
          {t("client:admin.supporters.program.subscription_count", { count: plan.members })}
        </DescriptionItem>
        {plan.retired_at !== undefined && (
          <DescriptionItem term={t("client:admin.supporters.plan.retired_at")}>
            <DateTime value={plan.retired_at} />
          </DescriptionItem>
        )}
      </DescriptionList>
      {!plan.retired && (
        <Cluster gap="2" align="center">
          <EditPlan plan={plan} actions={actions} />
          <RetirePlan plan={plan} actions={actions} />
        </Cluster>
      )}
    </Stack>
  );
}

/** The terms of a plan and its role's position in a dialog; it closes once they are saved, and keeps a refusal on its field. */
function EditPlan({ plan, actions }: { plan: SupporterPlan; actions: PlanActions }) {
  const t = useT();
  const name = planName(plan);
  return (
    <DialogTrigger>
      <Button size="sm" aria-label={t("client:admin.supporters.plan.edit_plan", { name })}>
        {t("client:admin.supporters.plan.edit")}
      </Button>
      <Dialog
        title={t("client:admin.supporters.plan.edit.title", { name })}
        closeLabel={t("client:admin.supporters.close")}
        isDismissable={false}
        size="md"
      >
        {({ close }) => <EditPlanForm plan={plan} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function EditPlanForm({ plan, actions, close }: { plan: SupporterPlan; actions: PlanActions; close: () => void }) {
  const t = useT();
  const [draft, setDraft] = useState<PlanDraft>(() => draftOfPlan(plan));
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<PlanFieldError | undefined>(undefined);
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    setIsSaving(true);
    setError(undefined);
    void actions.update(plan, draft).then((refused) => {
      setIsSaving(false);
      if (refused === undefined) close();
      else setError(refused);
    });
  };
  return (
    <Stack as="form" container gap="4" onSubmit={onSubmit} noValidate>
      <Hint>{t("client:admin.supporters.plan.edit.description")}</Hint>
      <PlanFields
        draft={draft}
        change={(change) => {
          setDraft((previous) => ({ ...previous, ...change }));
        }}
        error={error}
        isNew={false}
        autoFocus
      />
      <Cluster gap="2" justify="end">
        <Button onPress={close}>{t("client:admin.supporters.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.supporters.save")}
        </Button>
      </Cluster>
    </Stack>
  );
}

/** Retiring a plan, after an alert dialog: it cannot be undone, and every subscription of it ends. */
function RetirePlan({ plan, actions }: { plan: SupporterPlan; actions: PlanActions }) {
  const t = useT();
  const name = planName(plan);
  const [isRetiring, setIsRetiring] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button size="sm" variant="danger" aria-label={t("client:admin.supporters.plan.retire_plan", { name })}>
        {t("client:admin.supporters.plan.retire")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.supporters.plan.retire.title", { name })}
        description={t("client:admin.supporters.plan.retire.description")}
        confirmLabel={t("client:admin.supporters.plan.retire")}
        cancelLabel={t("client:admin.supporters.cancel")}
        tone="danger"
        isPending={isRetiring}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.supporters.plan.change_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsRetiring(true);
          setFailed(false);
          void actions.retire(plan).then((retired) => {
            setIsRetiring(false);
            if (retired) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}

/** The value of the "every" choice of the filters. */
const anyChoice = "any";

function Subscriptions({ filter, onFilterChange }: SupporterSettingsProps) {
  const t = useT();
  const { active, retired } = useSupporterPlans();
  const list = useSupporterSubscriptions(filter);
  const actions = useSubscriptionActions();
  const plans = [
    { id: anyChoice, name: t("client:admin.supporters.filter.any_plan") },
    ...[...active, ...retired].map((plan) => ({ id: plan.id, name: plan.name })),
  ];
  const states = [
    { id: anyChoice, name: t("client:admin.supporters.filter.any_state") },
    ...supporterSubscriptionStates.map((state) => ({ id: state, name: t(stateLabels[state]) })),
  ];
  return (
    <Stack gap="4">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.supporters.filter.plan")}
          value={filter.planId ?? anyChoice}
          onChange={(key) => {
            onFilterChange({ ...filter, planId: key === anyChoice || typeof key !== "string" ? undefined : key });
          }}
          items={plans}
        >
          {(item) => (
            <SelectItem id={item.id} textValue={item.name}>
              <span dir="auto">{item.name}</span>
            </SelectItem>
          )}
        </Select>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.supporters.filter.state")}
          value={filter.state ?? anyChoice}
          onChange={(key) => {
            const state = supporterSubscriptionStates.find((s) => s === key);
            onFilterChange({ ...filter, state });
          }}
          items={states}
        >
          {(item) => (
            <SelectItem id={item.id} textValue={item.name}>
              {item.name}
            </SelectItem>
          )}
        </Select>
      </ListToolbar>
      <StatusMessage>{actions.status}</StatusMessage>
      {/* Another filter is another list: it starts at its first page. */}
      <RecordTable
        key={`${filter.planId ?? anyChoice}:${filter.state ?? anyChoice}`}
        list={list}
        caption={t("client:admin.supporters.subscriptions.title")}
        getId={(subscription) => subscription.id}
        empty={<EmptyState headingLevel={3} heading={t("client:admin.supporters.subscriptions.empty")} />}
        head={
          <>
            <TableColumnHeader>{t("client:admin.supporters.subscription.account")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.supporters.subscription.state")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.supporters.subscription.plan")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.supporters.subscription.price")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.supporters.subscription.started_at")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.supporters.subscription.period")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.list.column.actions")}</TableColumnHeader>
          </>
        }
      >
        {(subscription) => <SubscriptionRow subscription={subscription} actions={actions} />}
      </RecordTable>
    </Stack>
  );
}

/** A date after its label, as one line of text. */
function LabeledDate({ label, value }: { label: string; value: string }) {
  return (
    <span>
      {label} <DateTime value={value} />
    </span>
  );
}

function SubscriptionRow({
  subscription,
  actions,
}: {
  subscription: SupporterSubscription;
  actions: SubscriptionActions;
}) {
  const t = useT();
  const format = useFormatMoney();
  const who =
    subscription.username === undefined
      ? t("client:admin.supporters.subscription.deleted_account")
      : `@${subscription.username}`;
  return (
    <TableRow data-row-id={subscription.id}>
      <TableRowHeader wrap="anywhere">
        <span dir="ltr">{who}</span>
      </TableRowHeader>
      <TableCell>{t(stateLabels[subscription.state])}</TableCell>
      <TableCell wrap="anywhere">
        <span dir="auto" lang="">
          {subscription.plan_name}
        </span>
      </TableCell>
      <TableCell numeric>
        {t(
          subscription.interval === "year"
            ? "client:admin.supporters.subscription.per_year"
            : "client:admin.supporters.subscription.per_month",
          {
            amount: format(subscription.price),
          },
        )}
      </TableCell>
      <TableCell>
        <DateTime value={subscription.started_at} />
      </TableCell>
      <TableCell>
        <Stack gap="1">
          {subscription.period_ends_at !== undefined && (
            <LabeledDate
              label={t("client:admin.supporters.subscription.period_ends_at")}
              value={subscription.period_ends_at}
            />
          )}
          {subscription.grace_ends_at !== undefined && (
            <LabeledDate
              label={t("client:admin.supporters.subscription.grace_ends_at")}
              value={subscription.grace_ends_at}
            />
          )}
          {subscription.scheduled_interval !== undefined && (
            <Cluster as="span" gap="1" rowGap="0">
              <span>{t("client:admin.supporters.subscription.scheduled")}</span>
              <span>
                {t(
                  subscription.scheduled_interval === "year"
                    ? "client:admin.supporters.subscription.scheduled_year"
                    : "client:admin.supporters.subscription.scheduled_month",
                )}
              </span>
            </Cluster>
          )}
        </Stack>
      </TableCell>
      <TableCell>
        {isLive(subscription.state) && <CancelSubscription subscription={subscription} who={who} actions={actions} />}
      </TableCell>
    </TableRow>
  );
}

/** Ending a subscription at once, after an alert dialog: the role goes now, without a refund. */
function CancelSubscription({
  subscription,
  who,
  actions,
}: {
  subscription: SupporterSubscription;
  who: string;
  actions: SubscriptionActions;
}) {
  const t = useT();
  const name = `⁦${who}⁩`;
  const [isCanceling, setIsCanceling] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <div>
      <DialogTrigger>
        <Button size="sm" variant="danger" aria-label={t("client:admin.supporters.subscription.cancel_of", { name })}>
          {t("client:admin.supporters.subscription.cancel")}
        </Button>
        <ConfirmDialog
          title={t("client:admin.supporters.subscription.cancel.title", { name })}
          description={t("client:admin.supporters.subscription.cancel.description")}
          confirmLabel={t("client:admin.supporters.subscription.cancel")}
          cancelLabel={t("client:admin.supporters.cancel")}
          tone="danger"
          isPending={isCanceling}
          {...(failed &&
            actions.error !== undefined && {
              errorMessage: t("client:admin.supporters.subscription.cancel_failed", { reason: actions.error }),
            })}
          onConfirm={(close) => {
            setIsCanceling(true);
            setFailed(false);
            void actions.cancel(subscription).then((canceled) => {
              setIsCanceling(false);
              if (canceled) close();
              else setFailed(true);
            });
          }}
        />
      </DialogTrigger>
    </div>
  );
}
