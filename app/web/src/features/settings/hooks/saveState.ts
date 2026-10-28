// SPDX-License-Identifier: AGPL-3.0-only
// The save state of a setting that saves itself as soon as it changes (web-client/16-look-states-and-wording.md G24),
// from the mutation that saves it, and the words of @my/ui's SaveIndicator in the app's locale.
import type { SaveIndicatorLabels, SaveState } from "@my/ui";
import { useT } from "../../../lib/i18n";

/** The state of a save from its mutation's flags: on its way, kept (until the next change), not kept, or none yet. */
export function saveStateOf(mutation: { isPending: boolean; isSuccess: boolean; isError: boolean }): SaveState {
  if (mutation.isPending) return "saving";
  if (mutation.isError) return "failed";
  if (mutation.isSuccess) return "saved";
  return "idle";
}

/** The words of a save state ("Saving…", "Saved", "Not saved"). */
export function useSaveLabels(): SaveIndicatorLabels {
  const t = useT();
  return {
    saving: t("client:shell.save.saving"),
    saved: t("client:shell.save.saved"),
    failed: t("client:shell.save.failed"),
  };
}
