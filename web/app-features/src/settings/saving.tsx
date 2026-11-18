// SPDX-License-Identifier: AGPL-3.0-only

// A setting that saves at once (ADR-0110 §1 P2, §5): where its last change is, as the save state its control shows
// (the design system's `SaveState`, in its own words). A change is saving until the server answers, saved for a
// moment, then nothing; one the server did not take stays not saved, with Try again, until the reader tries again or
// changes the setting; one whose value the server refused says the server's words, and trying again would not help.
// Only the last change of a setting speaks.

import { refusedField } from "@my/app-data";
import { SaveState } from "@my/design-system";
import { useEffect, useRef, useState, type ReactElement } from "react";

/** How long a setting says it was saved. */
const savedMs = 2000;

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
 * The save state of a setting's last change (`state`, as the `save` of a `SettingRow` or a `Switch` takes it: null
 * while there is nothing to say), and `save`, which runs a write of the setting and follows it.
 */
export function useSaving(): { state: ReactElement | null; save: (write: () => Promise<unknown>) => void } {
  const [saving, setSaving] = useState<Saving | null>(null);
  /** The last change, which alone speaks, and the timer that ends its "saved". */
  const last = useRef<{ run: number; timer: ReturnType<typeof setTimeout> | undefined }>({ run: 0, timer: undefined });
  useEffect(() => {
    const kept = last.current;
    return () => {
      kept.run += 1;
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
        if (last.current.run !== own) return;
        setSaving({ state: "saved" });
        last.current.timer = setTimeout(() => {
          setSaving(null);
        }, savedMs);
      },
      (error: unknown) => {
        if (last.current.run !== own) return;
        const refused = refusedField(error);
        setSaving(
          refused === undefined
            ? {
                state: "failed",
                retry: () => {
                  save(write);
                },
              }
            : { state: "failed", said: refused.detail },
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
  return { state, save };
}
