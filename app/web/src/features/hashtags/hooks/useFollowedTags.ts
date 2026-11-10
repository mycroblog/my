// SPDX-License-Identifier: AGPL-3.0-only
// The hashtags the user follows, in the settings (web-client/screens/settings-followed-tags.md): the list, following one
// with a form, and unfollowing one from its row. Each change also changes the home timeline (api/tags.ts).
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  type FollowedHashtag,
  followedTagsQueryOptions,
  hashtagFailure,
  hashtagInput,
  isHashtagName,
  maxFollowedTags,
  removeFollowedTag,
  storeTagFollow,
  tagFollowMutation,
} from "../api/tags";
import { hashtagInText } from "./text";

/** The followed hashtags, the latest followed first; the route has loaded them. */
export function useFollowedTagList(): { items: FollowedHashtag[]; isFull: boolean } {
  const api = useApi();
  const { data } = useSuspenseQuery(followedTagsQueryOptions(api));
  return { items: data.items, isFull: data.items.length >= maxFollowedTags };
}

/** Unfollowing a hashtag from its row. */
export type UnfollowTag = {
  /** Unfollows `name`; `onDone` is called once it is off the list. */
  run: (name: string, onDone: () => void) => void;
  /** The hashtag being unfollowed. */
  pendingName: string | undefined;
  /** Why the last unfollow failed, localized, shown above the list; undefined when it did not. */
  error: string | undefined;
};

/**
 * Unfollowing a hashtag: the row leaves the list once the answer is in and the home timeline on screen, if any, is
 * fetched again (the row stays pending until then); then the caller moves focus to the next row and the result is
 * announced. A failure keeps the row and its focus, and says why
 * above the list (web-client/README.md E2).
 */
export function useUnfollowTag(): UnfollowTag {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const unfollow = useMutation({
    ...tagFollowMutation(api),
    onSuccess: (result) => storeTagFollow(queryClient, api, result, { keepRow: true }),
  });
  return {
    pendingName: unfollow.isPending ? unfollow.variables.tag : undefined,
    error: unfollow.isError
      ? t("client:tag.followed_list.unfollow_failed", {
          tag: hashtagInText(unfollow.variables.tag),
          reason: errorMessage(unfollow.error),
        })
      : undefined,
    run: (name, onDone) => {
      unfollow.mutate(
        { tag: name, follow: false },
        {
          onSuccess: () => {
            removeFollowedTag(queryClient, api, name);
            onDone();
            notice.inPlace(t("client:tag.unfollowed", { tag: hashtagInText(name) }));
          },
        },
      );
    },
  };
}

/** What the form that follows a hashtag needs. */
export type FollowTagForm = {
  tag: string;
  setTag: (tag: string) => void;
  /** Follows the hashtag once the browser has checked the field; `onAdded` is called once it is on the list. */
  submit: (event: SyntheticEvent<HTMLFormElement>, onAdded?: () => void) => void;
  /** Why the text is no hashtag, for the field's own check; null when it is one or empty. */
  validate: (tag: string) => string | null;
  isPending: boolean;
  /** Why following failed, localized, for the form's action bar; the hashtag typed stays. */
  error: string | undefined;
  /** What the last follow did, shown above the list; undefined before the first one. */
  status: string | undefined;
};

/**
 * The form that follows a hashtag, with or without its `#`. A success empties the field and says which hashtag is
 * followed, as the server names it, once the list on screen is fetched again.
 */
export function useFollowTagForm(): FollowTagForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [tag, setTag] = useState("");
  const [status, setStatus] = useState<string | undefined>(undefined);
  const follow = useMutation({
    ...tagFollowMutation(api),
    onSuccess: async (result) => {
      await storeTagFollow(queryClient, api, result);
      setTag("");
      setStatus(t("client:tag.followed", { tag: hashtagInText(result.hashtag.name) }));
    },
  });
  let error: string | undefined;
  if (follow.isError) {
    error =
      hashtagFailure(follow.error) === "too-many"
        ? t("client:tag.follow.too_many", { max: String(maxFollowedTags) })
        : t("client:tag.followed_list.follow_failed", { reason: errorMessage(follow.error) });
  }
  return {
    tag,
    setTag: (value) => {
      setTag(value);
      if (follow.isError) follow.reset();
    },
    validate: (value) => {
      const name = hashtagInput(value);
      return name === "" || isHashtagName(name) ? null : t("client:tag.name.invalid");
    },
    submit: (event, onAdded) => {
      event.preventDefault();
      const name = hashtagInput(tag);
      if (follow.isPending || name === "") return;
      follow.reset();
      follow.mutate({ tag: name, follow: true }, { onSuccess: () => onAdded?.() });
    },
    isPending: follow.isPending,
    error,
    status,
  };
}
