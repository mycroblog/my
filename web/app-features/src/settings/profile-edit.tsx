// SPDX-License-Identifier: AGPL-3.0-only

// Edit profile, the screen under the settings' entry Profile at /settings/profile/edit (FR-006): the form of what
// others see of the reader's account, saved as a whole by the submit bar. Its pictures, in the profile's order: the
// header image, 3:1, and the avatar, in its circle, each picked from the device, shown at once as it will show,
// replaced or removed, and uploaded as the form is saved. The display name and the bio, each with its count near the
// limit and what is wrong under it past it. The fields, four at most, in the order they were entered: a label and
// its content each, added from the section's head, or from what stands in the list's place while there is none.
//
// Save sends what changed and nothing else (app-data's `updateProfile`), and the profile shows it at once. What the
// server refused stands beside the input it names, the submit bar saying the rest; a picture it could not take is
// said there too. A text over its limit is not sent: the focus goes to it. Leaving with changes not saved is the
// app's to notice (`onUnsaved` tells it there are some): it holds the reader back and gives `leaving`, and the
// screen asks whether to leave without saving.

import { fieldError, placeRefusal, type FieldMembers, type Refused } from "../field-errors";
import {
  MediaFailedError,
  profileLimits,
  useCredentials,
  useProfileRead,
  useUpdateProfile,
  viewerQuery,
  type AppData,
  type CredentialAccount,
  type ProfileChange,
  type ProfileField,
} from "@my/app-data";
import {
  BackButton,
  Button,
  ConfirmDialog,
  FormAlert,
  ModalPresence,
  PairField,
  PairList,
  PictureField,
  SaveState,
  SettingsEmpty,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { stringsIn } from "../locale";
import {
  LimitCount,
  ProfileAvatarField,
  ProfileNameField,
  usePickedPicture,
  useProfileIdentity,
} from "../profile-identity";
import { SettingsReading } from "./controls";

/** What the route's loader waits for: the reader's own account, from the cache when it has it. */
export async function loadProfileEdit(data: AppData): Promise<void> {
  await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The title of the screen. */
export function useProfileEditTitle(): string {
  return stringsIn(useLocale().locale).t("settings.profile.edit");
}

export type ProfileEditProps = {
  /** Goes back to the entry, Profile. */
  onBack: () => void;
  /** Hears whether the form holds changes that are not saved, as that comes and goes; none once it is gone. */
  onUnsaved?: ((unsaved: boolean) => void) | undefined;
  /**
   * The reader is leaving with changes not saved, and is held back: the screen asks whether to leave (`onLeave`, the
   * changes lost) or stay (`onStay`).
   */
  leaving?: { onStay: () => void; onLeave: () => void } | undefined;
};

/** The inputs of the form, which a refusal of the server is placed on. */
type Input = "name" | "note" | "avatar" | "header" | "fields";

/** Which input answers to which member of the request (`PATCH /accounts/update_credentials`). */
const members: FieldMembers<Input> = {
  display_name: "name",
  note: "note",
  avatar_media_id: "avatar",
  header_media_id: "header",
  fields: "fields",
  "fields[].name": "fields",
  "fields[].value": "fields",
};

/** A field of the profile as the form holds it: the pair, and a key of its own, which no other pair ever had. */
type Pair = ProfileField & { readonly key: number };

/** The characters of a text as the server counts them (code points). */
const charsOf = (text: string) => Array.from(text).length;

/** The fields to save: the pairs with something written, in their order. */
const written = (pairs: readonly Pair[]): ProfileField[] =>
  pairs.filter((p) => p.name.trim() !== "" || p.value.trim() !== "").map(({ name, value }) => ({ name, value }));

const sameFields = (a: readonly ProfileField[], b: readonly ProfileField[]) =>
  a.length === b.length &&
  a.every((f, i) => {
    const other = b[i];
    return other !== undefined && f.name === other.name && f.value === other.value;
  });

/** Where the form's last save is. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | { readonly kind: "refused"; readonly refused: Refused<Input>; readonly picture: boolean };

function EditForm({
  account,
  onBack,
  onUnsaved,
  leaving,
}: ProfileEditProps & {
  account: CredentialAccount;
}) {
  const { locale } = useLocale();
  const { t, lang } = stringsIn(locale);
  const id = useId();
  const update = useUpdateProfile();
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  /** A change of the form: what the last save said was about the form as it was. */
  const changed = () => {
    setSent((was) => (was.kind === "saved" || was.kind === "refused" ? { kind: "idle" } : was));
  };
  const identity = useProfileIdentity(account, changed);
  const header = usePickedPicture(account.header?.medium.url, changed);
  // What the account has of the bio and the fields, as the reader typed them: what the form is compared with.
  const [base, setBase] = useState<{ note: string; fields: readonly ProfileField[] }>(account.source);
  const [note, setNote] = useState(base.note);
  const [pairs, setPairs] = useState<readonly Pair[]>(() => base.fields.map((f, key) => ({ ...f, key })));
  // The keys of the pairs the reader adds follow those the form opened with.
  const [opened] = useState(pairs.length);
  const nextKey = useRef(opened);
  // What takes the focus once the list of fields is drawn again: the field after one removed, or what adds one.
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    document.getElementById(focusNext.current)?.focus();
    focusNext.current = undefined;
  });

  const busy = sent.kind === "sending";
  const refused = sent.kind === "refused" ? sent.refused : undefined;
  const fields = written(pairs);
  const noteLeft = profileLimits.note - charsOf(note);
  const noteChanged = note !== base.note;
  const fieldsChanged = !sameFields(fields, base.fields);
  const unsaved = identity.changed || header.file !== undefined || noteChanged || fieldsChanged;
  useEffect(() => {
    onUnsaved?.(unsaved);
  }, [onUnsaved, unsaved]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );

  const nameId = `${id}-name`;
  const noteId = `${id}-note`;
  const addId = `${id}-add`;
  const pairId = (key: number) => `${id}-pair-${String(key)}`;

  const save = () => {
    if (busy) return;
    // A text over its limit is not sent: its field says so already, and takes the focus.
    const over = identity.nameLeft < 0 ? nameId : noteLeft < 0 ? noteId : undefined;
    if (over !== undefined) {
      document.getElementById(over)?.focus();
      return;
    }
    if (!unsaved) {
      setSent({ kind: "saved" });
      return;
    }
    const change: ProfileChange = {
      ...identity.change,
      ...(header.file === undefined ? {} : { header: header.file }),
      ...(noteChanged ? { note } : {}),
      ...(fieldsChanged ? { fields } : {}),
    };
    setSent({ kind: "sending" });
    update(change).then(
      () => {
        identity.saved();
        header.reset();
        setBase({ note, fields });
        setSent({ kind: "saved" });
      },
      (error: unknown) => {
        setSent({ kind: "refused", refused: placeRefusal(error, members), picture: error instanceof MediaFailedError });
      },
    );
  };

  const addField = () => {
    const key = nextKey.current;
    nextKey.current += 1;
    setPairs([...pairs, { key, name: "", value: "" }]);
    changed();
  };
  const add = (
    <Button id={addId} size="timeline" isDisabled={busy} onPress={addField}>
      {t("settings.profile.fields.add")}
    </Button>
  );
  const fieldsError = fieldError(refused, "fields");

  return (
    <>
      <SettingsScreen
        sub
        title={t("settings.profile.edit")}
        back={<BackButton onPress={onBack}>{t("settings.entry.profile")}</BackButton>}
        foot={
          <SubmitBar
            result={
              sent.kind === "saved" ? (
                <SaveState state="saved" />
              ) : sent.kind === "refused" ? (
                <FormAlert>
                  {sent.picture
                    ? t("settings.profile.save.picture_failed")
                    : (sent.refused.reason ?? t("settings.profile.save.failed"))}
                </FormAlert>
              ) : undefined
            }
          >
            <Button kind="primary" busy={busy} onPress={save}>
              {t("settings.profile.save")}
            </Button>
          </SubmitBar>
        }
      >
        <SettingsSection heading={t("settings.profile.edit.section.pictures")}>
          <PictureField
            id={`${id}-header`}
            label={t("settings.profile.header")}
            shape="banner"
            src={header.src}
            error={fieldError(refused, "header")}
            choose={t(
              header.src === undefined ? "settings.profile.picture.choose" : "settings.profile.picture.replace",
            )}
            onFile={header.pick}
            remove={
              header.src === undefined
                ? undefined
                : { label: t("settings.profile.picture.remove"), onPress: header.remove }
            }
            isDisabled={busy}
          />
          <ProfileAvatarField
            id={`${id}-avatar`}
            identity={identity}
            error={fieldError(refused, "avatar")}
            busy={busy}
          />
        </SettingsSection>
        <SettingsSection heading={t("settings.profile.edit.section.text")}>
          <ProfileNameField id={nameId} identity={identity} error={fieldError(refused, "name")} busy={busy} />
          <TextField
            id={noteId}
            name="note"
            label={t("settings.profile.note")}
            multiline
            defaultValue={note}
            onChange={(text) => {
              setNote(text);
              changed();
            }}
            readOnly={busy}
            end={<LimitCount left={noteLeft} near={50} />}
            error={
              noteLeft < 0 ? t("settings.profile.note.over", { max: profileLimits.note }) : fieldError(refused, "note")
            }
          />
        </SettingsSection>
        <SettingsSection
          heading={t("settings.profile.fields")}
          count={pairs.length === 0 ? undefined : new Intl.NumberFormat(lang).format(pairs.length)}
          about={t("settings.profile.fields.about", { max: profileLimits.fields })}
          action={pairs.length > 0 && pairs.length < profileLimits.fields ? add : undefined}
        >
          {pairs.length === 0 ? (
            <SettingsEmpty action={add}>{t("settings.profile.fields.empty")}</SettingsEmpty>
          ) : (
            <PairList id={`${id}-fields`} label={t("settings.profile.fields")} error={fieldsError}>
              {pairs.map((pair, i) => (
                <PairField
                  key={pair.key}
                  id={pairId(pair.key)}
                  legend={t("settings.profile.fields.legend", { n: i + 1 })}
                  nameLabel={t("settings.profile.fields.name")}
                  name={pair.name}
                  onName={(name) => {
                    setPairs(pairs.map((p) => (p.key === pair.key ? { ...p, name } : p)));
                    changed();
                  }}
                  valueLabel={t("settings.profile.fields.value")}
                  value={pair.value}
                  onValue={(value) => {
                    setPairs(pairs.map((p) => (p.key === pair.key ? { ...p, value } : p)));
                    changed();
                  }}
                  maxLength={profileLimits.fieldText}
                  remove={{
                    label: t("settings.profile.fields.remove", { n: i + 1 }),
                    onPress: () => {
                      // The button leaves with its pair: the focus goes to the pair that takes its place, the one
                      // before it, or what adds a first one.
                      const after = pairs[i + 1] ?? pairs[i - 1];
                      focusNext.current = after === undefined ? addId : pairId(after.key);
                      setPairs(pairs.filter((p) => p.key !== pair.key));
                      changed();
                    },
                  }}
                  // A pair the reader just added is theirs to write: the button that added it may be gone.
                  autoFocus={pair.key >= opened}
                  invalid={fieldsError !== undefined}
                  isDisabled={busy}
                />
              ))}
            </PairList>
          )}
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {leaving !== undefined && (
          <ConfirmDialog
            title={t("settings.profile.leave.title")}
            cancel={t("settings.profile.leave.cancel")}
            confirm={t("settings.profile.leave.confirm")}
            onCancel={leaving.onStay}
            onConfirm={leaving.onLeave}
          >
            {t("settings.profile.leave.text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}

/** The screen: the form once the reader's account is read, and until then that it loads or failed. */
export function ProfileEdit({ onBack, onUnsaved, leaving }: ProfileEditProps) {
  const { t } = stringsIn(useLocale().locale);
  const account = useCredentials();
  const { status, retry } = useProfileRead();
  if (account === undefined)
    return (
      <SettingsReading
        sub
        title={t("settings.profile.edit")}
        back={<BackButton onPress={onBack}>{t("settings.entry.profile")}</BackButton>}
        failed={status === "failed"}
        onRetry={retry}
      />
    );
  return <EditForm account={account} onBack={onBack} onUnsaved={onUnsaved} leaving={leaving} />;
}
