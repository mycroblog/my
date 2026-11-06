// SPDX-License-Identifier: AGPL-3.0-only
// The page of one rule below its heading (manage_rules, FR-705): what it is (its text, hint, place or retirement, and
// dates), then, for a rule in effect, its text and hint in one form with one save (the result beside the button,
// FormActionBar), and retiring it after an alert dialog, which cannot be undone. A retired rule only shows its facts:
// reports keep citing it, and it can no longer change. Once a rule is retired the form and the retire region go away,
// so focus moves to the heading of its facts, whose StatusMessage says what happened.
import {
  Button,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  FormActionBar,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import { DateTime, RequirePermission, Section, useFocusAfterChange } from "../../moderation";
import type { Rule } from "../api/rules";
import { useRetireRule, useRule, useRuleForm } from "../hooks/useRules";
import { RuleText, RuleTextFields } from "./ruleParts";

export function RuleDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_rules">
      <RuleScreen id={id} />
    </RequirePermission>
  );
}

function RuleScreen({ id }: { id: string }) {
  const t = useT();
  const { rule, place } = useRule(id);
  const factsHeadingRef = useRef<HTMLHeadingElement>(null);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const focusFacts = useFocusAfterChange(factsHeadingRef, rule.retired_at);
  const inEffect = rule.retired_at === undefined;
  return (
    <Stack gap="8">
      <Section surface="card" title={t("client:admin.rule.facts")} headingRef={factsHeadingRef}>
        <Stack gap="3">
          <StatusMessage>{status}</StatusMessage>
          <RuleFacts rule={rule} place={place} />
        </Stack>
      </Section>
      {inEffect && (
        <Section surface="card" title={t("client:admin.rules.edit.title")}>
          <RuleForm rule={rule} />
        </Section>
      )}
      {inEffect && (
        <Section
          surface="card"
          title={t("client:admin.rule.retire.title")}
          description={t("client:admin.rules.retire.description")}
        >
          <RetireRule
            rule={rule}
            onRetired={(done) => {
              focusFacts();
              setStatus(done);
            }}
          />
        </Section>
      )}
    </Stack>
  );
}

function RuleFacts({ rule, place }: { rule: Rule; place: number | undefined }) {
  const t = useT();
  return (
    <DescriptionList columns={2}>
      <DescriptionItem term={t("client:admin.rules.text")}>
        <Text wrap="anywhere">
          <RuleText text={rule.text} />
        </Text>
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.rules.hint")}>
        {rule.hint === undefined ? (
          t("client:admin.rule.no_hint")
        ) : (
          // Written by an administrator in a language the app does not know: lang="" rather than the app's (Y10).
          <Text wrap="anywhere" whitespace="preWrap" dir="auto" lang="">
            {rule.hint}
          </Text>
        )}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.rule.state")}>
        <Text weight="semibold">
          {rule.retired_at === undefined
            ? t("client:admin.rules.filter.active")
            : t("client:admin.rules.filter.retired")}
        </Text>
      </DescriptionItem>
      {place !== undefined && <DescriptionItem term={t("client:admin.rules.place")}>{String(place)}</DescriptionItem>}
      <DescriptionItem term={t("client:admin.rule.created_at")}>
        <DateTime value={rule.created_at} />
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.rules.revised_at")}>
        <DateTime value={rule.revised_at} />
      </DescriptionItem>
      {rule.retired_at !== undefined && (
        <DescriptionItem term={t("client:admin.rules.retired_at")}>
          <DateTime value={rule.retired_at} />
        </DescriptionItem>
      )}
    </DescriptionList>
  );
}

function RuleForm({ rule }: { rule: Rule }) {
  const t = useT();
  const form = useRuleForm(rule);
  return (
    <Stack as="form" container gap="6" onSubmit={form.submit}>
      <RuleTextFields fields={form.draft.fields} />
      <FormActionBar result={form.result}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.rules.save")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}

/** Retiring the rule, after an alert dialog: it cannot be undone. A failure stays in the dialog. */
function RetireRule({ rule, onRetired }: { rule: Rule; onRetired: (status: string) => void }) {
  const t = useT();
  const retire = useRetireRule(rule, onRetired);
  const [error, setError] = useState<string | undefined>(undefined);
  return (
    <DialogTrigger
      onOpenChange={() => {
        setError(undefined);
      }}
    >
      <Button variant="danger">{t("client:admin.rules.retire")}</Button>
      <ConfirmDialog
        title={t("client:admin.rules.retire.title", { rule: rule.text })}
        description={t("client:admin.rules.retire.description")}
        confirmLabel={t("client:admin.rules.retire")}
        cancelLabel={t("client:admin.rules.cancel")}
        isPending={retire.isPending}
        {...(error !== undefined && { errorMessage: error })}
        onConfirm={(close) => {
          setError(undefined);
          void retire.retire().then((failed) => {
            if (failed === undefined) close();
            else setError(failed);
          });
        }}
      />
    </DialogTrigger>
  );
}
