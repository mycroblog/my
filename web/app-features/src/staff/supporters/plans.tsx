// SPDX-License-Identifier: AGPL-3.0-only

// The supporter plans as a holder of `manage_supporters` keeps them (ADR-0072; FR-910, FR-912): every plan, the
// retired ones too, by rank, at /admin/supporters/plans, each its monthly price, its members and where it stands, and
// the way to create one; a new plan at /admin/supporters/plans/new, on a role of its own; and one plan at
// /admin/supporters/plans/{id}: its facts, the change of its terms and of its role's permissions (the member ones
// alone) and position, and retiring it, for good, after the reader typed its name. A refusal of the server stands
// under the field it is about; what names none, beside the button.

import {
  amountFits,
  loadAdminPlans,
  loadAdminRoles,
  viewerQuery,
  planLimits,
  planPermissions,
  planRefusal,
  useAdminPlan,
  useAdminPlans,
  useAdminRoles,
  usePermissions,
  usePlanActions,
  refusal,
  type AdminRole,
  type AdminSupporterPlan,
  type AppData,
  type SupporterPlanChange,
  type SupporterPlanDraft,
  type SupporterPlanProblemReason,
} from "@my/app-data";
import {
  BackButton,
  Button,
  CheckField,
  ConfirmDialog,
  FactList,
  FeedNotice,
  FormAlert,
  LinkButton,
  ModalPresence,
  NumberField,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  Select,
  Stack,
  StateMark,
  TextField,
  TextLink,
  type Fact,
  type RecordColumn,
  type RecordRow,
  type RecordState,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import type { StaffUnderProps } from "../entries";
import { moneyText } from "../../finance-totals";
import { errorOf, Field, ReadNotice, Titled, useMoment, useStaffStrings, useSaid, type T } from "../parts";
import { supporterCurrencies, supportersPaths } from "./parts";

type PlanPermission = (typeof planPermissions)[number];

/** Where a plan stands: retired, preparing its prices, hidden from new subscriptions, or taking them. */
function standing(plan: AdminSupporterPlan): { state: RecordState; key: "retired" | "preparing" | "hidden" | "open" } {
  if (plan.retired) return { state: "expired", key: "retired" };
  if (!plan.prices_ready) return { state: "pending", key: "preparing" };
  if (plan.visibility === "hidden") return { state: "disabled", key: "hidden" };
  return { state: "active", key: "open" };
}

function PlanMark({ plan }: { plan: AdminSupporterPlan }) {
  const { t } = useStaffStrings();
  const { state, key } = standing(plan);
  return <StateMark state={state}>{t("staff_supporters.plan.standing", { standing: key })}</StateMark>;
}

/** Back to the first page of Supporters, or to the plans. */
function Back({ to, go }: { to: "program" | "plans"; go: (href: string) => void }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(to === "program" ? supportersPaths.program : supportersPaths.plans);
      }}
    >
      {t(to === "program" ? "staff_supporters.back.program" : "staff_supporters.back.plans")}
    </BackButton>
  );
}

// ---- The list -----------------------------------------------------------------------------------------------------------

type Column = "price" | "members" | "standing";

/** Every plan, by rank, the retired ones too, and Create a plan. */
export function PlanListScreen({ go }: StaffUnderProps): ReactNode {
  const { t, locale } = useStaffStrings();
  const read = useAdminPlans();
  const head = (
    <ScreenTitle
      back={<Back to="program" go={go} />}
      actions={
        <LinkButton kind="primary" size="timeline" href={supportersPaths.newPlan}>
          {t("staff_supporters.plans.create")}
        </LinkButton>
      }
    >
      {t("staff_supporters.plans")}
    </ScreenTitle>
  );
  if (read.forbidden) return <FeedNotice lead>{t("staff_supporters.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={read.status === "error" ? "error" : "pending"}
          loading={t("staff_supporters.plans.loading")}
          failed={t("staff_supporters.plans.failed")}
          retry={read.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<Column>[] = [
    { key: "price", label: t("staff_supporters.plans.column.price"), kind: "figures" },
    { key: "members", label: t("staff_supporters.plans.column.members"), kind: "figures" },
    { key: "standing", label: t("staff_supporters.plans.column.standing"), kind: "fit", beside: true },
  ];
  const rows = read.value.map((plan): RecordRow<Column> => ({
    id: plan.id,
    head: plan.name,
    href: supportersPaths.plan(plan.id),
    cells: {
      price: t("staff_supporters.plans.monthly", { price: moneyText(locale, plan.monthly_price) }),
      members: t("staff_supporters.plans.members", { count: plan.members }),
      standing: <PlanMark plan={plan} />,
    },
  }));
  return (
    <ScreenStack>
      {head}
      <RecordTable
        even
        label={t("staff_supporters.plans")}
        head={{ label: t("staff_supporters.plans.column.plan") }}
        columns={columns}
        rows={rows}
        empty={<FeedNotice>{t("staff_supporters.plans.empty")}</FeedNotice>}
      />
    </ScreenStack>
  );
}

/** Reads the plans, for the routes' loaders of the plans' screens. */
export const loadPlans = (data: AppData): Promise<void> => loadAdminPlans(data);

/** Reads the plans and, for a holder of `manage_roles`, the roles a new plan is on, for the new plan's loader. */
export async function loadPlanCreate(data: AppData): Promise<void> {
  const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
  await Promise.all([
    loadAdminPlans(data),
    viewer?.permissions.includes("manage_roles") === true ? loadAdminRoles(data) : undefined,
  ]);
}

// ---- The form -------------------------------------------------------------------------------------------------------

/** The inputs of a plan's form, each a member of the request or a group of them. */
type Input =
  "role" | "name" | "description" | "price" | "discount" | "emoji" | "rank" | "visibility" | "position" | "permissions";

/**
 * The input a refusal's reason is about, on a new plan's form (`editing` false) or a plan's; undefined for one that
 * names no input the form shows. A new plan's form has no position nor permissions, which are its role's: what the
 * server finds wrong with them stands under the role. A plan's form has no role, and a 404 there is the plan's.
 */
export function inputOfReason(
  reason: SupporterPlanProblemReason | "retired" | "unknown-role" | undefined,
  editing: boolean,
): Input | undefined {
  switch (reason) {
    case "name":
    case "description":
    case "visibility":
    case "rank":
      return reason;
    case "monthly_price":
    case "currency":
      return "price";
    case "annual_discount_percent":
      return "discount";
    case "emoji_limit":
    case "order_emoji_limit":
      return "emoji";
    case "order_rank":
      return "rank";
    case "role_builtin":
    case "role_taken":
    case "role_held":
    case "unknown-role":
      return editing ? undefined : "role";
    case "role_position":
    case "order_position":
      return editing ? "position" : "role";
    case "role_permissions":
    case "order_permissions":
      return editing ? "permissions" : "role";
    default:
      return undefined;
  }
}

/** What a plan's form holds. */
type Draft = {
  role: string;
  name: string;
  description: string;
  amount: string;
  currency: string;
  discount: number | null;
  emoji: number | null;
  rank: number | null;
  visibility: "public" | "hidden";
  position: number | null;
  permissions: readonly PlanPermission[];
};

/** Whether a role grants only what a plan may give (FR-912): member permissions alone. */
const memberOnly = (r: AdminRole) => r.permissions.every((p) => (planPermissions as readonly string[]).includes(p));

/**
 * The roles a new plan may be on (FR-912): with member permissions alone, below every staff role (one that grants
 * more, the built-in moderator's among them), and on no plan already (`taken`, the plans' roles). Whether a role has
 * holders the API does not say of a role; the server refuses one that has (`role_held`), under the role.
 */
export function planRoles(roles: readonly AdminRole[], taken: ReadonlySet<string>): AdminRole[] {
  const staff = roles.filter((r) => !memberOnly(r)).map((r) => r.position);
  const ceiling = staff.reduce((low, position) => Math.min(low, position), Infinity);
  return roles.filter((r) => memberOnly(r) && r.position < ceiling && !taken.has(r.key));
}

/**
 * The form of a plan: a new one (`plan` undefined), on a role chosen among the reader's roles where they may read them,
 * else named by its key; or one to change, with its role's position and permissions. `onSaved` takes the plan the server
 * answered.
 */
function PlanForm({
  plan,
  onSaved,
}: {
  plan?: AdminSupporterPlan | undefined;
  onSaved: (plan: AdminSupporterPlan) => void;
}) {
  const { t } = useStaffStrings();
  const actions = usePlanActions();
  const readsRoles = usePermissions().includes("manage_roles");
  const roles = useAdminRoles(plan === undefined && readsRoles);
  const ids = {
    role: useId(),
    name: useId(),
    description: useId(),
    price: useId(),
    currency: useId(),
    discount: useId(),
    emoji: useId(),
    rank: useId(),
    visibility: useId(),
    position: useId(),
    permissions: useId(),
  } satisfies Record<Input | "currency", string>;
  const [draft, setDraft] = useState<Draft>(() => ({
    role: "",
    name: plan?.name ?? "",
    description: plan?.description ?? "",
    amount: plan?.monthly_price.amount ?? "",
    currency: plan?.monthly_price.currency ?? "JPY",
    discount: plan?.annual_discount_percent ?? 0,
    emoji: plan?.emoji_limit ?? 0,
    rank: plan?.rank ?? null,
    visibility: plan?.visibility ?? "public",
    position: plan?.role.position ?? null,
    permissions: planPermissions.filter((p) => plan?.role.permissions.includes(p) === true),
  }));
  const [errors, setErrors] = useState<Partial<Record<Input, string>>>({});
  const [failed, setFailed] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const change = (next: Partial<Draft>, input: Input) => {
    setDraft((d) => ({ ...d, ...next }));
    setErrors((e) => ({ ...e, [input]: undefined }));
  };
  const plans = useAdminPlans().value;
  const offered =
    roles.value === undefined ? undefined : planRoles(roles.value, new Set((plans ?? []).map((p) => p.role.key)));
  /** The faults the form finds before sending, each under its input. */
  const faults = (): Partial<Record<Input, string>> => ({
    ...(plan === undefined && draft.role.trim() === "" ? { role: t("staff_supporters.plan.role.blank") } : {}),
    ...(draft.name.trim() === "" ? { name: t("staff_supporters.plan.name.blank") } : {}),
    ...(!amountFits(draft.amount, draft.currency)
      ? { price: t("staff_supporters.plan.price.invalid", { currency: draft.currency }) }
      : {}),
    ...(draft.discount === null ? { discount: t("staff_supporters.plan.number.blank") } : {}),
    ...(draft.emoji === null ? { emoji: t("staff_supporters.plan.number.blank") } : {}),
    ...(draft.rank === null ? { rank: t("staff_supporters.plan.number.blank") } : {}),
    ...(plan !== undefined && draft.position === null ? { position: t("staff_supporters.plan.number.blank") } : {}),
  });
  const send = () => {
    if (busy) return;
    setFailed(undefined);
    const found = faults();
    if (Object.keys(found).length > 0) {
      setErrors(found);
      const first = (Object.keys(ids) as (keyof typeof ids)[]).find((k) => k in found);
      if (first !== undefined) requestAnimationFrame(() => document.getElementById(ids[first])?.focus());
      return;
    }
    const price = { amount: draft.amount.trim(), currency: draft.currency };
    const terms = {
      name: draft.name.trim(),
      description: draft.description,
      monthly_price: price,
      annual_discount_percent: draft.discount ?? 0,
      emoji_limit: draft.emoji ?? 0,
      rank: draft.rank ?? 0,
      visibility: draft.visibility,
    };
    setBusy(true);
    const write =
      plan === undefined
        ? actions.create({ role: draft.role.trim(), ...terms } satisfies SupporterPlanDraft)
        : actions.change(plan.id, {
            ...terms,
            role_position: draft.position ?? plan.role.position,
            role_permissions: [...draft.permissions],
          } satisfies SupporterPlanChange);
    write.then(
      (saved) => {
        setBusy(false);
        onSaved(saved);
      },
      (e: unknown) => {
        setBusy(false);
        const reason = planRefusal(e);
        const input = inputOfReason(reason, plan !== undefined);
        const words = refusal(e) ?? t("staff_supporters.write_failed");
        if (input === undefined) {
          setFailed(reason === "retired" ? t("staff_supporters.plan.refused.retired") : words);
          return;
        }
        setErrors({ [input]: words });
        requestAnimationFrame(() => document.getElementById(ids[input])?.focus());
      },
    );
  };
  const numberField = (input: "discount" | "emoji" | "rank" | "position", label: string, min: number, max: number) => (
    <Field id={ids[input]} label={label} error={errors[input]}>
      <NumberField
        id={ids[input]}
        value={draft[input]}
        min={min}
        max={max}
        describedBy={errorOf(ids[input], errors[input])}
        onChange={(v) => {
          change({ [input]: v }, input);
        }}
      />
    </Field>
  );
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
      {plan === undefined &&
        (readsRoles && roles.status === "pending" ? (
          <Field id={ids.role} label={t("staff_supporters.plan.role")}>
            <FeedNotice start>{t("staff_supporters.plan.role.loading")}</FeedNotice>
          </Field>
        ) : offered === undefined || !readsRoles ? (
          <TextField
            id={ids.role}
            name="role"
            label={t("staff_supporters.plan.role")}
            hint={errors.role === undefined ? t("staff_supporters.plan.role.hint") : undefined}
            error={errors.role}
            onChange={(v) => {
              change({ role: v }, "role");
            }}
          />
        ) : offered.length === 0 ? (
          <Field id={ids.role} label={t("staff_supporters.plan.role")} error={errors.role}>
            <FeedNotice start>{t("staff_supporters.plan.role.none")}</FeedNotice>
          </Field>
        ) : (
          <Field id={ids.role} label={t("staff_supporters.plan.role")} error={errors.role}>
            <Select
              id={ids.role}
              describedBy={errorOf(ids.role, errors.role)}
              options={[
                { value: "", label: t("staff_supporters.plan.role.choose") },
                ...offered.map((r) => ({ value: r.key, label: r.key })),
              ]}
              value={draft.role}
              onChange={(v) => {
                change({ role: v }, "role");
              }}
            />
          </Field>
        ))}
      <TextField
        id={ids.name}
        name="name"
        label={t("staff_supporters.plan.name")}
        defaultValue={draft.name}
        maxLength={planLimits.name}
        error={errors.name}
        onChange={(v) => {
          change({ name: v }, "name");
        }}
      />
      <TextField
        id={ids.description}
        name="description"
        label={t("staff_supporters.plan.description")}
        defaultValue={draft.description}
        maxLength={planLimits.description}
        multiline
        rows={3}
        error={errors.description}
        onChange={(v) => {
          change({ description: v }, "description");
        }}
      />
      <Row wrap align="end" gap="screen-tab-gap">
        <TextField
          id={ids.price}
          name="price"
          label={t("staff_supporters.plan.price")}
          defaultValue={draft.amount}
          inputMode="decimal"
          figures
          error={errors.price}
          onChange={(v) => {
            change({ amount: v }, "price");
          }}
        />
        <Field id={ids.currency} label={t("staff_supporters.plan.currency")}>
          <Select
            id={ids.currency}
            options={supporterCurrencies.map((c) => ({ value: c, label: c }))}
            value={draft.currency}
            onChange={(v) => {
              change({ currency: v }, "price");
            }}
          />
        </Field>
      </Row>
      {numberField("discount", t("staff_supporters.plan.discount"), 0, planLimits.discount)}
      {numberField("emoji", t("staff_supporters.plan.emoji"), 0, planLimits.emoji)}
      {numberField("rank", t("staff_supporters.plan.rank"), 0, planLimits.rank)}
      <Field id={ids.visibility} label={t("staff_supporters.plan.visibility")} error={errors.visibility}>
        <Select
          id={ids.visibility}
          describedBy={errorOf(ids.visibility, errors.visibility)}
          options={(["public", "hidden"] as const).map((v) => ({
            value: v,
            label: t("staff_supporters.plan.visibility.value", { visibility: v }),
          }))}
          value={draft.visibility}
          onChange={(v) => {
            change({ visibility: v === "hidden" ? "hidden" : "public" }, "visibility");
          }}
        />
      </Field>
      {plan !== undefined && (
        <>
          {/* The server holds it under every staff role, and says so under the field. */}
          {numberField("position", t("staff_supporters.plan.position"), 0, 999)}
          <Field id={ids.permissions} label={t("staff_supporters.plan.permissions")} error={errors.permissions}>
            <Stack id={ids.permissions} tabIndex={-1} gap="entry-line">
              {planPermissions.map((p) => (
                <CheckField
                  key={p}
                  id={`${ids.permissions}-${p}`}
                  name="permissions"
                  value={p}
                  label={t("staff_supporters.permission", { permission: p })}
                  checked={draft.permissions.includes(p)}
                  touch
                  onChange={(on) => {
                    change(
                      {
                        permissions: planPermissions.filter((q) => (q === p ? on : draft.permissions.includes(q))),
                      },
                      "permissions",
                    );
                  }}
                />
              ))}
            </Stack>
          </Field>
        </>
      )}
      {failed !== undefined && <FormAlert regular>{failed}</FormAlert>}
      <Row>
        {/* A new plan's form is the screen's one action; a plan's change is one section of several, as Save is elsewhere. */}
        <Button kind={plan === undefined ? "primary" : "secondary"} type="submit" busy={busy}>
          {t(plan === undefined ? "staff_supporters.plan.create" : "staff_supporters.plan.save")}
        </Button>
      </Row>
    </Stack>
  );
}

/** A new plan: its form, and once created, its screen. */
export function PlanCreateScreen({ go }: StaffUnderProps): ReactNode {
  const { t } = useStaffStrings();
  return (
    <ScreenStack>
      <ScreenTitle back={<Back to="plans" go={go} />}>{t("staff_supporters.plans.create")}</ScreenTitle>
      <PlanForm
        onSaved={(plan) => {
          go(supportersPaths.plan(plan.id));
        }}
      />
    </ScreenStack>
  );
}

// ---- One plan -------------------------------------------------------------------------------------------------------

/** The facts of a plan. */
function planFacts(t: T, locale: string, moment: (iso: string) => string, plan: AdminSupporterPlan): Fact[] {
  return [
    { term: t("staff_supporters.plan.standing_term"), value: <PlanMark plan={plan} /> },
    {
      term: t("staff_supporters.plan.prices"),
      value: t("staff_supporters.plan.prices.value", {
        monthly: moneyText(locale, plan.monthly_price),
        yearly: moneyText(locale, plan.yearly_price),
        discount: plan.annual_discount_percent,
      }),
    },
    {
      term: t("staff_supporters.plan.members_term"),
      value: t("staff_supporters.plans.members", { count: plan.members }),
    },
    { term: t("staff_supporters.plan.role"), value: plan.role.key },
    // What the form below changes is its field alone, while the plan can be changed.
    ...(plan.retired ? retiredFacts(t, locale, plan) : []),
    ...(plan.retired_at === undefined
      ? []
      : [{ term: t("staff_supporters.plan.retired_at"), value: moment(plan.retired_at) }]),
  ];
}

/** The facts of a retired plan that its form would hold, which a retired plan no longer has. */
function retiredFacts(t: T, locale: string, plan: AdminSupporterPlan): Fact[] {
  return [
    {
      term: t("staff_supporters.plan.permissions"),
      value:
        plan.role.permissions.length === 0
          ? t("staff_supporters.plan.permissions.none")
          : new Intl.ListFormat(locale, { type: "conjunction" }).format(
              plan.role.permissions.map((p) => t("staff_supporters.permission", { permission: p })),
            ),
    },
    { term: t("staff_supporters.plan.emoji"), value: String(plan.emoji_limit) },
    { term: t("staff_supporters.plan.rank"), value: String(plan.rank) },
  ];
}

/** The name of the plan of `id`, the title of its screen; undefined until it is read. */
export function usePlanTitle(id: string): string | undefined {
  return useAdminPlan(id).value?.name;
}

/** One plan: its facts, the change of it and Retire…; a retired plan's facts alone. */
export function PlanScreen({ id, go }: StaffUnderProps & { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminPlan(id);
  if (read.forbidden) return <FeedNotice lead>{t("staff_supporters.forbidden")}</FeedNotice>;
  if (read.missing)
    return (
      <FeedNotice
        alert
        lead
        action={<TextLink href={supportersPaths.plans}>{t("staff_supporters.back.plans")}</TextLink>}
      >
        {t("staff_supporters.plan.missing")}
      </FeedNotice>
    );
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_supporters.plan.loading")}
        failed={t("staff_supporters.plan.failed")}
        retry={read.refetch}
      />
    );
  return <PlanView plan={read.value} go={go} />;
}

function PlanView({ plan, go }: { plan: AdminSupporterPlan; go: (href: string) => void }) {
  const { t, locale } = useStaffStrings();
  const moment = useMoment();
  const { retire } = usePlanActions();
  const { say, status } = useSaid();
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [round, setRound] = useState(0);
  return (
    <ScreenStack>
      <ScreenTitle back={<Back to="plans" go={go} />}>{plan.name}</ScreenTitle>
      {status}
      <FactList label={plan.name} items={planFacts(t, locale, moment, plan)} />
      {!plan.retired && (
        <>
          <Titled title={t("staff_supporters.plan.change")}>
            <PlanForm
              key={`${plan.revised_at}:${String(round)}`}
              plan={plan}
              onSaved={() => {
                setRound((n) => n + 1);
                say(t("staff_supporters.plan.done.saved"));
              }}
            />
          </Titled>
          <Titled title={t("staff_supporters.plan.retire")}>
            <Stack gap="screen-field-gap">
              <FeedNotice start lead>
                {t("staff_supporters.plan.retire.about", { count: plan.members })}
              </FeedNotice>
              <Row>
                <Button
                  onPress={() => {
                    setAsking(true);
                  }}
                >
                  {t("staff_supporters.plan.retire.ask")}
                </Button>
              </Row>
            </Stack>
          </Titled>
        </>
      )}
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={t("staff_supporters.plan.retire.confirm", { name: plan.name })}
            cancel={t("staff_supporters.cancel")}
            confirm={t("staff_supporters.plan.retire.do")}
            phrase={plan.name}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={() => {
              setBusy(true);
              retire(plan.id).then(
                () => {
                  setBusy(false);
                  setAsking(false);
                  say(t("staff_supporters.plan.done.retired"));
                },
                () => {
                  setBusy(false);
                  setAsking(false);
                  say(t("staff_supporters.write_failed"), true);
                },
              );
            }}
          >
            {t("staff_supporters.plan.retire.confirm.about", { count: plan.members })}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </ScreenStack>
  );
}
