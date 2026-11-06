// SPDX-License-Identifier: AGPL-3.0-only
// The rules screen below its heading (manage_rules, FR-705): "Add a rule" (a link to its page), then the rules in
// effect in order, or the retired ones (the filter, in the URL). Each rule's text links to its page, where it is
// changed and retired. "Move up" and "Move down" reorder the rules in effect with the keyboard as well as the pointer
// (no dragging); focus then moves to the moved rule's name. The rules in effect are at most 100, so they are one page
// and a move never takes a rule to another page; the retired ones accumulate and are paged 40 at a time.
// What a move did stays above the list in a StatusMessage; a failure stays there too (role="alert", web-client/README.md E2).
import {
  Button,
  ButtonLink,
  Cluster,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  Link,
  ListToolbar,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
} from "@my/ui";
import { useRef, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusHeadingAfterMove } from "../../../lib/useFocusAfterRemoval";
import { DateTime, type RecordColumns, RecordTable, RequirePermission, wholeList } from "../../moderation";
import type { Rule } from "../api/rules";
import { type RuleMoves, rulePath, useRetiredRules, useRuleList, useRuleMoves } from "../hooks/useRules";
import { RuleText } from "./ruleParts";

/** Which rules the list shows. */
export const ruleFilters = ["active", "retired"] as const;
export type RuleFilter = (typeof ruleFilters)[number];

const filterLabels: Readonly<Record<RuleFilter, ClientMessageKey>> = {
  active: "client:admin.rules.filter.active",
  retired: "client:admin.rules.filter.retired",
};

function isFilter(value: unknown): value is RuleFilter {
  return ruleFilters.some((filter) => filter === value);
}

/** The most rules in effect (GET /admin/rules): the list of them is one page. */
const maxRulesInEffect = 100;

type RuleSettingsProps = {
  filter: RuleFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: RuleFilter) => void;
};

export function RuleSettings({ filter, onFilterChange }: RuleSettingsProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_rules">
      <Stack gap="6">
        <Stack gap="3" align="start">
          <Hint size="inherit">{t("client:admin.rules.description")}</Hint>
          <ButtonLink href="/admin/rules/new" variant="primary">
            {t("client:admin.rules.new.title")}
          </ButtonLink>
        </Stack>
        <Stack gap="3">
          <Heading level={2}>
            {t(filter === "active" ? "client:admin.rules.list.title" : "client:admin.rules.retired.title")}
          </Heading>
          <ListToolbar label={t("client:admin.list.filters")}>
            <Select
              variant="toolbar"
              aria-label={t("client:admin.rules.filter.label")}
              value={filter}
              onChange={(key) => {
                if (isFilter(key)) onFilterChange(key);
              }}
            >
              {ruleFilters.map((item) => (
                <SelectItem key={item} id={item}>
                  {t(filterLabels[item])}
                </SelectItem>
              ))}
            </Select>
          </ListToolbar>
          {filter === "active" ? <RulesInEffect /> : <RetiredRules />}
        </Stack>
      </Stack>
    </RequirePermission>
  );
}

/** A rule's text as the link to its page. */
function RuleLink({ rule }: { rule: Rule }) {
  return (
    <Link href={rulePath(rule.id)} variant="standalone" wrap="anywhere">
      <RuleText text={rule.text} />
    </Link>
  );
}

/** A rule in effect with its place among the others, from 1. */
type PlacedRule = { rule: Rule; place: number };

function RulesInEffect() {
  const t = useT();
  const rules = useRuleList();
  const listRef = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const focusName = useFocusHeadingAfterMove(listRef);
  const moves = useRuleMoves((rule, done) => {
    focusName(rule.id);
    setStatus(done);
  });
  const placed: PlacedRule[] = rules.map((rule, index) => ({ rule, place: index + 1 }));
  const columns: RecordColumns<PlacedRule> = [
    {
      id: "rule",
      header: t("client:admin.rules.text"),
      wrap: "anywhere",
      cell: ({ rule }) => <RuleLink rule={rule} />,
    },
    { id: "place", header: t("client:admin.rules.place"), numeric: true, cell: ({ place }) => String(place) },
    {
      id: "revised_at",
      header: t("client:admin.rules.revised_at"),
      cell: ({ rule }) => <DateTime value={rule.revised_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: ({ rule, place }) => <MoveButtons rule={rule} place={place} count={rules.length} moves={moves} />,
    },
  ];
  return (
    <Stack gap="3">
      <StatusMessage>{status}</StatusMessage>
      {moves.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.rules.change_failed", { reason: moves.error })}
        </InlineError>
      )}
      <RecordTable
        list={wholeList(placed, maxRulesInEffect)}
        caption={t("client:admin.rules.list.title")}
        columns={columns}
        getId={({ rule }) => rule.id}
        headingLevel={3}
        listRef={listRef}
        empty={
          <EmptyState
            headingLevel={3}
            heading={t("client:admin.rules.empty")}
            description={t("client:admin.rules.empty.description")}
          />
        }
      />
    </Stack>
  );
}

/**
 * Moving a rule one place. The first rule cannot move up nor the last down: those buttons are disabled, so that every
 * row has its buttons in the same places. The names start with the words on the buttons (WCAG 2.5.3).
 */
function MoveButtons({ rule, place, count, moves }: { rule: Rule; place: number; count: number; moves: RuleMoves }) {
  const t = useT();
  const isMoving = moves.movingIds.includes(rule.id);
  return (
    <Cluster gap="2">
      <Button
        size="sm"
        isPending={isMoving}
        isDisabled={place === 1}
        aria-label={t("client:admin.rules.move_up_rule", { rule: rule.text })}
        onPress={() => {
          moves.move(rule, place - 1);
        }}
      >
        {t("client:admin.rules.move_up")}
      </Button>
      <Button
        size="sm"
        isPending={isMoving}
        isDisabled={place === count}
        aria-label={t("client:admin.rules.move_down_rule", { rule: rule.text })}
        onPress={() => {
          moves.move(rule, place + 1);
        }}
      >
        {t("client:admin.rules.move_down")}
      </Button>
    </Cluster>
  );
}

function RetiredRules() {
  const t = useT();
  const rules = useRetiredRules();
  const columns: RecordColumns<Rule> = [
    { id: "rule", header: t("client:admin.rules.text"), wrap: "anywhere", cell: (rule) => <RuleLink rule={rule} /> },
    {
      id: "retired_at",
      header: t("client:admin.rules.retired_at"),
      cell: (rule) => (rule.retired_at === undefined ? null : <DateTime value={rule.retired_at} />),
    },
  ];
  return (
    <RecordTable
      list={wholeList(rules)}
      caption={t("client:admin.rules.retired.title")}
      columns={columns}
      getId={(rule) => rule.id}
      headingLevel={3}
      empty={
        <EmptyState
          headingLevel={3}
          heading={t("client:admin.rules.retired.empty")}
          description={t("client:admin.rules.retired.empty.description")}
        />
      }
    />
  );
}
