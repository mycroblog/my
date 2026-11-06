// SPDX-License-Identifier: AGPL-3.0-only
// The plans page of the supporters screen (ADR-0072, manage_supporters; web-client/screens/admin-supporters-plans.md):
// every plan, those in effect by rank and then the retired ones, as one paginated table narrowed to a state (from the
// URL), and "Create a plan", which opens the form's page. Each row's name leads to the plan's page, where it is changed
// and retired. The names are the administrators' text, shown with dir="auto".
import { ButtonLink, Cluster, EmptyState, Link, ListToolbar, Select, SelectItem, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { DateTime, type RecordColumns, RecordTable, wholeList } from "../../moderation";
import { type SupporterPlan, type SupporterPlanState, supporterPlanStates } from "../api/supporters";
import { useSupporterPlans } from "../hooks/useSupporters";
import { planStateLabel, SupporterPages } from "./supporterParts";

/** The value of the "every state" choice of the filter. */
const anyChoice = "any";

const filterLabels = {
  in_effect: "client:admin.supporters.plan.state.in_effect",
  retired: "client:admin.supporters.plan.state.retired",
} as const satisfies Record<SupporterPlanState, string>;

type SupporterPlansProps = {
  /** The plans shown, from the URL (web-client/README.md S3); undefined shows every one. */
  state: SupporterPlanState | undefined;
  onStateChange: (state: SupporterPlanState | undefined) => void;
};

export function SupporterPlans({ state, onStateChange }: SupporterPlansProps) {
  const t = useT();
  const plans = useSupporterPlans(state);
  const columns: RecordColumns<SupporterPlan> = [
    {
      id: "name",
      header: t("client:admin.supporters.plan.name"),
      wrap: "anywhere",
      cell: (plan) => (
        <Link href={`/admin/supporters/plans/${encodeURIComponent(plan.id)}`} variant="standalone">
          <span dir="auto" lang="">
            {plan.name}
          </span>
        </Link>
      ),
    },
    {
      id: "state",
      header: t("client:admin.supporters.plan.state"),
      cell: (plan) => t(planStateLabel(plan)),
    },
    {
      id: "created_at",
      header: t("client:admin.supporters.plan.created_at"),
      cell: (plan) => <DateTime value={plan.created_at} />,
    },
  ];
  const states = [
    { id: anyChoice, name: t("client:admin.supporters.filter.any_state") },
    ...supporterPlanStates.map((value) => ({ id: value, name: t(filterLabels[value]) })),
  ];
  const create = (
    <ButtonLink href="/admin/supporters/plans/new" variant="primary">
      {t("client:admin.supporters.new.title")}
    </ButtonLink>
  );
  return (
    <SupporterPages current="plans">
      <Stack gap="4">
        <Cluster gap="4" justify="between" align="end">
          <ListToolbar label={t("client:admin.list.filters")}>
            <Select
              variant="toolbar"
              aria-label={t("client:admin.supporters.filter.state")}
              value={state ?? anyChoice}
              onChange={(key) => {
                onStateChange(supporterPlanStates.find((value) => value === key));
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
          {create}
        </Cluster>
        {/* Another filter is another list: it starts at its first page. */}
        <RecordTable
          key={state ?? anyChoice}
          list={wholeList(plans)}
          caption={t("client:admin.supporters.plans.title")}
          columns={columns}
          getId={(plan) => plan.id}
          headingLevel={2}
          empty={
            <EmptyState
              headingLevel={2}
              heading={t(
                state === undefined
                  ? "client:admin.supporters.plans.empty"
                  : "client:admin.supporters.plans.none_in_state",
              )}
              {...(state === undefined && { description: t("client:admin.supporters.plans.empty.description") })}
            />
          }
        />
      </Stack>
    </SupporterPages>
  );
}
