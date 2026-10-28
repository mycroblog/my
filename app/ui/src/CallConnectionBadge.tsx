// SPDX-License-Identifier: AGPL-3.0-only
// The state of the user's own connection to a voice chat call, apart from the room's "Live" pill (the room is open):
// a status badge with one shape per state (a weak signal while connecting, a full one when connected, two arrows
// turning while reconnecting), the state's name, and a tone (G10, G13; ADR-0110 P3). The room's page and the
// mini-player show the same badge. Reconnecting also turns its arrows, and connecting pulses; with reduced motion they
// hold still, and the shape and the tone still say the state.
import { tv } from "tailwind-variants";
import { CallConnectedIcon, CallConnectingIcon, CallReconnectingIcon } from "./Icons";
import { StatusBadge } from "./StatusBadge";

/** The user's connection to the call. */
export type CallConnectionState = "connecting" | "connected" | "reconnecting";

const motion = tv({
  variants: {
    state: {
      connecting: "motion-safe:[&>svg]:animate-pulse",
      connected: "",
      reconnecting: "motion-safe:[&>svg]:animate-spin",
    },
  },
});

const tones = { connecting: "neutral", connected: "success", reconnecting: "warning" } as const;

const icons = {
  connecting: <CallConnectingIcon />,
  connected: <CallConnectedIcon />,
  reconnecting: <CallReconnectingIcon />,
} as const;

export type CallConnectionBadgeProps = {
  state: CallConnectionState;
  /** The state's name ("Connected", "Reconnecting…"). */
  label: string;
  className?: string;
};

/** The badge of the user's connection to the call. */
export function CallConnectionBadge({ state, label, className }: CallConnectionBadgeProps) {
  return (
    <StatusBadge
      icon={icons[state]}
      label={label}
      tone={tones[state]}
      data-connection={state}
      className={motion({ state, className })}
    />
  );
}
