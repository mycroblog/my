// SPDX-License-Identifier: AGPL-3.0-only

// Signing out from the app (ADR-0090 §13, ADR-0028 §3), after a confirmation alone: the session ends on the server,
// through the web client's token revocation (@my/app-data), and only then what the session left in this browser goes:
// this tab's cache of server data, the language and time zone kept for the first paint (reader.tsx), and every cache
// of the Service Worker, with the worker (it installs anew on the next load). The app is then left for its start,
// which loads anew, signed out: the post still being written and the timelines live in the page alone, and go with
// it. Every other tab of the browser hears of it, empties its own cache of server data and loads anew at the start
// too, with what was being written there. What the reader saved on the server (their drafts) stays.
//
// When the app has no token to revoke with (the server had ended the token's family), the same is removed, but the
// session of the server's own sign-in pages may still live, with which the next Sign in would pass without a
// password: the app is left for the server's sign-in page, which shows that session with its Sign out, or the
// sign-in form when there is none. When the server does not take the revocation, nothing is removed, the reader stays
// signed in, and the confirmation says so. The settings' menu and the closed server's page both sign out this way.
//
// A session the server ended with what the reader did on a screen (revoking the server's own web client among the
// authorized apps) leaves the same way, with no question, since nothing is left to refuse.

import { endSession, type SessionEnd } from "@my/app-data";
import { SignOutDialog } from "@my/app-features";
import { ModalPresence } from "@my/design-system";
import { clearCaches } from "@my/platform";
import { useRouter } from "@tanstack/react-router";
import { forgetChoice } from "./reader";
import type { RouterContext } from "./router";
import { serverSignInPath } from "./sign-in";

/** Where the app starts again once signed out. */
const startPath = "/";

type Session = Pick<RouterContext, "data" | "config" | "leave">;

/** What the session left in this browser, which goes with it. */
async function removeKept(): Promise<void> {
  forgetChoice();
  // A browser that refuses to delete a cache keeps public files alone, which the next reader's worker replaces.
  await clearCaches().catch(() => undefined);
}

/**
 * Signs the reader of `context` out and leaves the app: for its start, or for the server's sign-in page when there
 * was no token to end the server's session with. It rejects, with nothing removed, when the server did not take the
 * revocation.
 */
async function signOut({ data, config, leave }: Session): Promise<void> {
  let end: SessionEnd = "revoked";
  if (data === null) await removeKept();
  else end = await endSession(data, config.clientId, removeKept);
  leave(end === "revoked" ? startPath : serverSignInPath);
}

/**
 * What a tab does when the reader signed out in another tab of the browser: it empties its cache of server data and
 * leaves for the app's start, which loads anew, signed out. The tab that signed out removed what the browser kept.
 */
export function followSignOut({ data, leave }: Pick<RouterContext, "data" | "leave">): void {
  data?.queryClient.clear();
  leave(startPath);
}

/**
 * Leaves the app as after signing out, for a session of `context` that the server has ended already. The token went
 * with the session, so a revocation the server does not take changes nothing: what the session left in this browser
 * is removed all the same, and the app is left for its start.
 */
export async function leaveEndedSession({ data, config, leave }: Session): Promise<void> {
  if (data === null) await removeKept();
  else
    await endSession(data, config.clientId, removeKept).catch(async () => {
      data.queryClient.clear();
      await removeKept();
    });
  leave(startPath);
}

/** The confirmation of signing out, over the page while `asking`; confirmed, it signs out and the app leaves. */
export function SignOutConfirm({ asking, onCancel }: { asking: boolean; onCancel: () => void }) {
  const { context } = useRouter().options;
  return (
    <ModalPresence>{asking && <SignOutDialog onCancel={onCancel} signOut={() => signOut(context)} />}</ModalPresence>
  );
}
