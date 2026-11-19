// SPDX-License-Identifier: AGPL-3.0-only

// A custom emoji as a holder of `manage_custom_emojis` keeps it (FR-111, ADR-0051), at /admin/custom-emojis/{id}: what
// can be done to it first (disable or enable it; a local one's image replaced at once), what it is (its image, where it
// comes from, who made it and when, when its image was set), then a local one's place in the picker, a form saved as
// a whole (its category, its aliases, whether the picker offers it), and Delete last and apart, which asks its
// shortcode typed first; a remote one is copied to this server under a shortcode, the copy's page opening then. A new
// local emoji, /admin/custom-emojis/new, is its shortcode, its image and its place in the picker; an image over 100 KB
// is said to be one some servers do not fetch (GoToSocial). What the server refuses of a field stands under it, which
// takes the focus.

import {
  aliasesOf,
  emojiLimits,
  emojiRefusal,
  useAdminEmoji,
  useAdminEmojiWrites,
  validShortcode,
  type AdminCustomEmoji,
  type EmojiSettings,
} from "@my/app-data";
import { sentenceHandleOf } from "@my/app-components";
import {
  Box,
  Button,
  ConfirmDialog,
  FactList,
  FileField,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  Row,
  SaveState,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useMoment, useStaffStrings, type T } from "../parts";
import { adminEmojiPath, adminEmojisPath, coded, EmojiImage, originOf } from "./emoji-parts";
import { Irreversible, LeaveDialog, OnOffRow, ScreenNotice, type AdminSubProps } from "../settings/parts";
import { BackTo, useStillThere, useUnsavedReport } from "./parts";

/** The fields a refusal is said under. */
type Field = "shortcode" | "image" | "category" | "aliases";

/** What the server's refusal of an emoji is about, and what to say there; else what to say beside the button. */
function emojiRefusalOf(t: T, error: unknown): { field?: Field; why: string } {
  switch (emojiRefusal(error)) {
    case "invalid_shortcode":
      return { field: "shortcode", why: t("staff_emoji.shortcode.rule") };
    case "shortcode_taken":
      return { field: "shortcode", why: t("staff_emoji.shortcode.taken") };
    case "invalid_alias":
      return { field: "aliases", why: t("staff_emoji.aliases.rule", { max: emojiLimits.aliases }) };
    case "invalid_category":
      return { field: "category", why: t("staff_emoji.category.rule", { max: emojiLimits.category }) };
    case "unsupported_format":
      return { field: "image", why: t("staff_emoji.image.format") };
    case "too_large":
      return { field: "image", why: t("staff_emoji.image.too_large") };
    case "too_big":
      return { field: "image", why: t("staff_emoji.image.too_big") };
    case "media_not_found":
    case "media_not_ready":
      return { field: "image", why: t("staff_emoji.image.failed") };
    case "fetch_failed":
      return { why: t("staff_emoji.copy.fetch_failed") };
    default:
  }
  if (error instanceof Error && error.name === "MediaFailedError")
    return { field: "image", why: t("staff_emoji.image.failed") };
  return { why: t("staff_emoji.save.failed") };
}

/** What a form of an emoji holds: its shortcode (a new one's), its image, and its place in the picker. */
type EmojiForm = {
  readonly shortcode: string;
  readonly file: File | undefined;
  readonly category: string;
  readonly aliases: string;
  readonly visible: boolean;
};

/** Where a form's last save is. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | { readonly kind: "refused"; readonly field?: Field | undefined; readonly why: string };

/** The state of an emoji's form, its focus on a refused field, and whether it holds changes not saved. */
function useEmojiForm(start: EmojiForm, onUnsaved: AdminSubProps["onUnsaved"]) {
  const { t } = useStaffStrings();
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
  const unsaved =
    form.shortcode !== base.shortcode ||
    form.file !== base.file ||
    form.category.trim() !== base.category.trim() ||
    aliasesOf(form.aliases).join(" ") !== aliasesOf(base.aliases).join(" ") ||
    form.visible !== base.visible;
  useUnsavedReport(onUnsaved, unsaved, busy);
  const refuse = (field: Field | undefined, why: string) => {
    setSent({ kind: "refused", field, why });
    if (field !== undefined) focusNext.current = `${id}-${field}`;
  };
  return {
    id,
    form,
    sent,
    busy,
    unsaved,
    setBase,
    setSent,
    change: (part: Partial<EmojiForm>) => {
      setForm((now) => ({ ...now, ...part }));
      setSent((was) => (was.kind === "saved" || was.kind === "refused" ? { kind: "idle" } : was));
    },
    refused: (error: unknown) => {
      const r = emojiRefusalOf(t, error);
      refuse(r.field, r.why);
    },
    /** What the form finds wrong before sending: a shortcode, an image, aliases or a category the server would refuse. */
    check: (creating: boolean): boolean => {
      if (creating && !validShortcode(form.shortcode)) refuse("shortcode", t("staff_emoji.shortcode.rule"));
      else if (creating && form.file === undefined) refuse("image", t("staff_emoji.image.missing"));
      else if (Array.from(form.category.trim()).length > emojiLimits.category)
        refuse("category", t("staff_emoji.category.rule", { max: emojiLimits.category }));
      else if (
        aliasesOf(form.aliases).length > emojiLimits.aliases ||
        aliasesOf(form.aliases).some((a) => !validShortcode(a))
      )
        refuse("aliases", t("staff_emoji.aliases.rule", { max: emojiLimits.aliases }));
      else return true;
      return false;
    },
    settings: (): EmojiSettings => ({
      category: form.category.trim(),
      aliases: aliasesOf(form.aliases),
      visibleInPicker: form.visible,
    }),
  };
}

type FormState = ReturnType<typeof useEmojiForm>;

/** What is wrong with `field`, as the form's last save says it. */
const errorOf = (f: FormState, field: Field): string | undefined =>
  f.sent.kind === "refused" && f.sent.field === field ? f.sent.why : undefined;

/** A local emoji's place in the picker: its category, its aliases, and whether the picker offers it. */
function PickerFields({ f }: { f: FormState }) {
  const { t } = useStaffStrings();
  return (
    <SettingsSection heading={t("staff_emoji.picker")} about={t("staff_emoji.picker.about")}>
      <TextField
        id={`${f.id}-category`}
        name="category"
        label={t("staff_emoji.category")}
        row
        defaultValue={f.form.category}
        maxLength={emojiLimits.category}
        error={errorOf(f, "category")}
        readOnly={f.busy}
        onChange={(category) => {
          f.change({ category });
        }}
      />
      <TextField
        id={`${f.id}-aliases`}
        name="aliases"
        label={t("staff_emoji.aliases")}
        hint={t("staff_emoji.aliases.hint")}
        row
        defaultValue={f.form.aliases}
        autoCapitalize="none"
        error={errorOf(f, "aliases")}
        readOnly={f.busy}
        onChange={(aliases) => {
          f.change({ aliases });
        }}
      />
      <OnOffRow
        id={`${f.id}-visible`}
        label={t("staff_emoji.visible")}
        on={f.form.visible}
        disabled={f.busy}
        onChange={(visible) => {
          f.change({ visible });
        }}
      />
    </SettingsSection>
  );
}

/** The submit bar of an emoji's form. */
function EmojiBar({ f, onSave, label }: { f: FormState; onSave: () => void; label: string }) {
  const { t } = useStaffStrings();
  return (
    <SubmitBar
      result={
        f.sent.kind === "saved" ? (
          <SaveState state="saved" />
        ) : f.sent.kind === "refused" ? (
          <FormAlert>{f.sent.field === undefined ? f.sent.why : t("staff_emoji.save.refused")}</FormAlert>
        ) : undefined
      }
    >
      <Button kind="primary" busy={f.busy} onPress={onSave}>
        {label}
      </Button>
    </SubmitBar>
  );
}

/** The line under an image's field: what the server takes, and that some servers do not fetch one over 100 KB. */
function imageHint(t: T, file: File | undefined): string {
  return file !== undefined && file.size > emojiLimits.warnBytes
    ? t("staff_emoji.image.heavy")
    : t("staff_emoji.image.hint");
}

/** The way back to the emoji. */
function BackToEmoji({ go }: { go: (href: string) => void }) {
  return <BackTo go={go} href={adminEmojisPath} name="staff_emoji.entry" />;
}

/** A new local emoji: its shortcode, its image, and its place in the picker. */
export function NewEmojiScreen({ go, onUnsaved, leaving }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const writes = useAdminEmojiWrites();
  const there = useStillThere();
  const f = useEmojiForm({ shortcode: "", file: undefined, category: "", aliases: "", visible: true }, onUnsaved);
  const save = () => {
    if (f.busy || !f.check(true) || f.form.file === undefined) return;
    f.setSent({ kind: "sending" });
    writes.register({ shortcode: f.form.shortcode, file: f.form.file, ...f.settings() }).then(
      (emoji) => {
        if (!there.current) return;
        f.setBase(f.form);
        f.setSent({ kind: "saved" });
        onUnsaved?.(false);
        go(adminEmojiPath(emoji.id));
      },
      (error: unknown) => {
        if (there.current) f.refused(error);
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={t("staff_emoji.new")}
        back={<BackToEmoji go={go} />}
        foot={<EmojiBar f={f} onSave={save} label={t("staff_emoji.register")} />}
      >
        <SettingsSection heading={t("staff_emoji.this")}>
          <TextField
            id={`${f.id}-shortcode`}
            name="shortcode"
            label={t("staff_emoji.shortcode")}
            hint={t("staff_emoji.shortcode.rule")}
            row
            defaultValue=""
            maxLength={30}
            autoCapitalize="none"
            error={errorOf(f, "shortcode")}
            readOnly={f.busy}
            onChange={(shortcode) => {
              f.change({ shortcode: shortcode.replace(/:/g, "") });
            }}
          />
          <FileField
            id={`${f.id}-image`}
            label={t("staff_emoji.image")}
            hint={imageHint(t, f.form.file)}
            error={errorOf(f, "image")}
            choose={t(f.form.file === undefined ? "staff_emoji.image.choose" : "staff_emoji.image.another")}
            file={f.form.file?.name}
            none={t("staff_emoji.image.none")}
            accept="image/png,image/gif,image/webp"
            row
            isDisabled={f.busy}
            onFile={(file) => {
              f.change({ file });
            }}
          />
        </SettingsSection>
        <PickerFields f={f} />
      </SettingsScreen>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}

/** What came of an action on an emoji that leaves the page as it was (disabled, a new image, a copy refused). */
type Said = { readonly text: string; readonly n: number };

/** What can be done to an emoji at once: disable or enable it, and a local one's new image. */
function EmojiActions({ emoji }: { emoji: AdminCustomEmoji }) {
  const { t } = useStaffStrings();
  const writes = useAdminEmojiWrites();
  const there = useStillThere();
  const id = useId();
  const [busy, setBusy] = useState<"state" | "image" | undefined>(undefined);
  const [said, setSaid] = useState<Said | undefined>(undefined);
  const [imageError, setImageError] = useState<string | undefined>(undefined);
  const [picked, setPicked] = useState<File | undefined>(undefined);
  const say = (text: string) => {
    setSaid((was) => ({ text, n: (was?.n ?? 0) + 1 }));
  };
  const disabled = emoji.state === "disabled";
  const toggle = () => {
    setBusy("state");
    (disabled ? writes.enable(emoji.id) : writes.disable(emoji.id)).then(
      () => {
        if (!there.current) return;
        setBusy(undefined);
        say(t(disabled ? "staff_emoji.enabled" : "staff_emoji.disabled"));
      },
      () => {
        if (!there.current) return;
        setBusy(undefined);
        say(t("staff_emoji.state.failed"));
      },
    );
  };
  const [asking, setAsking] = useState(false);
  const pick = (file: File) => {
    setPicked(file);
    setImageError(undefined);
  };
  const replace = () => {
    if (picked === undefined) return;
    setBusy("image");
    writes.replaceImage(emoji.id, picked).then(
      () => {
        if (!there.current) return;
        setBusy(undefined);
        setAsking(false);
        setPicked(undefined);
        say(t("staff_emoji.image.replaced"));
      },
      (error: unknown) => {
        if (!there.current) return;
        setBusy(undefined);
        setAsking(false);
        setImageError(emojiRefusalOf(t, error).why);
        focusRefusedField(`${id}-image`);
      },
    );
  };
  return (
    <SettingsSection
      heading={t("staff_emoji.actions")}
      about={t(disabled ? "staff_emoji.disabled.about" : "staff_emoji.enabled.about")}
    >
      <Row wrap align="center" gap="screen-row-gap">
        <Box text="title">
          <EmojiImage shortcode={emoji.shortcode} url={emoji.url} staticUrl={emoji.static_url} />
        </Box>
        <Button size="timeline" touch busy={busy === "state"} isDisabled={busy !== undefined} onPress={toggle}>
          {t(disabled ? "staff_emoji.enable" : "staff_emoji.disable")}
        </Button>
      </Row>
      {emoji.local && (
        <FileField
          id={`${id}-image`}
          label={t("staff_emoji.image.replace")}
          hint={imageHint(t, picked)}
          error={imageError}
          choose={t("staff_emoji.image.choose")}
          file={picked?.name}
          none={t("staff_emoji.image.current")}
          accept="image/png,image/gif,image/webp"
          row
          isDisabled={busy !== undefined}
          onFile={pick}
        />
      )}
      {emoji.local && picked !== undefined && imageError === undefined && (
        <Box>
          <Button
            size="timeline"
            touch
            busy={busy === "image"}
            isDisabled={busy !== undefined}
            onPress={() => {
              setAsking(true);
            }}
          >
            {t("staff_emoji.image.replace.press")}
          </Button>
        </Box>
      )}
      <SettingsResult>{said === undefined ? undefined : <span key={said.n}>{said.text}</span>}</SettingsResult>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key="replace"
            title={t("staff_emoji.image.replace.title", { shortcode: emoji.shortcode })}
            cancel={t("staff_emoji.image.replace.cancel")}
            confirm={t("staff_emoji.image.replace.press")}
            busy={busy === "image"}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={replace}
          >
            {t("staff_emoji.image.replace.text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </SettingsSection>
  );
}

/** What an emoji is: where it comes from, who made it, when it was registered and when its image was set. */
function EmojiFacts({ emoji }: { emoji: AdminCustomEmoji }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  return (
    <SettingsSection heading={t("staff_emoji.facts")}>
      <FactList
        label={t("staff_emoji.facts")}
        items={[
          { term: t("staff_emoji.origin"), value: <bdi>{originOf(t, emoji)}</bdi> },
          ...(emoji.local
            ? [
                {
                  term: t("staff_emoji.created_by"),
                  value:
                    emoji.created_by !== undefined
                      ? sentenceHandleOf(emoji.created_by)
                      : t(emoji.created_by_deleted ? "staff.account_gone" : "staff_emoji.created_by.none"),
                },
              ]
            : []),
          ...(emoji.uri === undefined ? [] : [{ term: t("staff_emoji.uri"), value: emoji.uri, code: true }]),
          { term: t(emoji.local ? "staff_emoji.created" : "staff_emoji.seen"), value: moment(emoji.created_at) },
          { term: t("staff_emoji.updated"), value: moment(emoji.updated_at) },
        ]}
      />
    </SettingsSection>
  );
}

/** A remote emoji's copy to this server, under the shortcode given (the remote one's by default). */
function CopyEmoji({ emoji, go }: { emoji: AdminCustomEmoji; go: (href: string) => void }) {
  const { t } = useStaffStrings();
  const writes = useAdminEmojiWrites();
  const there = useStillThere();
  const id = useId();
  const [shortcode, setShortcode] = useState(emoji.shortcode);
  const [state, setState] = useState<"idle" | "sending" | { field?: Field | undefined; why: string }>("idle");
  const copy = () => {
    if (!validShortcode(shortcode)) {
      setState({ field: "shortcode", why: t("staff_emoji.shortcode.rule") });
      focusRefusedField(`${id}-shortcode`);
      return;
    }
    setState("sending");
    writes.copy(emoji.id, shortcode === emoji.shortcode ? "" : shortcode).then(
      (copied) => {
        if (there.current) go(adminEmojiPath(copied.id));
      },
      (error: unknown) => {
        if (!there.current) return;
        const r = emojiRefusalOf(t, error);
        setState(r);
        if (r.field === "shortcode") focusRefusedField(`${id}-shortcode`);
      },
    );
  };
  if (emoji.url === undefined)
    return (
      <SettingsSection heading={t("staff_emoji.copy")}>
        <SettingsNote instead>{t("staff_emoji.copy.no_image")}</SettingsNote>
      </SettingsSection>
    );
  const refusal = typeof state === "object" ? state : undefined;
  return (
    <SettingsSection heading={t("staff_emoji.copy")} about={t("staff_emoji.copy.about")}>
      <TextField
        id={`${id}-shortcode`}
        name="shortcode"
        label={t("staff_emoji.copy.shortcode")}
        row
        defaultValue={emoji.shortcode}
        maxLength={30}
        autoCapitalize="none"
        error={refusal?.field === "shortcode" ? refusal.why : undefined}
        readOnly={state === "sending"}
        onChange={(value) => {
          setShortcode(value.replace(/:/g, ""));
        }}
      />
      <Row justify="start">
        <Button size="timeline" touch busy={state === "sending"} onPress={copy}>
          {t("staff_emoji.copy.press")}
        </Button>
      </Row>
      <SettingsResult>{refusal !== undefined && refusal.field === undefined ? refusal.why : undefined}</SettingsResult>
    </SettingsSection>
  );
}

/** A local emoji's page: what can be done, what it is, its place in the picker, and Delete last. */
function LocalEmoji({ emoji, go, onUnsaved, leaving }: AdminSubProps & { emoji: AdminCustomEmoji }) {
  const { t } = useStaffStrings();
  const writes = useAdminEmojiWrites();
  const there = useStillThere();
  const f = useEmojiForm(
    {
      shortcode: emoji.shortcode,
      file: undefined,
      category: emoji.category ?? "",
      aliases: emoji.aliases.join(" "),
      visible: emoji.visible_in_picker,
    },
    onUnsaved,
  );
  const [asking, setAsking] = useState(false);
  const [deleting, setDeleting] = useState<"idle" | "sending" | "failed">("idle");
  const save = () => {
    if (f.busy || !f.check(false)) return;
    if (!f.unsaved) {
      go(adminEmojisPath);
      return;
    }
    f.setSent({ kind: "sending" });
    writes.change(emoji.id, f.settings()).then(
      () => {
        if (!there.current) return;
        f.setBase(f.form);
        f.setSent({ kind: "saved" });
      },
      (error: unknown) => {
        if (there.current) f.refused(error);
      },
    );
  };
  const remove = () => {
    setDeleting("sending");
    writes.remove(emoji.id).then(
      () => {
        if (!there.current) return;
        setAsking(false);
        // The guard hears that nothing is left unsaved before the screen leaves, not at the next render.
        onUnsaved?.(false);
        go(adminEmojisPath);
      },
      () => {
        if (!there.current) return;
        setAsking(false);
        setDeleting("failed");
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={coded(emoji.shortcode)}
        back={<BackToEmoji go={go} />}
        // What ends the emoji stands last and apart, after the form and its Save.
        foot={
          <>
            <EmojiBar f={f} onSave={save} label={t("staff_emoji.save")} />
            <SettingsSection heading={t("staff_emoji.delete")} about={t("staff_emoji.delete.about")}>
              <Irreversible>{t("staff_emoji.irreversible")}</Irreversible>
              <Box>
                <Button
                  size="timeline"
                  touch
                  isDisabled={f.busy}
                  onPress={() => {
                    setAsking(true);
                  }}
                >
                  {t("staff_emoji.delete.press")}
                </Button>
              </Box>
              <SettingsResult>{deleting === "failed" ? t("staff_emoji.delete.failed") : undefined}</SettingsResult>
            </SettingsSection>
          </>
        }
      >
        <EmojiActions emoji={emoji} />
        <EmojiFacts emoji={emoji} />
        <PickerFields f={f} />
      </SettingsScreen>
      <ModalPresence>
        {asking ? (
          <ConfirmDialog
            key="delete"
            title={t("staff_emoji.delete.title", { shortcode: coded(emoji.shortcode) })}
            cancel={t("staff_emoji.delete.cancel")}
            confirm={t("staff_emoji.delete.press")}
            phrase={emoji.shortcode}
            busy={deleting === "sending"}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={remove}
          >
            {t("staff_emoji.delete.text")}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** The page of the emoji of `id`: a local one's or a remote one's, or that it loads, failed or is none. */
export function EmojiScreen({ id, ...sub }: AdminSubProps & { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminEmoji(id);
  const emoji = read.emoji;
  if (emoji === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff_emoji.entry")}
        back={<BackToEmoji go={sub.go} />}
        notice={
          <ScreenNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_emoji.one.loading"),
              failed: t("staff_emoji.one.failed"),
              missing: t("staff_emoji.missing"),
            }}
            all={{ label: t("staff_emoji.all"), href: adminEmojisPath }}
          />
        }
      />
    );
  if (emoji.local) return <LocalEmoji key={emoji.id} emoji={emoji} {...sub} />;
  return (
    <SettingsScreen sub title={coded(emoji.shortcode)} back={<BackToEmoji go={sub.go} />}>
      <EmojiActions emoji={emoji} />
      <EmojiFacts emoji={emoji} />
      <CopyEmoji emoji={emoji} go={sub.go} />
    </SettingsScreen>
  );
}
