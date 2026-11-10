// SPDX-License-Identifier: AGPL-3.0-only
// Following a hashtag from its page (web-client/screens/tag.md): whether the viewer follows it, and the press that
// follows it or stops. Only a signed-in viewer follows; a visitor sees no button.
import { type FollowStatus, useNotice, useToast } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { hashtagFailure, maxFollowedTags, storeTagFollow, tagFollowMutation, tagQueryOptions } from "../api/tags";
import { hashtagInText } from "./text";

/** The follow button of a hashtag's page. */
export type TagFollow =
  /** Nobody is signed in, or the hashtag could not be read: no button. */
  | { kind: "hidden" }
  | {
      kind: "button";
      /** `loading` keeps the button's place until the server answers. */
      status: Extract<FollowStatus, "none" | "following" | "loading">;
      label: string;
      isPending: boolean;
      /** Follows the hashtag, or stops; the result is announced, a failure shown in a toast. */
      toggle: () => void;
    };

/** The follow of the hashtag `tag` (without `#`), as the address of its page names it. */
export function useTagFollow(tag: string): TagFollow {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const toast = useToast();
  const signedIn = useSessionStatus() === "signed-in";
  const query = useQuery({ ...tagQueryOptions(api, tag), enabled: signedIn });
  const mutation = useMutation({
    ...tagFollowMutation(api),
    onSuccess: async (result) => {
      // Done once what it changes on screen is fetched again; the home timeline loads again when it opens (S1).
      await storeTagFollow(queryClient, api, result);
      const name = hashtagInText(result.hashtag.name);
      notice.inPlace(
        result.hashtag.following ? t("client:tag.followed", { tag: name }) : t("client:tag.unfollowed", { tag: name }),
      );
    },
    onError: (error) => {
      const message =
        hashtagFailure(error) === "too-many"
          ? t("client:tag.follow.too_many", { max: String(maxFollowedTags) })
          : t("client:tag.follow.failed", { tag: hashtagInText(tag), reason: errorMessage(error) });
      toast.show({ message, tone: "danger" });
    },
  });
  if (!signedIn || query.isError) return { kind: "hidden" };
  const following = query.data?.following;
  return {
    kind: "button",
    status: following === undefined ? "loading" : following ? "following" : "none",
    label: following === true ? t("client:tag.following") : t("client:tag.follow"),
    isPending: mutation.isPending,
    toggle: () => {
      if (following === undefined || mutation.isPending) return;
      mutation.mutate({ tag, follow: !following });
    },
  };
}
