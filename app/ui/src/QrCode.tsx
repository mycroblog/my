// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef } from "react";
import { tv } from "tailwind-variants";

const qrCode = tv({
  // QR readers need dark modules on a light ground, so the code keeps the light theme's colors in dark mode too
  // (color-scheme: light makes the tokens' light-dark() pick their light side).
  slots: {
    base: "size-48 shrink-0 rounded-lg scheme-light",
    ground: "fill-surface-raised",
    modules: "fill-fg",
  },
});

export type QrCodeProps = Omit<ComponentPropsWithRef<"svg">, "children" | "viewBox" | "role"> & {
  /** The width and height of the code in modules, its quiet zone included. */
  size: number;
  /** The SVG path data of the dark modules, one unit per module, as the server draws it. */
  path: string;
  /** What the code holds, read instead of the picture ("QR code of the authenticator app's secret"). */
  label: string;
};

/**
 * A QR code that the server encoded, drawn from its path: a picture named by `label`. Whoever cannot scan it
 * needs what it holds in text next to it (the caller's).
 */
export function QrCode({ size, path, label, className, ...props }: QrCodeProps) {
  const { base, ground, modules } = qrCode();
  const box = String(size);
  return (
    <svg
      {...props}
      viewBox={`0 0 ${box} ${box}`}
      role="img"
      aria-label={label}
      shapeRendering="crispEdges"
      className={base({ className })}
    >
      <rect width={size} height={size} className={ground()} />
      <path d={path} className={modules()} />
    </svg>
  );
}
