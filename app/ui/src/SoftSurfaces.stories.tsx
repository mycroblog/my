// SPDX-License-Identifier: AGPL-3.0-only
// The prototypes of the soft surfaces (ADR-0111): a button, a text field, a card and a settings section in each of the
// three directions of src/softDirections.css, next to the same parts in the current style, in both themes like every
// story. The decider picks one direction from the catalogue; the parts here are not exported, and no component uses
// the soft tokens until the chosen direction is applied to them.
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import {
  Button as RACButton,
  type ButtonProps as RACButtonProps,
  composeRenderProps,
  Input,
  Label,
  SwitchButton,
  SwitchField,
  TextField as RACTextField,
} from "react-aria-components";
import { expect } from "storybook/test";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { Card } from "./Card";
import { contrast, parseColor } from "./color";
import { Heading } from "./Heading";
import { Section } from "./Section";
import { Switch } from "./Switch";
import { Hint, Text } from "./Text";
import { TextField } from "./TextField";

type Direction = "quiet" | "soft" | "plush";

// --- The prototype parts ---------------------------------------------------------------------------

// A raised face that sinks while pressed (data-pressed), in duration-fast with ease-standard. The main action stays
// filled with the accent; the other keeps a border-strong boundary (3:1), which the shadow only adds to.
const protoButton = tv({
  base: [
    "inline-flex min-h-10 cursor-default items-center justify-center gap-2 rounded-full px-4 text-base font-extrabold select-none",
    "shadow-soft-raised data-pressed:shadow-soft-inset data-focus-visible:focus-ring pointer-coarse:min-h-11",
    "motion-safe:transition-[box-shadow,background-color,color] motion-safe:duration-fast motion-safe:ease-standard",
  ],
  variants: {
    variant: {
      primary: "bg-accent-fill text-accent-fill-fg data-hovered:bg-accent-fill-hover data-pressed:bg-accent-fill-hover",
      secondary:
        "border border-border-strong bg-surface-raised text-accent data-hovered:text-accent-hover data-pressed:bg-surface-sunken data-pressed:text-accent-hover",
    },
  },
  defaultVariants: { variant: "secondary" },
});

function ProtoButton({ variant, className, ...props }: RACButtonProps & { variant?: "primary" | "secondary" }) {
  return (
    <RACButton
      {...props}
      data-proto-control={variant === "primary" ? undefined : ""}
      className={composeRenderProps(className, (className) => protoButton({ variant, className }))}
    />
  );
}

// A well: the sunken surface with the inner shadow, and the border-strong boundary kept.
function ProtoTextField({ label, description }: { label: string; description?: string }) {
  return (
    <RACTextField className="flex flex-col gap-1" defaultValue="Alice Liddell">
      <Label className="text-sm font-bold text-fg">{label}</Label>
      <Input
        data-proto-control=""
        className="w-full min-w-0 rounded-md border border-border-strong bg-surface-sunken px-3 py-2 text-base text-fg shadow-soft-inset placeholder:text-fg-muted data-focused:focus-ring"
      />
      {description !== undefined && <Hint>{description}</Hint>}
    </RACTextField>
  );
}

// A switch whose groove is sunk with the inner shadow and whose thumb is raised; the colours are today's, so the
// track keeps 3:1 against the surface and the thumb against the track.
function ProtoSwitch({ children, defaultSelected = false }: { children: ReactNode; defaultSelected?: boolean }) {
  return (
    <SwitchField defaultSelected={defaultSelected} className="group">
      <SwitchButton className="flex min-h-6 cursor-default items-center gap-3 text-base text-fg pointer-coarse:min-h-11">
        {({ isSelected, isFocusVisible }) => (
          <>
            <span
              data-proto-control=""
              className={[
                "flex h-6.5 w-11 shrink-0 items-center rounded-full px-1 shadow-soft-inset motion-safe:transition-colors",
                isSelected ? "justify-end bg-accent" : "justify-start bg-border-strong",
                isFocusVisible ? "focus-ring" : "",
              ].join(" ")}
            >
              <span className="size-4.5 rounded-full bg-surface-raised shadow-soft-raised" />
            </span>
            {children}
          </>
        )}
      </SwitchButton>
    </SwitchField>
  );
}

// The faces a card and a settings section sit on: the raised surface, the scoped radius-xl, and the raised shadow
// instead of today's border. Forced-colors mode drops the shadow, so the border comes back there.
const protoCard =
  "rounded-xl bg-surface-raised p-6 shadow-soft-raised forced-colors:border forced-colors:border-[CanvasText]";

function ProtoCard() {
  return (
    <div className={protoCard}>
      <CardContent />
    </div>
  );
}

function ProtoSettingsSection() {
  return (
    <section aria-label="Privacy (prototype)" className={protoCard}>
      <div className="flex flex-col gap-3">
        <Heading level={3} size="section">
          Privacy
        </Heading>
        <Hint>Who can find you and see your posts.</Hint>
        <ProtoSwitch defaultSelected>Require approval for new followers</ProtoSwitch>
        <ProtoSwitch>Show sensitive media</ProtoSwitch>
        <div className="flex flex-wrap gap-2">
          <ProtoButton variant="primary">Save</ProtoButton>
          <ProtoButton>Cancel</ProtoButton>
        </div>
      </div>
    </section>
  );
}

function CardContent() {
  return (
    <div className="flex flex-col gap-2">
      <Text as="p" eyebrow>
        Welcome to my
      </Text>
      <Heading level={3}>A small place for what is on your mind</Heading>
      <Hint>Share small thoughts, safely.</Hint>
    </div>
  );
}

// --- The comparison ---------------------------------------------------------------------------------

function Current() {
  return (
    <div className="flex flex-col gap-6 rounded-lg bg-surface p-4">
      <Heading level={2}>Current</Heading>
      <div className="flex flex-wrap gap-2">
        <Button variant="primary">Post</Button>
        <Button>Cancel</Button>
      </div>
      <TextField label="Display name" description="Shown on your profile." defaultValue="Alice Liddell" />
      <Card>
        <CardContent />
      </Card>
      <Section surface="card" title="Privacy" description="Who can find you and see your posts.">
        <Switch defaultSelected>Require approval for new followers</Switch>
        <Switch>Show sensitive media</Switch>
        <div className="flex flex-wrap gap-2">
          <Button variant="primary">Save</Button>
          <Button>Cancel</Button>
        </div>
      </Section>
    </div>
  );
}

const directionNames: Record<Direction, string> = { quiet: "Quiet", soft: "Soft", plush: "Plush" };

function Prototype({ direction }: { direction: Direction }) {
  return (
    <div data-soft-direction={direction} className="flex flex-col gap-6 rounded-lg bg-surface p-4">
      <Heading level={2}>{directionNames[direction]} (prototype)</Heading>
      <div className="flex flex-wrap gap-2">
        <ProtoButton variant="primary">Post</ProtoButton>
        <ProtoButton>Cancel</ProtoButton>
      </div>
      <ProtoTextField label="Display name" description="Shown on your profile." />
      <ProtoCard />
      <ProtoSettingsSection />
    </div>
  );
}

function Comparison({ direction }: { direction: Direction }) {
  return (
    <div className="@container">
      <div className="grid gap-6 @3xl:grid-cols-2">
        <Current />
        <Prototype direction={direction} />
      </div>
    </div>
  );
}

/** `rgb(r, g, b)` or `rgba(…)`, as getComputedStyle writes a colour, to `#rrggbb`. */
function computedHex(value: string): string {
  const channels =
    /rgba?\(([^)]*)\)/
      .exec(value)?.[1]
      ?.split(/[\s,/]+/)
      .filter(Boolean) ?? [];
  const [r = "0", g = "0", b = "0"] = channels;
  return `#${[r, g, b].map((c) => Math.round(Number.parseFloat(c)).toString(16).padStart(2, "0")).join("")}`;
}

/**
 * Every bordered prototype control and switch track is told apart from the face it sits on by at least 3:1 (WCAG
 * 1.4.11): its border where it has one, otherwise its fill. The shadows add to the boundary and never replace it. The
 * main action is filled with the accent and named by its text, like today's primary Button, so it is not measured.
 */
async function expectControlBoundaries(canvasElement: HTMLElement) {
  const controls = canvasElement.querySelectorAll<HTMLElement>("[data-proto-control]");
  await expect(controls.length).toBeGreaterThan(0);
  for (const control of controls) {
    const style = getComputedStyle(control);
    const hasBorder = Number.parseFloat(style.borderTopWidth) > 0;
    const boundary = computedHex(hasBorder ? style.borderTopColor : style.backgroundColor);
    let ground = control.parentElement;
    while (ground !== null && getComputedStyle(ground).backgroundColor === "rgba(0, 0, 0, 0)")
      ground = ground.parentElement;
    const groundColor = computedHex(getComputedStyle(ground ?? document.body).backgroundColor);
    await expect(contrast(parseColor(boundary), parseColor(groundColor))).toBeGreaterThanOrEqual(3);
  }
}

export default {
  title: "Foundations/Soft surfaces",
} satisfies Meta;

type Story = StoryObj;

/** Faint shadows on today's radius and surface steps. */
export const Quiet: Story = {
  render: () => <Comparison direction="quiet" />,
  play: async ({ canvasElement }) => expectControlBoundaries(canvasElement),
};

/** The shadows of the soft tokens, a rounder card, and a raised face of the page's own colour, lifted by its shadow only. */
export const Soft: Story = {
  render: () => <Comparison direction="soft" />,
  play: async ({ canvasElement }) => expectControlBoundaries(canvasElement),
};

/** Strong shadows, the roundest card, and wider steps between the page, raised faces and wells. */
export const Plush: Story = {
  render: () => <Comparison direction="plush" />,
  play: async ({ canvasElement }) => expectControlBoundaries(canvasElement),
};
