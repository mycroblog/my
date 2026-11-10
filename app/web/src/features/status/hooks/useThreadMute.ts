// SPDX-License-Identifier: AGPL-3.0-only
// "Mute this thread" in the menu of a post (ADR-0158 §8, web-client §13): it waits for the server, then marks the
// post and the posts of its thread in the cache, and says so in a toast, since the menu is closed by then (G24). A
// failure changes nothing and says why (E2).
import { useToast } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useViewer } from "../../session";
import type { Status } from "../api/status";
import { cacheThreadMuteAnswer, setThreadMute } from "../api/threadMute";

export type ThreadMute = {
  /** Whether the viewer mutes the post's thread (`Status.muted`). */
  isMuted: boolean;
  /** Mutes the thread, or unmutes it while it is muted. */
  toggle: () => void;
};

/** The mute of the thread of `post` (not a boost: the boosted post), for a signed-in viewer; null otherwise. */
export function useThreadMute(post: Status): ThreadMute | null {
  const api = useApi();
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  const viewer = useViewer();
  const mutation = useMutation({
    mutationFn: ({ on }: { on: boolean }) => setThreadMute(api, post.id, on),
    // One request at a time per post, in the order chosen.
    scope: { id: `status:${post.id}:mute` },
    onSuccess: (answer, { on }) => {
      cacheThreadMuteAnswer(queryClient, post.id, answer);
      // The toast is announced too.
      toast.show({ message: on ? t("client:thread_mute.done") : t("client:thread_mute.undone"), tone: "success" });
    },
    onError: (error, { on }) => {
      const reason = errorMessage(error);
      toast.show({
        message: on ? t("client:thread_mute.failed", { reason }) : t("client:thread_mute.undo_failed", { reason }),
        tone: "danger",
      });
    },
  });
  if (viewer.status !== "signed-in") return null;
  const isMuted = post.muted === true;
  return {
    isMuted,
    toggle: () => {
      mutation.mutate({ on: !isMuted });
    },
  };
}
