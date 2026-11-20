// SPDX-License-Identifier: AGPL-3.0-only

// The rules of the link card crawler as a holder of `manage_settings` keeps them (ADR-0062, ADR-0058 §1): at
// /admin/link-card-domains, a rule a door to its page, by domain, each with what the crawler does not do there and
// when the rule was set, forty at a time, with Add a rule; at /admin/link-card-domains/new, a rule set by its domain,
// its mode and a note, a form saved as a whole; at /admin/link-card-domains/{domain}, the rule's mode and note as the
// same form, and, last in a section of its own, Remove, which asks first. A rule covers the domain and its subdomains;
// what the server refuses of the domain or the note stands under it, which takes the focus.

import {
  linkCardDomainLimit,
  linkCardModes,
  linkCardNoteLimit,
  loadLinkCardRules,
  refusalDetail,
  refusedField,
  useLinkCardRule,
  useLinkCardRules,
  useLinkCardRuleWrites,
  type AppData,
  type LinkCardMode,
  type LinkCardRule,
} from "@my/app-data";
import { ProblemError } from "@mycroblog/mykit";
import {
  BackButton,
  Box,
  Button,
  ChoiceList,
  ConfirmDialog,
  focusRefusedField,
  FormAlert,
  LinkButton,
  ModalPresence,
  ShowMore,
  useAddedFocus,
  SaveState,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
  TextLink,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useMoment, useStaffStrings } from "../parts";
import { LeaveDialog, ScreenNotice, type AdminSubProps } from "../settings/parts";

/** The address of the rules. */
const rulesPath = "/admin/link-card-domains";
/** The address of a new rule's form. */
const newRulePath = `${rulesPath}/new`;
/** The address of the rule of `domain`. */
const rulePath = (domain: string): string => `${rulesPath}/${encodeURIComponent(domain)}`;

/** How many rules the list shows at first, and how many more each Show more adds. */
const shownAtOnce = 40;

/** Reads the rules, for the routes' loaders. */
export const loadLinkCardDomains = (data: AppData): Promise<void> => loadLinkCardRules(data);

/** The rules, a door each, forty at a time, and Add a rule. */
export function LinkCardRulesScreen(): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const read = useLinkCardRules();
  const [shown, setShown] = useState(shownAtOnce);
  const added = useAddedFocus(shown);
  const title = t("staff_operations.entry.link_card_domains");
  if (read.rules === undefined)
    return (
      <SettingsScreen
        sub
        icon="linkCards"
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_operations.link_cards.loading"),
              failed: t("staff_operations.link_cards.failed"),
            }}
          />
        }
      />
    );
  const rules = read.rules;
  return (
    <SettingsScreen sub icon="linkCards" title={title}>
      <SettingsSection
        heading={t("staff_operations.link_cards.list")}
        count={String(rules.length)}
        action={
          <LinkButton size="timeline" href={newRulePath}>
            {t("staff_operations.link_cards.add")}
          </LinkButton>
        }
      >
        {rules.length === 0 ? (
          <SettingsEmpty>{t("staff_operations.link_cards.empty")}</SettingsEmpty>
        ) : (
          <SettingsList label={t("staff_operations.link_cards.list")}>
            {rules.slice(0, shown).map((rule, i) => (
              <SettingsListRow
                key={rule.domain}
                itemRef={added.item(i)}
                title={rule.domain}
                state={t("staff_operations.link_cards.mode", { mode: rule.mode })}
                facts={[t("staff_operations.link_cards.set_at", { date: moment(rule.updated_at) })]}
                href={rulePath(rule.domain)}
              />
            ))}
          </SettingsList>
        )}
        <ShowMore
          more={rules.length > shown}
          onPress={added.press(() => {
            setShown((n) => n + shownAtOnce);
          })}
        />
      </SettingsSection>
    </SettingsScreen>
  );
}

/** What the form of a rule holds. */
type RuleForm = { readonly domain: string; readonly mode: LinkCardMode; readonly note: string };

/** Where a rule's last save is: refused, with which field and why, or why in the submit bar. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | {
      readonly kind: "refused";
      readonly field?: "domain" | "note" | undefined;
      readonly reason?: string | undefined;
      /** The domain already has a rule, which a new one would replace: its domain as the list has it. */
      readonly taken?: string | undefined;
    };

/** The way back to the rules. */
function BackToRules({ go }: { go: (href: string) => void }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(rulesPath);
      }}
    >
      {t("staff_operations.entry.link_card_domains")}
    </BackButton>
  );
}

/** The state of a rule's form, its focus on a refused field, and whether it holds changes not saved. */
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
  const unsaved = form.domain !== base.domain || form.mode !== base.mode || form.note !== base.note;
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );
  const refuseField = (field: "domain" | "note", taken?: string) => {
    setSent({ kind: "refused", field, taken });
    focusNext.current = `${id}-${field}`;
  };
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
    /** Says what came of a refused write: the domain (a 400: not a host) or the note, which takes the focus. */
    refuse: (error: unknown) => {
      const member = refusedField(error)?.member;
      if (member === "note") refuseField("note");
      else if (error instanceof ProblemError && error.status === 400) refuseField("domain");
      else setSent({ kind: "refused", reason: refusalDetail(error) });
    },
    /**
     * What the form finds wrong before sending: no domain, or, of a new rule, a domain that has a rule already among
     * `rules`, which a save would replace without a word (the server sets a domain's rule whatever it had).
     */
    check: (rules?: readonly LinkCardRule[]): boolean => {
      const domain = form.domain.trim().toLowerCase();
      const taken = rules?.find((r) => r.domain === domain);
      if (domain !== "" && taken === undefined) return true;
      refuseField("domain", taken?.domain);
      return false;
    },
  };
}

/** The domain of a new rule, its mode and its note; a rule's page has its domain as its title. */
function RuleFields({
  id,
  form,
  sent,
  busy,
  withDomain,
  onChange,
}: {
  id: string;
  form: RuleForm;
  sent: Sent;
  busy: boolean;
  withDomain: boolean;
  onChange: (part: Partial<RuleForm>) => void;
}) {
  const { t } = useStaffStrings();
  const error = (field: "domain" | "note") =>
    sent.kind !== "refused" || sent.field !== field
      ? undefined
      : sent.taken !== undefined
        ? t("staff_operations.link_cards.taken", { domain: sent.taken })
        : t("staff_operations.link_cards.rule", { field });
  return (
    <SettingsSection heading={t("staff_operations.link_cards.section")}>
      {withDomain && (
        <TextField
          id={`${id}-domain`}
          name="domain"
          label={t("staff_operations.link_cards.domain")}
          row
          autoCapitalize="none"
          inputMode="text"
          maxLength={linkCardDomainLimit}
          defaultValue={form.domain}
          error={error("domain")}
          onChange={(domain) => {
            onChange({ domain });
          }}
        />
      )}
      {withDomain && sent.kind === "refused" && sent.taken !== undefined && (
        <Box>
          <TextLink href={rulePath(sent.taken)}>
            {t("staff_operations.link_cards.taken.open", { domain: sent.taken })}
          </TextLink>
        </Box>
      )}
      <ChoiceList
        id={`${id}-mode`}
        label={t("staff_operations.link_cards.mode.label")}
        multiple={false}
        items={linkCardModes.map((mode) => ({
          key: mode,
          label: t("staff_operations.link_cards.mode", { mode }),
        }))}
        chosen={[linkCardModes.indexOf(form.mode)]}
        onChange={(chosen) => {
          onChange({ mode: linkCardModes[chosen[0] ?? 0] ?? "no_fetch" });
        }}
        disabled={busy}
      />
      <TextField
        id={`${id}-note`}
        name="note"
        label={t("staff_operations.link_cards.note")}
        row
        multiline
        rows={3}
        maxLength={linkCardNoteLimit}
        defaultValue={form.note}
        error={error("note")}
        onChange={(note) => {
          onChange({ note });
        }}
      />
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

/** A new rule: its domain, its mode and a note; saved, it opens the rule's page. */
export function NewLinkCardRuleScreen({ go, onUnsaved, leaving }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const writes = useLinkCardRuleWrites();
  const f = useRuleForm({ domain: "", mode: "no_fetch", note: "" }, onUnsaved);
  const rules = useLinkCardRules().rules;
  const save = () => {
    if (f.busy || !f.check(rules)) return;
    f.setSent({ kind: "sending" });
    writes.set(f.form.domain.trim(), { mode: f.form.mode, note: f.form.note }).then(
      (rule) => {
        f.setBase(f.form);
        f.setSent({ kind: "saved" });
        onUnsaved?.(false);
        go(rulePath(rule.domain));
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
        icon="linkCards"
        title={t("staff_operations.link_cards.new")}
        back={<BackToRules go={go} />}
        foot={<RuleBar sent={f.sent} busy={f.busy} onSave={save} label={t("staff_operations.link_cards.add")} />}
      >
        <RuleFields id={f.id} form={f.form} sent={f.sent} busy={f.busy} withDomain onChange={f.change} />
      </SettingsScreen>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}

/** A rule as a form of its mode and note, and Remove last. */
function RuleForm({ rule, go, onUnsaved, leaving }: AdminSubProps & { rule: LinkCardRule }) {
  const { t } = useStaffStrings();
  const writes = useLinkCardRuleWrites();
  const f = useRuleForm({ domain: rule.domain, mode: rule.mode, note: rule.note }, onUnsaved);
  const [asking, setAsking] = useState(false);
  const [removing, setRemoving] = useState<"idle" | "sending" | "failed">("idle");
  const save = () => {
    if (f.busy) return;
    if (!f.unsaved) {
      go(rulesPath);
      return;
    }
    f.setSent({ kind: "sending" });
    writes.set(rule.domain, { mode: f.form.mode, note: f.form.note }).then(
      () => {
        f.setBase(f.form);
        f.setSent({ kind: "saved" });
      },
      (error: unknown) => {
        f.refuse(error);
      },
    );
  };
  const remove = () => {
    setRemoving("sending");
    writes.remove(rule.domain).then(
      () => {
        setAsking(false);
        // The rule is gone with whatever its form held: nothing is left to hold the reader back for.
        onUnsaved?.(false);
        go(rulesPath);
      },
      () => {
        setAsking(false);
        setRemoving("failed");
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        icon="linkCards"
        title={rule.domain}
        back={<BackToRules go={go} />}
        // What ends the rule stands last and apart, after the form and its Save.
        foot={
          <>
            <RuleBar sent={f.sent} busy={f.busy} onSave={save} label={t("staff_settings.save")} />
            <SettingsSection
              heading={t("staff_operations.link_cards.remove")}
              about={t("staff_operations.link_cards.remove.about")}
            >
              <Box>
                <Button
                  size="timeline"
                  touch
                  isDisabled={f.busy}
                  onPress={() => {
                    setAsking(true);
                  }}
                >
                  {t("staff_operations.link_cards.remove.press")}
                </Button>
              </Box>
              <SettingsResult>
                {removing === "failed" ? t("staff_operations.link_cards.remove.failed") : undefined}
              </SettingsResult>
            </SettingsSection>
          </>
        }
      >
        <RuleFields id={f.id} form={f.form} sent={f.sent} busy={f.busy} withDomain={false} onChange={f.change} />
      </SettingsScreen>
      <ModalPresence>
        {asking ? (
          <ConfirmDialog
            key="remove"
            title={t("staff_operations.link_cards.remove.title")}
            cancel={t("staff_operations.cancel")}
            confirm={t("staff_operations.link_cards.remove.press")}
            busy={removing === "sending"}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={remove}
          >
            {t("staff_operations.link_cards.remove.text", { domain: rule.domain })}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** The page of the rule of `domain`: its form, or that it loads, failed or is none. */
export function LinkCardRuleScreen({ domain, ...sub }: AdminSubProps & { domain: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useLinkCardRule(domain);
  if (read.rule === undefined)
    return (
      <SettingsScreen
        sub
        icon="linkCards"
        title={t("staff_operations.entry.link_card_domains")}
        back={<BackToRules go={sub.go} />}
        notice={
          <ScreenNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_operations.link_cards.loading"),
              failed: t("staff_operations.link_cards.failed"),
              missing: t("staff_operations.link_cards.missing"),
            }}
          />
        }
      />
    );
  return <RuleForm key={read.rule.domain} rule={read.rule} {...sub} />;
}

/** The title of the page of the rule of `domain`: its domain, once read. */
export function useLinkCardRuleTitle(domain: string): string | undefined {
  return useLinkCardRule(domain).rule?.domain;
}
