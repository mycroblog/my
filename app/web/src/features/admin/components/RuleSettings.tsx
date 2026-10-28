// SPDX-License-Identifier: AGPL-3.0-only
// The rules screen below its heading (manage_rules, FR-705): the form of a new rule, then the rules in effect in
// order. A rule has a text (one line) and an optional hint, written once in any language; the about page and the
// report form show them as they are in every language. "Move up" and "Move down" reorder the rules with the keyboard
// as well as the pointer (no dragging); focus then moves to the moved rule's heading. Editing opens a dialog with the
// text and the hint; retiring cannot be undone, so it asks first (an alert dialog). What a change did stays on the
// page in a StatusMessage above the list; a failure stays there too (role="alert", web-client/README.md E2).
import {
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  Dialog,
  DialogTrigger,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  Stack,
  StatusMessage,
  Text,
  TextArea,
  TextField,
} from "@my/ui";
import { useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval, useFocusHeadingAfterMove } from "../../../lib/useFocusAfterRemoval";
import { Section, RequirePermission } from "../../moderation";
import { type Rule, ruleHintLimit, ruleTextLimit } from "../api/rules";
import {
  type RuleActions,
  type RuleFields,
  useNewRuleForm,
  useRuleActions,
  useRuleDraft,
  useRuleList,
} from "../hooks/useRules";

export function RuleSettings() {
  const t = useT();
  return (
    <RequirePermission permission="manage_rules">
      <Stack gap="8">
        <Hint size="inherit">{t("client:admin.rules.description")}</Hint>
        <Section surface="card" title={t("client:admin.rules.new.title")}>
          <NewRuleForm />
        </Section>
        <Section surface="card" title={t("client:admin.rules.list.title")}>
          <RuleList />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

/** The text and hint fields of a rule. */
function RuleTextFields({ fields, autoFocus = false }: { fields: RuleFields; autoFocus?: boolean }) {
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

function NewRuleForm() {
  const t = useT();
  const form = useNewRuleForm();
  return (
    <Stack as="form" gap="6" onSubmit={form.submit}>
      <RuleTextFields fields={form.draft.fields} />
      {form.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.rules.add_failed", { reason: form.error })}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.rules.add")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function RuleList() {
  const t = useT();
  const rules = useRuleList();
  const listRef = useRef<HTMLOListElement>(null);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const removed = useFocusAfterRemoval(
    listRef,
    rules.map((rule) => rule.id),
  );
  const focusHeading = useFocusHeadingAfterMove(listRef);
  const actions = useRuleActions((rule, done, change) => {
    if (change === "retired") removed(rule.id);
    else if (change === "moved") focusHeading(rule.id);
    setStatus(done);
  });
  return (
    <Stack gap="3">
      <StatusMessage>{status}</StatusMessage>
      {actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.rules.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      {rules.length === 0 ? (
        <EmptyState
          headingLevel={3}
          heading={t("client:admin.rules.empty")}
          description={t("client:admin.rules.empty.description")}
        />
      ) : (
        <Stack as="ol" divided ref={listRef}>
          {rules.map((rule, index) => (
            <Box as="li" container paddingY="3" key={rule.id} data-row-id={rule.id}>
              <RuleRow rule={rule} place={index + 1} count={rules.length} actions={actions} />
            </Box>
          ))}
        </Stack>
      )}
    </Stack>
  );
}

function RuleRow({ rule, place, count, actions }: { rule: Rule; place: number; count: number; actions: RuleActions }) {
  const t = useT();
  const name = rule.text;
  const isMoving = actions.movingIds.includes(rule.id);
  return (
    <Stack gap="3">
      <Heading level={3} wrap="anywhere" focusable>
        {/* The rule is written by an administrator in any language: U+2068 FIRST STRONG ISOLATE … U+2069 POP DIRECTIONAL ISOLATE. */}
        {t("client:admin.rules.heading", { place: String(place), rule: `\u2068${name}\u2069` })}
      </Heading>
      {rule.hint !== undefined && (
        // Written by an administrator in a language the app does not know: lang="" rather than the app's (Y10).
        <Text as="p" tone="muted" wrap="anywhere" whitespace="preWrap" dir="auto" lang="">
          {rule.hint}
        </Text>
      )}
      <Cluster gap="2">
        {/* The first rule cannot move up nor the last down: those buttons stay out rather than doing nothing. */}
        {place > 1 && (
          <Button
            size="sm"
            isPending={isMoving}
            aria-label={t("client:admin.rules.move_up_rule", { rule: name })}
            onPress={() => {
              actions.move(rule, place - 1);
            }}
          >
            {t("client:admin.rules.move_up")}
          </Button>
        )}
        {place < count && (
          <Button
            size="sm"
            isPending={isMoving}
            aria-label={t("client:admin.rules.move_down_rule", { rule: name })}
            onPress={() => {
              actions.move(rule, place + 1);
            }}
          >
            {t("client:admin.rules.move_down")}
          </Button>
        )}
        <EditRule rule={rule} name={name} actions={actions} />
        <RetireRule rule={rule} name={name} actions={actions} />
      </Cluster>
    </Stack>
  );
}

type RuleActionProps = { rule: Rule; name: string; actions: RuleActions };

/** The text and the hint of a rule in a dialog; it closes once they are saved, and keeps a failure inside. */
function EditRule({ rule, name, actions }: RuleActionProps) {
  const t = useT();
  return (
    <DialogTrigger>
      <Button size="sm" aria-label={t("client:admin.rules.edit_rule", { rule: name })}>
        {t("client:admin.rules.edit")}
      </Button>
      <Dialog
        title={t("client:admin.rules.edit.title")}
        closeLabel={t("client:admin.rules.edit.close")}
        isDismissable={false}
        size="md"
      >
        {({ close }) => <EditRuleForm rule={rule} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function EditRuleForm({ rule, actions, close }: { rule: Rule; actions: RuleActions; close: () => void }) {
  const t = useT();
  const draft = useRuleDraft(rule);
  const [isSaving, setIsSaving] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <Stack
      as="form"
      gap="4"
      onSubmit={(event) => {
        event.preventDefault();
        if (isSaving) return;
        const input = draft.input();
        if (input === null) return;
        setIsSaving(true);
        setFailed(false);
        void actions.update(rule, input).then((saved) => {
          setIsSaving(false);
          if (saved) close();
          else setFailed(true);
        });
      }}
    >
      <RuleTextFields fields={draft.fields} autoFocus />
      {failed && actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.rules.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      <Cluster gap="2" justify="end">
        <Button onPress={close}>{t("client:admin.rules.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.rules.save")}
        </Button>
      </Cluster>
    </Stack>
  );
}

/** Retiring a rule, after an alert dialog: it cannot be undone. */
function RetireRule({ rule, name, actions }: RuleActionProps) {
  const t = useT();
  const [isRetiring, setIsRetiring] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button size="sm" variant="danger" aria-label={t("client:admin.rules.retire_rule", { rule: name })}>
        {t("client:admin.rules.retire")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.rules.retire.title", { rule: name })}
        description={t("client:admin.rules.retire.description")}
        confirmLabel={t("client:admin.rules.retire")}
        cancelLabel={t("client:admin.rules.cancel")}
        isPending={isRetiring}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.rules.change_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsRetiring(true);
          setFailed(false);
          void actions.retire(rule).then((retired) => {
            setIsRetiring(false);
            if (retired) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}
