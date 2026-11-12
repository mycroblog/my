// SPDX-License-Identifier: AGPL-3.0-only

// Live options (ADR-0168 §1): an agent wraps the variants of an element or a page in <Options>, and the decider
// switches between them in the overlay and picks one, which comes back as a note. Only the dev server loads
// @my/devtools, so a production build fails while an <Options> is left: keep the picked variant, remove the rest.

import { useEffect, useState, type ReactNode } from "react";
import { optionsAttribute, optionsEvent, switchEvent } from "./note";

const key = (id: string) => `my-devtools:option:${id}`;

function stored(id: string): string | null {
  try {
    return sessionStorage.getItem(key(id));
  } catch {
    return null;
  }
}

/**
 * Renders the variant the decider chose, the first one until then.
 *
 *   <Options id="swatch-shape" variants={{ square: <Swatch />, round: <Swatch round /> }} />
 */
export function Options({ id, variants }: { id: string; variants: Record<string, ReactNode> }) {
  const names = Object.keys(variants);
  const [chosen, setChosen] = useState(() => (typeof window === "undefined" ? null : stored(id)));
  const current = chosen !== null && names.includes(chosen) ? chosen : (names[0] ?? "");

  useEffect(() => {
    const onSwitch = (e: Event) => {
      const { detail } = e as CustomEvent<{ id: string; variant: string }>;
      if (detail.id !== id) return;
      setChosen(detail.variant);
      try {
        sessionStorage.setItem(key(id), detail.variant);
      } catch {
        // Storage is off: the choice lasts until the page reloads.
      }
    };
    window.addEventListener(switchEvent, onSwitch);
    return () => {
      window.removeEventListener(switchEvent, onSwitch);
    };
  }, [id]);

  // The overlay reads the options from the DOM again whenever one mounts, switches, or goes away.
  useEffect(() => {
    window.dispatchEvent(new Event(optionsEvent));
    return () => {
      window.dispatchEvent(new Event(optionsEvent));
    };
  }, [current]);

  return (
    <div
      {...{ [optionsAttribute]: id }}
      data-variant={current}
      data-variants={JSON.stringify(names)}
      style={{ display: "contents" }}
    >
      {variants[current]}
    </div>
  );
}
