// SPDX-License-Identifier: AGPL-3.0-only

// The name and the avatar an account goes by, as its owner sets them (FR-006): the two fields the form of the profile
// (settings/profile-edit.tsx) and the welcome step of a new account share, and where that form is. `useProfileIdentity`
// holds what the reader wrote and picked against the account as the server has it; `ProfileNameField` and
// `ProfileAvatarField` show it, the name with its count near the limit, which describes the field once it shows
// (`limitCount`, the bio's too), the avatar in its circle as it will show.
// Nothing is saved here: whoever holds the fields saves `change` with app-data's `updateProfile`, places what the
// server refused beside the field it names (`error`), and says `saved` once the server took it. A picture of the
// profile that the reader picks, replaces or removes is `usePickedPicture`, the header's too.

import { profileLimits, type Account, type ProfileChange } from "@my/app-data";
import { CharCount, PictureField, TextField, useLocale } from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactElement } from "react";
import { charsOf } from "./chars";
import { stringsIn } from "./locale";

/** The address of the form where the reader edits their profile, under the settings' entry Profile. */
export const profileEditPath = "/settings/profile/edit";

/** A picture of the profile as the reader left it in a form. */
export type PickedPicture = {
  /** What to save: the file picked, null to remove the picture, undefined to leave the account's as it is. */
  readonly file: File | null | undefined;
  /** The picture as it will show: the file picked, else the account's; undefined for none. */
  readonly src: string | undefined;
  /** Takes a file the reader picked, in place of what showed. */
  readonly pick: (file: File) => void;
  /** Removes the picture that shows. */
  readonly remove: () => void;
  /** Back to the account's picture: what was picked or removed was saved, and is the account's now. */
  readonly reset: () => void;
};

/** A picture as picked: the file, and the address it shows from. */
type Picked = { readonly file: File; readonly url: string };

/**
 * A picture of the profile in a form: `current` is the account's (its address, undefined for none), which shows until
 * the reader picks a file, which then shows from the device, or removes the picture. `onChange` hears each pick and
 * removal. `start` is a picture a story's specimen opens the form with, as if picked, at an address of the story's own.
 */
export function usePickedPicture(current: string | undefined, onChange?: () => void, start?: Picked): PickedPicture {
  const [picked, setPicked] = useState<Picked | null | undefined>(start);
  /** The address made for the file that shows, given back to the browser once another takes its place. */
  const made = useRef<string | undefined>(undefined);
  const put = (next: Picked | null | undefined) => {
    if (made.current !== undefined) URL.revokeObjectURL(made.current);
    made.current = next?.url;
    setPicked(next);
  };
  useEffect(
    () => () => {
      if (made.current !== undefined) URL.revokeObjectURL(made.current);
    },
    [],
  );
  return {
    file: picked === undefined || picked === null ? picked : picked.file,
    src: picked === undefined ? current : (picked?.url ?? undefined),
    pick: (file) => {
      put({ file, url: URL.createObjectURL(file) });
      onChange?.();
    },
    remove: () => {
      // Removing a picture just picked from an account that has none leaves nothing to save.
      put(current === undefined ? undefined : null);
      onChange?.();
    },
    reset: () => {
      put(undefined);
    },
  };
}

/** The name and the avatar of the reader's account, as a form holds them. */
export type ProfileIdentity = {
  /** The display name as written. */
  readonly displayName: string;
  readonly setDisplayName: (name: string) => void;
  /** How many characters the display name has left before its limit; negative past it. */
  readonly nameLeft: number;
  /** The name the avatar's initial stands for while there is no picture: the display name, else the username. */
  readonly name: string;
  readonly avatar: PickedPicture;
  /** Whether either differs from what the account has. */
  readonly changed: boolean;
  /** What to save, for app-data's `updateProfile`: the members that differ from the account's, none when nothing does. */
  readonly change: Pick<ProfileChange, "display_name" | "avatar">;
  /** What was written and picked is the account's now: nothing is changed any more. */
  readonly saved: () => void;
};

/** What a story's specimen of a form opens the identity with, as if the reader had written and picked it. */
export type IdentityStart = {
  readonly displayName?: string | undefined;
  /** The picture picked, and the address it shows from, which stays the story's own. */
  readonly avatar?: Picked | undefined;
};

/**
 * The name and the avatar of `account`, the reader's own, for a form to change; `onChange` hears each change of
 * either (a form forgets there what its last save said). `start` is what a story's specimen opens the form with.
 */
export function useProfileIdentity(
  account: Pick<Account, "display_name" | "username" | "avatar">,
  onChange?: () => void,
  start?: IdentityStart,
): ProfileIdentity {
  const [base, setBase] = useState(account.display_name);
  const [displayName, setName] = useState(start?.displayName ?? account.display_name);
  const avatar = usePickedPicture(account.avatar?.medium.url ?? account.avatar?.small.url, onChange, start?.avatar);
  const renamed = displayName !== base;
  return {
    displayName,
    setDisplayName: (name) => {
      setName(name);
      onChange?.();
    },
    nameLeft: profileLimits.displayName - charsOf(displayName),
    name: displayName || account.username,
    avatar,
    changed: renamed || avatar.file !== undefined,
    change: {
      ...(renamed ? { display_name: displayName } : {}),
      ...(avatar.file === undefined ? {} : { avatar: avatar.file }),
    },
    saved: () => {
      setBase(displayName);
      avatar.reset();
    },
  };
}

/**
 * The count of a field near its limit, negative past it; none until it is near. `id` is the id of its words, for the
 * field to be described by them (`limitCount`).
 */
export function LimitCount({ id, left, near }: { id?: string | undefined; left: number; near: number }) {
  const { t } = stringsIn(useLocale().locale);
  if (left > near) return undefined;
  return (
    <CharCount
      id={id}
      left={left}
      near={near}
      label={
        left < 0
          ? t("settings.profile.count.over", { count: -left })
          : t("settings.profile.count.left", { count: left })
      }
    />
  );
}

/**
 * What the `TextField` of `id` takes of its count against a limit (`left` characters before it, counted from `near`
 * it): the count at the end of its label and, once it shows, the count's words among what describes the field, so that
 * a screen reader hears how much room is left with the field, and not only who sees it.
 */
export function limitCount(
  id: string,
  left: number,
  near: number,
): { end: ReactElement; describedBy: string | undefined } {
  const countId = `${id}-count`;
  return {
    end: <LimitCount id={countId} left={left} near={near} />,
    describedBy: left > near ? undefined : countId,
  };
}

/** What the fields of the identity take from the form that holds them. */
type FieldProps = {
  identity: ProfileIdentity;
  /** The field's id, for the form to put the focus on it; one of its own without it. */
  id?: string | undefined;
  /**
   * What the server refused of its value: under the avatar, these words; under the name, the rule a display name
   * follows in their place.
   */
  error?: string | undefined;
  /** The form is being sent: the field cannot be changed now. */
  busy?: boolean | undefined;
};

/**
 * The display name: a line of text, its count near the limit, and what is wrong with it under it. Over the limit, it
 * says the limit. A name the server refused (`error`) is one it finds too long or not on one line of plain
 * characters, of which it says only that the value is not valid: the field says the rule a display name follows.
 */
export function ProfileNameField({ identity, id, error, busy = false }: FieldProps) {
  const { t } = stringsIn(useLocale().locale);
  const own = useId();
  const fieldId = id ?? own;
  const max = profileLimits.displayName;
  return (
    <TextField
      id={fieldId}
      name="display_name"
      label={t("settings.profile.name")}
      defaultValue={identity.displayName}
      onChange={identity.setDisplayName}
      autoComplete="nickname"
      readOnly={busy}
      {...limitCount(fieldId, identity.nameLeft, 10)}
      error={
        identity.nameLeft < 0
          ? t("settings.profile.name.over", { max })
          : error === undefined
            ? undefined
            : t("settings.profile.name.rule", { max })
      }
    />
  );
}

/** The avatar, in its circle as it will show: choose a picture, replace it, remove it. */
export function ProfileAvatarField({ identity, id, error, busy = false }: FieldProps) {
  const { t } = stringsIn(useLocale().locale);
  const own = useId();
  const { avatar } = identity;
  return (
    <PictureField
      id={id ?? own}
      label={t("settings.profile.avatar")}
      shape="round"
      name={identity.name}
      src={avatar.src}
      error={error}
      choose={t(avatar.src === undefined ? "settings.profile.picture.choose" : "settings.profile.picture.replace")}
      onFile={avatar.pick}
      remove={
        avatar.src === undefined ? undefined : { label: t("settings.profile.picture.remove"), onPress: avatar.remove }
      }
      isDisabled={busy}
    />
  );
}
