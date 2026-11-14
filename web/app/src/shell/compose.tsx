// SPDX-License-Identifier: AGPL-3.0-only

// Where the composer is open (ADR-0025 §4): over the screen the reader is on, a modal the shell holds, as the keyboard
// shortcuts' help is, not a place of the router. Opening it is no navigation, so the screen behind stays where it was
// and no View Transition runs: the composer rises in with its backdrop, and leaves the same way back, by Motion
// (@my/design-system's modal). It is a screen of its own on a phone and a dialog from `tablet` (the app's choice by the viewport).

import type { ComposeTarget } from "@my/app-features";
import { useState } from "react";

/** The composer's target, none while it is closed, and how to open and close it. */
export function useCompose() {
  const [target, setTarget] = useState<ComposeTarget | undefined>(undefined);
  const open = (to: ComposeTarget) => {
    setTarget(to);
  };
  const close = () => {
    setTarget(undefined);
  };
  return { target, open, close };
}
