// SPDX-License-Identifier: AGPL-3.0-only

// The switch (React Aria Components' SwitchField and SwitchButton): a setting that is on or off and takes effect as it
// is pressed, with no form to send. Its label and a line that describes it stand at the row's start, the track at its
// end beside both; the label and the track press it. On differs from off by shape and fill, with no hue (the accent
// is the primary action's and the links'): off, a muted thumb at the track's start on the raised surface; on, the
// thumb of the page's color at its end, on a track knocked out of the text's color. The thumb's side follows the
// reading direction. Disabled, the label turns muted, the thumb dims on the track, and the line under the label says
// why; assistive technology reads the line as the switch's description.

import type { ComponentProps, HTMLAttributes, ReactNode } from "react";
import { SwitchButton, SwitchField, Text } from "react-aria-components";
import { tv } from "tailwind-variants/lite";

const toggle = tv({
  slots: {
    root: "text-body text-text",
    // The button's box is its parts', placed in the root's grid; the label and the track take its presses.
    button: "group contents cursor-pointer data-disabled:cursor-not-allowed",
    // The label and its description wrap between phrases, never inside a word of ja (フォロワー); disabled, the label
    // turns muted, as every disabled label does.
    label: "[word-break:auto-phrase] group-data-disabled:text-muted-text",
    description: "text-caption text-muted-text [word-break:auto-phrase]",
    track: [
      "box-border h-toggle-height w-toggle-width",
      "rounded-[calc(var(--toggle-height)/2)] bg-surface-raised-hover group-data-hovered:bg-surface-raised-pressed",
      "group-data-selected:bg-text group-data-selected:group-data-hovered:bg-text",
      "group-data-focus-visible:shadow-focus",
      // Disabled, it keeps its surface, so that it shows in either scheme: on turns muted, and the thumb dims.
      "group-data-disabled:group-data-selected:bg-muted-text",
    ],
    thumb: [
      "size-toggle-thumb rounded-full bg-muted-text",
      "group-data-selected:bg-surface group-data-disabled:opacity-disabled",
    ],
  },
});

/** The switch's own grid: two columns, the label over its description and the track beside both. */
function ToggleFrame({ className, ...rest }: ComponentProps<typeof SwitchField> & { className: string }) {
  return (
    <SwitchField
      {...rest}
      className={`${className} grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-x-toggle-gap`}
    />
  );
}

const PLACE = {
  label: "col-start-1 row-start-1",
  // The track spans both rows, level with the label and its description.
  track: "relative col-start-2 row-span-2 row-start-1 inline-flex",
  // The thumb stands at the track's start, off, and at its end, on; the side follows the reading direction.
  thumb: [
    "absolute inset-y-0 my-auto start-[calc((var(--toggle-height)-var(--toggle-thumb))/2)]",
    "group-data-selected:start-[calc(var(--toggle-width)-(var(--toggle-height)+var(--toggle-thumb))/2)]",
  ].join(" "),
} as const;

/** Where a part of the switch stands in its grid: the label, the track, and the thumb on the track. */
function TogglePlace({
  at,
  className,
  ...rest
}: { at: keyof typeof PLACE; className: string } & HTMLAttributes<HTMLSpanElement>) {
  return <span {...rest} className={`${className} ${PLACE[at]}`} />;
}

/** The line that describes the switch, under its label. */
function ToggleNote({ className, ...rest }: ComponentProps<typeof Text> & { className: string }) {
  return <Text {...rest} className={`${className} col-start-1 row-start-2`} />;
}

export type SwitchProps = {
  /** What it turns on or off. */
  label: string;
  /** A line under the label that says more of it. */
  description?: ReactNode;
  /** Its state, where its owner keeps it, with `onChange`; else it keeps its own from `defaultSelected`. */
  isSelected?: boolean | undefined;
  defaultSelected?: boolean | undefined;
  onChange?: ((on: boolean) => void) | undefined;
  /** It cannot be changed now; `reason` says why, in place of the description. */
  isDisabled?: boolean | undefined;
  reason?: string | undefined;
  /** The name it posts under in a form, when it is in one. */
  name?: string | undefined;
};

/** A setting on or off: its label and description at the row's start, the track at its end. */
export function Switch({
  label,
  description,
  isSelected,
  defaultSelected,
  onChange,
  isDisabled = false,
  reason,
  name,
}: SwitchProps) {
  const s = toggle();
  const below = isDisabled && reason !== undefined ? reason : description;
  return (
    <ToggleFrame
      className={s.root()}
      {...(isSelected === undefined ? {} : { isSelected })}
      {...(defaultSelected === undefined ? {} : { defaultSelected })}
      {...(onChange === undefined ? {} : { onChange })}
      {...(name === undefined ? {} : { name })}
      isDisabled={isDisabled}
    >
      <SwitchButton className={s.button()}>
        <TogglePlace at="label" className={s.label()} dir="auto">
          {label}
        </TogglePlace>
        <TogglePlace at="track" className={s.track()} aria-hidden="true">
          <TogglePlace at="thumb" className={s.thumb()} />
        </TogglePlace>
      </SwitchButton>
      {below !== undefined && (
        <ToggleNote slot="description" className={s.description()} dir="auto">
          {below}
        </ToggleNote>
      )}
    </ToggleFrame>
  );
}
