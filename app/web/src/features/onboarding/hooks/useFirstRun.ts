// SPDX-License-Identifier: AGPL-3.0-only
// The first steps of a new user on the home timeline (web-client/screens/home-first-run.md): which of the three steps
// are done, read from the viewer's own account, whether the checklist and the suggestions show, and the celebration
// of a step done while the page is open (21-experience-principles.md §21.5). The hook returns no JSX.
import { type Celebration, useCelebration } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { firstRunDismissedKey, useStorage } from "../../../lib/storage";
import { viewerQueryOptions } from "../../session";

export const firstRunSteps = ["profile", "follow", "post"] as const;
export type FirstRunStep = (typeof firstRunSteps)[number];

type Done = Readonly<Record<FirstRunStep, boolean>>;

export type FirstRunState =
  | { status: "hidden" }
  | {
      status: "shown";
      /** Whether each step is done. */
      done: Done;
      /** The celebration of each step, which starts when the step is done while the page is open. */
      celebrations: Readonly<Record<FirstRunStep, Celebration>>;
      showChecklist: boolean;
      showSuggestions: boolean;
      /** Closes the checklist on this browser. */
      dismiss: () => void;
    };

function sameDone(a: Done, b: Done): boolean {
  return firstRunSteps.every((step) => a[step] === b[step]);
}

export function useFirstRun(): FirstRunState {
  const api = useApi();
  // The shell reads the viewer on every page; this only reads its answer.
  const { data: account } = useQuery({ ...viewerQueryOptions(api), ...shellQueryReader });
  const dismissed = useStorage(firstRunDismissedKey(account?.id ?? ""));
  const done: Done = {
    profile: account !== undefined && (account.display_name !== "" || account.avatar !== undefined),
    // The viewer's own account carries its follow counts; without them, the step is not asked for.
    follow: account !== undefined && (account.following_count ?? 1) > 0,
    post: account !== undefined && account.statuses_count > 0,
  };
  const celebrations = {
    profile: useCelebration(),
    follow: useCelebration(),
    post: useCelebration(),
  };
  // What the page started with: a step already done then is not celebrated, and the parts shown then stay until the
  // page is left, so that the result of a follow stays in sight (home-first-run.md §3).
  const [initial, setInitial] = useState<Done | undefined>(account === undefined ? undefined : done);
  const [seen, setSeen] = useState<Done | undefined>(initial);
  if (account !== undefined && initial === undefined) {
    setInitial(done);
    setSeen(done);
  } else if (seen !== undefined && !sameDone(seen, done)) {
    // Adjusting state while rendering, on a change of the account (react.dev: "Adjusting some state when a prop changes").
    for (const step of firstRunSteps) if (done[step] && !seen[step]) celebrations[step].celebrate();
    setSeen(done);
  }
  if (account === undefined || initial === undefined) return { status: "hidden" };
  const showChecklist = !dismissed && !firstRunSteps.every((step) => initial[step]);
  const showSuggestions = !initial.follow;
  if (!showChecklist && !showSuggestions) return { status: "hidden" };
  return {
    status: "shown",
    done,
    celebrations,
    showChecklist,
    showSuggestions,
    dismiss: () => {
      firstRunDismissedKey(account.id).write(true);
    },
  };
}
