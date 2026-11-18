// SPDX-License-Identifier: AGPL-3.0-only

// What a control that saves at once has to say of its last change (ADR-0110 §5), in the design system's own words:
// the save state (save-state.tsx) of a change on its way, saved a moment ago, or not saved, which the reader may try
// again; a change the server refused says the server's words about its value, with nothing to try again. Its owner
// holds where the change is (`Saving`) and gives the result to the control's `save` (a `SettingRow`'s, a `Switch`'s).

import type { ReactElement } from "react";
import { TextButton } from "./button";
import { stringsIn, useReader } from "./locale";
import { SaveState } from "./save-state";

/** A change that was not saved. */
type NotSaved = {
  readonly state: "failed";
  /** What the server said of a value it refused, in place of the design system's words. */
  readonly said?: string | undefined;
  /** Saves the change again; left out where trying again cannot help (a refused value). */
  readonly retry?: (() => void) | undefined;
};

/** Where a control's last change is: on its way, saved, or not saved. */
export type Saving = { readonly state: "saving" } | { readonly state: "saved" } | NotSaved;

/**
 * The save state of `saving` in the reader's language, as the `save` of a control that saves at once takes it; null
 * while there is nothing to say.
 */
export function useSaveState(saving: Saving | null): ReactElement | null {
  const { t } = stringsIn(useReader(undefined).lang);
  if (saving === null) return null;
  if (saving.state === "saving") return <SaveState state="saving">{t("save_state.saving")}</SaveState>;
  if (saving.state === "saved") return <SaveState state="saved">{t("save_state.saved")}</SaveState>;
  const { said, retry } = saving;
  return (
    <SaveState
      state="failed"
      action={retry === undefined ? undefined : <TextButton onPress={retry}>{t("save_state.retry")}</TextButton>}
    >
      {said ?? t("save_state.failed")}
    </SaveState>
  );
}
