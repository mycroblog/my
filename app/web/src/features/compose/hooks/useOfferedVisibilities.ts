// SPDX-License-Identifier: AGPL-3.0-only
// The visibilities that a new post and the default of the preferences may have on this server: every one, except
// `local` on a one-person server, where it reaches no more than the followers and the server refuses it (ADR-0152 §5).
// Posts that already have it keep it and show it as before.
import { useSingleUser } from "../../../lib/singleUser";
import type { Visibility } from "../api/compose";
import { visibilities } from "./visibilityMessages";

/** The visibilities offered on a server that is, or is not, a one-person server, from the widest to the narrowest. */
export function offeredVisibilities(singleUser: boolean): readonly Visibility[] {
  return singleUser ? visibilities.filter((visibility) => visibility !== "local") : visibilities;
}

/** The visibilities the server offers for new posts. */
export function useOfferedVisibilities(): readonly Visibility[] {
  return offeredVisibilities(useSingleUser());
}
