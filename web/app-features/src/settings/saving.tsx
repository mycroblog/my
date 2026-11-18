// SPDX-License-Identifier: AGPL-3.0-only

// A setting that saves at once (ADR-0110 §1 P2, §5): where its last change is, as the save state its control shows
// (the design system's `SaveState`, in its own words). A change is saving until the server answers, saved for a
// moment, then nothing; one the server did not take stays not saved, with Try again, until the reader tries again or
// changes the setting; one whose value the server refused says the server's words, and one the write itself explains
// (`Said`) says the write's, and trying again would not help either.
// Only the last change of a setting speaks. A change that is not saved once its control is gone (the reader left the
// page while it was on its way) has no row to say so: it is told to the toast of the settings (unsaved.tsx), with the
// setting's name and its page (ADR-0110 §6).

import { refusedField } from "@my/app-data";
import { SaveState } from "@my/design-system";
import { useEffect, useRef, useState, type ReactElement } from "react";
import { reportUnsaved } from "./unsaved";

/** How long a setting says it was saved. */
const savedMs = 2000;

/**
 * What a write rejects with when it knows why the change did not hold and trying the same again would not help (the
 * browser refused it): the setting says these words, the message, in the reader's language.
 */
export class Said extends Error {}

/** Where a setting's last change is: on its way, saved, or not saved. */
type Saving =
  | { readonly state: "saving" }
  | { readonly state: "saved" }
  | {
      readonly state: "failed";
      /** What the server said of a value it refused. */
      readonly said?: string | undefined;
      /** Saves the change again; none for a refused value. */
      readonly retry?: (() => void) | undefined;
    };

/**
 * The save state of the last change of the setting named `setting`, on the settings page at `page` (`state`, as the
 * `save` of a `SettingRow` or a `Switch` takes it: null while there is nothing to say), `save`, which runs a write
 * of the setting and follows it, past the control's own life, and `forget`, after which the state says nothing of the
 * change that last ended (what it said is of another time: its control could not be changed since, and can again).
 */
export function useSaving(
  setting: string,
  page: string,
): { state: ReactElement | null; save: (write: () => Promise<unknown>) => void; forget: () => void } {
  const [saving, setSaving] = useState<Saving | null>(null);
  /** The last change, which alone speaks, the timer that ends its "saved", and whether the control is gone. */
  const last = useRef<{ run: number; timer: ReturnType<typeof setTimeout> | undefined; gone: boolean }>({
    run: 0,
    timer: undefined,
    gone: false,
  });
  useEffect(() => {
    const kept = last.current;
    kept.gone = false;
    return () => {
      kept.gone = true;
      clearTimeout(kept.timer);
    };
  }, []);
  const save = (write: () => Promise<unknown>) => {
    const own = last.current.run + 1;
    last.current.run = own;
    clearTimeout(last.current.timer);
    setSaving({ state: "saving" });
    write().then(
      () => {
        if (last.current.run !== own || last.current.gone) return;
        setSaving({ state: "saved" });
        last.current.timer = setTimeout(() => {
          setSaving(null);
        }, savedMs);
      },
      (error: unknown) => {
        if (last.current.run !== own) return;
        if (last.current.gone) {
          reportUnsaved(setting, page);
          return;
        }
        const said = error instanceof Said ? error.message : refusedField(error)?.detail;
        setSaving(
          said === undefined
            ? {
                state: "failed",
                retry: () => {
                  save(write);
                },
              }
            : { state: "failed", said },
        );
      },
    );
  };
  const state =
    saving === null ? null : saving.state === "failed" ? (
      <SaveState state="failed" onRetry={saving.retry}>
        {saving.said}
      </SaveState>
    ) : (
      <SaveState state={saving.state} />
    );
  const forget = () => {
    setSaving(null);
  };
  return { state, save, forget };
}
