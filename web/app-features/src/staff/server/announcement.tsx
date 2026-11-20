// SPDX-License-Identifier: AGPL-3.0-only

// An announcement as a holder of `manage_announcements` writes it (FR-726, ADR-0125): a new one at
// /admin/announcements/new, and one of them at /admin/announcements/{id}. The form, saved as a whole, is its title in
// plain text, its text in Markdown in the app's editor (Lexical, a chunk of its own, fetched as the form first shows),
// and when it shows: from and until a moment of the reader's zone, each optional. A new one is kept as a draft or
// published; a draft is saved and published; a published one is revised, which notifies nobody again. Publishing asks
// first: every local user is notified once it starts to show. The page of one says first where it stands, then the
// form, what readers see as last saved (their own page of it, a draft's as if published now), and, last and apart,
// Withdraw for a published one and Delete for a draft, which never showed, each asking first and not to be undone; a
// deleted draft leaves for the list. A withdrawn one is what it was, nothing to change. What is wrong with a field
// stands under it, which takes the focus.

import { localInput, localInstant } from "@my/app-components";
import {
  announcementLimits,
  announcementStanding,
  announcementWithdrawn,
  refusedMember,
  useAdminAnnouncement,
  useAdminAnnouncementWrites,
  type AdminAnnouncement,
  type AdminAnnouncementInput,
  type Announcement,
} from "@my/app-data";
import {
  Box,
  Button,
  ComposerText,
  ConfirmDialog,
  FeedNotice,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  DateTimeField,
  Row,
  SaveState,
  SettingRow,
  SettingsFacts,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
  useTimeZone,
} from "@my/design-system";
import type { EditorHandle, PlainTextEditorProps } from "@my/design-system/editor";
import { Component, lazy, useEffect, useId, useRef, useState, type ReactNode } from "react";
import { AnnouncementEntry } from "../../announcements/announcements";
import { useMoment, useStaffStrings, type T } from "../parts";
import { adminAnnouncementPath, adminAnnouncementsPath, StandingMark, useWindowWords } from "./announcements";
import { Irreversible, LeaveDialog, ScreenNotice, type AdminSubProps } from "../settings/parts";
import { BackTo, useStillThere, useUnsavedReport } from "./parts";

/** The text's editor, a chunk of its own with Lexical, fetched as the form first shows. */
const LazyEditor = lazy(() => import("@my/design-system/editor").then((m) => ({ default: m.PlainTextEditor })));

/** In the editor's place, once its chunk could not be fetched: what to do. */
class EditorGuard extends Component<{ failed: ReactNode; children: ReactNode }, { failed: boolean }> {
  override state = { failed: false };

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true };
  }

  override render(): ReactNode {
    return this.state.failed ? this.props.failed : this.props.children;
  }
}

/** The editor of the text, or what to do where its chunk could not be fetched. */
function TextEditor(props: PlainTextEditorProps) {
  const { t } = useStaffStrings();
  return (
    <EditorGuard failed={<FeedNotice alert>{t("staff_announcements.editor_failed")}</FeedNotice>}>
      <LazyEditor {...props} />
    </EditorGuard>
  );
}

/** What the form holds: the title, the text, and the window as `datetime-local` fields hold it (empty for none). */
type Form = { readonly title: string; readonly text: string; readonly starts: string; readonly ends: string };

/** The fields a refusal is said under. */
type Field = "title" | "text" | "starts" | "ends";

/** Where the form's last send is. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" | "published" }
  | { readonly kind: "refused"; readonly field?: Field | undefined; readonly why: string };

/** How many user-perceived characters `s` holds, as the server counts a title. */
const graphemes = (s: string) => [...new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(s)].length;

/** The form of a saved announcement, its window in the reader's zone. */
const formOf = (a: AdminAnnouncement, timeZone: string | undefined): Form => ({
  title: a.title,
  text: a.text,
  starts: a.starts_at === undefined ? "" : localInput(new Date(a.starts_at), timeZone),
  ends: a.ends_at === undefined ? "" : localInput(new Date(a.ends_at), timeZone),
});

const same = (a: Form, b: Form) =>
  a.title === b.title && a.text === b.text && a.starts === b.starts && a.ends === b.ends;

/** The field a member of the request is, as the server names it. */
const fieldOfMember = (member: string): Field | undefined =>
  member === "title"
    ? "title"
    : member === "text"
      ? "text"
      : member === "starts_at"
        ? "starts"
        : member === "ends_at"
          ? "ends"
          : undefined;

/** The state of an announcement's form, its checks, and its focus on a refused field. */
function useAnnouncementForm(start: Form, onUnsaved: AdminSubProps["onUnsaved"]) {
  const { t } = useStaffStrings();
  const timeZone = useTimeZone();
  const id = useId();
  const editor = useRef<EditorHandle>(null);
  const [base, setBase] = useState(start);
  const [form, setForm] = useState(start);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const focusNext = useRef<Field | undefined>(undefined);
  useEffect(() => {
    const field = focusNext.current;
    if (field === undefined) return;
    focusNext.current = undefined;
    if (field === "text") editor.current?.focus();
    else focusRefusedField(field === "title" ? `${id}-title` : `${id}-${field}-control`);
  });
  const busy = sent.kind === "sending";
  const unsaved = !same(form, base);
  useUnsavedReport(onUnsaved, unsaved, busy);
  const refuse = (field: Field | undefined, why: string) => {
    setSent({ kind: "refused", field, why });
    focusNext.current = field;
  };
  /** The moment a window field names, as the API takes it; undefined for an empty field, null for no moment. */
  const instant = (value: string): string | undefined | null => {
    if (value === "") return undefined;
    const d = localInstant(value, timeZone);
    return Number.isNaN(d.getTime()) ? null : d.toISOString();
  };
  return {
    id,
    editor,
    form,
    sent,
    busy,
    unsaved,
    change: (part: Partial<Form>) => {
      setForm((now) => ({ ...now, ...part }));
      setSent((was) => (was.kind === "sending" ? was : { kind: "idle" }));
    },
    /** The form is saved as it stands: nothing of it is unsaved any more. */
    saved: (kind: "saved" | "published") => {
      setBase(form);
      setSent({ kind });
      onUnsaved?.(false);
    },
    sending: () => {
      setSent({ kind: "sending" });
    },
    /** Says what came of a refused write: under the field the server names, or in the submit bar. */
    refused: (error: unknown) => {
      const member = refusedMember(error);
      const field = member === undefined ? undefined : fieldOfMember(member.member);
      if (field !== undefined && member !== undefined) refuse(field, member.said);
      else if (announcementWithdrawn(error)) refuse(undefined, t("staff_announcements.save.withdrawn"));
      else refuse(undefined, t("staff_announcements.save.failed"));
    },
    /** What the form finds wrong before sending, the first in the form's order; the input to send where nothing is. */
    checked: (): AdminAnnouncementInput | undefined => {
      const title = form.title.trim();
      if (title === "" || graphemes(title) > announcementLimits.title || /[\n\r]/.test(title)) {
        refuse("title", t("staff_announcements.title.rule", { max: announcementLimits.title }));
        return undefined;
      }
      if (form.text.trim() === "" || Array.from(form.text).length > announcementLimits.text) {
        refuse("text", t("staff_announcements.text.rule", { max: announcementLimits.text }));
        return undefined;
      }
      const starts = instant(form.starts);
      if (starts === null) {
        refuse("starts", t("staff_announcements.when.invalid"));
        return undefined;
      }
      const ends = instant(form.ends);
      if (ends === null) {
        refuse("ends", t("staff_announcements.when.invalid"));
        return undefined;
      }
      if (starts !== undefined && ends !== undefined && Date.parse(ends) <= Date.parse(starts)) {
        refuse("ends", t("staff_announcements.ends.after"));
        return undefined;
      }
      return {
        title,
        text: form.text,
        ...(starts === undefined ? {} : { starts_at: starts }),
        ...(ends === undefined ? {} : { ends_at: ends }),
      };
    },
  };
}

type FormState = ReturnType<typeof useAnnouncementForm>;

/** What is wrong with `field`, as the form's last send says it. */
const errorOf = (f: FormState, field: Field): string | undefined =>
  f.sent.kind === "refused" && f.sent.field === field ? f.sent.why : undefined;

/** The fields of an announcement: its title and text, and when it shows. */
function AnnouncementFields({ f }: { f: FormState }) {
  const { t } = useStaffStrings();
  const textError = errorOf(f, "text");
  const textErrorId = `${f.id}-text-error`;
  const when = (field: "starts" | "ends") => {
    const rowId = `${f.id}-${field}`;
    const error = errorOf(f, field);
    return (
      <SettingRow
        id={rowId}
        label={t(field === "starts" ? "staff_announcements.starts" : "staff_announcements.ends")}
        form
        error={error}
      >
        <DateTimeField
          id={`${rowId}-control`}
          label={t(field === "starts" ? "staff_announcements.starts" : "staff_announcements.ends")}
          value={f.form[field]}
          onChange={(value) => {
            f.change({ [field]: value });
          }}
          invalid={error !== undefined}
          describedBy={error === undefined ? undefined : `${rowId}-error`}
          disabled={f.busy}
        />
      </SettingRow>
    );
  };
  return (
    <>
      <SettingsSection
        heading={t("staff_announcements.section")}
        about={t("staff_announcements.section.about", { max: announcementLimits.title })}
      >
        <TextField
          id={`${f.id}-title`}
          name="title"
          label={t("staff_announcements.title")}
          // The section's heading says what the field holds: its own label is for assistive technology alone.
          labelHidden
          defaultValue={f.form.title}
          error={errorOf(f, "title")}
          readOnly={f.busy}
          onChange={(title) => {
            f.change({ title });
          }}
        />
      </SettingsSection>
      <SettingsSection
        heading={t("staff_announcements.text")}
        about={t("staff_announcements.text.about", { max: announcementLimits.text })}
      >
        <ComposerText error={textError} errorId={textErrorId}>
          <TextEditor
            namespace="announcement"
            look="composer"
            roomy
            value={f.form.text}
            onChange={(text) => {
              f.change({ text });
            }}
            label={t("staff_announcements.text")}
            placeholder={t("staff_announcements.text.placeholder")}
            readOnly={f.busy}
            invalid={textError !== undefined}
            describedBy={textError === undefined ? undefined : textErrorId}
            handle={f.editor}
          />
        </ComposerText>
      </SettingsSection>
      <SettingsSection heading={t("staff_announcements.when")} about={t("staff_announcements.when.about")}>
        {when("starts")}
        {when("ends")}
      </SettingsSection>
    </>
  );
}

/** What came of the form's last send, for its submit bar. */
function Result({ f }: { f: FormState }) {
  const { t } = useStaffStrings();
  if (f.sent.kind === "saved") return <SaveState state="saved" />;
  if (f.sent.kind === "published") return <SaveState state="saved">{t("staff_announcements.published")}</SaveState>;
  if (f.sent.kind === "refused")
    return <FormAlert>{f.sent.field === undefined ? f.sent.why : t("staff_announcements.save.refused")}</FormAlert>;
  return undefined;
}

/** The question before publishing: every local user is notified once it starts to show. */
function PublishDialog({ busy, onCancel, onConfirm }: { busy: boolean; onCancel: () => void; onConfirm: () => void }) {
  const { t } = useStaffStrings();
  return (
    <ConfirmDialog
      title={t("staff_announcements.publish.title")}
      cancel={t("staff_announcements.publish.cancel")}
      confirm={t("staff_announcements.publish")}
      busy={busy}
      onCancel={onCancel}
      onConfirm={onConfirm}
    >
      {t("staff_announcements.publish.text")}
    </ConfirmDialog>
  );
}

/** The way back to the announcements. */
function BackToList({ go }: { go: (href: string) => void }) {
  return <BackTo go={go} href={adminAnnouncementsPath} name="staff_announcements.entry" />;
}

/** A new announcement: kept as a draft, or published, after which its page opens. */
export function NewAnnouncementScreen({ go, onUnsaved, leaving }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const writes = useAdminAnnouncementWrites();
  const there = useStillThere();
  const f = useAnnouncementForm({ title: "", text: "", starts: "", ends: "" }, onUnsaved);
  const [asking, setAsking] = useState(false);
  const send = (publish: boolean) => {
    const input = f.checked();
    if (f.busy || input === undefined) {
      setAsking(false);
      return;
    }
    f.sending();
    writes.create(input, publish).then(
      (a) => {
        if (!there.current) return;
        setAsking(false);
        f.saved(publish ? "published" : "saved");
        go(adminAnnouncementPath(a.id));
      },
      (error: unknown) => {
        if (!there.current) return;
        setAsking(false);
        f.refused(error);
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={t("staff_announcements.new")}
        back={<BackToList go={go} />}
        foot={
          <SubmitBar result={<Result f={f} />}>
            <Row align="center" gap="screen-row-gap">
              <Button
                kind="secondary"
                isDisabled={f.busy}
                onPress={() => {
                  send(false);
                }}
              >
                {t("staff_announcements.save_draft")}
              </Button>
              <Button
                kind="primary"
                busy={f.busy}
                onPress={() => {
                  if (f.checked() !== undefined) setAsking(true);
                }}
              >
                {t("staff_announcements.publish")}
              </Button>
            </Row>
          </SubmitBar>
        }
      >
        <AnnouncementFields f={f} />
      </SettingsScreen>
      <ModalPresence>
        {asking ? (
          <PublishDialog
            key="publish"
            busy={f.busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={() => {
              send(true);
            }}
          />
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** Where an announcement stands: its standing, when it shows, and when it was published. */
function Standing({ a, t }: { a: AdminAnnouncement; t: T }) {
  const moment = useMoment();
  const windowWords = useWindowWords();
  const [now] = useState(() => Date.now());
  return (
    <SettingsSection heading={t("staff_announcements.standing.heading")}>
      <StandingMark standing={announcementStanding(a, now)} />
      <SettingsFacts
        label={t("staff_announcements.standing.heading")}
        items={[
          { text: windowWords(a) },
          ...(a.published_at === undefined
            ? []
            : [{ text: t("staff_announcements.published_at", { date: moment(a.published_at) }) }]),
          { text: t("staff_announcements.revised_at", { date: moment(a.revised_at) }) },
        ]}
      />
    </SettingsSection>
  );
}

/**
 * An announcement as its readers get it, at `now`: from its publication, or the start of its window where that is
 * later, a draft as if it were published now.
 */
function asReadersGet(a: AdminAnnouncement, now: number): Announcement {
  const published = a.published_at === undefined ? now : Date.parse(a.published_at);
  const starts = a.starts_at === undefined ? published : Math.max(published, Date.parse(a.starts_at));
  return {
    id: a.id,
    title: a.title,
    content: a.content,
    published_at: new Date(starts).toISOString(),
    ...(a.ends_at === undefined ? {} : { ends_at: a.ends_at }),
    revised_at: a.revised_at,
    read: false,
    ended: a.ends_at !== undefined && Date.parse(a.ends_at) <= now,
  };
}

/** What readers see of an announcement, as last saved: its page as theirs shows it, a draft's as if published now. */
function AsReadersSee({ a, t }: { a: AdminAnnouncement; t: T }) {
  const [now] = useState(() => Date.now());
  return (
    <SettingsSection
      heading={t("staff_announcements.preview")}
      about={t(a.state === "draft" ? "staff_announcements.preview.about_draft" : "staff_announcements.preview.about")}
    >
      <AnnouncementEntry a={asReadersGet(a, now)} />
    </SettingsSection>
  );
}

/** A draft or a published announcement: where it stands, its form, what readers see, and Delete or Withdraw last. */
function AnnouncementForm({ a, go, onUnsaved, leaving }: AdminSubProps & { a: AdminAnnouncement }) {
  const { t } = useStaffStrings();
  const timeZone = useTimeZone();
  const writes = useAdminAnnouncementWrites();
  const there = useStillThere();
  const f = useAnnouncementForm(formOf(a, timeZone), onUnsaved);
  const draft = a.state === "draft";
  const [asking, setAsking] = useState<"publish" | "withdraw" | "delete" | undefined>(undefined);
  const [withdrawing, setWithdrawing] = useState<"idle" | "sending" | "failed">("idle");
  const [deleting, setDeleting] = useState<"idle" | "sending" | "failed" | "published">("idle");
  const save = async (publish: boolean) => {
    const input = f.checked();
    if (f.busy || input === undefined) {
      setAsking(undefined);
      return;
    }
    f.sending();
    try {
      if (f.unsaved) await writes.revise(a.id, input);
      if (publish) await writes.publish(a.id);
      if (!there.current) return;
      setAsking(undefined);
      f.saved(publish ? "published" : "saved");
    } catch (error) {
      if (!there.current) return;
      setAsking(undefined);
      f.refused(error);
    }
  };
  const withdraw = () => {
    setWithdrawing("sending");
    writes.withdraw(a.id).then(
      () => {
        if (!there.current) return;
        setAsking(undefined);
        setWithdrawing("idle");
      },
      () => {
        if (!there.current) return;
        setAsking(undefined);
        setWithdrawing("failed");
      },
    );
  };
  const remove = () => {
    setDeleting("sending");
    writes.remove(a.id).then(
      () => {
        if (!there.current) return;
        setAsking(undefined);
        onUnsaved?.(false);
        go(adminAnnouncementsPath);
      },
      (error: unknown) => {
        if (!there.current) return;
        setAsking(undefined);
        setDeleting(announcementWithdrawn(error) ? "published" : "failed");
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={a.title}
        back={<BackToList go={go} />}
        // What ends the announcement stands last and apart, after the form and its Save.
        foot={
          <>
            <SubmitBar result={<Result f={f} />}>
              {draft ? (
                <Row align="center" gap="screen-row-gap">
                  <Button
                    kind="secondary"
                    isDisabled={f.busy}
                    onPress={() => {
                      void save(false);
                    }}
                  >
                    {t("staff_announcements.save")}
                  </Button>
                  <Button
                    kind="primary"
                    busy={f.busy}
                    onPress={() => {
                      if (f.checked() !== undefined) setAsking("publish");
                    }}
                  >
                    {t("staff_announcements.publish")}
                  </Button>
                </Row>
              ) : (
                <Button
                  kind="primary"
                  busy={f.busy}
                  onPress={() => {
                    void save(false);
                  }}
                >
                  {t("staff_announcements.save")}
                </Button>
              )}
            </SubmitBar>
            {/* A draft never showed to anyone, so there is nothing to withdraw: it is deleted instead. */}
            {draft && (
              <SettingsSection heading={t("staff_announcements.delete")} about={t("staff_announcements.delete.about")}>
                <Irreversible>{t("staff_announcements.irreversible")}</Irreversible>
                <Box>
                  <Button
                    size="timeline"
                    touch
                    isDisabled={f.busy}
                    onPress={() => {
                      setAsking("delete");
                    }}
                  >
                    {t("staff_announcements.delete.press")}
                  </Button>
                </Box>
                <SettingsResult>
                  {deleting === "failed"
                    ? t("staff_announcements.delete.failed")
                    : deleting === "published"
                      ? t("staff_announcements.delete.published")
                      : undefined}
                </SettingsResult>
              </SettingsSection>
            )}
            {!draft && (
              <SettingsSection
                heading={t("staff_announcements.withdraw")}
                about={t("staff_announcements.withdraw.about")}
              >
                <Irreversible>{t("staff_announcements.irreversible")}</Irreversible>
                <Box>
                  <Button
                    size="timeline"
                    touch
                    isDisabled={f.busy}
                    onPress={() => {
                      setAsking("withdraw");
                    }}
                  >
                    {t("staff_announcements.withdraw.press")}
                  </Button>
                </Box>
                <SettingsResult>
                  {withdrawing === "failed" ? t("staff_announcements.withdraw.failed") : undefined}
                </SettingsResult>
              </SettingsSection>
            )}
          </>
        }
      >
        <Standing a={a} t={t} />
        <AnnouncementFields f={f} />
        <AsReadersSee a={a} t={t} />
      </SettingsScreen>
      <ModalPresence>
        {asking === "publish" ? (
          <PublishDialog
            key="publish"
            busy={f.busy}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={() => {
              void save(true);
            }}
          />
        ) : asking === "delete" ? (
          <ConfirmDialog
            key="delete"
            title={t("staff_announcements.delete.title")}
            cancel={t("staff_announcements.delete.cancel")}
            confirm={t("staff_announcements.delete.press")}
            busy={deleting === "sending"}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={remove}
          >
            {t("staff_announcements.delete.text")}
          </ConfirmDialog>
        ) : asking === "withdraw" ? (
          <ConfirmDialog
            key="withdraw"
            title={t("staff_announcements.withdraw.title")}
            cancel={t("staff_announcements.withdraw.cancel")}
            confirm={t("staff_announcements.withdraw.press")}
            busy={withdrawing === "sending"}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={withdraw}
          >
            {t("staff_announcements.withdraw.text")}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** A withdrawn announcement: where it stands and what it said, nothing to change. */
function WithdrawnAnnouncement({ a, go }: { a: AdminAnnouncement; go: (href: string) => void }) {
  const { t } = useStaffStrings();
  return (
    <SettingsScreen sub title={a.title} back={<BackToList go={go} />}>
      <Standing a={a} t={t} />
      <AsReadersSee a={a} t={t} />
    </SettingsScreen>
  );
}

/** The page of the announcement of `id`: its form, a withdrawn one's facts, or that it loads, failed or is none. */
export function AnnouncementScreen({ id, ...sub }: AdminSubProps & { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminAnnouncement(id);
  const a = read.announcement;
  if (a === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff_announcements.one")}
        back={<BackToList go={sub.go} />}
        notice={
          <ScreenNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_announcements.loading_one"),
              failed: t("staff_announcements.failed_one"),
              missing: t("staff_announcements.missing"),
            }}
            all={{ label: t("staff_announcements.all"), href: adminAnnouncementsPath }}
          />
        }
      />
    );
  if (a.state === "withdrawn") return <WithdrawnAnnouncement a={a} go={sub.go} />;
  return <AnnouncementForm key={a.id} a={a} {...sub} />;
}
