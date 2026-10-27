// SPDX-License-Identifier: AGPL-3.0-only
// The app's voice chat call (useVoiceCallProvider), for every page: the room's page and the mini-player read it. The app
// shell renders it once, inside the session, the Query cache, and the live regions.
import type { ReactNode } from "react";
import type { Maintenance } from "../../../lib/maintenance";
import { useVoiceCallProvider, VoiceCallContext } from "../hooks/useVoiceCallProvider";

export type VoiceCallProviderProps = {
  /** The app's maintenance: its screen replaces the pages, and ends the call. */
  maintenance: Maintenance;
  children: ReactNode;
};

export function VoiceCallProvider({ maintenance, children }: VoiceCallProviderProps) {
  const state = useVoiceCallProvider(maintenance);
  return <VoiceCallContext value={state}>{children}</VoiceCallContext>;
}
