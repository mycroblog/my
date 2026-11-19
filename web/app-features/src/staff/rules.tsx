// SPDX-License-Identifier: AGPL-3.0-only

// The instance's rules as a holder of `manage_rules` keeps them (FR-705, ADR-0058 §1): at /admin/rules, the rules in
// effect in their order, each a door to its page, with Add a rule; the retired ones after them, muted. A rule's page,
// /admin/rules/{id}, is a form saved as a whole of its text, its hint and its place among the rules, and, last in a
// section of its own, Retire, which asks first: a retired rule leaves the rules for good, and reports that cite it
// still refer to it. A retired rule's page says what it was and when it was retired. A new rule, /admin/rules/new, is
// written as a form of its text and hint, added after the others. Each is written once, in any language, and shown as
// it is in every locale. What the server refuses of a field stands under it, which takes the focus.

import {
  loadAdminRules,
  refusalDetail,
  refusedField,
  ruleLimits,
  useAdminRule,
  useAdminRules,
  useAdminRuleWrites,
  type AdminRule,
  type AppData,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ConfirmDialog,
  focusRefusedField,
  FormAlert,
  LinkButton,
  ModalPresence,
  SaveState,
  Select,
  SettingRow,
  SettingsEmpty,
  SettingsFacts,
  SettingsList,
  SettingsListRow,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useMoment, useStaffStrings } from "./parts";
import { LeaveDialog, ScreenNotice, type AdminSubProps } from "./settings/parts";

/** The address of the rules. */
const adminRulesPath = "/admin/rules";
/** The address of a new rule's form. */
const newRulePath = `${adminRulesPath}/new`;
/** The address of the rule of `id`. */
const rulePath = (id: string): string => `${adminRulesPath}/${id}`;

/** Reads the rules, for the routes' loaders. */
export const loadRules = (data: AppData): Promise<void> => loadAdminRules(data);

/** The rules: those in effect in their order, a door each, and Add a rule; the retired ones after them. */
export function AdminRulesScreen(): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminRules();
  const title = t("staff.entry.rules");
  if (read.inEffect === undefined || read.retired === undefined)
    return (
      <SettingsScreen
        icon="list"
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_settings.rules.loading"), failed: t("staff_settings.rules.failed") }}
          />
        }
      />
    );
  const full = read.inEffect.length >= ruleLimits.inEffect;
  return (
    <SettingsScreen icon="list" title={title}>
      <SettingsSection
        heading={t("staff_settings.rules.in_effect")}
        count={String(read.inEffect.length)}
        about={full ? t("staff_settings.rules.full", { max: ruleLimits.inEffect }) : undefined}
        action={
          full ? undefined : (
            <LinkButton size="timeline" href={newRulePath}>
              {t("staff_settings.rules.add")}
            </LinkButton>
          )
        }
      >
        {read.inEffect.length === 0 ? (
          <SettingsEmpty>{t("staff_settings.rules.empty")}</SettingsEmpty>
        ) : (
          <SettingsList label={t("staff_settings.rules.in_effect")}>
            {read.inEffect.map((rule, i) => (
              <SettingsListRow
                key={rule.id}
                title={t("staff_settings.rules.numbered", { n: i + 1, text: rule.text })}
                caption={rule.hint}
                href={rulePath(rule.id)}
              />
            ))}
          </SettingsList>
        )}
      </SettingsSection>
      {read.retired.length > 0 && (
        <SettingsSection heading={t("staff_settings.rules.retired")} count={String(read.retired.length)}>
          <SettingsList label={t("staff_settings.rules.retired")}>
            {read.retired.map((rule) => (
              <SettingsListRow key={rule.id} title={rule.text} inactive href={rulePath(rule.id)} />
            ))}
          </SettingsList>
        </SettingsSection>
      )}
    </SettingsScreen>
  );
}

/** What the form of a rule holds. */
type RuleForm = { readonly text: string; readonly hint: string; readonly position: number };

/** Where a rule's last save is: refused, with what is wrong with which field, or why in the submit bar. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | { readonly kind: "refused"; readonly field?: "text" | "hint" | undefined; readonly reason?: string | undefined };

/** The rule each field follows, in the form's words: the server says only that the value is not valid. */
const ruleWords = (field: "text" | "hint") =>
  field === "text" ? ({ field, max: ruleLimits.text } as const) : ({ field, max: ruleLimits.hint } as const);

/** The text and hint of a rule as a form, and the place of a rule in effect among the others. */
function RuleFields({
  id,
  form,
  places,
  sent,
  busy,
  onChange,
}: {
  id: string;
  form: RuleForm;
  /** How many rules are in effect, the places a rule may move to; none for a new rule. */
  places?: number | undefined;
  sent: Sent;
  busy: boolean;
  onChange: (part: Partial<RuleForm>) => void;
}) {
  const { t } = useStaffStrings();
  const error = (field: "text" | "hint") =>
    sent.kind === "refused" && sent.field === field ? t("staff_settings.rules.rule", ruleWords(field)) : undefined;
  return (
    <SettingsSection heading={t("staff_settings.rules.section")}>
      <TextField
        id={`${id}-text`}
        name="text"
        label={t("staff_settings.rules.text")}
        hint={t("staff_settings.rules.text.about")}
        defaultValue={form.text}
        maxLength={ruleLimits.text}
        error={error("text")}
        onChange={(text) => {
          onChange({ text });
        }}
      />
      <TextField
        id={`${id}-hint`}
        name="hint"
        label={t("staff_settings.rules.hint")}
        hint={t("staff_settings.rules.hint.about")}
        defaultValue={form.hint}
        multiline
        rows={3}
        maxLength={ruleLimits.hint}
        error={error("hint")}
        onChange={(hint) => {
          onChange({ hint });
        }}
      />
      {places !== undefined && places > 1 && (
        <SettingRow id={`${id}-position`} label={t("staff_settings.rules.position")} form>
          <Select
            name="position"
            options={Array.from({ length: places }, (_, i) => ({
              value: String(i + 1),
              label: t("staff_settings.rules.position.choice", { n: i + 1 }),
            }))}
            value={String(form.position)}
            onChange={(v) => {
              onChange({ position: Number(v) });
            }}
            isDisabled={busy}
          />
        </SettingRow>
      )}
    </SettingsSection>
  );
}

/** The submit bar of a rule's form. */
function RuleBar({ sent, busy, onSave, label }: { sent: Sent; busy: boolean; onSave: () => void; label: string }) {
  const { t } = useStaffStrings();
  return (
    <SubmitBar
      result={
        sent.kind === "saved" ? (
          <SaveState state="saved" />
        ) : sent.kind === "refused" ? (
          <FormAlert>
            {sent.reason ?? t(sent.field === undefined ? "staff_settings.save.failed" : "staff_settings.save.refused")}
          </FormAlert>
        ) : undefined
      }
    >
      <Button kind="primary" busy={busy} onPress={onSave}>
        {label}
      </Button>
    </SubmitBar>
  );
}

/** The state of a rule form, its focus on a refused field, and whether it holds changes not saved. */
function useRuleForm(start: RuleForm, onUnsaved: AdminSubProps["onUnsaved"]) {
  const id = useId();
  const [base, setBase] = useState(start);
  const [form, setForm] = useState(start);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    focusRefusedField(focusNext.current);
    focusNext.current = undefined;
  });
  const busy = sent.kind === "sending";
  const unsaved = form.text !== base.text || form.hint !== base.hint || form.position !== base.position;
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );
  return {
    id,
    base,
    form,
    sent,
    busy,
    unsaved,
    setBase,
    setSent,
    change: (part: Partial<RuleForm>) => {
      setForm((now) => ({ ...now, ...part }));
      setSent((was) => (was.kind === "saved" || was.kind === "refused" ? { kind: "idle" } : was));
    },
    /** Says what came of a refused write: a field the server or the form names, which takes the focus. */
    refuse: (error: unknown) => {
      const member = refusedField(error)?.member;
      if (member === "text" || member === "hint") {
        setSent({ kind: "refused", field: member });
        focusNext.current = `${id}-${member}`;
      } else setSent({ kind: "refused", reason: refusalDetail(error) });
    },
    /** What the form finds wrong before sending: no text. */
    check: (): boolean => {
      if (form.text.trim() !== "") return true;
      setSent({ kind: "refused", field: "text" });
      focusNext.current = `${id}-text`;
      return false;
    },
  };
}

/** The way back to the rules. */
function BackToRules({ go }: { go: (href: string) => void }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(adminRulesPath);
      }}
    >
      {t("staff.entry.rules")}
    </BackButton>
  );
}

/** A new rule: its text and hint, added after the rules in effect. */
export function NewRuleScreen({ go, onUnsaved, leaving }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const writes = useAdminRuleWrites();
  const f = useRuleForm({ text: "", hint: "", position: 0 }, onUnsaved);
  const save = () => {
    if (f.busy || !f.check()) return;
    f.setSent({ kind: "sending" });
    writes.create({ text: f.form.text, hint: f.form.hint }).then(
      () => {
        f.setBase(f.form);
        f.setSent({ kind: "saved" });
        go(adminRulesPath);
      },
      (error: unknown) => {
        f.refuse(error);
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={t("staff_settings.rules.new")}
        back={<BackToRules go={go} />}
        foot={<RuleBar sent={f.sent} busy={f.busy} onSave={save} label={t("staff_settings.rules.add")} />}
      >
        <RuleFields id={f.id} form={f.form} sent={f.sent} busy={f.busy} onChange={f.change} />
      </SettingsScreen>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}

/** A rule in effect as a form, and Retire last. */
function RuleForm({
  rule,
  position,
  places,
  go,
  onUnsaved,
  leaving,
}: AdminSubProps & { rule: AdminRule; position: number; places: number }) {
  const { t } = useStaffStrings();
  const writes = useAdminRuleWrites();
  const f = useRuleForm({ text: rule.text, hint: rule.hint ?? "", position }, onUnsaved);
  const [asking, setAsking] = useState<"retire" | undefined>(undefined);
  const [retiring, setRetiring] = useState<"idle" | "sending" | "failed">("idle");
  const save = async () => {
    if (f.busy || !f.check()) return;
    if (!f.unsaved) {
      go(adminRulesPath);
      return;
    }
    f.setSent({ kind: "sending" });
    try {
      if (f.form.text !== f.base.text || f.form.hint !== f.base.hint)
        await writes.revise(rule.id, { text: f.form.text, hint: f.form.hint });
      if (f.form.position !== f.base.position) await writes.move(rule.id, f.form.position);
      f.setBase(f.form);
      f.setSent({ kind: "saved" });
    } catch (error) {
      f.refuse(error);
    }
  };
  const retire = () => {
    setRetiring("sending");
    writes.retire(rule.id).then(
      () => {
        setAsking(undefined);
        go(adminRulesPath);
      },
      () => {
        setAsking(undefined);
        setRetiring("failed");
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={t("staff_settings.rules.one")}
        back={<BackToRules go={go} />}
        foot={
          <RuleBar
            sent={f.sent}
            busy={f.busy}
            onSave={() => {
              void save();
            }}
            label={t("staff_settings.save")}
          />
        }
      >
        <RuleFields id={f.id} form={f.form} places={places} sent={f.sent} busy={f.busy} onChange={f.change} />
        <SettingsSection heading={t("staff_settings.rules.retire")} about={t("staff_settings.rules.retire.about")}>
          <Box>
            <Button
              size="timeline"
              touch
              isDisabled={f.busy}
              onPress={() => {
                setAsking("retire");
              }}
            >
              {t("staff_settings.rules.retire.press")}
            </Button>
          </Box>
          <SettingsResult>{retiring === "failed" ? t("staff_settings.rules.retire.failed") : undefined}</SettingsResult>
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {asking === "retire" ? (
          <ConfirmDialog
            key="retire"
            title={t("staff_settings.rules.retire.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_settings.rules.retire.press")}
            busy={retiring === "sending"}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={retire}
          >
            {t("staff_settings.rules.retire.text", { text: rule.text })}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** A retired rule: what it said, and when it was retired. */
function RetiredRule({ rule, go }: { rule: AdminRule; go: (href: string) => void }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  return (
    <SettingsScreen sub title={t("staff_settings.rules.one")} back={<BackToRules go={go} />}>
      <SettingsSection heading={t("staff_settings.rules.section")}>
        <SettingsFacts
          label={t("staff_settings.rules.section")}
          items={[
            { text: rule.text, ...(rule.hint === undefined ? {} : { about: [rule.hint] }) },
            { text: t("staff_settings.rules.retired_at", { date: moment(rule.retired_at ?? rule.revised_at) }) },
          ]}
        />
      </SettingsSection>
    </SettingsScreen>
  );
}

/** The page of the rule of `id`: its form, a retired rule's facts, or that it loads, failed or is none. */
export function RuleScreen({ id, ...sub }: AdminSubProps & { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminRule(id);
  const rules = useAdminRules();
  const rule = read.rule;
  if (rule === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff_settings.rules.one")}
        back={<BackToRules go={sub.go} />}
        notice={
          <ScreenNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_settings.rules.loading"),
              failed: t("staff_settings.rules.failed"),
              missing: t("staff_settings.rules.missing"),
            }}
          />
        }
      />
    );
  if (rule.retired_at !== undefined) return <RetiredRule rule={rule} go={sub.go} />;
  const inEffect = rules.inEffect ?? [];
  return (
    <RuleForm
      key={rule.id}
      rule={rule}
      position={inEffect.findIndex((r) => r.id === rule.id) + 1}
      places={inEffect.length}
      {...sub}
    />
  );
}
