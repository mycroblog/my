// SPDX-License-Identifier: AGPL-3.0-only
// One supporter plan's page (ADR-0072, manage_supporters; web-client/screens/admin-supporters-plan.md): its facts (the
// prices, the rank, the emoji limit, the role with its position and permissions, the members, its state), the form that
// changes its terms and its role's position (one save, with its result in the form's action bar), and retiring it after
// an alert dialog, which cannot be undone. A retired plan shows its facts only. The plan's name and description are the
// administrators' text, shown with dir="auto".
import {
  Button,
  Card,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  FormActionBar,
  Hint,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { type RefObject, useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import { useFormatMoney } from "../../costs";
import { DateTime, type Permission, RequirePermission, Section, useFocusAfterChange } from "../../moderation";
import type { SupporterPlan } from "../api/supporters";
import { planName, usePlanEditForm, usePlanRetire, useSupporterPlan } from "../hooks/useSupporters";
import { PlanFields } from "./SupporterPlanForm";
import { planStateLabel } from "./supporterParts";

export function SupporterPlanDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_supporters">
      <PlanPage id={id} />
    </RequirePermission>
  );
}

function PlanPage({ id }: { id: string }) {
  const plan = useSupporterPlan(id);
  // The route's loader finds the plan before the page shows; a plan cannot go away, so this is only a guard.
  if (plan === undefined) return null;
  return <PlanView plan={plan} />;
}

function PlanView({ plan }: { plan: SupporterPlan }) {
  const retire = usePlanRetire(plan);
  const factsRef = useRef<HTMLHeadingElement>(null);
  // Retiring takes the form and the retire button away: focus goes to the heading of the plan's facts.
  const expectRetired = useFocusAfterChange(factsRef, plan.retired);
  return (
    <Stack gap="8">
      <PlanFacts plan={plan} headingRef={factsRef} status={retire.status} />
      {!plan.retired && (
        <>
          <EditPlan plan={plan} />
          <RetirePlan
            plan={plan}
            retire={() => {
              expectRetired();
              return retire.retire();
            }}
            error={retire.error}
          />
        </>
      )}
    </Stack>
  );
}

/** The permissions of a plan's role, by name, or that it has none. */
function RolePermissions({ permissions }: { permissions: readonly Permission[] }) {
  const t = useT();
  if (permissions.length === 0) return t("client:admin.supporters.plan.role.no_permissions");
  return <span dir="ltr">{permissions.join(", ")}</span>;
}

type PlanFactsProps = {
  plan: SupporterPlan;
  headingRef: RefObject<HTMLHeadingElement | null>;
  /** What retiring the plan did. */
  status: string | undefined;
};

function PlanFacts({ plan, headingRef, status }: PlanFactsProps) {
  const t = useT();
  const format = useFormatMoney();
  return (
    <Section
      surface="card"
      headingRef={headingRef}
      title={
        <span dir="auto" lang="">
          {plan.name}
        </span>
      }
    >
      <Card>
        <Stack gap="4">
          <StatusMessage>{status}</StatusMessage>
          {plan.retired && <Hint size="inherit">{t("client:admin.supporters.plan.retired_note")}</Hint>}
          {!plan.retired && !plan.prices_ready && (
            <Text as="p" tone="warning">
              {t("client:admin.supporters.plan.preparing.description")}
            </Text>
          )}
          {plan.description !== "" && (
            <Text as="p" whitespace="preLine" dir="auto" lang="">
              {plan.description}
            </Text>
          )}
          <DescriptionList columns={2}>
            <DescriptionItem term={t("client:admin.supporters.plan.state")}>{t(planStateLabel(plan))}</DescriptionItem>
            <DescriptionItem term={t("client:admin.supporters.plan.monthly")}>
              {format(plan.monthly_price)}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.supporters.plan.yearly")}>
              {t("client:admin.supporters.plan.yearly_value", {
                amount: format(plan.yearly_price),
                discount: String(plan.annual_discount_percent),
              })}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.supporters.plan.rank")}>{plan.rank}</DescriptionItem>
            <DescriptionItem term={t("client:admin.supporters.plan.visibility")}>
              {t(
                plan.visibility === "hidden"
                  ? "client:admin.supporters.plan.visibility.hidden"
                  : "client:admin.supporters.plan.visibility.public",
              )}
            </DescriptionItem>
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
            <DescriptionItem term={t("client:admin.supporters.plan.created_at")}>
              <DateTime value={plan.created_at} />
            </DescriptionItem>
            {plan.retired_at !== undefined && (
              <DescriptionItem term={t("client:admin.supporters.plan.retired_at")}>
                <DateTime value={plan.retired_at} />
              </DescriptionItem>
            )}
          </DescriptionList>
        </Stack>
      </Card>
    </Section>
  );
}

/** The form that changes the plan's terms and its role's position: the page's one save. */
function EditPlan({ plan }: { plan: SupporterPlan }) {
  const t = useT();
  const form = usePlanEditForm(plan);
  return (
    <Section surface="card" title={t("client:admin.supporters.plan.edit.title")}>
      <Card>
        <Stack as="form" container gap="4" onSubmit={form.submit} noValidate>
          <Hint size="inherit">{t("client:admin.supporters.plan.edit.description")}</Hint>
          <PlanFields draft={form.draft} change={form.change} error={form.error} isNew={false} />
          <FormActionBar result={form.result}>
            <Button type="submit" variant="primary" isPending={form.isPending}>
              {t("client:admin.supporters.save")}
            </Button>
          </FormActionBar>
        </Stack>
      </Card>
    </Section>
  );
}

type RetirePlanProps = {
  plan: SupporterPlan;
  /** Retires the plan; resolves with whether it succeeded. */
  retire: () => Promise<boolean>;
  /** Why the last retirement failed, in the server's words. */
  error: string | undefined;
};

/** Retiring the plan, after an alert dialog: it cannot be undone, and every subscription of it ends. */
function RetirePlan({ plan, retire, error }: RetirePlanProps) {
  const t = useT();
  const name = planName(plan);
  const [isRetiring, setIsRetiring] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <Section
      surface="card"
      title={t("client:admin.supporters.plan.retire.section")}
      description={t("client:admin.supporters.plan.retire.description")}
    >
      <Card>
        <DialogTrigger>
          <Button variant="danger">{t("client:admin.supporters.plan.retire")}</Button>
          <ConfirmDialog
            title={t("client:admin.supporters.plan.retire.title", { name })}
            description={t("client:admin.supporters.plan.retire.description")}
            confirmLabel={t("client:admin.supporters.plan.retire")}
            cancelLabel={t("client:admin.supporters.cancel")}
            tone="danger"
            isPending={isRetiring}
            {...(failed &&
              error !== undefined && {
                errorMessage: t("client:admin.supporters.plan.change_failed", { reason: error }),
              })}
            onConfirm={(close) => {
              setIsRetiring(true);
              setFailed(false);
              void retire().then((retired) => {
                setIsRetiring(false);
                if (retired) close();
                else setFailed(true);
              });
            }}
          />
        </DialogTrigger>
      </Card>
    </Section>
  );
}
