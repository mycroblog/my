// SPDX-License-Identifier: AGPL-3.0-only

// The announcement of the closure, at /admin/closure/announcement (FR-808, ADR-0088): a form saved as a whole, the
// closing date and time, the deadline of the export (30 days after the closing date when left empty) and the
// operator's message to the users, started from the announcement in effect. Sending it notifies and emails every
// user, so it is asked first, with what follows that withdrawing does not undo; a notice of less than 30 days is
// allowed and said beside the button before it is sent. A server that is closed already announces nothing.

import { localInput, localInstant } from "@my/app-components";
import {
  closureClosed,
  closureFieldOf,
  closureStateRefused,
  useAdminClosure,
  useAdminClosureWrites,
  type AdminClosure,
} from "@my/app-data";
import {
  BackButton,
  Button,
  ConfirmDialog,
  DateTimeField,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  SettingRow,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
  useTimeZone,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useStaffStrings } from "../parts";
import { LeaveDialog, ScreenNotice, type AdminSubProps } from "../settings/parts";

/** The address of the closure. */
const adminClosurePath = "/admin/closure";

/** The longest message the server takes. */
const messageLimit = 5000;

/** A notice shorter than this is short: the users may not have the time to take their data away. */
const shortNotice = 30 * 24 * 60 * 60 * 1000;

/** What the form holds: the dates as their fields hold them (`datetime-local`), and the message. */
type Form = { readonly closes: string; readonly until: string; readonly message: string };

/** The fields of the form, as the server's 422 names them. */
type Field = "closes_at" | "export_until" | "message";

/** What came of the last press of the form's button. */
type Sent =
  | { readonly kind: "idle" | "asking" | "sending" }
  | { readonly kind: "refused"; readonly field?: Field | undefined; readonly words: string };

/** The way back to the closure. */
function BackToClosure({ go }: { go: (href: string) => void }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(adminClosurePath);
      }}
    >
      {t("staff.entry.closure")}
    </BackButton>
  );
}

/** The form of the announcement, started from `closure`'s. */
function AnnounceForm({ closure, go, onUnsaved, leaving }: AdminSubProps & { closure: AdminClosure }): ReactNode {
  const { t } = useStaffStrings();
  const id = useId();
  const zone = useTimeZone();
  const writes = useAdminClosureWrites();
  const changing = closure.state === "announced";
  const [start] = useState<Form>(() => ({
    closes: closure.closes_at === undefined ? "" : localInput(new Date(closure.closes_at), zone),
    until: closure.export_until === undefined ? "" : localInput(new Date(closure.export_until), zone),
    message: closure.message ?? "",
  }));
  const [form, setForm] = useState(start);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const [now] = useState(() => Date.now());
  const focusNext = useRef<string | undefined>(undefined);
  const busy = sent.kind === "sending";
  const unsaved = form.closes !== start.closes || form.until !== start.until || form.message !== start.message;
  const control = (field: Field) => `${id}-${field}`;

  useEffect(() => {
    const next = focusNext.current;
    if (next === undefined) return;
    focusNext.current = undefined;
    focusRefusedField(next);
  });
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );

  const closes = localInstant(form.closes, zone);
  const until = form.until === "" ? undefined : localInstant(form.until, zone);
  const short = !Number.isNaN(closes.getTime()) && closes.getTime() - now < shortNotice;

  const change = (next: Partial<Form>) => {
    setForm((was) => ({ ...was, ...next }));
    setSent((was) => (was.kind === "refused" ? { kind: "idle" } : was));
  };

  const refuse = (field: Field, words: string) => {
    setSent({ kind: "refused", field, words });
    // A date's row holds its field at `<row>-control`; the message is its own field.
    focusNext.current = field === "message" ? control(field) : `${control(field)}-control`;
  };

  /** Checks the form before it is asked: the closing date given and whole, the deadline whole and not before it. */
  const ask = () => {
    if (busy) return;
    if (Number.isNaN(closes.getTime())) {
      refuse("closes_at", t("staff_closure.form.closes.missing"));
      return;
    }
    if (until !== undefined && (Number.isNaN(until.getTime()) || until.getTime() < closes.getTime())) {
      refuse("export_until", t("staff_closure.form.until.before"));
      return;
    }
    setSent({ kind: "asking" });
  };

  const send = () => {
    setSent({ kind: "sending" });
    writes
      .announce({
        closes_at: closes.toISOString(),
        ...(until === undefined ? {} : { export_until: until.toISOString() }),
        ...(form.message.trim() === "" ? {} : { message: form.message }),
      })
      .then(
        () => {
          onUnsaved?.(false);
          go(adminClosurePath);
        },
        (error: unknown) => {
          const field = closureFieldOf(error);
          if (field === "closes_at" || field === "export_until" || field === "message")
            refuse(field, t("staff_closure.form.refused", { field }));
          else
            setSent({
              kind: "refused",
              words: t(closureStateRefused(error) ? "staff_closure.form.closed" : "staff_settings.save.failed"),
            });
        },
      );
  };

  const fault = (field: Field) => (sent.kind === "refused" && sent.field === field ? sent.words : undefined);
  const aboutId = `${id}-short`;
  return (
    <>
      <SettingsScreen
        sub
        title={t(changing ? "staff_closure.change" : "staff_closure.announce")}
        back={<BackToClosure go={go} />}
        foot={
          <SubmitBar
            about={short ? { id: aboutId, text: t("staff_closure.form.short") } : undefined}
            result={
              sent.kind === "refused" ? (
                <FormAlert>{sent.field === undefined ? sent.words : t("staff_settings.save.refused")}</FormAlert>
              ) : undefined
            }
          >
            <Button kind="primary" busy={busy} onPress={ask}>
              {t(changing ? "staff_settings.save" : "staff_closure.announce.press")}
            </Button>
          </SubmitBar>
        }
      >
        <SettingsSection heading={t("staff_closure.form.dates")} about={t("staff_closure.form.until.about")}>
          <SettingRow id={control("closes_at")} label={t("staff_closure.form.closes")} form error={fault("closes_at")}>
            <DateTimeField
              id={`${control("closes_at")}-control`}
              label={t("staff_closure.form.closes")}
              value={form.closes}
              onChange={(value) => {
                change({ closes: value });
              }}
              invalid={fault("closes_at") !== undefined}
              describedBy={fault("closes_at") === undefined ? undefined : `${control("closes_at")}-error`}
              disabled={busy}
            />
          </SettingRow>
          <SettingRow
            id={control("export_until")}
            label={t("staff_closure.form.until")}
            form
            error={fault("export_until")}
          >
            <DateTimeField
              id={`${control("export_until")}-control`}
              label={t("staff_closure.form.until")}
              value={form.until}
              min={form.closes === "" ? undefined : form.closes}
              onChange={(value) => {
                change({ until: value });
              }}
              invalid={fault("export_until") !== undefined}
              describedBy={fault("export_until") === undefined ? undefined : `${control("export_until")}-error`}
              disabled={busy}
            />
          </SettingRow>
        </SettingsSection>
        <SettingsSection heading={t("staff_closure.form.message")}>
          <TextField
            id={control("message")}
            name="message"
            label={t("staff_closure.form.message")}
            labelHidden
            multiline
            rows={4}
            maxLength={messageLimit}
            defaultValue={form.message}
            error={fault("message")}
            onChange={(value) => {
              change({ message: value });
            }}
          />
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {sent.kind === "asking" || sent.kind === "sending" ? (
          <ConfirmDialog
            key="announce"
            title={t(changing ? "staff_closure.ask.change.title" : "staff_closure.ask.announce.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t(changing ? "staff_settings.save" : "staff_closure.announce.press")}
            busy={busy}
            onCancel={() => {
              setSent({ kind: "idle" });
            }}
            onConfirm={send}
          >
            {t(changing ? "staff_closure.ask.change.text" : "staff_closure.ask.announce.text")}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** The announcement's form once the closure is read; a closed server's says it announces nothing. */
export function ClosureAnnouncementScreen(sub: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminClosure();
  const closure = read.closure;
  if (closure === undefined || closureClosed(closure.state))
    return (
      <SettingsScreen
        sub
        title={t("staff_closure.announce")}
        back={<BackToClosure go={sub.go} />}
        notice={
          closure === undefined ? (
            <ScreenNotice
              status={read.status === "failed" ? "failed" : "loading"}
              retry={read.retry}
              words={{ loading: t("staff_closure.loading"), failed: t("staff_closure.failed") }}
            />
          ) : undefined
        }
      >
        {closure !== undefined && (
          <SettingsSection heading={t("staff_closure.announcement")}>
            <SettingsNote instead>{t("staff_closure.form.closed")}</SettingsNote>
          </SettingsSection>
        )}
      </SettingsScreen>
    );
  return <AnnounceForm closure={closure} {...sub} />;
}

/** The title of the announcement's screen: changing the dates of one in effect, or announcing one. */
export function useClosureAnnouncementTitle(): string | undefined {
  const { t } = useStaffStrings();
  const closure = useAdminClosure().closure;
  if (closure === undefined) return undefined;
  return t(closure.state === "announced" ? "staff_closure.change" : "staff_closure.announce");
}
