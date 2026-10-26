// SPDX-License-Identifier: AGPL-3.0-only
// The app's voice chat call (useVoiceCallProvider), for every page: the room's page and the call bar read it. The app
// shell renders it once, inside the session, the Query cache, and the live regions.
import type { ReactNode } from "react";
import { useVoiceCallProvider, VoiceCallContext } from "../hooks/useVoiceCallProvider";

export function VoiceCallProvider({ children }: { children: ReactNode }) {
  const state = useVoiceCallProvider();
  return <VoiceCallContext value={state}>{children}</VoiceCallContext>;
}
