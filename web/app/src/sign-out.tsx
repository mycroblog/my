// SPDX-License-Identifier: AGPL-3.0-only

// Signing out from the app (ADR-0090 §13, ADR-0028 §3), after a confirmation alone: the session ends on the server,
// through the web client's token revocation (@my/app-data), and only then what the session left in this browser goes:
// the cache of server data, the language and time zone kept for the first paint (reader.tsx), and every cache of the
// Service Worker. The app is then left for its start, which loads anew, signed out: the post still being written and
// the timelines live in the page alone, and go with it. What the reader saved on the server (their drafts) stays. When
// the server does not take the revocation, nothing is removed, the reader stays signed in, and the confirmation says
// so. The settings' menu and the closed server's page both sign out this way. A session the server ended with what the
// reader did on a screen (revoking the server's own web client among the authorized apps) leaves the same way, with
// no question, since nothing is left to refuse.

import { endSession } from "@my/app-data";
import { SignOutDialog } from "@my/app-features";
import { ModalPresence } from "@my/design-system";
import { clearCaches } from "@my/platform";
import { useRouter } from "@tanstack/react-router";
import { forgetChoice } from "./reader";
import type { RouterContext } from "./router";

/** Where the app starts again once signed out. */
const startPath = "/";

type Session = Pick<RouterContext, "data" | "config" | "leave">;

/** Removes what the session left in this browser, and leaves the app for its start, which loads anew. */
async function leaveSignedOut({ leave }: Session): Promise<void> {
  forgetChoice();
  // A browser that refuses to delete a cache keeps public files alone, which the next reader's worker replaces.
  await clearCaches().catch(() => undefined);
  leave(startPath);
}

/**
 * Signs the reader of `context` out and leaves the app for its start. It rejects, with nothing removed, when the
 * server did not end the session.
 */
async function signOut(context: Session): Promise<void> {
  const { data, config } = context;
  if (data !== null) await endSession(data, config.clientId);
  await leaveSignedOut(context);
}

/**
 * Leaves the app as after signing out, for a session of `context` that the server has ended already. The token went
 * with the session, so a revocation the server does not take changes nothing: the cache of server data is emptied all
 * the same.
 */
export async function leaveEndedSession(context: Session): Promise<void> {
  const { data, config } = context;
  if (data !== null)
    await endSession(data, config.clientId).catch(() => {
      data.queryClient.clear();
    });
  await leaveSignedOut(context);
}

/** The confirmation of signing out, over the page while `asking`; confirmed, it signs out and the app leaves. */
export function SignOutConfirm({ asking, onCancel }: { asking: boolean; onCancel: () => void }) {
  const { context } = useRouter().options;
  return (
    <ModalPresence>{asking && <SignOutDialog onCancel={onCancel} signOut={() => signOut(context)} />}</ModalPresence>
  );
}
