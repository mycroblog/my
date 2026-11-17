// SPDX-License-Identifier: AGPL-3.0-only
// restrict/token-colors: a colour is a token of @my/design-tokens (ADR-0171 §2).
// web/devtools/src/restrictions.test.js lints it; the lint of the package leaves it out.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";

// The colours of the theme, what is no colour of its own, and what only looks like one.
const fine = tv({
  base: [
    "bg-surface text-muted-text border-spinner-track bg-transparent text-current shadow-focus",
    "bg-[var(--color-surface)] text-[length:var(--text-title)] outline-[length:var(--form-page-invalid)]",
    "hover:bg-[color-mix(in_srgb,var(--color-surface)_15%,transparent)]",
  ],
});
const mixed = "color-mix(in srgb, var(--color-text) 50%, transparent)";
const anchors = ["#main", "/tags#add", "https://my.example/page#fed", "&#8203;", "#"];
const words = { kind: "red", tone: "white", align: "center" };

const raw = tv({
  base: [
    "bg-[#ff6b6b]", // error: color #ff6b6b
    "text-[#111]/50", // error: color #111
    "border-[rgb(0_0_0_/_50%)]", // error: color rgb()
    "shadow-[0_0_0_1px_hsl(0_0%_0%)]", // error: color hsl()
    "bg-[oklch(0.7_0.1_200)]", // error: color oklch()
    "text-[red]", // error: color red
    "[color:rebeccapurple]", // error: color rebeccapurple
    "fill-[color:white]", // error: color white
  ],
});

export function Colors({ hue, children }: { hue: number; children: ReactNode }) {
  return (
    <div className={`${fine()} ${raw()}`} data-anchors={anchors.join(" ")} data-kind={words.kind}>
      <p style={{ color: "var(--color-text)", background: mixed, borderColor: "currentColor", fill: "none" }}>
        {children}
      </p>
      <p
        style={{
          color: "red", // error: color red
          background: "#FFFFFF", // error: color #FFFFFF
          border: "1px solid #ccc", // error: color #ccc
          boxShadow: "0 0 0 1px rgba(0, 0, 0, 0.2)", // error: color rgba()
          outline: `2px solid oklch(0.7 0.1 ${hue})`, // error: color oklch()
          textDecorationColor: "DarkSlateGray", // error: color DarkSlateGray
          backgroundImage: "linear-gradient(to bottom, #0008, transparent)", // error: color #0008
        }}
      >
        {children}
      </p>
      <svg viewBox="0 0 8 8" fill="none" stroke="currentColor">
        <rect
          width="8"
          height="8"
          fill="black" // error: color black
        />
        <circle
          r="2"
          stroke="#3f6f86" // error: color #3f6f86
        />
      </svg>
    </div>
  );
}
