// SPDX-License-Identifier: AGPL-3.0-only
// What the pages of the rules share (FR-705): the text and hint fields of a new or changed rule, and a rule's text as
// people read it. An administrator writes a rule once, in any language; every locale shows it as it is.
import { Stack, TextArea, TextField } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { ruleHintLimit, ruleTextLimit } from "../api/rules";
import type { RuleFields } from "../hooks/useRules";

/** The text and hint fields of a rule. */
export function RuleTextFields({ fields, autoFocus = false }: { fields: RuleFields; autoFocus?: boolean }) {
  const t = useT();
  return (
    <Stack gap="3">
      <TextField
        label={t("client:admin.rules.text")}
        description={t("client:admin.rules.text.description", { max: String(ruleTextLimit) })}
        value={fields.text}
        onChange={fields.changeText}
        autoComplete="off"
        autoFocus={autoFocus}
        {...(fields.textError === undefined ? {} : { isInvalid: true, errorMessage: fields.textError })}
      />
      <TextArea
        label={t("client:admin.rules.hint")}
        value={fields.hint}
        onChange={fields.changeHint}
        rows={2}
        characterLimit={ruleHintLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.settings.characters_over", { count: -remaining })
            : t("client:admin.settings.characters_left", { count: remaining })
        }
        {...(fields.hintError === undefined ? {} : { isInvalid: true, errorMessage: fields.hintError })}
      />
    </Stack>
  );
}

/** A rule's text, written by an administrator in a language the app does not know: `lang=""` (Y10), `dir="auto"`. */
export function RuleText({ text }: { text: string }) {
  return (
    <span dir="auto" lang="">
      {text}
    </span>
  );
}
