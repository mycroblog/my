// SPDX-License-Identifier: AGPL-3.0-only

// The probe of the lab, which runs in each document a scenario loads before the page's own scripts (a Playwright init
// script, built from this file into one script by ux/test.ts): it watches the platform's vitals with the field
// measurement's own definitions (@my/platform/measure) and sends each value it hears to the test through the binding
// `probeBinding`, with the document it heard it in and, once, the document's module scripts, which name its entry. As
// the document is hidden or goes away, and when the test asks (`window.myUxSettle()`), it settles the largest paint
// and what the observers hold.

import { watchVitals } from "@my/platform/measure";
import type { Sample } from "./measures";

/** The binding the test exposes to the page, which the probe sends each sample to. */
export const probeBinding = "myUxSample";

type ProbeWindow = Window & {
  [probeBinding]?: (sample: Sample) => void;
  myUxSettle?: () => void;
};

if (typeof window !== "undefined") {
  const w = window as ProbeWindow;
  const doc = Math.random().toString(36).slice(2);
  const send = (sample: Sample) => {
    w[probeBinding]?.(sample);
  };
  const watched = watchVitals({
    largestPaint: (value) => {
      send({ doc, kind: "lcp", value });
    },
    interaction: (value) => {
      send({ doc, kind: "interaction", value });
    },
    longFrame: (value) => {
      send({ doc, kind: "long_frame", value });
    },
  });
  let described = false;
  const describe = () => {
    if (described) return;
    described = true;
    const scripts = Array.from(document.scripts)
      .filter((s) => s.type === "module" && s.src !== "")
      .map((s) => s.src);
    send({ doc, kind: "document", scripts });
  };
  const settle = () => {
    describe();
    watched.settle();
  };
  document.addEventListener("DOMContentLoaded", describe, { once: true });
  window.addEventListener("pagehide", settle);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") settle();
  });
  w.myUxSettle = settle;
}
