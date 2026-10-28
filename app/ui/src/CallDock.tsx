// SPDX-License-Identifier: AGPL-3.0-only
// The controls of a voice chat call on the room's page: a raised pill of round buttons (the microphone, reactions,
// invite, the raised hands) with leaving at its end. It is a toolbar (WAI-ARIA APG, toolbar): the arrow keys move
// between its buttons in the order of the line (React Aria's Toolbar mirrors them in
// right-to-left text), and Tab leaves it. It wraps onto a second row on narrow screens.
//
// Place it at the end of the page in the shell's `ShellDock`, which keeps it at the bottom edge of the page's column,
// clear of the tab bar; like `CallBar`, its height is the `--call-bar-block-size` of the document while it shows.
import { type ReactNode, useId, useRef } from "react";
import {
  Button as RACButton,
  type ButtonProps as RACButtonProps,
  ToggleButton as RACToggleButton,
  Toolbar,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import { Badge } from "./Badge";
import { useCallBarBlockSize } from "./callBarBlockSize";
import { MicrophoneIcon, MicrophoneOffIcon } from "./Icons";
import { Tooltip, TooltipTrigger } from "./Tooltip";

const callDock = tv({
  slots: {
    base: "flex flex-col gap-2",
    // Rounded as a pill on one row; the radius stays clear of the buttons when the row wraps.
    toolbar: "flex flex-wrap items-center gap-2 rounded-2xl border border-border bg-surface-raised p-2 shadow-lg",
    end: "ms-auto flex flex-wrap items-center gap-2",
    notice: "rounded-lg border border-border bg-surface-raised px-4 py-2 shadow-md",
  },
});

export type CallDockProps = {
  /** The toolbar's accessible name ("Call controls"). */
  label: string;
  /** The `CallDockButton`s, in their order on the line. */
  children: ReactNode;
  /** What comes last, after the room left on the line: the leave button. */
  end?: ReactNode;
  /** What failed (the microphone, leaving), under the toolbar; nothing for none. */
  notice?: ReactNode;
};

/** The pill toolbar of a call's controls. */
export function CallDock({ label, children, end, notice }: CallDockProps) {
  const slots = callDock();
  const ref = useRef<HTMLDivElement>(null);
  useCallBarBlockSize(ref);
  return (
    <div ref={ref} data-call-bar="" className={slots.base()}>
      <Toolbar aria-label={label} className={slots.toolbar()}>
        {children}
        {end !== undefined && end !== null && <div className={slots.end()}>{end}</div>}
      </Toolbar>
      {notice !== undefined && notice !== null && notice !== false && <div className={slots.notice()}>{notice}</div>}
    </div>
  );
}

const callDockButton = tv({
  slots: {
    base: [
      "relative inline-flex shrink-0 cursor-default items-center justify-center gap-2 rounded-full font-extrabold select-none",
      // A control that waits pulses where it was pressed (ADR-0110 P2); with reduced motion, the wait cursor alone.
      "data-disabled:opacity-50 data-focus-visible:focus-ring data-pending:cursor-wait motion-safe:data-pending:animate-pulse",
      "motion-safe:transition-[color,background-color,box-shadow] [&>svg]:size-5 [&>svg]:shrink-0",
    ],
    count: "",
  },
  variants: {
    size: {
      /** The dock of the room's page. */
      md: { base: "h-12 min-w-12 text-base" },
      /** A smaller control: the mini-player's. */
      sm: { base: "h-11 min-w-11 text-sm" },
    },
    variant: {
      /** The main control: the microphone that is on. */
      primary: {
        base: "bg-accent-fill text-accent-fill-fg data-hovered:bg-accent-fill-hover data-pressed:bg-accent-fill-hover",
      },
      /** The other controls, on cream. */
      soft: { base: "bg-surface-sunken text-fg data-hovered:bg-soft data-pressed:bg-soft" },
      /** Leaving the call, as the dock's last button. */
      danger: { base: "bg-danger text-danger-fg data-hovered:shadow-md data-pressed:shadow-sm" },
      /** Leaving the call beside other controls that it should not outweigh: the mini-player's. */
      "danger-soft": { base: "bg-danger-bg text-danger data-hovered:shadow-md data-pressed:shadow-sm" },
    },
    /** The look of a toggle that is on; the icon should change its shape too (web-client/16-look-states-and-wording.md G13). */
    selectedTone: {
      accent: {
        base: "data-selected:bg-accent-fill data-selected:text-accent-fill-fg data-selected:data-hovered:bg-accent-fill-hover",
      },
      hand: { base: "data-selected:bg-hand data-selected:text-hand-fg data-selected:data-hovered:shadow-md" },
    },
    showLabel: {
      true: { base: "px-4" },
      // Only the icon: a circle, with the count over its top end corner.
      false: { base: "px-0", count: "absolute -inset-e-1 -top-1" },
    },
  },
  defaultVariants: { size: "md", variant: "soft", selectedTone: "accent", showLabel: false },
});

type CallDockButtonBaseProps = {
  /** Turns the control off: it stays in view, half transparent. */
  isDisabled?: boolean;
  /**
   * What the control does, in a few words ("Microphone", "Reactions"): its accessible name, and its text with
   * `showLabel` or else its tooltip (web-client/16-look-states-and-wording.md G12). A toggle keeps the same name in
   * both states.
   */
  label: string;
  /** The icon: before the text, or alone. */
  icon: ReactNode;
  /** Shows the label beside the icon, in a pill; without it, the button is a circle with the label as its tooltip. */
  showLabel?: boolean;
  /** `primary` (coral), `soft` (cream, default), `danger` (leaving) or `danger-soft`; a toggle's look while it is off. */
  variant?: "primary" | "soft" | "danger" | "danger-soft";
  /** `md` (48 px, default) in the dock, `sm` (44 px) in the mini-player. */
  size?: "md" | "sm";
  /** A count over the button ("2" raised hands), with the words assistive technology reads as its description. */
  count?: { value: number; label: string } | undefined;
};

type CallDockPlainButtonProps = CallDockButtonBaseProps & {
  isSelected?: undefined;
  /** Runs when the button is pressed. Inside a `MenuTrigger` or a `DialogTrigger`, the trigger opens its overlay. */
  onPress?: RACButtonProps["onPress"];
  /** Waits for the action: presses are ignored while the button keeps its focus. */
  isPending?: boolean;
};

type CallDockToggleButtonProps = CallDockButtonBaseProps & {
  /** Whether the toggle is on (`aria-pressed`). */
  isSelected: boolean;
  /** Asks for the other state. */
  onChange: (isSelected: boolean) => void;
  /** The look while on: `accent` (coral, default) for the microphone, `hand` (mango) for the raised hand. */
  selectedTone?: "accent" | "hand";
};

export type CallDockButtonProps = CallDockPlainButtonProps | CallDockToggleButtonProps;

/**
 * A control of the call: a round icon button or an icon and text pill, plain or a toggle (`isSelected` /
 * `onChange`; WAI-ARIA APG, button pattern). Every one is at least 44 px square (WCAG 2.5.8).
 */
export function CallDockButton(props: CallDockButtonProps) {
  const { label, icon, showLabel = false, variant, size, count, isDisabled = false } = props;
  const countId = useId();
  const slots = callDockButton({
    size,
    variant,
    showLabel,
    selectedTone: props.isSelected === undefined ? undefined : props.selectedTone,
  });
  const className = slots.base({ className: showLabel ? undefined : size === "sm" ? "w-11" : "w-12" });
  const content = (
    <>
      {icon}
      {showLabel && <span>{label}</span>}
      {count !== undefined && (
        <Badge id={countId} tone="hand" count={count.value} label={count.label} className={slots.count()} />
      )}
    </>
  );
  // The name is the label alone, also with a count: the count describes the button, so its name stays the same.
  const common = {
    "aria-label": label,
    ...(count === undefined ? {} : { "aria-describedby": countId }),
    isDisabled,
    className,
  };
  const button =
    props.isSelected === undefined ? (
      <RACButton
        {...common}
        {...(props.onPress === undefined ? {} : { onPress: props.onPress })}
        isPending={props.isPending ?? false}
      >
        {content}
      </RACButton>
    ) : (
      <RACToggleButton {...common} isSelected={props.isSelected} onChange={props.onChange}>
        {content}
      </RACToggleButton>
    );
  if (showLabel) return button;
  return (
    <TooltipTrigger>
      {button}
      <Tooltip>{label}</Tooltip>
    </TooltipTrigger>
  );
}

export type CallMicrophoneButtonProps = {
  /** Whether the user's microphone is on now. */
  isOn: boolean;
  /** Asks to turn the microphone on (`true`) or off (`false`). */
  onChange: (on: boolean) => void;
  /** Waits for the microphone to turn on or off: the button keeps its focus and shows the wait. */
  isPending?: boolean;
  /** The names of the two actions ("Turn microphone on", "Turn microphone off"). */
  labels: { turnOn: string; turnOff: string };
  /** Shows the name beside the icon (the room's dock); without it, the name is the tooltip (the mini-player). */
  showLabel?: boolean;
  /** `md` (48 px, default) in the dock, `sm` (44 px) in the mini-player. */
  size?: "md" | "sm";
};

/**
 * The user's own microphone in a call: one shape for each state, the same as on the stage (a filled microphone on a
 * coral button while it is on, the slashed microphone of `MicrophoneOffIcon` on cream while it is off), and a name
 * that says what a press does ("Turn microphone off"). It is a plain button whose name changes with the state, not a
 * toggle with `aria-pressed` (WAI-ARIA APG, button pattern: a toggle keeps its name), so the name never leaves the
 * user to guess whether a press mutes or unmutes.
 */
export function CallMicrophoneButton({
  isOn,
  onChange,
  isPending = false,
  labels,
  showLabel = false,
  size = "md",
}: CallMicrophoneButtonProps) {
  return (
    <CallDockButton
      label={isOn ? labels.turnOff : labels.turnOn}
      icon={isOn ? <MicrophoneIcon filled /> : <MicrophoneOffIcon />}
      showLabel={showLabel}
      size={size}
      variant={isOn ? "primary" : "soft"}
      onPress={() => {
        onChange(!isOn);
      }}
      isPending={isPending}
    />
  );
}
