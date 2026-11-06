// SPDX-License-Identifier: AGPL-3.0-only
// The page of a new rule below its heading (manage_rules, FR-705): its text and optional hint, added after the other
// rules with one button. Once it is added, the rule's page opens and a toast says so; a failure stays beside the
// button (FormActionBar) with the fields as typed.
import { Button, FormActionBar, Hint, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { RequirePermission } from "../../moderation";
import { useNewRuleForm } from "../hooks/useRules";
import { RuleTextFields } from "./ruleParts";

export function NewRule() {
  return (
    <RequirePermission permission="manage_rules">
      <NewRuleForm />
    </RequirePermission>
  );
}

function NewRuleForm() {
  const t = useT();
  const form = useNewRuleForm();
  return (
    <Stack as="form" container gap="6" onSubmit={form.submit}>
      <Hint size="inherit">{t("client:admin.rules.new.description")}</Hint>
      <RuleTextFields fields={form.draft.fields} />
      <FormActionBar result={form.result}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.rules.add")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}
