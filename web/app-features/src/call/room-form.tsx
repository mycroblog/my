// SPDX-License-Identifier: AGPL-3.0-only

// The settings of a voice chat room as a form (ADR-0100 §3, §9; FR-923, FR-929): its name, its icon (a Unicode emoji
// or a custom emoji of this server, from the emoji picker of ADR-0091), its description, up to 5 hashtags, who sees it
// and who speaks without asking.
//
// - Opening a room, at /voice/new, for a reader who may open rooms (FR-922): the form, and under it the start post,
//   offered for a room of the local and followers visibilities only, checked at first, with a draft that holds the
//   room's name until the reader writes their own; Open sends it in the reader's gesture, which joins them to the room
//   they host (the call's sound starts there), and the room's page takes the form's place. The server's refusals
//   (not allowed to open rooms, too many open, hosting another room) are said before Open; what it says of a field
//   stands under that field.
// - Changing the settings of an open room, at /voice/{id}/settings, for its host: the same form with the room's
//   settings, Save sending them; another reader is told that only the host changes them.

import { nameOf, roomIcon } from "@my/app-components";
import {
  refusal,
  refusedMember,
  endVoiceRoom,
  updateVoiceRoom,
  useMayOpenVoiceRooms,
  useViewer,
  useVoiceRoom,
  voiceOpenRefusal,
  type AppData,
  type VoiceRoom,
  type VoiceRoomSettings,
  type VoiceRoomSpeakers,
  type VoiceRoomVisibility,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  CheckField,
  ChoiceList,
  CustomEmoji,
  EmojiPicker,
  EmojiPickerPopover,
  EmojiPickerSheet,
  FeedNotice,
  FieldError,
  focusRefusedField,
  FormAlert,
  FormSection,
  GroupHeading,
  ModalPresence,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  TextButton,
  TextField,
  useLocale,
  type PickerEmoji,
} from "@my/design-system";
import { useEffect, useEffectEvent, useId, useState, type ReactNode } from "react";
import { usePickerData } from "../emoji/picker";
import { emojiStrings } from "../emoji/strings";
import { stringsIn } from "../locale";
import { ProblemError } from "@mycroblog/mykit";
import { CallUnavailableError, useVoiceCall } from "./calls";
import { EndRoomDialog } from "./host";
import { useVoiceOff } from "./room";
import { openRoom } from "./room-end";

type T = ReturnType<typeof stringsIn>["t"];

/** The emoji before a room's name as the form holds it: what the settings send, and how it is drawn. */
type PickedIcon = { readonly name: string; readonly shown: ReactNode };

/** What the form holds. */
type Draft = {
  readonly name: string;
  readonly icon: PickedIcon | null;
  readonly description: string;
  /** The hashtags as the reader wrote them, separated by spaces or commas. */
  readonly hashtags: string;
  readonly visibility: VoiceRoomVisibility;
  readonly speakers: VoiceRoomSpeakers;
};

const visibilities: readonly VoiceRoomVisibility[] = ["local", "followers", "following", "mutuals", "invited"];
const speakerChoices: readonly VoiceRoomSpeakers[] = ["everyone", "followers", "following", "mutuals", "manual"];

/** The most hashtags a room has (ADR-0100 §3). */
const hashtagLimit = 5;

/** The hashtags written in `text`, without their `#`, in the order written. */
const hashtagsOf = (text: string): string[] =>
  text
    .split(/[\s,、，]+/u)
    .map((tag) => tag.replace(/^[#＃]+/u, ""))
    .filter((tag) => tag !== "");

/** The draft of a room's settings: the server's defaults for a new one, else the room's own. */
function draftOf(room: VoiceRoom | undefined): Draft {
  if (room === undefined)
    return { name: "", icon: null, description: "", hashtags: "", visibility: "local", speakers: "manual" };
  return {
    name: room.name,
    icon: room.icon === undefined ? null : { name: room.icon.name, shown: roomIcon(room.icon) },
    description: room.description,
    hashtags: room.hashtags.map((tag) => `#${tag}`).join(" "),
    visibility: room.visibility,
    speakers: room.speakers,
  };
}

/** The field of the form that a member of the request names (`hashtags[2]`, `post.text`), by its key. */
type FieldKey = "name" | "icon" | "description" | "hashtags" | "post";

function fieldOf(member: string): FieldKey | undefined {
  const head = member.split(/[.[]/u)[0];
  return head === "name" || head === "icon" || head === "description" || head === "hashtags" || head === "post"
    ? head
    : undefined;
}

/** What the form says is wrong, by field, and what names none (said before the button). */
type Faults = { readonly fields: Partial<Record<FieldKey, string>>; readonly form: string | undefined };

const noFaults: Faults = { fields: {}, form: undefined };

/** How many characters a reader sees in `text` (its graphemes), as the server counts a name. */
const graphemes = (text: string): number =>
  [...new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(text)].length;

/** How many code points `text` holds, as the server counts a description and bounds a name. */
const codePoints = (text: string): number => {
  let n = 0;
  for (let i = 0; i < text.length; i++) {
    const unit = text.charCodeAt(i);
    // A high surrogate and its low one are one code point.
    if (unit < 0xd800 || unit > 0xdbff) n++;
  }
  return n;
};

/**
 * What the form finds wrong before sending, by the server's own counts: a name with nothing in it or longer than 100
 * characters (1,000 code points), a description longer than 500 code points, more hashtags than a room has.
 */
function checked(draft: Draft, t: T): Faults {
  const fields: Partial<Record<FieldKey, string>> = {};
  const name = draft.name.trim();
  if (name === "") fields.name = t("voice.form.name_empty");
  else if (graphemes(name) > 100 || codePoints(name) > 1_000) fields.name = t("voice.form.name_rule");
  if (codePoints(draft.description) > 500) fields.description = t("voice.form.description_rule");
  if (hashtagsOf(draft.hashtags).length > hashtagLimit) fields.hashtags = t("voice.form.hashtags_rule");
  return { fields, form: Object.keys(fields).length === 0 ? undefined : t("voice.form.not_sent") };
}

/** The rule a field's value follows, said under it when the server refused it without words of its own. */
const fieldRules = {
  name: "voice.form.name_rule",
  icon: "voice.form.icon_rule",
  description: "voice.form.description_rule",
  hashtags: "voice.form.hashtags_rule",
  post: "voice.form.post_refused",
} as const;

/** The icon's choice: the emoji picker on a button, and the emoji chosen with the way to remove it. */
function IconField({
  icon,
  sheet,
  error,
  onChange,
  t,
}: {
  icon: PickedIcon | null;
  sheet: boolean;
  error: string | undefined;
  onChange: (icon: PickedIcon | null) => void;
  t: T;
}) {
  const { locale } = useLocale();
  const { preload, onOpen, ahead, ...picker } = usePickerData(locale);
  const [open, setOpen] = useState(false);
  const errorId = useId();
  useEffect(() => ahead(), [ahead]);
  const show = (shown: boolean) => {
    if (shown) {
      preload();
      onOpen();
    }
    setOpen(shown);
  };
  const pick = (emoji: PickerEmoji, text: string) => {
    setOpen(false);
    onChange(
      "image" in emoji
        ? {
            name: emoji.key.slice(1, -1),
            shown: (
              <CustomEmoji shortcode={emoji.key.slice(1, -1)} url={emoji.image.url} staticUrl={emoji.image.staticUrl} />
            ),
          }
        : { name: text, shown: text },
    );
  };
  const label = t(icon === null ? "voice.form.icon_choose" : "voice.form.icon_change");
  const content = <EmojiPicker {...picker} fill={sheet} autoFocus onSelect={pick} />;
  const button = (
    <Button
      size="timeline"
      touch
      {...(sheet
        ? {
            onPress: () => {
              show(true);
            },
          }
        : {})}
    >
      {label}
    </Button>
  );
  return (
    <Stack
      gap="entry-line"
      role="group"
      aria-label={t("voice.form.icon")}
      aria-describedby={error === undefined ? undefined : errorId}
    >
      <GroupHeading>{t("voice.form.icon")}</GroupHeading>
      <Row wrap align="center" gap="entry-caption">
        {icon !== null && (
          <Box as="span" rigid>
            {icon.shown}
          </Box>
        )}
        {sheet ? (
          <>
            {button}
            <EmojiPickerSheet
              open={open}
              onClose={() => {
                setOpen(false);
              }}
              title={label}
              closeLabel={emojiStrings(locale, []).sheet.close}
            >
              {content}
            </EmojiPickerSheet>
          </>
        ) : (
          <EmojiPickerPopover label={label} trigger={button} open={open} onOpenChange={show}>
            {content}
          </EmojiPickerPopover>
        )}
        {icon !== null && (
          <TextButton
            onPress={() => {
              onChange(null);
            }}
          >
            {t("voice.form.icon_remove")}
          </TextButton>
        )}
      </Row>
      {error !== undefined && <FieldError id={errorId}>{error}</FieldError>}
    </Stack>
  );
}

/** A choice of one among several, under its heading. */
function Choice<K extends string>({
  heading,
  keys,
  words,
  value,
  onChange,
}: {
  heading: string;
  keys: readonly K[];
  words: (key: K) => string;
  value: K;
  onChange: (key: K) => void;
}) {
  return (
    <Stack gap="entry-line">
      <GroupHeading>{heading}</GroupHeading>
      <ChoiceList
        label={heading}
        multiple={false}
        items={keys.map((key) => ({ key, label: words(key) }))}
        chosen={[keys.indexOf(value)]}
        onChange={([i]) => {
          const key = keys[i ?? -1];
          if (key !== undefined) onChange(key);
        }}
      />
    </Stack>
  );
}

/** The start post of a new room: whether it is posted, and its text, a draft that holds the room's name. */
type StartPost = {
  readonly posted: boolean;
  readonly text: string | null;
  /** The room's name when the reader began their own text, which keeps the field the one they write in. */
  readonly from: string;
};

/** The form of a room's settings, and for a new room its start post. */
function RoomForm({
  initial,
  opening,
  sheet,
  busy,
  refused,
  onEdit,
  submit,
  onSend,
  t,
}: {
  initial: Draft;
  /** A new room's form, which offers the start post. */
  opening: boolean;
  sheet: boolean;
  busy: boolean;
  /** What the server refused of the last send, until the reader changes something (`onEdit`). */
  refused: Faults;
  onEdit: () => void;
  submit: string;
  onSend: (settings: VoiceRoomSettings) => void;
  t: T;
}) {
  const id = useId();
  const [draft, setDraft] = useState(initial);
  const [post, setPost] = useState<StartPost>({ posted: true, text: null, from: "" });
  const [faults, setFaults] = useState<Faults>(noFaults);
  // Each press of the button, after which what is wrong takes the focus.
  const [tries, setTries] = useState(0);
  const shown: Faults = faults.form !== undefined ? faults : refused;
  const change = (next: Partial<Draft>) => {
    setDraft({ ...draft, ...next });
    onEdit();
    // The form's own check goes again with each change, so a mark goes as its value is put right.
    if (faults.form !== undefined) setFaults(checked({ ...draft, ...next }, t));
  };
  const offersPost = opening && (draft.visibility === "local" || draft.visibility === "followers");
  const postText = post.text ?? t("voice.form.post_draft", { name: draft.name.trim() });
  const fieldId = (key: FieldKey) => `${id}-${key}`;
  // A refusal takes the focus to the first field it marks, its error in view.
  const order: readonly FieldKey[] = ["name", "icon", "description", "hashtags", "post"];
  const firstFault = order.find((key) => shown.fields[key] !== undefined);
  // Once a press found something wrong, or the server's answer to it did, never as the reader types.
  const focusFault = useEffectEvent(() => {
    if (firstFault !== undefined) focusRefusedField(`${id}-${firstFault}`);
  });
  useEffect(() => {
    focusFault();
  }, [tries, refused]);
  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        if (busy) return;
        const found = checked(draft, t);
        setFaults(found);
        setTries(tries + 1);
        if (found.form !== undefined) return;
        const icon = draft.icon?.name;
        onSend({
          name: draft.name.trim(),
          ...(icon === undefined ? (opening ? {} : { icon: "" }) : { icon }),
          description: draft.description,
          hashtags: hashtagsOf(draft.hashtags),
          visibility: draft.visibility,
          speakers: draft.speakers,
          ...(offersPost && post.posted ? { post: { text: postText } } : {}),
        });
      }}
    >
      <ScreenStack>
        <TextField
          id={fieldId("name")}
          name="name"
          label={t("voice.form.name")}
          defaultValue={initial.name}
          required
          error={shown.fields.name}
          onChange={(name) => {
            change({ name });
          }}
        />
        <IconField
          icon={draft.icon}
          sheet={sheet}
          error={shown.fields.icon}
          onChange={(icon) => {
            change({ icon });
          }}
          t={t}
        />
        <TextField
          id={fieldId("description")}
          name="description"
          label={t("voice.form.description")}
          defaultValue={initial.description}
          multiline
          rows={3}
          error={shown.fields.description}
          onChange={(description) => {
            change({ description });
          }}
        />
        <TextField
          id={fieldId("hashtags")}
          name="hashtags"
          label={t("voice.form.hashtags")}
          hint={shown.fields.hashtags === undefined ? t("voice.form.hashtags_hint") : undefined}
          defaultValue={initial.hashtags}
          error={shown.fields.hashtags}
          onChange={(hashtags) => {
            change({ hashtags });
          }}
        />
        <Choice
          heading={t("voice.form.visibility")}
          keys={visibilities}
          words={(visibility) => t("voice.form.visibility_choice", { visibility })}
          value={draft.visibility}
          onChange={(visibility) => {
            change({ visibility });
          }}
        />
        <Choice
          heading={t("voice.form.speakers")}
          keys={speakerChoices}
          words={(speakers) => t("voice.form.speakers_choice", { speakers })}
          value={draft.speakers}
          onChange={(speakers) => {
            change({ speakers });
          }}
        />
        {offersPost && (
          <Stack gap="entry-line">
            <GroupHeading>{t("voice.form.post")}</GroupHeading>
            <CheckField
              id={`${id}-posted`}
              name="posted"
              value="yes"
              touch
              label={t("voice.form.post_check", { visibility: draft.visibility })}
              checked={post.posted}
              onChange={(posted) => {
                setPost({ ...post, posted });
              }}
            />
            {post.posted && (
              <>
                <TextField
                  // Drawn anew from the draft until the reader writes their own text, then the field they write in.
                  key={`draft-${post.text === null ? draft.name : post.from}`}
                  id={fieldId("post")}
                  name="post"
                  label={t("voice.form.post_text")}
                  defaultValue={postText}
                  multiline
                  rows={3}
                  hint={t("voice.form.post_link")}
                  error={shown.fields.post}
                  onChange={(text) => {
                    setPost({ ...post, text, from: post.text === null ? draft.name : post.from });
                    onEdit();
                  }}
                />
              </>
            )}
          </Stack>
        )}
        <Stack gap="entry-line">
          {shown.form !== undefined && <FormAlert regular>{shown.form}</FormAlert>}
          <Box>
            <Button kind="primary" type="submit" busy={busy}>
              {submit}
            </Button>
          </Box>
        </Stack>
      </ScreenStack>
    </form>
  );
}

/**
 * What the server's refusal of a send says, by field and before the button. With a start post (`posted`), what the
 * server says of the post (its members `text` and `language`, a post the reader may not post now) stands under the
 * post's text, with the way to open the room without it.
 */
function refusalFaults(error: unknown, t: T, posted: boolean): Faults {
  const member = refusedMember(error);
  if (posted && error instanceof ProblemError && voiceOpenRefusal(error) === null) {
    const ofPost = member !== undefined && (member.member === "text" || member.member === "language");
    if (ofPost || error.status === 403) {
      const said = (ofPost ? member.said : refusal(error)) || t("voice.form.post_refused");
      return { fields: { post: `${said} ${t("voice.form.post_without")}` }, form: t("voice.form.not_sent") };
    }
  }
  const key = member === undefined ? undefined : fieldOf(member.member);
  if (member !== undefined && key !== undefined)
    return { fields: { [key]: member.said || t(fieldRules[key]) }, form: t("voice.form.not_sent") };
  const kind = voiceOpenRefusal(error);
  if (kind !== null) return { fields: {}, form: t("voice.form.open_refused", { kind: kind.replaceAll("-", "_") }) };
  if (error instanceof CallUnavailableError) return { fields: {}, form: t("voice.join_unavailable") };
  return { fields: {}, form: t("voice.form.failed") };
}

/**
 * The form that opens a room, at /voice/new, for a reader who may open rooms; another is told they may not. Open
 * joins the reader to the room they host, from their gesture, and `onOpened` goes to its page; `onBack` to the rooms.
 * `sheet`: the emoji picker opens as a sheet (a phone's viewport), else as a popover.
 */
export function VoiceRoomOpen({
  data,
  sheet,
  onOpened,
  onBack,
}: {
  /** The session's data, which the room is opened on. */
  data: AppData;
  sheet: boolean;
  onOpened: (id: string) => void;
  onBack: () => void;
}) {
  const { t } = stringsIn(useLocale().locale);
  const may = useMayOpenVoiceRooms();
  const off = useVoiceOff();
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<Faults>(noFaults);
  const back = <BackButton onPress={onBack}>{t("voice.back")}</BackButton>;
  return (
    <ScreenStack>
      <ScreenTitle back={back} glyph="room">
        {t("voice.open")}
      </ScreenTitle>
      {off ? (
        <FeedNotice lead>{t("voice.off")}</FeedNotice>
      ) : may ? (
        <RoomForm
          initial={draftOf(undefined)}
          opening
          sheet={sheet}
          busy={busy}
          refused={refused}
          onEdit={() => {
            setRefused(noFaults);
          }}
          submit={t("voice.open.submit")}
          onSend={(settings) => {
            setBusy(true);
            // Inside the press: the call's sound starts there, or the browser holds it back until another.
            openRoom(data, settings).then(
              (call) => {
                onOpened(call.roomId);
              },
              (error: unknown) => {
                setBusy(false);
                setRefused(refusalFaults(error, t, settings.post !== undefined));
              },
            );
          }}
          t={t}
        />
      ) : (
        <FeedNotice lead>{t("voice.open.not_allowed")}</FeedNotice>
      )}
    </ScreenStack>
  );
}

/**
 * The settings of the open room of `id`, at /voice/{id}/settings, which its host changes; Save sends them and goes
 * back to the room (`onBack`, also the way back). Another reader is told that only the host changes them.
 */
export function VoiceRoomSettingsScreen({
  data,
  id,
  sheet,
  onBack,
}: {
  /** The session's data, which the change is sent on. */
  data: AppData;
  id: string;
  sheet: boolean;
  onBack: () => void;
}) {
  const { t } = stringsIn(useLocale().locale);
  const viewer = useViewer();
  const { room, error, refetch } = useVoiceRoom(id);
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<Faults>(noFaults);
  const [ending, setEnding] = useState(false);
  const [endFailed, setEndFailed] = useState(false);
  const call = useVoiceCall();
  const back = <BackButton onPress={onBack}>{t("voice.room_back")}</BackButton>;
  const head = (
    <ScreenTitle back={back} glyph="room">
      {t("voice.settings.title")}
    </ScreenTitle>
  );
  // A room that ended has no settings: the address says so, with nothing to try again.
  if (room === undefined && error instanceof ProblemError && error.status === 404)
    return (
      <ScreenStack>
        {head}
        <FeedNotice lead>{t("voice.settings.ended")}</FeedNotice>
      </ScreenStack>
    );
  if (room === undefined)
    return (
      <ScreenStack busy={error === null}>
        {head}
        {error === null ? (
          <FeedNotice>{t("voice.loading")}</FeedNotice>
        ) : (
          <FeedNotice alert action={<TextButton onPress={refetch}>{t("voice.retry")}</TextButton>}>
            {t("voice.failed")}
          </FeedNotice>
        )}
      </ScreenStack>
    );
  if (viewer === undefined || room.host.id !== viewer.id)
    return (
      <ScreenStack>
        {head}
        <FeedNotice lead>{t("voice.settings.host_only", { host: nameOf(room.host) })}</FeedNotice>
      </ScreenStack>
    );
  return (
    <ScreenStack>
      {head}
      <RoomForm
        key={room.id}
        initial={draftOf(room)}
        opening={false}
        sheet={sheet}
        busy={busy}
        refused={refused}
        onEdit={() => {
          setRefused(noFaults);
        }}
        submit={t("voice.settings.submit")}
        onSend={({ post: _post, ...settings }) => {
          setBusy(true);
          updateVoiceRoom(data, id, settings).then(onBack, (e: unknown) => {
            setBusy(false);
            setRefused(refusalFaults(e, t, false));
          });
        }}
        t={t}
      />

      {/* What ends the room stands last and apart, under a heading of its own, asked first. */}
      <FormSection id={`${id}-end`} heading={t("voice.settings.end_heading")} apart>
        <Box as="p">{t("voice.host.end_text")}</Box>
        {endFailed && <FormAlert regular>{t("voice.act_failed", { action: "end" })}</FormAlert>}
        <Box>
          <Button
            onPress={() => {
              setEndFailed(false);
              setEnding(true);
            }}
          >
            {t("voice.host.end")}
          </Button>
        </Box>
      </FormSection>
      <ModalPresence>
        {ending && (
          <EndRoomDialog
            key="end"
            end={() => (call !== null && call.roomId === id && !call.ended ? call.endRoom() : endVoiceRoom(data, id))}
            onEnding={() => undefined}
            onClose={() => {
              setEnding(false);
            }}
            onEnded={onBack}
            onFailed={() => {
              setEndFailed(true);
            }}
            t={t}
          />
        )}
      </ModalPresence>
    </ScreenStack>
  );
}
