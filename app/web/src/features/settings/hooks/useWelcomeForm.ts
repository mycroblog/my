// SPDX-License-Identifier: AGPL-3.0-only
// The welcome step's form (web-client/screens/welcome.md): the basics of the new user's profile, the avatar and the
// display name, saved together before the user goes on to the home timeline. The hook returns no JSX.
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { cacheSavedProfile, credentialsQueryOptions, type ProfileChange, updateProfileMutation } from "../api/me";
import { useImageControl } from "./useProfileForm";

export type WelcomeFormOptions = {
  /** Called once the profile is saved, or at once when nothing changed: goes on to the home timeline. */
  onDone: () => void;
};

export function useWelcomeForm({ onDone }: WelcomeFormOptions) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const { data: account } = useSuspenseQuery(credentialsQueryOptions(api));
  const [displayName, setDisplayName] = useState(account.display_name);
  const avatar = useImageControl(account.avatar, "avatar");
  const save = useMutation({
    ...updateProfileMutation(api),
    onSuccess: (saved) => {
      // The home timeline's checklist reads the saved profile from its first render (web-client/README.md S1).
      cacheSavedProfile(api, queryClient, saved);
      avatar.saved();
      onDone();
    },
  });
  const change: ProfileChange = {
    ...(displayName === account.display_name ? {} : { display_name: displayName }),
    ...avatar.change("avatar_media_id"),
  };
  return {
    account,
    displayName: { value: displayName, onChange: setDisplayName },
    avatar: avatar.control,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (save.isPending) return;
      if (Object.keys(change).length === 0) onDone();
      else save.mutate(change);
    },
    isSaving: save.isPending,
    /** Why the last save failed (localized), or undefined. */
    error: save.isError ? t("client:settings.profile.save_failed", { reason: errorMessage(save.error) }) : undefined,
  };
}
